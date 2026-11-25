package pos

import "testing"

func near(a, b float64) bool { return a-b < 0.0051 && b-a < 0.0051 }

func TestComputeTotalsExclusiveTax(t *testing.T) {
	r := ComputeTotals([]CalcLine{
		{Qty: 2, UnitPrice: 10, TaxRate: 10},
		{Qty: 1, UnitPrice: 5, DiscountKind: "percent", DiscountValue: 20},
	}, "", 0)
	if !near(r.Subtotal, 25) || !near(r.DiscountTotal, 1) || !near(r.TaxTotal, 2) || !near(r.Total, 26) {
		t.Fatalf("got %+v", r)
	}
}

func TestComputeTotalsInclusiveTax(t *testing.T) {
	r := ComputeTotals([]CalcLine{{Qty: 1, UnitPrice: 118, TaxRate: 18, TaxInclusive: true}}, "", 0)
	if !near(r.Total, 118) || !near(r.TaxTotal, 18) {
		t.Fatalf("got %+v", r)
	}
}

func TestOrderDiscountAllocation(t *testing.T) {
	r := ComputeTotals([]CalcLine{
		{Qty: 1, UnitPrice: 10},
		{Qty: 1, UnitPrice: 10},
		{Qty: 1, UnitPrice: 10},
	}, "amount", 10)
	if !near(r.Total, 20) || !near(r.DiscountTotal, 10) {
		t.Fatalf("got %+v", r)
	}
	var sum float64
	for _, l := range r.Lines {
		sum += l.DiscountAmount
	}
	if !near(sum, 10) {
		t.Fatalf("allocated %v", sum)
	}
}

func TestDiscountCapped(t *testing.T) {
	r := ComputeTotals([]CalcLine{{Qty: 1, UnitPrice: 5, DiscountKind: "amount", DiscountValue: 50}}, "percent", 150)
	if !near(r.Total, 0) {
		t.Fatalf("got %+v", r)
	}
}

func TestAllocateTenders(t *testing.T) {
	a, change, ok := AllocateTenders(26, []Tender{{"cash", 20}, {"card", 10}})
	if !ok || !near(change, 4) || len(a) != 2 || a[0].Method != "card" || !near(a[1].Amount, 16) {
		t.Fatalf("got %+v %v %v", a, change, ok)
	}
	if _, _, ok := AllocateTenders(26, []Tender{{"cash", 20}}); ok {
		t.Fatal("underpaid should fail")
	}
	if _, _, ok := AllocateTenders(10, []Tender{{"card", 12}}); ok {
		t.Fatal("card overpay should fail")
	}
}

func TestRefundAmount(t *testing.T) {
	// 3 units for 10.00: 1 + 1 + last 1 sums exactly.
	a := RefundAmount(10, 3, 0, 0, 1)
	b := RefundAmount(10, 3, 1, a, 1)
	c := RefundAmount(10, 3, 2, a+b, 1)
	if !near(a+b+c, 10) {
		t.Fatalf("%v %v %v", a, b, c)
	}
}
