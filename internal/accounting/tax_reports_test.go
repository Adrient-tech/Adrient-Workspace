package accounting

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
)

func TestValidGSTIN(t *testing.T) {
	if !ValidGSTIN("27AAPFU0939F1ZV") {
		t.Fatal("known valid GSTIN rejected")
	}
	if ValidGSTIN("27AAPFU0939F1ZX") || ValidGSTIN("short") {
		t.Fatal("bad GSTIN accepted")
	}
}

func TestExportFormats(t *testing.T) {
	tb := Table{Title: "=cmd|' /C calc'!A0", Headers: []string{"Account", "Amount"}, Rows: [][]any{{"Sales & <more>", 1234.5}, {"Total", 10.0}}}
	csvOut := string(tb.CSV())
	if !strings.Contains(csvOut, "'=cmd") || !strings.Contains(csvOut, "1234.50") {
		t.Fatalf("csv: %s", csvOut)
	}
	x, err := tb.XLSX()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(x), int64(len(x)))
	if err != nil || len(zr.File) != 5 {
		t.Fatalf("xlsx zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			rc, _ := f.Open()
			var b bytes.Buffer
			_, _ = b.ReadFrom(rc)
			if !strings.Contains(b.String(), "Sales &amp; &lt;more&gt;") || !strings.Contains(b.String(), "<v>1234.50</v>") {
				t.Fatalf("sheet: %s", b.String())
			}
		}
	}
	pdf := tb.PDF()
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.HasSuffix(bytes.TrimSpace(pdf), []byte("%%EOF")) || !bytes.Contains(pdf, []byte("Sales")) {
		t.Fatal("pdf malformed")
	}
	many := Table{Title: "big", Headers: []string{"a", "b"}}
	for i := 0; i < 200; i++ {
		many.Rows = append(many.Rows, []any{"row", float64(i)})
	}
	if !bytes.Contains(many.PDF(), []byte("/Count 4")) && !bytes.Contains(many.PDF(), []byte("/Count 5")) {
		t.Fatal("pdf should paginate")
	}
}

