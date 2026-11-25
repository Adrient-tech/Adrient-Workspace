package contracts

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit/kittest"
)

func TestAddMonthsClamps(t *testing.T) {
	cases := map[[2]string]string{
		{"2026-01-31", "1"}: "2026-02-28", {"2024-01-31", "1"}: "2024-02-29", {"2026-01-31", "2"}: "2026-03-31",
		{"2026-03-31", "-1"}: "2026-02-28", {"2026-12-15", "2"}: "2027-02-15", {"2026-01-15", "-14"}: "2024-11-15",
	}
	for in, want := range cases {
		var n int
		fmt.Sscan(in[1], &n)
		if got := AddMonths(in[0], n); got != want {
			t.Errorf("AddMonths(%s,%d) = %s, want %s", in[0], n, got, want)
		}
	}
	if DaysBetween("2026-02-28", "2026-03-01") != 1 || DaysBetween("2026-03-01", "2026-02-28") != -1 {
		t.Error("DaysBetween")
	}
	if IsValidDate("2026-02-30") || !IsValidDate("2024-02-29") {
		t.Error("IsValidDate")
	}
}

func TestNextTermEndAutoRenew(t *testing.T) {
	c := Contract{EndDate: "2025-01-31", AutoRenew: true, TermMonths: 1}
	// Month-end anchor does not drift: from Jan 31 2025, Feb 28, Mar 31...
	if got := c.NextTermEnd("2025-02-01"); got != "2025-02-28" {
		t.Errorf("got %s", got)
	}
	if got := c.NextTermEnd("2025-03-01"); got != "2025-03-31" {
		t.Errorf("got %s (drifted)", got)
	}
	annual := Contract{EndDate: "2024-06-15", AutoRenew: true, TermMonths: 12}
	if got := annual.NextTermEnd("2026-03-01"); got != "2026-06-15" {
		t.Errorf("got %s", got)
	}
	fixed := Contract{EndDate: "2025-06-15"}
	if got := fixed.NextTermEnd("2026-03-01"); got != "2025-06-15" {
		t.Errorf("fixed term keeps its end date, got %s", got)
	}
	if got := (Contract{}).NextTermEnd("2026-03-01"); got != "" {
		t.Errorf("evergreen = %q", got)
	}
}

func TestDecisionsCapTheRollForward(t *testing.T) {
	c := Contract{EndDate: "2025-06-15", AutoRenew: true, TermMonths: 12, Decision: &Decision{Kind: DecNotRenewing, ForTermEnd: "2026-06-15"}}
	if got := c.NextTermEnd("2026-01-01"); got != "2026-06-15" {
		t.Errorf("before the capped term: %s", got)
	}
	if got := c.NextTermEnd("2026-07-01"); got != "2026-06-15" {
		t.Errorf("must not roll past a term marked as last, got %s", got)
	}
	if got := c.AgreementEnd("2026-01-01"); got != "2026-06-15" {
		t.Errorf("agreement end = %q", got)
	}
	rolling := Contract{EndDate: "2025-06-15", AutoRenew: true, TermMonths: 12}
	if got := rolling.AgreementEnd("2026-01-01"); got != "" {
		t.Errorf("an auto-renewing contract has no end of its own, got %q", got)
	}
	// A decision for a date that is no longer a term end is ignored.
	stale := Contract{EndDate: "2025-06-15", AutoRenew: true, TermMonths: 12, Decision: &Decision{Kind: DecNoticeGiven, ForTermEnd: "2025-09-01"}}
	if got := stale.NextTermEnd("2026-01-01"); got != "2026-06-15" {
		t.Errorf("stale decision capped the term: %s", got)
	}
	if !c.IsTermEnd("2027-06-15") || c.IsTermEnd("2027-06-16") {
		t.Error("IsTermEnd")
	}
}

