package seats_test

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/lifecycle"
	"github.com/Adrient-tech/Adrient-Workspace/internal/seats"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

func TestSeatsPG(t *testing.T) {
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
	var org string
	_ = pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org)
	mk := func(sub, email, typ string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, user_type) VALUES ($1,'i',$2,$3,$2,$4) RETURNING id::text`, org, sub, email, typ).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	admin := mk("a", "a@x.test", "member")
	u1 := mk("u1", "u1@x.test", "member")
	_ = mk("g", "g@x.test", "guest")
	roles := authz.NewService(authz.PGLoader{Pool: pool})
	apps := authz.NewAppService(authz.PGAppStore{Pool: pool}, roles)
	svc := &seats.Service{Pool: pool, Apps: apps, Roles: roles, Invites: &lifecycle.Store{Pool: pool}}

	// Seats: guests do not count.
	us, _ := svc.GetUsage(ctx, org)
	if us.Used != 2 || us.Guests != 1 || us.Seats != nil || us.Free != nil {
		t.Fatalf("usage %+v", us)
	}
	if err := svc.Check(ctx, org); err != nil {
		t.Fatalf("unlimited plan: %v", err)
	}
	three, two := 3, 2
	if err := svc.PutPlan(ctx, org, admin, seats.Plan{Plan: "team", Seats: &three}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Check(ctx, org); err != nil {
		t.Fatalf("free seat: %v", err)
	}
	if err := svc.PutPlan(ctx, org, admin, seats.Plan{Plan: "team", Seats: &two}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Check(ctx, org); err == nil {
		t.Fatal("full plan allowed another member")
	}

	// Bulk invite with seats: raise to 3 seats (1 free), 4 lines.
	_ = svc.PutPlan(ctx, org, admin, seats.Plan{Plan: "team", Seats: &three})
	csv := "email,name\nnew1@x.test,New One\nnew2@x.test,New Two\nnot-an-email\nu1@x.test,Existing\nnew1@x.test,Dup\n"
	rows, err := svc.BulkInvite(ctx, org, admin, "Acme", "http://h", strings.NewReader(csv), true)
	if err != nil {
		t.Fatal(err)
	}
	st := map[string]string{}
	for _, r := range rows {
		st[r.Email+"/"+r.Status] = r.Reason
	}
	if len(rows) != 5 || st["new1@x.test/invited"] != "" || st["new2@x.test/error"] != "no free seat left" || st["not-an-email/error"] == "" || st["new1@x.test/skipped"] != "listed twice" {
		t.Fatalf("dry run rows: %+v", rows)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.invitations`).Scan(&n)
	if n != 0 {
		t.Fatalf("dry run created %d invitations", n)
	}
	rows, err = svc.BulkInvite(ctx, org, admin, "Acme", "http://h", strings.NewReader(csv), false)
	if err != nil {
		t.Fatal(err)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.invitations WHERE user_type='member'`).Scan(&n)
	if n != 1 || !strings.HasPrefix(rows[0].Link, "http://h/accept-invite?code=") {
		t.Fatalf("invitations %d rows %+v", n, rows)
	}
	if _, err := svc.BulkInvite(ctx, org, admin, "Acme", "http://h", strings.NewReader(""), true); err == nil {
		t.Fatal("empty file accepted")
	}

	// Per-app seats: assign/unassign with a cap of 1 on crm.
	one := 1
	if err := svc.SetCap(ctx, org, admin, "crm", &one); err != nil {
		t.Fatal(err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM adrient.app_access_policies WHERE service_id='crm'`)
	apps.Invalidate()
	if err := svc.Assign(ctx, org, admin, "crm", "user", u1, true); err != nil {
		t.Fatalf("assign within cap: %v", err)
	}
	if err := svc.Assign(ctx, org, admin, "crm", "user", admin, true); err == nil {
		t.Fatal("assign over the cap allowed")
	}
	rep, err := svc.LicenseReport(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range rep.Apps {
		if a.AppID == "crm" && (a.Assigned != 1 || a.Cap == nil || a.Over) {
			t.Fatalf("crm usage %+v", a)
		}
	}
	if err := svc.Assign(ctx, org, admin, "crm", "user", u1, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.Assign(ctx, org, admin, "nope", "user", u1, true); err == nil {
		t.Fatal("unknown app accepted")
	}

	// Session policy is validated and resolved per user.
	h12, idle := 12, 3
	if err := svc.PutSessionSettings(ctx, org, admin, seats.SessionSettings{MaxHours: &h12, IdleMinutes: &idle, FailThreshold: 5}); err == nil {
		t.Fatal("idle timeout of 3 minutes accepted")
	}
	idle = 30
	if err := svc.PutSessionSettings(ctx, org, admin, seats.SessionSettings{MaxHours: &h12, IdleMinutes: &idle, FailThreshold: 5, AlertNewDev: true}); err != nil {
		t.Fatal(err)
	}
	if l := svc.LimitsFor(ctx, u1); l.MaxAge.Hours() != 12 || l.Idle.Minutes() != 30 || l.MaxConcurrent != 0 {
		t.Fatalf("limits %+v", l)
	}

	// Login history: new device after a known one is suspicious and alerts; repeated failures too.
	var alerts []string
	svc.Alert = func(_ context.Context, _, event, _, _ string) { alerts = append(alerts, event) }
	svc.Record(ctx, auth.LoginEvent{OrgID: org, UserID: u1, Email: "u1@x.test", IP: "1.1.1.1", UserAgent: "Firefox", Method: "sso", Success: true})
	svc.Record(ctx, auth.LoginEvent{OrgID: org, UserID: u1, Email: "u1@x.test", IP: "1.1.1.1", UserAgent: "Firefox", Method: "sso", Success: true})
	if len(alerts) != 0 {
		t.Fatalf("known device alerted: %v", alerts)
	}
	svc.Record(ctx, auth.LoginEvent{OrgID: org, UserID: u1, Email: "u1@x.test", IP: "9.9.9.9", UserAgent: "Chrome", Method: "sso", Success: true})
	if len(alerts) != 1 {
		t.Fatalf("new device not alerted: %v", alerts)
	}
	for i := 0; i < 5; i++ {
		svc.Record(ctx, auth.LoginEvent{Email: "u1@x.test", IP: "5.5.5.5", UserAgent: "curl", Method: "password", Reason: "bad_credentials"})
	}
	if len(alerts) != 2 {
		t.Fatalf("failure burst not alerted exactly once: %v", alerts)
	}
	sus, _ := svc.Logins(ctx, org, "", "suspicious", 50)
	if len(sus) != 2 {
		t.Fatalf("suspicious rows %d", len(sus))
	}
	if all, _ := svc.Logins(ctx, org, u1, "", 50); len(all) < 8 {
		t.Fatalf("history rows %d", len(all))
	}
}
