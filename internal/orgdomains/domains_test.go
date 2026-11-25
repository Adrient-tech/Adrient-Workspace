package orgdomains

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

type fakeResolver struct {
	mu   sync.Mutex
	recs map[string][]string
}

func (f *fakeResolver) set(name string, v ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recs == nil {
		f.recs = map[string][]string{}
	}
	f.recs[name] = v
}
func (f *fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recs[name], nil
}

func setup(t *testing.T) (*pgxpool.Pool, string, *fakeResolver, *Store) {
	t.Helper()
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := testguard.DropSchema(ctx, t, pool); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var orgID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	fr := &fakeResolver{}
	return pool, orgID, fr, &Store{Pool: pool, Resolver: fr}
}

func TestNormalizeAndFreeMail(t *testing.T) {
	if d, err := Normalize(" Example.COM. "); err != nil || d != "example.com" {
		t.Fatalf("got %q %v", d, err)
	}
	for _, bad := range []string{"", "nodot", "a_b.com", "-x.com", "x..com"} {
		if _, err := Normalize(bad); err == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
	if !IsFreeMail("Gmail.com") {
		t.Error("gmail is free mail")
	}
}

func TestDomainLifecycle(t *testing.T) {
	pool, orgID, fr, st := setup(t)
	ctx := context.Background()
	if _, err := st.Add(ctx, orgID, "gmail.com"); err != ErrFreeMail {
		t.Fatalf("free mail: %v", err)
	}
	d, err := st.Add(ctx, orgID, "Acme.Test")
	if err != nil || d.Status != StatusPending || !strings.HasPrefix(d.VerifyTXT, "adrient-verify=") || d.VerifyHost != "_adrient-challenge.acme.test" {
		t.Fatalf("add: %+v %v", d, err)
	}
	var other string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ('other','Other') RETURNING id::text`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Add(ctx, other, "acme.test"); err != ErrClaimed {
		t.Fatalf("takeover must be refused: %v", err)
	}
	tr := true
	if _, err := st.Update(ctx, orgID, d.ID, Patch{AutoJoin: &tr}); err != ErrNotVerified {
		t.Fatalf("auto_join on pending: %v", err)
	}
	fr.set(ChallengePrefix+"acme.test", "adrient-verify=wrong")
	if _, ok, _ := st.Verify(ctx, orgID, d.ID); ok {
		t.Fatal("wrong txt verified")
	}
	fr.set(ChallengePrefix+"acme.test", "v=spf1", d.VerifyTXT)
	got, ok, err := st.Verify(ctx, orgID, d.ID)
	if err != nil || !ok || got.Status != StatusVerified || !got.IsPrimary || got.VerifiedAt == nil {
		t.Fatalf("verify: %+v ok=%v err=%v", got, ok, err)
	}
	if st.Primary(ctx, orgID) != "acme.test" || !st.DomainClaimed(ctx, "ACME.test") {
		t.Fatal("primary/claimed")
	}
	if got, err = st.Update(ctx, orgID, d.ID, Patch{AutoJoin: &tr, AllowSignup: &tr}); err != nil || !got.AutoJoin || !got.AllowSignup {
		t.Fatalf("update: %+v %v", got, err)
	}
	if err := st.Remove(ctx, orgID, d.ID); err != ErrPrimary {
		t.Fatalf("remove primary: %v", err)
	}
	d2, _ := st.Add(ctx, orgID, "second.test")
	fr.set(ChallengePrefix+"second.test", d2.VerifyTXT)
	if _, ok, _ := st.Verify(ctx, orgID, d2.ID); !ok {
		t.Fatal("verify 2")
	}
	if _, err := st.Update(ctx, orgID, d2.ID, Patch{Primary: &tr}); err != nil {
		t.Fatal(err)
	}
	if st.Primary(ctx, orgID) != "second.test" {
		t.Fatal("primary should move")
	}
	if err := st.Remove(ctx, orgID, d.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	fr.set(ChallengePrefix + "second.test")
	late := &Store{Pool: pool, Resolver: fr, Now: func() time.Time { return time.Now().Add(15 * 24 * time.Hour) }}
	if got, _, _ = late.Verify(ctx, orgID, d2.ID); got.Status != StatusFailed {
		t.Fatalf("expected failed, got %s", got.Status)
	}
}

func TestSettings(t *testing.T) {
	_, orgID, _, st := setup(t)
	ctx := context.Background()
	s, _ := st.GetSettings(ctx, orgID)
	if s.SignupMode != "invite_only" {
		t.Fatalf("default mode %s", s.SignupMode)
	}
	if _, err := st.PutSettings(ctx, orgID, Settings{SignupMode: "bogus"}); err == nil {
		t.Fatal("bad mode accepted")
	}
	out, err := st.PutSettings(ctx, orgID, Settings{SignupMode: "domain_auto_join", AllowedSigninDomains: []string{"ACME.test"}})
	if err != nil || out.AllowedSigninDomains[0] != "acme.test" {
		t.Fatalf("%+v %v", out, err)
	}
}

type auditSink struct {
	mu   sync.Mutex
	rows []string
}

func (a *auditSink) fn(_ context.Context, service, action string, _ map[string]any) {
	a.mu.Lock()
	a.rows = append(a.rows, service+"."+action)
	a.mu.Unlock()
}
func (a *auditSink) has(x string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, r := range a.rows {
		if r == x {
			return true
		}
	}
	return false
}

func TestHandlerPermissionsAndAudit(t *testing.T) {
	pool, orgID, fr, st := setup(t)
	ur := users.NewRepository(pool)
	adm, _ := ur.UpsertByOIDC(context.Background(), users.UpsertInput{OrgID: orgID, OIDCIssuer: "i", OIDCSubject: "a", Email: "a@acme.test"})
	mem, _ := ur.UpsertByOIDC(context.Background(), users.UpsertInput{OrgID: orgID, OIDCIssuer: "i", OIDCSubject: "m", Email: "m@acme.test"})
	authz.SetDefault(authz.NewService(authz.AdminOnlyLoader{IsAdmin: func(_ context.Context, _, uid string) (bool, error) { return uid == adm.ID, nil }}))
	t.Cleanup(func() { authz.SetDefault(nil) })
	sink := &auditSink{}
	h := &Handler{Store: st, Audit: sink.fn}
	org := orgs.Org{ID: orgID, Slug: "default"}
	do := func(u users.User, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		ctx := auth.WithOrg(auth.WithUser(req.Context(), u), org)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req.WithContext(ctx))
		return w
	}
	if w := do(mem, "POST", PathDomains, `{"domain":"acme.test"}`); w.Code != http.StatusForbidden {
		t.Fatalf("member add: %d", w.Code)
	}
	if len(sink.rows) != 0 {
		t.Fatal("denied member must not write audit")
	}
	w := do(adm, "POST", PathDomains, `{"domain":"acme.test"}`)
	if w.Code != http.StatusCreated || !sink.has("domain.add") {
		t.Fatalf("admin add: %d %s %v", w.Code, w.Body.String(), sink.rows)
	}
	if w := do(adm, "POST", PathDomains, `{"domain":"gmail.com"}`); w.Code != http.StatusBadRequest || !sink.has("domain.add.denied") {
		t.Fatalf("freemail: %d %v", w.Code, sink.rows)
	}
	ds, _ := st.List(context.Background(), orgID)
	fr.set(ChallengePrefix+"acme.test", ds[0].VerifyTXT)
	if w := do(adm, "POST", PathDomains+"/"+ds[0].ID+"/verify", ""); w.Code != 200 || !sink.has("domain.verify") {
		t.Fatalf("verify: %d %v", w.Code, sink.rows)
	}
	if w := do(adm, "PATCH", PathDomains+"/"+ds[0].ID, `{"allow_signup":true}`); w.Code != 200 || !sink.has("domain.update") {
		t.Fatalf("patch: %d", w.Code)
	}
	if w := do(adm, "PUT", PathSettings, `{"signup_mode":"invite_only","allowed_signin_domains":["other.test"]}`); w.Code != 400 {
		t.Fatalf("lockout guard: %d", w.Code)
	}
	if w := do(adm, "PUT", PathSettings, `{"signup_mode":"domain_auto_join","allowed_signin_domains":["acme.test"]}`); w.Code != 200 || !sink.has("domain.settings.change") {
		t.Fatalf("settings: %d %s", w.Code, w.Body.String())
	}
	if w := do(adm, "DELETE", PathDomains+"/"+ds[0].ID, ""); w.Code != http.StatusConflict {
		t.Fatalf("delete primary: %d", w.Code)
	}
	req := httptest.NewRequest("GET", PathHint+"?email=x@acme.test", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"claimed":true`) {
		t.Fatalf("hint: %d %s", rec.Code, rec.Body.String())
	}
}
