package inventory

import (
	"math"
	"testing"
	"time"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.0051 }

func TestFIFOvsAverageWithReturn(t *testing.T) {
	layers := []Layer{{ID: "a", Qty: 10, Cost: 100}, {ID: "b", Qty: 10, Cost: 120}}

	fifo := ConsumeLayers("fifo", layers, 12, nil, 0)
	// 10 @100 then 2 @120
	if v := ConsumedValue(fifo); !near(v, 1240) {
		t.Fatalf("fifo value = %v want 1240 (%+v)", v, fifo)
	}
	avg := ConsumeLayers("avg", layers, 12, nil, 0)
	// average 110 over both layers
	if v := ConsumedValue(avg); !near(v, 1320) {
		t.Fatalf("avg value = %v want 1320 (%+v)", v, avg)
	}
	// Average consumption leaves the pool average unchanged.
	left := []Layer{}
	for _, l := range layers {
		rem := l.Qty
		for _, c := range avg {
			if c.LayerID == l.ID {
				rem -= c.Qty
			}
		}
		left = append(left, Layer{ID: l.ID, Qty: rem, Cost: l.Cost})
	}
	if got := AverageCost(left, 0); !near(got, 110) {
		t.Fatalf("average after sale = %v want 110", got)
	}
	// FIFO leaves only the newer layer's cost behind.
	fl := []Layer{{ID: "b", Qty: 8, Cost: 120}}
	if got := AverageCost(fl, 0); !near(got, 120) {
		t.Fatalf("fifo remaining cost = %v", got)
	}
	// A return of 2 units at the sale cost (110 avg) becomes a new layer and the
	// pool average moves, not the sale cost.
	pool := append(left, Layer{ID: "r", Qty: 2, Cost: 110})
	if got := AverageCost(pool, 0); !near(got, 110) {
		t.Fatalf("avg after return at avg cost = %v want 110", got)
	}
	// A return to the specific layer it came from (prefer) is valued at its cost.
	back := ConsumeLayers("avg", layers, 3, map[string]bool{"b": true}, 0)
	if len(back) != 1 || back[0].LayerID != "b" || !near(ConsumedValue(back), 360) {
		t.Fatalf("preferred layer consume: %+v", back)
	}
}

func TestConsumeShortfallUsesFallback(t *testing.T) {
	cs := ConsumeLayers("fifo", []Layer{{ID: "a", Qty: 2, Cost: 10}}, 5, nil, 7)
	if len(cs) != 2 || cs[1].LayerID != "" || cs[1].Qty != 3 || cs[1].Cost != 7 {
		t.Fatalf("%+v", cs)
	}
	if v := ConsumedValue(cs); !near(v, 41) {
		t.Fatalf("value %v", v)
	}
}

func TestAverageRoundingNeverOverTakes(t *testing.T) {
	layers := []Layer{{ID: "a", Qty: 3.333, Cost: 10}, {ID: "b", Qty: 3.333, Cost: 11}, {ID: "c", Qty: 3.334, Cost: 12}}
	cs := ConsumeLayers("avg", layers, 10, nil, 0)
	var q float64
	for _, c := range cs {
		q += c.Qty
	}
	if !near(q, 10) {
		t.Fatalf("took %v want 10", q)
	}
	for _, c := range cs {
		for _, l := range layers {
			if l.ID == c.LayerID && c.Qty > l.Qty+1e-9 {
				t.Fatalf("took %v from layer of %v", c.Qty, l.Qty)
			}
		}
	}
}

func TestLandedCostAllocation(t *testing.T) {
	// 100 of freight over two lines: 10 x 50 and 10 x 150.
	byValue := AllocateLanded(100, "value", []float64{10, 10}, []float64{50, 150})
	if !near(byValue[0]*10, 25) || !near(byValue[1]*10, 75) {
		t.Fatalf("value alloc %v", byValue)
	}
	byQty := AllocateLanded(100, "qty", []float64{10, 10}, []float64{50, 150})
	if !near(byQty[0]*10, 50) || !near(byQty[1]*10, 50) {
		t.Fatalf("qty alloc %v", byQty)
	}
}

