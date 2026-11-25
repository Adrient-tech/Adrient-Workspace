package cloudimport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// Hand-written Graph responses following the v1.0 reference shapes.
func newFakeGraph(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	var srv *httptest.Server
	j := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/v1.0/users", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "" {
			j(w, map[string]any{"value": []any{map[string]any{"id": "m1", "mail": "ann@acme.example", "displayName": "Ann"}},
				"@odata.nextLink": srv.URL + "/v1.0/users?page=2"})
			return
		}
		j(w, map[string]any{"value": []any{map[string]any{"id": "m2", "userPrincipalName": "bob@acme.example", "displayName": "Bob"}}})
	})
	mux.HandleFunc("/v1.0/users/m1/drive/root/children", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"value": []any{
			map[string]any{"id": "f1", "name": "Contracts", "folder": map[string]any{"childCount": 1}},
			map[string]any{"id": "i1", "name": "notes.txt", "size": 5, "eTag": "e1", "file": map[string]any{"mimeType": "text/plain"}, "shared": map[string]any{}},
		}})
	})
	mux.HandleFunc("/v1.0/users/m1/drive/items/f1/children", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"value": []any{map[string]any{"id": "i2", "name": "nda.pdf", "size": 3, "eTag": "e2", "file": map[string]any{"mimeType": "application/pdf"}}}})
	})
	mux.HandleFunc("/v1.0/users/m1/drive/items/i1/content", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("hello")) })
	mux.HandleFunc("/v1.0/users/m1/drive/items/i2/content", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("PDF")) })
	mux.HandleFunc("/v1.0/users/m1/drive/items/i1/permissions", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"value": []any{map[string]any{"roles": []any{"write"}, "grantedToV2": map[string]any{"user": map[string]any{"email": "bob@acme.example"}}}}})
	})
	mux.HandleFunc("/v1.0/users/m2/drive/root/children", func(w http.ResponseWriter, r *http.Request) { http.Error(w, `{"error":{"code":"itemNotFound"}}`, 404) })
	mux.HandleFunc("/v1.0/users/m1/mailFolders", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"value": []any{map[string]any{"id": "inbox", "displayName": "Inbox", "childFolderCount": 1}, map[string]any{"id": "sent", "displayName": "Sent Items"}}})
	})
	mux.HandleFunc("/v1.0/users/m1/mailFolders/inbox/childFolders", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"value": []any{map[string]any{"id": "proj", "displayName": "Projects"}}})
	})
	msgs := map[string][]any{
		"inbox": {map[string]any{"id": "a", "isRead": true, "categories": []any{"Red category"}}},
		"sent":  {map[string]any{"id": "b", "isRead": true, "flag": map[string]any{"flagStatus": "flagged"}}},
		"proj":  {map[string]any{"id": "c"}},
	}
	for f, list := range msgs {
		list := list
		mux.HandleFunc("/v1.0/users/m1/mailFolders/"+f+"/messages", func(w http.ResponseWriter, r *http.Request) { j(w, map[string]any{"value": list}) })
	}
	for id, subj := range map[string]string{"a": "Hello", "b": "Sent one", "c": "Project mail"} {
		subj := subj
		id := id
		mux.HandleFunc("/v1.0/users/m1/messages/"+id+"/$value", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.ReplaceAll(rawMail("x@y.example", "ann@acme.example", subj, id+"@ms", "<p>body</p>"), "text/plain", "text/html")))
		})
	}
	mux.HandleFunc("/v1.0/users/m1/events", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"value": []any{map[string]any{"id": "ev1", "changeKey": "k", "subject": "Board meeting",
			"body":  map[string]any{"contentType": "html", "content": "<b>Agenda</b>"},
			"start": map[string]any{"dateTime": "2024-05-01T10:00:00.0000000", "timeZone": "UTC"},
			"end":   map[string]any{"dateTime": "2024-05-01T11:00:00.0000000", "timeZone": "UTC"},
			"recurrence": map[string]any{"pattern": map[string]any{"type": "weekly", "interval": 2, "daysOfWeek": []any{"wednesday"}},
				"range": map[string]any{"type": "numbered", "numberOfOccurrences": 5}}}}})
	})
	mux.HandleFunc("/v1.0/users/m1/contacts", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"value": []any{map[string]any{"id": "c1", "displayName": "Dan", "emailAddresses": []any{map[string]any{"address": "dan@x.example"}}, "mobilePhone": "+1 555"}}})
	})
	for _, p := range []string{"/v1.0/users/m2/mailFolders", "/v1.0/users/m2/events", "/v1.0/users/m2/contacts"} {
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) { j(w, map[string]any{"value": []any{}}) })
	}
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestMicrosoftOrgImportFromFixture(t *testing.T) {
	g := newFakeGraph(t)
	src := &MicrosoftSource{Graph: g.URL, Token: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "app"}), Self: "admin@acme.example"}
	sink := newMemSink(map[string]string{"ann@acme.example": "U-ann", "bob@acme.example": "U-bob"})
	st := newMemState()
	c, err := (&Engine{Src: src, Sink: sink, St: st}).Run(context.Background(), RunConfig{RunID: "r", ConnID: "c", ActingUserID: "U-admin", Scope: "org",
		Kinds: []Kind{MigUsers, MigFiles, MigMail, MigCalendar, MigContacts, MigChat}})
	if err != nil {
		t.Fatal(err)
	}
	if c["users"].Imported != 2 {
		t.Errorf("users %+v", c["users"])
	}
	if got := strings.Join(sink.filePaths(), ","); got != "U-ann|/Contracts/nda.pdf,U-ann|/notes.txt" {
		t.Errorf("files: %s", got)
	}
	if len(sink.shares) != 1 || sink.shares[0] != "/notes.txt->U-bob:editor" {
		t.Errorf("shares %v", sink.shares)
	}
	byFolder := map[string]MailFields{}
	for _, m := range sink.mails {
		byFolder[m.Subject] = m
	}
	if byFolder["Hello"].Folder != "inbox" || byFolder["Hello"].Labels[0] != "Red category" || byFolder["Sent one"].Folder != "sent" || !byFolder["Sent one"].Starred {
		t.Errorf("mail mapping: %+v", byFolder)
	}
	if pm := byFolder["Project mail"]; pm.Folder != "inbox" || len(pm.Labels) != 1 || pm.Labels[0] != "Inbox/Projects" || pm.Body != "body" {
		t.Errorf("nested folder -> label and html body: %+v", pm)
	}
	if len(sink.events) != 1 || sink.events[0].Recurrence != "FREQ=WEEKLY;INTERVAL=2;BYDAY=WE;COUNT=5" || sink.events[0].Description != "Agenda" {
		t.Errorf("events %+v", sink.events)
	}
	if len(sink.contacts) != 1 || sink.contacts[0].Phones[0] != "+1 555" {
		t.Errorf("contacts %+v", sink.contacts)
	}
	if c["chat"].Note == "" {
		t.Error("Teams chat must be reported as not supported")
	}
	if len(st.errs) != 0 {
		t.Errorf("unexpected errors: %v (Bob has no OneDrive: must be silent)", st.errs)
	}
}
