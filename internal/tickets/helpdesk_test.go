package tickets

import (
	"testing"
	"time"
)

func tm(h, m int) time.Time { return time.Date(2030, 1, 7, h, m, 0, 0, time.UTC) }

func TestComputeSLA(t *testing.T) {
	p := &SLAPolicy{Priority: "high", FirstResponseMin: 60, ResolveMin: 480}
	created := tm(9, 0)
	// Running, plenty of time left.
	i := ComputeSLA(p, created, nil, nil, tm(9, 10))
	if i.FirstResponse.State != SLAOK || i.Resolution.State != SLAOK {
		t.Fatalf("ok: %+v", i)
	}
	// Less than 20% of the first-response target left (12 min of 60).
	i = ComputeSLA(p, created, nil, nil, tm(9, 50))
	if i.FirstResponse.State != SLAAtRisk {
		t.Fatalf("at risk: %+v", i)
	}
	// Past the target and still waiting.
	i = ComputeSLA(p, created, nil, nil, tm(10, 1))
	if i.FirstResponse.State != SLABreached || i.Resolution.State != SLAOK {
		t.Fatalf("breached: %+v", i)
	}
	// Answered in time, resolved late.
	fr, res := tm(9, 30), tm(18, 0)
	i = ComputeSLA(p, created, &fr, &res, tm(19, 0))
	if i.FirstResponse.State != SLAMet || i.Resolution.State != SLABreached {
		t.Fatalf("met/breached: %+v", i)
	}
	// No policy for the priority: no clocks.
	if i := ComputeSLA(nil, created, nil, nil, tm(23, 0)); i.FirstResponse.State != SLAOff || i.FirstResponse.Due != nil {
		t.Fatalf("no policy: %+v", i)
	}
	// A zero target switches that clock off.
	if i := ComputeSLA(&SLAPolicy{Priority: "low", ResolveMin: 60}, created, nil, nil, tm(23, 0)); i.FirstResponse.State != SLAOff || i.Resolution.State != SLABreached {
		t.Fatalf("zero target: %+v", i)
	}
}

func TestValidateSLA(t *testing.T) {
	if err := ValidateSLA([]SLAPolicy{{"urgent", 15, 240}, {"low", 480, 2880}}); err != nil {
		t.Fatal(err)
	}
	for i, bad := range [][]SLAPolicy{
		{{"urgent", 15, 240}, {"urgent", 30, 480}},
		{{"critical", 15, 240}},
		{{"low", -1, 10}},
		{{"low", 600, 60}},
	} {
		if ValidateSLA(bad) == nil {
			t.Fatalf("case %d should fail", i)
		}
	}
}

func TestRules(t *testing.T) {
	rules := []Rule{
		{Name: "billing", Enabled: true, MatchKeyword: "invoice, refund", SetAssignee: "u1"},
		{Name: "off", Enabled: false, MatchKeyword: "outage", SetPriority: "urgent"},
		{Name: "outage", Enabled: true, MatchKeyword: "Outage", MatchSource: "public", SetPriority: "urgent"},
		{Name: "empty", Enabled: true, SetPriority: "low"},
	}
	if m := FirstMatch(rules, "Need a REFUND", "", "normal", "public"); m == nil || m.Name != "billing" {
		t.Fatalf("billing: %+v", m)
	}
	if m := FirstMatch(rules, "Site outage", "all down", "normal", "public"); m == nil || m.Name != "outage" {
		t.Fatalf("outage: %+v", m)
	}
	// Source condition must match too.
	if m := FirstMatch(rules, "Site outage", "", "normal", "team"); m != nil {
		t.Fatalf("source mismatch should not match: %+v", m)
	}
	// A rule without conditions never matches everything.
	if m := FirstMatch(rules[3:], "anything", "", "normal", "public"); m != nil {
		t.Fatal("rule without conditions must not match")
	}
	good := Rule{Name: "x", MatchKeyword: "a", SetStatus: "in_progress"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Rule{{Name: "", MatchKeyword: "a", SetStatus: "x"}, {Name: "n", SetPriority: "low"}, {Name: "n", MatchKeyword: "a"}, {Name: "n", MatchPriority: "huge", SetStatus: "x"}} {
		if bad.Validate() == nil {
			t.Fatalf("%+v should fail", bad)
		}
	}
}

func TestOrgOfAndSlugify(t *testing.T) {
	if OrgOf("ann@acme.co.in") != "acme.co.in" || OrgOf("bob@Gmail.com") != "" || OrgOf("nope") != "" {
		t.Fatal("org from email")
	}
	if Slugify("How do I reset my password?") != "how-do-i-reset-my-password" || Slugify("???") != "article" {
		t.Fatal("slug")
	}
}

func TestSummarize(t *testing.T) {
	from := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2030, 1, 8, 0, 0, 0, 0, time.UTC)
	now := to
	d := func(day, h int) time.Time { return time.Date(2030, 1, day, h, 0, 0, 0, time.UTC) }
	r1, f1 := d(2, 12), d(2, 10)
	r2 := d(3, 9)
	five, three := 5, 3
	rows := []ReportRow{
		{Created: d(2, 9), Resolved: &r1, FirstResponse: &f1, Priority: "high", Assignee: "Ann", Rating: &five,
			SLA: SLAInfo{FirstResponse: SLAClock{State: SLAMet}, Resolution: SLAClock{State: SLAMet}}},
		{Created: d(2, 15), Resolved: &r2, Priority: "normal", Assignee: "", Rating: &three,
			SLA: SLAInfo{FirstResponse: SLAClock{State: SLABreached}}},
		{Created: d(5, 8), Priority: "normal", Assignee: "Ann"},
		{Created: d(1, 0).AddDate(0, 0, -10), Priority: "low"}, // old and still open
	}
	rep := Summarize(rows, from, to, now)
	if rep.Created != 3 || rep.Resolved != 2 || rep.OpenNow != 2 {
		t.Fatalf("counts: %+v", rep)
	}
	if rep.AvgFirstRespMin != 60 {
		t.Fatalf("first response avg = %v", rep.AvgFirstRespMin)
	}
	if rep.SLAChecked != 3 || int(rep.SLAMetPct) != 66 {
		t.Fatalf("sla: checked=%d pct=%v", rep.SLAChecked, rep.SLAMetPct)
	}
	if rep.CSATCount != 2 || rep.CSATAvg != 4 {
		t.Fatalf("csat: %+v", rep)
	}
	if rep.ByAssignee["Unassigned"] != 1 || rep.ByAssignee["Ann"] != 2 || rep.ByPriority["normal"] != 2 {
		t.Fatalf("groups: %+v %+v", rep.ByAssignee, rep.ByPriority)
	}
	if len(rep.CreatedPerDay) != 7 || rep.CreatedPerDay[1].Count != 2 {
		t.Fatalf("series: %+v", rep.CreatedPerDay)
	}
}