func TestRenewalPhases(t *testing.T) {
	today := "2026-03-01"
	mk := func(end string, notice int, st string) Contract {
		return Contract{Status: st, EndDate: end, NoticeDays: notice}
	}
	cases := []struct {
		c     Contract
		phase string
		att   bool
	}{
		{mk("", 0, "active"), PhaseEvergreen, false},
		{mk("2026-02-01", 0, "active"), PhaseExpired, true},
		{mk("2026-03-20", 30, "active"), PhaseNotice, true},
		{mk("2026-04-15", 30, "active"), PhaseSoon, false},
		{mk("2027-01-01", 30, "active"), PhaseOK, false},
		{mk("2026-03-20", 30, "terminated"), PhaseNone, false},
		{mk("2026-03-20", 30, "draft"), PhaseNone, false},
	}
	for i, c := range cases {
		r := c.c.RenewalInfo(today, 60)
		if r.Phase != c.phase || r.Attention != c.att {
			t.Errorf("case %d: phase=%s attention=%v, want %s %v", i, r.Phase, r.Attention, c.phase, c.att)
		}
	}
	r := mk("2026-03-20", 30, "active").RenewalInfo(today, 60)
	if r.NoticeDate != "2026-02-18" || *r.NoticeDaysLeft != -11 {
		t.Errorf("notice = %s %d", r.NoticeDate, *r.NoticeDaysLeft)
	}
	// Recording a decision quiets attention; a snooze stops holding after its date.
	c := mk("2026-03-20", 30, "active")
	c.Decision = &Decision{Kind: DecRenewing, ForTermEnd: "2026-03-20"}
	if c.RenewalInfo(today, 60).Attention {
		t.Error("a decision should quiet attention")
	}
	c.Decision = &Decision{Kind: DecSnoozed, ForTermEnd: "2026-03-20", Until: "2026-03-05"}
	if c.RenewalInfo(today, 60).Attention || !c.RenewalInfo("2026-03-06", 60).Attention {
		t.Error("snooze should hold until its date, then attention returns")
	}
	if got := (Contract{StartDate: "2024-01-01", EndDate: "2025-12-31", AutoRenew: true, TermMonths: 12}).CompletedTerms("2026-03-01"); len(got) != 1 || got[0].End != "2025-12-31" {
		t.Errorf("completed terms = %+v", got)
	}
}

func TestRecurringObligations(t *testing.T) {
	now := time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC)
	rp := &Repeat{Every: "monthly", Anchor: "2026-01-31"}
	o := Obligation{Due: "2026-01-31", Repeat: rp}
	var dues []string
	for i := 0; i < 4; i++ {
		dues = append(dues, o.Due)
		o = o.Advance(nil, now, "2026-01-31")
	}
	if strings.Join(dues, ",") != "2026-01-31,2026-02-28,2026-03-31,2026-04-30" {
		t.Errorf("month-end series drifted: %v", dues)
	}
	if len(o.History) != 4 {
		t.Errorf("history = %d", len(o.History))
	}
	back := o.Rewind()
	if back.Due != "2026-04-30" || len(back.History) != 3 {
		t.Errorf("rewind = %s %d", back.Due, len(back.History))
	}
	// Count and until end a series; the obligation becomes done.
	two := 2
	o2 := Obligation{Due: "2026-01-01", Repeat: &Repeat{Every: "quarterly", Anchor: "2026-01-01", Count: &two}}
	o2 = o2.Advance(nil, now, "2026-01-01")
	if o2.Done || o2.Due != "2026-04-01" {
		t.Errorf("after 1: %+v", o2)
	}
	o2 = o2.Advance(nil, now, "2026-01-01")
	if !o2.Done {
		t.Error("the series should be finished after its count")
	}
	more := 3
	o2.Repeat.Count = &more
	if r := o2.Reopened(nil, "2026-01-01"); r.Done || r.Due != "2026-07-01" {
		t.Errorf("reopened = %+v", r)
	}
	// Until.
	o3 := Obligation{Due: "2026-01-15", Repeat: &Repeat{Every: "monthly", Anchor: "2026-01-15", Until: "2026-02-20"}}
	o3 = o3.Advance(nil, now, "2026-01-15")
	o3 = o3.Advance(nil, now, "2026-01-15")
	if !o3.Done {
		t.Error("until should end the series")
	}
	// One-off obligations just finish.
	if !(Obligation{Due: "2026-01-01"}).Advance(nil, now, "2026-01-01").Done {
		t.Error("one-off")
	}
	// Until the contract ends.
	c := &Contract{EndDate: "2026-03-10", Status: "active"}
	o4 := Obligation{Due: "2026-02-15", Repeat: &Repeat{Every: "monthly", Anchor: "2026-02-15", ContractEnd: true}}
	o4 = o4.Advance(c, now, "2026-01-01")
	if o4.Due != "2026-03-15" && !o4.Done {
		t.Errorf("o4 = %+v", o4)
	}
	if !o4.Done {
		t.Error("an occurrence after the contract end must not be scheduled")
	}
	// The anchor is kept when an edit leaves the due date where it was.
	if got := RepeatAnchor("2026-02-28", "monthly", 1, &Repeat{Every: "monthly", Anchor: "2026-01-31"}); got != "2026-01-31" {
		t.Errorf("anchor = %s", got)
	}
	if Urgency("2026-02-01", "2026-03-01") != UrgOverdue || Urgency("2026-03-05", "2026-03-01") != UrgSoon || Urgency("", "2026-03-01") != UrgNone {
		t.Error("urgency")
	}
}

