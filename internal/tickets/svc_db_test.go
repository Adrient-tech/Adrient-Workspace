package tickets

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

type deskEnv struct {
	r        *Repository
	ctx      context.Context
	org      string
	otherOrg string
	users    []string
	proj     Project
}

// newDeskEnv migrates (never drops) the scratch database named by ADRIENT_TEST_DSN and
// creates an isolated org with three agents and a project.
func newDeskEnv(t *testing.T) *deskEnv {
	t.Helper()
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
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	sfx := fmt.Sprintf("%d", time.Now().UnixNano())
	mkOrg := func(name string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ($1,$2) RETURNING id::text`, "w9-"+name+sfx, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	e := &deskEnv{r: NewRepository(pool), org: mkOrg("a"), otherOrg: mkOrg("b")}
	for i, n := range []string{"Asha", "Bo", "Chen"} {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
			VALUES ($1,'test',$2,$3,$4) RETURNING id::text`, e.org, fmt.Sprintf("s%d-%s", i, sfx), fmt.Sprintf("u%d-%s@x.test", i, sfx), n).Scan(&id); err != nil {
			t.Fatal(err)
		}
		e.users = append(e.users, id)
	}
	e.ctx = WithActor(ctx, Actor{ID: e.users[0], Name: "Asha"})
	e.proj, err = e.r.CreateProject(e.ctx, e.org, e.users[0], "S"+sfx[len(sfx)-5:], "Support", "", "team")
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *deskEnv) ticket(t *testing.T, title string, opts CreateOpts) Ticket {
	t.Helper()
	tk, err := e.r.CreateTicket(WithCreateOpts(e.ctx, opts), e.org, e.proj.ID, title, "body "+title, "normal", nil, "Ravi", "ravi@acme.example", "web")
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestDeskRoutingRoundRobinAndOOO(t *testing.T) {
	e := newDeskEnv(t)
	g, err := e.r.SaveGroup(e.ctx, e.org, e.proj.ID, Group{Name: "Network", Strategy: StrategyRoundRobin, Members: []Member{
		{UserID: e.users[0], Capacity: 5}, {UserID: e.users[1], Capacity: 5}, {UserID: e.users[2], Capacity: 5}}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for i := 0; i < 4; i++ {
		tk := e.ticket(t, fmt.Sprintf("net %d", i), CreateOpts{GroupID: g.ID})
		if tk.AssigneeUserID == nil {
			t.Fatalf("ticket %d not routed", i)
		}
		got = append(got, *tk.AssigneeUserID)
	}
	want := []string{e.users[0], e.users[1], e.users[2], e.users[0]}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("round robin order: got %v want %v", got, want)
		}
	}
	// Bo goes out of office: skipped
	until := time.Now().Add(48 * time.Hour)
	if err := e.r.SetOOO(e.ctx, e.org, e.users[1], &until); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		tk := e.ticket(t, fmt.Sprintf("after ooo %d", i), CreateOpts{GroupID: g.ID})
		if tk.AssigneeUserID != nil && *tk.AssigneeUserID == e.users[1] {
			t.Fatal("out-of-office agent received a ticket")
		}
	}
	// a member outside the org is refused
	if _, err := e.r.SaveGroup(e.ctx, e.org, e.proj.ID, Group{Name: "Bad", Members: []Member{{UserID: "00000000-0000-0000-0000-000000000001"}}}); err == nil {
		t.Fatal("foreign member accepted")
	}
	// another org cannot see or delete this group
	if gs, _ := e.r.ListGroups(e.ctx, e.otherOrg, e.proj.ID); len(gs) != 0 {
		t.Fatal("group leaked across orgs")
	}
	if err := e.r.DeleteGroup(e.ctx, e.otherOrg, g.ID); err == nil {
		t.Fatal("deleted a group of another org")
	}
}

