package inventory

import (
	"context"
	"testing"
)

func mustProduct(t *testing.T, r *Repository, org, user, name, sku string, cost float64) string {
	t.Helper()
	p, err := r.CreateProduct(context.Background(), org, user, ProductFields{Name: name, SKU: sku, Cost: cost, Price: cost * 2})
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func stockAt(t *testing.T, r *Repository, org, pid, loc string) float64 {
	t.Helper()
	p, err := r.GetProduct(context.Background(), org, pid)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p.Stock {
		if s.LocationID == loc {
			return s.Qty
		}
	}
	return 0
}

func valueOf(t *testing.T, r *Repository, org, pid string) (float64, float64) {
	t.Helper()
	rep, err := r.ValuationAsOf(context.Background(), org, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range rep.Rows {
		if x.ProductID == pid {
			return x.Qty, x.Value
		}
	}
	return 0, 0
}

func TestValuationLayersInDB(t *testing.T) {
	pool, org, user := setupDB(t)
	r := NewRepository(pool)
	ctx := context.Background()
	locs, _ := r.ListLocations(ctx, org)
	main := locs[0].ID

	fifo := mustProduct(t, r, org, user, "Fifo item", "F-1", 0)
	avg := mustProduct(t, r, org, user, "Avg item", "A-1", 0)
	if _, err := r.SetProductSettings(ctx, org, fifo, ProductSettings{ValuationMethod: "fifo", Tracking: "none"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{fifo, avg} {
		for _, c := range []struct{ q, cost float64 }{{10, 100}, {10, 120}} {
			if _, err := r.AdjustStock(ctx, org, user, id, AdjustInput{Qty: c.q, Reason: "receive", UnitCost: c.cost, LocationID: main}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, id := range []string{fifo, avg} {
		out := &AdjustOut{}
		if _, err := r.AdjustStock(ctx, org, user, id, AdjustInput{Qty: 12, Reason: "sale", Ref: "INV-1", LocationID: main, Out: out}); err != nil {
			t.Fatal(err)
		}
		if id == fifo && out.Value != 1240 {
			t.Fatalf("fifo cogs = %v", out.Value)
		}
		if id == avg && out.Value != 1320 {
			t.Fatalf("avg cogs = %v", out.Value)
		}
	}
	q, v := valueOf(t, r, org, fifo)
	if q != 8 || v != 960 {
		t.Fatalf("fifo left qty=%v value=%v want 8 / 960", q, v)
	}
	q, v = valueOf(t, r, org, avg)
	if q != 8 || v != 880 {
		t.Fatalf("avg left qty=%v value=%v want 8 / 880", q, v)
	}
	// Customer returns 2 of the avg item at the cost they left at.
	if _, err := r.AdjustStock(ctx, org, user, avg, AdjustInput{Qty: 2, Reason: "refund", UnitCost: 110, Ref: "CN-1", LocationID: main}); err != nil {
		t.Fatal(err)
	}
	q, v = valueOf(t, r, org, avg)
	if q != 10 || v != 1100 {
		t.Fatalf("avg after return qty=%v value=%v", q, v)
	}
	// Layers match stock.
	layers, _ := r.ProductLayers(ctx, org, fifo)
	if len(layers) != 1 || layers[0].QtyLeft != 8 || layers[0].UnitCost != 120 {
		t.Fatalf("fifo layers %+v", layers)
	}
	// COGS rows are pending for Accounting.
	rows, _ := r.ListLedger(ctx, org, "", "", "sale", "stock_cogs", 50)
	if len(rows) != 2 || rows[0].JournalState != "pending" {
		t.Fatalf("ledger %+v", rows)
	}
	// A transfer between locations does not change value.
	shop, _ := r.SaveLocation(ctx, org, "", Location{Name: "Shop"})
	if _, err := r.TransferStock(ctx, org, user, fifo, main, shop.ID, 3, ""); err != nil {
		t.Fatal(err)
	}
	if q, v := valueOf(t, r, org, fifo); q != 8 || v != 960 {
		t.Fatalf("transfer changed valuation: %v %v", q, v)
	}
	// Valuation as of yesterday is empty, as of today is full.
	rep, _ := r.ValuationAsOf(ctx, org, "2020-01-01", "")
	if rep.TotalValue != 0 {
		t.Fatalf("historic valuation %v", rep.TotalValue)
	}
	// Poster receives balanced entries and rows become posted.
	fp := &fakePoster{}
	SetJournalPoster(fp)
	defer SetJournalPoster(nil)
	posted, failed, err := r.FlushJournals(ctx, org)
	if err != nil || failed != 0 || posted == 0 {
		t.Fatalf("flush posted=%d failed=%d err=%v", posted, failed, err)
	}
	var dr, cr float64
	for _, e := range fp.entries {
		for _, l := range e.Lines {
			dr += l.Debit
			cr += l.Credit
		}
		if e.Source != "stock_cogs" && e.Source != "stock_adjustment" {
			t.Fatalf("source %s", e.Source)
		}
	}
	if dr != cr || dr != 1240+1320+220 {
		t.Fatalf("journal totals dr=%v cr=%v", dr, cr)
	}
	p, _, _ := r.PendingJournalCount(ctx, org)
	if p != 0 {
		t.Fatalf("still pending %d", p)
	}
}

type fakePoster struct{ entries []JournalEntry }

func (f *fakePoster) PostEntry(_ context.Context, _ string, _ string, e JournalEntry) error {
	f.entries = append(f.entries, e)
	return nil
}

func TestLotsFEFOAndRecallInDB(t *testing.T) {
	pool, org, user := setupDB(t)
	r := NewRepository(pool)
	ctx := context.Background()
	locs, _ := r.ListLocations(ctx, org)
	main := locs[0].ID
	pid := mustProduct(t, r, org, user, "Yoghurt", "Y-1", 10)
	if _, err := r.SetProductSettings(ctx, org, pid, ProductSettings{ValuationMethod: "fifo", Tracking: "lot"}); err != nil {
		t.Fatal(err)
	}
	recv := func(lot, exp string, q float64) {
		t.Helper()
		if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: q, Reason: "receive", UnitCost: 10, LocationID: main, Ref: "GRN-X",
			Lots: []LotPick{{LotNo: lot, Expiry: exp, Qty: q}}}); err != nil {
			t.Fatal(err)
		}
	}
	recv("B-LATE", "2099-01-01", 10)
	recv("B-SOON", "2098-01-01", 5)
	recv("B-GONE", "2020-01-01", 7) // already expired

	// Sale takes the earliest in-date lot first and never touches the expired one.
	out := &AdjustOut{}
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 8, Reason: "sale", Ref: "INV-9", Party: "Acme", LocationID: main, Out: out}); err != nil {
		t.Fatal(err)
	}
	if len(out.Picks) != 2 || out.Picks[0].LotNo != "B-SOON" || out.Picks[0].Qty != 5 || out.Picks[1].LotNo != "B-LATE" || out.Picks[1].Qty != 3 {
		t.Fatalf("picks %+v", out.Picks)
	}
	// Selling more than the in-date stock is refused (7 expired units must not be sold).
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 8, Reason: "sale", Ref: "INV-10", LocationID: main}); err == nil {
		t.Fatal("sold expired stock")
	}
	// Expired stock can be written off.
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 7, Reason: "remove", Note: "expired", LocationID: main}); err != nil {
		t.Fatalf("write-off: %v", err)
	}
	lots, _ := r.ListLots(ctx, org, pid, false, 0, "")
	var late Lot
	for _, l := range lots {
		if l.LotNo == "B-LATE" {
			late = l
		}
	}
	if late.OnHand != 7 {
		t.Fatalf("late on hand %v", late.OnHand)
	}
	rc, err := r.LotRecall(ctx, org, late.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rc.Received != 10 || rc.Shipped != 3 || len(rc.Outbound) != 1 || rc.Outbound[0].Ref != "INV-9" || rc.Outbound[0].Party != "Acme" || len(rc.Origins) != 1 {
		t.Fatalf("recall %+v", rc)
	}
	// A recalled lot cannot be sold.
	if _, err := r.UpdateLot(ctx, org, late.ID, "2099-01-01", "", "bad batch", true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 1, Reason: "sale", LocationID: main}); err == nil {
		t.Fatal("sold a recalled lot")
	}
	// Transfers carry lots along.
	shop, _ := r.SaveLocation(ctx, org, "", Location{Name: "Shop"})
	if _, err := r.TransferStock(ctx, org, user, pid, main, shop.ID, 2, ""); err != nil {
		t.Fatal(err)
	}
	locsOf, _ := r.LotLocations(ctx, org, late.ID)
	got := map[string]float64{}
	for _, l := range locsOf {
		got[l.LocationID] = l.Qty
	}
	if got[main] != 5 || got[shop.ID] != 2 {
		t.Fatalf("lot locations %+v", got)
	}
}

