package crm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---- pure unit tests (no database) ----

func TestValidateCustom(t *testing.T) {
	defs := []FieldDef{
		{Key: "budget", Label: "Budget", Type: "number", Required: true},
		{Key: "tier", Label: "Tier", Type: "select", Options: []string{"gold", "silver"}},
		{Key: "interests", Label: "Interests", Type: "multiselect", Options: []string{"a", "b", "c"}},
		{Key: "renewal", Label: "Renewal", Type: "date"},
		{Key: "site", Label: "Site", Type: "url"},
		{Key: "rep", Label: "Rep", Type: "user"},
	}
	ok := func(string) bool { return true }
	out, err := ValidateCustom(defs, map[string]any{"budget": "1,200.50", "tier": "gold", "interests": []any{"b", "a", "b"}, "renewal": "2026-05-01", "site": "https://x.io/a"}, true, ok)
	if err != nil {
		t.Fatal(err)
	}
	if out["budget"] != 1200.5 || fmt.Sprint(out["interests"]) != "[a b]" {
		t.Fatalf("normalised wrong: %v", out)
	}
	bad := []map[string]any{
		{"budget": "abc"}, {"budget": 1, "tier": "bronze"}, {"budget": 1, "renewal": "01/05/2026"},
		{"budget": 1, "site": "javascript:alert(1)"}, {"budget": 1, "nope": "x"}, {"budget": 1, "interests": []any{"z"}},
		{"budget": 1, "rep": "not-a-uuid"},
	}
	for i, b := range bad {
		if _, err := ValidateCustom(defs, b, false, ok); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d should be invalid: %v", i, err)
		}
	}
	if _, err := ValidateCustom(defs, map[string]any{"tier": "gold"}, true, ok); err == nil {
		t.Error("missing required field must fail")
	}
	if _, err := ValidateCustom(defs, map[string]any{"budget": 1, "rep": "11111111-1111-1111-1111-111111111111"}, true, func(string) bool { return false }); err == nil {
		t.Error("unknown user must fail")
	}
	if o, _ := ValidateCustom(defs, map[string]any{"budget": 1, "tier": ""}, true, ok); o["tier"] != nil {
		t.Error("empty value should be dropped")
	}
}

func TestVisibilityPredicates(t *testing.T) {
	const me = "11111111-1111-1111-1111-111111111111"
	if (Viewer{}).ownerSQL("d") != "TRUE" || (Viewer{UserID: me, Scope: ScopeAll}).ownerSQL("d") != "TRUE" {
		t.Error("unrestricted viewers see everything")
	}
	if got := (Viewer{UserID: "x'; DROP", Scope: ScopeOwn}).ownerSQL("d"); got != "FALSE" {
		t.Errorf("a non-uuid user id must fail closed, got %s", got)
	}
	own := (Viewer{UserID: me, Scope: ScopeOwn}).ownerSQL("d")
	team := (Viewer{UserID: me, Scope: ScopeTeam}).ownerSQL("d")
	if !strings.Contains(own, "d.owner_id='"+me+"'") || strings.Contains(own, "crm_team_members") {
		t.Errorf("own predicate wrong: %s", own)
	}
	if !strings.Contains(team, "crm_team_members") {
		t.Errorf("team predicate must use team membership: %s", team)
	}
	mates := map[string]bool{"b": true}
	v := Viewer{UserID: "a", Scope: ScopeOwn}
	if !v.CanSee("a", "", mates) || v.CanSee("b", "", mates) || !v.CanSee("", "a", mates) || v.CanSee("", "c", mates) {
		t.Error("own scope rule")
	}
	v.Scope = ScopeTeam
	if !v.CanSee("b", "", mates) || v.CanSee("z", "", mates) {
		t.Error("team scope rule")
	}
}

func TestRuleMatchingAndRender(t *testing.T) {
	r := Rule{Enabled: true, Trigger: TrigStageChanged, Conditions: map[string]any{"stage": "proposal"}}
	if !RuleMatches(r, Event{Trigger: TrigStageChanged, Stage: "proposal"}) || RuleMatches(r, Event{Trigger: TrigStageChanged, Stage: "new"}) ||
		RuleMatches(r, Event{Trigger: TrigLeadCreated, Stage: "proposal"}) {
		t.Error("stage rule matching")
	}
	r.Enabled = false
	if RuleMatches(r, Event{Trigger: TrigStageChanged, Stage: "proposal"}) {
		t.Error("disabled rule must not match")
	}
	m := Rule{Enabled: true, Trigger: TrigEmailReceived, Conditions: map[string]any{"from_domain": "@Acme.com"}}
	if !RuleMatches(m, Event{Trigger: TrigEmailReceived, FromAddr: "bob@acme.com"}) || RuleMatches(m, Event{Trigger: TrigEmailReceived, FromAddr: "bob@evil.com"}) {
		t.Error("domain rule matching")
	}
	if got := RenderTemplate("Hi {{ person.name }}, re {{deal.title}} {{missing}}!", map[string]string{"person.name": "Asha", "deal.title": "Big"}); got != "Hi Asha, re Big !" {
		t.Errorf("render: %q", got)
	}
	bad := Rule{Name: "x", Trigger: TrigDealIdle, Actions: []Action{{Type: "notify_chat", Text: "hi"}}}
	if bad.Validate() == nil {
		t.Error("idle rule without idle_days must be rejected")
	}
	if PickRoundRobin([]string{"a", "b", "c"}, 4) != "b" || PickRoundRobin(nil, 1) != "" {
		t.Error("round robin pick")
	}
}

func TestScoring(t *testing.T) {
	rules := []ScoreRule{
		{Field: "source", Op: "eq", Value: "Referral", Points: 20},
		{Field: "tag", Op: "eq", Value: "vip", Points: 10},
		{Field: "has_phone", Op: "exists", Points: 5},
		{Field: "activities_30d", Op: "gt", Value: "2", Points: 15},
		{Field: "custom:budget", Op: "gt", Value: "1000", Points: 25},
		{Field: "status", Op: "eq", Value: "inactive", Points: -50},
	}
	a := ScoreAttrs{"source": "referral", "tag": []string{"vip", "x"}, "has_phone": true, "activities_30d": 3, "custom:budget": 5000.0, "status": "lead"}
	if got := EvalScore(a, rules); got != 75 {
		t.Errorf("score %d, want 75", got)
	}
	a2 := ScoreAttrs{"source": "ads", "tag": []string{}, "has_phone": false, "activities_30d": 1, "status": "inactive"}
	if got := EvalScore(a2, rules); got != -50 {
		t.Errorf("score %d, want -50", got)
	}
}

