package inventory

import (
	"context"
	"strings"
	"testing"
)

func poLines(lines ...[2]any) []struct {
	ProductID   string   `json:"product_id"`
	Description string   `json:"description"`
	Qty         float64  `json:"qty"`
	UnitCost    *float64 `json:"unit_cost"`
	TaxPct      float64  `json:"tax_pct"`
} {
	var out []struct {
		ProductID   string   `json:"product_id"`
		Description string   `json:"description"`
		Qty         float64  `json:"qty"`
		UnitCost    *float64 `json:"unit_cost"`
		TaxPct      float64  `json:"tax_pct"`
	}
	for _, l := range lines {
		c := l[1].(float64)
		var x struct {
			ProductID   string   `json:"product_id"`
			Description string   `json:"description"`
			Qty         float64  `json:"qty"`
			UnitCost    *float64 `json:"unit_cost"`
			TaxPct      float64  `json:"tax_pct"`
		}
		x.ProductID, x.Qty, x.UnitCost = l[0].(string), 10, &c
		out = append(out, x)
	}
	return out
}

func grnLines(poLine string, qty float64, lot, exp string) []struct {
	POLineID  string   `json:"po_line_id"`
	ProductID string   `json:"product_id"`
	Qty       float64  `json:"qty"`
	UnitCost  *float64 `json:"unit_cost"`
	LotNo     string   `json:"lot_no"`
	Expiry    string   `json:"expiry"`
	Serials   []string `json:"serials"`
} {
	return []struct {
		POLineID  string   `json:"po_line_id"`
		ProductID string   `json:"product_id"`
		Qty       float64  `json:"qty"`
		UnitCost  *float64 `json:"unit_cost"`
		LotNo     string   `json:"lot_no"`
		Expiry    string   `json:"expiry"`
		Serials   []string `json:"serials"`
	}{{POLineID: poLine, Qty: qty, LotNo: lot, Expiry: exp}}
}

