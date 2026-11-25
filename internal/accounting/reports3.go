// (report catalogue and grouping rules; rewritten for the Adrient ledger, integer paise throughout).

package accounting

import (
	"sort"
	"strings"
	"time"
)

// TableReport is a report that is only ever shown as a table. Labels maps the text of a
// first-column cell to an account id so the web app can drill down.
type TableReport struct {
	Tbl    Table             `json:"-"`
	Labels map[string]string `json:"-"`
	// Totals carries the figures tests and API clients check (rupees).
	Totals map[string]float64 `json:"totals"`
}

func acctLabel(a Account) string { return strings.TrimSpace(a.Code + " " + a.Name) }

func inRange(d, from, to string) bool { return (from == "" || d >= from) && (to == "" || d <= to) }

func contactKey(p Posting) (key, name string) {
	if p.ContactID != "" {
		return p.ContactID, orDefault(p.ContactName, "(no contact)")
	}
	if p.ContactName != "" {
		return "name:" + p.ContactName, p.ContactName
	}
	return "", "(no contact)"
}

// BuildGeneralLedger lists, per account, the opening balance, every posting in the range with
// a running balance in the account's natural direction, and the closing balance.
func BuildGeneralLedger(accts []Account, ps []Posting, from, to string) TableReport {
	t := Table{Title: "General Ledger", Subtitle: from + " to " + to,
		Headers: []string{"Account / date", "No.", "Memo", "Contact", "Debit", "Credit", "Balance"}}
	labels := map[string]string{}
	byAcct := map[string][]Posting{}
	open := map[string]int64{}
	for _, p := range ps {
		switch {
		case from != "" && p.Date < from:
			open[p.AccountID] += p.Debit - p.Credit
		case inRange(p.Date, from, to):
			byAcct[p.AccountID] = append(byAcct[p.AccountID], p)
		}
	}
	var td, tc int64
	for _, a := range accts {
		rows, op := byAcct[a.ID], open[a.ID]
		if len(rows) == 0 && op == 0 {
			continue
		}
		sign := int64(1)
		if !debitNormal(a.Type) {
			sign = -1
		}
		labels[acctLabel(a)] = a.ID
		t.Rows = append(t.Rows, []any{acctLabel(a), "", "Opening balance", "", "", "", rupees(sign * op)})
		run := op
		var d, c int64
		for _, p := range rows {
			run += p.Debit - p.Credit
			d += p.Debit
			c += p.Credit
			t.Rows = append(t.Rows, []any{"  " + p.Date, p.Number, p.Memo, p.ContactName, rupees(p.Debit), rupees(p.Credit), rupees(sign * run)})
		}
		t.Rows = append(t.Rows, []any{"Total " + acctLabel(a), "", "", "", rupees(d), rupees(c), rupees(sign * run)})
		td += d
		tc += c
	}
	t.Rows = append(t.Rows, []any{"Grand total", "", "", "", rupees(td), rupees(tc), ""})
	return TableReport{Tbl: t, Labels: labels, Totals: map[string]float64{"debit": rupees(td), "credit": rupees(tc)}}
}

// BuildJournalReport lists every journal in the range with all its lines.
func BuildJournalReport(accts []Account, ps []Posting, from, to string) TableReport {
	am := accountMap(accts)
	t := Table{Title: "Journal", Subtitle: from + " to " + to, Headers: []string{"Date / account", "No.", "Memo", "Debit", "Credit"}}
	labels := map[string]string{}
	var order []string
	byJ := map[string][]Posting{}
	for _, p := range ps {
		if !inRange(p.Date, from, to) {
			continue
		}
		if _, ok := byJ[p.JournalID]; !ok {
			order = append(order, p.JournalID)
		}
		byJ[p.JournalID] = append(byJ[p.JournalID], p)
	}
	var td, tc int64
	for _, id := range order {
		lines := byJ[id]
		t.Rows = append(t.Rows, []any{lines[0].Date, lines[0].Number, lines[0].Memo, "", ""})
		for _, p := range lines {
			a := am[p.AccountID]
			lbl := "  " + acctLabel(a)
			labels[strings.TrimSpace(lbl)] = a.ID
			t.Rows = append(t.Rows, []any{lbl, "", "", rupees(p.Debit), rupees(p.Credit)})
			td += p.Debit
			tc += p.Credit
		}
	}
	t.Rows = append(t.Rows, []any{"Total", "", "", rupees(td), rupees(tc)})
	return TableReport{Tbl: t, Labels: labels, Totals: map[string]float64{"debit": rupees(td), "credit": rupees(tc)}}
}