func TestDuplicateKeysAndGrouping(t *testing.T) {
	if NormPhone("+91 98765-43210") != NormPhone("098765 43210") || NormPhone("12") != "" {
		t.Error("phone normalisation")
	}
	if NormCompany("Acme Pvt. Ltd.") != NormCompany("ACME") || NormCompany("The Acme Co") != "acme" {
		t.Error("company normalisation")
	}
	if WebsiteDomain("https://www.Acme.com/about?x=1") != "acme.com" {
		t.Error("domain")
	}
	g := GroupDuplicates("person", []dupItem{
		{id: "1", name: "A", keys: []string{"email:a@x.com"}},
		{id: "2", name: "B", keys: []string{"email:a@x.com", "phone:111"}},
		{id: "3", name: "C", keys: []string{"phone:111"}},
		{id: "4", name: "D", keys: []string{"email:d@x.com"}},
	})
	if len(g) != 1 || len(g[0].IDs) != 3 {
		t.Fatalf("expected one group of three (transitive), got %+v", g)
	}
}

func TestImportMappingAndParsing(t *testing.T) {
	m := SuggestMapping("hubspot", "lead", []string{"Record ID", "First Name", "Last Name", "Email", "Phone Number", "Company Name", "Lead Status", "Odd Column"})
	want := map[string]string{"Record ID": "external_id", "First Name": "first_name", "Last Name": "last_name", "Email": "email", "Phone Number": "phone", "Company Name": "company", "Lead Status": "status"}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("hubspot %q -> %q, want %q", k, m[k], v)
		}
	}
	if _, ok := m["Odd Column"]; ok {
		t.Error("unknown columns must stay unmapped")
	}
	if z := SuggestMapping("zoho", "deal", []string{"Deal Name", "Stage", "Amount", "Closing Date", "Account Name"}); z["Deal Name"] != "title" || z["Amount"] != "value" || z["Account Name"] != "company" {
		t.Errorf("zoho deal mapping %v", z)
	}
	if s := SuggestMapping("salesforce", "company", []string{"Id", "Name", "BillingStreet"}); s["Id"] != "external_id" || s["BillingStreet"] != "address" {
		t.Errorf("salesforce company mapping %v", s)
	}
	stages := []Stage{{ID: "new", Name: "New"}, {ID: "proposal", Name: "Proposal Sent"}}
	for in, want := range map[string]string{"Closed Won": "won", "closed lost": "lost", "Proposal Sent": "proposal", "Qualification": "new", "": "new"} {
		if got := MapStage(in, stages); got != want {
			t.Errorf("MapStage(%q)=%q want %q", in, got, want)
		}
	}
	if MapStatus("Converted") != "customer" || MapStatus("Junk Lead") != "inactive" || MapStatus("Open - Not Contacted") != "lead" {
		t.Error("MapStatus")
	}
	if d, ok := ParseDate("31/12/2025"); !ok || d != "2025-12-31" {
		t.Errorf("day-first date: %q", d)
	}
	if d, _ := ParseDate("03/04/2025"); d != "2025-04-03" {
		t.Errorf("ambiguous slash dates are read day-first: %q", d)
	}
	if v, err := ParseMoney("Rs. 1,25,000.50"); err != nil || v != 125000.5 {
		t.Errorf("money %v %v", v, err)
	}
	h, rows, err := ParseCSV([]byte("\xef\xbb\xbfName,Email\r\nAsha,a@x.com\r\n\r\n\"Li, Wei\",w@x.com\r\n"), 10)
	if err != nil || h[0] != "Name" || len(rows) != 2 || rows[1][0] != "Li, Wei" {
		t.Fatalf("csv %v %v %v", h, rows, err)
	}
	if _, _, err := ParseCSV([]byte("a\n1\n2\n3\n"), 2); err == nil {
		t.Error("row limit")
	}
}

func TestCSVFormulaGuardAndSerialisation(t *testing.T) {
	rp := Report{Columns: []string{"a", "b"}, Rows: [][]any{{"=SUM(A1)", "ok, with comma"}, {"@cmd", 3.5}}}
	out := string(rp.CSV())
	if !strings.Contains(out, "'=SUM(A1)") || !strings.Contains(out, "'@cmd") || !strings.Contains(out, `"ok, with comma"`) || !strings.Contains(out, "3.5") {
		t.Errorf("csv output: %s", out)
	}
}

