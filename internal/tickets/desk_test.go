package tickets

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{" Billing ", "billing", "Refund Request", "#vip,", "", "x"})
	want := []string{"billing", "refund-request", "vip", "x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v != %v", got, want)
	}
	if got := AddRemoveTags([]string{"a", "b"}, []string{"c", "B"}, []string{"a"}); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("add/remove %v", got)
	}
	if NormalizeType("INCIDENT") != "incident" || NormalizeType("nope") != "question" {
		t.Fatal("type")
	}
}

func TestParseSearchAndMatch(t *testing.T) {
	q := ParseSearch(`status:open tag:billing "double charge" #12 foo:bar requester:ana`)
	if q.Number != 12 || len(q.Filters) != 3 {
		t.Fatalf("%+v", q)
	}
	if !reflect.DeepEqual(q.Words, []string{"double charge", "foo:bar"}) {
		t.Fatalf("words %v", q.Words)
	}
	tk := DeskTicket{Ref: "SUP-12", Number: 12, Title: "A double charge", Status: "open", Priority: "high", Type: "incident",
		Tags: []string{"billing"}, RequesterName: "Ana Ray", Assignee: "u1", Source: "web"}
	q2 := ParseSearch(`status:open tag:billing requester:ana double`)
	if !q2.Matches(tk, "u1") {
		t.Fatal("should match")
	}
	for _, bad := range []string{"status:closed", "assignee:none", "tag:other", "#13", "priority:low", "type:problem", "assignee:me", "status:solved", "via:email", "nothere"} {
		me := "u2"
		if bad == "assignee:me" {
			me = "u2"
		}
		if ParseSearch(bad).Matches(tk, me) {
			t.Fatalf("%q should not match", bad)
		}
	}
	if !ParseSearch("assignee:me status:unsolved type:incident #12").Matches(tk, "u1") {
		t.Fatal("me/unsolved/type/number")
	}
}

func TestViews(t *testing.T) {
	now := time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)
	old := now.Add(-5 * 24 * time.Hour)
	res := now.Add(-2 * time.Hour)
	ts := []DeskTicket{
		{ID: "1", Status: "open", Priority: "urgent", Assignee: "me", UpdatedAt: now},
		{ID: "2", Status: "open", Priority: "normal", UpdatedAt: old, Type: "incident"},
		{ID: "3", Status: "resolved", ResolvedAt: &res, UpdatedAt: res},
		{ID: "4", Status: "closed", MergedInto: "1", UpdatedAt: now},
	}
	counts := map[string]int{}
	for _, v := range ViewCounts(ts, "me", now) {
		counts[v.ID] = v.Count
	}
	want := map[string]int{"mine": 1, "unassigned": 1, "urgent": 1, "all_open": 2, "stale": 1, "incidents": 1, "solved_recent": 1}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("%v != %v", counts, want)
	}
	if _, ok := InView(ts, "nope", "me", now); ok {
		t.Fatal("unknown view")
	}
	l, _ := InView(ts, "all_open", "me", now)
	if len(l) != 2 || l[0].ID != "1" {
		t.Fatalf("order %+v", l)
	}
}

func TestMacroValidateAndPlaceholders(t *testing.T) {
	m := Macro{Name: " ", Actions: MacroActions{Status: "closed"}}
	if m.Validate(DefaultStatuses) == nil {
		t.Fatal("name required")
	}
	m = Macro{Name: "x"}
	if m.Validate(DefaultStatuses) == nil {
		t.Fatal("action required")
	}
	m = Macro{Name: "x", Actions: MacroActions{Status: "weird"}}
	if m.Validate(DefaultStatuses) == nil {
		t.Fatal("status check")
	}
	m = Macro{Name: "Solve", Actions: MacroActions{Status: "resolved", Priority: "URGENT", AddTags: []string{"Done"}}}
	if err := m.Validate(DefaultStatuses); err != nil || m.Actions.Priority != "urgent" || m.Actions.AddTags[0] != "done" {
		t.Fatalf("%v %+v", err, m)
	}
	out := FillPlaceholders("Hi {{requester.first_name}}, {{ticket.ref}} by {{agent.name}} {{unknown.x}}", DeskTicket{Ref: "SUP-1", RequesterName: "Ana Ray"}, "Sam")
	if out != "Hi Ana, SUP-1 by Sam {{unknown.x}}" {
		t.Fatal(out)
	}
}

