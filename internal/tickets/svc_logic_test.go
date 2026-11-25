package tickets

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func testCal(t *testing.T) *Calendar {
	t.Helper()
	c := &Calendar{TZ: "Asia/Kolkata", Hours: map[string][][2]string{
		"mon": {{"09:00", "17:00"}}, "tue": {{"09:00", "17:00"}}, "wed": {{"09:00", "17:00"}},
		"thu": {{"09:00", "17:00"}}, "fri": {{"09:00", "13:00"}, {"14:00", "17:00"}},
	}, Holidays: map[string]bool{"2026-11-25": true}} // a Wednesday
	if err := c.Prepare(); err != nil {
		t.Fatal(err)
	}
	return c
}

func ist(t *testing.T, s string) time.Time {
	t.Helper()
	loc, _ := time.LoadLocation("Asia/Kolkata")
	v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestBusinessMinutes(t *testing.T) {
	c := testCal(t)
	// Mon 2026-11-23 16:00 -> Tue 10:00 = 60 + 60
	if got := c.BusinessMinutes(ist(t, "2026-11-23 16:00"), ist(t, "2026-11-24 10:00")); got != 120 {
		t.Fatalf("got %d", got)
	}
	// Tue 16:00 -> Thu 10:00: Wed is a holiday => 60 + 60
	if got := c.BusinessMinutes(ist(t, "2026-11-24 16:00"), ist(t, "2026-11-26 10:00")); got != 120 {
		t.Fatalf("holiday: got %d", got)
	}
	// Friday lunch gap 13-14 is not counted
	if got := c.BusinessMinutes(ist(t, "2026-11-27 12:00"), ist(t, "2026-11-27 15:00")); got != 120 {
		t.Fatalf("lunch: got %d", got)
	}
	// weekend: Fri 16:00 -> Mon 10:00 = 60 + 60
	if got := c.BusinessMinutes(ist(t, "2026-11-27 16:00"), ist(t, "2026-11-30 10:00")); got != 120 {
		t.Fatalf("weekend: got %d", got)
	}
	var nilCal *Calendar
	if got := nilCal.BusinessMinutes(ist(t, "2026-11-27 16:00"), ist(t, "2026-11-28 16:00")); got != 1440 {
		t.Fatalf("24x7: got %d", got)
	}
}

func TestAddBusinessMinutes(t *testing.T) {
	c := testCal(t)
	// created Mon 16:30, 120 working minutes -> 30 left Mon, 90 on Tue => Tue 10:30
	if got := c.AddBusinessMinutes(ist(t, "2026-11-23 16:30"), 120); !got.Equal(ist(t, "2026-11-24 10:30")) {
		t.Fatalf("got %v", got)
	}
	// outside hours: Sat 11:00 + 30 -> Mon 09:30
	if got := c.AddBusinessMinutes(ist(t, "2026-11-28 11:00"), 30); !got.Equal(ist(t, "2026-11-30 09:30")) {
		t.Fatalf("got %v", got)
	}
	// skips the holiday: Tue 16:00 + 120 -> Thu 10:00
	if got := c.AddBusinessMinutes(ist(t, "2026-11-24 16:00"), 120); !got.Equal(ist(t, "2026-11-26 10:00")) {
		t.Fatalf("got %v", got)
	}
	// inverse of BusinessMinutes
	s := ist(t, "2026-11-24 09:45")
	e := c.AddBusinessMinutes(s, 900)
	if got := c.BusinessMinutes(s, e); got != 900 {
		t.Fatalf("roundtrip %d", got)
	}
}

func TestSLAPauseResume(t *testing.T) {
	c := testCal(t)
	created := ist(t, "2026-11-23 09:00")
	// 4h (240 min) target. Worked 1h, paused 10:00 -> next day 10:00 (pending), resumes.
	pausedAt := ist(t, "2026-11-23 10:00")
	now := ist(t, "2026-11-24 10:00")
	timer := SLATimer{Created: created, TargetMin: 240, Calendar: c, PausedAt: &pausedAt}
	clk, used, left := SLAState(timer, now)
	if used != 60 || left != 180 || clk.State != SLAOK {
		t.Fatalf("paused: used=%d left=%d state=%s", used, left, clk.State)
	}
	// resume at now: paused business minutes = 8h = 480
	timer.PausedAt = nil
	timer.PausedMin = PauseMinutesOnResume(c, pausedAt, now)
	if timer.PausedMin != 480 {
		t.Fatalf("pausedMin %d", timer.PausedMin)
	}
	_, used, left = SLAState(timer, now)
	if used != 60 || left != 180 {
		t.Fatalf("resumed: used=%d left=%d", used, left)
	}
	// 3 working hours later => breach
	late := ist(t, "2026-11-24 13:30")
	clk, _, left = SLAState(timer, late)
	if clk.State != SLABreached || left != -30 {
		t.Fatalf("late: %s %d", clk.State, left)
	}
	// resolved in time => met; resolved late => breached
	done := ist(t, "2026-11-24 12:00")
	timer.DoneAt = &done
	if clk, _, _ = SLAState(timer, late); clk.State != SLAMet {
		t.Fatalf("want met got %s", clk.State)
	}
	done = ist(t, "2026-11-24 13:30")
	if clk, _, _ = SLAState(timer, late); clk.State != SLABreached {
		t.Fatalf("want breached got %s", clk.State)
	}
	// at risk: under 20% left
	timer.DoneAt = nil
	if clk, _, _ = SLAState(timer, ist(t, "2026-11-24 12:40")); clk.State != SLAAtRisk {
		t.Fatalf("want at_risk got %s", clk.State)
	}
}

func TestCalendarValidation(t *testing.T) {
	bad := []*Calendar{
		{TZ: "Nowhere/City"},
		{TZ: "UTC", Hours: map[string][][2]string{"mon": {{"18:00", "09:00"}}}},
		{TZ: "UTC", Hours: map[string][][2]string{"xyz": {{"09:00", "10:00"}}}},
		{TZ: "UTC", Hours: map[string][][2]string{"mon": {{"9am", "10:00"}}}},
	}
	for i, c := range bad {
		if c.Prepare() == nil {
			t.Fatalf("calendar %d should be rejected", i)
		}
	}
	// a calendar with no hours at all must not hang
	empty := &Calendar{TZ: "UTC"}
	_ = empty.Prepare()
	got := empty.AddBusinessMinutes(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 60)
	if got.IsZero() {
		t.Fatal("empty calendar")
	}
}

func TestPickAssignee(t *testing.T) {
	now := time.Now()
	away := now.Add(24 * time.Hour)
	ms := []Member{
		{UserID: "a", Capacity: 3, Open: 1, Skills: []string{"billing"}, Position: 0},
		{UserID: "b", Capacity: 3, Open: 0, Skills: []string{"network"}, Position: 1},
		{UserID: "c", Capacity: 3, Open: 3, Position: 2},
		{UserID: "d", Capacity: 3, Open: 0, OOOUntil: &away, Position: 3},
	}
	// round robin skips full (c) and out-of-office (d), wraps around
	u, next := PickAssignee(StrategyRoundRobin, ms, nil, 0, now)
	if u != "a" || next != 1 {
		t.Fatalf("rr1 %s %d", u, next)
	}
	u, next = PickAssignee(StrategyRoundRobin, ms, nil, next, now)
	if u != "b" || next != 2 {
		t.Fatalf("rr2 %s %d", u, next)
	}
	u, next = PickAssignee(StrategyRoundRobin, ms, nil, next, now)
	if u != "a" || next != 1 {
		t.Fatalf("rr3 (skip c,d, wrap) %s %d", u, next)
	}
	// load-based picks lowest utilisation
	if u, _ = PickAssignee(StrategyLoad, ms, nil, 0, now); u != "b" {
		t.Fatalf("load %s", u)
	}
	// skills routing
	if u, _ = PickAssignee(StrategyLoad, ms, []string{"billing"}, 0, now); u != "a" {
		t.Fatalf("skill %s", u)
	}
	if u, _ = PickAssignee(StrategyRoundRobin, ms, []string{"legal"}, 0, now); u != "" {
		t.Fatalf("no skill match should leave unassigned, got %s", u)
	}
	// out of office that has ended counts as available again
	past := now.Add(-time.Hour)
	ms[3].OOOUntil = &past
	ms[0].Open, ms[1].Open = 3, 3
	if u, _ = PickAssignee(StrategyLoad, ms, nil, 0, now); u != "d" {
		t.Fatalf("returned from ooo %s", u)
	}
	if u, _ = PickAssignee(StrategyManual, ms, nil, 0, now); u != "" {
		t.Fatal("manual never assigns")
	}
	if u, _ = PickAssignee(StrategyRoundRobin, nil, nil, 0, now); u != "" {
		t.Fatal("empty group")
	}
}

func TestPriorityMatrix(t *testing.T) {
	cases := map[[2]string]string{
		{"high", "high"}: "urgent", {"high", "medium"}: "high", {"medium", "medium"}: "normal",
		{"low", "medium"}: "normal", {"low", "low"}: "low", {"", ""}: "normal",
	}
	for in, want := range cases {
		if got := PriorityFromMatrix(in[0], in[1]); got != want {
			t.Errorf("%v: got %s want %s", in, got, want)
		}
	}
}

func TestStatusWorkflow(t *testing.T) {
	defs := DefaultStatusDefs()
	if err := ValidateStatusDefs(defs); err != nil {
		t.Fatal(err)
	}
	if !PausesSLA(defs, "pending") || PausesSLA(defs, "open") {
		t.Fatal("pause flags")
	}
	if CategoryOf(defs, "resolved") != "solved" {
		t.Fatal("category")
	}
	if !ShouldReopen(defs, "resolved", true) || ShouldReopen(defs, "closed", true) || ShouldReopen(defs, "resolved", false) {
		t.Fatal("reopen")
	}
	defs[4].Requires = []string{"category", "assignee"}
	miss := MissingRequired(defs, "resolved", map[string]bool{"assignee": true})
	if !reflect.DeepEqual(miss, []string{"category"}) {
		t.Fatalf("missing %v", miss)
	}
	if err := ValidateStatusDefs([]StatusDef{{Key: "a", Category: "open"}}); err == nil {
		t.Fatal("needs a done status")
	}
	if err := ValidateStatusDefs([]StatusDef{{Key: "a", Category: "open"}, {Key: "A", Category: "solved"}, {Key: "a", Category: "closed"}}); err == nil {
		t.Fatal("duplicate keys")
	}
}

func sampleTicket(now time.Time) AutoTicket {
	return AutoTicket{Title: "VPN down for finance", Body: "cannot connect", Status: "open", Priority: "normal",
		Type: "incident", Source: "web", RequesterEmail: "ravi@acme.example", Tags: []string{"vpn"},
		CreatedAt: now.Add(-50 * time.Hour), UpdatedAt: now.Add(-49 * time.Hour)}
}

func TestAutomationEvaluateAndApply(t *testing.T) {
	now := time.Now()
	a := Automation{Name: "VPN to network", Kind: KindTrigger, Event: "create", MatchMode: "all", Enabled: true,
		Conds:   []Condition{{"title", "contains", "vpn"}, {"requester_domain", "is", "acme.example"}},
		Actions: []Action{{"set_priority", "high"}, {"set_group", "g-net"}, {"add_tag", "Network"}, {"notify", "assignee"}}}
	if err := a.Validate(nil); err != nil {
		t.Fatal(err)
	}
	ok, trace := a.Evaluate(sampleTicket(now), "create", now)
	if !ok || len(trace) != 2 {
		t.Fatalf("should match: %v %v", ok, trace)
	}
	if ok, _ := a.Evaluate(sampleTicket(now), "update", now); ok {
		t.Fatal("create-only rule fired on update")
	}
	out, fx := a.Apply(sampleTicket(now))
	if out.Priority != "high" || out.Group != "g-net" || !containsStr(out.Tags, "network") {
		t.Fatalf("apply: %+v", out)
	}
	if len(fx) != 1 || fx[0].Type != "notify" {
		t.Fatalf("effects %v", fx)
	}
	// original untouched
	if sampleTicket(now).Priority != "normal" {
		t.Fatal("mutated input")
	}
	// match any
	a.MatchMode = "any"
	a.Conds = []Condition{{"title", "contains", "printer"}, {"tag", "is", "VPN"}}
	if ok, _ := a.Evaluate(sampleTicket(now), "create", now); !ok {
		t.Fatal("any should match via tag")
	}
	a.Enabled = false
	if ok, _ := a.Evaluate(sampleTicket(now), "create", now); ok {
		t.Fatal("disabled rule fired")
	}
}

func TestAutomationTimeRule(t *testing.T) {
	now := time.Now()
	a := Automation{Name: "Auto close", Kind: KindTime, MatchMode: "all", Enabled: true,
		Conds:   []Condition{{"status", "is", "resolved"}, {"hours_since_update", "gt", "72"}},
		Actions: []Action{{"set_status", "closed"}}}
	if err := a.Validate([]string{"open", "resolved", "closed"}); err != nil {
		t.Fatal(err)
	}
	tk := sampleTicket(now)
	tk.Status = "resolved"
	tk.UpdatedAt = now.Add(-80 * time.Hour)
	if ok, _ := a.Evaluate(tk, "any", now); !ok {
		t.Fatal("should close")
	}
	tk.UpdatedAt = now.Add(-10 * time.Hour)
	if ok, _ := a.Evaluate(tk, "any", now); ok {
		t.Fatal("too recent")
	}
	if ok, _ := a.Evaluate(tk, "create", now); ok {
		t.Fatal("time rules only run on sweeps")
	}
}

func TestAutomationValidation(t *testing.T) {
	bad := []Automation{
		{Name: "", Actions: []Action{{"add_tag", "x"}}},
		{Name: "x"},
		{Name: "x", Actions: []Action{{"launch_rocket", "1"}}},
		{Name: "x", Actions: []Action{{"set_priority", "huge"}}},
		{Name: "x", Actions: []Action{{"set_status", "nope"}}},
		{Name: "x", Actions: []Action{{"webhook", "ftp://x"}}},
		{Name: "x", Actions: []Action{{"webhook", "javascript:alert(1)"}}},
		{Name: "x", Conds: []Condition{{"nope", "is", "1"}}, Actions: []Action{{"add_tag", "x"}}},
		{Name: "x", Conds: []Condition{{"title", "gt", "1"}}, Actions: []Action{{"add_tag", "x"}}},
		{Name: "x", Conds: []Condition{{"hours_since_update", "gt", "abc"}}, Actions: []Action{{"add_tag", "x"}}},
		{Name: "x", Kind: KindTime, Actions: []Action{{"add_tag", "x"}}},
	}
	for i, a := range bad {
		if a.Validate([]string{"open", "closed"}) == nil {
			t.Errorf("rule %d should be rejected: %+v", i, a)
		}
	}
}

// Rules survive a JSON round trip and behave identically afterwards.
func TestAutomationRoundTrip(t *testing.T) {
	now := time.Now()
	rules := []Automation{
		{Name: "r1", Kind: KindDispatch, MatchMode: "all", Enabled: true, Position: 0,
			Conds: []Condition{{"type", "is", "incident"}}, Actions: []Action{{"set_group", "g1"}, {"mark_major", "1"}}},
		{Name: "r2", Kind: KindTrigger, Event: "any", MatchMode: "any", Enabled: true, Position: 1,
			Conds: []Condition{{"priority", "in", "high,urgent"}, {"tag", "is", "vip"}}, Actions: []Action{{"add_tag", "escalated"}}},
	}
	for i := range rules {
		if err := rules[i].Validate(nil); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	var back []Automation
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rules, back) {
		t.Fatalf("round trip changed rules:\n%+v\n%+v", rules, back)
	}
	tk := sampleTicket(now)
	tk.Priority = "urgent"
	a1, f1, e1 := RunAll(rules, tk, "create", now)
	a2, f2, e2 := RunAll(back, tk, "create", now)
	if !reflect.DeepEqual(a1, a2) || !reflect.DeepEqual(f1, f2) || !reflect.DeepEqual(e1, e2) {
		t.Fatal("results differ after round trip")
	}
	if !a1.Major || a1.Group != "g1" || !containsStr(a1.Tags, "escalated") || len(f1) != 2 {
		t.Fatalf("unexpected result: %+v fired=%v", a1, f1)
	}
	if DescribeAutomation(rules[1]) == "" {
		t.Fatal("describe")
	}
}
