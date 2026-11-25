package offboard_test

import (
	"context"
	"errors"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/offboard"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

func TestOffboardPG(t *testing.T) {
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
	mk := func(sub, email string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'i',$2,$3,$2) RETURNING id::text`, org, sub, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	admin, leaver, heir := mk("admin", "admin@a.test"), mk("leaver", "leaver@a.test"), mk("heir", "heir@a.test")
	for i := 0; i < 3; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, name, mime_type) VALUES ($1,$2,$3,'text/plain')`, org, leaver, "f"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.calendar_events (org_id, owner_id, title, start_at, end_at) VALUES ($1,$2,'standup',now(),now()+interval '1 hour')`, org, leaver); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.chat_channels (org_id, kind, name, member_ids) VALUES ($1,'group','team',$2::jsonb)`, org, `["`+leaver+`","`+heir+`"]`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.sessions (token, user_id, expires_at) VALUES ('tok1',$1, now()+interval '1 day')`, leaver); err != nil {
		t.Fatal(err)
	}
	var mailed []string
	svc := &offboard.Service{Pool: pool, Mail: func(_ context.Context, to []string, subject, _ string) error {
		mailed = append(mailed, subject)
		return nil
	}}
	if err := svc.PutAlerts(ctx, org, offboard.AlertSettings{Recipients: []string{"boss@a.test"}, Events: []string{"user.delete"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.PutAlerts(ctx, org, offboard.AlertSettings{Recipients: []string{"not mail"}}); err == nil {
		t.Fatal("bad alert recipient accepted")
	}

	// Legal hold blocks delete (and is recorded as refused).
	hold, err := svc.CreateHold(ctx, org, admin, offboard.Hold{Name: "Case 42", UserIDs: []string{leaver}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Delete(ctx, offboard.DeleteInput{OrgID: org, UserID: leaver, Actor: admin, TransferTo: heir})
	var he *offboard.HoldError
	if !errors.As(err, &he) || he.Hold != "Case 42" {
		t.Fatalf("delete under hold: %v", err)
	}
	if err := svc.SoftDelete(ctx, org, leaver, ""); err == nil {
		t.Fatal("SCIM-style delete bypassed the hold")
	}
	var st string
	_ = pool.QueryRow(ctx, `SELECT status FROM adrient.users WHERE id::text=$1`, leaver).Scan(&st)
	if st != "active" {
		t.Fatalf("user changed although the delete was refused: %s", st)
	}
	if err := svc.ReleaseHold(ctx, org, admin, hold.ID); err != nil {
		t.Fatal(err)
	}

	// Delete with transfer.
	res, err := svc.Delete(ctx, offboard.DeleteInput{OrgID: org, UserID: leaver, Actor: admin, TransferTo: heir, ForwardMail: true, LeaveChats: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Transferred["Drive files and folders"] != 3 || res.Transferred["Calendar events"] != 1 || res.MailForward != "heir@a.test" || res.ChatsLeft != 1 {
		t.Fatalf("result %+v", res)
	}
	var owned, ev, rules, chans, sess int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.drive_files WHERE owner_id::text=$1`, heir).Scan(&owned)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.calendar_events WHERE owner_id::text=$1`, heir).Scan(&ev)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.mail_rules WHERE owner_id::text=$1 AND act_forward='heir@a.test'`, leaver).Scan(&rules)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.chat_channels WHERE member_ids ? $1`, leaver).Scan(&chans)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.sessions WHERE user_id::text=$1 AND revoked_at IS NULL`, leaver).Scan(&sess)
	if owned != 3 || ev != 1 || rules != 1 || chans != 0 || sess != 0 {
		t.Fatalf("owned=%d events=%d rules=%d chans=%d sessions=%d", owned, ev, rules, chans, sess)
	}
	if len(mailed) != 1 {
		t.Fatalf("admin alert e-mails: %v", mailed)
	}

	// Restore inside the window; an expired window cannot be restored.
	if err := svc.Restore(ctx, org, leaver, admin); err != nil {
		t.Fatal(err)
	}
	_ = pool.QueryRow(ctx, `SELECT status FROM adrient.users WHERE id::text=$1`, leaver).Scan(&st)
	if st != "active" {
		t.Fatalf("restored status %s", st)
	}
	if _, err := svc.Delete(ctx, offboard.DeleteInput{OrgID: org, UserID: leaver, Actor: admin}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE adrient.users SET purge_after = now() - interval '1 day' WHERE id::text=$1`, leaver); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(ctx, org, leaver, admin); err == nil {
		t.Fatal("restore after the window")
	}
	// Purge respects holds, then anonymizes.
	if _, err := svc.CreateHold(ctx, org, admin, offboard.Hold{Name: "Org-wide", OrgWide: true}); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.Purge(ctx); n != 0 {
		t.Fatalf("purged %d under an org-wide hold", n)
	}
	holds, _ := svc.ListHolds(ctx, org)
	for _, h := range holds {
		if h.ReleasedAt == nil {
			_ = svc.ReleaseHold(ctx, org, admin, h.ID)
		}
	}
	if n, _ := svc.Purge(ctx); n != 1 {
		t.Fatalf("purge count %d", n)
	}
	var email string
	_ = pool.QueryRow(ctx, `SELECT email FROM adrient.users WHERE id::text=$1`, leaver).Scan(&email)
	if email != "deleted-"+leaver+"@deleted.invalid" {
		t.Fatalf("not anonymized: %s", email)
	}

	// Suspend / archive guards.
	if err := svc.Suspend(ctx, org, admin, admin, ""); err == nil {
		t.Fatal("suspended self")
	}
	if err := svc.Archive(ctx, org, heir, admin, "left the company"); err != nil {
		t.Fatal(err)
	}
	var arch *time.Time
	_ = pool.QueryRow(ctx, `SELECT archived_at FROM adrient.users WHERE id::text=$1`, heir).Scan(&arch)
	if arch == nil {
		t.Fatal("archived_at not set")
	}
	if err := svc.Unsuspend(ctx, org, heir, admin); err != nil {
		t.Fatal(err)
	}
}
