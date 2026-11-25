package invoicing

import (
	"context"
	"testing"
)

func TestProgressArithmetic(t *testing.T) {
	lines := []Line{{Ref: "l1", Qty: 10}, {Ref: "l2", Qty: 4}}
	ps := Progress(lines,
		[]QtyRef{{"l1", 6}, {"l1", 1}},      // dispatched 7 of l1
		[]QtyRef{{"l1", 2}, {"l2", 4}},      // drafts claim 2 of l1, all of l2
		[]QtyRef{{"l1", 5}, {"l2", 1}})      // invoiced 5 of l1, 1 of l2
	l1, l2 := ps[0], ps[1]
	if l1.Delivered != 7 || l1.Packed != 2 || l1.ToPack != 1 || l1.ToInvoice != 5 || l1.Billable != 2 {
		t.Fatalf("l1: %+v", l1)
	}
	if l2.Delivered != 0 || l2.ToPack != 0 || l2.Billable != 0 || l2.ToInvoice != 3 {
		t.Fatalf("l2: %+v", l2)
	}
	if f, b := orderStates(ps); f != "partially_delivered" || b != "partially_invoiced" {
		t.Fatalf("states %s %s", f, b)
	}
	full := Progress(lines, []QtyRef{{"l1", 10}, {"l2", 4}}, nil, []QtyRef{{"l1", 10}, {"l2", 4}})
	if f, b := orderStates(full); f != "delivered" || b != "invoiced" {
		t.Fatalf("states %s %s", f, b)
	}
	// Over-billing (invoice edited after the fact) never shows negative remainders.
	over := Progress(lines[:1], nil, nil, []QtyRef{{"l1", 12}})
	if over[0].ToInvoice != 0 {
		t.Fatalf("%+v", over[0])
	}
}

func TestCheckQty(t *testing.T) {
	limit := map[string]float64{"l1": 3, "l2": 0}
	if err := CheckQty([]QtyRef{{"l1", 3}}, limit, "x"); err != nil {
		t.Fatal(err)
	}
	for name, req := range map[string][]QtyRef{
		"over":    {{"l1", 3.01}},
		"nothing": {{"l1", 0}},
		"unknown": {{"zz", 1}},
		"empty":   nil,
		"neg":     {{"l1", -1}},
		"dup":     {{"l1", 1}, {"l1", 1}},
		"zero":    {{"l2", 1}},
	} {
		if CheckQty(req, limit, "x") == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

func TestOrderPartialDeliveryAndInvoicing(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	saveGSTSettings(t, repo, org)
	est, err := repo.SaveDocument(ctx, org, user, "", DocInput{Kind: "estimate", ClientName: "Acme", PlaceOfSupply: "29",
		Lines: []Line{{Description: "Widget", Qty: 10, Price: 100, TaxRate: 18, ProductID: "p1", HSNSAC: "8479"}, {Description: "Setup", Qty: 1, Price: 500, TaxRate: 18}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.OrderFromEstimate(ctx, org, user, est.ID); err == nil {
		t.Fatal("a draft estimate must not become an order")
	}
	repo.MarkSent(ctx, org, est.ID)
	o, err := repo.OrderFromEstimate(ctx, org, user, est.ID)
	if err != nil || o.Number != "SO-0001" || o.Status != "confirmed" || o.Total != 1770 || len(o.Lines) != 2 || o.Lines[0].Ref != "l1" {
		t.Fatalf("order: %v %+v", err, o)
	}
	o2, err := repo.OrderFromEstimate(ctx, org, user, est.ID)
	if err != nil || o2.ID != o.ID {
		t.Fatalf("idempotent: %v", err)
	}
	e2, _ := repo.GetDocument(ctx, org, est.ID)
	if e2.Status != "converted" || e2.SalesOrderID != o.ID {
		t.Fatalf("estimate: %+v", e2)
	}
	// Deliver 6 of 10, then try to deliver 5 more: refused, 4 left.
	d1, err := repo.CreateDelivery(ctx, org, user, o.ID, DeliveryInput{Lines: []QtyRef{{"l1", 6}}, Carrier: "DTDC"})
	if err != nil || d1.Number != "DN-0001" || len(d1.Lines) != 1 || d1.Lines[0].HSNSAC != "8479" {
		t.Fatalf("d1: %v %+v", err, d1)
	}
	if _, err := repo.CreateDelivery(ctx, org, user, o.ID, DeliveryInput{Lines: []QtyRef{{"l1", 5}}}); err == nil {
		t.Fatal("a draft note already claims 6; 5 more exceeds the order")
	}
	if _, err := repo.MarkDispatched(ctx, org, d1.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CancelDelivery(ctx, org, d1.ID); err == nil {
		t.Fatal("a dispatched note cannot be cancelled")
	}
	// Bill what was delivered: 6 x 100 + 18% = 708.
	inv, err := repo.InvoiceOrder(ctx, org, user, o.ID, OrderInvoiceInput{Mode: "delivered"})
	if err != nil || inv.Total != 708 || inv.SalesOrderID != o.ID || !inv.StockDeducted || len(inv.Lines) != 1 || inv.Lines[0].SOLine != "l1" {
		t.Fatalf("invoice: %v %+v", err, inv)
	}
	if _, err := repo.InvoiceOrder(ctx, org, user, o.ID, OrderInvoiceInput{Mode: "delivered"}); err == nil {
		t.Fatal("nothing delivered is left to bill")
	}
	// Explicit partial: 5 of the 10 widgets are already... 6 billed, so 5 more is too many (only 4 left).
	if _, err := repo.InvoiceOrder(ctx, org, user, o.ID, OrderInvoiceInput{Lines: []QtyRef{{"l1", 5}}}); err == nil {
		t.Fatal("over-billing must be refused")
	}
	rest, err := repo.InvoiceOrder(ctx, org, user, o.ID, OrderInvoiceInput{Mode: "remaining"})
	if err != nil || rest.Total != 4*100*1.18+590 {
		t.Fatalf("remaining: %v %+v", err, rest)
	}
	ord, _ := repo.GetOrder(ctx, org, o.ID)
	if ord.Billing != "invoiced" || ord.Fulfilment != "partially_delivered" || len(ord.Invoices) != 2 {
		t.Fatalf("order state: %+v", ord)
	}
	// Total billed equals the order total to the paisa.
	if Round2(inv.Total+rest.Total) != ord.Total {
		t.Fatalf("billed %v vs order %v", inv.Total+rest.Total, ord.Total)
	}
	// Lines are frozen once documents exist.
	ord2, err := repo.SaveOrder(ctx, org, user, o.ID, OrderInput{ClientName: "Acme", Lines: []Line{{Description: "x", Qty: 1, Price: 1}}, Notes: "n"})
	if err != nil || len(ord2.Lines) != 2 || ord2.Notes != "n" {
		t.Fatalf("frozen lines: %v %+v", err, ord2)
	}
}