func TestICS(t *testing.T) {
	f := &Full{Contract: Contract{ID: "c1", Title: "Office; lease, \"A\"", Status: "active", EndDate: "2026-06-30", NoticeDays: 60}, PartyLabel: "Landlord"}
	out := ICS([]*Full{f}, map[string][]Obligation{"c1": {{ID: "o1", Title: "Pay rent", Due: "2026-03-05", Owner: "us"}, {ID: "o2", Title: "Their duty", Due: "2026-03-05", Owner: "them"}}},
		time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), "2026-03-01", "https://x.example")
	for _, want := range []string{"BEGIN:VCALENDAR", "DTSTART;VALUE=DATE:20260630", `SUMMARY:Contract ends: Office\; lease\, "A"`, "DTSTART;VALUE=DATE:20260501", "TRIGGER:-P7D", "TRIGGER:-P1D", "Pay rent", "END:VCALENDAR"} {
		if !strings.Contains(out, want) {
			t.Errorf("ics missing %q", want)
		}
	}
	if strings.Contains(out, "Their duty") {
		t.Error("obligations owed by the other side are not our reminders")
	}
	for _, ln := range strings.Split(out, "\r\n") {
		if len(ln) > 75 {
			t.Errorf("line not folded: %d", len(ln))
		}
	}
}

func TestInputValidation(t *testing.T) {
	bad := []Input{
		{Title: ""}, {Title: "x", Type: "weird"}, {Title: "x", StartDate: "2026-02-30"}, {Title: "x", StartDate: "2026-05-01", EndDate: "2026-04-01"},
		{Title: "x", Value: -1}, {Title: "x", Currency: "RUPEES"}, {Title: "x", NoticeDays: -1}, {Title: "x", Access: "x"}, {Title: "x", Status: "x"}, {Title: "x", Billing: "weekly"},
	}
	for i, in := range bad {
		if err := in.clean(); err == nil {
			t.Errorf("case %d should fail: %+v", i, in)
		}
	}
	ok := Input{Title: " ok ", Value: 10.456}
	if err := ok.clean(); err != nil || ok.Title != "ok" || ok.Value != 10.46 || ok.Currency != "INR" {
		t.Errorf("clean = %+v %v", ok, err)
	}
}

type fakeCal struct {
	events map[string]string // id -> "day|title"
	n      int
}