func TestPriceMathAndTerritory(t *testing.T) {
	if got := (DealItem{Qty: 3, UnitPrice: 33.33, DiscountPct: 10}).LineTotal(); got != 89.99 {
		t.Errorf("line total %v", got)
	}
	ts := []Territory{{ID: "t1", Industries: []string{"Retail"}}, {ID: "t2", Keywords: []string{"labs"}}}
	if MatchTerritory(ts, "retail", "X") != "t1" || MatchTerritory(ts, "", "Zeta Labs Ltd") != "t2" || MatchTerritory(ts, "Steel", "Zeta") != "" {
		t.Error("territory matching")
	}
	w := StageWeights([]Stage{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	if w["a"] != 0.25 || w["c"] != 0.75 {
		t.Errorf("weights %v", w)
	}
}

// ---- integration tests (need ADRIENT_TEST_DSN, a throwaway database) ----

var (
	migrateOnce sync.Once
	migrateErr  error
)

func testRepo(t *testing.T) (*Repository, *pgxpool.Pool) {
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
	migrateOnce.Do(func() { migrateErr = storage.RunMigrations(ctx, pool) })
	if migrateErr != nil {
		t.Fatal(migrateErr)
	}
	return NewRepository(pool), pool
}

type fixture struct {
	org   string
	users []string
}

// newOrg creates an isolated org with n users, so tests never see each other's data.
func newOrg(t *testing.T, pool *pgxpool.Pool, n int) fixture {
	t.Helper()
	ctx := context.Background()
	var f fixture
	slug := fmt.Sprintf("t%d", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ($1,$1) RETURNING id::text`, slug).Scan(&f.org); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		var u string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'test',$2,$3,$4) RETURNING id::text`,
			f.org, fmt.Sprintf("%s-%d", slug, i), fmt.Sprintf("u%d@%s.test", i, slug), fmt.Sprintf("User %d", i)).Scan(&u); err != nil {
			t.Fatal(err)
		}
		f.users = append(f.users, u)
	}
	return f
}

func newContact(t *testing.T, pool *pgxpool.Pool, f fixture, name, email string) string {
	t.Helper()
	var id string
	emails, _ := json.Marshal([]string{email})
	if err := pool.QueryRow(context.Background(), `INSERT INTO adrient.contacts (org_id, owner_id, display_name, emails) VALUES ($1,$2,$3,$4) RETURNING id::text`,
		f.org, f.users[0], name, emails).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func idOf(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &v); err != nil || v.ID == "" {
		t.Fatalf("no id in %s (%v)", raw, err)
	}
	return v.ID
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestVisibilityEnforcedOnServer(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 3)
	alice, bob, carol := f.users[0], f.users[1], f.users[2]
	ctx := context.Background()
	as := func(u, scope string) context.Context { return WithViewer(ctx, Viewer{UserID: u, Scope: scope}) }

	dAlice, err := r.SaveDeal(as(alice, ScopeOwn), f.org, "", Deal{Title: "Alice deal", Value: 100})
	if err != nil {
		t.Fatal(err)
	}
	dBob, err := r.SaveDeal(as(bob, ScopeOwn), f.org, "", Deal{Title: "Bob deal", Value: 200})
	if err != nil {
		t.Fatal(err)
	}
	cAlice, _ := r.SaveCompany(as(alice, ScopeOwn), f.org, "", Company{Name: "Alice Co"})
	idA, idB := idOf(t, dAlice), idOf(t, dBob)

	titles := func(c context.Context) string {
		raw, err := r.ListDeals(c, f.org)
		if err != nil {
			t.Fatal(err)
		}
		var l []struct{ Title string }
		_ = json.Unmarshal(raw, &l)
		var s []string
		for _, d := range l {
			s = append(s, d.Title)
		}
		return strings.Join(s, ",")
	}
	if got := titles(as(alice, ScopeOwn)); got != "Alice deal" {
		t.Errorf("alice(own) sees %q", got)
	}
	if got := titles(as(bob, ScopeOwn)); got != "Bob deal" {
		t.Errorf("bob(own) sees %q", got)
	}
	if got := titles(as(carol, ScopeOwn)); got != "" {
		t.Errorf("carol(own) sees %q", got)
	}
	if got := titles(ctx); !strings.Contains(got, "Alice deal") || !strings.Contains(got, "Bob deal") {
		t.Errorf("unrestricted must see all, got %q", got)
	}
	if _, err := r.GetDeal(as(bob, ScopeOwn), f.org, idA); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob reading alice's deal: %v", err)
	}
	if _, err := r.MoveDeal(as(bob, ScopeOwn), f.org, idA, "qualified", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob moving alice's deal: %v", err)
	}
	if _, err := r.SaveDeal(as(bob, ScopeOwn), f.org, idA, Deal{Title: "hijack"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob editing alice's deal: %v", err)
	}
	if err := r.DeleteDeal(as(bob, ScopeOwn), f.org, idA); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob deleting alice's deal: %v", err)
	}
	if err := r.DeleteCompany(as(bob, ScopeOwn), f.org, idOf(t, cAlice)); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob deleting alice's company: %v", err)
	}
	// Search and reports honour the same rule.
	hits, _ := r.Search(as(bob, ScopeOwn), f.org, "deal", 10)
	if len(hits) != 1 || hits[0].Title != "Bob deal" {
		t.Errorf("bob's search saw %+v", hits)
	}
	rep, err := r.RunReport(as(bob, ScopeOwn), f.org, "aging", nil)
	if err != nil || len(rep.Rows) != 1 {
		t.Errorf("bob's aging report rows=%d err=%v", len(rep.Rows), err)
	}
	// Activities and follow-ups follow their deal.
	if _, err := r.SaveFollowup(as(alice, ScopeOwn), f.org, "", Followup{Title: "Call A", DueDate: "2030-01-01", DealID: idA}); err != nil {
		t.Fatal(err)
	}
	raw, _ := r.ListFollowups(as(bob, ScopeOwn), f.org)
	if strings.Contains(string(raw), "Call A") {
		t.Error("bob can see alice's follow-up")
	}
	// Team scope: bob and carol share a team; carol then sees bob's deal but not alice's.
	team, err := r.SaveTeam(ctx, f.org, "", Team{Name: "East", Members: []string{bob, carol}})
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(as(carol, ScopeTeam)); got != "Bob deal" {
		t.Errorf("carol(team) sees %q", got)
	}
	if _, err := r.GetDeal(as(carol, ScopeTeam), f.org, idB); err != nil {
		t.Errorf("carol(team) reading bob's deal: %v", err)
	}
	if _, err := r.GetDeal(as(carol, ScopeTeam), f.org, idA); !errors.Is(err, ErrNotFound) {
		t.Errorf("carol(team) reading alice's deal: %v", err)
	}
	_ = r.DeleteTeam(ctx, f.org, team.ID)
	// Scope resolution: org default applies until a user override is set.
	set, _ := r.GetSettings(ctx, f.org)
	set.DefaultScope = ScopeOwn
	if _, err := r.SaveSettings(ctx, f.org, set); err != nil {
		t.Fatal(err)
	}
	if s, _ := r.ResolveScope(ctx, f.org, carol); s != ScopeOwn {
		t.Errorf("default scope %q", s)
	}
	_ = r.SetUserScope(ctx, f.org, carol, ScopeAll)
	if s, _ := r.ResolveScope(ctx, f.org, carol); s != ScopeAll {
		t.Errorf("override scope %q", s)
	}
	// Another org sees nothing.
	other := newOrg(t, pool, 1)
	if raw, _ := r.ListDeals(ctx, other.org); string(raw) != "[]" {
		t.Errorf("other org sees %s", raw)
	}
}

