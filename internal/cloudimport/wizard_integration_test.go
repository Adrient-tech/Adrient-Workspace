package cloudimport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/chat"
	"github.com/Adrient-tech/Adrient-Workspace/internal/cloudimport"
	"github.com/Adrient-tech/Adrient-Workspace/internal/cloudimport/reposink"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contacts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
	"github.com/Adrient-tech/Adrient-Workspace/internal/groups"
	"github.com/Adrient-tech/Adrient-Workspace/internal/mail"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sharing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage/storagetest"
)

type memBlobs struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (b *memBlobs) Put(_ context.Context, k, _ string, _ int64, r io.Reader) error {
	d, err := io.ReadAll(r)
	b.mu.Lock()
	b.m[k] = d
	b.mu.Unlock()
	return err
}
func (b *memBlobs) Delete(_ context.Context, k string) error {
	b.mu.Lock()
	delete(b.m, k)
	b.mu.Unlock()
	return nil
}

type ctxKey string

// TestWizardGoogleEndToEnd drives the real HTTP API, Postgres store and
// repository sink against the local Google fixture: admin enters the OAuth
// app, a user connects (OAuth code flow against the fake token endpoint), runs
// a dry run, a real run, and an idempotent re-run.
func TestWizardGoogleEndToEnd(t *testing.T) {
	pool := storagetest.ScratchDB(t)
	ctx := context.Background()
	var orgID, ann, bob string
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ('acme','Acme') RETURNING id::text`).Scan(&orgID))
	must(pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'i','a','ann@acme.example','Ann') RETURNING id::text`, orgID).Scan(&ann))
	must(pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'i','b','bob@acme.example','Bob') RETURNING id::text`, orgID).Scan(&bob))

	blobs := &memBlobs{m: map[string][]byte{}}
	w := &cloudimport.Wizard{
		Store: cloudimport.NewWizardStore(pool, "test-secret-key"),
		Sink: &reposink.Sink{Pool: pool, Drive: drive.NewRepository(pool), Blobs: blobs, Sharing: sharing.NewRepository(pool),
			Contacts: contacts.NewRepository(pool), Calendar: calendar.NewRepository(pool), Mail: mail.NewRepository(pool),
			Groups: groups.NewRepository(pool), Chat: chat.NewRepository(pool)},
		CallerOf: func(ctx context.Context) (string, string, string, bool) {
			u, _ := ctx.Value(ctxKey("user")).(string)
			e := map[string]string{ann: "ann@acme.example", bob: "bob@acme.example"}[u]
			return orgID, u, e, u != ""
		},
		IsAdmin:     func(r *http.Request) bool { return r.Header.Get("X-Test-Admin") == "1" },
		RedirectURL: "https://workspace.test/api/v1/import/wizard/oauth/callback",
		Endpoints:   cloudimport.FakeGoogleEndpoints(t, "live-token"),
	}
	h := cloudimport.NewHandler(cloudimport.NewRepository(pool), nil, func(context.Context) (string, string, bool) { return orgID, ann, true })
	h.Wizard = w

	do := func(user, method, path string, body any, admin bool) (int, map[string]any, http.Header) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, rd).WithContext(context.WithValue(ctx, ctxKey("user"), user))
		if admin {
			req.Header.Set("X-Test-Admin", "1")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out, rec.Header()
	}
	base := "/api/v1/import/wizard/"

	// Not configured until an admin enters the OAuth app.
	_, pv, _ := do(ann, "GET", base+"providers", nil, false)
	if pv["providers"].([]any)[0].(map[string]any)["configured"] != false {
		t.Fatal("google must report not configured")
	}
	if code, _, _ := do(ann, "POST", base+"connections", map[string]any{"provider": "google"}, false); code != 409 {
		t.Fatalf("connect before configuration: %d", code)
	}
	if code, _, _ := do(bob, "PUT", base+"settings/google", map[string]any{"client_id": "cid", "client_secret": "shh"}, false); code != 403 {
		t.Fatalf("non-admin settings: %d", code)
	}
	if code, _, _ := do(ann, "PUT", base+"settings/google", map[string]any{"client_id": "cid", "client_secret": "shh"}, true); code != 200 {
		t.Fatalf("admin settings: %d", code)
	}
	_, st, _ := do(ann, "GET", base+"settings/google", nil, true)
	if st["has_secret"] != true || strings.Contains(fmt.Sprint(st), "shh") {
		t.Fatalf("settings readback leaked or lost secret: %v", st)
	}
	var stored []byte
	must(pool.QueryRow(ctx, `SELECT client_secret_enc FROM adrient.import_provider_settings WHERE org_id=$1`, orgID).Scan(&stored))
	if bytes.Contains(stored, []byte("shh")) {
		t.Fatal("client secret stored in clear")
	}

	// Connect with OAuth.
	code, conn, _ := do(ann, "POST", base+"connections", map[string]any{"provider": "google", "scope": "me"}, false)
	if code != 201 || conn["status"] != "needs_auth" {
		t.Fatalf("create connection: %d %v", code, conn)
	}
	cid := conn["id"].(string)
	_, start, _ := do(ann, "POST", base+"connections/"+cid+"/oauth/start", nil, false)
	au, _ := url.Parse(start["url"].(string))
	if au.Query().Get("client_id") != "cid" || au.Query().Get("access_type") != "offline" || au.Query().Get("redirect_uri") != w.RedirectURL {
		t.Fatalf("auth url: %s", au)
	}
	state := au.Query().Get("state")
	// Bob cannot complete Ann's sign-in.
	if code, _, hdr := do(bob, "GET", base+"oauth/callback?code=good&state="+url.QueryEscape(state), nil, false); code != 302 || !strings.Contains(hdr.Get("Location"), "oauth=error") {
		t.Fatalf("cross-user callback accepted: %d %s", code, hdr.Get("Location"))
	}
	if code, _, hdr := do(ann, "GET", base+"oauth/callback?code=good&state="+url.QueryEscape(state), nil, false); code != 302 || hdr.Get("Location") != "/cloudimport?oauth=ok" {
		t.Fatalf("callback: %d %s", code, hdr.Get("Location"))
	}
	_, conn, _ = do(ann, "GET", base+"connections/"+cid, nil, false)
	if conn["status"] != "ready" || conn["config"].(map[string]any)["account_email"] != "ann@acme.example" {
		t.Fatalf("after oauth: %v", conn)
	}
	if code, _, _ := do(bob, "GET", base+"connections/"+cid, nil, false); code != 404 {
		t.Fatalf("other user can see connection: %d", code)
	}

	runAndWait := func(dry bool) map[string]any {
		t.Helper()
		code, run, _ := do(ann, "POST", base+"connections/"+cid+"/runs", map[string]any{"kinds": []string{"files", "mail", "calendar", "contacts", "chat"}, "dry_run": dry}, false)
		if code != 202 {
			t.Fatalf("start run: %d %v", code, run)
		}
		id := run["id"].(string)
		for i := 0; i < 200; i++ {
			_, run, _ = do(ann, "GET", base+"runs/"+id, nil, false)
			if s := run["status"]; s == "done" || s == "failed" || s == "cancelled" {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if run["status"] != "done" {
			t.Fatalf("run did not finish: %v", run)
		}
		return run
	}
	count := func(q string) int {
		var n int
		must(pool.QueryRow(ctx, q, orgID).Scan(&n))
		return n
	}
	counts := func() string {
		return fmt.Sprintf("drive=%d mail=%d events=%d contacts=%d chat=%d grants=%d",
			count(`SELECT count(*) FROM adrient.drive_files WHERE org_id=$1`),
			count(`SELECT count(*) FROM adrient.mail_messages WHERE org_id=$1`),
			count(`SELECT count(*) FROM adrient.calendar_events WHERE org_id=$1`),
			count(`SELECT count(*) FROM adrient.contacts WHERE org_id=$1`),
			count(`SELECT count(*) FROM adrient.chat_messages WHERE org_id=$1`),
			count(`SELECT count(*) FROM adrient.object_grants g JOIN adrient.drive_files f ON f.id=g.object_id WHERE f.org_id=$1`))
	}

	dry := runAndWait(true)
	if c := counts(); c != "drive=0 mail=0 events=0 contacts=0 chat=0 grants=0" {
		t.Fatalf("dry run wrote data: %s", c)
	}
	if dry["counters"].(map[string]any)["files"].(map[string]any)["imported"].(float64) < 5 {
		t.Errorf("dry run counters: %v", dry["counters"])
	}

	real := runAndWait(false)
	// drive: import root + Projects + Plan.docx + budget.csv + Marketing + logo.png = 6
	want := "drive=6 mail=2 events=2 contacts=1 chat=1 grants=1"
	if c := counts(); c != want {
		t.Fatalf("after import: %s want %s (run %v)", c, want, real)
	}
	var path string
	must(pool.QueryRow(ctx, `SELECT p.name || '/' || f.name FROM adrient.drive_files f JOIN adrient.drive_files p ON p.id=f.parent_id WHERE f.org_id=$1 AND f.name='Plan.docx'`, orgID).Scan(&path))
	if path != "Imported from Google/Plan.docx" {
		t.Errorf("file placed at %q", path)
	}
	var labels string
	must(pool.QueryRow(ctx, `SELECT labels::text FROM adrient.mail_messages WHERE org_id=$1 AND subject='Quarterly numbers'`, orgID).Scan(&labels))
	if !strings.Contains(labels, "Clients/Acme") {
		t.Errorf("gmail label lost: %s", labels)
	}
	if real["error_count"].(float64) != 1 {
		t.Errorf("expected exactly the Google Form error, got %v", real["error_count"])
	}

	// Error report as CSV.
	req := httptest.NewRequest("GET", base+"runs/"+real["id"].(string)+"/errors?format=csv", nil).WithContext(context.WithValue(ctx, ctxKey("user"), ann))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Survey") || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") {
		t.Errorf("csv report: %s", rec.Body.String())
	}

	// Idempotent re-run.
	again := runAndWait(false)
	if c := counts(); c != want {
		t.Fatalf("re-run duplicated data: %s", c)
	}
	if again["counters"].(map[string]any)["mail"].(map[string]any)["skipped"].(float64) != 3 {
		t.Errorf("re-run mail counters: %v", again["counters"])
	}

	// Deleting the connection keeps the imported data.
	if code, _, _ := do(ann, "DELETE", base+"connections/"+cid, nil, false); code != 200 {
		t.Fatal("delete connection")
	}
	if c := counts(); c != want {
		t.Errorf("delete connection removed imported data: %s", c)
	}
}