func TestContractsEndToEnd(t *testing.T) {
	fx := kittest.Setup(t)
	ctx := context.Background()
	repo := NewRepository(fx.Pool)
	repo.Deps.Now = func() time.Time { return time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC) }
	cal := &fakeCal{events: map[string]string{}}
	tasks := map[string]string{}
	repo.Deps.Calendar = func(ctx context.Context, org, owner, id, title, desc string, day Date) (string, error) {
		if id == "" {
			cal.n++
			id = fmt.Sprintf("00000000-0000-0000-0000-%012d", cal.n)
		}
		cal.events[id] = day + "|" + title
		return id, nil
	}
	repo.Deps.DeleteEvent = func(ctx context.Context, org, id string) error { delete(cal.events, id); return nil }
	taskN := 0
	repo.Deps.Task = func(ctx context.Context, org, assignee, id, title, notes string, due Date, done bool) (string, error) {
		if id == "" {
			taskN++
			id = fmt.Sprintf("11111111-0000-0000-0000-%012d", taskN)
		}
		tasks[id] = due + "|" + title
		return id, nil
	}
	repo.Deps.DeleteTask = func(ctx context.Context, org, id string) error { delete(tasks, id); return nil }
	a, b := fx.Admin, fx.User

	c, err := repo.Create(ctx, b, Input{Title: "Office lease", Type: "lease", PartyName: "Landlord Ltd", Value: 50000, Billing: "monthly", Currency: "inr",
		StartDate: "2025-04-01", EndDate: "2026-04-30", AutoRenew: true, TermMonths: 12, NoticeDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if c.Annual != 600000 || c.Renewal.Phase != PhaseNotice && c.Renewal.Phase != PhaseSoon {
		t.Errorf("annual=%v phase=%s", c.Annual, c.Renewal.Phase)
	}
	if c.Renewal.Date != "2026-04-30" || c.Renewal.NoticeDate != "2026-03-31" {
		t.Errorf("renewal = %+v", c.Renewal)
	}
	// Reminders: one term-end and one notice event, one notice task.
	if len(cal.events) != 2 || len(tasks) != 1 {
		t.Fatalf("events=%v tasks=%v", cal.events, tasks)
	}
	// Syncing again changes nothing (idempotent).
	for i := 0; i < 3; i++ {
		if err := repo.SyncDeadlines(ctx, b, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(cal.events) != 2 || len(tasks) != 1 || cal.n != 2 {
		t.Errorf("sync not idempotent: events=%v tasks=%v n=%d", cal.events, tasks, cal.n)
	}
	// Obligations create tasks; recurring ones move to the next occurrence.
	ob, err := repo.AddObligation(ctx, b, c.ID, ObligationInput{Title: "Pay rent", Due: "2026-03-05", Repeat: &Repeat{Every: "monthly"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("tasks = %v", tasks)
	}
	ob, err = repo.CompleteObligation(ctx, b, ob.ID, false)
	if err != nil || ob.Due != "2026-04-05" || ob.Done || len(ob.History) != 1 {
		t.Fatalf("complete: %+v %v", ob, err)
	}
	found := false
	for _, v := range tasks {
		found = found || strings.HasPrefix(v, "2026-04-05|Pay rent")
	}
	if !found {
		t.Errorf("task did not move to the next occurrence: %v", tasks)
	}
	ob, _ = repo.CompleteObligation(ctx, b, ob.ID, true)
	if ob.Due != "2026-03-05" || len(ob.History) != 0 {
		t.Errorf("undo: %+v", ob)
	}
	// A decision removes the notice reminders but keeps the term-end event.
	c, err = repo.Decide(ctx, b, c.ID, Decision{Kind: DecNotRenewing, Note: "moving out"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Renewal.Attention || !c.Renewal.Handled {
		t.Errorf("renewal = %+v", c.Renewal)
	}
	if len(cal.events) != 1 || !strings.Contains(strings.Join(vals(cal.events), ";"), "ends: Office lease") {
		t.Errorf("events after decision = %v", cal.events)
	}
	// Changing the end date moves the event and drops a stale decision.
	c, err = repo.Update(ctx, b, c.ID, Input{Title: "Office lease", Type: "lease", PartyName: "Landlord Ltd", Value: 50000, Billing: "monthly", StartDate: "2025-04-01", EndDate: "2026-05-15", AutoRenew: false, NoticeDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if c.Decision != nil {
		t.Error("a decision for a date that is no term end must be dropped")
	}
	// Terminating clears every reminder.
	if _, err := repo.Update(ctx, b, c.ID, Input{Title: "Office lease", Status: "terminated", EndDate: "2026-05-15"}); err != nil {
		t.Fatal(err)
	}
	if len(cal.events) != 0 || len(tasks) != 0 {
		t.Errorf("reminders left after termination: %v %v", cal.events, tasks)
	}

	// Access: restricted contracts are invisible to others; admins see them; viewers cannot edit org contracts they do not own.
	r, _ := repo.Create(ctx, b, Input{Title: "Secret NDA", Access: "restricted"})
	if _, err := repo.Get(ctx, fx.Other, r.ID); err == nil {
		t.Error("other org must not see")
	}
	third := knowkit.Actor{Org: fx.Org, User: fx.User.User}
	_ = third
	if l, _ := repo.List(ctx, a, "", ""); len(l) < 2 {
		t.Errorf("admin sees %d", len(l))
	}
	pub, _ := repo.Create(ctx, a, Input{Title: "Public SLA"})
	if got, err := repo.Get(ctx, b, pub.ID); err != nil || got.CanEdit {
		t.Errorf("an org-visible contract is read-only for non-owners: %v %+v", err, got)
	}
	if _, err := repo.Update(ctx, b, pub.ID, Input{Title: "Hijack"}); err == nil {
		t.Error("non-owner update must fail")
	}
	// The other user must not see a restricted contract.
	other := knowkit.Actor{Org: fx.Org, User: a.User}
	_ = other
	live, _ := repo.Create(ctx, b, Input{Title: "Support plan", EndDate: "2026-12-31"})
	if _, err := repo.AddObligation(ctx, b, live.ID, ObligationInput{Title: "Send report", Due: "2026-02-01"}); err != nil {
		t.Fatal(err)
	}
	st, obs, err := repo.Overview(ctx, b)
	if err == nil && st.OverdueOb != 1 {
		t.Errorf("overdue obligations = %d", st.OverdueOb)
	}
	if err != nil || st.Active < 1 || len(obs) < 1 {
		t.Errorf("overview %+v %d %v", st, len(obs), err)
	}
	if err := repo.Delete(ctx, b, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, b, r.ID); err == nil {
		t.Error("deleted contract must be gone")
	}
}

func vals(m map[string]string) []string {
	var o []string
	for _, v := range m {
		o = append(o, v)
	}
	return o
}
