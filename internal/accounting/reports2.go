package accounting

import (
	"context"
	"sort"
	"strings"
	"time"
)

// ---- comparison ----

// CompareRow is one account with the current and the comparison amount.
type CompareRow struct {
	AccountID string  `json:"account_id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Current   float64 `json:"current"`
	Compare   float64 `json:"compare"`
	Change    float64 `json:"change"`
	ChangePct float64 `json:"change_pct"` // 0 when the comparison is zero
}

// PnLCompare is a profit and loss with a second column (previous period, previous year or budget).
type PnLCompare struct {
	Mode          string       `json:"mode"`
	From          string       `json:"from"`
	To            string       `json:"to"`
	CompareFrom   string       `json:"compare_from"`
	CompareTo     string       `json:"compare_to"`
	Income        []CompareRow `json:"income"`
	Expenses      []CompareRow `json:"expenses"`
	TotalIncome   CompareRow   `json:"total_income"`
	TotalExpenses CompareRow   `json:"total_expenses"`
	NetProfit     CompareRow   `json:"net_profit"`
}

func mkCompare(cur, cmp float64) (change, pct float64) {
	change = rupees(paise(cur) - paise(cmp))
	if cmp != 0 {
		pct = float64(int64((change/absf(cmp))*10000+0.5*signf(change))) / 100
	}
	return
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func signf(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// ComparisonRange returns the range to compare with: the same-length period just
// before (prev_period) or the same dates one year earlier (prev_year).
func ComparisonRange(from, to, mode string) (string, string, error) {
	f, err := time.Parse("2006-01-02", from)
	if err != nil {
		return "", "", invalid("bad from date")
	}
	t, err := time.Parse("2006-01-02", to)
	if err != nil {
		return "", "", invalid("bad to date")
	}
	switch mode {
	case "prev_year":
		return addMonthsClamped(f, -12).Format("2006-01-02"), addMonthsClamped(t, -12).Format("2006-01-02"), nil
	case "prev_period":
		days := int(t.Sub(f).Hours()/24) + 1
		return f.AddDate(0, 0, -days).Format("2006-01-02"), f.AddDate(0, 0, -1).Format("2006-01-02"), nil
	}
	return "", "", invalid("compare must be prev_period, prev_year or budget")
}

func mergePnL(cur, cmp PnL) PnLCompare {
	out := PnLCompare{From: cur.From, To: cur.To, CompareFrom: cmp.From, CompareTo: cmp.To, Income: []CompareRow{}, Expenses: []CompareRow{}}
	merge := func(a, b []ReportRow, typ string) []CompareRow {
		idx := map[string]*CompareRow{}
		var order []string
		put := func(r ReportRow, isCur bool) {
			c, ok := idx[r.AccountID]
			if !ok {
				c = &CompareRow{AccountID: r.AccountID, Code: r.Code, Name: r.Name, Type: typ}
				idx[r.AccountID] = c
				order = append(order, r.AccountID)
			}
			if isCur {
				c.Current = r.Amount
			} else {
				c.Compare = r.Amount
			}
		}
		for _, r := range a {
			put(r, true)
		}
		for _, r := range b {
			put(r, false)
		}
		sort.Slice(order, func(i, j int) bool { return idx[order[i]].Code < idx[order[j]].Code })
		res := []CompareRow{}
		for _, id := range order {
			c := idx[id]
			c.Change, c.ChangePct = mkCompare(c.Current, c.Compare)
			res = append(res, *c)
		}
		return res
	}
	out.Income = merge(cur.Income, cmp.Income, "income")
	out.Expenses = merge(cur.Expenses, cmp.Expenses, "expense")
	tot := func(name string, c, p float64) CompareRow {
		r := CompareRow{Name: name, Current: c, Compare: p}
		r.Change, r.ChangePct = mkCompare(c, p)
		return r
	}
	out.TotalIncome = tot("Total income", cur.TotalIncome, cmp.TotalIncome)
	out.TotalExpenses = tot("Total expenses", cur.TotalExpenses, cmp.TotalExpenses)
	out.NetProfit = tot("Net profit", cur.NetProfit, cmp.NetProfit)
	return out
}

// BuildPnLCompare compares the period with the previous period or year.
func BuildPnLCompare(accts []Account, ps []Posting, from, to, mode string) (PnLCompare, error) {
	cf, ct, err := ComparisonRange(from, to, mode)
	if err != nil {
		return PnLCompare{}, err
	}
	out := mergePnL(BuildPnL(accts, ps, from, to), BuildPnL(accts, ps, cf, ct))
	out.Mode = mode
	return out, nil
}

// BalanceSheetCompare is the balance sheet at two dates.
type BalanceSheetCompare struct {
	AsOf    string       `json:"as_of"`
	Prior   string       `json:"compare_as_of"`
	Rows    []CompareRow `json:"rows"` // assets, liabilities, equity with Type set
	Balance bool         `json:"balanced"`
}

// BuildBalanceSheetCompare lines up the balance sheet at asOf with one year earlier.
func BuildBalanceSheetCompare(accts []Account, ps []Posting, asOf string, fyStartMonth int) (BalanceSheetCompare, error) {
	t, err := time.Parse("2006-01-02", asOf)
	if err != nil {
		return BalanceSheetCompare{}, invalid("bad date")
	}
	prior := addMonthsClamped(t, -12).Format("2006-01-02")
	a := BuildBalanceSheet(accts, ps, asOf, fyStartMonth)
	b := BuildBalanceSheet(accts, ps, prior, fyStartMonth)
	out := BalanceSheetCompare{AsOf: asOf, Prior: prior, Rows: []CompareRow{}, Balance: a.Balanced && b.Balanced}
	add := func(typ string, x, y []ReportRow) {
		idx := map[string]int{}
		var rows []CompareRow
		for _, r := range x {
			idx[r.AccountID] = len(rows)
			rows = append(rows, CompareRow{AccountID: r.AccountID, Code: r.Code, Name: r.Name, Type: typ, Current: r.Amount})
		}
		for _, r := range y {
			if i, ok := idx[r.AccountID]; ok {
				rows[i].Compare = r.Amount
			} else {
				rows = append(rows, CompareRow{AccountID: r.AccountID, Code: r.Code, Name: r.Name, Type: typ, Compare: r.Amount})
			}
		}
		for i := range rows {
			rows[i].Change, rows[i].ChangePct = mkCompare(rows[i].Current, rows[i].Compare)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Code < rows[j].Code })
		out.Rows = append(out.Rows, rows...)
	}
	add("asset", a.Assets, b.Assets)
	add("liability", a.Liabilities, b.Liabilities)
	add("equity", a.Equity, b.Equity)
	extra := func(name string, c, p float64) {
		r := CompareRow{Name: name, Type: "equity", Current: c, Compare: p}
		r.Change, r.ChangePct = mkCompare(c, p)
		out.Rows = append(out.Rows, r)
	}
	extra("Retained earnings", a.PriorEarnings, b.PriorEarnings)
	extra("Current year earnings", a.CurrentYearPL, b.CurrentYearPL)
	return out, nil
}

// ---- P&L by class ----

// ClassPnL is the profit and loss split by class / cost center / tag.
type ClassPnL struct {
	From    string                 `json:"from"`
	To      string                 `json:"to"`
	Classes []string               `json:"classes"` // column order; "" is shown as "(untagged)"
	Rows    []ClassPnLRow          `json:"rows"`
	Totals  map[string]ClassTotals `json:"totals"`
}

// ClassPnLRow is one income or expense account across classes.
type ClassPnLRow struct {
	AccountID string             `json:"account_id"`
	Code      string             `json:"code"`
	Name      string             `json:"name"`
	Type      string             `json:"type"`
	ByClass   map[string]float64 `json:"by_class"`
	Total     float64            `json:"total"`
}

// ClassTotals is income, expenses and profit for one class.
type ClassTotals struct {
	Income   float64 `json:"income"`
	Expenses float64 `json:"expenses"`
	Profit   float64 `json:"profit"`
}

// BuildPnLByClass splits income and expenses by the class on each journal line.
func BuildPnLByClass(accts []Account, ps []Posting, from, to string) ClassPnL {
	am := accountMap(accts)
	type key struct{ acct, class string }
	net := map[key]int64{}
	classes := map[string]bool{}
	for _, p := range ps {
		if (from != "" && p.Date < from) || (to != "" && p.Date > to) {
			continue
		}
		a := am[p.AccountID]
		if a.Type != "income" && a.Type != "expense" {
			continue
		}
		net[key{p.AccountID, p.Class}] += p.Debit - p.Credit
		classes[p.Class] = true
	}
	out := ClassPnL{From: from, To: to, Classes: []string{}, Rows: []ClassPnLRow{}, Totals: map[string]ClassTotals{}}
	for c := range classes {
		out.Classes = append(out.Classes, c)
	}
	sort.Slice(out.Classes, func(i, j int) bool {
		if out.Classes[i] == "" || out.Classes[j] == "" {
			return out.Classes[i] != "" // untagged last
		}
		return out.Classes[i] < out.Classes[j]
	})
	totals := map[string]*[2]int64{}
	for _, a := range accts {
		var row *ClassPnLRow
		var sum int64
		for _, c := range out.Classes {
			n := net[key{a.ID, c}]
			if n == 0 {
				continue
			}
			if row == nil {
				row = &ClassPnLRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Type: a.Type, ByClass: map[string]float64{}}
			}
			v := n
			if a.Type == "income" {
				v = -n
			}
			row.ByClass[c] = rupees(v)
			sum += v
			t := totals[c]
			if t == nil {
				t = &[2]int64{}
				totals[c] = t
			}
			if a.Type == "income" {
				t[0] += v
			} else {
				t[1] += v
			}
		}
		if row != nil {
			row.Total = rupees(sum)
			out.Rows = append(out.Rows, *row)
		}
	}
	for c, t := range totals {
		out.Totals[c] = ClassTotals{Income: rupees(t[0]), Expenses: rupees(t[1]), Profit: rupees(t[0] - t[1])}
	}
	return out
}

// ListClasses returns every class used on a journal line.
func (r *Repository) ListClasses(ctx context.Context, orgID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT class FROM adrient.acct_lines WHERE org_id=$1 AND class<>'' ORDER BY class`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// ---- budgets ----

// BudgetCell is the budget of one account for one month ("YYYY-MM").
type BudgetCell struct {
	AccountID string  `json:"account_id"`
	Month     string  `json:"month"`
	Amount    float64 `json:"amount"`
}

// SaveBudgets upserts budget cells. A zero amount removes the cell.
func (r *Repository) SaveBudgets(ctx context.Context, orgID, userID string, cells []BudgetCell) (int, error) {
	if len(cells) > 5000 {
		return 0, invalid("too many budget cells in one request")
	}
	for _, c := range cells {
		if !isUUID(c.AccountID) {
			return 0, invalid("bad account")
		}
		if _, err := time.Parse("2006-01", c.Month); err != nil {
			return 0, invalid("month must be YYYY-MM")
		}
		if c.Amount < 0 {
			return 0, invalid("a budget cannot be negative")
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	for _, c := range cells {
		if paise(c.Amount) == 0 {
			_, err = tx.Exec(ctx, `DELETE FROM adrient.acct_budgets WHERE org_id=$1 AND account_id=$2::uuid AND month=($3||'-01')::date`, orgID, c.AccountID, c.Month)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO adrient.acct_budgets (org_id, account_id, month, amount)
				SELECT $1,$2::uuid,($3||'-01')::date,$4 WHERE EXISTS (SELECT 1 FROM adrient.acct_accounts WHERE org_id=$1 AND id=$2::uuid)
				ON CONFLICT (org_id, account_id, month) DO UPDATE SET amount=EXCLUDED.amount`, orgID, c.AccountID, c.Month, rupees(paise(c.Amount)))
		}
		if err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	r.audit(ctx, orgID, userID, "save", "budget", "", strings.TrimSpace(strings.Join([]string{itoa(len(cells)), "cells"}, " ")))
	return len(cells), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// ListBudgets returns the non-zero budget cells between two months (inclusive, YYYY-MM).
func (r *Repository) ListBudgets(ctx context.Context, orgID, fromMonth, toMonth string) ([]BudgetCell, error) {
	rows, err := r.pool.Query(ctx, `SELECT account_id::text, to_char(month,'YYYY-MM'), amount::float8 FROM adrient.acct_budgets
		WHERE org_id=$1 AND month >= ($2||'-01')::date AND month <= ($3||'-01')::date ORDER BY month`, orgID, fromMonth, toMonth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BudgetCell{}
	for rows.Next() {
		var c BudgetCell
		if err := rows.Scan(&c.AccountID, &c.Month, &c.Amount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// BudgetVsActual lists every income/expense account with budget, actual and variance.
type BudgetVsActual struct {
	From      string       `json:"from"`
	To        string       `json:"to"`
	Income    []CompareRow `json:"income"`   // Compare = budget
	Expenses  []CompareRow `json:"expenses"` // Compare = budget
	NetActual float64      `json:"net_actual"`
	NetBudget float64      `json:"net_budget"`
}

// BuildBudgetVsActual compares actuals in [from,to] with the budgets of the months they cover.
// Variance (Change) is actual minus budget.
func BuildBudgetVsActual(accts []Account, ps []Posting, cells []BudgetCell, from, to string) BudgetVsActual {
	pnl := BuildPnL(accts, ps, from, to)
	bud := map[string]int64{}
	for _, c := range cells {
		if c.Month >= from[:7] && c.Month <= to[:7] {
			bud[c.AccountID] += paise(c.Amount)
		}
	}
	out := BudgetVsActual{From: from, To: to, Income: []CompareRow{}, Expenses: []CompareRow{}}
	seen := map[string]bool{}
	build := func(rows []ReportRow, typ string) []CompareRow {
		res := []CompareRow{}
		for _, r := range rows {
			seen[r.AccountID] = true
			c := CompareRow{AccountID: r.AccountID, Code: r.Code, Name: r.Name, Type: typ, Current: r.Amount, Compare: rupees(bud[r.AccountID])}
			c.Change, c.ChangePct = mkCompare(c.Current, c.Compare)
			res = append(res, c)
		}
		return res
	}
	out.Income = build(pnl.Income, "income")
	out.Expenses = build(pnl.Expenses, "expense")
	for _, a := range accts {
		if seen[a.ID] || bud[a.ID] == 0 || (a.Type != "income" && a.Type != "expense") {
			continue
		}
		c := CompareRow{AccountID: a.ID, Code: a.Code, Name: a.Name, Type: a.Type, Compare: rupees(bud[a.ID])}
		c.Change, c.ChangePct = mkCompare(0, c.Compare)
		if a.Type == "income" {
			out.Income = append(out.Income, c)
		} else {
			out.Expenses = append(out.Expenses, c)
		}
	}
	var bi, be int64
	for _, c := range out.Income {
		bi += paise(c.Compare)
	}
	for _, c := range out.Expenses {
		be += paise(c.Compare)
	}
	out.NetActual, out.NetBudget = pnl.NetProfit, rupees(bi-be)
	return out
}