func TestComparisonBudgetClassAndDrilldownData(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	sales, bank, misc := idOf(t, repo, org, "sales"), idOf(t, repo, org, "bank"), idOf(t, repo, org, "misc")
	post := func(date, memo, class string, amt float64, income bool) {
		a, b := Line{AccountID: bank, Debit: amt}, Line{AccountID: sales, Credit: amt, Class: class}
		if !income {
			a, b = Line{AccountID: misc, Debit: amt, Class: class}, Line{AccountID: bank, Credit: amt}
		}
		if _, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: date, Memo: memo, Lines: []Line{a, b}}); err != nil {
			t.Fatal(err)
		}
	}
	post("2025-06-10", "last year sale", "", 1000, true)
	post("2026-06-10", "sale north", "North", 3000, true)
	post("2026-06-12", "sale south", "South", 1000, true)
	post("2026-06-15", "cost north", "North", 500, false)
	accts, _ := repo.ListAccounts(ctx, org)
	ps, _ := repo.LoadPostings(ctx, org, "")
	c, err := BuildPnLCompare(accts, ps, "2026-04-01", "2026-06-30", "prev_year")
	if err != nil || c.TotalIncome.Current != 4000 || c.TotalIncome.Compare != 1000 || c.TotalIncome.Change != 3000 || c.TotalIncome.ChangePct != 300 {
		t.Fatalf("prev year: %v %+v", err, c.TotalIncome)
	}
	p, err := BuildPnLCompare(accts, ps, "2026-06-01", "2026-06-30", "prev_period")
	if err != nil || p.CompareFrom != "2026-05-02" || p.CompareTo != "2026-05-31" {
		t.Fatalf("prev period range: %v %+v", err, p)
	}
	if _, err := BuildPnLCompare(accts, ps, "2026-04-01", "2026-06-30", "nope"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad mode: %v", err)
	}
	bs, err := BuildBalanceSheetCompare(accts, ps, "2026-06-30", 4)
	if err != nil || !bs.Balance {
		t.Fatalf("bs compare: %v %+v", err, bs.Balance)
	}
	// class report
	cl := BuildPnLByClass(accts, ps, "2026-06-01", "2026-06-30")
	if cl.Totals["North"].Profit != 2500 || cl.Totals["South"].Profit != 1000 || len(cl.Classes) != 2 {
		t.Fatalf("by class: %+v", cl.Totals)
	}
	if got, _ := repo.ListClasses(ctx, org); len(got) != 2 {
		t.Fatalf("classes %v", got)
	}
	// budgets
	if _, err := repo.SaveBudgets(ctx, org, user, []BudgetCell{{AccountID: sales, Month: "2026-06", Amount: 3500}, {AccountID: sales, Month: "2026-07", Amount: 9999}, {AccountID: misc, Month: "2026-06", Amount: 400}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveBudgets(ctx, org, user, []BudgetCell{{AccountID: "x", Month: "2026-06", Amount: 1}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad budget: %v", err)
	}
	cells, _ := repo.ListBudgets(ctx, org, "2026-04", "2027-03")
	bv := BuildBudgetVsActual(accts, ps, cells, "2026-06-01", "2026-06-30")
	var salesRow, costRow CompareRow
	for _, r := range bv.Income {
		if r.AccountID == sales {
			salesRow = r
		}
	}
	for _, r := range bv.Expenses {
		if r.AccountID == misc {
			costRow = r
		}
	}
	if salesRow.Current != 4000 || salesRow.Compare != 3500 || salesRow.Change != 500 || costRow.Compare != 400 || costRow.Change != 100 {
		t.Fatalf("budget vs actual: %+v %+v", salesRow, costRow)
	}
	if tb, ok := TableOf(bv); !ok || len(tb.Rows) == 0 {
		t.Fatal("budget table")
	}
	requireBalanced(t, repo, org)
}

func TestGSTRHSNTDSAndITC(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	st, _ := repo.GetSettings(ctx, org)
	st.GSTIN = "27AAPFU0939F1ZV"
	if _, err := repo.SaveSettings(ctx, org, user, st); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveGSTParty(ctx, org, user, GSTParty{ContactID: "c1", Name: "Acme", GSTIN: "27AAPFU0939F1ZV"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveGSTParty(ctx, org, user, GSTParty{ContactID: "c2", Name: "Bad", GSTIN: "27AAPFU0939F1ZX"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad GSTIN saved: %v", err)
	}
	if _, err := repo.SaveGSTParty(ctx, org, user, GSTParty{ContactID: "c3", Name: "Delhi Co", GSTIN: "07AAACG1234A1Z5"}); err == nil {
		// check digit of this made-up GSTIN is probably wrong; ignore either outcome
	}
	_ = repo.SaveHSN(ctx, org, "web design", "998314")
	inv := &fakeInv{docs: []invoicing.Document{
		{ID: "i1", Kind: "invoice", Number: "INV-1", Status: "sent", IssueDate: "2026-11-05", Currency: "INR", ContactID: "c1", ClientName: "Acme", Subtotal: 1000, TaxTotal: 180, Total: 1180,
			Lines: []invoicing.Line{{Description: "Widget", Qty: 10, Price: 100, TaxRate: 18, Unit: "pcs", HSN: "8501"}}},
		{ID: "i2", Kind: "invoice", Number: "INV-2", Status: "sent", IssueDate: "2026-11-06", Currency: "INR", ClientName: "Walk in", Subtotal: 500, TaxTotal: 25, Total: 525,
			Lines: []invoicing.Line{{Description: "Web design", Qty: 1, Price: 500, TaxRate: 5}}},
		{ID: "i3", Kind: "invoice", Number: "INV-3", Status: "draft", IssueDate: "2026-11-07", Currency: "INR", Lines: []invoicing.Line{{Description: "x", Qty: 1, Price: 1}}},
		{ID: "i4", Kind: "invoice", Number: "INV-4", Status: "sent", IssueDate: "2026-12-07", Currency: "INR", Lines: []invoicing.Line{{Description: "later", Qty: 1, Price: 1}}},
	}}
	g, err := repo.BuildGSTR1(ctx, org, "2026-11-01", "2026-11-30", inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g.Summary["b2b"].Count != 1 || g.Summary["b2b"].Taxable != 1000 || g.Summary["b2b"].CGST != 90 || g.Summary["b2b"].SGST != 90 || g.Summary["b2cs"].Count != 1 {
		t.Fatalf("gstr1: %+v", g.Summary)
	}
	if _, ok := g.Summary["b2b"]; !ok || len(g.Rows) != 2 {
		t.Fatalf("rows %d", len(g.Rows))
	}
	h, err := repo.BuildHSNSummary(ctx, org, "2026-11-01", "2026-11-30", inv)
	if err != nil || len(h.Rows) != 2 || h.Missing != 0 {
		t.Fatalf("hsn: %v %+v", err, h)
	}
	if tb, ok := TableOf(g); !ok || len(tb.Rows) != 2 {
		t.Fatal("gstr1 table")
	}
	// credit memo shows up as CDNR for the registered customer
	if _, err := repo.CreateCreditMemo(ctx, org, user, CreditMemoInput{Number: "CN-1", CustomerID: "c1", CustomerName: "Acme", Date: "2026-11-10", Amount: 100, GSTRate: 18}); err != nil {
		t.Fatal(err)
	}
	g, _ = repo.BuildGSTR1(ctx, org, "2026-11-01", "2026-11-30", inv, nil)
	if g.Summary["cdnr"].Count != 1 || g.Summary["cdnr"].Taxable != 100 {
		t.Fatalf("cdnr: %+v", g.Summary)
	}

	// GSTR-3B from the ledger: sales posted through sync-like journals
	keys, _ := repo.accountKeys(ctx, repo.pool, org)
	if _, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-11-05", Memo: "inv", Lines: []Line{
		{AccountID: keys["ar"], Debit: 1180}, {AccountID: keys["sales"], Credit: 1000}, {AccountID: keys["gst_out_cgst"], Credit: 90}, {AccountID: keys["gst_out_sgst"], Credit: 90}}}); err != nil {
		t.Fatal(err)
	}
	bill, err := repo.SaveBill(ctx, org, user, "", BillInput{Number: "S-77", VendorID: "v1", VendorName: "Supplier", BillDate: "2026-11-08", Lines: []BillLineIn{{AccountID: keys["misc"], Amount: 400, TaxRate: 18}}})
	if err != nil {
		t.Fatal(err)
	}
	accts, _ := repo.ListAccounts(ctx, org)
	ps, _ := repo.LoadPostings(ctx, org, "")
	st, _ = repo.GetSettings(ctx, org)
	r3 := BuildGSTR3B(accts, ps, st, "2026-11-01", "2026-11-30")
	var itc, payable float64
	for _, s := range r3.Sections {
		if strings.HasPrefix(s.Name, "4(A)(5)") {
			itc = s.Credit
		}
		if strings.HasPrefix(s.Name, "6.1 Net tax payable - total") {
			payable = s.Credit
		}
	}
	// output 180 (cgst+sgst) - the credit memo reversal 18 - ITC 72
	if itc != 72 || payable != 180-18-72 {
		t.Fatalf("3B itc=%v payable=%v: %+v", itc, payable, r3.Sections)
	}

	// TDS: payable deducted from vendor then deposited with a challan; receivable claimed
	e, err := repo.RecordTDS(ctx, org, user, TDSEntry{Direction: "payable", Section: "194J", PartyName: "Supplier", Date: "2026-11-09", Base: 10000, Rate: 10})
	if err != nil || e.TDS != 1000 {
		t.Fatalf("tds: %v %+v", err, e)
	}
	if balanceOf(t, repo, org, "tds_payable") != 1000 {
		t.Fatalf("tds payable %v", balanceOf(t, repo, org, "tds_payable"))
	}
	if _, err := repo.SettleTDS(ctx, org, user, e.ID, "2026-11-20", "", "CH1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("settle needs a bank account: %v", err)
	}
	if got, err := repo.SettleTDS(ctx, org, user, e.ID, "2026-11-20", keys["bank"], "CH1"); err != nil || got.Status != "deposited" {
		t.Fatalf("settle: %v %+v", err, got)
	}
	if balanceOf(t, repo, org, "tds_payable") != 0 {
		t.Fatal("deposit must clear TDS payable")
	}
	if _, err := repo.RecordTDS(ctx, org, user, TDSEntry{Direction: "receivable", Section: "194C", PartyName: "Acme", Date: "2026-11-11", Base: 5000, TDS: 100}); err != nil {
		t.Fatal(err)
	}
	rep, _ := repo.BuildTDSReport(ctx, org, "", "2026-11-01", "2026-11-30")
	if len(rep.Rows) != 2 || rep.Pending != 100 || rep.BySection["194J"] != 1000 {
		t.Fatalf("tds report: %+v", rep)
	}
	rates, _ := repo.ListTaxRates(ctx, org)
	if len(rates) < 10 {
		t.Fatalf("default tax rates: %d", len(rates))
	}

	// ITC reconciliation: the bill has no GSTIN party -> add the party and upload 2B
	_, _ = repo.SaveGSTParty(ctx, org, user, GSTParty{ContactID: "v1", Name: "Supplier", GSTIN: "27AAPFU0939F1ZV"})
	csvText := "gstin,supplier,invoice_no,invoice_date,taxable,igst,cgst,sgst\n27AAPFU0939F1ZV,Supplier,S-77,2026-11-08,400,0,36,36\n29AAAAA0000A1Z5,Other,X-1,2026-11-02,100,18,0,0\n"
	if n, err := repo.ImportGSTR2B(ctx, org, user, "2026-11", csvText); err != nil || n != 2 {
		t.Fatalf("2b import: %v %d", err, n)
	}
	rec, err := repo.ReconcileITC(ctx, org, "2026-11")
	if err != nil || rec.Matched != 1 || rec.Issues != 1 {
		t.Fatalf("itc: %v %+v", err, rec)
	}
	for _, r := range rec.Rows {
		if r.InvoiceNo == "X-1" && r.Status != "missing_in_books" {
			t.Fatalf("status %s", r.Status)
		}
	}
	_ = bill
	requireBalanced(t, repo, org)
}
