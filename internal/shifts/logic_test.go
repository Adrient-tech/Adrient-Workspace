package shifts

import "testing"

func sh(id, date, start, end, emp string) Shift {
	return Shift{ID: id, Date: date, Start: start, End: end, Role: "Cashier", EmployeeID: emp}
}

func TestOvernightDurationAndNet(t *testing.T) {
	if d, ok := DurationMinutes("22:00", "06:00"); !ok || d != 480 {
		t.Fatalf("overnight duration = %d,%v", d, ok)
	}
	if _, ok := DurationMinutes("09:00", "09:00"); ok {
		t.Fatal("equal times must be invalid")
	}
	s := sh("a", "2026-11-02", "09:00", "17:00", "e1")
	s.BreakMinutes = 30
	if NetMinutes(s) != 450 {
		t.Fatalf("net = %d", NetMinutes(s))
	}
}

func TestValidateShift(t *testing.T) {
	s := sh("a", "2026-11-02", "09:00", "10:00", "")
	s.BreakMinutes = 60
	if len(ValidateShift(s)) != 1 {
		t.Fatalf("break >= duration must be rejected: %v", ValidateShift(s))
	}
	s.BreakMinutes, s.Role = 0, " "
	if len(ValidateShift(s)) != 1 {
		t.Fatalf("role required: %v", ValidateShift(s))
	}
}

func TestConflictsAcrossMidnight(t *testing.T) {
	night := sh("n", "2026-11-02", "22:00", "06:00", "e1")
	early := sh("m", "2026-11-03", "05:00", "09:00", "e1")
	later := sh("l", "2026-11-03", "06:00", "09:00", "e1")
	if len(FindConflicts(early, []Shift{night}, "e1")) != 1 {
		t.Fatal("05:00 shift overlaps the night shift")
	}
	if len(FindConflicts(later, []Shift{night}, "e1")) != 0 {
		t.Fatal("06:00 shift starts as the night shift ends")
	}
	if len(FindConflicts(early, []Shift{night}, "")) != 0 {
		t.Fatal("open shifts never conflict")
	}
}

func TestAvailability(t *testing.T) {
	a := DefaultAvailability()
	a.Days[1].Off = true // Monday
	mon := sh("a", "2026-11-02", "09:00", "12:00", "e1")
	if AvailabilityStatus(mon, a) != AvailUnavailable {
		t.Fatal("day off must be unavailable")
	}
	tue := sh("b", "2026-11-03", "15:00", "19:00", "e1")
	if AvailabilityStatus(tue, a) != AvailPartial {
		t.Fatal("shift running past the window is partial")
	}
	a.Exceptions = []TimeOff{{From: "2026-11-03", To: "2026-11-03", Note: "dentist"}}
	if AvailabilityStatus(sh("c", "2026-11-03", "10:00", "11:00", ""), a) != AvailUnavailable {
		t.Fatal("time off blanks the day")
	}
}

func TestRules(t *testing.T) {
	r := Rules{MinRestHours: 11, MaxDailyHours: 10, MaxConsecutive: 3}
	prev := sh("p", "2026-11-02", "14:00", "22:00", "e1")
	next := sh("n", "2026-11-03", "06:00", "10:00", "e1")
	v := RuleViolations(next, "e1", []Shift{prev}, r, "Asha")
	if len(v) != 1 || v[0].Rule != "rest" {
		t.Fatalf("want a rest violation, got %+v", v)
	}
	var run []Shift
	for i, d := range []string{"2026-11-02", "2026-11-03", "2026-11-04"} {
		run = append(run, sh(string(rune('a'+i)), d, "09:00", "13:00", "e1"))
	}
	four := sh("z", "2026-11-05", "09:00", "13:00", "e1")
	v = RuleViolations(four, "e1", run, r, "Asha")
	if len(v) != 1 || v[0].Rule != "consecutive" {
		t.Fatalf("want consecutive violation, got %+v", v)
	}
}

func fp(f float64) *float64 { return &f }