func TestDeskWorkflowPauseReopenRequired(t *testing.T) {
	e := newDeskEnv(t)
	if _, err := e.r.SaveStatusDefs(e.ctx, e.org, e.proj.ID, []StatusDef{
		{Key: "open", Category: "open"}, {Key: "pending", Category: "pending", PausesSLA: true},
		{Key: "resolved", Category: "solved", Requires: []string{"category"}}, {Key: "closed", Category: "closed"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.r.SaveSLARule(e.ctx, e.org, e.proj.ID, SLARule{Name: "All", Enabled: true, FirstResponseMin: 60, ResolveMin: 480}); err != nil {
		t.Fatal(err)
	}
	tk := e.ticket(t, "pause me", CreateOpts{})
	pending, resolved, open := "pending", "resolved", "open"
	if _, err := e.r.UpdateTicket(e.ctx, e.org, tk.ID, &pending, nil, nil, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	info, err := e.r.SLAFor(e.ctx, e.org, tk.ID, time.Now().Add(10*time.Hour))
	if err != nil || !info.Resolution.Paused {
		t.Fatalf("clock should be paused: %+v %v", info, err)
	}
	if info.Resolution.State == SLABreached {
		t.Fatal("a paused ticket must not breach while waiting")
	}
	// resolving without a category is refused
	if _, err := e.r.UpdateTicket(e.ctx, e.org, tk.ID, &resolved, nil, nil, nil, nil, false); err == nil {
		t.Fatal("required field not enforced")
	}
	cat := "Network"
	if _, err := e.r.SetFields(e.ctx, e.org, tk.ID, FieldsPatch{Category: &cat}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.r.UpdateTicket(e.ctx, e.org, tk.ID, &resolved, nil, nil, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	// requester reply reopens a solved ticket
	if _, err := e.r.AddComment(e.ctx, e.org, tk.ID, nil, "Ravi", "still broken", false); err != nil {
		t.Fatal(err)
	}
	d, _ := e.r.GetDesk(e.ctx, e.org, tk.ID)
	if d.Status != open {
		t.Fatalf("status after requester reply: %s", d.Status)
	}
	// an agent reply sets first response
	agent := e.users[0]
	if _, err := e.r.AddComment(e.ctx, e.org, tk.ID, &agent, "Asha", "looking", false); err != nil {
		t.Fatal(err)
	}
	d, _ = e.r.GetDesk(e.ctx, e.org, tk.ID)
	if d.FirstResponse == nil {
		t.Fatal("first response not recorded")
	}
	// removing a status that has tickets is refused
	if _, err := e.r.SaveStatusDefs(e.ctx, e.org, e.proj.ID, []StatusDef{{Key: "done", Category: "solved"}, {Key: "new", Category: "open"}}); err == nil {
		t.Fatal("status with tickets removed")
	}
}

func TestDeskAutomationsAndSweep(t *testing.T) {
	e := newDeskEnv(t)
	g, _ := e.r.SaveGroup(e.ctx, e.org, e.proj.ID, Group{Name: "Net", Strategy: StrategyLoad, Members: []Member{{UserID: e.users[2], Capacity: 3}}})
	if _, err := e.r.SaveAutomation(e.ctx, e.org, e.users[0], e.proj.ID, Automation{Name: "VPN", Kind: KindDispatch, Enabled: true,
		Conds:   []Condition{{"title", "contains", "vpn"}},
		Actions: []Action{{"set_priority", "high"}, {"set_group", g.ID}, {"add_tag", "network"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.r.SaveAutomation(e.ctx, e.org, e.users[0], e.proj.ID, Automation{Name: "bad", Kind: KindTrigger, Actions: []Action{{"set_priority", "huge"}}}); err == nil {
		t.Fatal("invalid rule saved")
	}
	tk := e.ticket(t, "VPN is down", CreateOpts{})
	d, _ := e.r.GetDesk(e.ctx, e.org, tk.ID)
	if d.Priority != "high" || d.GroupID != g.ID || len(d.Tags) != 1 || d.Assignee != e.users[2] {
		t.Fatalf("automation + routing result: %+v", d)
	}
	other := e.ticket(t, "Printer jam", CreateOpts{})
	if od, _ := e.r.GetDesk(e.ctx, e.org, other.ID); od.Priority != "normal" || od.GroupID != "" {
		t.Fatalf("rule fired on a non-matching ticket: %+v", od)
	}
	// dry run changes nothing
	res, err := e.r.TestAutomation(e.ctx, e.org, other.ID, Automation{Name: "t", Kind: KindTrigger, Event: "create", MatchMode: "all",
		Conds: []Condition{{"title", "contains", "printer"}}, Actions: []Action{{"set_priority", "urgent"}}}, "create")
	if err != nil || !res.Matched || len(res.Changes) != 1 {
		t.Fatalf("dry run: %+v %v", res, err)
	}
	if od, _ := e.r.GetDesk(e.ctx, e.org, other.ID); od.Priority != "normal" {
		t.Fatal("dry run modified the ticket")
	}
	// time rule: stale open tickets get a tag, once
	if _, err := e.r.SaveAutomation(e.ctx, e.org, e.users[0], e.proj.ID, Automation{Name: "stale", Kind: KindTime, Enabled: true,
		Conds:   []Condition{{"hours_since_update", "gt", "24"}, {"status", "is", "open"}},
		Actions: []Action{{"add_tag", "stale"}}}); err != nil {
		t.Fatal(err)
	}
	_, _ = e.r.pool.Exec(e.ctx, `UPDATE adrient.tickets SET updated_at = now() - interval '3 days' WHERE id=$1`, other.ID)
	sr, err := e.r.Sweep(e.ctx, time.Now())
	if err != nil || sr.RuleHits < 1 {
		t.Fatalf("sweep: %+v %v", sr, err)
	}
	od, _ := e.r.GetDesk(e.ctx, e.org, other.ID)
	if len(od.Tags) != 1 || od.Tags[0] != "stale" {
		t.Fatalf("time rule result: %+v", od.Tags)
	}
	sr2, _ := e.r.Sweep(e.ctx, time.Now())
	if sr2.RuleHits != 0 {
		t.Fatal("time rule ran twice on the same ticket")
	}
	// snooze wakes up
	soon := time.Now().Add(1100 * time.Millisecond)
	if err := e.r.Snooze(e.ctx, e.org, other.ID, &soon); err != nil {
		t.Fatal(err)
	}
	if err := e.r.Snooze(e.ctx, e.org, other.ID, ptrTime(time.Now().Add(-time.Hour))); err == nil {
		t.Fatal("snooze in the past accepted")
	}
	time.Sleep(1300 * time.Millisecond)
	if sr3, _ := e.r.Sweep(e.ctx, time.Now()); sr3.Woken != 1 {
		t.Fatalf("woken %d", sr3.Woken)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestDeskListPagingAndSearch(t *testing.T) {
	e := newDeskEnv(t)
	for i := 0; i < 130; i++ {
		title := fmt.Sprintf("Ticket number %d", i)
		if i%10 == 0 {
			title = fmt.Sprintf("Invoice question %d", i)
		}
		e.ticket(t, title, CreateOpts{})
	}
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		items, next, err := e.r.ListPage(e.ctx, e.org, ListOpts{ProjectID: e.proj.ID, View: "all_open", Limit: 50, Cursor: cursor, Me: e.users[0]})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, it := range items {
			if seen[it.ID] {
				t.Fatal("duplicate across pages")
			}
			seen[it.ID] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != 130 || pages != 3 {
		t.Fatalf("paging: %d tickets over %d pages", len(seen), pages)
	}
	items, _, err := e.r.ListPage(e.ctx, e.org, ListOpts{ProjectID: e.proj.ID, View: "all", Q: "invoice priority:normal", Limit: 100})
	if err != nil || len(items) != 13 {
		t.Fatalf("search: %d %v", len(items), err)
	}
	// wildcard characters in the search are literal
	if items, _, _ := e.r.ListPage(e.ctx, e.org, ListOpts{ProjectID: e.proj.ID, View: "all", Q: "100%", Limit: 10}); len(items) != 0 {
		t.Fatal("percent sign matched as wildcard")
	}
	// another org sees nothing of this project
	if items, _, _ := e.r.ListPage(e.ctx, e.otherOrg, ListOpts{ProjectID: e.proj.ID, View: "all", Limit: 10}); len(items) != 0 {
		t.Fatal("list leaked across orgs")
	}
	if _, _, err := e.r.ListPage(e.ctx, e.org, ListOpts{ProjectID: e.proj.ID, View: "all", Cursor: "garbage"}); err == nil {
		t.Fatal("bad cursor accepted")
	}
	views, err := e.r.ViewCountsSQL(e.ctx, e.org, e.proj.ID, e.users[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.ID == "all_open" && v.Count != 130 {
			t.Fatalf("all_open count %d", v.Count)
		}
	}
	rep, err := e.r.ReportV2(e.ctx, e.org, e.proj.ID, ReportOpts{Days: 7}, time.Now())
	if err != nil || rep.Created != 130 {
		t.Fatalf("report: %+v %v", rep.Created, err)
	}
	csvb, err := e.r.ExportCSV(e.ctx, e.org, e.proj.ID, ReportOpts{Days: 7}, time.Now())
	if err != nil || len(csvb) < 1000 {
		t.Fatalf("csv %d %v", len(csvb), err)
	}
}

func TestDeskApprovalsAndForms(t *testing.T) {
	e := newDeskEnv(t)
	tk := e.ticket(t, "Change firewall rule", CreateOpts{Type: "change"})
	if err := e.r.RequestApprovals(e.ctx, e.org, tk.ID, [][]string{{e.users[1]}, {e.users[2]}}); err != nil {
		t.Fatal(err)
	}
	// step 2 approver cannot decide before step 1
	if err := e.r.Decide(e.ctx, e.org, tk.ID, e.users[2], "approved", ""); err == nil {
		t.Fatal("step 2 approver decided out of order")
	}
	if err := e.r.Decide(e.ctx, e.org, tk.ID, e.users[1], "approved", "ok"); err != nil {
		t.Fatal(err)
	}
	if mine, _ := e.r.MyApprovals(e.ctx, e.org, e.users[2]); len(mine) != 1 {
		t.Fatal("step 2 approver should now see it")
	}
	if err := e.r.Decide(e.ctx, e.org, tk.ID, e.users[2], "rejected", "no"); err != nil {
		t.Fatal(err)
	}
	if _, st, _ := e.r.Approvals(e.ctx, e.org, tk.ID); st != "rejected" {
		t.Fatalf("state %s", st)
	}
	// request forms through the public token
	p := e.proj
	token := newToken()
	if _, err := e.r.pool.Exec(e.ctx, `UPDATE adrient.ticket_projects SET intake_mode='public', public_token=$2 WHERE id=$1`, p.ID, token); err != nil {
		t.Fatal(err)
	}
	f, err := e.r.SaveForm(e.ctx, e.org, p.ID, RequestForm{Name: "Laptop request", TicketType: "request", Enabled: true,
		Fields: []FormField{{Key: "model", Label: "Model", Type: "select", Required: true, Options: []string{"Air", "Pro"}}, {Key: "why", Label: "Reason", Type: "longtext"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.r.SubmitForm(e.ctx, token, f.ID, "Ravi", "ravi@acme.example", "", map[string]string{"model": "Zeta"}); err == nil {
		t.Fatal("invalid option accepted")
	}
	if _, _, err := e.r.SubmitForm(e.ctx, token, f.ID, "Ravi", "ravi@acme.example", "", map[string]string{}); err == nil {
		t.Fatal("missing required field accepted")
	}
	pt, ref, err := e.r.SubmitForm(e.ctx, token, f.ID, "Ravi", "ravi@acme.example", "", map[string]string{"model": "Pro", "why": "old one died"})
	if err != nil || ref == "" {
		t.Fatalf("submit: %v", err)
	}
	d, _ := e.r.GetDesk(e.ctx, e.org, pt.ID)
	if d.Type != "request" || d.Custom["model"] != "Pro" || d.FormID != f.ID {
		t.Fatalf("ticket from form: %+v", d)
	}
	// requester tracking: reply and close through the portal token
	tr := e.r.PortalToken(e.ctx, pt.ID)
	if tr == "" {
		t.Fatal("no portal token")
	}
	if err := e.r.TrackReply(e.ctx, tr, "any update?"); err != nil {
		t.Fatal(err)
	}
	v, err := e.r.TrackByToken(e.ctx, tr)
	if err != nil || len(v.Comments) != 1 {
		t.Fatalf("track: %+v %v", v, err)
	}
	if err := e.r.TrackClose(e.ctx, tr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.r.TrackByToken(e.ctx, "tk_does-not-exist"); err == nil {
		t.Fatal("unknown token accepted")
	}
	// parent/child rules
	a, b := e.ticket(t, "parent", CreateOpts{}), e.ticket(t, "child", CreateOpts{})
	if err := e.r.SetParent(e.ctx, e.org, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.r.SetParent(e.ctx, e.org, a.ID, b.ID); err == nil {
		t.Fatal("a parent became a child")
	}
	if kids, _ := e.r.Children(e.ctx, e.org, a.ID); len(kids) != 1 {
		t.Fatal("children")
	}
	// time entries
	if _, err := e.r.AddTime(e.ctx, e.org, e.users[0], "Asha", a.ID, TimeEntry{Minutes: 0}); err == nil {
		t.Fatal("zero minutes accepted")
	}
	if _, err := e.r.AddTime(e.ctx, e.org, e.users[0], "Asha", a.ID, TimeEntry{Minutes: 25, Billable: true}); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := e.r.ListTime(e.ctx, e.org, a.ID); total != 25 {
		t.Fatalf("time total %d", total)
	}
}

func TestDeskPresenceAndDrafts(t *testing.T) {
	e := newDeskEnv(t)
	tk := e.ticket(t, "collide", CreateOpts{})
	if _, err := e.r.Heartbeat(e.ctx, e.org, tk.ID, e.users[1], "Bo", "replying"); err != nil {
		t.Fatal(err)
	}
	others, err := e.r.Heartbeat(e.ctx, e.org, tk.ID, e.users[0], "Asha", "viewing")
	if err != nil || len(others) != 1 || others[0].Name != "Bo" || others[0].Mode != "replying" {
		t.Fatalf("presence: %+v %v", others, err)
	}
	if _, err := e.r.Heartbeat(e.ctx, e.otherOrg, tk.ID, e.users[0], "Asha", "viewing"); err == nil {
		t.Fatal("presence across orgs")
	}
	if err := e.r.SaveDraft(e.ctx, e.org, tk.ID, e.users[0], Draft{Body: "half a reply", Internal: true}); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.r.GetDraft(e.ctx, e.org, tk.ID, e.users[0]); d.Body != "half a reply" || !d.Internal {
		t.Fatalf("draft %+v", d)
	}
	if d, _ := e.r.GetDraft(e.ctx, e.org, tk.ID, e.users[1]); d.Body != "" {
		t.Fatal("draft visible to someone else")
	}
	a := e.users[0]
	if _, err := e.r.AddComment(e.ctx, e.org, tk.ID, &a, "Asha", "sent", false); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.r.GetDraft(e.ctx, e.org, tk.ID, e.users[0]); d.Body != "" {
		t.Fatal("draft not cleared after sending")
	}
}
