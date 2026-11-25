package accounting

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

// setupDB creates a throwaway database next to the server named in
// ADRIENT_TEST_DSN (used only to issue CREATE/DROP DATABASE), migrates it and
// drops it afterwards. It never touches the dev database's data.
func setupDB(t *testing.T) (*Repository, string, string) {
	t.Helper()
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set; skipping integration test")
	}
	ctx := context.Background()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("acct_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close(ctx)
		t.Fatalf("create scratch database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		admin.Close(context.Background())
	})
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pc.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var org, user string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		VALUES ($1,'test','subject-acct','acct@adrient.localtest.me','Acct') RETURNING id::text`, org).Scan(&user); err != nil {
		t.Fatal(err)
	}
	return NewRepository(pool), org, user
}

type fakePos struct{ sales []*adrientv1.PosSale }

func (f *fakePos) ListSales(_ context.Context, _ string, req *adrientv1.ListPosSalesRequest) ([]*adrientv1.PosSale, error) {
	if req.GetUntil() != "" { // single page in tests
		return nil, nil
	}
	return f.sales, nil
}

type fakeInv struct {
	docs []invoicing.Document
	pays []invoicing.Payment
}

func (f *fakeInv) ListDocuments(_ context.Context, _ string, fl invoicing.DocFilter) ([]invoicing.Document, error) {
	out := []invoicing.Document{}
	for _, d := range f.docs {
		if d.Deleted == fl.Trash {
			out = append(out, d)
		}
	}
	return out, nil
}
func (f *fakeInv) ListPayments(context.Context, string, string, string) ([]invoicing.Payment, error) {
	return f.pays, nil
}

func requireBalanced(t *testing.T, repo *Repository, org string) TrialBalance {
	t.Helper()
	ctx := context.Background()
	accts, err := repo.ListAccounts(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := repo.LoadPostings(ctx, org, "")
	if err != nil {
		t.Fatal(err)
	}
	tb := BuildTrialBalance(accts, ps, "")
	if !tb.Balanced {
		t.Fatalf("trial balance out of balance: debit %v credit %v", tb.TotalDebit, tb.TotalCredit)
	}
	bs := BuildBalanceSheet(accts, ps, "2099-12-31", 4)
	if !bs.Balanced {
		t.Fatalf("balance sheet out of balance: assets %v, L+E %v", bs.TotalAssets, bs.TotalLiabilities+bs.TotalEquity)
	}
	return tb
}

func idOf(t *testing.T, repo *Repository, org, key string) string {
	t.Helper()
	_ = repo.EnsureSeeded(context.Background(), org)
	keys, err := repo.accountKeys(context.Background(), repo.pool, org)
	if err != nil || keys[key] == "" {
		t.Fatalf("account key %s: %v", key, err)
	}
	return keys[key]
}

func TestSeedAndManualJournal(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	accts, err := repo.ListAccounts(ctx, org)
	if err != nil || len(accts) != len(DefaultChart) {
		t.Fatalf("seed: %v %d", err, len(accts))
	}
	if again, _ := repo.ListAccounts(ctx, org); len(again) != len(accts) {
		t.Fatal("seeding must be idempotent")
	}
	bank, capital := idOf(t, repo, org, "bank"), idOf(t, repo, org, "capital")
	if _, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-04-02", Memo: "bad",
		Lines: []Line{{AccountID: bank, Debit: 100}, {AccountID: capital, Credit: 90}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbalanced journal must be rejected, got %v", err)
	}
	j, created, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-04-02", Memo: "Capital introduced",
		Lines: []Line{{AccountID: bank, Debit: 100000}, {AccountID: capital, Credit: 100000}}})
	if err != nil || !created || j.Number != 1 || j.Total != 100000 {
		t.Fatalf("post: %v %+v", err, j)
	}
	// Editing is allowed, deleting too.
	j, err = repo.UpdateManual(ctx, org, user, j.ID, JournalInput{Date: "2026-04-03", Memo: "Capital",
		Lines: []Line{{AccountID: bank, Debit: 120000}, {AccountID: capital, Credit: 120000}}})
	if err != nil || j.Total != 120000 {
		t.Fatalf("update: %v %+v", err, j)
	}
	requireBalanced(t, repo, org)

	// Closing date locks the period for every write path.
	if _, err := repo.SaveSettings(ctx, org, user, Settings{FYStartMonth: 4, Currency: "INR", Intrastate: true, ClosingDate: "2026-04-30"}); err != nil {
		t.Fatal(err)
	}
	_, _, err = repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-04-30", Memo: "late",
		Lines: []Line{{AccountID: bank, Debit: 1}, {AccountID: capital, Credit: 1}}})
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("post in closed period: %v", err)
	}
	if _, err := repo.UpdateManual(ctx, org, user, j.ID, JournalInput{Date: "2026-05-02", Memo: "x",
		Lines: []Line{{AccountID: bank, Debit: 1}, {AccountID: capital, Credit: 1}}}); !errors.Is(err, ErrLocked) {
		t.Fatalf("editing a closed journal: %v", err)
	}
	if err := repo.DeleteJournal(ctx, org, user, j.ID); !errors.Is(err, ErrLocked) {
		t.Fatalf("deleting a closed journal: %v", err)
	}
	if _, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-05-01", Memo: "open",
		Lines: []Line{{AccountID: bank, Debit: 5}, {AccountID: capital, Credit: 5}}}); err != nil {
		t.Fatalf("after the closing date: %v", err)
	}
	// Another org's account is rejected.
	if _, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-05-01", Memo: "x",
		Lines: []Line{{AccountID: "00000000-0000-0000-0000-000000000000", Debit: 5}, {AccountID: capital, Credit: 5}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown account: %v", err)
	}
	if a, err := repo.ListAudit(ctx, org, 50); err != nil || len(a) == 0 {
		t.Fatalf("audit: %v %d", err, len(a))
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	pos := &fakePos{sales: []*adrientv1.PosSale{
		{Id: "sale-1", Number: 1, Status: "completed", Total: 1180, TaxTotal: 180, CompletedAt: "2026-11-02T10:00:00Z",
			Payments: []*adrientv1.PosPayment{{Method: "cash", Amount: 1180}}},
		{Id: "sale-2", Number: 2, Status: "partially_refunded", Total: 590, TaxTotal: 90, CompletedAt: "2026-11-03T10:00:00Z",
			Payments: []*adrientv1.PosPayment{{Method: "card", Amount: 590}},
			Refunds:  []*adrientv1.PosRefund{{Id: "ref-1", Number: "R2-1", Total: 118, Method: "cash", CreatedAt: "2026-11-04T10:00:00Z"}}},
		{Id: "sale-held", Number: 0, Status: "held", Total: 99},
	}}
	inv := &fakeInv{
		docs: []invoicing.Document{
			{ID: "inv-1", Kind: "invoice", Number: "INV-0001", Status: "sent", Total: 2360, TaxTotal: 360, IssueDate: "2026-11-01", DueDate: "2026-11-15", ClientName: "Acme", ContactID: "c1", Currency: "INR"},
			{ID: "inv-draft", Kind: "invoice", Number: "INV-0002", Status: "draft", Total: 100, IssueDate: "2026-11-01", Currency: "INR"},
			{ID: "inv-usd", Kind: "invoice", Number: "INV-0003", Status: "sent", Total: 100, IssueDate: "2026-11-01", Currency: "USD"},
		},
		pays: []invoicing.Payment{{ID: "pay-1", DocumentID: "inv-1", Kind: "payment", Amount: 1000, Method: "upi", PaidAt: "2026-11-05", DocNumber: "INV-0001", ClientName: "Acme", Currency: "INR"}},
	}
	res, err := repo.Sync(ctx, org, user, pos, inv, true)
	if err != nil {
		t.Fatal(err)
	}
	if res["pos"].Created != 3 || res["invoicing"].Created != 2 || res["invoicing"].Foreign != 1 {
		t.Fatalf("first sync: pos %+v invoicing %+v", res["pos"], res["invoicing"])
	}
	first := requireBalanced(t, repo, org)
	count := func() int {
		var n int
		_ = repo.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.acct_journals WHERE org_id=$1`, org).Scan(&n)
		return n
	}
	before := count()
	for i := 0; i < 3; i++ {
		res, err = repo.Sync(ctx, org, user, pos, inv, i == 0)
		if err != nil {
			t.Fatal(err)
		}
		if res["pos"].Created+res["invoicing"].Created+res["pos"].Updated+res["invoicing"].Updated != 0 {
			t.Fatalf("re-sync %d created or updated rows: %+v %+v", i, res["pos"], res["invoicing"])
		}
	}
	if count() != before {
		t.Fatalf("journals %d -> %d: double posting", before, count())
	}
	if again := requireBalanced(t, repo, org); again.TotalDebit != first.TotalDebit {
		t.Fatalf("totals moved on re-sync: %v -> %v", first.TotalDebit, again.TotalDebit)
	}

	// An edited invoice is rewritten in place, not posted twice.
	inv.docs[0].Total, inv.docs[0].TaxTotal = 3540, 540
	res, _ = repo.Sync(ctx, org, user, pos, inv, false)
	if res["invoicing"].Updated != 1 || res["invoicing"].Created != 0 || count() != before {
		t.Fatalf("edit: %+v journals %d/%d", res["invoicing"], before, count())
	}
	accts, _ := repo.ListAccounts(ctx, org)
	for _, a := range accts {
		if a.SystemKey == "ar" && a.Balance != 3540-1000 {
			t.Fatalf("receivable %v", a.Balance)
		}
	}
	// A payment removed in Invoicing and an invoice sent back to draft are voided.
	inv.pays = nil
	inv.docs[0].Status = "draft"
	res, _ = repo.Sync(ctx, org, user, pos, inv, false)
	if res["invoicing"].Voided != 2 {
		t.Fatalf("void: %+v", res["invoicing"])
	}
	requireBalanced(t, repo, org)
	for _, a := range func() []Account { x, _ := repo.ListAccounts(ctx, org); return x }() {
		if a.SystemKey == "ar" && a.Balance != 0 {
			t.Fatalf("receivable after void: %v", a.Balance)
		}
	}

	// Reports: GST output is 450 (sales 270 + refund reversal -18 = 252?) -> compute from sources.
	ps, _ := repo.LoadPostings(ctx, org, "")
	g := BuildGST(accts, ps, "", "2099-12-31")
	// sale-1 tax 180, sale-2 tax 90, refund reverses 90*118/590 = 18.
	if g.OutputTotal != 252 {
		t.Fatalf("GST output %v", g.OutputTotal)
	}
	// A closed period skips posting instead of failing the sync.
	if _, err := repo.SaveSettings(ctx, org, user, Settings{FYStartMonth: 4, Currency: "INR", Intrastate: true, ClosingDate: "2026-11-30"}); err != nil {
		t.Fatal(err)
	}
	pos.sales = append(pos.sales, &adrientv1.PosSale{Id: "sale-3", Number: 3, Status: "completed", Total: 100, CompletedAt: "2026-11-10T10:00:00Z"})
	res, _ = repo.Sync(ctx, org, user, pos, inv, false)
	if res["pos"].Locked != 1 || res["pos"].Created != 0 {
		t.Fatalf("locked: %+v", res["pos"])
	}
}

