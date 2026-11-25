package lifecycle

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
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgdomains"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

type sink struct {
	mu   sync.Mutex
	rows []string
}

func (s *sink) fn(_ context.Context, service, action string, _ map[string]any) {
	s.mu.Lock()
	s.rows = append(s.rows, service+"."+action)
	s.mu.Unlock()
}
func (s *sink) has(x string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rows {
		if r == x {
			return true
		}
	}
	return false
}

type env struct {
	pool  *pgxpool.Pool
	org   string
	st    *Store
	dom   *orgdomains.Store
	pol   *Policy
	audit *sink
	ur    *users.Repository
}

func setup(t *testing.T) *env {
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
	e := &env{pool: pool, audit: &sink{}, ur: users.NewRepository(pool)}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&e.org); err != nil {
		t.Fatal(err)
	}
	e.st = &Store{Pool: pool}
	e.dom = &orgdomains.Store{Pool: pool}
	e.pol = &Policy{Store: e.st, Domains: e.dom, Audit: e.audit.fn}
	return e
}

func (e *env) user(t *testing.T, sub, email string) users.User {
	t.Helper()
	u, err := e.ur.UpsertByOIDC(context.Background(), users.UpsertInput{OrgID: e.org, OIDCIssuer: "i", OIDCSubject: sub, Email: email})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestInvitationFlow(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	admin := e.user(t, "a", "a@acme.test")
	inv, secret, err := e.st.CreateInvitation(ctx, e.org, admin.ID, InviteInput{Email: "Pat@Partner.example", UserType: "guest"})
	if err != nil || inv.State != "pending" || inv.Email != "pat@partner.example" || inv.SponsorID != admin.ID || inv.UserExpiresAt == nil {
		t.Fatalf("create: %+v %v", inv, err)
	}
	if len(inv.AppIDs) != 1 || inv.AppIDs[0] != "drive" {
		t.Fatalf("default guest apps: %v", inv.AppIDs)
	}
	if _, _, err := e.st.CreateInvitation(ctx, e.org, admin.ID, InviteInput{Email: "a@acme.test"}); err != ErrExists {
		t.Fatalf("existing member: %v", err)
	}
	if _, _, err := e.st.CreateInvitation(ctx, e.org, admin.ID, InviteInput{Email: "nonsense"}); err == nil {
		t.Fatal("bad email accepted")
	}
	if _, org, err := e.st.LookupPublic(ctx, secret); err != nil || org == "" {
		t.Fatalf("lookup: %v", err)
	}
	// Sign-in routes the invited e-mail (verified claim) into the org as a guest.
	if r, _ := e.pol.Route(ctx, "pat@partner.example", false); r != nil {
		t.Fatal("unverified e-mail must not route")
	}
	route, err := e.pol.Route(ctx, "pat@partner.example", true)
	if err != nil || route == nil || route.OrgID != e.org || route.UserType != "guest" {
		t.Fatalf("route: %+v %v", route, err)
	}
	guest := e.user(t, "g", "pat@partner.example")
	if err := e.pol.Finish(ctx, guest, route, "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	g, _ := e.ur.GetByID(ctx, guest.ID)
	if g.UserType != "guest" || g.ExpiresAt == nil || len(g.GuestApps) != 1 || g.UPN == "" {
		t.Fatalf("guest after finish: %+v", g)
	}
	if !e.audit.has("lifecycle.user.accept") || !e.audit.has("identity.signin.allowed") {
		t.Fatalf("audit: %v", e.audit.rows)
	}
	// One-time: the link no longer works for anyone else, but is idempotent for the redeemer.
	if _, err := e.st.Accept(ctx, secret, admin.ID, admin.Email, e.org); err == nil {
		t.Fatal("reuse by another user must fail")
	}
	if _, err := e.st.Accept(ctx, secret, guest.ID, guest.Email, e.org); err != nil {
		t.Fatalf("idempotent accept: %v", err)
	}
	if r, _ := e.pol.Route(ctx, "pat@partner.example", true); r != nil {
		t.Fatal("accepted invitation must not route again")
	}

	// Accept flow for an already-provisioned user; wrong email refused; revoked/expired fail.
	inv2, s2, _ := e.st.CreateInvitation(ctx, e.org, admin.ID, InviteInput{Email: "kim@partner.example", UserType: "external"})
	kim := e.user(t, "k", "kim@partner.example")
	if _, err := e.st.Accept(ctx, s2, admin.ID, admin.Email, e.org); err != ErrEmailMismatch {
		t.Fatalf("mismatch: %v", err)
	}
	if _, err := e.st.Accept(ctx, s2, kim.ID, kim.Email, e.org); err != nil {
		t.Fatalf("accept: %v", err)
	}
	k, _ := e.ur.GetByID(ctx, kim.ID)
	if k.UserType != "external" || k.ExpiresAt == nil || time.Until(*k.ExpiresAt) < 80*24*time.Hour {
		t.Fatalf("external default 90d: %+v", k)
	}
	_ = inv2
	_, s3, _ := e.st.CreateInvitation(ctx, e.org, admin.ID, InviteInput{Email: "old@partner.example"})
	late := &Store{Pool: e.pool, Now: func() time.Time { return time.Now().Add(8 * 24 * time.Hour) }}
	if _, _, err := late.LookupPublic(ctx, s3); err != ErrInviteGone {
		t.Fatalf("expired invite: %v", err)
	}
	i4, s4, _ := e.st.CreateInvitation(ctx, e.org, admin.ID, InviteInput{Email: "rev@partner.example"})
	if err := e.st.RevokeInvitation(ctx, e.org, i4.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.st.LookupPublic(ctx, s4); err != ErrInviteGone {
		t.Fatalf("revoked invite: %v", err)
	}
}

func TestDomainAutoJoinAndGate(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	fr := map[string][]string{}
	e.dom.Resolver = resolverFunc(func(n string) []string { return fr[n] })
	d, _ := e.dom.Add(ctx, e.org, "acme.test")
	fr[orgdomains.ChallengePrefix+"acme.test"] = []string{d.VerifyTXT}
	if _, ok, _ := e.dom.Verify(ctx, e.org, d.ID); !ok {
		t.Fatal("verify")
	}
	tr := true
	_, _ = e.dom.Update(ctx, e.org, d.ID, orgdomains.Patch{AutoJoin: &tr})
	// Mode still invite_only: no auto-join.
	if r, _ := e.pol.Route(ctx, "new@acme.test", true); r != nil {
		t.Fatal("invite_only must not auto-join")
	}
	if e.pol.SignupOpen(ctx, e.org) {
		t.Fatal("default must be closed")
	}
	_, _ = e.dom.PutSettings(ctx, e.org, orgdomains.Settings{SignupMode: "domain_auto_join", AllowedSigninDomains: []string{"acme.test"}})
	r, err := e.pol.Route(ctx, "New@Acme.test", true)
	if err != nil || r == nil || r.OrgID != e.org || r.UserType != "member" || r.HomeDomainID != d.ID {
		t.Fatalf("auto-join: %+v %v", r, err)
	}
	if r, _ := e.pol.Route(ctx, "new@other.test", true); r != nil {
		t.Fatal("foreign domain must not join")
	}
	u := e.user(t, "n", "new@acme.test")
	_ = e.pol.Finish(ctx, u, r, "")
	got, _ := e.ur.GetByID(ctx, u.ID)
	if got.UPN != "new@acme.test" {
		t.Fatalf("upn %q", got.UPN)
	}
	// Allowed-domain gate: a member on a foreign domain is refused; guests are exempt.
	if err := e.pol.Gate(ctx, e.org, "x@other.test", "", nil, nil); err == nil {
		t.Fatal("domain_not_allowed expected")
	}
	if err := e.pol.Gate(ctx, e.org, "x@other.test", "", nil, &auth.SignInRoute{UserType: "guest"}); err != nil {
		t.Fatalf("guest exempt: %v", err)
	}
	if !e.audit.has("identity.signin.denied") {
		t.Fatal("denied sign-in must be audited")
	}
	// Blocked user is refused on sign-in.
	if _, err := e.st.Block(ctx, e.org, u.ID, "test"); err != nil {
		t.Fatal(err)
	}
	bu, _ := e.ur.GetByID(ctx, u.ID)
	if err := e.pol.Gate(ctx, e.org, bu.Email, "", &bu, nil); err == nil {
		t.Fatal("blocked must be denied")
	}
}

type resolverFunc func(string) []string

func (f resolverFunc) LookupTXT(_ context.Context, n string) ([]string, error) { return f(n), nil }

func TestBlockMiddlewareAndExpiry(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	u := e.user(t, "b", "b@acme.test")
	sessions := auth.NewSessionStore(e.pool)
	tok, err := sessions.Create(ctx, u.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	orepo := orgs.NewRepository(e.pool)
	def, _ := orepo.GetBySlug(ctx, "default")
	cfg := auth.Config{CookieName: "sid"}
	var seen bool
	h := auth.HTTPMiddleware(cfg, sessions, e.ur, orepo, def, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, seen = auth.UserFromContext(r.Context())
	}))
	call := func() bool {
		seen = false
		req := httptest.NewRequest("GET", "/x", nil)
		req.AddCookie(&http.Cookie{Name: "sid", Value: tok})
		h.ServeHTTP(httptest.NewRecorder(), req)
		return seen
	}
	if !call() {
		t.Fatal("active user must authenticate")
	}
	if _, err := e.st.Block(ctx, e.org, u.ID, "x"); err != nil {
		t.Fatal(err)
	}
	if call() {
		t.Fatal("blocked user must be anonymous on the very next request")
	}
	if _, err := e.st.Unblock(ctx, e.org, u.ID); err != nil {
		t.Fatal(err)
	}

	// Expiry job: an active guest past expires_at is blocked and its sessions end.
	gid := e.user(t, "ex", "ex@partner.example").ID
	_, _ = e.pool.Exec(ctx, `UPDATE adrient.users SET user_type='guest', expires_at=now()-interval '1 hour' WHERE id=$1::uuid`, gid)
	soon := e.user(t, "so", "so@partner.example").ID
	_, _ = e.pool.Exec(ctx, `UPDATE adrient.users SET user_type='guest', expires_at=now()+interval '2 days' WHERE id=$1::uuid`, soon)
	n, err := e.pol.RunExpiry(ctx, nil)
	if err != nil || n != 1 {
		t.Fatalf("expiry blocked %d %v", n, err)
	}
	x, _ := e.ur.GetByID(ctx, gid)
	if x.Status != "blocked" || x.BlockedReason != "expired" {
		t.Fatalf("after expiry: %+v", x)
	}
	if !e.audit.has("lifecycle.user.expire") || !e.audit.has("lifecycle.user.expiry_notice") {
		t.Fatalf("audit: %v", e.audit.rows)
	}
	if n, _ := e.pol.RunExpiry(ctx, nil); n != 0 {
		t.Fatal("second run must be a no-op")
	}
	// A guest whose expiry passed is also refused by the middleware before the job runs.
	if !(users.User{UserType: "guest", ExpiresAt: ptr(time.Now().Add(-time.Minute))}).Blocked(time.Now()) {
		t.Fatal("expired guest must count as blocked")
	}
}

func ptr[T any](v T) *T { return &v }

func TestHandlerAuditAndPermissions(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	adm := e.user(t, "a", "a@acme.test")
	mem := e.user(t, "m", "m@acme.test")
	victim := e.user(t, "v", "v@acme.test")
	authz.SetDefault(authz.NewService(authz.AdminOnlyLoader{IsAdmin: func(_ context.Context, _, uid string) (bool, error) { return uid == adm.ID, nil }}))
	t.Cleanup(func() { authz.SetDefault(nil) })
	h := &Handler{Store: e.st, Policy: e.pol, Audit: e.audit.fn}
	org := orgs.Org{ID: e.org, Slug: "default", DisplayName: "Acme"}
	do := func(u users.User, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		c := auth.WithOrg(auth.WithUser(req.Context(), u), org)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req.WithContext(c))
		return w
	}
	if w := do(mem, "POST", PathInvitations, `{"email":"g@x.example"}`); w.Code != 403 {
		t.Fatalf("member invite: %d", w.Code)
	}
	if w := do(mem, "GET", PathUsers, ""); w.Code != 403 {
		t.Fatalf("member list: %d", w.Code)
	}
	if len(e.audit.rows) != 0 {
		t.Fatal("refused member must not audit")
	}
	w := do(adm, "POST", PathInvitations, `{"email":"g@x.example","user_type":"guest"}`)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "/accept-invite?code=") || !e.audit.has("lifecycle.user.invite") {
		t.Fatalf("invite: %d %s", w.Code, w.Body.String())
	}
	if w := do(adm, "POST", PathInvitations, `{"email":"g@x.example","user_type":"root"}`); w.Code != 400 || !e.audit.has("lifecycle.invite.denied") {
		t.Fatalf("bad type: %d", w.Code)
	}
	if w := do(adm, "POST", PathUsers+"/"+adm.ID+"/block", `{}`); w.Code != 409 {
		t.Fatalf("self block: %d", w.Code)
	}
	if w := do(adm, "POST", PathUsers+"/"+victim.ID+"/block", `{"reason":"left"}`); w.Code != 200 || !e.audit.has("lifecycle.user.block") {
		t.Fatalf("block: %d", w.Code)
	}
	if w := do(adm, "GET", PathUsers+"?status=blocked", ""); !strings.Contains(w.Body.String(), "v@acme.test") || strings.Contains(w.Body.String(), "m@acme.test") {
		t.Fatalf("filter: %s", w.Body.String())
	}
	if w := do(adm, "PATCH", PathUsers+"/"+victim.ID, `{"user_type":"external"}`); w.Code != 200 || !e.audit.has("lifecycle.user.type_change") {
		t.Fatalf("type: %d", w.Code)
	}
	if w := do(adm, "GET", PathUsers+"?type=external", ""); !strings.Contains(w.Body.String(), "v@acme.test") {
		t.Fatalf("type filter: %s", w.Body.String())
	}
	if w := do(adm, "POST", PathUsers+"/"+victim.ID+"/unblock", ``); w.Code != 200 || !e.audit.has("lifecycle.user.unblock") {
		t.Fatalf("unblock: %d", w.Code)
	}
	if w := do(adm, "POST", PathRunExpiry, ``); w.Code != 200 || !e.audit.has("lifecycle.expiry.run") {
		t.Fatalf("run expiry: %d", w.Code)
	}
	_ = ctx
}