// BuildTransactionList is one row per journal: date, number, memo, contact and amount.
func BuildTransactionList(ps []Posting, from, to string) TableReport {
	t := Table{Title: "Transaction List", Subtitle: from + " to " + to, Headers: []string{"Date", "No.", "Memo", "Contact", "Amount"}}
	type row struct {
		p   Posting
		amt int64
	}
	idx := map[string]int{}
	var rows []row
	for _, p := range ps {
		if !inRange(p.Date, from, to) {
			continue
		}
		i, ok := idx[p.JournalID]
		if !ok {
			i = len(rows)
			idx[p.JournalID] = i
			rows = append(rows, row{p: p})
		}
		rows[i].amt += p.Debit
		if rows[i].p.ContactName == "" {
			rows[i].p.ContactName = p.ContactName
		}
	}
	var total int64
	for _, r := range rows {
		t.Rows = append(t.Rows, []any{r.p.Date, r.p.Number, r.p.Memo, r.p.ContactName, rupees(r.amt)})
		total += r.amt
	}
	t.Rows = append(t.Rows, []any{"Total", "", "", "", rupees(total)})
	return TableReport{Tbl: t, Totals: map[string]float64{"amount": rupees(total), "count": float64(len(rows))}}
}

// BuildAccountList is the chart of accounts with each balance at asOf.
func BuildAccountList(accts []Account, ps []Posting, asOf string) TableReport {
	bal := map[string]int64{}
	for _, p := range ps {
		if asOf == "" || p.Date <= asOf {
			bal[p.AccountID] += p.Debit - p.Credit
		}
	}
	t := Table{Title: "Account List", Subtitle: "As of " + asOf, Headers: []string{"Account", "Type", "Detail type", "Balance"}}
	labels := map[string]string{}
	for _, a := range accts {
		b := bal[a.ID]
		if !debitNormal(a.Type) {
			b = -b
		}
		labels[acctLabel(a)] = a.ID
		t.Rows = append(t.Rows, []any{acctLabel(a), a.Type, a.Subtype, rupees(b)})
	}
	return TableReport{Tbl: t, Labels: labels}
}

// BuildContactBalances is what each customer owes (kind "receivable") or each vendor is owed
// (kind "payable") at asOf. Unapplied payments show as negative balances.
func BuildContactBalances(accts []Account, ps []Posting, asOf, kind string) TableReport {
	am := accountMap(accts)
	bal := map[string]int64{}
	names := map[string]string{}
	for _, p := range ps {
		if am[p.AccountID].Subtype != kind || (asOf != "" && p.Date > asOf) {
			continue
		}
		k, n := contactKey(p)
		d := p.Debit - p.Credit
		if kind == "payable" {
			d = -d
		}
		bal[k] += d
		if _, ok := names[k]; !ok || p.ContactName != "" {
			names[k] = n
		}
	}
	title, head := "Customer Balance Summary", "Customer"
	if kind == "payable" {
		title, head = "Vendor Balance Summary", "Vendor"
	}
	t := Table{Title: title, Subtitle: "As of " + asOf, Headers: []string{head, "Balance"}}
	keys := make([]string, 0, len(bal))
	for k, v := range bal {
		if v != 0 {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return strings.ToLower(names[keys[i]]) < strings.ToLower(names[keys[j]]) })
	var total int64
	for _, k := range keys {
		t.Rows = append(t.Rows, []any{names[k], rupees(bal[k])})
		total += bal[k]
	}
	t.Rows = append(t.Rows, []any{"Total", rupees(total)})
	return TableReport{Tbl: t, Totals: map[string]float64{"total": rupees(total)}}
}

// BuildByContact totals income (side "sales") or expense (side "expenses") postings per contact.
func BuildByContact(accts []Account, ps []Posting, from, to, side string) TableReport {
	am := accountMap(accts)
	want, title, head := "income", "Sales by Customer", "Customer"
	if side == "expenses" {
		want, title, head = "expense", "Expenses by Vendor", "Vendor"
	}
	sum := map[string]int64{}
	names := map[string]string{}
	for _, p := range ps {
		if am[p.AccountID].Type != want || !inRange(p.Date, from, to) {
			continue
		}
		k, n := contactKey(p)
		if want == "income" {
			sum[k] += p.Credit - p.Debit
		} else {
			sum[k] += p.Debit - p.Credit
		}
		if _, ok := names[k]; !ok || p.ContactName != "" {
			names[k] = n
		}
	}
	keys := make([]string, 0, len(sum))
	for k, v := range sum {
		if v != 0 {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if sum[keys[i]] != sum[keys[j]] {
			return sum[keys[i]] > sum[keys[j]]
		}
		return names[keys[i]] < names[keys[j]]
	})
	t := Table{Title: title, Subtitle: from + " to " + to, Headers: []string{head, "Amount"}}
	var total int64
	for _, k := range keys {
		t.Rows = append(t.Rows, []any{names[k], rupees(sum[k])})
		total += sum[k]
	}
	t.Rows = append(t.Rows, []any{"Total", rupees(total)})
	return TableReport{Tbl: t, Totals: map[string]float64{"total": rupees(total)}}
}