func TestBankImportRulesAndReconcile(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	bank, capital := idOf(t, repo, org, "bank"), idOf(t, repo, org, "capital")
	accts, _ := repo.ListAccounts(ctx, org)
	var rent, misc string
	for _, a := range accts {
		switch a.Name {
		case "Rent":
			rent = a.ID
		case "Miscellaneous Expenses":
			misc = a.ID
		}
	}
	if _, err := repo.SaveRule(ctx, org, user, "", Rule{Contains: "landlord", Direction: "out", CategoryID: rent, AutoAdd: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveRule(ctx, org, user, "", Rule{Contains: "swiggy", CategoryID: misc}); err != nil {
		t.Fatal(err)
	}
	// Opening money so the statement balance is meaningful.
	if _, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-04-01", Memo: "open",
		Lines: []Line{{AccountID: bank, Debit: 50000}, {AccountID: capital, Credit: 50000}}}); err != nil {
		t.Fatal(err)
	}
	csv := "Date,Narration,Debit,Credit\n05/04/2026,NEFT LANDLORD RENT,15000,\n06/04/2026,UPI SWIGGY,500,\n07/04/2026,UNKNOWN DEPOSIT,,2000\n"
	res, err := repo.ImportCSV(ctx, org, user, bank, csv)
	if err != nil || res.Imported != 3 || res.Duplicates != 0 || res.Categorized != 2 || res.Added != 1 {
		t.Fatalf("import: %v %+v", err, res)
	}
	again, err := repo.ImportCSV(ctx, org, user, bank, csv)
	if err != nil || again.Imported != 0 || again.Duplicates != 3 {
		t.Fatalf("re-import must be a no-op: %v %+v", err, again)
	}
	pending, _ := repo.ListBankLines(ctx, org, bank, "pending")
	if len(pending) != 2 {
		t.Fatalf("pending %d", len(pending))
	}
	for _, l := range pending {
		switch l.Description {
		case "UPI SWIGGY":
			if l.CategoryID != misc {
				t.Fatalf("rule did not categorize: %+v", l)
			}
			if _, err := repo.AddBankLine(ctx, org, user, l.ID, l.CategoryID, ""); err != nil {
				t.Fatal(err)
			}
		case "UNKNOWN DEPOSIT":
			if _, err := repo.AddBankLine(ctx, org, user, l.ID, capital, "Owner deposit"); err != nil {
				t.Fatal(err)
			}
		}
	}
	requireBalanced(t, repo, org)
	// Bank = 50000 - 15000 - 500 + 2000 = 36500.
	if a, _ := repo.ListAccounts(ctx, org); func() float64 {
		for _, x := range a {
			if x.ID == bank {
				return x.Balance
			}
		}
		return -1
	}() != 36500 {
		t.Fatal("bank balance")
	}
	// Undo removes the journal and returns the line to pending.
	added, _ := repo.ListBankLines(ctx, org, bank, "added")
	if len(added) != 3 {
		t.Fatalf("added %d", len(added))
	}
	if err := repo.UndoBankLine(ctx, org, user, added[0].ID); err != nil {
		t.Fatal(err)
	}
	if p, _ := repo.ListBankLines(ctx, org, bank, "pending"); len(p) != 1 {
		t.Fatalf("pending after undo %d", len(p))
	}
	if _, err := repo.AddBankLine(ctx, org, user, added[0].ID, capital, "again"); err != nil {
		t.Fatal(err)
	}

	// Reconcile: a wrong ending balance is refused, the right one reconciles.
	d, err := repo.ReconcileDraft(ctx, org, bank, "2026-04-30")
	if err != nil || len(d.Lines) != 4 {
		t.Fatalf("draft: %v %+v", err, d)
	}
	ids := []string{}
	for _, l := range d.Lines {
		ids = append(ids, l.LineID)
	}
	if _, err := repo.FinishReconciliation(ctx, org, user, bank, "2026-04-30", 36000, ids); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong balance: %v", err)
	}
	rec, err := repo.FinishReconciliation(ctx, org, user, bank, "2026-04-30", 36500, ids)
	if err != nil || rec.Cleared != 4 {
		t.Fatalf("finish: %v %+v", err, rec)
	}
	if d2, _ := repo.ReconcileDraft(ctx, org, bank, "2026-04-30"); len(d2.Lines) != 0 || d2.OpeningBalance != 36500 {
		t.Fatalf("after: %+v", d2)
	}
	// A reconciled entry cannot be edited or deleted until the reconciliation is undone.
	js, _ := repo.ListJournals(ctx, org, JournalFilter{Source: "manual"})
	if err := repo.DeleteJournal(ctx, org, user, js[0].ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reconciled delete: %v", err)
	}
	if err := repo.UndoReconciliation(ctx, org, user, rec.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteJournal(ctx, org, user, js[0].ID); err != nil {
		t.Fatalf("delete after undo: %v", err)
	}
}

