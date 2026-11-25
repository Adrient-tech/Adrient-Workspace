package accounting

import (
	"context"
	"sort"
	"time"
)

// Posting is one ledger line joined with its journal, used by the report engine.
type Posting struct {
	Date        string // YYYY-MM-DD
	DueDate     string
	JournalID   string
	Number      int64
	Memo        string
	AccountID   string
	Debit       int64 // paise
	Credit      int64
	ContactID   string
	ContactName string
	Class       string
}

// LoadPostings reads every non-voided posting up to `to` (inclusive, "" = all).
func (r *Repository) LoadPostings(ctx context.Context, orgID, to string) ([]Posting, error) {
	rows, err := r.pool.Query(ctx, `SELECT to_char(j.entry_date,'YYYY-MM-DD'), coalesce(to_char(j.due_date,'YYYY-MM-DD'),''), j.id::text, j.number,
		j.memo, l.account_id::text, round(l.debit*100)::bigint, round(l.credit*100)::bigint, j.contact_id, j.contact_name, l.class
		FROM adrient.acct_lines l JOIN adrient.acct_journals j ON j.id=l.journal_id
		WHERE l.org_id=$1 AND NOT j.voided AND ($2='' OR j.entry_date <= $2::date)
		ORDER BY j.entry_date, j.number, l.position`, orgID, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Posting{}
	for rows.Next() {
		var p Posting
		if err := rows.Scan(&p.Date, &p.DueDate, &p.JournalID, &p.Number, &p.Memo, &p.AccountID, &p.Debit, &p.Credit, &p.ContactID, &p.ContactName, &p.Class); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func accountMap(accts []Account) map[string]Account {
	m := make(map[string]Account, len(accts))
	for _, a := range accts {
		m[a.ID] = a
	}
	return m
}

// ReportRow is one account line of a report; amounts in rupees.
type ReportRow struct {
	AccountID string  `json:"account_id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Type      string  `json:"type,omitempty"`
	Debit     float64 `json:"debit"`
	Credit    float64 `json:"credit"`
	Amount    float64 `json:"amount"`
}

// ---- trial balance ----

type TrialBalance struct {
	AsOf        string      `json:"as_of"`
	Rows        []ReportRow `json:"rows"`
	TotalDebit  float64     `json:"total_debit"`
	TotalCredit float64     `json:"total_credit"`
	Balanced    bool        `json:"balanced"`
}

// BuildTrialBalance lists every account's closing balance at asOf.
func BuildTrialBalance(accts []Account, ps []Posting, asOf string) TrialBalance {
	net := map[string]int64{}
	for _, p := range ps {
		if asOf == "" || p.Date <= asOf {
			net[p.AccountID] += p.Debit - p.Credit
		}
	}
	tb := TrialBalance{AsOf: asOf, Rows: []ReportRow{}}
	var td, tc int64
	for _, a := range accts {
		n := net[a.ID]
		if n == 0 {
			continue
		}
		row := ReportRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Type: a.Type}
		if n > 0 {
			row.Debit = rupees(n)
			td += n
		} else {
			row.Credit = rupees(-n)
			tc += -n
		}
		tb.Rows = append(tb.Rows, row)
	}
	tb.TotalDebit, tb.TotalCredit, tb.Balanced = rupees(td), rupees(tc), td == tc
	return tb
}

// ---- profit and loss ----

type PnL struct {
	From          string      `json:"from"`
	To            string      `json:"to"`
	Income        []ReportRow `json:"income"`
	Expenses      []ReportRow `json:"expenses"`
	TotalIncome   float64     `json:"total_income"`
	TotalExpenses float64     `json:"total_expenses"`
	NetProfit     float64     `json:"net_profit"`
}

func BuildPnL(accts []Account, ps []Posting, from, to string) PnL {
	am := accountMap(accts)
	net := map[string]int64{}
	for _, p := range ps {
		if (from == "" || p.Date >= from) && (to == "" || p.Date <= to) {
			net[p.AccountID] += p.Debit - p.Credit
		}
	}
	out := PnL{From: from, To: to, Income: []ReportRow{}, Expenses: []ReportRow{}}
	var ti, te int64
	for _, a := range accts {
		n := net[a.ID]
		if n == 0 || am[a.ID].ID == "" {
			continue
		}
		switch a.Type {
		case "income":
			out.Income = append(out.Income, ReportRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Amount: rupees(-n)})
			ti += -n
		case "expense":
			out.Expenses = append(out.Expenses, ReportRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Amount: rupees(n)})
			te += n
		}
	}
	out.TotalIncome, out.TotalExpenses, out.NetProfit = rupees(ti), rupees(te), rupees(ti-te)
	return out
}

// ---- balance sheet ----

type BalanceSheet struct {
	AsOf             string      `json:"as_of"`
	Assets           []ReportRow `json:"assets"`
	Liabilities      []ReportRow `json:"liabilities"`
	Equity           []ReportRow `json:"equity"`
	CurrentYearPL    float64     `json:"current_year_earnings"`
	PriorEarnings    float64     `json:"retained_earnings"`
	TotalAssets      float64     `json:"total_assets"`
	TotalLiabilities float64     `json:"total_liabilities"`
	TotalEquity      float64     `json:"total_equity"`
	Balanced         bool        `json:"balanced"`
}

// FYStart returns the first day of the financial year containing asOf.
func FYStart(asOf string, startMonth int) string {
	t, err := time.Parse("2006-01-02", asOf)
	if err != nil {
		return ""
	}
	if startMonth < 1 || startMonth > 12 {
		startMonth = 4
	}
	y := t.Year()
	if int(t.Month()) < startMonth {
		y--
	}
	return time.Date(y, time.Month(startMonth), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// FYEnd returns the last day of the FY that starts on fyStart.
func FYEnd(fyStart string) string {
	t, err := time.Parse("2006-01-02", fyStart)
	if err != nil {
		return ""
	}
	return t.AddDate(1, 0, -1).Format("2006-01-02")
}

func BuildBalanceSheet(accts []Account, ps []Posting, asOf string, fyStartMonth int) BalanceSheet {
	net := map[string]int64{}
	for _, p := range ps {
		if asOf == "" || p.Date <= asOf {
			net[p.AccountID] += p.Debit - p.Credit
		}
	}
	bs := BalanceSheet{AsOf: asOf, Assets: []ReportRow{}, Liabilities: []ReportRow{}, Equity: []ReportRow{}}
	var ta, tl, te, earnAll int64
	for _, a := range accts {
		n := net[a.ID]
		if n == 0 {
			continue
		}
		switch a.Type {
		case "asset":
			bs.Assets = append(bs.Assets, ReportRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Amount: rupees(n)})
			ta += n
		case "liability":
			bs.Liabilities = append(bs.Liabilities, ReportRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Amount: rupees(-n)})
			tl += -n
		case "equity":
			bs.Equity = append(bs.Equity, ReportRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Amount: rupees(-n)})
			te += -n
		case "income", "expense":
			earnAll += -n // credit-positive
		}
	}
	// Current-FY earnings: income less expense since the FY start.
	var cur int64
	if fy := FYStart(asOf, fyStartMonth); fy != "" {
		for _, p := range ps {
			if p.Date >= fy && p.Date <= asOf {
				if a := findAcct(accts, p.AccountID); a.Type == "income" || a.Type == "expense" {
					cur += p.Credit - p.Debit
				}
			}
		}
	} else {
		cur = earnAll
	}
	prior := earnAll - cur
	bs.CurrentYearPL, bs.PriorEarnings = rupees(cur), rupees(prior)
	te += earnAll
	bs.TotalAssets, bs.TotalLiabilities, bs.TotalEquity = rupees(ta), rupees(tl), rupees(te)
	bs.Balanced = ta == tl+te
	return bs
}

func findAcct(accts []Account, id string) Account {
	for _, a := range accts {
		if a.ID == id {
			return a
		}
	}
	return Account{}
}

// ---- cash flow ----

type CashFlowGroup struct {
	Name  string      `json:"name"`
	Rows  []ReportRow `json:"rows"`
	Total float64     `json:"total"`
}

type CashFlow struct {
	From      string          `json:"from"`
	To        string          `json:"to"`
	Opening   float64         `json:"opening"`
	Closing   float64         `json:"closing"`
	Groups    []CashFlowGroup `json:"groups"`
	NetChange float64         `json:"net_change"`
	Ties      bool            `json:"ties"`
}

func isCash(a Account) bool { return a.Type == "asset" && (a.Subtype == "cash" || a.Subtype == "bank") }

func cashCategory(a Account) string {
	switch {
	case a.Type == "equity" || a.Subtype == "loan":
		return "Financing activities"
	case a.Subtype == "fixed_asset":
		return "Investing activities"
	}
	return "Operating activities"
}

// BuildCashFlow is a direct-method statement: every non-cash line of a journal
// that touches cash is classified by its account, so the groups always sum to the
// change in cash and bank balances.
func BuildCashFlow(accts []Account, ps []Posting, from, to string) CashFlow {
	am := accountMap(accts)
	cf := CashFlow{From: from, To: to, Groups: []CashFlowGroup{}}
	byJournal := map[string][]Posting{}
	order := []string{}
	var opening, closing int64
	for _, p := range ps {
		if isCash(am[p.AccountID]) {
			if p.Date < from {
				opening += p.Debit - p.Credit
			}
			if to == "" || p.Date <= to {
				closing += p.Debit - p.Credit
			}
		}
		if p.Date >= from && (to == "" || p.Date <= to) {
			if _, ok := byJournal[p.JournalID]; !ok {
				order = append(order, p.JournalID)
			}
			byJournal[p.JournalID] = append(byJournal[p.JournalID], p)
		}
	}
	type key struct{ cat, acct string }
	sums := map[key]int64{}
	var net int64
	for _, jid := range order {
		lines := byJournal[jid]
		hasCash := false
		for _, l := range lines {
			if isCash(am[l.AccountID]) {
				hasCash = true
			}
		}
		if !hasCash {
			continue
		}
		var cashNet, nonCash int64
		for _, l := range lines {
			if isCash(am[l.AccountID]) {
				cashNet += l.Debit - l.Credit
			} else {
				nonCash += l.Credit - l.Debit
			}
		}
		net += cashNet
		if cashNet == 0 && nonCash == 0 {
			continue
		}
		for _, l := range lines {
			a := am[l.AccountID]
			if isCash(a) {
				continue
			}
			sums[key{cashCategory(a), a.ID}] += l.Credit - l.Debit
		}
		// A cash-to-cash transfer has no non-cash side and nets to zero.
	}
	for _, cat := range []string{"Operating activities", "Investing activities", "Financing activities"} {
		g := CashFlowGroup{Name: cat, Rows: []ReportRow{}}
		var tot int64
		for _, a := range accts {
			if v := sums[key{cat, a.ID}]; v != 0 {
				g.Rows = append(g.Rows, ReportRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Amount: rupees(v)})
				tot += v
			}
		}
		g.Total = rupees(tot)
		cf.Groups = append(cf.Groups, g)
	}
	cf.Opening, cf.Closing, cf.NetChange = rupees(opening), rupees(closing), rupees(net)
	var sum int64
	for _, g := range cf.Groups {
		sum += paise(g.Total)
	}
	cf.Ties = sum == net && opening+net == closing
	return cf
}

// ---- aging ----

type AgingRow struct {
	ContactID   string  `json:"contact_id"`
	ContactName string  `json:"contact_name"`
	Current     float64 `json:"current"`
	D1_30       float64 `json:"d1_30"`
	D31_60      float64 `json:"d31_60"`
	D61_90      float64 `json:"d61_90"`
	D90Plus     float64 `json:"d90_plus"`
	Total       float64 `json:"total"`
}

type Aging struct {
	Kind  string     `json:"kind"`
	AsOf  string     `json:"as_of"`
	Rows  []AgingRow `json:"rows"`
	Total AgingRow   `json:"total"`
}

// BuildAging allocates payments to the oldest open items first, per contact,
// then buckets what is left by days past due at asOf. kind is "receivable" or
// "payable".
func BuildAging(accts []Account, ps []Posting, asOf, kind string) Aging {
	am := accountMap(accts)
	type item struct {
		due string
		amt int64
	}
	items := map[string][]item{}
	credits := map[string]int64{}
	names := map[string]string{}
	for _, p := range ps {
		a := am[p.AccountID]
		if a.Subtype != kind || (asOf != "" && p.Date > asOf) {
			continue
		}
		k := p.ContactID
		if k == "" {
			k = "name:" + p.ContactName
		}
		if p.ContactName != "" {
			names[k] = p.ContactName
		}
		due := p.DueDate
		if due == "" {
			due = p.Date
		}
		raise, settle := p.Debit, p.Credit // receivable: debit raises
		if kind == "payable" {
			raise, settle = p.Credit, p.Debit
		}
		if raise > 0 {
			items[k] = append(items[k], item{due, raise})
		}
		credits[k] += settle
	}
	out := Aging{Kind: kind, AsOf: asOf, Rows: []AgingRow{}}
	asT, _ := time.Parse("2006-01-02", asOf)
	var tot struct{ c, a, b, d, e int64 }
	keys := make([]string, 0, len(items)+len(credits))
	seen := map[string]bool{}
	for k := range items {
		keys = append(keys, k)
		seen[k] = true
	}
	for k := range credits {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		its := items[k]
		sort.SliceStable(its, func(i, j int) bool { return its[i].due < its[j].due })
		left := credits[k]
		var c, a, b, d, e int64
		for _, it := range its {
			amt := it.amt
			if left >= amt {
				left -= amt
				continue
			}
			amt -= left
			left = 0
			days := 0
			if t, err := time.Parse("2006-01-02", it.due); err == nil && !asT.IsZero() {
				days = int(asT.Sub(t).Hours() / 24)
			}
			switch {
			case days <= 0:
				c += amt
			case days <= 30:
				a += amt
			case days <= 60:
				b += amt
			case days <= 90:
				d += amt
			default:
				e += amt
			}
		}
		if left > 0 { // unapplied payment/credit
			c -= left
		}
		if c+a+b+d+e == 0 {
			continue
		}
		cid, name := "", names[k]
		if len(k) < 5 || k[:5] != "name:" {
			cid = k
		}
		if name == "" {
			name = "(no contact)"
		}
		out.Rows = append(out.Rows, AgingRow{ContactID: cid, ContactName: name, Current: rupees(c), D1_30: rupees(a), D31_60: rupees(b),
			D61_90: rupees(d), D90Plus: rupees(e), Total: rupees(c + a + b + d + e)})
		tot.c, tot.a, tot.b, tot.d, tot.e = tot.c+c, tot.a+a, tot.b+b, tot.d+d, tot.e+e
	}
	out.Total = AgingRow{ContactName: "Total", Current: rupees(tot.c), D1_30: rupees(tot.a), D31_60: rupees(tot.b), D61_90: rupees(tot.d),
		D90Plus: rupees(tot.e), Total: rupees(tot.c + tot.a + tot.b + tot.d + tot.e)}
	return out
}

// ---- GST ----

type GSTReport struct {
	From         string      `json:"from"`
	To           string      `json:"to"`
	Output       []ReportRow `json:"output"`
	Input        []ReportRow `json:"input"`
	OutputTotal  float64     `json:"output_total"`
	InputTotal   float64     `json:"input_total"`
	NetPayable   float64     `json:"net_payable"` // negative = refundable / carried forward
	TaxableSales float64     `json:"taxable_sales"`
}

func BuildGST(accts []Account, ps []Posting, from, to string) GSTReport {
	net := map[string]int64{}
	var sales int64
	am := accountMap(accts)
	for _, p := range ps {
		if (from != "" && p.Date < from) || (to != "" && p.Date > to) {
			continue
		}
		net[p.AccountID] += p.Debit - p.Credit
		if a := am[p.AccountID]; a.Type == "income" {
			sales += p.Credit - p.Debit
		}
	}
	g := GSTReport{From: from, To: to, Output: []ReportRow{}, Input: []ReportRow{}, TaxableSales: rupees(sales)}
	var out, in int64
	for _, a := range accts {
		n := net[a.ID]
		switch a.Subtype {
		case "gst_output":
			g.Output = append(g.Output, ReportRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Amount: rupees(-n)})
			out += -n
		case "gst_input":
			g.Input = append(g.Input, ReportRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Amount: rupees(n)})
			in += n
		}
	}
	g.OutputTotal, g.InputTotal, g.NetPayable = rupees(out), rupees(in), rupees(out-in)
	return g
}

// ---- register ----

type RegisterRow struct {
	Date      string  `json:"date"`
	JournalID string  `json:"journal_id"`
	Number    int64   `json:"number"`
	Memo      string  `json:"memo"`
	Contact   string  `json:"contact"`
	Debit     float64 `json:"debit"`
	Credit    float64 `json:"credit"`
	Balance   float64 `json:"balance"`
}

func BuildRegister(a Account, ps []Posting, from, to string) (rows []RegisterRow, opening float64) {
	var bal int64
	rows = []RegisterRow{}
	sign := int64(1)
	if !debitNormal(a.Type) {
		sign = -1
	}
	for _, p := range ps {
		if p.AccountID != a.ID {
			continue
		}
		d := (p.Debit - p.Credit) * sign
		if from != "" && p.Date < from {
			bal += d
			continue
		}
		if to != "" && p.Date > to {
			continue
		}
		if len(rows) == 0 {
			opening = rupees(bal)
		}
		bal += d
		rows = append(rows, RegisterRow{Date: p.Date, JournalID: p.JournalID, Number: p.Number, Memo: p.Memo, Contact: p.ContactName,
			Debit: rupees(p.Debit), Credit: rupees(p.Credit), Balance: rupees(bal)})
	}
	if len(rows) == 0 {
		opening = rupees(bal)
	}
	return rows, opening
}
