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

// TestEnforcementPG covers the controls that used to be saved but not enforced:
// chat upload policy, link visibility default, anonymous Meet, guest-invite
// levels and cross-org trust. It needs a scratch Postgres (ADRIENT_TEST_DSN).
func TestEnforcementPG(t *testing.T) {
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
	var otherOrg string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ('other','Other Co') RETURNING id::text`).Scan(&otherOrg); err != nil {
		t.Fatal(err)
	}
	mk := func(org, sub, email, typ string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, user_type)
VALUES ($1,'https://issuer.test',$2,$3,$2,$4) RETURNING id::text`, org, sub, email, typ).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	member := mk(orgID, "m", "m@acme.test", "member")
	member2 := mk(orgID, "m2", "m2@acme.test", "member")
	guest := mk(orgID, "g", "g@partner.test", "guest")
	outsider := mk(otherOrg, "o", "o@other.test", "member")
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.org_domains (org_id, domain, status, verified_at, verify_txt) VALUES ($1,'acme.test','verified',now(),'x'), ($2,'other.test','verified',now(),'y')`, orgID, otherOrg); err != nil {
		t.Fatal(err)
	}
	roles := authz.NewService(authz.PGLoader{Pool: pool})
	authz.SetDefault(roles)
	ctl := authz.NewControlService(authz.PGControlStore{Pool: pool}, authz.PGAppStore{Pool: pool}, roles)
	authz.SetDefaultControls(ctl)
	t.Cleanup(func() { authz.SetDefaultControls(nil) })
	st := &externalpolicy.Store{Pool: pool}
	g := &externalpolicy.Gate{Store: st, Pool: pool}
	cx := func(id, email, typ string) context.Context {
		c := auth.WithUser(ctx, users.User{ID: id, OrgID: orgID, Email: email, UserType: typ})
		return auth.WithOrg(c, orgs.Org{ID: orgID})
	}
	m1 := cx(member, "m@acme.test", "member")
	gctx := cx(guest, "g@partner.test", "guest")
	setRule := func(app, control, value, subjType, subjID string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO adrient.app_control_rules (org_id, app_id, control, value, subject_type, subject_id) VALUES ($1,$2,$3,$4::jsonb,$5,$6)`,
			orgID, app, control, value, subjType, subjID); err != nil {
			t.Fatal(err)
		}
		ctl.Invalidate()
	}

	t.Run("chat file upload in external chats", func(t *testing.T) {
		// Default is on: attachments are fine everywhere.
		if err := g.CheckChatUpload(m1, []string{member, guest}); err != nil {
			t.Fatalf("default: %v", err)
		}
		setRule("chat", "chat.file_upload_external", "false", "everyone", "")
		if err := g.CheckChatUpload(m1, []string{member, guest}); err == nil {
			t.Fatal("upload to a chat with a guest allowed although off")
		} else if d, ok := authz.AsDenied(err); !ok || d.Control != "chat.file_upload_external" {
			t.Fatalf("wrong denial %v", err)
		}
		if err := g.CheckChatUpload(m1, []string{member, outsider}); err == nil {
			t.Fatal("upload to a chat with another organization's member allowed although off")
		}
		if err := g.CheckChatUpload(m1, []string{member, member2}); err != nil {
			t.Fatalf("internal chat must not be affected: %v", err)
		}
		if err := g.CheckChatUpload(gctx, []string{guest, member}); err == nil {
			t.Fatal("a guest posting a file allowed although off")
		}
	})

	t.Run("link sharing default", func(t *testing.T) {
		if v := g.LinkVisibility(m1); v != "org_only" {
			t.Fatalf("default %q", v)
		}
		setRule("drive", "drive.link_sharing_default", `"anyone"`, "user", member)
		if v := g.LinkVisibility(m1); v != "anyone" {
			t.Fatalf("rule %q", v)
		}
		if v := g.LinkVisibility(cx(member2, "m2@acme.test", "member")); v != "org_only" {
			t.Fatalf("other user %q", v)
		}
	})

	t.Run("anonymous meet", func(t *testing.T) {
		if g.AnonymousMeetAllowed(ctx, orgID) {
			t.Fatal("anonymous join allowed by default")
		}
		setRule("meet", "meet.allow_anonymous", "true", "everyone", "")
		if !g.AnonymousMeetAllowed(ctx, orgID) {
			t.Fatal("anonymous join refused although on")
		}
		if g.AnonymousMeetAllowed(ctx, otherOrg) {
			t.Fatal("rule leaked into another organization")
		}
	})

	t.Run("guest invite levels", func(t *testing.T) {
		put := func(level string) {
			p := externalpolicy.Default()
			p.GuestInvites = level
			if err := st.Put(ctx, orgID, p); err != nil {
				t.Fatal(err)
			}
			st.Invalidate()
		}
		exp := map[string]struct{ member, guest bool }{
			"none": {false, false}, "admins": {false, false}, "admins_and_sponsors": {false, false},
			"members": {true, false}, "everyone": {true, false},
		}
		for level, want := range exp {
			put(level)
			if got := g.InviterAllowed(m1, level); got != want.member {
				t.Errorf("level %s member: got %v want %v", level, got, want.member)
			}
			if got := g.InviterAllowed(gctx, level); got != want.guest {
				t.Errorf("level %s guest: got %v want %v", level, got, want.guest)
			}
		}
		put("admins_and_sponsors")
		if _, err := pool.Exec(ctx, `UPDATE adrient.users SET sponsor_id=$1::uuid WHERE id=$2::uuid`, member, guest); err != nil {
			t.Fatal(err)
		}
		if !g.InviterAllowed(m1, "admins_and_sponsors") {
			t.Error("an existing sponsor must be allowed at admins_and_sponsors")
		}
		if g.InviterAllowed(cx(member2, "m2@acme.test", "member"), "admins_and_sponsors") {
			t.Error("a non-sponsor must not be allowed at admins_and_sponsors")
		}
		put("members")
		if err := g.CheckInvite(m1, orgID, "new@fine.test", "guest"); err != nil {
			t.Errorf("member invite at level members: %v", err)
		}
		put("admins")
		if err := g.CheckInvite(m1, orgID, "new@fine.test", "guest"); err == nil {
			t.Error("member invited a guest at level admins")
		}
		put("none")
		if err := g.CheckInvite(m1, orgID, "new@fine.test", "external"); err == nil {
			t.Error("invite at level none")
		}
	})

	t.Run("cross-org trust", func(t *testing.T) {
		p := externalpolicy.Default()
		p.DomainMode = externalpolicy.ModeAllowlist
		if err := st.Put(ctx, orgID, p); err != nil {
			t.Fatal(err)
		}
		setRule("drive", "drive.external_sharing", `"domains"`, "everyone", "")
		if err := g.CheckShareToUser(m1, outsider); err == nil {
			t.Fatal("share to unlisted organization allowed under allowlist")
		}
		if _, err := st.ProposeTrust(ctx, orgID, "other.test", nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := g.CheckShareToUser(m1, outsider); err == nil {
			t.Fatal("a pending trust must not allow sharing")
		}
		if err := st.AcceptTrust(ctx, otherOrg, orgID, nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := g.CheckShareToUser(m1, outsider); err != nil {
			t.Fatalf("active trust: %v", err)
		}
		if err := g.CheckChatMembers(m1, []string{member, outsider}); err != nil {
			t.Fatalf("active trust chat: %v", err)
		}
		// Our side switches chat off for the partner: chat is refused, sharing still works.
		if _, err := st.ProposeTrust(ctx, orgID, "other.test", map[string]bool{"chat": false}, nil); err != nil {
			t.Fatal(err)
		}
		if err := st.AcceptTrust(ctx, otherOrg, orgID, nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := g.CheckChatMembers(m1, []string{member, outsider}); err == nil {
			t.Fatal("chat allowed although outbound chat is off")
		}
		if err := g.CheckShareToUser(m1, outsider); err != nil {
			t.Fatalf("share still trusted: %v", err)
		}
		// An explicit block of the domain beats the trust.
		if err := st.PutPartner(ctx, orgID, member, externalpolicy.Partner{Domain: "other.test", Effect: "block"}); err != nil {
			t.Fatal(err)
		}
		if err := g.CheckShareToUser(m1, outsider); err == nil {
			t.Fatal("explicit block must beat the trust")
		}
		if _, err := st.DeletePartner(ctx, orgID, "other.test"); err != nil {
			t.Fatal(err)
		}
		// Either side can revoke.
		if err := st.RevokeTrust(ctx, otherOrg, orgID); err != nil {
			t.Fatal(err)
		}
		if err := g.CheckShareToUser(m1, outsider); err == nil {
			t.Fatal("revoked trust still allows sharing")
		}
		if l, _ := st.ListTrusts(ctx, orgID); len(l) != 0 {
			t.Fatalf("revoked trust listed: %v", l)
		}
	})
}