func TestCheckAssignAndSuggest(t *testing.T) {
	staff := []Staff{
		{EmployeeID: "e1", Name: "Asha", Role: "Cashier", Active: true, Availability: DefaultAvailability(), TargetHours: fp(40)},
		{EmployeeID: "e2", Name: "Ben", Role: "Cook", Active: true, Availability: DefaultAvailability(), MaxHours: fp(4)},
		{EmployeeID: "e3", Name: "Cy", Role: "Cashier", Active: false, Availability: DefaultAvailability()},
	}
	open := sh("o", "2026-11-02", "09:00", "17:00", "")
	got := SuggestAssignees(open, staff, nil, Rules{}, 3)
	if len(got) != 1 || got[0].EmployeeID != "e1" {
		t.Fatalf("only Asha can take it (Ben over max, Cy inactive): %+v", got)
	}
	blocked, _ := CheckAssign(sh("x", "2026-11-02", "10:00", "12:00", "e1"), staff[0], []Shift{sh("y", "2026-11-02", "09:00", "11:00", "e1")}, Rules{})
	if blocked == "" {
		t.Fatal("overlap must block")
	}
	_, w := CheckAssign(open, staff[1], nil, Rules{})
	if len(w) != 1 {
		t.Fatalf("max hours warning expected, got %v", w)
	}
}

func TestPlanFillIsAwareOfEarlierProposals(t *testing.T) {
	staff := []Staff{{EmployeeID: "e1", Name: "Asha", Role: "Cashier", Active: true, Availability: DefaultAvailability()}}
	a := sh("a", "2026-11-02", "09:00", "13:00", "")
	b := sh("b", "2026-11-02", "12:00", "16:00", "")
	plan := PlanFill([]Shift{a, b}, staff, Rules{}, []string{"2026-11-02"})
	if len(plan) != 2 || plan[0].Proposal == nil || plan[1].Proposal != nil {
		t.Fatalf("second overlapping shift must stay open: %+v", plan)
	}
}

func TestCoverageGaps(t *testing.T) {
	reqs := []Coverage{{ID: "r", Role: "cashier", Days: []int{1}, Count: 2}}
	shifts := []Shift{sh("a", "2026-11-02", "09:00", "17:00", "e1")}
	g := CoverageGaps(shifts, reqs, []string{"2026-11-02", "2026-11-03"})
	if len(g) != 1 || g[0].Missing != 1 || g[0].Date != "2026-11-02" {
		t.Fatalf("gaps = %+v", g)
	}
}

func TestLaborCostOvertime(t *testing.T) {
	staff := []Staff{{EmployeeID: "e1", Name: "Asha", HourlyRate: fp(100), Active: true}}
	var shifts []Shift
	for i, d := range []string{"2026-11-02", "2026-11-03", "2026-11-04"} {
		shifts = append(shifts, sh(string(rune('a'+i)), d, "08:00", "18:00", "e1")) // 10h each
	}
	dates := []string{"2026-11-02", "2026-11-03", "2026-11-04"}
	c := LaborCost(shifts, staff, dates, CostOptions{ThresholdHours: 20, Multiplier: 1.5})
	// 30h, 10h over 20h at 1.5x: 20*100 + 10*150 = 3500
	if c.Cost != 3500 || c.OvertimeMinutes != 600 || c.RatedCount != 1 {
		t.Fatalf("cost = %+v", c)
	}
	plain := LaborCost(shifts, staff, dates, CostOptions{})
	if plain.Cost != 3000 {
		t.Fatalf("plain cost = %v", plain.Cost)
	}
}

func TestPaidMinutes(t *testing.T) {
	s := sh("a", "2026-11-02", "09:00", "17:00", "e1")
	s.BreakMinutes = 30
	if PaidMinutes(s, nil) != 450 {
		t.Fatal("scheduled net without attendance")
	}
	if PaidMinutes(s, &Attendance{Status: "absent"}) != 0 {
		t.Fatal("absent is unpaid")
	}
	if PaidMinutes(s, &Attendance{Status: "late", ClockIn: "09:15", ClockOut: "17:00"}) != 435 {
		t.Fatal("clocked time minus break")
	}
	if LateMinutes(s, "09:20") != 20 {
		t.Fatal("late minutes")
	}
}
