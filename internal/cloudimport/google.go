package cloudimport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// GoogleEndpoints are the API roots. Tests point them at a local fake server.
type GoogleEndpoints struct {
	Directory string // https://admin.googleapis.com
	Drive     string // https://www.googleapis.com
	Calendar  string // https://www.googleapis.com
	People    string // https://people.googleapis.com
	Chat      string // https://chat.googleapis.com
	IMAPAddr  string // imap.gmail.com:993
	IMAPSec   string // tls
}

// DefaultGoogleEndpoints are the real Google APIs.
var DefaultGoogleEndpoints = GoogleEndpoints{
	Directory: "https://admin.googleapis.com", Drive: "https://www.googleapis.com",
	Calendar: "https://www.googleapis.com", People: "https://people.googleapis.com",
	Chat: "https://chat.googleapis.com", IMAPAddr: "imap.gmail.com:993", IMAPSec: "tls",
}

// GoogleScopes are requested in the OAuth consent (scope "me", or an admin
// for the directory). All read-only except mail.google.com, which is the only
// scope Gmail IMAP accepts; Adrient never writes to the source mailbox (folders
// are SELECTed read-only and bodies fetched with PEEK).
var GoogleScopes = []string{
	"openid", "email",
	"https://www.googleapis.com/auth/admin.directory.user.readonly",
	"https://www.googleapis.com/auth/admin.directory.group.readonly",
	"https://www.googleapis.com/auth/admin.directory.group.member.readonly",
	"https://www.googleapis.com/auth/drive.readonly",
	"https://www.googleapis.com/auth/calendar.readonly",
	"https://www.googleapis.com/auth/contacts.readonly",
	"https://www.googleapis.com/auth/chat.messages.readonly",
	"https://www.googleapis.com/auth/chat.spaces.readonly",
	"https://mail.google.com/",
}

// GoogleSource migrates from Google Workspace / Gmail.
//
// Scope "me": one OAuth token (the connecting person's own account).
// Scope "org": Directory listing with the admin token; per-user data needs a
// service account with domain-wide delegation (UserToken impersonates each
// user). Without it only users/groups are available org-wide.
type GoogleSource struct {
	Unsupported
	EP        GoogleEndpoints
	HTTP      *http.Client
	Token     oauth2.TokenSource                    // admin or self token
	UserToken func(email string) oauth2.TokenSource // nil = no delegation
	Self      string                                // e-mail of the token owner
	Retry     RetryPolicy
}

func (g *GoogleSource) Capabilities() []Kind {
	return []Kind{MigUsers, MigGroups, MigFiles, MigMail, MigCalendar, MigContacts, MigChat}
}

func (g *GoogleSource) tokenFor(u RemoteUser) (oauth2.TokenSource, error) {
	if u.ID == "me" || strings.EqualFold(u.Email, g.Self) || g.UserToken == nil {
		if u.ID != "me" && !strings.EqualFold(u.Email, g.Self) {
			return nil, fmt.Errorf("reading %s needs a service account with domain-wide delegation (not configured)", u.Email)
		}
		return g.Token, nil
	}
	return g.UserToken(u.Email), nil
}

func (g *GoogleSource) api(ctx context.Context, ts oauth2.TokenSource, base string) *apiClient {
	a := newAPIClient(ctx, ts, base, g.HTTP)
	a.retry = g.Retry
	return a
}

func (g *GoogleSource) Users(ctx context.Context, fn func(RemoteUser) error) error {
	a := g.api(ctx, g.Token, g.EP.Directory)
	q := url.Values{"customer": {"my_customer"}, "maxResults": {"500"}}
	tok := ""
	for {
		var r struct {
			Users []struct {
				ID           string `json:"id"`
				PrimaryEmail string `json:"primaryEmail"`
				Suspended    bool   `json:"suspended"`
				Name         struct {
					FullName string `json:"fullName"`
				} `json:"name"`
			} `json:"users"`
			Next string `json:"nextPageToken"`
		}
		if err := a.getJSON(ctx, "/admin/directory/v1/users", cloneQ(q, "pageToken", tok), &r); err != nil {
			return err
		}
		for _, u := range r.Users {
			if err := fn(RemoteUser{ID: u.ID, Email: u.PrimaryEmail, Name: u.Name.FullName}); err != nil {
				return err
			}
		}
		if tok = r.Next; tok == "" {
			return nil
		}
	}
}