func TestAutomationIsIdempotent(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 2)
	ctx := WithViewer(context.Background(), Viewer{UserID: f.users[0], Scope: ScopeAll})
	if _, err := r.SaveRule(ctx, f.org, "", Rule{Name: "Proposal follow-up", Enabled: true, Trigger: TrigStageChanged, Conditions: map[string]any{"stage": "proposal"},
		Actions: []Action{{Type: "create_followup", Title: "Chase {{deal.title}}", DueInDays: 2}, {Type: "assign_owner", Users: f.users}}}); err != nil {
		t.Fatal(err)
	}
	d, err := r.SaveDeal(ctx, f.org, "", Deal{Title: "Big deal", Value: 10})
	if err != nil {
		t.Fatal(err)
	}
	id := idOf(t, d)
	if _, err := r.MoveDeal(ctx, f.org, id, "proposal", ""); err != nil {
		t.Fatal(err)
	}
	followups := func() int {
		return count(t, pool, `SELECT count(*) FROM adrient.crm_followups WHERE org_id=$1 AND title='Chase Big deal'`, f.org)
	}
	if followups() != 1 {
		t.Fatalf("expected one follow-up after the move, got %d", followups())
	}
	// Re-firing the very same event (a retry) must not run the actions again.
	var hid string
	_ = pool.QueryRow(ctx, `SELECT id::text FROM adrient.crm_stage_history WHERE deal_id=$1 ORDER BY entered_at DESC LIMIT 1`, id).Scan(&hid)
	r.fire(ctx, Event{Org: f.org, Trigger: TrigStageChanged, DealID: id, Stage: "proposal", FromStage: "new", Key: "stage:" + hid})
	r.fire(ctx, Event{Org: f.org, Trigger: TrigStageChanged, DealID: id, Stage: "proposal", FromStage: "new", Key: "stage:" + hid})
	if followups() != 1 {
		t.Fatalf("duplicate event created %d follow-ups", followups())
	}
	// Moving to the same stage again is not a change and creates no event.
	_, _ = r.MoveDeal(ctx, f.org, id, "proposal", "")
	if followups() != 1 {
		t.Fatal("no-op move fired a rule")
	}
	// A genuine second entry into the stage is a new event and fires again.
	_, _ = r.MoveDeal(ctx, f.org, id, "new", "")
	_, _ = r.MoveDeal(ctx, f.org, id, "proposal", "")
	if followups() != 2 {
		t.Fatalf("re-entering the stage should fire once more, got %d", followups())
	}
	// Round robin spreads owners over runs.
	owners := map[string]bool{}
	for i := 0; i < 2; i++ {
		raw, _ := r.SaveDeal(ctx, f.org, "", Deal{Title: fmt.Sprintf("RR %d", i)})
		rid := idOf(t, raw)
		_, _ = r.MoveDeal(ctx, f.org, rid, "proposal", "")
		var o string
		_ = pool.QueryRow(ctx, `SELECT owner_id::text FROM adrient.crm_deals WHERE id=$1`, rid).Scan(&o)
		owners[o] = true
	}
	if len(owners) != 2 {
		t.Errorf("round robin used %d owners", len(owners))
	}
	// Idle sweep: an old deal fires once, a second sweep does nothing.
	if _, err := r.SaveRule(ctx, f.org, "", Rule{Name: "Idle", Enabled: true, Trigger: TrigDealIdle, Conditions: map[string]any{"idle_days": 7},
		Actions: []Action{{Type: "create_followup", Title: "Revive {{deal.title}}"}}}); err != nil {
		t.Fatal(err)
	}
	raw, _ := r.SaveDeal(ctx, f.org, "", Deal{Title: "Dusty"})
	did := idOf(t, raw)
	_, _ = pool.Exec(ctx, `UPDATE adrient.crm_deals SET updated_at=now()-interval '30 days', stage_entered_at=now()-interval '30 days', created_at=now()-interval '30 days' WHERE id=$1`, did)
	n1, err := r.SweepIdle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n2, _ := r.SweepIdle(ctx)
	if n1 < 1 || n2 != 0 {
		t.Errorf("sweep fired %d then %d (want >=1 then 0)", n1, n2)
	}
	if got := count(t, pool, `SELECT count(*) FROM adrient.crm_followups WHERE org_id=$1 AND title='Revive Dusty'`, f.org); got != 1 {
		t.Errorf("idle follow-ups: %d", got)
	}
}

func TestPlaybookGate(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 1)
	ctx := WithViewer(context.Background(), Viewer{UserID: f.users[0], Scope: ScopeAll})
	if _, err := r.SavePlaybook(ctx, f.org, Playbook{Stage: "new", Name: "Intake", Items: []string{"Qualify budget", "Book call"}, Required: true}); err != nil {
		t.Fatal(err)
	}
	raw, _ := r.SaveDeal(ctx, f.org, "", Deal{Title: "P"})
	id := idOf(t, raw)
	items, _ := r.ListChecklist(ctx, f.org, id)
	if len(items) != 2 {
		t.Fatalf("checklist seeded with %d items", len(items))
	}
	if _, err := r.MoveDeal(ctx, f.org, id, "qualified", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("required checklist must block the move: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := r.SetChecklistDone(ctx, f.org, id, "new", i, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.MoveDeal(ctx, f.org, id, "qualified", ""); err != nil {
		t.Fatalf("move after completing the checklist: %v", err)
	}
	// Re-entering a stage keeps the done state (seeding is idempotent).
	_, _ = r.MoveDeal(ctx, f.org, id, "new", "")
	items, _ = r.ListChecklist(ctx, f.org, id)
	if len(items) != 2 || !items[0].Done {
		t.Errorf("checklist after re-entry: %+v", items)
	}
}

