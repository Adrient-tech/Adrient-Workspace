package invoicing

import (
	"testing"
	"time"
)

func TestComputeDiscountAndTax(t *testing.T) {
	lines := []Line{
		{Qty: 2, Price: 100, TaxRate: 18},
		{Qty: 1, Price: 50, TaxRate: 0},
	}
	got := Compute(lines, "percent", 10)
	// sub 250, disc 25, tax = (200-20)*18% = 32.4
	if got.Subtotal != 250 || got.DiscountTotal != 25 || got.TaxTotal != 32.4 || got.Total != 257.4 {
		t.Fatalf("got %+v", got)
	}
	amt := Compute(lines, "amount", 1000) // clamped to subtotal
	if amt.Total != 0 || amt.DiscountTotal != 250 {
		t.Fatalf("clamp: %+v", amt)
	}
}

func TestLineDiscount(t *testing.T) {
	got := Compute([]Line{{Qty: 3, Price: 10, DiscountPct: 50, TaxRate: 10}}, "percent", 0)
	if got.Subtotal != 15 || got.TaxTotal != 1.5 || got.Total != 16.5 {
		t.Fatalf("got %+v", got)
	}
}

func TestStandAndStatus(t *testing.T) {
	today := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	past := today.AddDate(0, 0, -3)
	future := today.AddDate(0, 0, 5)
	s := Stand(100, []Payment{{Kind: "payment", Amount: 60}})
	if s.Balance != 40 {
		t.Fatal(s)
	}
	if st := DisplayStatus("invoice", "sent", &future, 100, s, today); st != "partial" {
		t.Fatal(st)
	}
	if st := DisplayStatus("invoice", "sent", &past, 100, s, today); st != "overdue" {
		t.Fatal(st)
	}
	s = Stand(100, []Payment{{Kind: "payment", Amount: 100}})
	if st := DisplayStatus("invoice", "sent", &past, 100, s, today); st != "paid" {
		t.Fatal(st)
	}
	s = Stand(100, []Payment{{Kind: "payment", Amount: 100}, {Kind: "refund", Amount: 100}})
	if s.Balance != 100 {
		t.Fatal(s)
	}
	s = Stand(100, []Payment{{Kind: "writeoff", Amount: 100}})
	if st := DisplayStatus("invoice", "sent", &future, 100, s, today); st != "written_off" {
		t.Fatal(st)
	}
	if st := DisplayStatus("invoice", "draft", &past, 100, Stand(100, nil), today); st != "draft" {
		t.Fatal(st)
	}
}

func TestNextDateClamps(t *testing.T) {
	d := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	if got := NextDate(d, "monthly"); got.Format("2006-01-02") != "2026-02-28" {
		t.Fatal(got)
	}
	if got := NextDate(d, "weekly"); got.Format("2006-01-02") != "2026-02-07" {
		t.Fatal(got)
	}
	if got := NextDate(d, "yearly"); got.Format("2006-01-02") != "2027-01-31" {
		t.Fatal(got)
	}
}

func TestToBase(t *testing.T) {
	r := map[string]float64{"USD": 83}
	if ToBase(2, "USD", "INR", r) != 166 || ToBase(5, "INR", "INR", r) != 5 || ToBase(5, "EUR", "INR", r) != 5 {
		t.Fatal("convert")
	}
}

func TestTaxInclusivePricing(t *testing.T) {
	// 3 x 11.90 incl. 19% VAT = 35.70 gross, 30.00 net, 5.70 tax
	got := Compute([]Line{{Qty: 3, Price: 11.90, TaxRate: 19, TaxInclusive: true}}, "percent", 0)
	if got.Subtotal != 30 || got.TaxTotal != 5.70 || got.Total != 35.70 {
		t.Fatalf("%+v", got)
	}
	got = Compute([]Line{{Qty: 1, Price: 119, TaxRate: 19, TaxInclusive: true}}, "percent", 10)
	if got.Subtotal != 100 || got.DiscountTotal != 10 || got.TaxTotal != 17.10 || got.Total != 107.10 {
		t.Fatalf("%+v", got)
	}
	for _, p := range []float64{0.99, 1.07, 3.33, 19.99, 100.01} {
		g := Compute([]Line{{Qty: 7, Price: p, TaxRate: 18, TaxInclusive: true}}, "percent", 0)
		if want := Round2(7 * p); g.Total != want || Round2(g.Subtotal-g.DiscountTotal+g.TaxTotal) != g.Total {
			t.Fatalf("price %v: %+v want total %v", p, g, want)
		}
	}
	if g := Compute([]Line{{Qty: 1, Price: 100, TaxRate: 10}}, "percent", 0); g.Total != 110 {
		t.Fatalf("%+v", g)
	}
	mixed := Compute([]Line{{Qty: 1, Price: 100, TaxRate: 10}, {Qty: 1, Price: 110, TaxRate: 10, TaxInclusive: true}}, "percent", 0)
	if mixed.Subtotal != 200 || mixed.TaxTotal != 20 || mixed.Total != 220 {
		t.Fatalf("%+v", mixed)
	}
	b := GST([]Line{{Qty: 1, Price: 118, TaxRate: 18, TaxInclusive: true}}, "percent", 0, "27", "27")
	if b.Taxable != 100 || b.TaxTotal != 18 || b.CGST != 9 || b.SGST != 9 {
		t.Fatalf("%+v", b)
	}
}