func day(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

func TestPickFEFO(t *testing.T) {
	stock := []LotStock{
		{LotID: "late", LotNo: "L3", Expiry: day("2027-06-01"), Qty: 10},
		{LotID: "none", LotNo: "L0", Qty: 10},
		{LotID: "soon", LotNo: "L1", Expiry: day("2026-12-01"), Qty: 4},
		{LotID: "gone", LotNo: "LX", Expiry: day("2026-01-01"), Qty: 50},
	}
	asOf := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	picks, short := PickFEFO(stock, 9, asOf, false)
	if short != 0 || len(picks) != 2 || picks[0].LotID != "soon" || picks[0].Qty != 4 || picks[1].LotID != "late" || picks[1].Qty != 5 {
		t.Fatalf("picks %+v short %v", picks, short)
	}
	// Expired lot never picked automatically; no-expiry lot comes last.
	picks, short = PickFEFO(stock, 24, asOf, false)
	if short != 0 || picks[len(picks)-1].LotID != "none" {
		t.Fatalf("picks %+v short %v", picks, short)
	}
	for _, p := range picks {
		if p.LotID == "gone" {
			t.Fatal("picked an expired lot")
		}
	}
	_, short = PickFEFO(stock, 30, asOf, false)
	if short != 6 {
		t.Fatalf("short %v want 6", short)
	}
	_, short = PickFEFO(stock, 30, asOf, true)
	if short != 0 {
		t.Fatalf("allowExpired short %v", short)
	}
}

func TestPOStateMachine(t *testing.T) {
	ok := [][2]string{{"draft", "approved"}, {"approved", "sent"}, {"sent", "acknowledged"}, {"acknowledged", "partial"},
		{"partial", "received"}, {"received", "closed"}, {"draft", "cancelled"}, {"sent", "cancelled"}}
	for _, m := range ok {
		if !POCanMove(m[0], m[1]) {
			t.Errorf("%s -> %s should be allowed", m[0], m[1])
		}
	}
	bad := [][2]string{{"draft", "sent"}, {"draft", "received"}, {"closed", "draft"}, {"cancelled", "approved"},
		{"partial", "cancelled"}, {"received", "cancelled"}}
	for _, m := range bad {
		if POCanMove(m[0], m[1]) {
			t.Errorf("%s -> %s should be refused", m[0], m[1])
		}
	}
	if POCanReceive("draft") || !POCanReceive("partial") || POCanReceive("closed") {
		t.Fatal("receive gating wrong")
	}
	if got := POStateAfterReceipt([][2]float64{{10, 4}, {5, 5}}, false); got != POpartial {
		t.Fatalf("got %s", got)
	}
	if got := POStateAfterReceipt([][2]float64{{10, 10}, {5, 5}}, false); got != POreceived {
		t.Fatalf("got %s", got)
	}
	if got := POStateAfterReceipt([][2]float64{{10, 4}}, true); got != POclosed {
		t.Fatalf("got %s", got)
	}
}

func TestReceiptLimit(t *testing.T) {
	if ReceiptLimit(100, 60, 0) != 40 {
		t.Fatal("exact")
	}
	if ReceiptLimit(100, 60, 10) != 50 {
		t.Fatal("10% over")
	}
	if ReceiptLimit(100, 120, 0) != 0 {
		t.Fatal("never negative")
	}
}

func TestThreeWayMatch(t *testing.T) {
	good := []MatchLine{{LineID: "1", Name: "A", OrderedQty: 10, ReceivedQty: 6, BillQty: 6, POCost: 100, BillCost: 100}}
	if got := ThreeWayMatch(good, 0, 2); len(got) != 0 {
		t.Fatalf("clean bill flagged: %+v", got)
	}
	over := []MatchLine{{LineID: "1", Name: "A", OrderedQty: 10, ReceivedQty: 6, BillQty: 8, POCost: 100, BillCost: 100}}
	if got := ThreeWayMatch(over, 0, 2); len(got) != 1 || got[0].Kind != "qty_over_received" {
		t.Fatalf("over-billing not flagged: %+v", got)
	}
	// Earlier bills count against what was received.
	twice := []MatchLine{{LineID: "1", Name: "A", ReceivedQty: 6, BilledBefore: 6, BillQty: 1, POCost: 5, BillCost: 5}}
	if got := ThreeWayMatch(twice, 0, 2); len(got) != 1 {
		t.Fatalf("double billing not flagged: %+v", got)
	}
	price := []MatchLine{{LineID: "1", Name: "A", ReceivedQty: 6, BillQty: 6, POCost: 100, BillCost: 103}}
	if got := ThreeWayMatch(price, 0, 2); len(got) != 1 || got[0].Kind != "price_variance" {
		t.Fatalf("price variance not flagged: %+v", got)
	}
	if got := ThreeWayMatch(price, 0, 5); len(got) != 0 {
		t.Fatalf("within tolerance flagged: %+v", got)
	}
	cheaper := []MatchLine{{LineID: "1", ReceivedQty: 6, BillQty: 6, POCost: 100, BillCost: 90}}
	if got := ThreeWayMatch(cheaper, 0, 0); len(got) != 0 {
		t.Fatalf("cheaper bill flagged: %+v", got)
	}
	if got := ThreeWayMatch([]MatchLine{{Name: "freight", BillQty: 1}}, 0, 0); len(got) != 1 || got[0].Kind != "no_po_line" {
		t.Fatalf("unknown line: %+v", got)
	}
}

func TestPlanningMath(t *testing.T) {
	cls := ABCClass([]float64{800, 100, 60, 30, 10})
	want := []string{"A", "B", "B", "C", "C"}
	for i := range want {
		if cls[i] != want[i] {
			t.Fatalf("abc %v want %v", cls, want)
		}
	}
	if got := ABCClass([]float64{0, 0}); got[0] != "C" || got[1] != "C" {
		t.Fatal("zero value items are C")
	}
	if got := MovingAverage([]float64{1, 2, 3, 4, 5, 6}, 3); got != 5 {
		t.Fatalf("sma %v", got)
	}
	if DaysOfCover(30, 3) != 10 || DaysOfCover(5, 0) != -1 || DaysOfCover(-2, 1) != 0 {
		t.Fatal("days of cover")
	}
	if ReorderQty(12, 10, 50, 1) != 0 {
		t.Fatal("above min must not reorder")
	}
	if got := ReorderQty(4, 10, 50, 12); got != 48 { // 46 rounded up to a multiple of 12
		t.Fatalf("reorder qty %v", got)
	}
	tier, ok := PickTier([]PriceTier{{MinQty: 1, Cost: 10}, {MinQty: 100, Cost: 8}, {MinQty: 1000, Cost: 6}}, 150, time.Now())
	if !ok || tier.Cost != 8 {
		t.Fatalf("tier %+v", tier)
	}
	past := time.Now().AddDate(0, 0, -5)
	if _, ok := PickTier([]PriceTier{{MinQty: 1, Cost: 10, ValidTo: &past}}, 1, time.Now()); ok {
		t.Fatal("expired tier used")
	}
}
