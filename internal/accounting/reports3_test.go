package accounting

import "testing"

func rptFixture() ([]Account, []Posting) {
	accts := []Account{
		{ID: "cash", Code: "1000", Name: "Cash", Type: "asset", Subtype: "cash"},
		{ID: "ar", Code: "1100", Name: "Receivable", Type: "asset", Subtype: "receivable"},
		{ID: "ap", Code: "2000", Name: "Payable", Type: "liability", Subtype: "payable"},
		{ID: "sales", Code: "4000", Name: "Sales", Type: "income"},
		{ID: "rent", Code: "5000", Name: "Rent", Type: "expense"},
	}
	ps := []Posting{
		{Date: "2026-03-20", JournalID: "j0", Number: 1, Memo: "old sale", AccountID: "ar", Debit: 10000, ContactID: "c1", ContactName: "Asha"},
		{Date: "2026-03-20", JournalID: "j0", Number: 1, Memo: "old sale", AccountID: "sales", Credit: 10000, ContactID: "c1", ContactName: "Asha"},
		{Date: "2026-04-05", JournalID: "j1", Number: 2, Memo: "sale", AccountID: "ar", Debit: 59999, ContactID: "c2", ContactName: "Bhim"},
		{Date: "2026-04-05", JournalID: "j1", Number: 2, Memo: "sale", AccountID: "sales", Credit: 59999, ContactID: "c2", ContactName: "Bhim"},
		{Date: "2026-05-10", JournalID: "j2", Number: 3, Memo: "paid", AccountID: "cash", Debit: 20000, ContactID: "c2", ContactName: "Bhim"},
		{Date: "2026-05-10", JournalID: "j2", Number: 3, Memo: "paid", AccountID: "ar", Credit: 20000, ContactID: "c2", ContactName: "Bhim"},
		{Date: "2026-05-12", JournalID: "j3", Number: 4, Memo: "rent bill", AccountID: "rent", Debit: 12345, ContactID: "v1", ContactName: "Landlord"},
		{Date: "2026-05-12", JournalID: "j3", Number: 4, Memo: "rent bill", AccountID: "ap", Credit: 12345, ContactID: "v1", ContactName: "Landlord"},
		{Date: "2026-06-02", JournalID: "j4", Number: 5, Memo: "sale", AccountID: "ar", Debit: 5001, ContactID: "c1", ContactName: "Asha"},
		{Date: "2026-06-02", JournalID: "j4", Number: 5, Memo: "sale", AccountID: "sales", Credit: 5001, ContactID: "c1", ContactName: "Asha"},
	}
	return accts, ps
}

func TestGeneralLedgerOpeningRunningClosing(t *testing.T) {
	accts, ps := rptFixture()
	r := BuildGeneralLedger(accts, ps, "2026-04-01", "2026-06-30")
	if r.Totals["debit"] != r.Totals["credit"] {
		t.Fatalf("ledger out of balance: %v", r.Totals)
	}
	// AR: opening 100.00, +599.99, -200.00, +50.01 => 550.00
	var arClose any
	for _, row := range r.Tbl.Rows {
		if row[0] == "Total 1100 Receivable" {
			arClose = row[6]
		}
	}
	if arClose != 550.00 {
		t.Fatalf("AR closing = %v", arClose)
	}
	if r.Labels["1100 Receivable"] != "ar" {
		t.Fatal("drill-down label missing")
	}
}

func TestJournalAndTransactionList(t *testing.T) {
	accts, ps := rptFixture()
	j := BuildJournalReport(accts, ps, "2026-01-01", "2026-12-31")
	if j.Totals["debit"] != j.Totals["credit"] || j.Totals["debit"] != 1073.45 {
		t.Fatalf("journal totals %v", j.Totals)
	}
	tl := BuildTransactionList(ps, "2026-04-01", "2026-05-31")
	if tl.Totals["count"] != 3 || tl.Totals["amount"] != 599.99+200+123.45 {
		t.Fatalf("transaction list %v", tl.Totals)
	}
}

func TestContactBalances(t *testing.T) {
	accts, ps := rptFixture()
	ar := BuildContactBalances(accts, ps, "2026-12-31", "receivable")
	if ar.Totals["total"] != 550.00 {
		t.Fatalf("AR total %v", ar.Totals)
	}
	ap := BuildContactBalances(accts, ps, "2026-12-31", "payable")
	if ap.Totals["total"] != 123.45 {
		t.Fatalf("AP total %v", ap.Totals)
	}
	early := BuildContactBalances(accts, ps, "2026-03-31", "receivable")
	if early.Totals["total"] != 100 {
		t.Fatalf("as-of filter %v", early.Totals)
	}
}

func TestByContactSortedAndTotals(t *testing.T) {
	accts, ps := rptFixture()
	s := BuildByContact(accts, ps, "2026-04-01", "2026-12-31", "sales")
	if s.Totals["total"] != 650.00 || s.Tbl.Rows[0][0] != "Bhim" {
		t.Fatalf("sales by customer %v %v", s.Totals, s.Tbl.Rows)
	}
	e := BuildByContact(accts, ps, "2026-04-01", "2026-12-31", "expenses")
	if e.Totals["total"] != 123.45 {
		t.Fatalf("expenses by vendor %v", e.Totals)
	}
}

func TestPnLColumnsTotalMatchesPnL(t *testing.T) {
	accts, ps := rptFixture()
	for _, by := range []string{"month", "quarter", "year"} {
		c, err := BuildPnLColumns(accts, ps, "2026-04-01", "2026-06-30", by, 4)
		if err != nil {
			t.Fatal(err)
		}
		want := BuildPnL(accts, ps, "2026-04-01", "2026-06-30").NetProfit
		if c.Totals["net_profit"] != want {
			t.Fatalf("%s: columns net %v != pnl %v", by, c.Totals["net_profit"], want)
		}
	}
	m, _ := BuildPnLColumns(accts, ps, "2026-04-01", "2026-06-30", "month", 4)
	if m.Totals["columns"] != 3 {
		t.Fatalf("columns %v", m.Totals)
	}
	if _, err := BuildPnLColumns(accts, ps, "2026-04-01", "2026-06-30", "week", 4); err == nil {
		t.Fatal("expected error for bad columns")
	}
}

func TestPeriodStartsQuarterAlignsToFY(t *testing.T) {
	got, err := periodStarts("2026-05-15", "2026-11-10", "quarter", 4)
	if err != nil {
		t.Fatal(err)
	}
	// FY quarters: Apr-Jun, Jul-Sep, Oct-Dec
	if len(got) != 3 || got[0] != [2]string{"2026-05-15", "2026-06-30"} || got[2] != [2]string{"2026-10-01", "2026-11-10"} {
		t.Fatalf("%v", got)
	}
}