func TestDeskLifecycle(t *testing.T) {
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS adrient CASCADE"); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var org, user string
	_ = pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org)
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		VALUES ($1,'test','subject-tk','tk@adrient.localtest.me','Sam Agent') RETURNING id::text`, org).Scan(&user); err != nil {
		t.Fatal(err)
	}
	r := NewRepository(pool)
	ctx = WithActor(ctx, Actor{ID: user, Name: "Sam Agent"})
	p, err := r.CreateProject(ctx, org, user, "SUP", "Support", "", "team")
	if err != nil {
		t.Fatal(err)
	}
	mk := func(title string) Ticket {
		tk, err := r.CreateTicket(ctx, org, p.ID, title, "body of "+title, "normal", &user, "Ana Ray", "ana@example.com", "web")
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}
	a, b, c := mk("Cannot log in"), mk("Login broken"), mk("Outage")

	// tags and type, problem link
	tags := []string{"Login", "vip"}
	ty := "incident"
	if _, err := r.SetMeta(ctx, org, a.ID, Meta{Tags: &tags, Type: &ty}); err != nil {
		t.Fatal(err)
	}
	prob := "problem"
	if _, err := r.SetMeta(ctx, org, c.ID, Meta{Type: &prob}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetMeta(ctx, org, a.ID, Meta{ProblemID: &b.ID}); err == nil {
		t.Fatal("linking to a non-problem must fail")
	}
	if _, err := r.SetMeta(ctx, org, a.ID, Meta{ProblemID: &c.ID}); err != nil {
		t.Fatal(err)
	}
	inc, _ := r.Incidents(ctx, org, c.ID)
	if len(inc) != 1 || inc[0].ID != a.ID {
		t.Fatalf("incidents %+v", inc)
	}

	// bulk: assign + status + tag; foreign ids are skipped
	resolved := "resolved"
	n, err := r.Bulk(ctx, org, BulkChange{IDs: []string{a.ID, b.ID, "00000000-0000-0000-0000-000000000000"}, Status: resolved, Assignee: user, AddTags: []string{"bulk"}})
	if err != nil || n != 2 {
		t.Fatalf("bulk %d %v", n, err)
	}
	if _, err := r.Bulk(ctx, org, BulkChange{IDs: []string{a.ID}, Status: "nonsense"}); err == nil {
		t.Fatal("unknown status must fail")
	}
	da, _ := r.GetDesk(ctx, org, a.ID)
	if da.Status != "resolved" || da.Assignee != user || !reflect.DeepEqual(da.Tags, []string{"login", "vip", "bulk"}) || da.ResolvedAt == nil {
		t.Fatalf("after bulk %+v", da)
	}

	// merge b into a
	if _, err := r.AddComment(ctx, org, b.ID, &user, "Sam", "second report", false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Merge(ctx, org, b.ID, b.ID); err == nil {
		t.Fatal("self merge must fail")
	}
	merged, err := r.Merge(ctx, org, b.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.ID != a.ID {
		t.Fatal("merge returns the target")
	}
	db, _ := r.GetDesk(ctx, org, b.ID)
	if db.MergedInto != a.ID || db.Status != "closed" {
		t.Fatalf("source %+v", db)
	}
	if _, err := r.Merge(ctx, org, b.ID, c.ID); err == nil {
		t.Fatal("double merge must fail")
	}
	_, comments, _ := r.GetTicket(ctx, org, a.ID)
	found := false
	for _, cm := range comments {
		found = found || strings.Contains(cm.Body, "[from SUP-2] second report")
	}
	if !found {
		t.Fatalf("comments not moved: %+v", comments)
	}

	// macro: reply with placeholders, change priority, assignee "me"
	m, err := r.SaveMacro(ctx, org, user, Macro{Name: "Escalate", Actions: MacroActions{Priority: "urgent", Assignee: "me", Reply: "Hi {{requester.first_name}}, on it ({{ticket.ref}}).", AddTags: []string{"escalated"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ApplyMacro(ctx, org, m.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	dc, _ := r.GetDesk(ctx, org, c.ID)
	if dc.Priority != "urgent" || dc.Assignee != user || !reflect.DeepEqual(dc.Tags, []string{"escalated"}) {
		t.Fatalf("macro result %+v", dc)
	}
	_, cc, _ := r.GetTicket(ctx, org, c.ID)
	if len(cc) != 1 || cc[0].Body != "Hi Ana, on it (SUP-3)." {
		t.Fatalf("macro reply %+v", cc)
	}
	// another org cannot see it
	if _, err := r.GetDesk(ctx, "00000000-0000-0000-0000-000000000000", c.ID); err == nil {
		t.Fatal("org scoping")
	}
	if err := r.DeleteMacro(ctx, org, m.ID); err != nil {
		t.Fatal(err)
	}

	// events trail
	ev, _ := r.Events(ctx, org, c.ID)
	kinds := []string{}
	for _, e := range ev {
		kinds = append(kinds, e.Kind)
	}
	if !contains(kinds, "priority") || !contains(kinds, "assignee") || !contains(kinds, "macro") || !contains(kinds, "type") {
		t.Fatalf("events %v", kinds)
	}
	on, _ := r.ToggleFollow(ctx, org, c.ID, user)
	off, _ := r.ToggleFollow(ctx, org, c.ID, user)
	if !on || off {
		t.Fatal("follow toggle")
	}
	all, _ := r.ListDesk(ctx, org, p.ID)
	if len(all) != 3 {
		t.Fatalf("list %d", len(all))
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
