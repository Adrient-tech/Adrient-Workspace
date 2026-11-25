package accounting

import (
	"context"
	"os"
	"testing"
)

func readTD(t *testing.T, name string) string {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseOFX(t *testing.T) {
	ls, err := ParseOFX(readTD(t, "sample.ofx"))
	if err != nil || len(ls) != 2 {
		t.Fatalf("ofx: %v %v", err, ls)
	}
	if ls[0].Date != "2026-01-15" || ls[0].Amount != -125050 || ls[0].Description != "ELECTRICITY BOARD - Bill Jan & fees" || ls[1].Amount != 5000000 {
		t.Fatalf("lines: %+v", ls)
	}
	if _, err := ParseOFX("not an ofx"); err == nil {
		t.Fatal("garbage must fail")
	}
}

func TestMigrationImportsDryRunAndRerun(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	run := func(req ImportRequest) ImportReport {
		t.Helper()
		rep, err := repo.RunImport(ctx, org, user, req)
		if err != nil {
			t.Fatalf("%s: %v", req.Type, err)
		}
		return rep
	}
	// chart of accounts: dry run changes nothing, run creates, re-run is a no-op
	_ = repo.EnsureSystemAccounts(ctx, org)
	before, _ := repo.ListAccounts(ctx, org)
	rep := run(ImportRequest{Type: "coa", Source: "zoho", CSV: readTD(t, "zoho_chart_of_accounts.csv"), DryRun: true})
	if rep.OK != 6 || rep.Errors != 2 || rep.Applied != 0 {
		t.Fatalf("coa dry: %+v", rep)
	}
	if after, _ := repo.ListAccounts(ctx, org); len(after) != len(before) {
		t.Fatal("dry run must not write")
	}
	rep = run(ImportRequest{Type: "coa", Source: "zoho", CSV: readTD(t, "zoho_chart_of_accounts.csv")})
	if rep.Applied != 6 || rep.Errors != 2 {
		t.Fatalf("coa run: %+v", rep)
	}
	rep = run(ImportRequest{Type: "coa", Source: "zoho", CSV: readTD(t, "zoho_chart_of_accounts.csv")})
	if rep.Applied != 0 || rep.Existing != 6 || rep.Errors != 2 {
		t.Fatalf("coa rerun: %+v", rep)
	}
	// opening balances
	rep = run(ImportRequest{Type: "opening_balances", Source: "zoho", CSV: readTD(t, "zoho_trial_balance.csv"), AsOf: "2026-04-01"})
	if rep.Errors != 1 || rep.Applied != 0 {
		t.Fatalf("opening with a bad row posts nothing: %+v", rep)
	}
	good := "Account,Debit,Credit\nPetty Cash,5000.00,\nHDFC Current,120000.00,\nTrade Debtors,45000.00,\nSundry Creditors,,30000.00\nZoho Income,,100000.00\n"
	rep = run(ImportRequest{Type: "opening_balances", Source: "zoho", CSV: good, AsOf: "2026-04-01"})
	if rep.Applied == 0 || rep.Errors != 0 {
		t.Fatalf("opening: %+v", rep)
	}
	if again := run(ImportRequest{Type: "opening_balances", Source: "zoho", CSV: good, AsOf: "2026-04-01"}); again.Applied != 0 {
		t.Fatalf("opening rerun: %+v", again)
	}
	if n, _ := repo.ListJournals(ctx, org, JournalFilter{Source: "manual_import"}); len(n) != 1 {
		t.Fatalf("one opening journal expected, got %d", len(n))
	}
	for _, a := range mustAccts(t, repo, org) {
		if a.Name == "HDFC Current" && a.Balance != 120000 {
			t.Fatalf("HDFC balance %v", a.Balance)
		}
	}
	requireBalanced(t, repo, org)
	// invoices, open items: one open, one paid (skipped), draft skipped, bad total error
	rep = run(ImportRequest{Type: "invoices", Source: "zoho", CSV: readTD(t, "zoho_invoices.csv")})
	if rep.Applied != 1 || rep.Errors != 1 || rep.Existing != 2 {
		t.Fatalf("invoices: %+v", rep)
	}
	if rep2 := run(ImportRequest{Type: "invoices", Source: "zoho", CSV: readTD(t, "zoho_invoices.csv")}); rep2.Applied != 0 {
		t.Fatalf("invoice rerun duplicated: %+v", rep2)
	}
	if balanceOf(t, repo, org, "ar") != 11800 {
		t.Fatalf("ar %v", balanceOf(t, repo, org, "ar"))
	}
	// history mode on a second source keeps full sales and GST, with the paid part received
	rep = run(ImportRequest{Type: "invoices", Source: "quickbooks", Mode: "history", CSV: readTD(t, "zoho_invoices.csv")})
	if rep.Applied != 2 {
		t.Fatalf("history: %+v", rep)
	}
	if balanceOf(t, repo, org, "gst_out_cgst") != 1350 {
		t.Fatalf("gst %v", balanceOf(t, repo, org, "gst_out_cgst"))
	}
	// bills
	rep = run(ImportRequest{Type: "bills", Source: "zoho", CSV: readTD(t, "zoho_bills.csv")})
	if rep.Applied != 1 || rep.Existing != 1 {
		t.Fatalf("bills: %+v", rep)
	}
	if rep2 := run(ImportRequest{Type: "bills", Source: "zoho", CSV: readTD(t, "zoho_bills.csv")}); rep2.Applied != 0 {
		t.Fatalf("bill rerun: %+v", rep2)
	}
	bills, _ := repo.ListBills(ctx, org, "bill", "", "")
	if len(bills) != 1 || bills[0].Total != 25000 {
		t.Fatalf("bills %+v", bills)
	}
	// contacts: GSTIN validation, GST party saved, no ContactSink here
	rep = run(ImportRequest{Type: "contacts", Source: "quickbooks", CSV: readTD(t, "qbo_contacts.csv")})
	if rep.Errors != 2 || rep.Applied != 1 {
		t.Fatalf("contacts: %+v", rep)
	}
	if ps, _ := repo.ListGSTParties(ctx, org); len(ps) != 1 || ps[0].StateCode != "27" {
		t.Fatalf("parties %+v", ps)
	}
	// unknown source / type are rejected
	if _, err := repo.RunImport(ctx, org, user, ImportRequest{Type: "x", CSV: "a,b\n1,2"}); err == nil {
		t.Fatal("bad type")
	}
	if _, err := repo.RunImport(ctx, org, user, ImportRequest{Type: "coa", Source: "sage", CSV: "a,b\n1,2"}); err == nil {
		t.Fatal("bad source")
	}
	requireBalanced(t, repo, org)

	// OFX into a bank account, twice
	bank := idOf(t, repo, org, "bank")
	res, err := repo.ImportOFX(ctx, org, user, bank, readTD(t, "sample.ofx"))
	if err != nil || res.Imported != 2 {
		t.Fatalf("ofx import: %v %+v", err, res)
	}
	if res, _ = repo.ImportOFX(ctx, org, user, bank, readTD(t, "sample.ofx")); res.Imported != 0 || res.Duplicates != 2 {
		t.Fatalf("ofx rerun: %+v", res)
	}
}
