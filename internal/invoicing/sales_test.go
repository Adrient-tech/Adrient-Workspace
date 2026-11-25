package invoicing

import (
	"context"
	"testing"
)

func saveGSTSettings(t *testing.T, repo *Repository, org string) {
	t.Helper()
	s, err := repo.GetSettings(context.Background(), org)
	if err != nil {
		t.Fatal(err)
	}
	s.Sales.StateCode = "29"
	if _, err := repo.SaveSettings(context.Background(), org, s); err != nil {
		t.Fatal(err)
	}
}

func TestEstimateVersionsAcceptConvert(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	saveGSTSettings(t, repo, org)

	est, err := repo.SaveDocument(ctx, org, user, "", DocInput{Kind: "estimate", ClientName: "Acme", PlaceOfSupply: "Maharashtra",
		Lines: []Line{{Description: "Design", Qty: 1, Price: 1000, TaxRate: 18, HSNSAC: "998391"}}})
	if err != nil {
		t.Fatal(err)
	}
	if est.IGST != 180 || est.CGST != 0 || est.Total != 1180 || est.ValidUntil == "" || est.PlaceOfSupply != "27" || est.Version != 1 {
		t.Fatalf("estimate: %+v", est)
	}
	// Not sent yet: not reachable on the public link.
	if _, err := repo.AcceptByToken(ctx, est.ShareToken, Acceptance{Name: "Jo Client", Agree: true}); err == nil {
		t.Fatal("a draft must not be acceptable")
	}
	if _, _, err := repo.MarkSent(ctx, org, est.ID); err != nil {
		t.Fatal(err)
	}
	v2, err := repo.NewVersion(ctx, org, user, est.ID)
	if err != nil || v2.Version != 2 || v2.VersionGroup != est.ID || v2.Number != est.Number+"-v2" {
		t.Fatalf("v2: %v %+v", err, v2)
	}
	old, _ := repo.GetDocument(ctx, org, est.ID)
	if old.Status != "superseded" || old.LatestID != v2.ID {
		t.Fatalf("old: %+v", old)
	}
	vs, _ := repo.Versions(ctx, org, v2.ID)
	if len(vs) != 2 {
		t.Fatalf("versions %d", len(vs))
	}
	// Superseded: cannot be accepted.
	if _, err := repo.AcceptByToken(ctx, est.ShareToken, Acceptance{Name: "Jo Client", Agree: true}); err == nil {
		t.Fatal("superseded must not be acceptable")
	}
	if _, _, err := repo.MarkSent(ctx, org, v2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AcceptByToken(ctx, v2.ShareToken, Acceptance{Name: "J", Agree: true}); err == nil {
		t.Fatal("name too short")
	}
	if _, err := repo.AcceptByToken(ctx, v2.ShareToken, Acceptance{Name: "Jo Client"}); err == nil {
		t.Fatal("needs the confirmation box")
	}
	acc, err := repo.AcceptByToken(ctx, v2.ShareToken, Acceptance{Name: "Jo Client", Agree: true, IP: "1.2.3.4"})
	if err != nil || acc.Status != "accepted" || acc.AcceptedName != "Jo Client" || len(acc.AcceptedHash) != 64 {
		t.Fatalf("accept: %v %+v", err, acc)
	}
	again, err := repo.AcceptByToken(ctx, v2.ShareToken, Acceptance{Name: "Someone Else", Agree: true})
	if err != nil || again.AcceptedName != "Jo Client" {
		t.Fatalf("accepting twice must keep the first acceptance: %v %+v", err, again)
	}
	inv, err := repo.ConvertEstimateTo(ctx, org, user, v2.ID)
	if err != nil || inv.Kind != "invoice" || inv.Total != 1180 || inv.IGST != 180 {
		t.Fatalf("invoice: %v %+v", err, inv)
	}
	inv2, err := repo.ConvertEstimateTo(ctx, org, user, v2.ID)
	if err != nil || inv2.ID != inv.ID {
		t.Fatalf("converting twice must return the same invoice: %v", err)
	}
}

func TestCreditNoteFlow(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	saveGSTSettings(t, repo, org)
	inv, err := repo.SaveDocument(ctx, org, user, "", DocInput{ClientName: "Acme", PlaceOfSupply: "29",
		Lines: []Line{{Description: "A", Qty: 2, Price: 500, TaxRate: 18}}}) // 1000 + 180
	if err != nil || inv.Total != 1180 || inv.CGST != 90 || inv.SGST != 90 {
		t.Fatalf("inv %v %+v", err, inv)
	}
	if _, err := repo.CreateCreditNote(ctx, org, user, inv.ID, CreditInput{}); err == nil {
		t.Fatal("draft invoice cannot be credited")
	}
	repo.MarkSent(ctx, org, inv.ID)
	inv, _ = repo.AddPayment(ctx, org, user, inv.ID, Payment{Amount: 400, Method: "upi"})
	// Credit one unit (590) while 780 is outstanding: applied to the balance.
	cn, err := repo.CreateCreditNote(ctx, org, user, inv.ID, CreditInput{Lines: []Line{{Description: "A", Qty: 1, Price: 500, TaxRate: 18}}, Reason: "returned"})
	if err != nil || cn.Kind != "credit_note" || cn.Total != 590 || cn.CreditRemain != 0 || cn.DisplayStatus != "applied" || cn.Number != "CN-0001" {
		t.Fatalf("cn: %v %+v", err, cn)
	}
	inv, _ = repo.GetDocument(ctx, org, inv.ID)
	if inv.Credited != 590 || inv.Paid != 400 || inv.Balance != 190 {
		t.Fatalf("inv after credit: %+v", inv)
	}
	// Over-crediting is refused: only 590 of 1180 is left to credit.
	if _, err := repo.CreateCreditNote(ctx, org, user, inv.ID, CreditInput{Lines: []Line{{Description: "x", Qty: 2, Price: 500, TaxRate: 18}}}); err == nil {
		t.Fatal("must refuse more than the invoice total")
	}
	// Credit the rest, pay the balance, and what exceeds the balance stays refundable.
	repo.AddPayment(ctx, org, user, inv.ID, Payment{Amount: 190, Method: "cash"})
	cn2, err := repo.CreateCreditNote(ctx, org, user, inv.ID, CreditInput{Lines: []Line{{Description: "A", Qty: 1, Price: 500, TaxRate: 18}}})
	if err != nil || cn2.CreditRemain != 590 || cn2.DisplayStatus != "open" {
		t.Fatalf("cn2: %v %+v", err, cn2)
	}
	if _, err := repo.RefundCreditNote(ctx, org, user, cn2.ID, 600, "upi", ""); err == nil {
		t.Fatal("refund above the credit left must fail")
	}
	cn2, err = repo.RefundCreditNote(ctx, org, user, cn2.ID, 0, "upi", "ref1")
	if err != nil || cn2.CreditRemain != 0 || cn2.DisplayStatus != "refunded" {
		t.Fatalf("refund: %v %+v", err, cn2)
	}
	// Accounting reads cash rows only: credit/applied rows are not in the payments feed.
	ps, _ := repo.ListPayments(ctx, org, "", "")
	for _, p := range ps {
		if p.Kind == "credit" || p.Kind == "applied" {
			t.Fatalf("credit row leaked into the cash feed: %+v", p)
		}
	}
}

func TestRecurringIdempotent(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	rc, err := repo.SaveRecurring(ctx, org, "", Recurring{Name: "Retainer", Frequency: "monthly", Active: true,
		NextRun: today().AddDate(0, 0, -1).Format("2006-01-02"),
		Template: DocInput{ClientName: "Acme", Lines: []Line{{Description: "Retainer", Qty: 1, Price: 100}}}})
	if err != nil {
		t.Fatal(err)
	}
	made, _, err := repo.RunRecurring(ctx, org, user)
	if err != nil || len(made) != 1 {
		t.Fatalf("first run: %v %d", err, len(made))
	}
	// Rewind the template as if a second worker ran at the same time: the period is not billed twice.
	if _, err := repo.pool.Exec(ctx, `UPDATE adrient.invc_recurring SET next_run=$2::date WHERE id=$1::uuid`, rc.ID, today().AddDate(0, 0, -1).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	made, _, err = repo.RunRecurring(ctx, org, user)
	if err != nil || len(made) != 0 {
		t.Fatalf("second run must create nothing: %v %d", err, len(made))
	}
	all, _ := repo.ListDocuments(ctx, org, DocFilter{Kind: "invoice"})
	if len(all) != 1 {
		t.Fatalf("invoices: %d", len(all))
	}
}

func TestCreateEstimateFromDealIdempotent(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	d1, created, err := repo.CreateEstimateFromDeal(ctx, org, user, DealEstimate{DealID: "deal-1", Title: "Website", ClientName: "Acme", Value: 5000, TaxRate: 18})
	if err != nil || !created || d1.Kind != "estimate" || d1.Total != 5900 || d1.SourceRef != "crm:deal:deal-1" {
		t.Fatalf("%v %v %+v", err, created, d1)
	}
	d2, created, err := repo.CreateEstimateFromDeal(ctx, org, user, DealEstimate{DealID: "deal-1", Value: 1})
	if err != nil || created || d2.ID != d1.ID {
		t.Fatalf("second call must return the same estimate: %v %v", err, created)
	}
}
