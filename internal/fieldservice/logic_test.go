package fieldservice

import (
	"reflect"
	"testing"
)

func TestVisitDates(t *testing.T) {
	cases := []struct {
		name string
		s    Schedule
		want []string
	}{
		{"one-off", Schedule{Start: "2026-11-02"}, []string{"2026-11-02"}},
		{"daily every 2, 3 visits", Schedule{Start: "2026-11-02", Repeat: "daily", Interval: 2, EndType: "count", EndCount: 3}, []string{"2026-11-02", "2026-11-04", "2026-11-06"}},
		{"weekly Mon+Thu", Schedule{Start: "2026-11-02", Repeat: "weekly", Weekdays: []int{4, 1}, EndType: "count", EndCount: 4}, []string{"2026-11-02", "2026-11-05", "2026-11-09", "2026-11-12"}},
		{"weekly skips days before the start", Schedule{Start: "2026-11-04", Repeat: "weekly", Weekdays: []int{1, 3}, EndType: "count", EndCount: 2}, []string{"2026-11-04", "2026-11-09"}},
		{"monthly clamps Jan 31", Schedule{Start: "2027-01-31", Repeat: "monthly", EndType: "count", EndCount: 3}, []string{"2027-01-31", "2027-02-28", "2027-03-31"}},
		{"monthly 2nd Tuesday", Schedule{Start: "2026-11-10", Repeat: "monthly", MonthlyBy: "weekday", EndType: "count", EndCount: 3}, []string{"2026-11-10", "2026-12-08", "2027-01-12"}},
		{"until date", Schedule{Start: "2026-11-02", Repeat: "weekly", EndType: "until", EndDate: "2026-11-20"}, []string{"2026-11-02", "2026-11-09", "2026-11-16"}},
		{"yearly", Schedule{Start: "2024-02-29", Repeat: "yearly", EndType: "count", EndCount: 3}, []string{"2024-02-29", "2025-02-28", "2026-02-28"}},
	}
	for _, c := range cases {
		if got := VisitDates(c.s); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	if VisitDates(Schedule{Start: "nope"}) != nil {
		t.Error("bad start gives no visits")
	}
	if n := len(VisitDates(Schedule{Start: "2026-01-01", Repeat: "daily", EndType: "until", EndDate: "2099-01-01"})); n != MaxVisits {
		t.Errorf("cap = %d", n)
	}
}

func TestCompute(t *testing.T) {
	items := []Item{
		{Name: "Mow", Qty: 2, UnitPrice: 50, UnitCost: 20, Taxable: true},
		{Name: "Fertiliser", Qty: 1, UnitPrice: 100, UnitCost: 60, Taxable: false},
		{Name: "Edging", Qty: 1, UnitPrice: 40, Optional: true},
	}
	t1 := Compute(items, &Discount{Kind: "pct", Value: 10}, 18)
	// subtotal 200, discount 20, taxable 100 -> 90 after discount share, tax 16.2, total 196.2, cost 100.
	if t1.Subtotal != 200 || t1.Discount != 20 || t1.Tax != 16.2 || t1.Total != 196.2 || t1.Cost != 100 || t1.Profit != 80 {
		t.Fatalf("totals = %+v", t1)
	}
	items[2].Selected = true
	if got := Compute(items, nil, 0); got.Subtotal != 240 || got.Total != 240 {
		t.Fatalf("selected add-on counts: %+v", got)
	}
	if got := Compute(items, &Discount{Kind: "amount", Value: 9999}, 18); got.Discount != 240 || got.Total != 0 {
		t.Fatalf("discount capped at the subtotal: %+v", got)
	}
}

func TestRequestFlowAndJobStatus(t *testing.T) {
	if !CanMoveRequest(ReqNew, ReqQuoted) || CanMoveRequest(ReqConverted, ReqNew) || !CanMoveRequest(ReqClosed, ReqNew) {
		t.Fatal("request transitions")
	}
	v := func(d, s string) VisitState { return VisitState{d, s} }
	today := "2026-11-10"
	if JobStatus("active", []VisitState{v("2026-11-12", "scheduled")}, false, today) != "active" {
		t.Fatal("future visit is active")
	}
	if JobStatus("active", []VisitState{v("2026-11-01", "scheduled")}, false, today) != "late" {
		t.Fatal("overdue visit is late")
	}
	if JobStatus("active", []VisitState{v("2026-11-01", "completed")}, false, today) != "requires_invoicing" {
		t.Fatal("done but not invoiced")
	}
	if JobStatus("active", []VisitState{v("2026-11-01", "completed")}, true, today) != "completed" {
		t.Fatal("done and invoiced")
	}
	if JobStatus("cancelled", nil, false, today) != "cancelled" {
		t.Fatal("cancelled")
	}
}
