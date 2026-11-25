package invoicing

import (
	"testing"
	"time"
)

func TestInstalmentsFillInDateOrderAndSumToTotal(t *testing.T) {
	sched := []Schedule{{Label: "final", DueDate: "2026-07-01", Amount: 33.33}, {Label: "deposit", DueDate: "2026-05-01", Amount: 33.33}, {Label: "mid", DueDate: "2026-06-01", Amount: 33.33}}
	rows := Instalments(100, 40, sched)
	if rows[0].Label != "deposit" || rows[1].Label != "mid" || rows[2].Label != "final" {
		t.Fatalf("order %+v", rows)
	}
	var sum float64
	for _, r := range rows {
		sum += r.Amount
	}
	if Round2(sum) != 100 || rows[2].Amount != 33.34 {
		t.Fatalf("rows must add to the total, the last takes the remainder: %+v", rows)
	}
	if rows[0].Open != 0 || rows[1].Paid != 6.67 || rows[1].Open != 26.66 || rows[2].Paid != 0 {
		t.Fatalf("payment fills in date order: %+v", rows)
	}
}

func TestInstalmentsClampOverAsk(t *testing.T) {
	rows := Instalments(100, 0, []Schedule{{DueDate: "2026-05-01", Amount: 80}, {DueDate: "2026-06-01", Amount: 80}, {DueDate: "2026-07-01", Amount: 80}})
	if rows[0].Amount != 80 || rows[1].Amount != 20 || rows[2].Amount != 0 {
		t.Fatalf("%+v", rows)
	}
	if Instalments(100, 0, nil) != nil {
		t.Fatal("no schedule, no rows")
	}
}

func planDoc(applied float64) Document {
	d := Document{Kind: "invoice", Status: "sent", Total: 300, IssueDate: "2026-01-01", DueDate: "2026-04-01", Currency: "INR",
		Schedule: []Schedule{{Label: "1", DueDate: "2026-02-01", Amount: 100}, {Label: "2", DueDate: "2026-03-01", Amount: 100}, {Label: "3", DueDate: "2026-12-01", Amount: 100}},
		Payments: []Payment{}}
	if applied > 0 {
		d.Payments = []Payment{{Kind: "payment", Amount: applied}}
	}
	d.derive(time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC))
	return d
}

func TestPlanOverdueIsEarliestOpenInstalmentOnly(t *testing.T) {
	now := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	d := planDoc(0)
	if d.DisplayStatus != "overdue" || d.EffectiveDue != "2026-02-01" || d.OverdueAmount != 200 || d.Balance != 300 {
		t.Fatalf("unpaid plan: %s %s %v", d.DisplayStatus, d.EffectiveDue, d.OverdueAmount)
	}
	d = planDoc(100) // first instalment paid, second (March 1) is the late one
	if d.DisplayStatus != "overdue" || d.EffectiveDue != "2026-03-01" || d.OverdueAmount != 100 {
		t.Fatalf("one paid: %s %s %v", d.DisplayStatus, d.EffectiveDue, d.OverdueAmount)
	}
	d = planDoc(200) // only the December instalment is left: not late
	if d.DisplayStatus != "partial" || d.OverdueAmount != 0 {
		t.Fatalf("up to date: %s %v", d.DisplayStatus, d.OverdueAmount)
	}
	// aging ages each open instalment by its own date and keeps the balance whole
	parts := AgingParts(planDoc(0), now)
	var sum float64
	for _, p := range parts {
		sum += p.Amount
	}
	if len(parts) != 3 || sum != 300 || ageBucket(parts[0].Days) != 2 || ageBucket(parts[1].Days) != 1 || ageBucket(parts[2].Days) != 0 {
		t.Fatalf("parts %+v", parts)
	}
}

func TestNoScheduleBehavesAsBefore(t *testing.T) {
	d := Document{Kind: "invoice", Status: "sent", Total: 100, DueDate: "2026-03-01", Payments: []Payment{}}
	d.derive(time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC))
	if d.DisplayStatus != "overdue" || d.OverdueAmount != 100 || d.EffectiveDue != "2026-03-01" || len(d.Instalments) != 0 {
		t.Fatalf("%+v", d)
	}
}
