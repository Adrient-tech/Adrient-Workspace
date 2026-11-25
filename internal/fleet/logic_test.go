package fleet

import "testing"

func TestVIN(t *testing.T) {
	// A well-known valid North American example.
	shape, check := ValidVIN("1M8GDM9AXKP042788")
	if !shape || !check {
		t.Fatalf("valid VIN rejected: %v %v", shape, check)
	}
	if shape, check = ValidVIN("1M8GDM9A1KP042788"); !shape || check {
		t.Fatal("wrong check digit is a warning, shape is fine")
	}
	for _, bad := range []string{"", "ABC", "1M8GDM9AXKP04278I", "1M8GDM9AXKP0427888"} {
		if ok, _ := ValidVIN(bad); ok {
			t.Errorf("%q must be rejected", bad)
		}
	}
	if CleanVIN(" 1m8-gdm 9axkp042788 ") != "1M8GDM9AXKP042788" {
		t.Fatal("clean")
	}
	if ModelYear("1M8GDM9AXKP042788") != 1989 {
		t.Fatalf("model year = %d", ModelYear("1M8GDM9AXKP042788"))
	}
}

func TestIntervals(t *testing.T) {
	if got := AddInterval("2027-01-31", Interval{Every: 1, Unit: "months"}, 1); got != "2027-02-28" {
		t.Fatalf("month clamp: %s", got)
	}
	if got := AddInterval("2024-02-29", Interval{Every: 1, Unit: "years"}, 1); got != "2025-02-28" {
		t.Fatalf("year clamp: %s", got)
	}
	if got := AddInterval("2026-11-30", Interval{Every: 2, Unit: "weeks"}, 1); got != "2026-12-14" {
		t.Fatalf("weeks: %s", got)
	}
	if DiffDays("2026-11-01", "2026-11-11") != 10 {
		t.Fatal("diff")
	}
}

func TestReminderTimeOrMeter(t *testing.T) {
	r := Reminder{Title: "Oil", Interval: Interval{Every: 6, Unit: "months"}, MeterInterval: 10000, LastDate: "2026-01-10", LastMeter: 50000}
	// Next: 2026-07-10 or 60000, whichever comes first.
	if s := r.StateAt(52000, "2026-03-01", 14, 500); s.Status != "upcoming" || s.NextDate != "2026-07-10" || *s.NextMeter != 60000 {
		t.Fatalf("upcoming %+v", s)
	}
	if s := r.StateAt(59700, "2026-03-01", 14, 500); s.Status != "due_soon" {
		t.Fatalf("meter due soon: %+v", s)
	}
	if s := r.StateAt(60001, "2026-03-01", 14, 500); s.Status != "overdue" {
		t.Fatalf("meter overdue: %+v", s)
	}
	if s := r.StateAt(52000, "2026-07-20", 14, 500); s.Status != "overdue" {
		t.Fatalf("date overdue: %+v", s)
	}
	r.SnoozedUntil = "2026-08-01"
	if s := r.StateAt(52000, "2026-07-20", 14, 500); s.Status != "snoozed" {
		t.Fatalf("snoozed: %+v", s)
	}
}

func TestReminderCompleteTracksCompliance(t *testing.T) {
	r := Reminder{Interval: Interval{Every: 3, Unit: "months"}, LastDate: "2026-01-01"}
	r = r.Complete("2026-04-10", 100, "2026-04-10", 14, 500) // due 2026-04-01 -> late
	r = r.Complete("2026-07-05", 200, "2026-07-05", 14, 500) // due 2026-07-10 -> on time
	if r.LastDate != "2026-07-05" || len(r.History) != 2 || r.History[0].OnTime || !r.History[1].OnTime || r.Compliance() != 0.5 {
		t.Fatalf("history %+v", r)
	}
	if next, _ := r.NextDue(); next != "2026-10-05" {
		t.Fatalf("next due %s", next)
	}
}

func TestRenewalRollsFromTheDueDate(t *testing.T) {
	rn := Renewal{DueDate: "2026-11-30", Repeat: Interval{Every: 1, Unit: "years"}}
	if st, d := rn.StateAt("2026-12-05", 14); st != "overdue" || *d != -5 {
		t.Fatalf("overdue %s %d", st, *d)
	}
	rn = rn.Complete("2026-12-05", 1200, "")
	if rn.DueDate != "2027-11-30" || rn.Done || len(rn.History) != 1 {
		t.Fatalf("rolled %+v", rn)
	}
	once := Renewal{DueDate: "2026-11-30"}.Complete("2026-11-29", 0, "")
	if !once.Done {
		t.Fatal("a one-off renewal is done")
	}
}

func TestCosts(t *testing.T) {
	if CostPerDistance(1500, 1000) != 1.5 || CostPerDistance(10, 0) != 0 {
		t.Fatal("cost per distance")
	}
	if FuelEconomy(1000, 1450, 30) != 15 || FuelEconomy(1000, 900, 30) != 0 {
		t.Fatal("fuel economy")
	}
}
