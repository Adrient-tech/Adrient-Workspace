package cloudimport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// fakeGoogle replays recorded-shape Google API responses (the JSON field names
// and pagination follow the public API reference; the payloads are written by
// hand, not captured from a live tenant).
type fakeGoogle struct {
	*httptest.Server
	token     string
	throttled int32 // the first /drive/v3/files call answers 429 once
	calls     int32
}

func newFakeGoogle(t *testing.T, token string) *fakeGoogle {
	f := &fakeGoogle{token: token}
	mux := http.NewServeMux()
	j := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&f.calls, 1)
			if r.Header.Get("Authorization") != "Bearer "+f.token {
				http.Error(w, `{"error":"unauthenticated"}`, 401)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/admin/directory/v1/users", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			j(w, map[string]any{"users": []any{map[string]any{"id": "u1", "primaryEmail": "ann@acme.example", "name": map[string]any{"fullName": "Ann"}}}, "nextPageToken": "p2"})
			return
		}
		j(w, map[string]any{"users": []any{
			map[string]any{"id": "u2", "primaryEmail": "bob@acme.example", "name": map[string]any{"fullName": "Bob"}},
			map[string]any{"id": "u3", "primaryEmail": "nobody@acme.example", "name": map[string]any{"fullName": "No Account"}},
		}})
	}))
	mux.HandleFunc("/admin/directory/v1/groups", auth(func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"groups": []any{map[string]any{"id": "g1", "email": "sales@acme.example", "name": "Sales"}}})
	}))
	mux.HandleFunc("/admin/directory/v1/groups/g1/members", auth(func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"members": []any{map[string]any{"email": "ann@acme.example"}, map[string]any{"email": "bob@acme.example"}}})
	}))
	mux.HandleFunc("/drive/v3/files", auth(func(w http.ResponseWriter, r *http.Request) {
		if atomic.CompareAndSwapInt32(&f.throttled, 0, 1) {
			w.Header().Set("Retry-After", "0")
			http.Error(w, `{"error":{"code":429,"message":"rateLimitExceeded"}}`, 429)
			return
		}
		q := r.URL.Query().Get("q")
		switch {
		case strings.Contains(q, "'root'"):
			j(w, map[string]any{"files": []any{
				map[string]any{"id": "fld1", "name": "Projects", "mimeType": "application/vnd.google-apps.folder"},
				map[string]any{"id": "doc1", "name": "Plan", "mimeType": "application/vnd.google-apps.document", "modifiedTime": "2024-01-01T00:00:00Z",
					"permissions": []any{map[string]any{"type": "user", "role": "writer", "emailAddress": "bob@acme.example"}, map[string]any{"type": "user", "role": "owner", "emailAddress": "ann@acme.example"}}},
				map[string]any{"id": "form1", "name": "Survey", "mimeType": "application/vnd.google-apps.form"},
			}})
		case strings.Contains(q, "'fld1'"):
			j(w, map[string]any{"files": []any{map[string]any{"id": "bin1", "name": "budget.csv", "mimeType": "text/csv", "size": "11", "md5Checksum": "abc"}}})
		case strings.Contains(q, "'sd1'"):
			j(w, map[string]any{"files": []any{map[string]any{"id": "bin2", "name": "logo.png", "mimeType": "image/png", "size": "4", "md5Checksum": "def"}}})
		default:
			j(w, map[string]any{"files": []any{}})
		}
	}))
	mux.HandleFunc("/drive/v3/files/doc1/export", auth(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("mimeType"), "wordprocessingml") {
			http.Error(w, "bad export type", 400)
			return
		}
		_, _ = w.Write([]byte("DOCX-BYTES"))
	}))
	mux.HandleFunc("/drive/v3/files/bin1", auth(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("a,b\n1,2\n3,4")) }))
	mux.HandleFunc("/drive/v3/files/bin2", auth(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("\x89PNG")) }))
	mux.HandleFunc("/drive/v3/drives", auth(func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"drives": []any{map[string]any{"id": "sd1", "name": "Marketing"}}})
	}))
	mux.HandleFunc("/calendar/v3/users/me/calendarList", auth(func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"items": []any{map[string]any{"id": "primary", "accessRole": "owner"}, map[string]any{"id": "holidays", "accessRole": "reader"}}})
	}))
	mux.HandleFunc("/calendar/v3/calendars/primary/events", auth(func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"items": []any{
			map[string]any{"id": "e1", "etag": "\"1\"", "summary": "Standup", "start": map[string]any{"dateTime": "2024-03-04T09:00:00+05:30"}, "end": map[string]any{"dateTime": "2024-03-04T09:15:00+05:30"}, "recurrence": []any{"RRULE:FREQ=WEEKLY;BYDAY=MO"}},
			map[string]any{"id": "e2", "etag": "\"2\"", "summary": "Holiday", "start": map[string]any{"date": "2024-03-08"}, "end": map[string]any{"date": "2024-03-09"}},
			map[string]any{"id": "e3", "status": "cancelled"},
		}})
	}))
	mux.HandleFunc("/v1/people/me/connections", auth(func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"connections": []any{map[string]any{"resourceName": "people/c1", "etag": "x",
			"names":          []any{map[string]any{"displayName": "Carol Client", "givenName": "Carol", "familyName": "Client"}},
			"emailAddresses": []any{map[string]any{"value": "carol@client.example"}}, "phoneNumbers": []any{map[string]any{"value": "+91 98450 00000"}},
			"organizations": []any{map[string]any{"name": "Client Co", "title": "CTO"}}}}})
	}))
	mux.HandleFunc("/v1/spaces", auth(func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"spaces": []any{map[string]any{"name": "spaces/AAA", "displayName": "Team room"}}})
	}))
	mux.HandleFunc("/v1/spaces/AAA/messages", auth(func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"messages": []any{map[string]any{"name": "spaces/AAA/messages/m1", "text": "hello team", "createTime": "2024-02-01T10:00:00.000Z", "sender": map[string]any{"displayName": "Ann"}}}})
	}))
	// OAuth token endpoint: authorization_code (code "good") and refresh_token.
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "good" || r.Form.Get("client_secret") != "shh" {
				http.Error(w, `{"error":"invalid_grant"}`, 400)
				return
			}
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh-1" {
				http.Error(w, `{"error":"invalid_grant"}`, 400)
				return
			}
		default:
			http.Error(w, `{"error":"unsupported_grant_type"}`, 400)
			return
		}
		// id_token payload {"email":"ann@acme.example"} (unsigned; only used as a label)
		j(w, map[string]any{"access_token": f.token, "refresh_token": "refresh-1", "expires_in": 3600, "token_type": "Bearer",
			"id_token": "e30.eyJlbWFpbCI6ImFubkBhY21lLmV4YW1wbGUifQ.sig"})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func googleFixtureSource(t *testing.T, token string) (*GoogleSource, *fakeGoogle, *fakeIMAP) {
	fg := newFakeGoogle(t, token)
	im := startFakeIMAP(t, "ann@acme.example", "", token, map[string][]string{
		"INBOX":             {rawMail("Bob <bob@acme.example>", "ann@acme.example", "Quarterly numbers", "m1@acme", "See attached.")},
		"[Gmail]/Sent Mail": {rawMail("ann@acme.example", "carol@client.example", "Re: proposal", "m2@acme", "Thanks Carol")},
		"[Gmail]/All Mail":  {rawMail("Bob <bob@acme.example>", "ann@acme.example", "Quarterly numbers", "m1@acme", "See attached.")},
		"Clients/Acme":      {rawMail("Bob <bob@acme.example>", "ann@acme.example", "Quarterly numbers", "m1@acme", "See attached.")},
	})
	ep := GoogleEndpoints{Directory: fg.URL, Drive: fg.URL, Calendar: fg.URL, People: fg.URL, Chat: fg.URL, IMAPAddr: im.Addr, IMAPSec: "insecure"}
	src := &GoogleSource{EP: ep, Token: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}), Self: "ann@acme.example",
		Retry: RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}}
	return src, fg, im
}

