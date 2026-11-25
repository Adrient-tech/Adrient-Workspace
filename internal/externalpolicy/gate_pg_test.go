package externalpolicy_test

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/externalpolicy"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

func TestGatePG(t *testing.T) {
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
	mk := func(sub, email, typ string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, user_type)
VALUES ($1,'https://issuer.test',$2,$3,$2,$4) RETURNING id::text`, orgID, sub, email, typ).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	member := mk("m", "m@acme.test", "member")
	member2 := mk("m2", "m2@acme.test", "member")
	guestOK := mk("g1", "g@partner.test", "guest")
	guestBad := mk("g2", "g@evil.test", "guest")
	guest := guestOK
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.org_domains (org_id, domain, status, verified_at, verify_txt) VALUES ($1,'acme.test','verified',now(),'x')`, orgID); err != nil {
		t.Fatal(err)
	}
	roles := authz.NewService(authz.PGLoader{Pool: pool})
	authz.SetDefault(roles)
	ctl := authz.NewControlService(authz.PGControlStore{Pool: pool}, authz.PGAppStore{Pool: pool}, roles)
	authz.SetDefaultControls(ctl)
	t.Cleanup(func() { authz.SetDefaultControls(nil) })
	st := &externalpolicy.Store{Pool: pool}
	g := &externalpolicy.Gate{Store: st, Pool: pool}
	open := externalpolicy.Default()
	open.GuestInvites = "everyone" // who may invite is tested in enforce_pg_test.go
	if err := st.Put(ctx, orgID, open); err != nil {
		t.Fatal(err)
	}
	cx := func(id, email, typ string) context.Context {
		c := auth.WithUser(ctx, users.User{ID: id, Email: email, UserType: typ})
		return auth.WithOrg(c, orgs.Org{ID: orgID})
	}
	member1 := cx(member, "m@acme.test", "member")
	guestCtx := cx(guest, "g@partner.test", "guest")

	// Default (allow_all): member shares with a guest; blocked domain refuses everywhere.
	if err := g.CheckShareToUser(member1, guestBad); err != nil {
		t.Fatalf("default share: %v", err)
	}
	if err := st.PutPartner(ctx, orgID, member, externalpolicy.Partner{Domain: "evil.test", Effect: "block"}); err != nil {
		t.Fatal(err)
	}
	if err := g.CheckShareToUser(member1, guestBad); err == nil {
		t.Fatal("share to blocked domain allowed")
	}
	if err := g.CheckInvite(member1, orgID, "x@evil.test", "guest"); err == nil {
		t.Fatal("invite to blocked domain allowed")
	}
	if err := g.CheckInvite(member1, orgID, "x@evil.test", ""); err == nil {
		t.Fatal("accept from blocked domain allowed")
	}
	if err := g.CheckInvite(member1, orgID, "x@fine.test", "guest"); err != nil {
		t.Fatalf("invite ok domain: %v", err)
	}
	if err := g.CheckChatMembers(member1, []string{member, guestBad}); err == nil {
		t.Fatal("chat with blocked domain allowed")
	}
	if err := g.CheckChatMembers(member1, []string{member, member2}); err != nil {
		t.Fatalf("internal chat: %v", err)
	}

	// Allowlist mode: only partner.test.
	if err := st.Put(ctx, orgID, func() externalpolicy.Policy { p := open; p.DomainMode = externalpolicy.ModeAllowlist; return p }()); err != nil {
		t.Fatal(err)
	}
	if err := g.CheckInvite(member1, orgID, "x@partner.test", "guest"); err == nil {
		t.Fatal("unlisted domain invited under allowlist")
	}
	if err := st.PutPartner(ctx, orgID, member, externalpolicy.Partner{Domain: "partner.test", Effect: "allow"}); err != nil {
		t.Fatal(err)
	}
	if err := g.CheckInvite(member1, orgID, "x@partner.test", "guest"); err != nil {
		t.Fatalf("listed domain: %v", err)
	}
	if err := g.CheckShareToUser(member1, guestOK); err != nil {
		t.Fatalf("share to allowed partner: %v", err)
	}

	// Guest reshare control (default false) and external_sharing=off rule.
	if err := g.CheckShareToUser(guestCtx, member2); err == nil {
		t.Fatal("guest reshare allowed by default")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.app_control_rules (org_id, app_id, control, value, subject_type, subject_id)
VALUES ($1,'drive','drive.guest_reshare','true','user_type','guest'), ($1,'drive','drive.external_sharing','"off"','user_type','guest')`, orgID); err != nil {
		t.Fatal(err)
	}
	ctl.Invalidate()
	if err := g.CheckShareToUser(guestCtx, member2); err != nil {
		t.Fatalf("guest sharing with an internal person is not external sharing: %v", err)
	}
	if err := g.CheckShareToUser(guestCtx, guestOK); err != nil && guestOK != guest {
		t.Logf("self/other: %v", err)
	}
	// Guest sharing to another guest (external) with external_sharing off -> blocked.
	other := mk("g3", "g3@partner.test", "guest")
	err = g.CheckShareToUser(guestCtx, other)
	if err == nil {
		t.Fatal("guest external sharing allowed although off")
	}
	if d, ok := authz.AsDenied(err); !ok || d.Control != "drive.external_sharing" {
		t.Fatalf("wrong denial: %v", err)
	}
	if err := g.CheckLink(guestCtx); err == nil {
		t.Fatal("link allowed with sharing off")
	}
	// Directory visibility: guest sees self only until a group/sponsor/grant exists.
	ids, restricted := g.GuestVisible(guestCtx)
	if !restricted || len(ids) != 1 || !ids[guest] {
		t.Fatalf("visible %v restricted=%v", ids, restricted)
	}
	if _, restricted := g.GuestVisible(member1); restricted {
		t.Fatal("member must not be restricted")
	}
	if _, err := pool.Exec(ctx, `UPDATE adrient.users SET sponsor_id=$2::uuid WHERE id=$1::uuid`, guest, member); err != nil {
		t.Fatal(err)
	}
	if ids, _ := g.GuestVisible(guestCtx); !ids[member] || ids[member2] {
		t.Fatalf("sponsor visibility %v", ids)
	}
	// Meet join: guests allowed by default; off -> refused.
	if err := g.CheckMeetJoin(guestCtx); err != nil {
		t.Fatalf("meet join: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.app_control_rules (org_id, app_id, control, value, subject_type, subject_id) VALUES ($1,'meet','meet.guests_allowed','false','user_type','guest')`, orgID); err != nil {
		t.Fatal(err)
	}
	ctl.Invalidate()
	if err := g.CheckMeetJoin(guestCtx); err == nil {
		t.Fatal("guest joined meet while disallowed")
	}
}