func TestMergePeopleAndCompanies(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 1)
	ctx := WithViewer(context.Background(), Viewer{UserID: f.users[0], Scope: ScopeAll})
	c1 := newContact(t, pool, f, "Asha Rao", "asha@x.com")
	c2 := newContact(t, pool, f, "A. Rao", "asha.rao@y.com")
	_, _ = pool.Exec(ctx, `UPDATE adrient.contacts SET phones='["+91 98765 43210"]' WHERE id=$1`, c1)
	_, _ = pool.Exec(ctx, `UPDATE adrient.contacts SET phones='["098765 43210"]' WHERE id=$1`, c2)
	p1raw, _ := r.SavePerson(ctx, f.org, "", Person{ContactID: c1, Tags: []string{"a"}})
	p2raw, _ := r.SavePerson(ctx, f.org, "", Person{ContactID: c2, Tags: []string{"b"}, Source: "web"})
	p1, p2 := idOf(t, p1raw), idOf(t, p2raw)
	groups, err := r.FindDuplicates(ctx, f.org)
	if err != nil || len(groups) != 1 || groups[0].Kind != "person" {
		t.Fatalf("duplicates %+v %v", groups, err)
	}
	d, _ := r.SaveDeal(ctx, f.org, "", Deal{Title: "D", ContactID: c2})
	_, _ = r.SaveActivity(ctx, f.org, Activity{Kind: "call", Summary: "hi", ContactID: c2})
	if err := r.MergePeople(ctx, f.org, p1, p2); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_people WHERE org_id=$1`, f.org) != 1 {
		t.Error("loser person not removed")
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_deals WHERE id=$1 AND contact_id=$2`, idOf(t, d), c1) != 1 {
		t.Error("deal did not move to the winner")
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_activities WHERE org_id=$1 AND contact_id=$2`, f.org, c1) != 1 {
		t.Error("activity did not move to the winner")
	}
	if count(t, pool, `SELECT jsonb_array_length(emails) FROM adrient.contacts WHERE id=$1`, c1) != 2 {
		t.Error("emails not unioned")
	}
	if count(t, pool, `SELECT count(*) FROM adrient.contacts WHERE id=$1 AND trashed_at IS NOT NULL`, c2) != 1 {
		t.Error("loser contact should be trashed, not deleted")
	}
	var tags []string
	_ = pool.QueryRow(ctx, `SELECT tags FROM adrient.crm_people WHERE id=$1`, p1).Scan(&tags)
	if len(tags) != 2 {
		t.Errorf("tags %v", tags)
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_merge_log WHERE org_id=$1`, f.org) != 1 {
		t.Error("merge not logged")
	}
	if err := r.MergePeople(ctx, f.org, p1, p1); !errors.Is(err, ErrInvalid) {
		t.Error("merging a record into itself must fail")
	}
	// Companies.
	a, _ := r.SaveCompany(ctx, f.org, "", Company{Name: "Acme Pvt Ltd", Website: "https://acme.com", Tags: []string{"x"}})
	b, _ := r.SaveCompany(ctx, f.org, "", Company{Name: "ACME", Industry: "Retail", Tags: []string{"y"}})
	ia, ib := idOf(t, a), idOf(t, b)
	groups, _ = r.FindDuplicates(ctx, f.org)
	found := false
	for _, g := range groups {
		if g.Kind == "company" && len(g.IDs) == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("company duplicate not found: %+v", groups)
	}
	_, _ = r.SaveDeal(ctx, f.org, "", Deal{Title: "CD", CompanyID: ib})
	if err := r.MergeCompanies(ctx, f.org, ia, ib); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_deals WHERE company_id=$1`, ia) != 1 || count(t, pool, `SELECT count(*) FROM adrient.crm_companies WHERE org_id=$1`, f.org) != 1 {
		t.Error("company merge did not repoint or delete")
	}
	var industry string
	_ = pool.QueryRow(ctx, `SELECT industry FROM adrient.crm_companies WHERE id=$1`, ia).Scan(&industry)
	if industry != "Retail" {
		t.Errorf("empty winner field should take the loser value, got %q", industry)
	}
}

func TestCustomFieldsRoundTrip(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 1)
	ctx := WithViewer(context.Background(), Viewer{UserID: f.users[0], Scope: ScopeAll})
	if _, err := r.SaveFieldDef(ctx, f.org, "", FieldDef{Entity: "deal", Key: "region", Label: "Region", Type: "select", Options: []string{"north", "south"}, Required: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SaveFieldDef(ctx, f.org, "", FieldDef{Entity: "lead", Key: "budget", Label: "Budget", Type: "number"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SaveFieldDef(ctx, f.org, "", FieldDef{Entity: "contact", Key: "budget", Label: "Budget 2", Type: "number"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("lead/contact key clash should fail: %v", err)
	}
	if _, err := r.SaveDeal(ctx, f.org, "", Deal{Title: "No region"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("required custom field not enforced: %v", err)
	}
	if _, err := r.SaveDeal(ctx, f.org, "", Deal{Title: "Bad", Custom: map[string]any{"region": "east"}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("select option not enforced: %v", err)
	}
	raw, err := r.SaveDeal(ctx, f.org, "", Deal{Title: "Good", Custom: map[string]any{"region": "north"}})
	if err != nil {
		t.Fatal(err)
	}
	// Updating without custom keeps the stored value.
	raw, err = r.SaveDeal(ctx, f.org, idOf(t, raw), Deal{Title: "Good 2"})
	if err != nil || !strings.Contains(string(raw), `"region": "north"`) && !strings.Contains(string(raw), `"region":"north"`) {
		t.Fatalf("custom lost on update: %s %v", raw, err)
	}
	c := newContact(t, pool, f, "Lead One", "one@x.com")
	praw, err := r.SavePerson(ctx, f.org, "", Person{ContactID: c, Custom: map[string]any{"budget": "2500"}})
	if err != nil || !strings.Contains(string(praw), "2500") {
		t.Fatalf("person custom: %s %v", praw, err)
	}
	// Search finds a record by a custom value; export has the column.
	hits, _ := r.Search(ctx, f.org, "north", 10)
	if len(hits) != 1 || hits[0].Kind != "deal" {
		t.Errorf("custom value search: %+v", hits)
	}
	rp, err := r.Export(ctx, f.org, "deals")
	if err != nil || rp.Columns[len(rp.Columns)-1] != "custom:region" || fmt.Sprint(rp.Rows[0][len(rp.Columns)-1]) != "north" {
		t.Errorf("export: %v %v", rp.Columns, err)
	}
	// Deleting a definition strips the values.
	defs, _ := r.ListFieldDefs(ctx, f.org)
	for _, d := range defs {
		if d.Key == "region" {
			_ = r.DeleteFieldDef(ctx, f.org, d.ID)
		}
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_deals WHERE org_id=$1 AND custom ? 'region'`, f.org) != 0 {
		t.Error("deleted field values still stored")
	}
}

