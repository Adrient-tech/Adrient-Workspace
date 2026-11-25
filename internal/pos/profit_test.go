package pos

import (
	"context"
	"testing"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
)

func i64(v int64) *int64 { return &v }

func TestSummariseProfitCoverageAndRefunds(t *testing.T) {
	s := SummariseProfit([]ProfitLine{
		{Key: "a", Name: "Latte", Net: 10000, Cost: i64(4000), KeptShare: 1},
		{Key: "a", Name: "Latte", Net: 10000, Cost: i64(4000), KeptShare: 0.5}, // half refunded
		{Key: "b", Name: "Gift wrap", Net: 5000, KeptShare: 1},                 // no cost recorded
		{Key: "c", Name: "Cup", Net: 3333, Cost: i64(1111), KeptShare: 1},
	})
	// kept net: 100 + 50 + 50 + 33.33 ; costed: 100 + 50 + 33.33 = 183.33, cost 40 + 20 + 11.11 = 71.11
	if s.Net != 233.33 || s.KnownNet != 183.33 || s.Cost != 71.11 || s.Profit != 112.22 {
		t.Fatalf("%+v", s)
	}
	if s.Margin == nil || *s.Margin != 61.21 || s.Known != 3 || s.Unknown != 1 || !s.Any {
		t.Fatalf("margin %+v", s)
	}
	if s.Coverage < 0.78 || s.Coverage > 0.79 {
		t.Fatalf("coverage %v", s.Coverage)
	}
	if s.TopProfit[0].Key != "a" || s.TopProfit[0].Profit != 90 {
		t.Fatalf("%+v", s.TopProfit)
	}
	none := SummariseProfit([]ProfitLine{{Net: 100, KeptShare: 1}})
	if none.Any || none.Margin != nil || none.Profit != 0 {
		t.Fatalf("no costs must show no margin: %+v", none)
	}
}

func TestMarginPercent(t *testing.T) {
	if m, ok := MarginPercent(200, 150); !ok || m != 25 {
		t.Fatal(m, ok)
	}
	if m, _ := MarginPercent(3, 2); m != 33.33 {
		t.Fatal(m)
	}
	if m, _ := MarginPercent(90, 100); m != -11.11 {
		t.Fatal(m)
	}
	if _, ok := MarginPercent(0, 5); ok {
		t.Fatal("no price")
	}
}

func TestSaleFreezesCostAndProfitReport(t *testing.T) {
	pool, org, user := setupDB(t)
	ctx := context.Background()
	inv := inventory.NewRepository(pool)
	repo := NewRepository(pool, inv)
	prod, err := inv.CreateProduct(ctx, org, user, inventory.ProductFields{Name: "Widget", SKU: "W1", Price: 10, Cost: 4, OpeningQty: 10})
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := repo.SaveRegister(ctx, org, "", "Front", "")
	if _, err := repo.OpenShift(ctx, org, user, reg.Id, 0); err != nil {
		t.Fatal(err)
	}
	sale, err := repo.SaveSale(ctx, org, user, &adrientv1.PosSaleInput{RegisterId: reg.Id,
		Lines:    []*adrientv1.PosLineInput{{ProductId: prod.ID, Qty: 3, UnitPrice: 10}, {Name: "Gift wrap", Qty: 1, UnitPrice: 5}},
		Payments: []*adrientv1.PosPaymentInput{{Method: "cash", Amount: 35}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	var unit, total *float64
	if err := pool.QueryRow(ctx, `SELECT unit_cost::float8, cost_total::float8 FROM adrient.pos_sale_lines WHERE sale_id=$1 AND position=0`, sale.Id).Scan(&unit, &total); err != nil {
		t.Fatal(err)
	}
	if unit == nil || *unit != 4 || *total != 12 {
		t.Fatalf("frozen cost %v %v", unit, total)
	}
	var gw *float64
	_ = pool.QueryRow(ctx, `SELECT cost_total::float8 FROM adrient.pos_sale_lines WHERE sale_id=$1 AND position=1`, sale.Id).Scan(&gw)
	if gw != nil {
		t.Fatalf("one-off item must have no cost, got %v", *gw)
	}
	// a later cost change must not move what the sale earned
	if _, err := pool.Exec(ctx, `UPDATE adrient.inv_products SET cost=9 WHERE id=$1`, prod.ID); err != nil {
		t.Fatal(err)
	}
	rep, err := repo.ProfitReport(ctx, org, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.KnownNet != 30 || rep.Cost != 12 || rep.Profit != 18 || rep.Net != 35 || rep.Unknown != 1 || rep.Margin == nil || *rep.Margin != 60 {
		t.Fatalf("%+v", rep)
	}
}
