package cloudimport

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// DAVClient is a minimal WebDAV / CalDAV / CardDAV client (RFC 4918, 4791,
// 6352) with basic auth (app passwords) or a bearer token.
type DAVClient struct {
	HTTP     *http.Client
	User     string
	Password string
	Bearer   string
	Retry    RetryPolicy
}

type davResponse struct {
	Href  string `xml:"href"`
	Props []struct {
		Status string  `xml:"status"`
		Prop   davProp `xml:"prop"`
	} `xml:"propstat"`
}

type davProp struct {
	DisplayName   string `xml:"displayname"`
	ContentType   string `xml:"getcontenttype"`
	ContentLength string `xml:"getcontentlength"`
	ETag          string `xml:"getetag"`
	LastModified  string `xml:"getlastmodified"`
	FileID        string `xml:"fileid"` // Nextcloud/ownCloud oc:fileid
	ResourceType  struct {
		Collection  *struct{} `xml:"collection"`
		Calendar    *struct{} `xml:"calendar"`
		AddressBook *struct{} `xml:"addressbook"`
	} `xml:"resourcetype"`
	CalendarData    string `xml:"calendar-data"`
	AddressData     string `xml:"address-data"`
	CurrentUserPrin struct {
		Href string `xml:"href"`
	} `xml:"current-user-principal"`
	CalendarHome struct {
		Href string `xml:"href"`
	} `xml:"calendar-home-set"`
	AddressHome struct {
		Href string `xml:"href"`
	} `xml:"addressbook-home-set"`
	SupportedComponents struct {
		Comp []struct {
			Name string `xml:"name,attr"`
		} `xml:"comp"`
	} `xml:"supported-calendar-component-set"`
}

type davMultistatus struct {
	Responses []davResponse `xml:"response"`
}

func (r davResponse) ok() (davProp, bool) {
	for _, p := range r.Props {
		if p.Status == "" || strings.Contains(p.Status, " 200") {
			return p.Prop, true
		}
	}
	return davProp{}, false
}

func (c *DAVClient) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (c *DAVClient) auth(r *http.Request) {
	switch {
	case c.Bearer != "":
		r.Header.Set("Authorization", "Bearer "+c.Bearer)
	case c.User != "":
		r.SetBasicAuth(c.User, c.Password)
	}
}

func (c *DAVClient) do(ctx context.Context, method, u, depth, body string) (*http.Response, error) {
	return c.Retry.DoRetry(ctx, c.client(), func() (*http.Request, error) {
		req, err := http.NewRequest(method, u, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		c.auth(req)
		if body != "" {
			req.Header.Set("Content-Type", "application/xml; charset=utf-8")
		}
		if depth != "" {
			req.Header.Set("Depth", depth)
		}
		return req, nil
	})
}

func (c *DAVClient) multistatus(ctx context.Context, method, u, depth, body string) ([]davResponse, error) {
	resp, err := c.do(ctx, method, u, depth, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var ms davMultistatus
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 256<<20)).Decode(&ms); err != nil {
		return nil, fmt.Errorf("%s %s: bad multistatus: %w", method, u, err)
	}
	return ms.Responses, nil
}

// resolve turns an href from a response into an absolute URL on base's host.
func resolve(base, href string) string {
	b, err := url.Parse(base)
	if err != nil {
		return href
	}
	h, err := url.Parse(href)
	if err != nil {
		return href
	}
	return b.ResolveReference(h).String()
}

func samePath(a, b string) bool {
	ua, _ := url.Parse(a)
	ub, _ := url.Parse(b)
	if ua == nil || ub == nil {
		return a == b
	}
	pa, _ := url.PathUnescape(strings.TrimRight(ua.Path, "/"))
	pb, _ := url.PathUnescape(strings.TrimRight(ub.Path, "/"))
	return pa == pb
}

const propfindFiles = `<?xml version="1.0"?>
<d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">
 <d:prop><d:displayname/><d:resourcetype/><d:getcontenttype/><d:getcontentlength/><d:getetag/><d:getlastmodified/><oc:fileid/></d:prop>
</d:propfind>`