func TestIsExternalAndGuestApps(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	g := e.user(t, "g", "g@x.example")
	_, _ = e.pool.Exec(ctx, `UPDATE adrient.users SET user_type='guest', guest_apps='{drive}' WHERE id=$1::uuid`, g.ID)
	g, _ = e.ur.GetByID(ctx, g.ID)
	c := auth.WithUser(ctx, g)
	if !authz.IsExternal(c) || !authz.IsGuest(c) || authz.UserType(c) != "guest" {
		t.Fatal("guest must be external")
	}
	m := e.user(t, "mm", "mm@acme.test")
	m, _ = e.ur.GetByID(ctx, m.ID)
	if authz.IsExternal(auth.WithUser(ctx, m)) || authz.IsExternal(ctx) {
		t.Fatal("member/anonymous are not external")
	}
	roles := authz.NewService(authz.PGLoader{Pool: e.pool})
	apps := authz.NewAppService(authz.PGAppStore{Pool: e.pool}, roles)
	gc := auth.WithOrg(c, orgs.Org{ID: e.org})
	if !apps.CanOpen(gc, "drive") || apps.CanOpen(gc, "mail") || !apps.CanOpen(gc, "profile") {
		t.Fatal("guest limited to invitation apps + core apps")
	}
	set, _ := roles.Resolve(gc)
	if set.Can("org:member") || set.Can("crm:deal:read") {
		t.Fatal("guest must not hold member permissions")
	}
	ds, _ := apps.ResolveAll(gc, e.org, g.ID, set)
	allowed := 0
	for _, d := range ds {
		if d.Allowed {
			allowed++
		}
	}
	if allowed != 1 {
		t.Fatalf("launcher should list 1 app for guest, got %d", allowed)
	}
}