func (g *GoogleSource) Groups(ctx context.Context, fn func(RemoteGroup) error) error {
	a := g.api(ctx, g.Token, g.EP.Directory)
	q := url.Values{"customer": {"my_customer"}, "maxResults": {"200"}}
	tok := ""
	for {
		var r struct {
			Groups []struct{ ID, Email, Name string } `json:"groups"`
			Next   string                             `json:"nextPageToken"`
		}
		if err := a.getJSON(ctx, "/admin/directory/v1/groups", cloneQ(q, "pageToken", tok), &r); err != nil {
			return err
		}
		for _, gr := range r.Groups {
			rg := RemoteGroup{ID: gr.ID, Email: gr.Email, Name: gr.Name}
			mt := ""
			for {
				var m struct {
					Members []struct{ Email string } `json:"members"`
					Next    string                   `json:"nextPageToken"`
				}
				if err := a.getJSON(ctx, "/admin/directory/v1/groups/"+url.PathEscape(gr.ID)+"/members", cloneQ(url.Values{"maxResults": {"200"}}, "pageToken", mt), &m); err != nil {
					return err
				}
				for _, x := range m.Members {
					rg.Members = append(rg.Members, x.Email)
				}
				if mt = m.Next; mt == "" {
					break
				}
			}
			if err := fn(rg); err != nil {
				return err
			}
		}
		if tok = r.Next; tok == "" {
			return nil
		}
	}
}

