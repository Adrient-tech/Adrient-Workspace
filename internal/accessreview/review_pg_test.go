package accessreview_test

import (
	"context"
	"errors"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/accessreview"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

func TestAccessReviewPG(t *testing.T) {
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
	mk := func(sub, typ string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, user_type) VALUES ($1,'i',$2,$2||'@x.test',$2,$3) RETURNING id::text`, org, sub, typ).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	boss, lead, ann, bob, guest := mk("boss", "member"), mk("lead", "member"), mk("ann", "member"), mk("bob", "member"), mk("guest", "guest")
	var grp string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.groups (org_id, owner_id, name, member_ids) VALUES ($1,$2,'Finance',$3::jsonb) RETURNING id::text`, org, lead,
		`["`+ann+`","`+bob+`","`+lead+`"]`).Scan(&grp); err != nil {
		t.Fatal(err)
	}
	_, _ = pool.Exec(ctx, `INSERT INTO adrient.org_admins (org_id, user_id) VALUES ($1,$2),($1,$3)`, org, boss, ann)

	var suspended []string
	var notified []string
	svc := &accessreview.Service{Pool: pool,
		Suspend: func(_ context.Context, _, uid, _, _ string) error {
			suspended = append(suspended, uid)
			_, err := pool.Exec(ctx, `UPDATE adrient.users SET status='blocked' WHERE id::text=$1`, uid)
			return err
		},
		Notify: func(_ context.Context, _, uid, title, _, _ string) { notified = append(notified, uid) }}

	// Group campaign: the group owner reviews members; the owner's own row falls to the creator.
	c, err := svc.Create(ctx, org, boss, accessreview.CreateInput{Name: "Q4 groups", Kind: accessreview.KindGroups, OnNoResponse: "remove"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 3 || c.Pending != 3 {
		t.Fatalf("campaign %+v", c)
	}
	items, _ := svc.Items(ctx, org, c.ID)
	var annItem, bobItem, leadItem accessreview.Item
	for _, it := range items {
		switch it.SubjectID {
		case ann:
			annItem = it
		case bob:
			bobItem = it
		case lead:
			leadItem = it
		}
	}
	if annItem.ReviewerID != lead || leadItem.ReviewerID != boss {
		t.Fatalf("reviewers: ann->%s lead->%s", annItem.ReviewerID, leadItem.ReviewerID)
	}
	if len(notified) == 0 {
		t.Fatal("reviewers were not notified")
	}
	// Permission rules.
	if err := svc.Decide(ctx, org, bob, false, annItem.ID, "keep", ""); !errors.Is(err, accessreview.ErrForbid) {
		t.Fatalf("a non-reviewer decided: %v", err)
	}
	if err := svc.Decide(ctx, org, ann, true, annItem.ID, "keep", ""); !errors.Is(err, accessreview.ErrForbid) {
		t.Fatalf("self review allowed: %v", err)
	}
	if err := svc.Decide(ctx, org, lead, false, annItem.ID, "maybe", ""); err == nil {
		t.Fatal("bad decision accepted")
	}
	if err := svc.Decide(ctx, org, lead, false, annItem.ID, "keep", "still on the team"); err != nil {
		t.Fatal(err)
	}
	// Bob is left unanswered: on_no_response=remove takes him out automatically.
	_ = bobItem
	if mine, _ := svc.Mine(ctx, org, lead); len(mine) != 1 {
		t.Fatalf("lead's pending items: %d", len(mine))
	}
	closed, err := svc.Close(ctx, org, boss, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != "closed" || closed.Removed != 2 || closed.Kept != 1 {
		t.Fatalf("closed %+v", closed)
	}
	var members string
	_ = pool.QueryRow(ctx, `SELECT member_ids::text FROM adrient.groups WHERE id::text=$1`, grp).Scan(&members)
	if !strings.Contains(members, ann) || strings.Contains(members, bob) || strings.Contains(members, lead) {
		t.Fatalf("members after review: %s", members)
	}
	var gm int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.group_members WHERE group_id::text=$1`, grp).Scan(&gm)
	if gm != 1 {
		t.Fatalf("group_members not synced: %d", gm)
	}
	if err := svc.Decide(ctx, org, lead, false, annItem.ID, "remove", ""); err == nil {
		t.Fatal("decision on a closed campaign")
	}

	// Guest campaign: sponsor reviews, removal suspends.
	_, _ = pool.Exec(ctx, `UPDATE adrient.users SET sponsor_id=$2::uuid WHERE id::text=$1`, guest, lead)
	g, err := svc.Create(ctx, org, boss, accessreview.CreateInput{Name: "Guests", Kind: accessreview.KindGuests})
	if err != nil {
		t.Fatal(err)
	}
	gi, _ := svc.Items(ctx, org, g.ID)
	if len(gi) != 1 || gi[0].ReviewerID != lead {
		t.Fatalf("guest items %+v", gi)
	}
	if err := svc.Decide(ctx, org, lead, false, gi[0].ID, "remove", "contract ended"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Close(ctx, org, boss, g.ID); err != nil {
		t.Fatal(err)
	}
	if len(suspended) != 1 || suspended[0] != guest {
		t.Fatalf("guest not suspended: %v", suspended)
	}

	// Admin roles: boss reviews ann's admin role; boss's own role goes to another admin.
	a, err := svc.Create(ctx, org, boss, accessreview.CreateInput{Name: "Admins", Kind: accessreview.KindAdmins})
	if err != nil {
		t.Fatal(err)
	}
	ai, _ := svc.Items(ctx, org, a.ID)
	for _, it := range ai {
		if it.SubjectID == boss && it.ReviewerID != ann {
			t.Fatalf("boss's admin role must be reviewed by another admin, got %s", it.ReviewerName)
		}
		if it.SubjectID == ann {
			if err := svc.Decide(ctx, org, boss, false, it.ID, "remove", "no longer needs admin"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := svc.Close(ctx, org, boss, a.ID); err != nil {
		t.Fatal(err)
	}
	var isAdmin bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.org_admins WHERE user_id::text=$1)`, ann).Scan(&isAdmin)
	if isAdmin {
		t.Fatal("admin role not removed")
	}
	csv, _ := svc.Export(ctx, org, c.ID)
	if !strings.Contains(csv, "no response") && !strings.Contains(csv, "No response") {
		t.Fatalf("export lacks the no-response note: %s", csv)
	}
	if _, err := svc.Create(ctx, org, boss, accessreview.CreateInput{Name: "x", Kind: "bogus"}); err == nil {
		t.Fatal("bad kind accepted")
	}
}
