package sso_test

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/sso"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/Adrient-tech/Adrient-Workspace/internal/zitadelmgmt"
)

// Needs the scratch Postgres (ADRIENT_TEST_DSN) and a local Zitadel
// (ZITADEL_URL, ZITADEL_TOKEN): providers are really created and removed there.
func TestSSOPG(t *testing.T) {
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	zm := zitadelmgmt.New(os.Getenv("ZITADEL_URL"), os.Getenv("ZITADEL_TOKEN"))
	if dsn == "" || zm == nil {
		t.Skip("ADRIENT_TEST_DSN / ZITADEL_URL / ZITADEL_TOKEN not set")
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
	var org string
	_ = pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org)
	var admin, member string
	_ = pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'i','adm','boss@acme.test','Boss') RETURNING id::text`, org).Scan(&admin)
	_ = pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'i','mem','ann@acme.test','Ann') RETURNING id::text`, org).Scan(&member)
	_, _ = pool.Exec(ctx, `INSERT INTO adrient.org_admins (org_id, user_id) VALUES ($1,$2)`, org, admin)
	_, _ = pool.Exec(ctx, `INSERT INTO adrient.org_domains (org_id, domain, status, verified_at, verify_txt) VALUES ($1,'acme.test','verified',now(),'x')`, org)

	s := &sso.Service{Pool: pool, ZM: zm}
	// A domain that is not verified cannot be routed.
	_, err = s.Create(ctx, org, admin, sso.CreateInput{Kind: "google", Name: "Google", ClientID: "c", ClientSecret: "s", Domains: []string{"evil.test"}})
	if err == nil {
		t.Fatal("unverified domain accepted")
	}
	conn, err := s.Create(ctx, org, admin, sso.CreateInput{Kind: "entra", Name: "Acme Entra", ClientID: "cid", ClientSecret: "sec",
		TenantID: "00000000-0000-0000-0000-000000000001", Domains: []string{"@Acme.test"}, JIT: true, Enforce: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Delete(ctx, org, admin, conn.ID) })
	if conn.IdPID == "" || len(conn.Domains) != 1 || conn.Domains[0] != "acme.test" {
		t.Fatalf("conn %+v", conn)
	}
	// The provider exists in Zitadel.
	list, err := zm.ListIdPs(ctx)
	found := false
	for _, p := range list {
		found = found || p.ID == conn.IdPID
	}
	if err != nil || !found {
		t.Fatalf("provider not in Zitadel: %v", err)
	}
	// JIT switched on the domain's auto-join.
	var auto bool
	var mode string
	_ = pool.QueryRow(ctx, `SELECT auto_join FROM adrient.org_domains WHERE domain='acme.test'`).Scan(&auto)
	_ = pool.QueryRow(ctx, `SELECT signup_mode FROM adrient.org_settings WHERE org_id::text=$1`, org).Scan(&mode)
	if !auto || mode != "domain_auto_join" {
		t.Fatalf("jit not applied: auto_join=%v mode=%s", auto, mode)
	}
	// A second connection cannot take the same domain.
	if _, err := s.Create(ctx, org, admin, sso.CreateInput{Kind: "google", Name: "G2", ClientID: "c", ClientSecret: "s", Domains: []string{"acme.test"}}); err == nil {
		t.Fatal("domain routed twice")
	}
	// Discovery and domain routing.
	if c, ok := s.Discover(ctx, "someone@ACME.test"); !ok || c.ID != conn.ID {
		t.Fatalf("discover %v %v", c, ok)
	}
	if _, ok := s.Discover(ctx, "someone@other.test"); ok {
		t.Fatal("unrelated domain routed")
	}
	// Enforcement: members must use SSO, admins keep a break-glass password.
	if err := s.PasswordGate(ctx, "ann@acme.test"); err == nil {
		t.Fatal("password sign-in allowed for an SSO-enforced domain")
	}
	if err := s.PasswordGate(ctx, "boss@acme.test"); err != nil {
		t.Fatalf("admin locked out: %v", err)
	}
	if err := s.PasswordGate(ctx, "x@other.test"); err != nil {
		t.Fatalf("other domain affected: %v", err)
	}
	off := false
	if _, err := s.Update(ctx, org, admin, conn.ID, sso.UpdateInput{Enforce: &off}); err != nil {
		t.Fatal(err)
	}
	if err := s.PasswordGate(ctx, "ann@acme.test"); err != nil {
		t.Fatalf("still enforced after switching off: %v", err)
	}
	disabled := false
	if _, err := s.Update(ctx, org, admin, conn.ID, sso.UpdateInput{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Discover(ctx, "ann@acme.test"); ok {
		t.Fatal("disabled connection still routes")
	}
}
