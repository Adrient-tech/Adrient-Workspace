package accounting

import (
	"context"
	"errors"
	"testing"
)

func balanceOf(t *testing.T, repo *Repository, org, key string) float64 {
	t.Helper()
	accts, err := repo.ListAccounts(context.Background(), org)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range accts {
		if a.SystemKey == key {
			return a.Balance
		}
	}
	t.Fatalf("no account %s", key)
	return 0
}

func TestBillLifecycleCreditsAndWriteOff(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	rent := idOf(t, repo, org, "misc")
	bank := idOf(t, repo, org, "bank")
	b, err := repo.SaveBill(ctx, org, user, "", BillInput{Number: "B-1", VendorID: "v1", VendorName: "Landlord", BillDate: "2026-06-01",
		Lines: []BillLineIn{{AccountID: rent, Amount: 1000, TaxRate: 18, Class: "HQ"}}})
	if err != nil || b.Total != 1180 || b.Status != "open" {
		t.Fatalf("bill: %v %+v", err, b)
	}
	if balanceOf(t, repo, org, "ap") != 1180 || balanceOf(t, repo, org, "gst_in_cgst") != 90 {
		t.Fatalf("ap %v gst %v", balanceOf(t, repo, org, "ap"), balanceOf(t, repo, org, "gst_in_cgst"))
	}
	requireBalanced(t, repo, org)
	// editing a posted bill is refused
	if _, err := repo.SaveBill(ctx, org, user, b.ID, BillInput{VendorName: "x", BillDate: "2026-06-01", Lines: []BillLineIn{{AccountID: rent, Amount: 1}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("edit posted: %v", err)
	}
	// overpay refused, partial ok
	if _, err := repo.PayVendorBill(ctx, org, user, b.ID, BillPayment{Date: "2026-06-05", Amount: 2000, AccountID: bank}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overpay: %v", err)
	}
	b, err = repo.PayVendorBill(ctx, org, user, b.ID, BillPayment{Date: "2026-06-05", Amount: 500, AccountID: bank})
	if err != nil || b.Balance != 680 || b.Status != "open" {
		t.Fatalf("partial: %v %+v", err, b)
	}
	// vendor credit of 118 applied
	c, err := repo.SaveBill(ctx, org, user, "", BillInput{Kind: "vendor_credit", Number: "DN-1", VendorID: "v1", VendorName: "Landlord", BillDate: "2026-06-06",
		Lines: []BillLineIn{{AccountID: rent, Amount: 100, TaxRate: 18}}})
	if err != nil || c.Total != 118 {
		t.Fatalf("credit: %v %+v", err, c)
	}
	if balanceOf(t, repo, org, "ap") != 1180-500-118 {
		t.Fatalf("ap after credit %v", balanceOf(t, repo, org, "ap"))
	}
	b, err = repo.ApplyCredit(ctx, org, user, b.ID, c.ID, "2026-06-07", 118)
	if err != nil || b.Balance != 562 {
		t.Fatalf("apply: %v %+v", err, b)
	}
	// AP unchanged by applying (both sides already in AP)
	if balanceOf(t, repo, org, "ap") != 562 {
		t.Fatalf("ap after apply %v", balanceOf(t, repo, org, "ap"))
	}
	// write off the rest
	b, err = repo.WriteOffBill(ctx, org, user, b.ID, "2026-06-30", "settled")
	if err != nil || b.Status != "paid" || balanceOf(t, repo, org, "ap") != 0 {
		t.Fatalf("writeoff: %v %+v ap=%v", err, b, balanceOf(t, repo, org, "ap"))
	}
	requireBalanced(t, repo, org)
	// A paid bill cannot be voided; an unpaid one can.
	if _, err := repo.VoidBill(ctx, org, user, b.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("void paid: %v", err)
	}
	b2, _ := repo.SaveBill(ctx, org, user, "", BillInput{VendorName: "V2", BillDate: "2026-07-01", Lines: []BillLineIn{{AccountID: rent, Amount: 50}}})
	if b2, err = repo.VoidBill(ctx, org, user, b2.ID); err != nil || b2.Status != "void" {
		t.Fatalf("void: %v", err)
	}
	if balanceOf(t, repo, org, "ap") != 0 {
		t.Fatal("void must remove the liability")
	}
}

func TestBillApprovalLimitAndSeparationOfDuties(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	rent := idOf(t, repo, org, "misc")
	st, _ := repo.GetSettings(ctx, org)
	st.ApprovalLimit = 5000
	if _, err := repo.SaveSettings(ctx, org, user, st); err != nil {
		t.Fatal(err)
	}
	small, _ := repo.SaveBill(ctx, org, user, "", BillInput{VendorName: "V", BillDate: "2026-06-01", Lines: []BillLineIn{{AccountID: rent, Amount: 4000}}})
	if small.Status != "open" {
		t.Fatalf("under limit: %s", small.Status)
	}
	big, err := repo.SaveBill(ctx, org, user, "", BillInput{VendorName: "V", BillDate: "2026-06-01", Lines: []BillLineIn{{AccountID: rent, Amount: 9000}}})
	if err != nil || big.Status != "pending_approval" {
		t.Fatalf("over limit: %v %+v", err, big)
	}
	if balanceOf(t, repo, org, "ap") != 4000 {
		t.Fatalf("pending bill must not post: ap=%v", balanceOf(t, repo, org, "ap"))
	}
	if _, err := repo.PayVendorBill(ctx, org, user, big.ID, BillPayment{Date: "2026-06-02", Amount: 1, AccountID: idOf(t, repo, org, "bank")}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("pay pending: %v", err)
	}
	var boss string
	if err := repo.pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		VALUES ($1,'test','subject-boss','boss@adrient.localtest.me','Boss') RETURNING id::text`, org).Scan(&boss); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApproveBill(ctx, org, user, "Me", big.ID, true, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("self-approval must be refused: %v", err)
	}
	if got, err := repo.ApproveBill(ctx, org, boss, "Boss", big.ID, true, "ok"); err != nil || got.Status != "open" || got.ApprovedBy != "Boss" {
		t.Fatalf("approve: %v %+v", err, got)
	}
	if balanceOf(t, repo, org, "ap") != 13000 {
		t.Fatalf("approved bill must post: ap=%v", balanceOf(t, repo, org, "ap"))
	}
	rej, _ := repo.SaveBill(ctx, org, user, "", BillInput{VendorName: "V", BillDate: "2026-06-01", Lines: []BillLineIn{{AccountID: rent, Amount: 8000}}})
	if got, err := repo.ApproveBill(ctx, org, boss, "Boss", rej.ID, false, "no"); err != nil || got.Status != "rejected" {
		t.Fatalf("reject: %v %+v", err, got)
	}
	requireBalanced(t, repo, org)
}

func TestForeignBillPaymentLossAndCreditMemoAndDeposit(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	_, _ = repo.SaveRate(ctx, org, user, Rate{Currency: "USD", Date: "2026-06-01", Rate: 80})
	_, _ = repo.SaveRate(ctx, org, user, Rate{Currency: "USD", Date: "2026-06-20", Rate: 82})
	exp := idOf(t, repo, org, "misc")
	bank := idOf(t, repo, org, "bank")
	b, err := repo.SaveBill(ctx, org, user, "", BillInput{VendorName: "AWS", BillDate: "2026-06-02", Currency: "USD", Lines: []BillLineIn{{AccountID: exp, Amount: 100}}})
	if err != nil || b.Rate != 80 || balanceOf(t, repo, org, "ap") != 8000 {
		t.Fatalf("foreign bill: %v %+v ap=%v", err, b, balanceOf(t, repo, org, "ap"))
	}
	if _, err = repo.PayVendorBill(ctx, org, user, b.ID, BillPayment{Date: "2026-06-21", Amount: 100, AccountID: bank}); err != nil {
		t.Fatal(err)
	}
	if balanceOf(t, repo, org, "ap") != 0 || balanceOf(t, repo, org, "fx_gain_loss") != -200 {
		t.Fatalf("ap %v fx %v (loss of 200 shows as negative income)", balanceOf(t, repo, org, "ap"), balanceOf(t, repo, org, "fx_gain_loss"))
	}
	if balanceOf(t, repo, org, "bank") != -8200 {
		t.Fatalf("bank %v", balanceOf(t, repo, org, "bank"))
	}
	requireBalanced(t, repo, org)

	// credit memo with GST, then refund part
	cm, err := repo.CreateCreditMemo(ctx, org, user, CreditMemoInput{Number: "CN-1", CustomerID: "c1", CustomerName: "Acme", Date: "2026-07-01", Amount: 1000, GSTRate: 18})
	if err != nil || cm.Total != 1180 {
		t.Fatalf("memo: %v %+v", err, cm)
	}
	if balanceOf(t, repo, org, "ar") != -1180 {
		t.Fatalf("ar after memo %v", balanceOf(t, repo, org, "ar"))
	}
	cm, err = repo.RefundCreditMemo(ctx, org, user, cm.ID, "2026-07-02", bank, 1180)
	if err != nil || cm.Balance != 0 || balanceOf(t, repo, org, "ar") != 0 {
		t.Fatalf("refund: %v %+v", err, cm)
	}
	if _, err := repo.RefundCreditMemo(ctx, org, user, cm.ID, "2026-07-02", bank, 1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("over refund: %v", err)
	}
	requireBalanced(t, repo, org)

	// deposits via Undeposited Funds
	if _, _, err := repo.PostSourceJournal(ctx, org, user, SourcePost{Source: SourceBillPayment, SourceID: "rcpt-1", Date: "2026-07-03", Memo: "Receipt A",
		Lines: []PostLine{{Account: "undeposited_funds", Debit: 700}, {Account: "ar", Credit: 700}}}); err != nil {
		t.Fatal(err)
	}
	un, err := repo.ListUndeposited(ctx, org)
	if err != nil || len(un) != 1 || un[0].Amount != 700 {
		t.Fatalf("undeposited: %v %+v", err, un)
	}
	dep, err := repo.CreateDeposit(ctx, org, user, "2026-07-04", bank, "slip 9", []string{un[0].LineID})
	if err != nil || dep.Total != 700 {
		t.Fatalf("deposit: %v %+v", err, dep)
	}
	if _, err := repo.CreateDeposit(ctx, org, user, "2026-07-04", bank, "again", []string{un[0].LineID}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("double deposit: %v", err)
	}
	if balanceOf(t, repo, org, "undeposited_funds") != 0 {
		t.Fatalf("undeposited not cleared: %v", balanceOf(t, repo, org, "undeposited_funds"))
	}
	if err := repo.DeleteDeposit(ctx, org, user, dep.ID); err != nil {
		t.Fatal(err)
	}
	if un, _ = repo.ListUndeposited(ctx, org); len(un) != 1 {
		t.Fatalf("deleted deposit must free the receipt: %d", len(un))
	}
	// receivable write-off
	if _, err := repo.WriteOffReceivable(ctx, org, user, "c1", "Acme", "2026-07-10", 50, "small"); err != nil {
		t.Fatal(err)
	}
	requireBalanced(t, repo, org)
}
