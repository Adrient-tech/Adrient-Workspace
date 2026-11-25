package invoicing

import "testing"

func TestResolvePriceOrder(t *testing.T) {
	rows := []PriceRow{
		{ListID: "def", ProductID: "p1", MinQty: 1, Price: 100},
		{ListID: "def", ProductID: "p1", MinQty: 10, Price: 90},
		{ListID: "wholesale", ProductID: "p1", MinQty: 1, Price: 80},
		{ListID: "wholesale", ProductID: "p1", MinQty: 50, Price: 70},
		{ListID: "wholesale", ProductID: "p1", ContactID: "acme", MinQty: 1, Price: 65},
	}
	pc := PriceCtx{ContactID: "acme", Assigned: map[string]bool{"wholesale": true}, Default: map[string]bool{"def": true}}
	if p, s := ResolvePrice(120, "p1", 5, pc, rows); p != 65 || s != SrcCustomer {
		t.Fatalf("customer-specific wins: %v %v", p, s)
	}
	other := PriceCtx{ContactID: "bob", Assigned: map[string]bool{"wholesale": true}, Default: map[string]bool{"def": true}}
	if p, s := ResolvePrice(120, "p1", 5, other, rows); p != 80 || s != SrcAssigned {
		t.Fatalf("assigned list: %v %v", p, s)
	}
	if p, _ := ResolvePrice(120, "p1", 60, other, rows); p != 70 {
		t.Fatalf("quantity tier: %v", p)
	}
	none := PriceCtx{ContactID: "cy", Default: map[string]bool{"def": true}}
	if p, s := ResolvePrice(120, "p1", 12, none, rows); p != 90 || s != SrcDefault {
		t.Fatalf("default list tier: %v %v", p, s)
	}
	if p, s := ResolvePrice(120, "p2", 1, none, rows); p != 120 || s != SrcBase {
		t.Fatalf("unlisted product falls back to base: %v %v", p, s)
	}
	// A level whose tier is not reached (min_qty above qty) is skipped, not zero.
	tier := []PriceRow{{ListID: "w", ProductID: "p", MinQty: 10, Price: 5}}
	if p, s := ResolvePrice(9, "p", 3, PriceCtx{Assigned: map[string]bool{"w": true}}, tier); p != 9 || s != SrcBase {
		t.Fatalf("unreached tier: %v %v", p, s)
	}
}

func TestDiscountRules(t *testing.T) {
	rules := []DiscountRule{
		{Name: "Bulk 10", Scope: "line", Kind: "percent", Value: 10, MinQty: 10, Active: true},
		{Name: "Bulk 15", Scope: "line", Kind: "percent", Value: 15, MinQty: 50, Active: true},
		{Name: "Acme only", Scope: "line", Kind: "percent", Value: 20, ContactID: "acme", Active: true, Priority: 5},
		{Name: "Order 500 off", Scope: "order", Kind: "amount", Value: 500, MinAmount: 10000, Active: true},
		{Name: "Expired", Scope: "line", Kind: "percent", Value: 90, ValidTo: "2020-01-01", Active: true, Priority: 99},
		{Name: "Off", Scope: "line", Kind: "percent", Value: 90, Active: false, Priority: 99},
	}
	lines := []Line{{ProductID: "a", Qty: 12, Price: 100}, {ProductID: "b", Qty: 60, Price: 200}, {ProductID: "c", Qty: 1, Price: 50}}
	out, k, v, applied := ApplyDiscountRules(lines, rules, "bob", "2026-11-18")
	if out[0].DiscountPct != 10 || out[1].DiscountPct != 15 || out[2].DiscountPct != 0 {
		t.Fatalf("line discounts: %+v", out)
	}
	// subtotal = 12*100*.9 + 60*200*.85 + 50 = 1080 + 10200 + 50 >= 10000
	if k != "amount" || v != 500 || len(applied) != 3 {
		t.Fatalf("order rule: %v %v %+v", k, v, applied)
	}
	out, _, _, _ = ApplyDiscountRules(lines, rules, "acme", "2026-11-18")
	if out[2].DiscountPct != 20 {
		t.Fatalf("customer rule has priority and applies to a small line: %+v", out)
	}
	// The user's own bigger discount is kept.
	typed := []Line{{ProductID: "a", Qty: 12, Price: 100, DiscountPct: 25}}
	out, _, _, _ = ApplyDiscountRules(typed, rules, "bob", "2026-11-18")
	if out[0].DiscountPct != 25 {
		t.Fatalf("typed discount must stay: %+v", out)
	}
	if lines[0].DiscountPct != 0 {
		t.Fatal("input lines must not be mutated")
	}
}
