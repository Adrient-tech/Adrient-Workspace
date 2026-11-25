package inventory

import (
	"context"
	"errors"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"

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
		 VALUES ($1,'test','subject-inv','inv-tester@adrient.localtest.me','Inv Tester') RETURNING id::text`,
		orgID).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return pool, orgID, userID
}

func TestStockLifecycle(t *testing.T) {
	pool, org, user := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	locs, err := repo.ListLocations(ctx, org)
	if err != nil || len(locs) != 1 || !locs[0].IsDefault {
		t.Fatalf("default location: %+v %v", locs, err)
	}
	main := locs[0].ID
	shop, err := repo.SaveLocation(ctx, org, "", Location{Name: "Shop"})
	if err != nil {
		t.Fatal(err)
	}

	p, err := repo.CreateProduct(ctx, org, user, ProductFields{Name: "Widget", SKU: "W-1", Barcode: "123", Price: 10, Cost: 4, MinStock: 5, OpeningQty: 8})
	if err != nil {
		t.Fatal(err)
	}
	if p.TotalStock != 8 || len(p.Stock) != 1 {
		t.Fatalf("opening stock: %+v", p)
	}
	// Duplicate SKU / barcode.
	if _, err := repo.CreateProduct(ctx, org, user, ProductFields{Name: "Dup", SKU: "w-1"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("dup sku want ErrConflict, got %v", err)
	}
	if _, err := repo.CreateProduct(ctx, org, user, ProductFields{Name: "Dup", Barcode: "123"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("dup barcode want ErrConflict, got %v", err)
	}

	// sale (positive qty is subtracted), refund, receive.
	p, err = repo.AdjustStock(ctx, org, user, p.ID, AdjustInput{Qty: 3, Reason: "sale", Ref: "INV-1"})
	if err != nil || p.TotalStock != 5 {
		t.Fatalf("sale: %v %+v", err, p.TotalStock)
	}
	p, _ = repo.AdjustStock(ctx, org, user, p.ID, AdjustInput{Qty: 1, Reason: "refund", Ref: "INV-1"})
	if p.TotalStock != 6 {
		t.Fatalf("refund: %v", p.TotalStock)
	}
	p, _ = repo.AdjustStock(ctx, org, user, p.ID, AdjustInput{LocationID: shop.ID, Qty: 4, Reason: "receive", UnitCost: 5})
	if p.TotalStock != 10 || p.Cost != 5 {
		t.Fatalf("receive: total=%v cost=%v", p.TotalStock, p.Cost)
	}
	// Cannot remove more than held.
	if _, err := repo.AdjustStock(ctx, org, user, p.ID, AdjustInput{LocationID: shop.ID, Qty: 99, Reason: "remove"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("over-remove want ErrInvalid, got %v", err)
	}
	// Absolute count.
	p, _ = repo.AdjustStock(ctx, org, user, p.ID, AdjustInput{LocationID: main, Qty: 2, Absolute: true, Reason: "count"})
	if p.TotalStock != 6 {
		t.Fatalf("count: %v", p.TotalStock)
	}
	// Transfer.
	p, err = repo.TransferStock(ctx, org, user, p.ID, shop.ID, main, 3, "restock")
	if err != nil || p.TotalStock != 6 {
		t.Fatalf("transfer: %v %v", err, p.TotalStock)
	}
	for _, s := range p.Stock {
		if s.LocationID == main && s.Qty != 5 {
			t.Fatalf("main qty %v", s.Qty)
		}
	}
	// Invalid reason.
	if _, err := repo.AdjustStock(ctx, org, user, p.ID, AdjustInput{Qty: 1, Reason: "bogus"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bogus reason: %v", err)
	}

	h, err := repo.ListHistory(ctx, org, p.ID, "", nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]int{}
	for _, e := range h {
		reasons[e.Reason]++
	}
	// opening receive + receive = 2, sale, refund, count, 2 transfer.
	if reasons["receive"] != 2 || reasons["sale"] != 1 || reasons["refund"] != 1 || reasons["count"] != 1 || reasons["transfer"] != 2 {
		t.Fatalf("history reasons: %v", reasons)
	}
	// Sale ref recorded.
	if hs, _ := repo.ListHistory(ctx, org, "", "sale", nil, 10); len(hs) != 1 || hs[0].Ref != "INV-1" {
		t.Fatalf("sale history: %+v", hs)
	}

	// Location with stock can't be deleted.
	if err := repo.DeleteLocation(ctx, org, shop.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("delete shop with stock: %v", err)
	}
}

func TestVariantsReorderReport(t *testing.T) {
	pool, org, user := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	fam, err := repo.CreateProduct(ctx, org, user, ProductFields{Name: "T-Shirt", Category: "Apparel"})
	if err != nil {
		t.Fatal(err)
	}
	red, err := repo.CreateProduct(ctx, org, user, ProductFields{ParentID: fam.ID, VariantLabel: "Red / M", Name: "T-Shirt", SKU: "TS-R-M", Price: 20, Cost: 8, MinStock: 5, OpeningQty: 2, Category: "Apparel"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateProduct(ctx, org, user, ProductFields{ParentID: red.ID, Name: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("variant of variant: %v", err)
	}
	if _, err := repo.CreateProduct(ctx, org, user, ProductFields{ParentID: fam.ID, VariantLabel: "Blue / M", Name: "T-Shirt", SKU: "TS-B-M", Price: 20, Cost: 8, MinStock: 5, OpeningQty: 10, Category: "Apparel"}); err != nil {
		t.Fatal(err)
	}

	list, err := repo.ListProducts(ctx, org, ProductFilter{})
	if err != nil || len(list) != 1 || len(list[0].Variants) != 2 || list[0].TotalStock != 12 {
		t.Fatalf("nested list: %v %+v", err, list)
	}
	// Searching a variant SKU still returns the family with all variants.
	list, _ = repo.ListProducts(ctx, org, ProductFilter{Query: "ts-b"})
	if len(list) != 1 || len(list[0].Variants) != 2 {
		t.Fatalf("variant search: %+v", list)
	}
	// Exact barcode/SKU lookup in flat mode.
	flat, _ := repo.ListProducts(ctx, org, ProductFilter{Barcode: "ts-r-m", Flat: true})
	if len(flat) != 1 || flat[0].ID != red.ID {
		t.Fatalf("sku lookup: %+v", flat)
	}

	// Supplier source + reorder list.
	sup, err := repo.SaveSupplier(ctx, org, "", Supplier{Name: "Acme", ContactID: "c-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetProductSuppliers(ctx, org, red.ID, []ProductSupplier{{SupplierID: sup.ID, SupplierSKU: "A-77", Cost: 6.5, PackSize: 6, Preferred: true}}); err != nil {
		t.Fatal(err)
	}
	lines, err := repo.ReorderList(ctx, org)
	if err != nil || len(lines) != 1 {
		t.Fatalf("reorder: %v %+v", err, lines)
	}
	// min 5, stock 2 -> 2*5-2 = 8, rounded up to pack of 6 => 12.
	if lines[0].SuggestedQty != 12 || lines[0].SupplierName != "Acme" || lines[0].UnitCost != 6.5 {
		t.Fatalf("reorder line: %+v", lines[0])
	}
	if low, _ := repo.ListProducts(ctx, org, ProductFilter{LowStockOnly: true}); len(low) != 1 {
		t.Fatalf("low stock: %+v", low)
	}

	rep, err := repo.Report(ctx, org)
	if err != nil || rep.ProductCount != 2 || rep.Units != 12 || rep.CostValue != 96 || rep.RetailValue != 240 || rep.LowStockCount != 1 {
		t.Fatalf("report: %v %+v", err, rep)
	}

	// Archive family hides it and its variants.
	if _, err := repo.ArchiveProduct(ctx, org, fam.ID, true); err != nil {
		t.Fatal(err)
	}
	if l, _ := repo.ListProducts(ctx, org, ProductFilter{}); len(l) != 0 {
		t.Fatalf("archived still listed: %+v", l)
	}
	if l, _ := repo.ListProducts(ctx, org, ProductFilter{ArchivedOnly: true}); len(l) != 1 {
		t.Fatalf("archived list: %+v", l)
	}
}

func TestImport(t *testing.T) {
	pool, org, user := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	rows := []ProductFields{
		{Name: "A", SKU: "A1", Price: 1, OpeningQty: 5},
		{Name: "B", SKU: "B1", Price: 2},
		{Name: "", SKU: "BAD"},
	}
	res, _ := repo.ImportProducts(ctx, org, user, "update", rows, []string{"Warehouse", "", ""})
	if res.Created != 2 || len(res.Errors) != 1 {
		t.Fatalf("first import: %+v", res)
	}
	rows[0].OpeningQty = 3
	rows[0].Price = 9
	res, _ = repo.ImportProducts(ctx, org, user, "add", rows[:2], []string{"Warehouse", ""})
	if res.Updated != 2 {
		t.Fatalf("add import: %+v", res)
	}
	ps, _ := repo.ListProducts(ctx, org, ProductFilter{Barcode: "A1"})
	if len(ps) != 1 || ps[0].TotalStock != 8 || ps[0].Price != 9 {
		t.Fatalf("after add: %+v", ps)
	}
	res, _ = repo.ImportProducts(ctx, org, user, "skip", rows[:2], nil)
	if res.Skipped != 2 {
		t.Fatalf("skip import: %+v", res)
	}
	rows[0].OpeningQty = 1
	_, _ = repo.ImportProducts(ctx, org, user, "update", rows[:1], []string{"Warehouse"})
	ps, _ = repo.ListProducts(ctx, org, ProductFilter{Barcode: "A1"})
	if ps[0].TotalStock != 1 {
		t.Fatalf("update import should set qty: %+v", ps[0])
	}
}