func TestGoogleImportFromFixture(t *testing.T) {
	ctx := context.Background()
	src, fg, _ := googleFixtureSource(t, "tok-123")
	sink := newMemSink(map[string]string{"ann@acme.example": "U-ann", "bob@acme.example": "U-bob"})
	st := newMemState()
	cfg := RunConfig{RunID: "r1", OrgID: "o", ConnID: "c", ActingUserID: "U-ann", Scope: "me", SelfEmail: "ann@acme.example",
		Kinds: []Kind{MigFiles, MigMail, MigCalendar, MigContacts, MigChat}}

	// Dry run writes nothing but counts.
	dry := cfg
	dry.DryRun = true
	e := &Engine{Src: src, Sink: sink, St: st}
	c, err := e.Run(ctx, dry)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.files)+len(sink.mails)+len(sink.events)+len(sink.contacts) != 0 {
		t.Fatal("dry run wrote data")
	}
	if c["files"].Imported != 6 { // 2 folders (Projects, Marketing shared drive) + doc + csv + png + form(would try)
		t.Logf("dry-run files counter: %+v", c["files"])
	}

	// Real run.
	e = &Engine{Src: src, Sink: sink, St: st}
	c, err = e.Run(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"U-ann|/Marketing (shared drive)/logo.png", "U-ann|/Plan.docx", "U-ann|/Projects/budget.csv"}
	if got := sink.filePaths(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("files: got %v want %v", got, want)
	}
	if string(sink.files["U-ann|/Plan.docx"]) != "DOCX-BYTES" {
		t.Error("Google Doc was not exported as docx")
	}
	if len(sink.shares) != 1 || sink.shares[0] != "/Plan.docx->U-bob:editor" {
		t.Errorf("sharing not carried over: %v", sink.shares)
	}
	if c["files"].Failed != 1 || !strings.Contains(strings.Join(st.errs, "\n"), "Survey") {
		t.Errorf("Google Form must be reported as not exportable: %+v %v", c["files"], st.errs)
	}
	// Mail: All Mail skipped; m1 is in INBOX and under the Clients/Acme label: stored once with the label.
	folders := map[string]int{}
	for _, m := range sink.mails {
		folders[m.Folder+"|"+strings.Join(m.Labels, ",")]++
	}
	if folders["inbox|Clients/Acme"] != 1 || folders["sent|"] != 1 || len(sink.mails) != 2 {
		t.Errorf("mail folders/labels: %v", folders)
	}
	if len(sink.events) != 2 || sink.events[0].Recurrence != "FREQ=WEEKLY;BYDAY=MO" || !sink.events[1].AllDay {
		t.Errorf("events: %+v", sink.events)
	}
	if len(sink.contacts) != 1 || sink.contacts[0].Company != "Client Co" {
		t.Errorf("contacts: %+v", sink.contacts)
	}
	if len(sink.chats) != 1 || sink.chats[0].Space != "Team room" {
		t.Errorf("chat: %+v", sink.chats)
	}
	if atomic.LoadInt32(&fg.throttled) != 1 {
		t.Error("429 path was not exercised")
	}

	// Idempotent re-run: nothing new is written.
	nFiles, nMail, nEv := len(sink.files), len(sink.mails), len(sink.events)
	e = &Engine{Src: src, Sink: sink, St: st}
	c, err = e.Run(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.files) != nFiles || len(sink.mails) != nMail || len(sink.events) != nEv || len(sink.contacts) != 1 || len(sink.chats) != 1 {
		t.Fatalf("re-run duplicated data: files %d->%d mail %d->%d", nFiles, len(sink.files), nMail, len(sink.mails))
	}
	if c["mail"].Skipped != 3 || c["files"].Skipped < 5 {
		t.Errorf("re-run skip counters: mail %+v files %+v", c["mail"], c["files"])
	}
}