// periodStarts splits [from,to] into month, quarter or year columns. Quarters and years follow
// the financial year that starts in fyStartMonth.
func periodStarts(from, to, by string, fyStartMonth int) ([][2]string, error) {
	f, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil, invalid("bad from date")
	}
	e, err := time.Parse("2006-01-02", to)
	if err != nil || e.Before(f) {
		return nil, invalid("bad to date")
	}
	if fyStartMonth < 1 || fyStartMonth > 12 {
		fyStartMonth = 4
	}
	months := map[string]int{"month": 1, "quarter": 3, "year": 12}[by]
	if months == 0 {
		return nil, invalid("columns must be month, quarter or year")
	}
	// align the first column to the period containing `from`
	off := (int(f.Month()) - fyStartMonth + 12) % 12
	cur := time.Date(f.Year(), f.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(off % months), 0)
	var out [][2]string
	for !cur.After(e) {
		next := cur.AddDate(0, months, 0)
		s, en := cur.Format("2006-01-02"), next.AddDate(0, 0, -1).Format("2006-01-02")
		if s < from {
			s = from
		}
		if en > to {
			en = to
		}
		out = append(out, [2]string{s, en})
		cur = next
		if len(out) > 60 {
			return nil, invalid("too many columns, shorten the range")
		}
	}
	return out, nil
}

// BuildPnLColumns is the profit and loss with one column per month, quarter or year plus a total.
func BuildPnLColumns(accts []Account, ps []Posting, from, to, by string, fyStartMonth int) (TableReport, error) {
	cols, err := periodStarts(from, to, by, fyStartMonth)
	if err != nil {
		return TableReport{}, err
	}
	heads := []string{"Account"}
	for _, c := range cols {
		if by == "month" {
			heads = append(heads, c[0][:7])
		} else {
			heads = append(heads, c[0]+" to "+c[1])
		}
	}
	heads = append(heads, "Total")
	t := Table{Title: "Profit and Loss by " + by, Subtitle: from + " to " + to, Headers: heads}
	labels := map[string]string{}
	net := map[string][]int64{}
	for _, p := range ps {
		if !inRange(p.Date, from, to) {
			continue
		}
		for i, c := range cols {
			if p.Date >= c[0] && p.Date <= c[1] {
				if net[p.AccountID] == nil {
					net[p.AccountID] = make([]int64, len(cols))
				}
				net[p.AccountID][i] += p.Debit - p.Credit
			}
		}
	}
	totI, totE := make([]int64, len(cols)), make([]int64, len(cols))
	section := func(name, typ string, tot []int64) {
		t.Rows = append(t.Rows, append([]any{name}, blanks(len(cols)+1)...))
		for _, a := range accts {
			if a.Type != typ || net[a.ID] == nil {
				continue
			}
			var sum int64
			row := []any{"  " + acctLabel(a)}
			zero := true
			for i, v := range net[a.ID] {
				if typ == "income" {
					v = -v
				}
				sum += v
				tot[i] += v
				if v != 0 {
					zero = false
				}
				row = append(row, rupees(v))
			}
			if zero {
				continue
			}
			labels[acctLabel(a)] = a.ID
			t.Rows = append(t.Rows, append(row, rupees(sum)))
		}
		row, sum := []any{"Total " + strings.ToLower(name)}, int64(0)
		for _, v := range tot {
			row, sum = append(row, rupees(v)), sum+v
		}
		t.Rows = append(t.Rows, append(row, rupees(sum)))
	}
	section("Income", "income", totI)
	section("Expenses", "expense", totE)
	row, sum := []any{"Net profit"}, int64(0)
	for i := range cols {
		n := totI[i] - totE[i]
		row, sum = append(row, rupees(n)), sum+n
	}
	t.Rows = append(t.Rows, append(row, rupees(sum)))
	return TableReport{Tbl: t, Labels: labels, Totals: map[string]float64{"net_profit": rupees(sum), "columns": float64(len(cols))}}, nil
}

func blanks(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = ""
	}
	return out
}
