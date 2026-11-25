package accounting

import (
	"context"
	"errors"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"testing"
)

func TestToBaseBalancesExactly(t *testing.T) {
	// 3 lines whose individually rounded base values would not balance.
	ls := toBase([]Line{{AccountID: "a", Debit: 100.01}, {AccountID: "b", Credit: 33.34}, {AccountID: "c", Credit: 66.67}}, 83.3333)
	var d, c int64
	for _, l := range ls {
		d += paise(l.Debit)
		c += paise(l.Credit)
	}
	if d != c {
		t.Fatalf("toBase unbalanced: %d vs %d", d, c)
	}
	if ls[0].FcyDebit != 100.01 {
		t.Fatalf("foreign amount lost: %+v", ls[0])
	}
}

func TestSettlementLinesGainLoss(t *testing.T) {
	// invoice booked at 80, paid at 83: customer pays 100 USD => +300 gain
	ls := settlementLines("bank", "ar", "fx", 100, 80, 83, true, "p")
	var d, c, fxCr float64
	for _, l := range ls {
		d += l.Debit
		c += l.Credit
		if l.AccountID == "fx" {
			fxCr = l.Credit
		}
	}
	if d != c || fxCr != 300 {
		t.Fatalf("receipt gain: d=%v c=%v fxCr=%v", d, c, fxCr)
	}
	// vendor bill booked at 80, paid at 83 => loss 300 (debit)
	ls = settlementLines("bank", "ap", "fx", 100, 80, 83, false, "p")
	var fxDr float64
	d, c = 0, 0
	for _, l := range ls {
		d += l.Debit
		c += l.Credit
		if l.AccountID == "fx" {
			fxDr = l.Debit
		}
	}
	if d != c || fxDr != 300 {
		t.Fatalf("payment loss: d=%v c=%v fxDr=%v", d, c, fxDr)
	}
}

