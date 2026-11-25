package cloudimport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// MicrosoftSource migrates from Microsoft 365 via Microsoft Graph.
//
// Scope "org": app-only token (client credentials, admin-consented
// application permissions User.Read.All, Group.Read.All, Files.Read.All,
// Sites.Read.All, Mail.Read, Calendars.Read, Contacts.Read) reads every user.
// Scope "me": a delegated token for the signed-in person, paths use /me.
type MicrosoftSource struct {
	Unsupported
	Graph string // https://graph.microsoft.com
	HTTP  *http.Client
	Token oauth2.TokenSource
	Self  string
	Retry RetryPolicy
}

func (m *MicrosoftSource) Capabilities() []Kind {
	// Teams chat export is a Microsoft "protected API" that needs Microsoft's
	// approval and is metered; it is not offered.
	return []Kind{MigUsers, MigGroups, MigFiles, MigMail, MigCalendar, MigContacts}
}

func (m *MicrosoftSource) api(ctx context.Context) *apiClient {
	a := newAPIClient(ctx, m.Token, m.Graph, m.HTTP)
	a.retry = m.Retry
	return a
}

func userPath(u RemoteUser) string {
	if u.ID == "me" {
		return "/v1.0/me"
	}
	return "/v1.0/users/" + url.PathEscape(u.ID)
}

// pages follows @odata.nextLink.
func pages[T any](ctx context.Context, a *apiClient, first string, q url.Values, fn func(T) error) error {
	next := a.url(first, q)
	for next != "" {
		var r struct {
			Value []T    `json:"value"`
			Next  string `json:"@odata.nextLink"`
		}
		if err := a.getJSON(ctx, next, nil, &r); err != nil {
			return err
		}
		for _, v := range r.Value {
			if err := fn(v); err != nil {
				return err
			}
		}
		next = r.Next
	}
	return nil
}

type msUser struct {
	ID   string `json:"id"`
	Mail string `json:"mail"`
	UPN  string `json:"userPrincipalName"`
	Name string `json:"displayName"`
}

func (u msUser) email() string {
	if u.Mail != "" {
		return u.Mail
	}
	return u.UPN
}

func (m *MicrosoftSource) Users(ctx context.Context, fn func(RemoteUser) error) error {
	return pages(ctx, m.api(ctx), "/v1.0/users", url.Values{"$select": {"id,mail,userPrincipalName,displayName"}, "$top": {"999"}},
		func(u msUser) error { return fn(RemoteUser{ID: u.ID, Email: u.email(), Name: u.Name}) })
}

func (m *MicrosoftSource) Groups(ctx context.Context, fn func(RemoteGroup) error) error {
	a := m.api(ctx)
	return pages(ctx, a, "/v1.0/groups", url.Values{"$select": {"id,displayName,mail"}, "$top": {"999"}}, func(g struct {
		ID   string `json:"id"`
		Name string `json:"displayName"`
		Mail string `json:"mail"`
	}) error {
		rg := RemoteGroup{ID: g.ID, Name: g.Name, Email: g.Mail}
		if err := pages(ctx, a, "/v1.0/groups/"+url.PathEscape(g.ID)+"/members", url.Values{"$select": {"mail,userPrincipalName"}}, func(u msUser) error {
			if e := u.email(); e != "" {
				rg.Members = append(rg.Members, e)
			}
			return nil
		}); err != nil {
			return err
		}
		return fn(rg)
	})
}

type msItem struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Size   int64     `json:"size"`
	ETag   string    `json:"eTag"`
	Folder *struct{} `json:"folder"`
	File   *struct {
		MimeType string `json:"mimeType"`
	} `json:"file"`
	Shared *struct{} `json:"shared"`
	DL     string    `json:"@microsoft.graph.downloadUrl"`
}

