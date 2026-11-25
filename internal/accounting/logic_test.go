package accounting

import (
	"testing"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
)

func testKeys() map[string]string {
	m := map[string]string{}
	for _, a := range DefaultChart {
		if a.key != "" {
			m[a.key] = "id-" + a.key
		}
	}
	return m
}

func sums(ls []Line) (d, c int64) {
	for _, l := range ls {
		d += paise(l.Debit)
		c += paise(l.Credit)
	}
	return
}

func mustBalance(t *testing.T, name string, in JournalInput) {
	t.Helper()
	d, c := sums(in.Lines)
	if d != c || d == 0 {
		t.Fatalf("%s: debits %d != credits %d (%+v)", name, d, c, in.Lines)
	}
	for _, l := range in.Lines {
		if l.AccountID == "" {
			t.Fatalf("%s: line without account: %+v", name, l)
		}
	}
	if _, err := checkBalanced(in.Lines); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func TestPosSalePostingBalances(t *testing.T) {
	keys := testKeys()
	sale := &adrientv1.PosSale{Id: "s1", Number: 7, Total: 1180, TaxTotal: 180, CompletedAt: "2026-11-02T10:00:00Z",
		Payments: []*adrientv1.PosPayment{{Method: "cash", Amount: 500}, {Method: "card", Amount: 680}}}
	in := BuildPosSale(keys, sale, true)
	mustBalance(t, "pos sale", in)
	if in.Source != "pos_sale" || in.SourceID != "s1" {
		t.Fatalf("source key: %+v", in)
	}
	// 90 CGST + 90 SGST, 1000 sales.
	var cg, sg, sales float64
	for _, l := range in.Lines {
		switch l.AccountID {
		case keys["gst_out_cgst"]:
			cg = l.Credit
		case keys["gst_out_sgst"]:
			sg = l.Credit
		case keys["sales"]:
			sales = l.Credit
		}
	}
	if cg != 90 || sg != 90 || sales != 1000 {
		t.Fatalf("cgst %v sgst %v sales %v", cg, sg, sales)
	}
	// Interstate uses IGST only; odd paise split stays balanced.
	odd := &adrientv1.PosSale{Id: "s2", Number: 8, Total: 105.01, TaxTotal: 5.01, Payments: []*adrientv1.PosPayment{{Method: "other", Amount: 105.01}}}
	mustBalance(t, "odd", BuildPosSale(keys, odd, true))
	inter := BuildPosSale(keys, sale, false)
	mustBalance(t, "igst", inter)
	for _, l := range inter.Lines {
		if l.AccountID == keys["gst_out_cgst"] {
			t.Fatal("interstate sale must not use CGST")
		}
	}
	// Unrecorded tender (change/rounding) settles to cash and stays balanced.
	short := &adrientv1.PosSale{Id: "s3", Number: 9, Total: 100, TaxTotal: 0, Payments: []*adrientv1.PosPayment{{Method: "card", Amount: 99.5}}}
	mustBalance(t, "short", BuildPosSale(keys, short, true))
	// Same input twice gives the same hash (idempotent update check).
	if BuildPosSale(keys, sale, true).SourceHash != in.SourceHash {
		t.Fatal("hash must be stable")
	}
}

func TestPosRefundPostingBalances(t *testing.T) {
	keys := testKeys()
	sale := &adrientv1.PosSale{Id: "s1", Number: 7, Total: 1180, TaxTotal: 180}
	for _, m := range []string{"cash", "card", "store_credit", "other"} {
		in := BuildPosRefund(keys, sale, &adrientv1.PosRefund{Id: "r1", Number: "R7-1", Total: 590, Method: m, CreatedAt: "2026-11-03T10:00:00Z"}, true)
		mustBalance(t, "refund "+m, in)
		if in.Source != "pos_refund" || in.SourceID != "r1" {
			t.Fatalf("source key %+v", in)
		}
	}
}

func TestInvoicePostings(t *testing.T) {
	keys := testKeys()
	d := invoicing.Document{ID: "d1", Number: "INV-0001", Total: 236, TaxTotal: 36, IssueDate: "2026-11-01", DueDate: "2026-11-15", ClientName: "Acme"}
	in := BuildInvoice(keys, d, true)
	mustBalance(t, "invoice", in)
	if in.DueDate != "2026-11-15" || in.Source != "invoice" {
		t.Fatalf("%+v", in)
	}
	for _, k := range []string{"payment", "refund", "writeoff"} {
		p := invoicing.Payment{ID: "p-" + k, Kind: k, Amount: 100, Method: "upi", PaidAt: "2026-11-05", DocNumber: "INV-0001"}
		mustBalance(t, "inv "+k, BuildInvoicePayment(keys, p, d))
	}
	// A changed total changes the hash so the journal is rewritten, not duplicated.
	d2 := d
	d2.Total, d2.TaxTotal = 354, 54
	if BuildInvoice(keys, d2, true).SourceHash == in.SourceHash {
		t.Fatal("hash must change with the amount")
	}
}

func TestExpenseJournal(t *testing.T) {
	keys := testKeys()
	e := Expense{ID: "e1", Date: "2026-11-04", CategoryID: "11111111-1111-1111-1111-111111111111", Amount: 1000, GSTRate: 18, Paid: true, PaidFromID: "bank-id", VendorName: "Vendor"}
	if err := normalizeExpense(&e); err != nil {
		t.Fatal(err)
	}
	if e.GSTAmount != 180 || e.Total != 1180 {
		t.Fatalf("gst %v total %v", e.GSTAmount, e.Total)
	}
	mustBalance(t, "paid", ExpenseJournal(keys, e, true))
	bill := e
	bill.Paid, bill.PaidFromID = false, ""
	in := ExpenseJournal(keys, bill, false)
	mustBalance(t, "bill", in)
	if in.Kind != "bill" {
		t.Fatalf("kind %s", in.Kind)
	}
	oop := e
	oop.OutOfPocket = true
	mustBalance(t, "oop", ExpenseJournal(keys, oop, true))
	bad := Expense{Date: "2026-11-04", CategoryID: "11111111-1111-1111-1111-111111111111", Amount: 0}
	if normalizeExpense(&bad) == nil {
		t.Fatal("zero amount must be rejected")
	}
}

func TestCheckBalancedRejects(t *testing.T) {
	cases := map[string][]Line{
		"one line":   {{AccountID: "a", Debit: 5}},
		"unbalanced": {{AccountID: "a", Debit: 5}, {AccountID: "b", Credit: 4}},
		"both sides": {{AccountID: "a", Debit: 5, Credit: 5}, {AccountID: "b", Credit: 5}},
		"zero":       {{AccountID: "a"}, {AccountID: "b"}},
		"negative":   {{AccountID: "a", Debit: -5}, {AccountID: "b", Credit: -5}},
		"no account": {{Debit: 5}, {AccountID: "b", Credit: 5}},
	}
	for name, ls := range cases {
		if _, err := checkBalanced(ls); err == nil {
			t.Errorf("%s should be rejected", name)
		}
	}
	if _, err := checkBalanced([]Line{{AccountID: "a", Debit: 0.1}, {AccountID: "b", Debit: 0.2}, {AccountID: "c", Credit: 0.3}}); err != nil {
		t.Errorf("0.1+0.2=0.3 must balance in paise: %v", err)
	}
}

func TestParseStatementCSV(t *testing.T) {
	csv := "Account Statement\n\nDate,Narration,Withdrawal Amt.,Deposit Amt.,Balance\n" +
		"01/04/2026,UPI-SWIGGY-123,\"1,250.50\",,10000\n" +
		"02-04-2026,NEFT SALARY ACME,,\"50,000.00\",60000\n" +
		"02-04-2026,NEFT SALARY ACME,,\"50,000.00\",110000\n" +
		"junk,row,,,\n"
	ls, err := ParseStatementCSV(csv)
	if err != nil || len(ls) != 3 {
		t.Fatalf("%v %+v", err, ls)
	}
	if ls[0].Date != "2026-04-01" || ls[0].Amount != -125050 || ls[1].Amount != 5000000 {
		t.Fatalf("%+v", ls)
	}
	single, err := ParseStatementCSV("Date,Description,Amount\n2026-05-01,Rent,-15000\n2026-05-02,Sale,2500.75\n")
	if err != nil || single[0].Amount != -1500000 || single[1].Amount != 250075 {
		t.Fatalf("%v %+v", err, single)
	}
	if _, err := ParseStatementCSV("a,b\n1,2\n"); err == nil {
		t.Fatal("missing columns must fail")
	}
	for in, want := range map[string]int64{"1,23,456.78": 12345678, "(500)": -50000, "500 Dr": -50000, "500 Cr": 50000, "-12.5": -1250, "₹ 99": 9900} {
		if got, ok := ParseAmount(in); !ok || got != want {
			t.Errorf("ParseAmount(%q)=%d,%v want %d", in, got, ok, want)
		}
	}
	if _, ok := ParseAmount("abc"); ok {
		t.Fatal("garbage must not parse")
	}
}

func TestMatchRule(t *testing.T) {
	rules := []Rule{
		{ID: "a", Contains: "swiggy", Direction: "out", CategoryID: "food", Priority: 50},
		{ID: "b", Contains: "upi", Direction: "any", CategoryID: "misc", Priority: 100},
		{ID: "c", Contains: "salary", Direction: "in", AccountID: "bank2", CategoryID: "sal", Priority: 10},
	}
	if r := MatchRule(rules, "bank1", "UPI-SWIGGY-1", -100); r == nil || r.ID != "a" {
		t.Fatalf("priority: %+v", r)
	}
	if r := MatchRule(rules, "bank1", "UPI-SWIGGY-1", 100); r == nil || r.ID != "b" {
		t.Fatalf("direction: %+v", r)
	}
	if r := MatchRule(rules, "bank1", "NEFT SALARY", 100); r != nil {
		t.Fatalf("account-scoped rule leaked: %+v", r)
	}
	if r := MatchRule(rules, "bank2", "NEFT SALARY", 100); r == nil || r.ID != "c" {
		t.Fatalf("scoped: %+v", r)
	}
}

func TestFYStart(t *testing.T) {
	for in, want := range map[string]string{"2026-11-16": "2026-04-01", "2026-03-31": "2025-04-01", "2026-04-01": "2026-04-01"} {
		if got := FYStart(in, 4); got != want {
			t.Errorf("FYStart(%s)=%s want %s", in, got, want)
		}
	}
	if FYEnd("2026-04-01") != "2027-03-31" {
		t.Fatal("FYEnd")
	}
}

// ---- report engine on a synthetic book ----

func fixture() ([]Account, []Posting) {
	accts := []Account{
		{ID: "cash", Code: "1000", Name: "Cash", Type: "asset", Subtype: "cash"},
		{ID: "bank", Code: "1010", Name: "Bank", Type: "asset", Subtype: "bank"},
		{ID: "ar", Code: "1100", Name: "AR", Type: "asset", Subtype: "receivable"},
		{ID: "fa", Code: "1500", Name: "Equipment", Type: "asset", Subtype: "fixed_asset"},
		{ID: "ap", Code: "2000", Name: "AP", Type: "liability", Subtype: "payable"},
		{ID: "gso", Code: "2100", Name: "GST out", Type: "liability", Subtype: "gst_output"},
		{ID: "gsi", Code: "1300", Name: "GST in", Type: "asset", Subtype: "gst_input"},
		{ID: "cap", Code: "3000", Name: "Capital", Type: "equity", Subtype: "capital"},
		{ID: "sales", Code: "4000", Name: "Sales", Type: "income", Subtype: "sales"},
		{ID: "rent", Code: "6000", Name: "Rent", Type: "expense", Subtype: "opex"},
	}
	var ps []Posting
	j := 0
	add := func(date string, due string, contact string, lines ...[3]any) {
		j++
		id := string(rune('A' + j))
		for _, l := range lines {
			p := Posting{Date: date, DueDate: due, JournalID: id, Number: int64(j), AccountID: l[0].(string), ContactID: contact, ContactName: contact}
			if v := l[1].(int); v > 0 {
				p.Debit = int64(v)
			}
			if v := l[2].(int); v > 0 {
				p.Credit = int64(v)
			}
			ps = append(ps, p)
		}
	}
	// FY 2025-26: capital 1,00,000 into bank; buy equipment 20,000 from bank.
	add("2025-04-02", "", "", [3]any{"bank", 10000000, 0}, [3]any{"cap", 0, 10000000})
	add("2025-05-01", "", "", [3]any{"fa", 2000000, 0}, [3]any{"bank", 0, 2000000})
	add("2025-06-01", "2025-06-15", "acme", [3]any{"ar", 118000, 0}, [3]any{"sales", 0, 100000}, [3]any{"gso", 0, 18000})
	// FY 2026-27: invoice, part payment, rent, cash sale, bill.
	add("2026-04-10", "2026-04-24", "beta", [3]any{"ar", 236000, 0}, [3]any{"sales", 0, 200000}, [3]any{"gso", 0, 36000})
	add("2026-05-01", "", "beta", [3]any{"bank", 100000, 0}, [3]any{"ar", 0, 100000})
	add("2026-05-05", "", "", [3]any{"rent", 500000, 0}, [3]any{"gsi", 90000, 0}, [3]any{"bank", 0, 590000})
	add("2026-06-01", "", "", [3]any{"cash", 59000, 0}, [3]any{"sales", 0, 50000}, [3]any{"gso", 0, 9000})
	add("2026-06-10", "2026-06-20", "landlord", [3]any{"rent", 100000, 0}, [3]any{"ap", 0, 100000})
	return accts, ps
}

func TestReportsBalance(t *testing.T) {
	accts, ps := fixture()
	for _, asOf := range []string{"2025-03-31", "2025-06-30", "2026-04-30", "2026-05-31", "2026-11-16"} {
		tb := BuildTrialBalance(accts, ps, asOf)
		if !tb.Balanced || tb.TotalDebit != tb.TotalCredit {
			t.Fatalf("trial balance %s: %v vs %v", asOf, tb.TotalDebit, tb.TotalCredit)
		}
		bs := BuildBalanceSheet(accts, ps, asOf, 4)
		if !bs.Balanced || bs.TotalAssets != bs.TotalLiabilities+bs.TotalEquity {
			t.Fatalf("balance sheet %s: assets %v vs L+E %v", asOf, bs.TotalAssets, bs.TotalLiabilities+bs.TotalEquity)
		}
	}
	bs := BuildBalanceSheet(accts, ps, "2026-11-16", 4)
	// FY25-26 profit 1000.00 is retained; FY26-27 profit = 2000+500-5000-1000+... computed below.
	if bs.PriorEarnings != 1000 {
		t.Fatalf("retained earnings %v", bs.PriorEarnings)
	}
	pl := BuildPnL(accts, ps, "2026-04-01", "2027-03-31")
	if pl.TotalIncome != 2500 || pl.TotalExpenses != 6000 || pl.NetProfit != -3500 {
		t.Fatalf("pnl %+v", pl)
	}
	if bs.CurrentYearPL != pl.NetProfit {
		t.Fatalf("balance sheet current year %v vs P&L %v", bs.CurrentYearPL, pl.NetProfit)
	}
}

func TestCashFlowTies(t *testing.T) {
	accts, ps := fixture()
	cf := BuildCashFlow(accts, ps, "2025-04-01", "2026-11-16")
	if !cf.Ties {
		t.Fatalf("cash flow does not tie: %+v", cf)
	}
	if cf.Opening != 0 {
		t.Fatalf("unexpected: %+v", cf)
	}
	// Opening/closing equal the cash+bank balances at each end.
	cf2 := BuildCashFlow(accts, ps, "2026-04-01", "2027-03-31")
	if !cf2.Ties || cf2.Opening != 80000 {
		t.Fatalf("FY26-27 cash flow: %+v", cf2)
	}
	var fin, inv float64
	for _, g := range cf.Groups {
		switch g.Name {
		case "Financing activities":
			fin = g.Total
		case "Investing activities":
			inv = g.Total
		}
	}
	if fin != 100000 || inv != -20000 {
		t.Fatalf("financing %v investing %v", fin, inv)
	}
}

func TestAgingFIFO(t *testing.T) {
	accts, ps := fixture()
	// acme owes 1,180 due 2025-06-15 (>90 days at 2026-11-16); beta owes 1,360 net, due 2026-04-24.
	ag := BuildAging(accts, ps, "2026-11-16", "receivable")
	if len(ag.Rows) != 2 {
		t.Fatalf("rows %+v", ag.Rows)
	}
	byName := map[string]AgingRow{}
	for _, r := range ag.Rows {
		byName[r.ContactName] = r
	}
	if byName["acme"].D90Plus != 1180 || byName["beta"].D90Plus != 1360 || ag.Total.Total != 2540 {
		t.Fatalf("%+v", ag)
	}
	// Before the due date the amount is current.
	early := BuildAging(accts, ps, "2026-04-20", "receivable")
	for _, r := range early.Rows {
		if r.ContactName == "beta" && r.Current != 2360 {
			t.Fatalf("beta current %+v", r)
		}
	}
	ap := BuildAging(accts, ps, "2026-07-01", "payable")
	if len(ap.Rows) != 1 || ap.Rows[0].D1_30 != 1000 {
		t.Fatalf("payables %+v", ap)
	}
}

func TestGSTReport(t *testing.T) {
	accts, ps := fixture()
	g := BuildGST(accts, ps, "2026-04-01", "2026-06-30")
	if g.OutputTotal != 450 || g.InputTotal != 900 || g.NetPayable != -450 {
		t.Fatalf("%+v", g)
	}
}

func TestRegisterRunningBalance(t *testing.T) {
	accts, ps := fixture()
	rows, opening := BuildRegister(accts[1], ps, "2026-04-01", "")
	if opening != 80000 || len(rows) != 2 || rows[1].Balance != 80000+1000-5900 {
		t.Fatalf("opening %v rows %+v", opening, rows)
	}
}