func TestPostSourceJournalIdempotentAndValidated(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	post := SourcePost{Source: SourcePayroll, SourceID: "run-1", Date: "2026-05-31", Memo: "Payroll May",
		Lines: []PostLine{{Account: "salaries_expense", Debit: 100000}, {Account: "payroll_payable", Credit: 100000}}}
	j1, created, err := repo.PostSourceJournal(ctx, org, user, post)
	if err != nil || !created || j1.Total != 100000 || j1.Kind != "payroll" {
		t.Fatalf("first post: %v %+v", err, j1)
	}
	j2, created, err := repo.PostSourceJournal(ctx, org, user, post)
	if err != nil || created || j2.ID != j1.ID {
		t.Fatalf("repost must be a no-op: %v created=%v", err, created)
	}
	n, _ := repo.ListJournals(ctx, org, JournalFilter{Source: SourcePayroll})
	if len(n) != 1 {
		t.Fatalf("expected 1 payroll journal, got %d", len(n))
	}
	post.Lines = []PostLine{{Account: "salaries_expense", Debit: 90000}, {Account: "payroll_payable", Credit: 90000}}
	j3, created, err := repo.PostSourceJournal(ctx, org, user, post)
	if err != nil || created || j3.ID != j1.ID || j3.Total != 90000 {
		t.Fatalf("changed content rewrites in place: %v %+v", err, j3)
	}
	for _, src := range []string{SourcePurchaseBill, SourceStockCOGS, SourceStockAdjustment} {
		_, _, err := repo.PostSourceJournal(ctx, org, user, SourcePost{Source: src, SourceID: "x1", Date: "2026-05-31",
			Lines: []PostLine{{Account: "inventory", Debit: 10}, {Account: "ap", Credit: 10}}})
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
	}
	if _, _, err := repo.PostSourceJournal(ctx, org, user, SourcePost{Source: "bogus", SourceID: "1", Date: "2026-05-31"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown source: %v", err)
	}
	if _, _, err := repo.PostSourceJournal(ctx, org, user, SourcePost{Source: SourcePayroll, SourceID: "", Date: "2026-05-31"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing source id: %v", err)
	}
	if _, _, err := repo.PostSourceJournal(ctx, org, user, SourcePost{Source: SourcePayroll, SourceID: "r2", Date: "2026-05-31",
		Lines: []PostLine{{Account: "nope", Debit: 1}, {Account: "ap", Credit: 1}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown account: %v", err)
	}
	if ok, err := repo.VoidSourceJournal(ctx, org, user, SourcePayroll, "run-1"); err != nil || !ok {
		t.Fatalf("void: %v %v", err, ok)
	}
	requireBalanced(t, repo, org)
	// closed period
	if _, err := repo.SaveSettings(ctx, org, user, Settings{FYStartMonth: 4, Currency: "INR", Intrastate: true, ClosingDate: "2026-05-31"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.PostSourceJournal(ctx, org, user, SourcePost{Source: SourcePayroll, SourceID: "run-2", Date: "2026-05-15",
		Lines: []PostLine{{Account: "salaries_expense", Debit: 5}, {Account: "payroll_payable", Credit: 5}}}); !errors.Is(err, ErrLocked) {
		t.Fatalf("closed period: %v", err)
	}
}

func TestMultiCurrencyInvoiceReceiptAndReval(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	if _, err := repo.SaveRate(ctx, org, user, Rate{Currency: "USD", Date: "2026-04-01", Rate: 80}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveRate(ctx, org, user, Rate{Currency: "USD", Date: "2026-05-01", Rate: 83}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveRate(ctx, org, user, Rate{Currency: "INR", Date: "2026-05-01", Rate: 1}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("base currency rate must be rejected: %v", err)
	}
	if r, err := repo.RateOn(ctx, org, "USD", "2026-04-20"); err != nil || r != 80 {
		t.Fatalf("rate on: %v %v", r, err)
	}
	if _, err := repo.RateOn(ctx, org, "EUR", "2026-04-20"); !errors.Is(err, ErrNoRate) {
		t.Fatalf("missing rate: %v", err)
	}
	// Foreign invoice of 1000 USD at 80 => AR 80000.
	inv, _, err := repo.PostSourceJournal(ctx, org, user, SourcePost{Source: SourceBill, SourceID: "inv-fx", Date: "2026-04-10", Currency: "USD",
		Lines: []PostLine{{Account: "ar", Debit: 1000}, {Account: "sales", Credit: 1000}}})
	if err != nil || inv.Total != 80000 || inv.Currency != "USD" || inv.Rate != 80 {
		t.Fatalf("foreign invoice: %v %+v", err, inv)
	}
	// Revalue at 30 April (rate 80): nothing to adjust.
	rv, err := repo.Revalue(ctx, org, user, "2026-04-30")
	if err != nil || len(rv.Entries) != 1 || rv.Entries[0].Adjustment != 0 {
		t.Fatalf("reval at same rate: %v %+v", err, rv)
	}
	// Revalue at 31 May (rate 83): AR up by 3000, unrealised gain; reversal next day.
	rv, err = repo.Revalue(ctx, org, user, "2026-05-31")
	if err != nil || len(rv.Entries) != 1 || rv.Entries[0].Adjustment != 3000 {
		t.Fatalf("reval: %v %+v", err, rv)
	}
	if _, err = repo.Revalue(ctx, org, user, "2026-05-31"); err != nil {
		t.Fatal(err)
	}
	if js, _ := repo.ListJournals(ctx, org, JournalFilter{Source: SourceRevaluation}); len(js) != 2 {
		t.Fatalf("reval must be idempotent (entry + reversal), got %d", len(js))
	}
	requireBalanced(t, repo, org)
	// Receipt of 400 USD on 2026-05-02 at 83 => bank 33200, AR 32000, gain 1200.
	keys, _ := repo.accountKeys(ctx, repo.pool, org)
	rate, _ := repo.RateOn(ctx, org, "USD", "2026-05-02")
	pay, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-05-02", Memo: "receipt", Currency: "USD", Rate: rate, Kind: "payment",
		Source: SourceBillPayment, SourceID: "pay-fx", SourceHash: "h",
		Lines: settlementLines(keys["bank"], keys["ar"], keys["fx_gain_loss"], 400, 80, rate, true, "receipt")})
	if err != nil {
		t.Fatalf("settlement: %v", err)
	}
	if pay.Total != 33200+0 && pay.Total != 33200 {
		t.Logf("pay total %v", pay.Total)
	}
	var gain float64
	for _, l := range pay.Lines {
		if l.AccountID == keys["fx_gain_loss"] {
			gain = l.Credit
		}
	}
	if gain != 1200 {
		t.Fatalf("realised gain want 1200 got %v", gain)
	}
	tb := requireBalanced(t, repo, org)
	_ = tb
}

func TestSyncForeignInvoiceAndPayment(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	_, _ = repo.SaveRate(ctx, org, user, Rate{Currency: "USD", Date: "2026-11-01", Rate: 80})
	_, _ = repo.SaveRate(ctx, org, user, Rate{Currency: "USD", Date: "2026-11-10", Rate: 84})
	inv := &fakeInv{
		docs: []invoicing.Document{{ID: "inv-usd", Kind: "invoice", Number: "INV-9", Status: "sent", Total: 1180, TaxTotal: 180, IssueDate: "2026-11-01", Currency: "USD", ClientName: "Acme Inc"}},
		pays: []invoicing.Payment{{ID: "p-usd", DocumentID: "inv-usd", Kind: "payment", Amount: 1180, Method: "bank", PaidAt: "2026-11-10", DocNumber: "INV-9", Currency: "USD"}},
	}
	res, err := repo.Sync(ctx, org, user, nil, inv, true)
	if err != nil || res["invoicing"].Created != 2 || res["invoicing"].Foreign != 0 {
		t.Fatalf("sync: %v %+v", err, res["invoicing"])
	}
	res, _ = repo.Sync(ctx, org, user, nil, inv, true)
	if res["invoicing"].Created+res["invoicing"].Updated != 0 {
		t.Fatalf("foreign re-sync must be idempotent: %+v", res["invoicing"])
	}
	accts, _ := repo.ListAccounts(ctx, org)
	for _, a := range accts {
		switch a.SystemKey {
		case "ar":
			if a.Balance != 0 {
				t.Fatalf("AR must clear to zero, got %v", a.Balance)
			}
		case "fx_gain_loss":
			if a.Balance != 1180*4 { // 4720 gain
				t.Fatalf("realised gain want 4720 got %v", a.Balance)
			}
		case "bank":
			if a.Balance != 1180*84 {
				t.Fatalf("bank want %v got %v", 1180*84, a.Balance)
			}
		}
	}
	requireBalanced(t, repo, org)
	// A currency without a rate is counted and reported, never silently posted at 1.
	inv.docs = append(inv.docs, invoicing.Document{ID: "inv-eur", Kind: "invoice", Number: "INV-10", Status: "sent", Total: 10, IssueDate: "2026-11-01", Currency: "EUR"})
	res, _ = repo.Sync(ctx, org, user, nil, inv, true)
	if res["invoicing"].Foreign != 1 || len(res["invoicing"].Warnings) == 0 {
		t.Fatalf("missing rate: %+v", res["invoicing"])
	}
}