func TestViewsPerUserAndShared(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 2)
	ctx := context.Background()
	a, b := f.users[0], f.users[1]
	mine, err := r.SaveView(ctx, f.org, a, "", View{Entity: "deal", Name: "Hot", Config: json.RawMessage(`{"filters":{"stage":"proposal"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	shared, _ := r.SaveView(ctx, f.org, a, "", View{Entity: "lead", Name: "Team leads", Shared: true})
	if vs, _ := r.ListViews(ctx, f.org, b); len(vs) != 1 || vs[0].ID != shared.ID || vs[0].Mine {
		t.Errorf("bob should only see the shared view: %+v", vs)
	}
	if err := r.DeleteView(ctx, f.org, b, mine.ID); !errors.Is(err, ErrNotFound) {
		t.Error("bob deleted alice's view")
	}
	if _, err := r.SaveView(ctx, f.org, b, shared.ID, View{Entity: "lead", Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Error("bob edited alice's shared view")
	}
	if vs, _ := r.ListViews(ctx, f.org, a); len(vs) != 2 {
		t.Errorf("alice should see both: %+v", vs)
	}
}

const sfLeads = "Id,FirstName,LastName,Email,Phone,Company,Title,LeadSource,Status\n" +
	"00Q1,Asha,Rao,asha@acme.com,98765 43210,Acme,CTO,Web,Open - Not Contacted\n" +
	"00Q2,Li,Wei,li@beta.io,,Beta Inc,,Referral,Converted\n" +
	"00Q3,,,not-an-email,,Gamma,,,\n" +
	"00Q4,,,,,,,,\n"

func TestImportDryRunAndRerun(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 1)
	ctx := WithViewer(context.Background(), Viewer{UserID: f.users[0], Scope: ScopeAll})
	head, _, _ := ParseCSV([]byte(sfLeads), 100)
	req := ImportRequest{Source: "salesforce", Entity: "lead", CSV: sfLeads, Mapping: SuggestMapping("salesforce", "lead", head), DryRun: true}
	rep, err := r.RunImport(ctx, f.org, f.users[0], req)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Created != 2 || len(rep.Errors) != 2 || rep.Errors[0].Row != 3 || rep.Total != 4 {
		t.Fatalf("dry run report %+v", rep)
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_people WHERE org_id=$1`, f.org) != 0 || count(t, pool, `SELECT count(*) FROM adrient.contacts WHERE org_id=$1`, f.org) != 0 {
		t.Fatal("dry run wrote data")
	}
	req.DryRun = false
	rep, err = r.RunImport(ctx, f.org, f.users[0], req)
	if err != nil || rep.Created != 2 || rep.Updated != 0 {
		t.Fatalf("real run %+v %v", rep, err)
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_people WHERE org_id=$1 AND status='customer'`, f.org) != 1 {
		t.Error("Converted should map to customer")
	}
	// Re-running the same file updates, never duplicates.
	rep, err = r.RunImport(ctx, f.org, f.users[0], req)
	if err != nil || rep.Created != 0 || rep.Updated != 2 {
		t.Fatalf("re-run %+v %v", rep, err)
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_people WHERE org_id=$1`, f.org) != 2 || count(t, pool, `SELECT count(*) FROM adrient.crm_companies WHERE org_id=$1`, f.org) < 2 {
		t.Error("re-run duplicated records")
	}
	// A file without ids still matches on e-mail.
	noID := "Email,FirstName,LastName,Title\nasha@acme.com,Asha,Rao,CEO\n"
	rep, _ = r.RunImport(ctx, f.org, f.users[0], ImportRequest{Source: "csv", Entity: "lead", CSV: noID, Mapping: map[string]string{"Email": "email", "FirstName": "first_name", "LastName": "last_name", "Title": "job_title"}})
	if rep.Updated != 1 || rep.Created != 0 {
		t.Errorf("email match %+v", rep)
	}
	if count(t, pool, `SELECT count(*) FROM adrient.contacts WHERE org_id=$1 AND job_title='CEO'`, f.org) != 1 {
		t.Error("job title not updated")
	}
	// Deals with stage mapping and a company link; second run is idempotent.
	deals := "Deal Name,Stage,Amount,Closing Date,Account Name,Record Id\nBig,Closed Won,\"1,50,000\",31/12/2025,Acme,D1\nSmall,Proposal/Price Quote,500,2026-02-01,Beta Inc,D2\n"
	dh, _, _ := ParseCSV([]byte(deals), 10)
	dreq := ImportRequest{Source: "zoho", Entity: "deal", CSV: deals, Mapping: SuggestMapping("zoho", "deal", dh)}
	rep, err = r.RunImport(ctx, f.org, f.users[0], dreq)
	if err != nil || rep.Created != 2 || len(rep.Errors) != 0 {
		t.Fatalf("deal import %+v %v", rep, err)
	}
	rep, _ = r.RunImport(ctx, f.org, f.users[0], dreq)
	if rep.Created != 0 || rep.Updated != 2 || count(t, pool, `SELECT count(*) FROM adrient.crm_deals WHERE org_id=$1`, f.org) != 2 {
		t.Errorf("deal re-run %+v", rep)
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_deals WHERE org_id=$1 AND title='Big' AND stage='won' AND value=150000`, f.org) != 1 {
		t.Error("deal fields not mapped")
	}
	// Unknown mapping target is refused before anything is written.
	if _, err := r.RunImport(ctx, f.org, f.users[0], ImportRequest{Source: "csv", Entity: "lead", CSV: noID, Mapping: map[string]string{"Email": "nonsense"}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad target: %v", err)
	}
}

func TestLeadCaptureDedupeAndFormLink(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 2)
	ctx := WithViewer(context.Background(), Viewer{UserID: f.users[0], Scope: ScopeAll})
	if _, err := r.SaveRule(ctx, f.org, "", Rule{Name: "New lead", Enabled: true, Trigger: TrigLeadCreated,
		Actions: []Action{{Type: "create_followup", Title: "Call {{person.name}}", DueInDays: 1}, {Type: "assign_owner", Users: f.users}}}); err != nil {
		t.Fatal(err)
	}
	form, err := r.SaveLeadForm(ctx, f.org, f.users[0], "", LeadForm{Name: "Contact us", Fields: []FormField{{Key: "name", Required: true}, {Key: "email", Required: true}, {Key: "message"}}})
	if err != nil || len(form.Token) < 20 {
		t.Fatalf("form %+v %v", form, err)
	}
	vals := map[string]string{"name": "Ravi", "email": "Ravi@Shop.in", "message": "Need pricing"}
	res, _, err := r.SubmitLeadForm(context.Background(), form.Token, vals, "s1")
	if err != nil || !res.Created {
		t.Fatalf("submit %+v %v", res, err)
	}
	// The same person submitting again (new submission id) attaches to the same lead.
	res2, _, err := r.SubmitLeadForm(context.Background(), form.Token, vals, "s2")
	if err != nil || res2.Created || res2.PersonID != res.PersonID {
		t.Fatalf("second submit %+v %v", res2, err)
	}
	// Replaying the same submission id does not run form rules twice.
	_, _, _ = r.SubmitLeadForm(context.Background(), form.Token, vals, "s2")
	if count(t, pool, `SELECT count(*) FROM adrient.crm_people WHERE org_id=$1`, f.org) != 1 {
		t.Error("duplicate lead created")
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_followups WHERE org_id=$1 AND title='Call Ravi'`, f.org) != 1 {
		t.Error("lead_created rule should have fired exactly once")
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_people WHERE org_id=$1 AND owner_id IS NOT NULL AND source='web form'`, f.org) != 1 {
		t.Error("lead should have source and an owner")
	}
	if _, _, err := r.SubmitLeadForm(context.Background(), form.Token, map[string]string{"name": "X", "email": "bad"}, "s3"); !errors.Is(err, ErrInvalid) {
		t.Errorf("invalid email accepted: %v", err)
	}
	if _, _, err := r.SubmitLeadForm(context.Background(), form.Token, map[string]string{"name": "X"}, "s4"); !errors.Is(err, ErrInvalid) {
		t.Errorf("required email missing accepted: %v", err)
	}
	if _, _, err := r.SubmitLeadForm(context.Background(), "nope", vals, "s5"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown token: %v", err)
	}
	form.Enabled = false
	_, _ = r.SaveLeadForm(ctx, f.org, f.users[0], form.ID, form)
	if _, _, err := r.SubmitLeadForm(context.Background(), form.Token, vals, "s6"); !errors.Is(err, ErrNotFound) {
		t.Errorf("disabled form accepted a submission: %v", err)
	}
	// Forms-app link.
	if _, err := r.SaveFormLink(ctx, f.org, f.users[0], FormLink{FormID: "form-1", Mapping: map[string]string{"q1": "name", "q2": "email", "q3": "company"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	ans := map[string]any{"q1": "Meera", "q2": "meera@zeta.com", "q3": "Zeta Labs"}
	if err := r.FromFormsResponse(context.Background(), f.org, "form-1", "Demo request", "resp-1", "", ans); err != nil {
		t.Fatal(err)
	}
	_ = r.FromFormsResponse(context.Background(), f.org, "form-1", "Demo request", "resp-1", "", ans)
	if count(t, pool, `SELECT count(*) FROM adrient.contacts WHERE org_id=$1 AND display_name='Meera'`, f.org) != 1 {
		t.Error("forms response should create exactly one contact")
	}
	if err := r.FromFormsResponse(context.Background(), f.org, "unlinked", "x", "r", "", ans); err != nil {
		t.Errorf("unlinked form must be ignored: %v", err)
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_companies WHERE org_id=$1 AND name='Zeta Labs'`, f.org) != 1 {
		t.Error("company from form answer missing")
	}
}

func TestMailLogMatchesByAddress(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 1)
	ctx := WithViewer(context.Background(), Viewer{UserID: f.users[0], Scope: ScopeAll})
	c := newContact(t, pool, f, "Asha", "asha@acme.com")
	praw, _ := r.SavePerson(ctx, f.org, "", Person{ContactID: c})
	pid := idOf(t, praw)
	ins := func(folder, from, to, subj string) {
		tj, _ := json.Marshal([]string{to})
		if _, err := pool.Exec(ctx, `INSERT INTO adrient.mail_messages (org_id, owner_id, folder, from_addr, to_addrs, subject, body, snippet) VALUES ($1,$2,$3,$4,$5,$6,'secret body','snip')`,
			f.org, f.users[0], folder, from, tj, subj); err != nil {
			t.Fatal(err)
		}
	}
	ins("inbox", "Asha <ASHA@acme.com>", "me@us.com", "Quote please")
	ins("sent", "me@us.com", "asha@acme.com", "Re: Quote please")
	ins("inbox", "stranger@else.com", "me@us.com", "Spam")
	n, err := r.SyncMail(ctx, f.org, f.users[0])
	if err != nil || n != 2 {
		t.Fatalf("logged %d err %v", n, err)
	}
	n, _ = r.SyncMail(ctx, f.org, f.users[0])
	if n != 0 {
		t.Errorf("second sync logged %d (must be idempotent)", n)
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_mail_log WHERE person_id=$1`, pid) != 2 || count(t, pool, `SELECT count(*) FROM adrient.crm_activities WHERE org_id=$1 AND kind='email'`, f.org) != 2 {
		t.Error("log or activity rows missing")
	}
	if count(t, pool, `SELECT count(*) FROM adrient.crm_mail_log WHERE snippet LIKE '%secret%'`) != 0 {
		t.Error("message body must never be copied")
	}
}

func TestPriceBookAndDealItems(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 1)
	ctx := WithViewer(context.Background(), Viewer{UserID: f.users[0], Scope: ScopeAll})
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.inv_products (org_id,name,sku,price) VALUES ($1,'Widget','W-1',250), ($1,'Old','O-1',5)`, f.org); err != nil {
		t.Skipf("inventory table unavailable: %v", err)
	}
	_, _ = pool.Exec(ctx, `UPDATE adrient.inv_products SET archived=true WHERE org_id=$1 AND sku='O-1'`, f.org)
	a, u, err := r.SyncInventoryProducts(ctx, f.org)
	if err != nil || a != 1 || u != 0 {
		t.Fatalf("sync %d/%d %v", a, u, err)
	}
	_, _ = pool.Exec(ctx, `UPDATE adrient.inv_products SET price=300 WHERE org_id=$1 AND sku='W-1'`, f.org)
	a, u, _ = r.SyncInventoryProducts(ctx, f.org)
	if a != 0 || u != 1 {
		t.Errorf("re-sync %d/%d", a, u)
	}
	book, _ := r.ListPriceBook(ctx, f.org)
	if len(book) != 1 || book[0].Price != 300 {
		t.Fatalf("book %+v", book)
	}
	raw, err := r.SaveDeal(ctx, f.org, "", Deal{Title: "With items", Value: 1, Items: []DealItem{
		{ProductID: book[0].ID, Name: "Widget", Qty: 2, UnitPrice: 300}, {Name: "Setup", Qty: 1, UnitPrice: 100, DiscountPct: 50}}})
	if err != nil || !strings.Contains(string(raw), "650") {
		t.Fatalf("deal value should be the item total 650: %s %v", raw, err)
	}
	if _, err := r.SaveDeal(ctx, f.org, "", Deal{Title: "Bad", Items: []DealItem{{Name: "x", Qty: 0}}}); !errors.Is(err, ErrInvalid) {
		t.Error("zero quantity accepted")
	}
}