// WalkFiles lists root recursively (Depth: 1 per folder, which every server
// supports; Depth: infinity is often disabled) and emits folders before their
// children.
func (c *DAVClient) WalkFiles(ctx context.Context, root string, fn func(RemoteFile) error) error {
	if !strings.HasSuffix(root, "/") {
		root += "/"
	}
	type item struct{ url, id string }
	queue := []item{{root, ""}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		rs, err := c.multistatus(ctx, "PROPFIND", cur.url, "1", propfindFiles)
		if err != nil {
			return err
		}
		for _, r := range rs {
			abs := resolve(cur.url, r.Href)
			if samePath(abs, cur.url) {
				continue
			}
			p, ok := r.ok()
			if !ok {
				continue
			}
			name := p.DisplayName
			if name == "" {
				u, _ := url.Parse(abs)
				name, _ = url.PathUnescape(path.Base(strings.TrimRight(u.Path, "/")))
			}
			id := p.FileID
			if id == "" {
				u, _ := url.Parse(abs)
				id = strings.TrimRight(u.Path, "/")
			}
			f := RemoteFile{ID: id, ParentID: cur.id, Name: name}
			if p.ResourceType.Collection != nil {
				f.IsDir = true
				f.Mime = "folder"
				if err := fn(f); err != nil {
					return err
				}
				if !strings.HasSuffix(abs, "/") {
					abs += "/"
				}
				queue = append(queue, item{abs, id})
				continue
			}
			f.Mime = p.ContentType
			if f.Mime == "" {
				f.Mime = "application/octet-stream"
			}
			f.Size, _ = strconv.ParseInt(p.ContentLength, 10, 64)
			f.Fingerprint = strings.Trim(p.ETag, `"`) + "|" + p.ContentLength
			fileURL := abs
			f.Open = func(ctx context.Context) (io.ReadCloser, error) {
				resp, err := c.do(ctx, http.MethodGet, fileURL, "", "")
				if err != nil {
					return nil, err
				}
				return resp.Body, nil
			}
			if err := fn(f); err != nil {
				return err
			}
		}
	}
	return nil
}

const propfindHome = `<?xml version="1.0"?>
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" xmlns:card="urn:ietf:params:xml:ns:carddav">
 <d:prop><d:current-user-principal/><c:calendar-home-set/><card:addressbook-home-set/></d:prop>
</d:propfind>`

// discoverHome finds the calendar or address-book home: either the URL is
// already a home/collection, or we follow current-user-principal (RFC 5397).
func (c *DAVClient) discoverHome(ctx context.Context, base string, card bool) (string, error) {
	try := []string{base}
	if u, err := url.Parse(base); err == nil && (u.Path == "" || u.Path == "/") {
		if card {
			try = append(try, resolve(base, "/.well-known/carddav"))
		} else {
			try = append(try, resolve(base, "/.well-known/caldav"))
		}
	}
	for _, t := range try {
		rs, err := c.multistatus(ctx, "PROPFIND", t, "0", propfindHome)
		if err != nil {
			continue
		}
		for _, r := range rs {
			p, _ := r.ok()
			if card && p.AddressHome.Href != "" {
				return resolve(t, p.AddressHome.Href), nil
			}
			if !card && p.CalendarHome.Href != "" {
				return resolve(t, p.CalendarHome.Href), nil
			}
			if p.CurrentUserPrin.Href != "" {
				pr := resolve(t, p.CurrentUserPrin.Href)
				rs2, err := c.multistatus(ctx, "PROPFIND", pr, "0", propfindHome)
				if err != nil {
					return "", err
				}
				for _, r2 := range rs2 {
					p2, _ := r2.ok()
					if card && p2.AddressHome.Href != "" {
						return resolve(pr, p2.AddressHome.Href), nil
					}
					if !card && p2.CalendarHome.Href != "" {
						return resolve(pr, p2.CalendarHome.Href), nil
					}
				}
			}
		}
	}
	return base, nil // assume the URL is already the home or a single collection
}

const propfindCollections = `<?xml version="1.0"?>
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
 <d:prop><d:displayname/><d:resourcetype/><c:supported-calendar-component-set/></d:prop>
</d:propfind>`

