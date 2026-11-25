package cloudimport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeNextcloud answers the PROPFIND/REPORT requests a Nextcloud 28 server
// answers, with response bodies shaped like its sabre/dav output.
func newFakeNextcloud(t *testing.T, user, pass string) *httptest.Server {
	files := map[string]string{
		"/remote.php/dav/files/ann/Documents/report.txt": "quarterly",
		"/remote.php/dav/files/ann/photo.jpg":            "JPEGDATA",
	}
	dirs := map[string][]string{
		"/remote.php/dav/files/ann/":           {"/remote.php/dav/files/ann/Documents/", "/remote.php/dav/files/ann/photo.jpg"},
		"/remote.php/dav/files/ann/Documents/": {"/remote.php/dav/files/ann/Documents/report.txt"},
	}
	propResp := func(href string) string {
		if c, ok := files[href]; ok {
			return fmt.Sprintf(`<d:response><d:href>%s</d:href><d:propstat><d:prop><d:getcontentlength>%d</d:getcontentlength><d:getcontenttype>text/plain</d:getcontenttype><d:getetag>"et-%d"</d:getetag><oc:fileid>%d</oc:fileid><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, href, len(c), len(c), len(href))
		}
		return fmt.Sprintf(`<d:response><d:href>%s</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype><oc:fileid>%d</oc:fileid></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat><d:propstat><d:prop><d:getcontentlength/></d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat></d:response>`, href, len(href))
	}
	ms := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(207)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns" xmlns:cal="urn:ietf:params:xml:ns:caldav" xmlns:card="urn:ietf:params:xml:ns:carddav">`+body+`</d:multistatus>`)
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.WriteHeader(401)
			return
		}
		path := r.URL.Path
		switch {
		case r.Method == "GET":
			if c, ok := files[path]; ok {
				_, _ = io.WriteString(w, c)
				return
			}
			w.WriteHeader(404)
		case r.Method == "PROPFIND" && strings.HasPrefix(path, "/remote.php/dav/files/"):
			kids, ok := dirs[path]
			if !ok {
				w.WriteHeader(404)
				return
			}
			body := propResp(path)
			if r.Header.Get("Depth") == "1" {
				for _, k := range kids {
					body += propResp(k)
				}
			}
			ms(w, body)
		case r.Method == "PROPFIND" && path == "/remote.php/dav/calendars/ann/":
			if r.Header.Get("Depth") == "0" {
				ms(w, `<d:response><d:href>/remote.php/dav/calendars/ann/</d:href><d:propstat><d:prop><cal:calendar-home-set><d:href>/remote.php/dav/calendars/ann/</d:href></cal:calendar-home-set></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
				return
			}
			ms(w, `<d:response><d:href>/remote.php/dav/calendars/ann/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
<d:response><d:href>/remote.php/dav/calendars/ann/personal/</d:href><d:propstat><d:prop><d:displayname>Personal</d:displayname><d:resourcetype><d:collection/><cal:calendar/></d:resourcetype><cal:supported-calendar-component-set><cal:comp name="VEVENT"/></cal:supported-calendar-component-set></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
<d:response><d:href>/remote.php/dav/calendars/ann/tasks/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/><cal:calendar/></d:resourcetype><cal:supported-calendar-component-set><cal:comp name="VTODO"/></cal:supported-calendar-component-set></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
		case r.Method == "REPORT" && path == "/remote.php/dav/calendars/ann/personal/":
			ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:ev-1@nc\r\nSUMMARY:Dentist\r\nDTSTART:20240601T090000Z\r\nDTEND:20240601T100000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
			ms(w, `<d:response><d:href>/remote.php/dav/calendars/ann/personal/ev-1.ics</d:href><d:propstat><d:prop><d:getetag>"1"</d:getetag><cal:calendar-data>`+ics+`</cal:calendar-data></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
		case r.Method == "PROPFIND" && path == "/remote.php/dav/addressbooks/users/ann/":
			if r.Header.Get("Depth") == "0" {
				ms(w, `<d:response><d:href>/remote.php/dav/addressbooks/users/ann/</d:href><d:propstat><d:prop><card:addressbook-home-set><d:href>/remote.php/dav/addressbooks/users/ann/</d:href></card:addressbook-home-set></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
				return
			}
			ms(w, `<d:response><d:href>/remote.php/dav/addressbooks/users/ann/contacts/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/><card:addressbook/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
		case r.Method == "REPORT" && path == "/remote.php/dav/addressbooks/users/ann/contacts/":
			vcf := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:card-1\r\nFN:Eve Example\r\nN:Example;Eve;;;\r\nEMAIL:eve@example.org\r\nTEL:+44 20 0000\r\nORG:Example Ltd\r\nEND:VCARD\r\n"
			ms(w, `<d:response><d:href>/remote.php/dav/addressbooks/users/ann/contacts/card-1.vcf</d:href><d:propstat><d:prop><d:getetag>"c1"</d:getetag><card:address-data>`+vcf+`</card:address-data></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
		default:
			w.WriteHeader(405)
		}
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestNextcloudImportFromFakeServer(t *testing.T) {
	srv := newFakeNextcloud(t, "ann", "app-pass")
	im := startFakeIMAP(t, "ann", "mailpass", "", map[string][]string{
		"INBOX": {rawMail("x@y.example", "ann@example.org", "Hi", "nc1@x", "hello")},
		"Junk":  {rawMail("spam@y.example", "ann@example.org", "WIN", "nc2@x", "spam")},
	})
	conn := Connection{Provider: "nextcloud", Scope: "me", Config: map[string]string{"server": srv.URL, "username": "ann",
		"imap_host": strings.Split(im.Addr, ":")[0], "imap_port": strings.Split(im.Addr, ":")[1], "imap_security": "insecure", "imap_username": "ann"},
		secret: connSecret{Password: "app-pass", IMAPPassword: "mailpass"}}
	src, err := BuildSource(context.Background(), DefaultOAuthEndpoints, conn, ProviderSettings{}, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := testSource(context.Background(), src); err != nil {
		t.Fatalf("connection test: %v", err)
	}
	sink := newMemSink(nil)
	st := newMemState()
	c, err := (&Engine{Src: src, Sink: sink, St: st}).Run(context.Background(), RunConfig{RunID: "r", ConnID: "c", ActingUserID: "U", Scope: "me",
		Kinds: []Kind{MigFiles, MigMail, MigCalendar, MigContacts}})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(sink.filePaths(), ","); got != "U|/Documents/report.txt,U|/photo.jpg" {
		t.Errorf("files: %s (%+v, errs %v)", got, c["files"], st.errs)
	}
	if string(sink.files["U|/Documents/report.txt"]) != "quarterly" {
		t.Error("file content")
	}
	if len(sink.events) != 1 || sink.events[0].Title != "Dentist" {
		t.Errorf("events (VTODO list must be skipped): %+v", sink.events)
	}
	if len(sink.contacts) != 1 || sink.contacts[0].DisplayName != "Eve Example" || sink.contacts[0].Emails[0] != "eve@example.org" {
		t.Errorf("contacts %+v", sink.contacts)
	}
	folders := map[string]bool{}
	for _, m := range sink.mails {
		folders[m.Folder] = true
	}
	if !folders["inbox"] || !folders["spam"] {
		t.Errorf("mail folders %v", folders)
	}

	// Wrong password is caught by the connection test.
	bad := conn
	bad.secret.Password = "nope"
	src2, _ := BuildSource(context.Background(), DefaultOAuthEndpoints, bad, ProviderSettings{}, "", nil, nil)
	src2.(*StandardSource).IMAP = nil
	if err := testSource(context.Background(), src2); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("bad password not detected: %v", err)
	}
}

func TestIMAPRefusesCleartextToRemoteHost(t *testing.T) {
	_, err := IMAPConfig{Addr: "mail.example.com:143", Security: "insecure"}.dial()
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("got %v", err)
	}
}

func TestMapMailFolder(t *testing.T) {
	for in, want := range map[string][2]string{
		"INBOX": {"inbox", ""}, "[Gmail]/Sent Mail": {"sent", ""}, "Sent Items": {"sent", ""}, "Deleted Items": {"trash", ""},
		"Junk Email": {"spam", ""}, "INBOX.Drafts": {"drafts", ""}, "Clients/Acme": {"inbox", "Clients/Acme"},
	} {
		f, l := MapMailFolder(in)
		if f != want[0] || l != want[1] {
			t.Errorf("%s -> %s,%s want %v", in, f, l, want)
		}
	}
}