func TestExpensesAndReports(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	bank, capital := idOf(t, repo, org, "bank"), idOf(t, repo, org, "capital")
	accts, _ := repo.ListAccounts(ctx, org)
	var rent, travel string
	for _, a := range accts {
		switch a.Name {
		case "Rent":
			rent = a.ID
		case "Travel & Conveyance":
			travel = a.ID
		}
	}
	if _, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-04-01", Memo: "open",
		Lines: []Line{{AccountID: bank, Debit: 100000}, {AccountID: capital, Credit: 100000}}}); err != nil {
		t.Fatal(err)
	}
	// Paid expense with GST: Dr rent 10000 + input 1800 (CGST/SGST), Cr bank 11800.
	e, err := repo.SaveExpense(ctx, org, user, "", Expense{Date: "2026-04-05", VendorName: "Landlord", CategoryID: rent, Amount: 10000, GSTRate: 18,
		Paid: true, PaidFromID: bank, Receipts: []Receipt{{ID: "file-1", Name: "rent.pdf"}}})
	if err != nil || e.Total != 11800 || !e.Posted || len(e.Receipts) != 1 {
		t.Fatalf("paid expense: %v %+v", err, e)
	}
	// Unpaid bill, then paid later.
	bill, err := repo.SaveExpense(ctx, org, user, "", Expense{Date: "2026-04-06", VendorName: "Printer", CategoryID: travel, Amount: 1000, GSTRate: 0})
	if err != nil || bill.Paid {
		t.Fatalf("bill: %v %+v", err, bill)
	}
	requireBalanced(t, repo, org)
	if ap := func() float64 {
		a, _ := repo.ListAccounts(ctx, org)
		for _, x := range a {
			if x.SystemKey == "ap" {
				return x.Balance
			}
		}
		return -1
	}(); ap != 1000 {
		t.Fatalf("payables %v", ap)
	}
	if _, err := repo.PayBill(ctx, org, user, bill.ID, bank, "2026-04-20"); err != nil {
		t.Fatal(err)
	}
	// Editing a settled bill must not turn it into a second payment.
	if _, err := repo.SaveExpense(ctx, org, user, bill.ID, Expense{Date: "2026-04-06", VendorName: "Printer Co", CategoryID: travel, Amount: 1000, Paid: true, PaidFromID: bank}); err != nil {
		t.Fatal(err)
	}
	requireBalanced(t, repo, org)
	if ap := func() float64 {
		a, _ := repo.ListAccounts(ctx, org)
		for _, x := range a {
			if x.SystemKey == "ap" {
				return x.Balance
			}
		}
		return -1
	}(); ap != 0 {
		t.Fatalf("payables after payment and edit: %v", ap)
	}

	// Expense report: out-of-pocket claims post only on approval.
	rep, err := repo.SaveReport(ctx, org, user, "Acct", "", ExpenseReport{Title: "Client visit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveExpense(ctx, org, user, "", Expense{Date: "2026-04-10", CategoryID: travel, Amount: 2000, OutOfPocket: true}); err == nil {
		t.Fatal("out-of-pocket without a report must be refused")
	}
	x, err := repo.SaveExpense(ctx, org, user, "", Expense{Date: "2026-04-10", VendorName: "Cab", CategoryID: travel, Amount: 2000, GSTRate: 5, OutOfPocket: true, ReportID: rep.ID})
	if err != nil || x.Posted {
		t.Fatalf("claim: %v %+v", err, x)
	}
	if _, err := repo.ReportAction(ctx, org, user, rep.ID, "approve", "", "", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("approve before submit: %v", err)
	}
	if rep, err = repo.ReportAction(ctx, org, user, rep.ID, "submit", "", "", ""); err != nil || rep.Status != "submitted" || rep.Total != 2100 {
		t.Fatalf("submit: %v %+v", err, rep)
	}
	if rep, err = repo.ReportAction(ctx, org, user, rep.ID, "reject", "need receipt", "", ""); err != nil || rep.Status != "rejected" {
		t.Fatalf("reject: %v %+v", err, rep)
	}
	repo.ReportAction(ctx, org, user, rep.ID, "submit", "", "", "")
	if rep, err = repo.ReportAction(ctx, org, user, rep.ID, "approve", "ok", "", ""); err != nil || rep.Status != "approved" {
		t.Fatalf("approve: %v %+v", err, rep)
	}
	requireBalanced(t, repo, org)
	if emp := func() float64 {
		a, _ := repo.ListAccounts(ctx, org)
		for _, v := range a {
			if v.SystemKey == "emp_payable" {
				return v.Balance
			}
		}
		return -1
	}(); emp != 2100 {
		t.Fatalf("employee payable %v", emp)
	}
	if rep, err = repo.ReportAction(ctx, org, user, rep.ID, "reimburse", "", bank, "2026-04-25"); err != nil || rep.Status != "reimbursed" {
		t.Fatalf("reimburse: %v %+v", err, rep)
	}
	tb := requireBalanced(t, repo, org)
	// Bank: +100000 -11800 -1000 -2100 = 85100.
	var bankBal float64
	a, _ := repo.ListAccounts(ctx, org)
	for _, v := range a {
		if v.ID == bank {
			bankBal = v.Balance
		}
		if v.SystemKey == "emp_payable" && v.Balance != 0 {
			t.Fatalf("employee payable not cleared: %v", v.Balance)
		}
	}
	if bankBal != 85100 {
		t.Fatalf("bank %v (tb %+v)", bankBal, tb.TotalDebit)
	}

	// Reports built from the real ledger balance.
	ps, _ := repo.LoadPostings(ctx, org, "")
	pl := BuildPnL(a, ps, "2026-04-01", "2027-03-31")
	if pl.TotalExpenses != 10000+1000+2000 || pl.NetProfit != -13000 {
		t.Fatalf("pnl %+v", pl)
	}
	g := BuildGST(a, ps, "2026-04-01", "2027-03-31")
	if g.InputTotal != 1800+100 {
		t.Fatalf("gst input %v", g.InputTotal)
	}
	if cf := BuildCashFlow(a, ps, "2026-04-01", "2027-03-31"); !cf.Ties || cf.Closing != 85100 {
		t.Fatalf("cash flow %+v", cf)
	}
	if err := repo.DeleteExpense(ctx, org, user, e.ID); err != nil {
		t.Fatal(err)
	}
	requireBalanced(t, repo, org)
	if err := repo.DeleteExpense(ctx, org, user, x.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reimbursed report expense must be locked: %v", err)
	}
}