func TestSerialsAndKitsInDB(t *testing.T) {
	pool, org, user := setupDB(t)
	r := NewRepository(pool)
	ctx := context.Background()
	locs, _ := r.ListLocations(ctx, org)
	main := locs[0].ID
	pid := mustProduct(t, r, org, user, "Phone", "PH-1", 100)
	if _, err := r.SetProductSettings(ctx, org, pid, ProductSettings{ValuationMethod: "fifo", Tracking: "serial"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 2, Reason: "receive", UnitCost: 100, LocationID: main, Serials: []string{"S1"}}); err == nil {
		t.Fatal("accepted 1 serial for 2 units")
	}
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 3, Reason: "receive", UnitCost: 100, LocationID: main, Serials: []string{"S1", "S2", "S3"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 1, Reason: "receive", UnitCost: 100, LocationID: main, Serials: []string{"s2"}}); err == nil {
		t.Fatal("duplicate serial accepted")
	}
	out := &AdjustOut{}
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 1, Reason: "sale", Ref: "INV-5", Serials: []string{"S2"}, LocationID: main, Out: out}); err != nil {
		t.Fatal(err)
	}
	ser, _ := r.ListSerials(ctx, org, pid, "in_stock", "")
	if len(ser) != 2 {
		t.Fatalf("in stock serials %d", len(ser))
	}
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 1, Reason: "sale", Serials: []string{"S2"}, LocationID: main}); err == nil {
		t.Fatal("sold the same serial twice")
	}
	all, _ := r.ListSerials(ctx, org, pid, "out", "S2")
	h, _ := r.SerialHistory(ctx, org, all[0].ID)
	if len(h) != 2 || h[1].Ref != "INV-5" {
		t.Fatalf("serial history %+v", h)
	}

	// Kit: sold as a bundle, components leave stock.
	a := mustProduct(t, r, org, user, "Pen", "PEN", 2)
	b := mustProduct(t, r, org, user, "Pad", "PAD", 5)
	kit := mustProduct(t, r, org, user, "Starter kit", "KIT", 0)
	for _, id := range []string{a, b} {
		if _, err := r.AdjustStock(ctx, org, user, id, AdjustInput{Qty: 10, Reason: "receive", UnitCost: map[string]float64{a: 2, b: 5}[id], LocationID: main}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.SaveBOM(ctx, org, BOMInput{ProductID: kit, Kind: "kit", Qty: 1, Lines: []BOMLineInput{{ComponentID: a, Qty: 2}, {ComponentID: b, Qty: 1}}}); err != nil {
		t.Fatal(err)
	}
	av, _ := r.ListAvailability(ctx, org, "Starter", "")
	if len(av) != 1 || av[0].OnHand != 5 {
		t.Fatalf("kit availability %+v", av)
	}
	kout := &AdjustOut{}
	if _, err := r.AdjustStock(ctx, org, user, kit, AdjustInput{Qty: 2, Reason: "sale", Ref: "INV-6", LocationID: main, Out: kout}); err != nil {
		t.Fatal(err)
	}
	if stockAt(t, r, org, a, main) != 6 || stockAt(t, r, org, b, main) != 8 {
		t.Fatalf("components after kit sale: %v %v", stockAt(t, r, org, a, main), stockAt(t, r, org, b, main))
	}
	if kout.Value != 18 { // 2 kits * (2*2 + 5)
		t.Fatalf("kit cogs %v", kout.Value)
	}
	if _, err := r.AdjustStock(ctx, org, user, kit, AdjustInput{Qty: 1, Reason: "refund", Ref: "CN-6", LocationID: main}); err != nil {
		t.Fatal(err)
	}
	if stockAt(t, r, org, a, main) != 8 || stockAt(t, r, org, b, main) != 9 {
		t.Fatal("kit refund did not return components")
	}
}

func TestReservationsAndTransfersInDB(t *testing.T) {
	pool, org, user := setupDB(t)
	r := NewRepository(pool)
	ctx := context.Background()
	locs, _ := r.ListLocations(ctx, org)
	main := locs[0].ID
	pid := mustProduct(t, r, org, user, "Bolt", "BOLT", 1)
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 10, Reason: "receive", UnitCost: 1, LocationID: main}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reserve(ctx, org, user, ReserveInput{ProductID: pid, Qty: 6, Ref: "SO-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reserve(ctx, org, user, ReserveInput{ProductID: pid, Qty: 5, Ref: "SO-2"}); err == nil {
		t.Fatal("reserved more than available")
	}
	av, _ := r.ListAvailability(ctx, org, "", "")
	if av[0].OnHand != 10 || av[0].Reserved != 6 || av[0].Available != 4 {
		t.Fatalf("availability %+v", av[0])
	}
	// Reserved stock cannot be removed by someone else, but the order's own sale can fulfil it.
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 5, Reason: "remove", LocationID: main}); err == nil {
		t.Fatal("removed reserved stock")
	}
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 4, Reason: "remove", LocationID: main}); err != nil {
		t.Fatalf("removing the free part failed: %v", err)
	}
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 6, Reason: "sale", Ref: "SO-1", ReserveRef: "SO-1", LocationID: main}); err != nil {
		t.Fatal(err)
	}
	rs, _ := r.ListReservations(ctx, org, "active", "", "")
	if len(rs) != 0 {
		t.Fatalf("reservation still active: %+v", rs)
	}

	// Transfer with in-transit state and partial receipt.
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 20, Reason: "receive", UnitCost: 1, LocationID: main}); err != nil {
		t.Fatal(err)
	}
	shop, err := r.SaveLocationNode(ctx, org, "", Loc{Name: "Shop warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	bin, err := r.SaveLocationNode(ctx, org, "", Loc{Name: "Bin A1", Kind: "bin", ParentID: shop.ID})
	if err != nil {
		t.Fatal(err)
	}
	if bin.Path != "Shop warehouse / Bin A1" || bin.WarehouseID != shop.ID {
		t.Fatalf("bin %+v", bin)
	}
	if _, err := r.SaveLocationNode(ctx, org, shop.ID, Loc{Name: "Shop warehouse", Kind: "warehouse", ParentID: bin.ID}); err == nil {
		t.Fatal("accepted a cycle / nested warehouse")
	}
	tr, err := r.CreateTransfer(ctx, org, user, TransferInput{FromID: main, ToID: bin.ID, Ship: true, Lines: []struct {
		ProductID string  `json:"product_id"`
		Qty       float64 `json:"qty"`
	}{{ProductID: pid, Qty: 12}}})
	if err != nil {
		t.Fatal(err)
	}
	if tr.State != "in_transit" || stockAt(t, r, org, pid, main) != 8 || stockAt(t, r, org, pid, bin.ID) != 0 {
		t.Fatalf("after ship: %+v main=%v", tr.State, stockAt(t, r, org, pid, main))
	}
	av, _ = r.ListAvailability(ctx, org, "", bin.ID)
	if av[0].InTransit != 12 {
		t.Fatalf("in transit %v", av[0].InTransit)
	}
	tr, err = r.ReceiveTransfer(ctx, org, user, tr.ID, []ReceiveLine{{LineID: tr.Lines[0].ID, Qty: 5}})
	if err != nil || tr.State != "in_transit" || stockAt(t, r, org, pid, bin.ID) != 5 {
		t.Fatalf("partial receive: %v state=%s", err, tr.State)
	}
	if _, err := r.ReceiveTransfer(ctx, org, user, tr.ID, []ReceiveLine{{LineID: tr.Lines[0].ID, Qty: 8}}); err == nil {
		t.Fatal("received more than shipped")
	}
	tr, err = r.ReceiveTransfer(ctx, org, user, tr.ID, nil)
	if err != nil || tr.State != "received" || stockAt(t, r, org, pid, bin.ID) != 12 {
		t.Fatalf("final receive: %v %+v", err, tr.State)
	}
	if err := r.SetShelfCode(ctx, org, pid, bin.ID, "A1-03"); err != nil {
		t.Fatal(err)
	}
	pl, _ := r.ProductLocations(ctx, org, pid)
	found := false
	for _, l := range pl {
		if l.LocationID == bin.ID && l.ShelfCode == "A1-03" && l.Qty == 12 {
			found = true
		}
	}
	if !found {
		t.Fatalf("shelf code / qty not shown: %+v", pl)
	}
	// Shipping then cancelling returns stock to the source.
	tr2, err := r.CreateTransfer(ctx, org, user, TransferInput{FromID: main, ToID: bin.ID, Ship: true, Lines: []struct {
		ProductID string  `json:"product_id"`
		Qty       float64 `json:"qty"`
	}{{ProductID: pid, Qty: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if stockAt(t, r, org, pid, main) != 5 {
		t.Fatal("not shipped")
	}
	if _, err := r.CancelTransfer(ctx, org, user, tr2.ID); err != nil || stockAt(t, r, org, pid, main) != 8 {
		t.Fatalf("cancel did not return stock: %v %v", err, stockAt(t, r, org, pid, main))
	}
}