func (m *MicrosoftSource) walk(ctx context.Context, a *apiClient, drivePath, rootParent, driveName, owner string, fn func(RemoteFile) error) error {
	type item struct{ path, parent string }
	queue := []item{{drivePath + "/root/children", rootParent}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		err := pages(ctx, a, cur.path, url.Values{"$top": {"999"}}, func(it msItem) error {
			rf := RemoteFile{ID: it.ID, ParentID: cur.parent, Name: it.Name, Drive: driveName, Fingerprint: it.ETag}
			if it.Folder != nil {
				rf.IsDir, rf.Mime = true, "folder"
				queue = append(queue, item{drivePath + "/items/" + url.PathEscape(it.ID) + "/children", it.ID})
				return fn(rf)
			}
			rf.Size = it.Size
			rf.Mime = "application/octet-stream"
			if it.File != nil && it.File.MimeType != "" {
				rf.Mime = it.File.MimeType
			}
			id := it.ID
			rf.Open = func(ctx context.Context) (io.ReadCloser, error) {
				return a.open(ctx, drivePath+"/items/"+url.PathEscape(id)+"/content", nil)
			}
			if it.Shared != nil {
				_ = pages(ctx, a, drivePath+"/items/"+url.PathEscape(id)+"/permissions", nil, func(p struct {
					Roles     []string `json:"roles"`
					GrantedTo struct {
						User struct {
							Email string `json:"email"`
						} `json:"user"`
					} `json:"grantedToV2"`
				}) error {
					e := p.GrantedTo.User.Email
					if e == "" || strings.EqualFold(e, owner) {
						return nil
					}
					role := "viewer"
					for _, r := range p.Roles {
						if r == "write" {
							role = "editor"
						}
						if r == "owner" {
							return nil
						}
					}
					rf.Shares = append(rf.Shares, RemoteShare{Email: e, Role: role})
					return nil
				})
			}
			return fn(rf)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Files walks the user's OneDrive. SharePoint document libraries (the Microsoft
// equivalent of shared drives) are read once, for the connecting admin's
// org-scope run or the "me" user, under a top-level folder per site.
func (m *MicrosoftSource) Files(ctx context.Context, u RemoteUser, fn func(RemoteFile) error) error {
	a := m.api(ctx)
	if err := m.walk(ctx, a, userPath(u)+"/drive", "", "", u.Email, fn); err != nil {
		var he *HTTPError
		if asHTTP(err, &he) && he.Status == 404 {
			return nil // user has no OneDrive provisioned
		}
		return err
	}
	if u.ID != "me" && !strings.EqualFold(u.Email, m.Self) {
		return nil
	}
	return pages(ctx, a, "/v1.0/sites", url.Values{"search": {"*"}}, func(s struct {
		ID   string `json:"id"`
		Name string `json:"displayName"`
	}) error {
		top := "site:" + s.ID
		if err := fn(RemoteFile{ID: top, Name: s.Name + " (SharePoint)", IsDir: true, Mime: "folder", Drive: s.Name}); err != nil {
			return err
		}
		return m.walk(ctx, a, "/v1.0/sites/"+url.PathEscape(s.ID)+"/drive", top, s.Name, u.Email, fn)
	})
}

type msFolder struct {
	ID       string `json:"id"`
	Name     string `json:"displayName"`
	Children int    `json:"childFolderCount"`
	WK       string `json:"wellKnownName"`
}

func (m *MicrosoftSource) Mail(ctx context.Context, u RemoteUser, fn func(RemoteMail) error) error {
	a := m.api(ctx)
	base := userPath(u)
	var walk func(path, prefix string) error
	walk = func(path, prefix string) error {
		return pages(ctx, a, path, url.Values{"$top": {"100"}, "includeHiddenFolders": {"false"}}, func(f msFolder) error {
			name := f.Name
			if prefix != "" {
				name = prefix + "/" + f.Name
			}
			err := pages(ctx, a, base+"/mailFolders/"+url.PathEscape(f.ID)+"/messages", url.Values{"$select": {"id,isRead,flag,categories"}, "$top": {"100"}}, func(msg struct {
				ID     string   `json:"id"`
				IsRead bool     `json:"isRead"`
				Cats   []string `json:"categories"`
				Flag   struct {
					Status string `json:"flagStatus"`
				} `json:"flag"`
			}) error {
				rc, err := a.open(ctx, base+"/messages/"+url.PathEscape(msg.ID)+"/$value", nil)
				if err != nil {
					return err
				}
				raw, err := readAllLimit(rc, 50<<20)
				rc.Close()
				if err != nil {
					return err
				}
				return fn(RemoteMail{ID: msg.ID, Folder: name, Labels: msg.Cats, Read: msg.IsRead, Starred: msg.Flag.Status == "flagged", Raw: raw})
			})
			if err != nil {
				return err
			}
			if f.Children > 0 {
				return walk(base+"/mailFolders/"+url.PathEscape(f.ID)+"/childFolders", name)
			}
			return nil
		})
	}
	return walk(base+"/mailFolders", "")
}

func msTime(dt struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}) time.Time {
	loc := time.UTC
	if dt.TimeZone != "" && dt.TimeZone != "UTC" {
		if l, err := time.LoadLocation(dt.TimeZone); err == nil {
			loc = l
		}
	}
	for _, layout := range []string{"2006-01-02T15:04:05.0000000", "2006-01-02T15:04:05", time.RFC3339Nano} {
		if t, err := time.ParseInLocation(layout, dt.DateTime, loc); err == nil {
			return t
		}
	}
	return time.Time{}
}

// msRRule maps the common Graph recurrence patterns to an RRULE. Patterns it
// cannot express exactly return "" and the event is imported as a single
// occurrence; the error report says so.
func msRRule(p struct {
	Type       string   `json:"type"`
	Interval   int      `json:"interval"`
	DaysOfWeek []string `json:"daysOfWeek"`
}, r struct {
	Type    string `json:"type"`
	EndDate string `json:"endDate"`
	Count   int    `json:"numberOfOccurrences"`
}) string {
	freq := map[string]string{"daily": "DAILY", "weekly": "WEEKLY", "absoluteMonthly": "MONTHLY", "absoluteYearly": "YEARLY"}[p.Type]
	if freq == "" {
		return ""
	}
	s := "FREQ=" + freq
	if p.Interval > 1 {
		s += fmt.Sprintf(";INTERVAL=%d", p.Interval)
	}
	if freq == "WEEKLY" && len(p.DaysOfWeek) > 0 {
		var d []string
		for _, w := range p.DaysOfWeek {
			if len(w) >= 2 {
				d = append(d, strings.ToUpper(w[:2]))
			}
		}
		s += ";BYDAY=" + strings.Join(d, ",")
	}
	switch r.Type {
	case "numbered":
		s += fmt.Sprintf(";COUNT=%d", r.Count)
	case "endDate":
		if t, err := time.Parse("2006-01-02", r.EndDate); err == nil {
			s += ";UNTIL=" + t.Format("20060102") + "T235959Z"
		}
	}
	return s
}

func (m *MicrosoftSource) Events(ctx context.Context, u RemoteUser, fn func(RemoteEvent) error) error {
	type dt = struct {
		DateTime string `json:"dateTime"`
		TimeZone string `json:"timeZone"`
	}
	return pages(ctx, m.api(ctx), userPath(u)+"/events", url.Values{"$top": {"100"}}, func(e struct {
		ID        string `json:"id"`
		ChangeKey string `json:"changeKey"`
		Subject   string `json:"subject"`
		Body      struct {
			ContentType string `json:"contentType"`
			Content     string `json:"content"`
		} `json:"body"`
		Location struct {
			Name string `json:"displayName"`
		} `json:"location"`
		Start      dt   `json:"start"`
		End        dt   `json:"end"`
		AllDay     bool `json:"isAllDay"`
		Cancelled  bool `json:"isCancelled"`
		Recurrence *struct {
			Pattern struct {
				Type       string   `json:"type"`
				Interval   int      `json:"interval"`
				DaysOfWeek []string `json:"daysOfWeek"`
			} `json:"pattern"`
			Range struct {
				Type    string `json:"type"`
				EndDate string `json:"endDate"`
				Count   int    `json:"numberOfOccurrences"`
			} `json:"range"`
		} `json:"recurrence"`
	}) error {
		if e.Cancelled {
			return nil
		}
		desc := e.Body.Content
		if strings.EqualFold(e.Body.ContentType, "html") {
			desc = htmlToText(desc)
		}
		rr := ""
		if e.Recurrence != nil {
			rr = msRRule(e.Recurrence.Pattern, e.Recurrence.Range)
		}
		return fn(RemoteEvent{ID: e.ID, Fingerprint: e.ChangeKey, Fields: EventFields{
			Title: e.Subject, Description: desc, Location: e.Location.Name,
			StartAt: msTime(e.Start), EndAt: msTime(e.End), AllDay: e.AllDay, Recurrence: rr}})
	})
}

func (m *MicrosoftSource) Contacts(ctx context.Context, u RemoteUser, fn func(RemoteContact) error) error {
	return pages(ctx, m.api(ctx), userPath(u)+"/contacts", url.Values{"$top": {"500"}}, func(c struct {
		ID        string   `json:"id"`
		ChangeKey string   `json:"changeKey"`
		Name      string   `json:"displayName"`
		Given     string   `json:"givenName"`
		Surname   string   `json:"surname"`
		Company   string   `json:"companyName"`
		Title     string   `json:"jobTitle"`
		Business  []string `json:"businessPhones"`
		Home      []string `json:"homePhones"`
		Mobile    string   `json:"mobilePhone"`
		Notes     string   `json:"personalNotes"`
		Cats      []string `json:"categories"`
		Emails    []struct {
			Address string `json:"address"`
		} `json:"emailAddresses"`
	}) error {
		f := ContactFields{DisplayName: c.Name, FirstName: c.Given, LastName: c.Surname, Company: c.Company, JobTitle: c.Title, Notes: c.Notes, Labels: c.Cats}
		for _, e := range c.Emails {
			f.Emails = append(f.Emails, e.Address)
		}
		if c.Mobile != "" {
			f.Phones = append(f.Phones, c.Mobile)
		}
		f.Phones = append(append(f.Phones, c.Business...), c.Home...)
		return fn(RemoteContact{ID: c.ID, Fingerprint: c.ChangeKey, Fields: f})
	})
}