func TestPurchaseFlowInDB(t *testing.T) {
	pool, org, user := setupDB(t)
	r := NewRepository(pool)
	ctx := context.Background()
	locs, _ := r.ListLocations(ctx, org)
	main := locs[0].ID
	sup, err := r.SaveSupplier(ctx, org, "", Supplier{Name: "Acme Supply", Email: "orders@acme.test"})
	if err != nil {
		t.Fatal(err)
	}
	pid := mustProduct(t, r, org, user, "Milk", "MILK", 0)
	if _, err := r.SetProductSettings(ctx, org, pid, ProductSettings{ValuationMethod: "fifo", Tracking: "lot"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SaveSupplierPrice(ctx, org, "", SupplierPrice{SupplierID: sup.ID, ProductID: pid, MinQty: 1, UnitCost: 12}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SaveSupplierPrice(ctx, org, "", SupplierPrice{SupplierID: sup.ID, ProductID: pid, MinQty: 50, UnitCost: 10, LeadDays: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SavePurchaseSettings(ctx, org, PurchaseSettings{Currency: "INR", RequireApproval: true, UnderReceiptPolicy: "backorder", PriceTolerancePct: 2, TaxSplit: "cgst_sgst", OverReceiptPct: 5}); err != nil {
		t.Fatal(err)
	}

	// Price list is used when no price is typed: 100 units hit the 50+ tier.
	po, err := r.CreatePO(ctx, org, user, POInput{SupplierID: sup.ID, Lines: []struct {
		ProductID   string   `json:"product_id"`
		Description string   `json:"description"`
		Qty         float64  `json:"qty"`
		UnitCost    *float64 `json:"unit_cost"`
		TaxPct      float64  `json:"tax_pct"`
	}{{ProductID: pid, Qty: 100, TaxPct: 12}}})
	if err != nil {
		t.Fatal(err)
	}
	if po.State != "draft" || po.Lines[0].UnitCost != 10 || po.Subtotal != 1000 || po.TaxTotal != 120 || po.Total != 1120 {
		t.Fatalf("po %+v", po)
	}
	// State machine: cannot receive a draft, cannot send before approval.
	if _, err := r.CreateGRN(ctx, org, user, GRNInput{POID: po.ID, Lines: grnLines(po.Lines[0].ID, 10, "L1", "2099-01-01")}); err == nil {
		t.Fatal("received against a draft")
	}
	if _, err := r.MarkPOSent(ctx, org, po.ID, "x@y.z"); err == nil {
		t.Fatal("sent a draft")
	}
	if po, err = r.ApprovePO(ctx, org, user, po.ID); err != nil || po.State != "approved" {
		t.Fatalf("approve: %v %s", err, po.State)
	}
	if po, err = r.MarkPOSent(ctx, org, po.ID, "orders@acme.test"); err != nil || po.State != "sent" {
		t.Fatalf("send: %v", err)
	}
	if po, err = r.AcknowledgePO(ctx, org, po.ID, AckInput{Ref: "ACK-77", PromisedDate: "2099-02-01"}); err != nil || po.State != "acknowledged" || po.AckRef != "ACK-77" {
		t.Fatalf("ack: %v %+v", err, po)
	}
	if _, err := r.UpdatePO(ctx, org, po.ID, POInput{SupplierID: sup.ID}); err == nil {
		t.Fatal("edited a confirmed order")
	}

	// Part receipt with landed cost: 40 units, freight 80 spread over them.
	pl := po.Lines[0].ID
	g1, err := r.CreateGRN(ctx, org, user, GRNInput{POID: po.ID, LandedCost: 80, LandedMethod: "qty", Lines: grnLines(pl, 40, "LOT-A", "2099-06-01")})
	if err != nil {
		t.Fatal(err)
	}
	if g1.Lines[0].LandedAlloc != 2 {
		t.Fatalf("landed %v", g1.Lines[0].LandedAlloc)
	}
	po, _ = r.GetPO(ctx, org, po.ID)
	if po.State != "partial" || po.Lines[0].QtyReceived != 40 {
		t.Fatalf("after part receipt %s %v", po.State, po.Lines[0].QtyReceived)
	}
	if q, v := valueOf(t, r, org, pid); q != 40 || v != 480 { // 40 * (10 + 2)
		t.Fatalf("valuation after landed receipt qty=%v value=%v", q, v)
	}
	// A lot number is required for lot-tracked goods.
	if _, err := r.CreateGRN(ctx, org, user, GRNInput{POID: po.ID, Lines: grnLines(pl, 5, "", "")}); err == nil || !strings.Contains(err.Error(), "lot number") {
		t.Fatalf("lot required: %v", err)
	}
	// Over-receipt: 60 left, 5% tolerance on 100 allows 5 extra, so 66 is refused and 65 passes.
	if _, err := r.CreateGRN(ctx, org, user, GRNInput{POID: po.ID, Lines: grnLines(pl, 66, "LOT-B", "2099-07-01")}); err == nil {
		t.Fatal("accepted over-receipt beyond tolerance")
	}

	// Bill 40 received units: matches. Billing 70 does not.
	bill, err := r.CreateBill(ctx, org, user, BillInput{POID: po.ID, BillRef: "INV-A-1", Lines: []struct {
		POLineID    string  `json:"po_line_id"`
		ProductID   string  `json:"product_id"`
		Description string  `json:"description"`
		Qty         float64 `json:"qty"`
		UnitCost    float64 `json:"unit_cost"`
		TaxPct      float64 `json:"tax_pct"`
	}{{POLineID: pl, Qty: 70, UnitCost: 10, TaxPct: 12}}})
	if err != nil {
		t.Fatal(err)
	}
	if bill.MatchStatus != "exceptions" || len(bill.MatchDetail) != 1 || bill.MatchDetail[0].Kind != "qty_over_received" {
		t.Fatalf("match %+v", bill.MatchDetail)
	}
	if _, err := r.PostBill(ctx, org, user, bill.ID, ""); err == nil || !strings.Contains(err.Error(), "three-way match") {
		t.Fatalf("posted a bill that does not match: %v", err)
	}
	if _, err := r.CreateBill(ctx, org, user, BillInput{POID: po.ID, BillRef: "inv-a-1", Lines: bill.toInput()}); err == nil {
		t.Fatal("duplicate supplier invoice number accepted")
	}
	draft, _ := r.BillDraftFromPO(ctx, org, po.ID)
	if len(draft.Lines) != 1 || draft.Lines[0].Qty != 40 {
		t.Fatalf("bill draft %+v", draft.Lines)
	}
	bill, err = r.UpdateBill(ctx, org, bill.ID, BillInput{BillRef: "INV-A-1", Lines: draft.Lines})
	if err != nil || bill.MatchStatus != "matched" {
		t.Fatalf("fixed bill: %v %s", err, bill.MatchStatus)
	}
	fp := &fakePoster{}
	SetJournalPoster(fp)
	defer SetJournalPoster(nil)
	bill, err = r.PostBill(ctx, org, user, bill.ID, "")
	if err != nil || bill.State != "posted" || bill.AccountingState != "posted" {
		t.Fatalf("post bill: %v %+v", err, bill)
	}
	if len(fp.entries) != 1 || fp.entries[0].Source != "purchase_bill" {
		t.Fatalf("journal %+v", fp.entries)
	}
	var dr, cr float64
	for _, l := range fp.entries[0].Lines {
		dr += l.Debit
		cr += l.Credit
	}
	if dr != cr || cr != 448 { // 400 + 12% tax
		t.Fatalf("bill journal dr=%v cr=%v", dr, cr)
	}
	// The same quantity cannot be billed twice.
	again, err := r.CreateBill(ctx, org, user, BillInput{POID: po.ID, BillRef: "INV-A-2", Lines: draft.Lines})
	if err != nil {
		t.Fatal(err)
	}
	if again.MatchStatus != "exceptions" {
		t.Fatalf("double billing not caught: %s", again.MatchStatus)
	}
	// Override is allowed with a reason and is recorded.
	if again, err = r.PostBill(ctx, org, user, again.ID, "supplier shipped early, GRN follows"); err != nil || again.OverrideReason == "" {
		t.Fatalf("override: %v", err)
	}

	// Receive the rest (up to the tolerance): the PO becomes received.
	g2, err := r.CreateGRN(ctx, org, user, GRNInput{POID: po.ID, Lines: grnLines(pl, 65, "LOT-B", "2099-07-01")})
	if err != nil {
		t.Fatal(err)
	}
	po, _ = r.GetPO(ctx, org, po.ID)
	if po.State != "received" || po.Lines[0].QtyReceived != 105 {
		t.Fatalf("after full receipt %s %v", po.State, po.Lines[0].QtyReceived)
	}
	lots, _ := r.ListLots(ctx, org, pid, true, 0, "")
	if len(lots) != 2 {
		t.Fatalf("lots %+v", lots)
	}

	// Return 5 units of lot A to the vendor: stock drops at the receipt cost, PO net received drops.
	ret, err := r.CreateReturn(ctx, org, user, ReturnInput{GRNID: g1.ID, Reason: "damaged", Lines: []struct {
		GRNLineID string   `json:"grn_line_id"`
		Qty       float64  `json:"qty"`
		LotNo     string   `json:"lot_no"`
		Serials   []string `json:"serials"`
	}{{GRNLineID: g1.Lines[0].ID, Qty: 5}}})
	if err != nil {
		t.Fatal(err)
	}
	if ret.Total != 60 { // 5 * (10+2)
		t.Fatalf("return total %v", ret.Total)
	}
	if stockAt(t, r, org, pid, main) != 100 {
		t.Fatalf("stock after return %v", stockAt(t, r, org, pid, main))
	}
	// 70 units billed vs 100 net received: nothing to reverse yet in Accounting for billed part.
	// Returning more than was received on the line is refused.
	if _, err := r.CreateReturn(ctx, org, user, ReturnInput{GRNID: g1.ID, Reason: "x", Lines: []struct {
		GRNLineID string   `json:"grn_line_id"`
		Qty       float64  `json:"qty"`
		LotNo     string   `json:"lot_no"`
		Serials   []string `json:"serials"`
	}{{GRNLineID: g1.Lines[0].ID, Qty: 36}}}); err == nil {
		t.Fatal("returned more than received")
	}
	_ = g2

	// Supplier performance reflects the order.
	perf, err := r.SupplierPerformance(ctx, org, "", "")
	if err != nil || len(perf) != 1 || perf[0].Orders != 1 || perf[0].FillRatePct != 100 || perf[0].ReturnRatePct <= 0 {
		t.Fatalf("performance %+v %v", perf, err)
	}
	if perf[0].OnTimePct != 100 { // promised 2099
		t.Fatalf("on time %v", perf[0].OnTimePct)
	}

	// Close short: a new PO received partially and closed.
	po2, _ := r.CreatePO(ctx, org, user, POInput{SupplierID: sup.ID, Lines: poLines([2]any{pid, 10.0})})
	if _, err := r.CancelPO(ctx, org, po2.ID); err != nil {
		t.Fatalf("cancel draft: %v", err)
	}
	if _, err := r.ApprovePO(ctx, org, user, po2.ID); err == nil {
		t.Fatal("approved a cancelled order")
	}
}

func (b Bill) toInput() []struct {
	POLineID    string  `json:"po_line_id"`
	ProductID   string  `json:"product_id"`
	Description string  `json:"description"`
	Qty         float64 `json:"qty"`
	UnitCost    float64 `json:"unit_cost"`
	TaxPct      float64 `json:"tax_pct"`
} {
	var out []struct {
		POLineID    string  `json:"po_line_id"`
		ProductID   string  `json:"product_id"`
		Description string  `json:"description"`
		Qty         float64 `json:"qty"`
		UnitCost    float64 `json:"unit_cost"`
		TaxPct      float64 `json:"tax_pct"`
	}
	for _, l := range b.Lines {
		out = append(out, struct {
			POLineID    string  `json:"po_line_id"`
			ProductID   string  `json:"product_id"`
			Description string  `json:"description"`
			Qty         float64 `json:"qty"`
			UnitCost    float64 `json:"unit_cost"`
			TaxPct      float64 `json:"tax_pct"`
		}{l.POLineID, l.ProductID, l.Description, l.Qty, l.UnitCost, l.TaxPct})
	}
	return out
}

func TestReorderCountsAndAssemblyInDB(t *testing.T) {
	pool, org, user := setupDB(t)
	r := NewRepository(pool)
	ctx := context.Background()
	locs, _ := r.ListLocations(ctx, org)
	main := locs[0].ID
	s1, _ := r.SaveSupplier(ctx, org, "", Supplier{Name: "Alpha"})
	s2, _ := r.SaveSupplier(ctx, org, "", Supplier{Name: "Beta"})
	a := mustProduct(t, r, org, user, "Screw", "SCR", 1)
	b := mustProduct(t, r, org, user, "Nut", "NUT", 1)
	c := mustProduct(t, r, org, user, "Washer", "WSH", 1)
	for _, p := range []struct{ id, sup string }{{a, s1.ID}, {b, s1.ID}, {c, s2.ID}} {
		if _, err := r.SetProductSuppliers(ctx, org, p.id, []ProductSupplier{{SupplierID: p.sup, Cost: 2, Preferred: true, PackSize: 1}}); err != nil {
			t.Fatal(err)
		}
		if _, err := r.SaveReorderRule(ctx, org, ReorderRule{ProductID: p.id, MinQty: 10, MaxQty: 50, Multiple: 1, Active: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.AdjustStock(ctx, org, user, c, AdjustInput{Qty: 30, Reason: "receive", UnitCost: 1, LocationID: main}); err != nil {
		t.Fatal(err)
	}
	run, err := r.RunReorder(ctx, org, user, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Created) != 1 || len(run.Created[0].Lines) != 2 || run.Created[0].SupplierName != "Alpha" || run.Created[0].Lines[0].Qty != 50 {
		t.Fatalf("expected one Alpha draft PO with two lines of 50: %+v", run.Created)
	}
	// Running again does not duplicate lines.
	run2, _ := r.RunReorder(ctx, org, user, true)
	if len(run2.Created) != 0 || len(run2.Skipped) != 2 {
		t.Fatalf("second run created=%d skipped=%d", len(run2.Created), len(run2.Skipped))
	}
	// Once the draft is approved its quantities count as incoming and nothing more is proposed.
	if _, err := r.ApprovePO(ctx, org, user, run.Created[0].ID); err != nil {
		t.Fatal(err)
	}
	run3, _ := r.RunReorder(ctx, org, user, false)
	if len(run3.Proposals) != 0 {
		t.Fatalf("proposed again despite an open PO: %+v", run3.Proposals)
	}

	// Count: expected 30, counted 27, approve with reason posts -3.
	cnt, err := r.StartCount(ctx, org, user, CountInput{Kind: "cycle", LocationID: main, Category: ""})
	if err != nil {
		t.Fatal(err)
	}
	var line CountLine
	for _, l := range cnt.Lines {
		if l.ProductID == c {
			line = l
		}
	}
	if line.Expected != 30 {
		t.Fatalf("expected qty %v", line.Expected)
	}
	// Stock moves after the snapshot are preserved.
	if _, err := r.AdjustStock(ctx, org, user, c, AdjustInput{Qty: 5, Reason: "receive", UnitCost: 1, LocationID: main}); err != nil {
		t.Fatal(err)
	}
	q := 27.0
	if _, err := r.RecordCount(ctx, org, cnt.ID, []CountEntry{{LineID: line.ID, Counted: &q}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ApproveCount(ctx, org, user, cnt.ID, "annual"); err == nil {
		t.Fatal("approved before review")
	}
	if _, err := r.SubmitCount(ctx, org, cnt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ApproveCount(ctx, org, user, cnt.ID, ""); err == nil {
		t.Fatal("approved without a reason")
	}
	done, err := r.ApproveCount(ctx, org, user, cnt.ID, "annual count")
	if err != nil || done.State != "approved" {
		t.Fatalf("approve: %v", err)
	}
	if got := stockAt(t, r, org, c, main); got != 32 { // 30 + 5 received - 3 variance
		t.Fatalf("stock after count %v want 32", got)
	}
	h, _ := r.ListHistory(ctx, org, c, "count", nil, 10)
	if len(h) != 1 || h[0].Delta != -3 || !strings.Contains(h[0].Note, "annual count") {
		t.Fatalf("history %+v", h)
	}

	// Assembly: 2 screws + 1 nut = 1 bolt-set, labour 2 per set.
	set := mustProduct(t, r, org, user, "Bolt set", "SET", 0)
	if _, err := r.AdjustStock(ctx, org, user, a, AdjustInput{Qty: 20, Reason: "receive", UnitCost: 3, LocationID: main}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AdjustStock(ctx, org, user, b, AdjustInput{Qty: 20, Reason: "receive", UnitCost: 4, LocationID: main}); err != nil {
		t.Fatal(err)
	}
	bom, err := r.SaveBOM(ctx, org, BOMInput{ProductID: set, Kind: "assembly", Qty: 1, Lines: []BOMLineInput{{ComponentID: a, Qty: 2}, {ComponentID: b, Qty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if bom.Buildable != 10 || bom.UnitCost != 10 { // 20 screws / 2; 2*3+4 cost (product cost = last receipt cost)
		t.Fatalf("bom %+v", bom)
	}
	if _, err := r.SaveBOM(ctx, org, BOMInput{ProductID: a, Kind: "assembly", Qty: 1, Lines: []BOMLineInput{{ComponentID: set, Qty: 1}}}); err == nil {
		t.Fatal("accepted a circular BOM")
	}
	asm, err := r.Assemble(ctx, org, user, AssembleInput{BomID: bom.ID, Qty: 4, LocationID: main, LaborCost: 8})
	if err != nil {
		t.Fatal(err)
	}
	// Components cost: 8 screws @3 + 4 nuts @4 = 40, plus labour 8 = 48 for 4 sets.
	if asm.UnitCost != 12 || stockAt(t, r, org, set, main) != 4 || stockAt(t, r, org, a, main) != 12 || stockAt(t, r, org, b, main) != 16 {
		t.Fatalf("assembly %+v set=%v", asm, stockAt(t, r, org, set, main))
	}
	if q, v := valueOf(t, r, org, set); q != 4 || v != 48 {
		t.Fatalf("assembled valuation %v %v", q, v)
	}
	if _, err := r.Assemble(ctx, org, user, AssembleInput{BomID: bom.ID, Qty: 100, LocationID: main}); err == nil {
		t.Fatal("assembled without enough components")
	}

	// Planning reports run and are consistent.
	abc, err := r.ABCAnalysis(ctx, org, 365)
	if err != nil || len(abc) == 0 {
		t.Fatalf("abc %v", err)
	}
	cover, err := r.DaysOfCoverReport(ctx, org, 8)
	if err != nil || len(cover) == 0 {
		t.Fatalf("cover %v", err)
	}
	if _, err := r.AdjustStock(ctx, org, user, set, AdjustInput{Qty: 2, Reason: "sale", Ref: "INV-1", LocationID: main}); err != nil {
		t.Fatal(err)
	}
	cover, _ = r.DaysOfCoverReport(ctx, org, 8)
	for _, x := range cover {
		if x.ProductID == set && (x.Forecast4w != 2 || x.DailyDemand <= 0) { // 2 units this week: 0.5/week over the last 4 weeks, 2 units for the next 4
			t.Fatalf("cover row %+v", x)
		}
	}
	dead, err := r.DeadStock(ctx, org, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = dead
}
