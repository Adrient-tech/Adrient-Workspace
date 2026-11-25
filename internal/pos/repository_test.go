package pos

import (
	"context"
	"errors"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupDB(t *testing.T) (*pgxpool.Pool, string, string) {
	t.Helper()
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := testguard.DropSchema(ctx, t, pool); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	var orgID, userID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&orgID); err != nil {
		t.Fatalf("default org: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		 VALUES ($1,'test','subject-pos','pos-tester@adrient.localtest.me','Pos Tester') RETURNING id::text`,
		orgID).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return pool, orgID, userID
}

func stockOf(t *testing.T, inv *inventory.Repository, org, id string) float64 {
	t.Helper()
	p, err := inv.GetProduct(context.Background(), org, id)
	if err != nil {
		t.Fatal(err)
	}
	return p.TotalStock
}

func TestSaleLifecycle(t *testing.T) {
	pool, org, user := setupDB(t)
	ctx := context.Background()
	inv := inventory.NewRepository(pool)
	repo := NewRepository(pool, inv)

	prod, err := inv.CreateProduct(ctx, org, user, inventory.ProductFields{Name: "Widget", SKU: "W1", Price: 10, OpeningQty: 10})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := repo.SaveRegister(ctx, org, "", "Front", "")
	if err != nil {
		t.Fatal(err)
	}
	tax, err := repo.SaveTaxRate(ctx, org, &adrientv1.SavePosTaxRateRequest{Name: "VAT", Rate: 10, IsDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveSettings(ctx, org, &adrientv1.PosSettings{LoyaltyEarnPerUnit: 1, LoyaltyPointValue: 0.1, Currency: "usd"}); err != nil {
		t.Fatal(err)
	}

	in := &adrientv1.PosSaleInput{
		RegisterId: reg.Id, CustomerId: "cust-1", CustomerName: "Sam",
		Lines: []*adrientv1.PosLineInput{
			{ProductId: prod.ID, Qty: 2, UnitPrice: 10, TaxRateId: tax.Id},
			{Name: "Gift wrap", Qty: 1, UnitPrice: 5},
		},
		Payments: []*adrientv1.PosPaymentInput{{Method: "cash", Amount: 50}},
	}
	// No shift yet.
	if _, err := repo.SaveSale(ctx, org, user, in, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict without shift, got %v", err)
	}
	sh, err := repo.OpenShift(ctx, org, user, reg.Id, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.OpenShift(ctx, org, user, reg.Id, 0); !errors.Is(err, ErrConflict) {
		t.Fatalf("second open shift should conflict, got %v", err)
	}

	// Underpayment and an unknown product both leave stock and sales untouched.
	bad := &adrientv1.PosSaleInput{RegisterId: reg.Id, Lines: in.Lines, Payments: []*adrientv1.PosPaymentInput{{Method: "cash", Amount: 5}}}
	if _, err := repo.SaveSale(ctx, org, user, bad, true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("underpaid: %v", err)
	}
	ghost := &adrientv1.PosSaleInput{RegisterId: reg.Id, Payments: in.Payments, Lines: []*adrientv1.PosLineInput{
		{ProductId: prod.ID, Qty: 1, UnitPrice: 10}, {ProductId: "00000000-0000-0000-0000-000000000001", Qty: 1, UnitPrice: 1}}}
	if _, err := repo.SaveSale(ctx, org, user, ghost, true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ghost product: %v", err)
	}
	if got := stockOf(t, inv, org, prod.ID); got != 10 {
		t.Fatalf("stock drifted after failed sales: %v", got)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.pos_sales`).Scan(&n)
	if n != 0 {
		t.Fatalf("failed sales left %d rows", n)
	}

	sale, err := repo.SaveSale(ctx, org, user, in, true)
	if err != nil {
		t.Fatal(err)
	}
	// 2*10 + 10% = 22, + 5 = 27; 50 tendered => change 23.
	if sale.Number != 1 || sale.Status != "completed" || sale.Total != 27 || sale.TaxTotal != 2 || sale.ChangeDue != 23 || sale.Paid != 27 {
		t.Fatalf("sale %+v", sale)
	}
	if sale.LoyaltyEarned != 27 {
		t.Fatalf("loyalty earned %v", sale.LoyaltyEarned)
	}
	if got := stockOf(t, inv, org, prod.ID); got != 8 {
		t.Fatalf("stock after sale %v", got)
	}
	hist, err := inv.ListHistory(ctx, org, prod.ID, "sale", nil, 10)
	if err != nil || len(hist) != 1 || hist[0].Ref != sale.Id {
		t.Fatalf("history %+v %v", hist, err)
	}

	// Hold, update, resume.
	held, err := repo.SaveSale(ctx, org, user, &adrientv1.PosSaleInput{RegisterId: reg.Id, HoldLabel: "Table 4",
		Lines: []*adrientv1.PosLineInput{{ProductId: prod.ID, Qty: 1, UnitPrice: 10}}}, false)
	if err != nil || held.Status != "held" || held.Number != 0 {
		t.Fatalf("hold %+v %v", held, err)
	}
	if got := stockOf(t, inv, org, prod.ID); got != 8 {
		t.Fatalf("hold moved stock: %v", got)
	}
	done, err := repo.SaveSale(ctx, org, user, &adrientv1.PosSaleInput{Id: held.Id, RegisterId: reg.Id,
		Lines:    []*adrientv1.PosLineInput{{ProductId: prod.ID, Qty: 3, UnitPrice: 10}},
		Payments: []*adrientv1.PosPaymentInput{{Method: "card", Amount: 30}}}, true)
	if err != nil || done.Number != 2 || done.Total != 30 || done.ChangeDue != 0 {
		t.Fatalf("resume %+v %v", done, err)
	}
	if got := stockOf(t, inv, org, prod.ID); got != 5 {
		t.Fatalf("stock %v", got)
	}
	if _, err := repo.SaveSale(ctx, org, user, &adrientv1.PosSaleInput{Id: held.Id, RegisterId: reg.Id,
		Lines: in.Lines, Payments: in.Payments}, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("double-complete should conflict, got %v", err)
	}

	// Store credit refund of one widget, restocked.
	var widgetLine string
	for _, l := range sale.Lines {
		if l.ProductId == prod.ID {
			widgetLine = l.Id
		}
	}
	ref, err := repo.RefundSale(ctx, org, user, &adrientv1.RefundPosSaleRequest{Id: sale.Id, RegisterId: reg.Id,
		Lines: []*adrientv1.RefundLineInput{{LineId: widgetLine, Qty: 1}}, Method: "store_credit", Restock: true})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Status != "partially_refunded" || ref.RefundedTotal != 11 || len(ref.Refunds) != 1 {
		t.Fatalf("refund %+v", ref)
	}
	if got := stockOf(t, inv, org, prod.ID); got != 6 {
		t.Fatalf("restock %v", got)
	}
	bal, err := repo.GetBalance(ctx, org, "cust-1")
	if err != nil || bal.StoreCredit != 11 {
		t.Fatalf("balance %+v %v", bal, err)
	}
	if _, err := repo.RefundSale(ctx, org, user, &adrientv1.RefundPosSaleRequest{Id: sale.Id, RegisterId: reg.Id,
		Lines: []*adrientv1.RefundLineInput{{LineId: widgetLine, Qty: 2}}, Method: "cash"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("over-refund should be invalid, got %v", err)
	}
	// Pay a later sale with the credit.
	paid, err := repo.SaveSale(ctx, org, user, &adrientv1.PosSaleInput{RegisterId: reg.Id, CustomerId: "cust-1",
		Lines:    []*adrientv1.PosLineInput{{Name: "Tip jar", Qty: 1, UnitPrice: 11}},
		Payments: []*adrientv1.PosPaymentInput{{Method: "store_credit", Amount: 11}}}, true)
	if err != nil || paid.Total != 11 {
		t.Fatalf("credit sale %+v %v", paid, err)
	}
	if _, err := repo.SaveSale(ctx, org, user, &adrientv1.PosSaleInput{RegisterId: reg.Id, CustomerId: "cust-1",
		Lines:    []*adrientv1.PosLineInput{{Name: "More", Qty: 1, UnitPrice: 1}},
		Payments: []*adrientv1.PosPaymentInput{{Method: "store_credit", Amount: 1}}}, true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overdrawn credit should be invalid, got %v", err)
	}

	// Cash drawer: 100 + 27 cash sales - 0 cash refunds + 10 in - 4 out.
	if _, err := repo.CashMove(ctx, org, sh.Id, 10, "float top-up"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CashMove(ctx, org, sh.Id, -4, "paid supplier"); err != nil {
		t.Fatal(err)
	}
	live, err := repo.GetShift(ctx, org, sh.Id)
	if err != nil || live.ExpectedCash != 133 || live.SalesCount != 3 {
		t.Fatalf("live shift %+v %v", live, err)
	}
	closed, err := repo.CloseShift(ctx, org, user, sh.Id, 130, "short 3")
	if err != nil || !closed.Closed || closed.Variance != -3 || closed.ExpectedCash != 133 {
		t.Fatalf("close %+v %v", closed, err)
	}

	list, err := repo.ListSales(ctx, org, &adrientv1.ListPosSalesRequest{Status: "refunded"})
	if err != nil || len(list) != 1 {
		t.Fatalf("list refunded %v %v", list, err)
	}
	list, err = repo.ListSales(ctx, org, &adrientv1.ListPosSalesRequest{Query: "#2", Detail: true})
	if err != nil || len(list) != 1 || len(list[0].Lines) != 1 {
		t.Fatalf("search %v %v", list, err)
	}
	rep, err := repo.Report(ctx, org, "", "")
	if err != nil || rep.SalesCount != 3 || rep.Total != 68 || rep.Refunds != 11 || rep.Net != 57 || len(rep.TaxSummary) != 1 {
		t.Fatalf("report %+v %v", rep, err)
	}
}