// Google-native documents have no bytes; they are exported to Office formats.
var googleExport = map[string][2]string{
	"application/vnd.google-apps.document":     {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
	"application/vnd.google-apps.spreadsheet":  {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
	"application/vnd.google-apps.presentation": {"application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"},
	"application/vnd.google-apps.drawing":      {"image/png", ".png"},
}

type gFile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	Size         string `json:"size"`
	MD5          string `json:"md5Checksum"`
	ModifiedTime string `json:"modifiedTime"`
	Permissions  []struct {
		Type         string `json:"type"`
		Role         string `json:"role"`
		EmailAddress string `json:"emailAddress"`
	} `json:"permissions"`
}

func (g *GoogleSource) walkDrive(ctx context.Context, a *apiClient, rootID, rootParent, driveName, driveID string, owner string, fn func(RemoteFile) error) error {
	type item struct{ id, parent string }
	queue := []item{{rootID, rootParent}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		q := url.Values{
			"q":                         {fmt.Sprintf("'%s' in parents and trashed=false", strings.ReplaceAll(cur.id, "'", ""))},
			"fields":                    {"nextPageToken,files(id,name,mimeType,size,md5Checksum,modifiedTime,permissions(type,role,emailAddress))"},
			"pageSize":                  {"1000"},
			"supportsAllDrives":         {"true"},
			"includeItemsFromAllDrives": {"true"},
		}
		if driveID != "" {
			q.Set("corpora", "drive")
			q.Set("driveId", driveID)
		}
		tok := ""
		for {
			var r struct {
				Files []gFile `json:"files"`
				Next  string  `json:"nextPageToken"`
			}
			if err := a.getJSON(ctx, "/drive/v3/files", cloneQ(q, "pageToken", tok), &r); err != nil {
				return err
			}
			for _, f := range r.Files {
				rf := RemoteFile{ID: f.ID, ParentID: cur.parent, Name: f.Name, Mime: f.MimeType, Drive: driveName,
					Fingerprint: f.MD5 + "|" + f.ModifiedTime}
				for _, p := range f.Permissions {
					if p.Type == "user" && p.Role != "owner" && !strings.EqualFold(p.EmailAddress, owner) {
						rf.Shares = append(rf.Shares, RemoteShare{Email: p.EmailAddress, Role: p.Role})
					}
				}
				if f.MimeType == "application/vnd.google-apps.folder" {
					rf.IsDir, rf.Mime = true, "folder"
					if err := fn(rf); err != nil {
						return err
					}
					queue = append(queue, item{f.ID, f.ID})
					continue
				}
				id := f.ID
				if ex, ok := googleExport[f.MimeType]; ok {
					rf.Mime = ex[0]
					if !strings.HasSuffix(strings.ToLower(rf.Name), ex[1]) {
						rf.Name += ex[1]
					}
					mt := ex[0]
					rf.Open = func(ctx context.Context) (io.ReadCloser, error) {
						return a.open(ctx, "/drive/v3/files/"+url.PathEscape(id)+"/export", url.Values{"mimeType": {mt}})
					}
				} else if strings.HasPrefix(f.MimeType, "application/vnd.google-apps.") {
					// Forms, sites, maps, shortcuts: no export format. Reported, not faked.
					rf.Open = nil
				} else {
					rf.Size, _ = strconv.ParseInt(f.Size, 10, 64)
					rf.Open = func(ctx context.Context) (io.ReadCloser, error) {
						return a.open(ctx, "/drive/v3/files/"+url.PathEscape(id), url.Values{"alt": {"media"}, "supportsAllDrives": {"true"}})
					}
				}
				if err := fn(rf); err != nil {
					return err
				}
			}
			if tok = r.Next; tok == "" {
				break
			}
		}
	}
	return nil
}

// Files walks My Drive, then every shared drive the user can see. Each shared
// drive becomes a top-level folder named after it.
func (g *GoogleSource) Files(ctx context.Context, u RemoteUser, fn func(RemoteFile) error) error {
	ts, err := g.tokenFor(u)
	if err != nil {
		return err
	}
	a := g.api(ctx, ts, g.EP.Drive)
	if err := g.walkDrive(ctx, a, "root", "", "", "", u.Email, fn); err != nil {
		return err
	}
	tok := ""
	for {
		var r struct {
			Drives []struct{ ID, Name string } `json:"drives"`
			Next   string                      `json:"nextPageToken"`
		}
		if err := a.getJSON(ctx, "/drive/v3/drives", cloneQ(url.Values{"pageSize": {"100"}}, "pageToken", tok), &r); err != nil {
			return err
		}
		for _, d := range r.Drives {
			top := "shared-drive:" + d.ID
			if err := fn(RemoteFile{ID: top, Name: d.Name + " (shared drive)", IsDir: true, Mime: "folder", Drive: d.Name}); err != nil {
				return err
			}
			if err := g.walkDrive(ctx, a, d.ID, top, d.Name, d.ID, u.Email, fn); err != nil {
				return err
			}
		}
		if tok = r.Next; tok == "" {
			return nil
		}
	}
}

type gTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

func (t gTime) parse() (time.Time, bool) {
	if t.DateTime != "" {
		v, _ := time.Parse(time.RFC3339, t.DateTime)
		return v, false
	}
	v, _ := time.Parse("2006-01-02", t.Date)
	return v, true
}

func (g *GoogleSource) Events(ctx context.Context, u RemoteUser, fn func(RemoteEvent) error) error {
	ts, err := g.tokenFor(u)
	if err != nil {
		return err
	}
	a := g.api(ctx, ts, g.EP.Calendar)
	var cl struct {
		Items []struct {
			ID         string `json:"id"`
			AccessRole string `json:"accessRole"`
		} `json:"items"`
	}
	if err := a.getJSON(ctx, "/calendar/v3/users/me/calendarList", nil, &cl); err != nil {
		return err
	}
	for _, c := range cl.Items {
		if c.AccessRole != "owner" && c.AccessRole != "" {
			continue // subscribed/other people's calendars are theirs to migrate
		}
		tok := ""
		for {
			var r struct {
				Items []struct {
					ID          string   `json:"id"`
					ETag        string   `json:"etag"`
					Status      string   `json:"status"`
					Summary     string   `json:"summary"`
					Description string   `json:"description"`
					Location    string   `json:"location"`
					Start       gTime    `json:"start"`
					End         gTime    `json:"end"`
					Recurrence  []string `json:"recurrence"`
				} `json:"items"`
				Next string `json:"nextPageToken"`
			}
			if err := a.getJSON(ctx, "/calendar/v3/calendars/"+url.PathEscape(c.ID)+"/events", cloneQ(url.Values{"maxResults": {"2500"}, "singleEvents": {"false"}}, "pageToken", tok), &r); err != nil {
				return err
			}
			for _, e := range r.Items {
				if e.Status == "cancelled" {
					continue
				}
				st, allDay := e.Start.parse()
				en, _ := e.End.parse()
				rr := ""
				for _, x := range e.Recurrence {
					if strings.HasPrefix(x, "RRULE:") {
						rr = strings.TrimPrefix(x, "RRULE:")
					}
				}
				if err := fn(RemoteEvent{ID: c.ID + "/" + e.ID, Fingerprint: e.ETag, Fields: EventFields{
					Title: e.Summary, Description: e.Description, Location: e.Location,
					StartAt: st, EndAt: en, AllDay: allDay, Recurrence: rr}}); err != nil {
					return err
				}
			}
			if tok = r.Next; tok == "" {
				break
			}
		}
	}
	return nil
}

func (g *GoogleSource) Contacts(ctx context.Context, u RemoteUser, fn func(RemoteContact) error) error {
	ts, err := g.tokenFor(u)
	if err != nil {
		return err
	}
	a := g.api(ctx, ts, g.EP.People)
	q := url.Values{"personFields": {"names,emailAddresses,phoneNumbers,organizations,biographies,memberships"}, "pageSize": {"1000"}}
	tok := ""
	for {
		var r struct {
			Connections []struct {
				ResourceName string `json:"resourceName"`
				ETag         string `json:"etag"`
				Names        []struct {
					DisplayName, GivenName, FamilyName string
				} `json:"names"`
				Emails []struct{ Value string }       `json:"emailAddresses"`
				Phones []struct{ Value string }       `json:"phoneNumbers"`
				Orgs   []struct{ Name, Title string } `json:"organizations"`
				Bios   []struct{ Value string }       `json:"biographies"`
			} `json:"connections"`
			Next string `json:"nextPageToken"`
		}
		if err := a.getJSON(ctx, "/v1/people/me/connections", cloneQ(q, "pageToken", tok), &r); err != nil {
			return err
		}
		for _, p := range r.Connections {
			var f ContactFields
			if len(p.Names) > 0 {
				f.DisplayName, f.FirstName, f.LastName = p.Names[0].DisplayName, p.Names[0].GivenName, p.Names[0].FamilyName
			}
			for _, e := range p.Emails {
				f.Emails = append(f.Emails, e.Value)
			}
			for _, ph := range p.Phones {
				f.Phones = append(f.Phones, ph.Value)
			}
			if len(p.Orgs) > 0 {
				f.Company, f.JobTitle = p.Orgs[0].Name, p.Orgs[0].Title
			}
			if len(p.Bios) > 0 {
				f.Notes = p.Bios[0].Value
			}
			if f.DisplayName == "" && len(f.Emails) > 0 {
				f.DisplayName = f.Emails[0]
			}
			if err := fn(RemoteContact{ID: p.ResourceName, Fingerprint: p.ETag, Fields: f}); err != nil {
				return err
			}
		}
		if tok = r.Next; tok == "" {
			return nil
		}
	}
}

// Mail reads Gmail over IMAP with XOAUTH2. Gmail labels are IMAP folders; the
// "All Mail" virtual folder is skipped and a message that carries several
// labels is imported once per label folder, then collapsed by Message-ID.
func (g *GoogleSource) Mail(ctx context.Context, u RemoteUser, fn func(RemoteMail) error) error {
	ts, err := g.tokenFor(u)
	if err != nil {
		return err
	}
	tok, err := ts.Token()
	if err != nil {
		return fmt.Errorf("google token: %w", err)
	}
	email := u.Email
	if u.ID == "me" {
		email = g.Self
	}
	return IMAPMail(ctx, IMAPConfig{Addr: g.EP.IMAPAddr, Security: g.EP.IMAPSec, Username: email, AccessToken: tok.AccessToken, GmailLabels: true}, fn)
}

// Chat reads Google Chat spaces the user is a member of (Chat API). Only
// scope "me": the Chat API has no admin export of other people's messages.
func (g *GoogleSource) Chat(ctx context.Context, u RemoteUser, fn func(RemoteChat) error) error {
	if u.ID != "me" && !strings.EqualFold(u.Email, g.Self) {
		return ErrUnsupported
	}
	a := g.api(ctx, g.Token, g.EP.Chat)
	tok := ""
	for {
		var r struct {
			Spaces []struct{ Name, DisplayName, SpaceType string } `json:"spaces"`
			Next   string                                          `json:"nextPageToken"`
		}
		if err := a.getJSON(ctx, "/v1/spaces", cloneQ(url.Values{"pageSize": {"1000"}}, "pageToken", tok), &r); err != nil {
			return err
		}
		for _, s := range r.Spaces {
			name := s.DisplayName
			if name == "" {
				name = "Google Chat " + strings.TrimPrefix(s.Name, "spaces/")
			}
			mt := ""
			for {
				var m struct {
					Messages []struct {
						Name       string `json:"name"`
						Text       string `json:"text"`
						CreateTime string `json:"createTime"`
						Sender     struct {
							DisplayName string `json:"displayName"`
						} `json:"sender"`
					} `json:"messages"`
					Next string `json:"nextPageToken"`
				}
				if err := a.getJSON(ctx, "/v1/"+s.Name+"/messages", cloneQ(url.Values{"pageSize": {"1000"}}, "pageToken", mt), &m); err != nil {
					return err
				}
				for _, x := range m.Messages {
					at, _ := time.Parse(time.RFC3339Nano, x.CreateTime)
					if err := fn(RemoteChat{ID: x.Name, Space: name, Author: x.Sender.DisplayName, Text: x.Text, At: at}); err != nil {
						return err
					}
				}
				if mt = m.Next; mt == "" {
					break
				}
			}
		}
		if tok = r.Next; tok == "" {
			return nil
		}
	}
}