func (c *DAVClient) collections(ctx context.Context, home string, card bool) ([]string, error) {
	rs, err := c.multistatus(ctx, "PROPFIND", home, "1", propfindCollections)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range rs {
		p, ok := r.ok()
		if !ok {
			continue
		}
		if card && p.ResourceType.AddressBook != nil {
			out = append(out, resolve(home, r.Href))
		}
		if !card && p.ResourceType.Calendar != nil {
			comps := p.SupportedComponents.Comp
			hasEvent := len(comps) == 0
			for _, cp := range comps {
				if strings.EqualFold(cp.Name, "VEVENT") {
					hasEvent = true
				}
			}
			if hasEvent {
				out = append(out, resolve(home, r.Href))
			}
		}
	}
	return out, nil
}

const reportCalendar = `<?xml version="1.0"?>
<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
 <d:prop><d:getetag/><c:calendar-data/></d:prop>
 <c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VEVENT"/></c:comp-filter></c:filter>
</c:calendar-query>`

const reportCards = `<?xml version="1.0"?>
<card:addressbook-query xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
 <d:prop><d:getetag/><card:address-data/></d:prop>
</card:addressbook-query>`

// Events reads every VEVENT from every calendar under base.
func (c *DAVClient) Events(ctx context.Context, base string, fn func(RemoteEvent) error) error {
	home, err := c.discoverHome(ctx, base, false)
	if err != nil {
		return err
	}
	cols, err := c.collections(ctx, home, false)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		cols = []string{home}
	}
	for _, col := range cols {
		rs, err := c.multistatus(ctx, "REPORT", col, "1", reportCalendar)
		if err != nil {
			return err
		}
		for _, r := range rs {
			p, ok := r.ok()
			if !ok || p.CalendarData == "" {
				continue
			}
			evs, err := ParseICS(strings.NewReader(p.CalendarData))
			if err != nil {
				continue
			}
			for i, ev := range evs {
				id := ev.UID
				if id == "" {
					id = r.Href + "#" + strconv.Itoa(i)
				}
				if err := fn(RemoteEvent{ID: id, Fingerprint: p.ETag, Fields: icsToFields(ev)}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Contacts reads every vCard from every address book under base.
func (c *DAVClient) Contacts(ctx context.Context, base string, fn func(RemoteContact) error) error {
	home, err := c.discoverHome(ctx, base, true)
	if err != nil {
		return err
	}
	cols, err := c.collections(ctx, home, true)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		cols = []string{home}
	}
	for _, col := range cols {
		rs, err := c.multistatus(ctx, "REPORT", col, "1", reportCards)
		if err != nil {
			return err
		}
		for _, r := range rs {
			p, ok := r.ok()
			if !ok || p.AddressData == "" {
				continue
			}
			for i, card := range splitVCards(strings.NewReader(p.AddressData)) {
				f := parseVCard(card)
				id := vcardUID(card)
				if id == "" {
					id = r.Href + "#" + strconv.Itoa(i)
				}
				if err := fn(RemoteContact{ID: id, Fingerprint: p.ETag, Fields: f}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func vcardUID(card string) string {
	for _, l := range strings.Split(card, "\n") {
		l = strings.TrimRight(l, "\r")
		if strings.HasPrefix(strings.ToUpper(l), "UID:") {
			return strings.TrimSpace(l[4:])
		}
	}
	return ""
}

func icsToFields(ev ICSEvent) EventFields {
	end := ev.EndAt
	if end.IsZero() {
		end = ev.StartAt.Add(time.Hour)
		if ev.AllDay {
			end = ev.StartAt.Add(24 * time.Hour)
		}
	}
	return EventFields{Title: ev.Summary, Description: ev.Description, Location: ev.Location,
		StartAt: ev.StartAt, EndAt: end, AllDay: ev.AllDay, Recurrence: ev.RRule}
}

// bytesReadCloser is a helper for fixture-backed sources.
func bytesReadCloser(b []byte) io.ReadCloser { return io.NopCloser(bytes.NewReader(b)) }