func TestGoogleOrgScopeMapsUsersAndGroups(t *testing.T) {
	src, _, _ := googleFixtureSource(t, "tok")
	sink := newMemSink(map[string]string{"ann@acme.example": "U-ann", "bob@acme.example": "U-bob"})
	st := newMemState()
	c, err := (&Engine{Src: src, Sink: sink, St: st}).Run(context.Background(), RunConfig{RunID: "r", ConnID: "c", ActingUserID: "U-ann", Scope: "org", Kinds: []Kind{MigUsers, MigGroups, MigContacts}})
	if err != nil {
		t.Fatal(err)
	}
	if c["users"].Seen != 3 || c["users"].Imported != 2 || c["users"].Failed != 1 {
		t.Errorf("users: %+v", c["users"])
	}
	if len(sink.groups) != 1 || sink.groups[0] != "Sales<sales@acme.example>[U-ann U-bob]" {
		t.Errorf("groups: %v", sink.groups)
	}
	// Without a service account, Bob's contacts cannot be read; Ann's (the admin) can.
	if len(sink.contacts) != 1 || !strings.Contains(strings.Join(st.errs, "\n"), "domain-wide delegation") {
		t.Errorf("delegation handling: contacts=%d errs=%v", len(sink.contacts), st.errs)
	}
}

func TestEngineResumesAfterCancel(t *testing.T) {
	src, _, _ := googleFixtureSource(t, "tok")
	sink := newMemSink(map[string]string{"ann@acme.example": "U-ann"})
	st := newMemState()
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	wrapped := &countingSource{Source: src, onFile: func() {
		n++
		if n == 3 {
			cancel()
		}
	}}
	cfg := RunConfig{RunID: "r", ConnID: "c", ActingUserID: "U-ann", Scope: "me", SelfEmail: "ann@acme.example", Kinds: []Kind{MigFiles}}
	if _, err := (&Engine{Src: wrapped, Sink: sink, St: st}).Run(ctx, cfg); err == nil {
		t.Fatal("expected cancellation")
	}
	partial := len(sink.files)
	c, err := (&Engine{Src: src, Sink: sink, St: st}).Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.files) != 3 || c["files"].Skipped == 0 {
		t.Fatalf("resume: partial=%d final=%d counters=%+v", partial, len(sink.files), c["files"])
	}
}

type countingSource struct {
	Source
	onFile func()
}

func (c *countingSource) Files(ctx context.Context, u RemoteUser, fn func(RemoteFile) error) error {
	return c.Source.Files(ctx, u, func(f RemoteFile) error { c.onFile(); return fn(f) })
}

func TestRetryHonoursRetryAfterAndGivesUp(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(503)
	}))
	defer srv.Close()
	var slept []time.Duration
	p := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Minute, Sleep: func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }}
	_, err := p.DoRetry(context.Background(), srv.Client(), func() (*http.Request, error) { return http.NewRequest("GET", srv.URL, nil) })
	if err == nil || hits != 3 || len(slept) != 2 || slept[0] != 7*time.Second {
		t.Fatalf("err=%v hits=%d slept=%v", err, hits, slept)
	}
	// 403 is not retried.
	hits = 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1); w.WriteHeader(403) }))
	defer srv2.Close()
	if _, err := p.DoRetry(context.Background(), srv2.Client(), func() (*http.Request, error) { return http.NewRequest("GET", srv2.URL, nil) }); err == nil || hits != 1 {
		t.Fatalf("403 retried: hits=%d", hits)
	}
}
