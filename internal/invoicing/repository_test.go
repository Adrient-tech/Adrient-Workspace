package invoicing

import (
	"context"
	"errors"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupDB(t *testing.T) (*Repository, string, string) {
	t.Helper()
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := testguard.DropSchema(ctx, t, pool); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var org, user string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		VALUES ($1,'test','subject-invc','invc@adrient.localtest.me','Invc') RETURNING id::text`, org).Scan(&user); err != nil {
		t.Fatal(err)
	}
	return NewRepository(pool), org, user
}

func TestInvoiceLifecycle(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()

	d, err := repo.SaveDocument(ctx, org, user, "", DocInput{
		ClientName: "Acme", ClientEmail: "a@acme.test",
		Lines:        []Line{{Description: "Work", Qty: 2, Price: 100, TaxRate: 18}},
		DiscountKind: "percent", DiscountValue: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Number != "INV-0001" || d.Total != 236 || d.DisplayStatus != "draft" || d.Currency != "INR" || d.DueDate == "" {
		t.Fatalf("created: %+v", d)
	}
	// Payments on a draft mark it sent; partial then paid.
	d, err = repo.AddPayment(ctx, org, user, d.ID, Payment{Amount: 100, Method: "upi"})
	if err != nil || d.DisplayStatus != "partial" || d.Balance != 136 {
		t.Fatalf("partial: %v %+v", err, d)
	}
	if _, err = repo.AddPayment(ctx, org, user, d.ID, Payment{Amount: 500}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overpay should be invalid: %v", err)
	}
	d, err = repo.AddPayment(ctx, org, user, d.ID, Payment{Amount: 136})
	if err != nil || d.DisplayStatus != "paid" {
		t.Fatalf("paid: %v %+v", err, d)
	}
	d, err = repo.AddPayment(ctx, org, user, d.ID, Payment{Kind: "refund", Amount: 36})
	if err != nil || d.Balance != 36 || d.Paid != 200 {
		t.Fatalf("refund: %v %+v", err, d)
	}
	d, err = repo.AddPayment(ctx, org, user, d.ID, Payment{Kind: "writeoff", Amount: 36})
	if err != nil || d.Balance != 0 {
		t.Fatalf("writeoff: %v %+v", err, d)
	}

	// Trash is not void: hidden from the list, restorable, purge only from trash.
	if err := repo.Purge(ctx, org, d.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("purge of a live doc must fail: %v", err)
	}
	if err := repo.Trash(ctx, org, d.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := repo.ListDocuments(ctx, org, DocFilter{}); len(l) != 0 {
		t.Fatal("trashed doc listed")
	}
	if l, _ := repo.ListDocuments(ctx, org, DocFilter{Trash: true}); len(l) != 1 {
		t.Fatal("trash list")
	}
	if err := repo.Restore(ctx, org, d.ID); err != nil {
		t.Fatal(err)
	}
}

func TestEstimateConvertAndRecurringAndPublic(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()

	e, err := repo.SaveDocument(ctx, org, user, "", DocInput{Kind: "estimate", ClientName: "Beta", Currency: "usd",
		Lines: []Line{{Description: "Design", Qty: 1, Price: 500}}})
	if err != nil || e.Number != "EST-0001" || e.Currency != "USD" {
		t.Fatalf("estimate: %v %+v", err, e)
	}
	inv, err := repo.ConvertEstimate(ctx, org, user, e.ID)
	if err != nil || inv.Kind != "invoice" || inv.ConvertedFrom != e.ID || inv.Number != "INV-0001" {
		t.Fatalf("convert: %v %+v", err, inv)
	}
	if _, err := repo.ConvertEstimate(ctx, org, user, e.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("double convert: %v", err)
	}

	// Drafts are not public; sent ones are, and org scoping holds.
	if _, _, err := repo.PublicByToken(ctx, inv.ShareToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("draft must not be public: %v", err)
	}
	if _, _, err := repo.MarkSent(ctx, org, inv.ID); err != nil {
		t.Fatal(err)
	}
	pd, st, err := repo.PublicByToken(ctx, inv.ShareToken)
	if err != nil || pd.ID != inv.ID || pd.ViewedAt == "" {
		t.Fatalf("public: %v %+v", err, pd)
	}
	_ = st
	if _, err := repo.GetDocument(ctx, "00000000-0000-0000-0000-000000000000", inv.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org read: %v", err)
	}

	// Recurring catches up missed periods.
	rc, err := repo.SaveRecurring(ctx, org, "", Recurring{Name: "Retainer", Frequency: "weekly", Active: true,
		NextRun:  today().AddDate(0, 0, -14).Format("2006-01-02"),
		Template: DocInput{ClientName: "Gamma", Lines: []Line{{Description: "Retainer", Qty: 1, Price: 10}}}})
	if err != nil {
		t.Fatal(err)
	}
	made, _, err := repo.RunRecurring(ctx, org, user)
	if err != nil || len(made) != 3 {
		t.Fatalf("recurring made %d: %v", len(made), err)
	}
	made, _, _ = repo.RunRecurring(ctx, org, user)
	if len(made) != 0 {
		t.Fatalf("second run should be a no-op, made %d", len(made))
	}
	l, _ := repo.ListRecurring(ctx, org)
	if l[0].ID != rc.ID || l[0].Generated != 3 {
		t.Fatalf("generated count: %+v", l)
	}

	rep, err := repo.BuildReport(ctx, org, "", "")
	if err != nil || rep.Invoiced <= 0 {
		t.Fatalf("report: %v %+v", err, rep)
	}
	cl, err := repo.Clients(ctx, org)
	if err != nil || len(cl) == 0 {
		t.Fatalf("clients: %v", err)
	}
}