func TestReportsLeaderboardAndSettings(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 2)
	ctx := WithViewer(context.Background(), Viewer{UserID: f.users[0], Scope: ScopeAll})
	mk := func(owner, title, stage, source string, v float64) {
		raw, err := r.SaveDeal(ctx, f.org, "", Deal{Title: title, Value: v, Source: source, OwnerID: owner})
		if err != nil {
			t.Fatal(err)
		}
		if stage != "new" {
			if _, err := r.MoveDeal(ctx, f.org, idOf(t, raw), stage, "r"); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk(f.users[0], "w1", "won", "web", 1000)
	mk(f.users[0], "l1", "lost", "web", 500)
	mk(f.users[1], "w2", "won", "ads", 300)
	mk(f.users[1], "open", "new", "ads", 700)
	rp, err := r.RunReport(ctx, f.org, "winrate_source", nil)
	if err != nil || len(rp.Rows) != 2 {
		t.Fatalf("winrate %+v %v", rp, err)
	}
	for _, row := range rp.Rows {
		if row[0] == "web" && fmt.Sprint(row[3]) != "50" {
			t.Errorf("web win rate %v", row[3])
		}
	}
	for _, name := range ReportNames {
		if _, err := r.RunReport(ctx, f.org, name, nil); err != nil {
			t.Errorf("report %s: %v", name, err)
		}
	}
	if _, err := r.RunReport(ctx, f.org, "nope", nil); !errors.Is(err, ErrInvalid) {
		t.Error("unknown report")
	}
	period := time.Now().Format("2006-01")
	if err := r.SetQuota(ctx, f.org, Quota{UserID: f.users[0], Period: period, Target: 2000}); err != nil {
		t.Fatal(err)
	}
	lb, err := r.Leaderboard(ctx, f.org, period)
	if err != nil || len(lb) != 2 || lb[0].UserID != f.users[0] || lb[0].Won != 1000 || lb[0].Pct != 50 {
		t.Fatalf("leaderboard %+v %v", lb, err)
	}
	if err := r.SetQuota(ctx, f.org, Quota{UserID: f.users[0], Period: "2026-13", Target: 1}); !errors.Is(err, ErrInvalid) {
		t.Error("bad period accepted")
	}
	if _, err := r.BuildCustomer360(ctx, f.org, "company", "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Error("360 of a missing company")
	}
}

func TestCustomer360(t *testing.T) {
	r, pool := testRepo(t)
	f := newOrg(t, pool, 1)
	ctx := WithViewer(context.Background(), Viewer{UserID: f.users[0], Scope: ScopeAll})
	craw, _ := r.SaveCompany(ctx, f.org, "", Company{Name: "Acme Corp"})
	cid := idOf(t, craw)
	contact := newContact(t, pool, f, "Asha", "asha@acme.com")
	_, _ = r.SavePerson(ctx, f.org, "", Person{ContactID: contact, CompanyID: cid})
	_, _ = r.SaveDeal(ctx, f.org, "", Deal{Title: "Acme deal", CompanyID: cid, ContactID: contact, Value: 1000})
	var doc string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.invc_documents (org_id, kind, number, status, contact_id, client_name, client_email, total, share_token)
		VALUES ($1,'invoice','INV-1','sent',$2,'Acme Corp','asha@acme.com',1000,$3) RETURNING id::text`, f.org, contact, "tok"+f.org).Scan(&doc); err != nil {
		t.Skipf("invoicing table shape differs: %v", err)
	}
	_, _ = pool.Exec(ctx, `INSERT INTO adrient.invc_payments (org_id, document_id, amount) VALUES ($1,$2,400)`, f.org, doc)
	var tp string
	_ = pool.QueryRow(ctx, `INSERT INTO adrient.ticket_projects (org_id, key, name) VALUES ($1,'SUP','Support') RETURNING id::text`, f.org).Scan(&tp)
	_, _ = pool.Exec(ctx, `INSERT INTO adrient.tickets (project_id, org_id, number, title, requester_email) VALUES ($1,$2,1,'Printer broken','asha@acme.com')`, tp, f.org)
	c, err := r.BuildCustomer360(ctx, f.org, "company", cid)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.People) != 1 || len(c.Deals) != 1 || len(c.Invoices) != 1 || len(c.Tickets) != 1 {
		t.Fatalf("360 sections: people=%d deals=%d invoices=%d tickets=%d", len(c.People), len(c.Deals), len(c.Invoices), len(c.Tickets))
	}
	if c.Paid != 400 || c.Outstanding != 600 {
		t.Errorf("paid/outstanding %v/%v", c.Paid, c.Outstanding)
	}
	if len(c.Links) < 5 {
		t.Error("deep links missing")
	}
}
