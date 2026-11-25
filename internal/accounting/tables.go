package accounting

import (
	"fmt"
	"net/http"
	"strings"
)

func rowsOf(rs []ReportRow) [][]any {
	out := [][]any{}
	for _, r := range rs {
		out = append(out, []any{strings.TrimSpace(r.Code + " " + r.Name), r.Amount})
	}
	return out
}

func cmpRows(label string, rs []CompareRow, cmpHead string) [][]any {
	out := [][]any{{label, "", "", "", ""}}
	_ = cmpHead
	for _, r := range rs {
		out = append(out, []any{"  " + strings.TrimSpace(r.Code+" "+r.Name), r.Current, r.Compare, r.Change, r.ChangePct})
	}
	return out
}

// TableOf converts any report value into an exportable table. ok is false for unknown types.
func TableOf(v any) (Table, bool) {
	switch x := v.(type) {
	case TrialBalance:
		t := Table{Title: "Trial Balance", Subtitle: "As of " + x.AsOf, Headers: []string{"Account", "Debit", "Credit"}}
		for _, r := range x.Rows {
			t.Rows = append(t.Rows, []any{strings.TrimSpace(r.Code + " " + r.Name), r.Debit, r.Credit})
		}
		t.Rows = append(t.Rows, []any{"Total", x.TotalDebit, x.TotalCredit})
		return t, true
	case PnL:
		t := Table{Title: "Profit and Loss", Subtitle: x.From + " to " + x.To, Headers: []string{"Account", "Amount"}}
		t.Rows = append(t.Rows, []any{"Income", ""})
		t.Rows = append(t.Rows, rowsOf(x.Income)...)
		t.Rows = append(t.Rows, []any{"Total income", x.TotalIncome}, []any{"Expenses", ""})
		t.Rows = append(t.Rows, rowsOf(x.Expenses)...)
		t.Rows = append(t.Rows, []any{"Total expenses", x.TotalExpenses}, []any{"Net profit", x.NetProfit})
		return t, true
	case PnLCompare:
		head := map[string]string{"prev_period": "Previous period", "prev_year": "Previous year", "budget": "Budget"}[x.Mode]
		t := Table{Title: "Profit and Loss vs " + head, Subtitle: x.From + " to " + x.To + " | compared " + x.CompareFrom + " to " + x.CompareTo,
			Headers: []string{"Account", "Current", head, "Change", "Change %"}}
		t.Rows = append(t.Rows, cmpRows("Income", x.Income, head)...)
		t.Rows = append(t.Rows, []any{"Total income", x.TotalIncome.Current, x.TotalIncome.Compare, x.TotalIncome.Change, x.TotalIncome.ChangePct})
		t.Rows = append(t.Rows, cmpRows("Expenses", x.Expenses, head)...)
		t.Rows = append(t.Rows, []any{"Total expenses", x.TotalExpenses.Current, x.TotalExpenses.Compare, x.TotalExpenses.Change, x.TotalExpenses.ChangePct})
		t.Rows = append(t.Rows, []any{"Net profit", x.NetProfit.Current, x.NetProfit.Compare, x.NetProfit.Change, x.NetProfit.ChangePct})
		return t, true
	case BudgetVsActual:
		t := Table{Title: "Budget vs Actual", Subtitle: x.From + " to " + x.To, Headers: []string{"Account", "Actual", "Budget", "Variance", "Variance %"}}
		t.Rows = append(t.Rows, cmpRows("Income", x.Income, "")...)
		t.Rows = append(t.Rows, cmpRows("Expenses", x.Expenses, "")...)
		t.Rows = append(t.Rows, []any{"Net", x.NetActual, x.NetBudget, rupees(paise(x.NetActual) - paise(x.NetBudget)), ""})
		return t, true
	case BalanceSheet:
		t := Table{Title: "Balance Sheet", Subtitle: "As of " + x.AsOf, Headers: []string{"Account", "Amount"}}
		t.Rows = append(t.Rows, []any{"Assets", ""})
		t.Rows = append(t.Rows, rowsOf(x.Assets)...)
		t.Rows = append(t.Rows, []any{"Total assets", x.TotalAssets}, []any{"Liabilities", ""})
		t.Rows = append(t.Rows, rowsOf(x.Liabilities)...)
		t.Rows = append(t.Rows, []any{"Total liabilities", x.TotalLiabilities}, []any{"Equity", ""})
		t.Rows = append(t.Rows, rowsOf(x.Equity)...)
		t.Rows = append(t.Rows, []any{"Retained earnings", x.PriorEarnings}, []any{"Current year earnings", x.CurrentYearPL}, []any{"Total equity", x.TotalEquity})
		return t, true
	case BalanceSheetCompare:
		t := Table{Title: "Balance Sheet comparison", Subtitle: x.AsOf + " vs " + x.Prior, Headers: []string{"Account", x.AsOf, x.Prior, "Change", "Change %"}}
		for _, r := range x.Rows {
			t.Rows = append(t.Rows, []any{r.Type + ": " + strings.TrimSpace(r.Code+" "+r.Name), r.Current, r.Compare, r.Change, r.ChangePct})
		}
		return t, true
	case CashFlow:
		t := Table{Title: "Cash Flow", Subtitle: x.From + " to " + x.To, Headers: []string{"Item", "Amount"}}
		t.Rows = append(t.Rows, []any{"Opening cash and bank", x.Opening})
		for _, g := range x.Groups {
			t.Rows = append(t.Rows, []any{g.Name, ""})
			t.Rows = append(t.Rows, rowsOf(g.Rows)...)
			t.Rows = append(t.Rows, []any{"Total " + g.Name, g.Total})
		}
		t.Rows = append(t.Rows, []any{"Net change", x.NetChange}, []any{"Closing cash and bank", x.Closing})
		return t, true
	case Aging:
		t := Table{Title: "Aged " + strings.Title(x.Kind) + "s", Subtitle: "As of " + x.AsOf, Headers: []string{"Contact", "Current", "1-30", "31-60", "61-90", "90+", "Total"}}
		for _, r := range append(x.Rows, x.Total) {
			t.Rows = append(t.Rows, []any{r.ContactName, r.Current, r.D1_30, r.D31_60, r.D61_90, r.D90Plus, r.Total})
		}
		return t, true
	case GSTReport:
		t := Table{Title: "GST summary", Subtitle: x.From + " to " + x.To, Headers: []string{"Account", "Amount"}}
		t.Rows = append(t.Rows, []any{"Output tax", ""})
		t.Rows = append(t.Rows, rowsOf(x.Output)...)
		t.Rows = append(t.Rows, []any{"Total output", x.OutputTotal}, []any{"Input tax credit", ""})
		t.Rows = append(t.Rows, rowsOf(x.Input)...)
		t.Rows = append(t.Rows, []any{"Total input", x.InputTotal}, []any{"Net payable", x.NetPayable})
		return t, true
	case ClassPnL:
		h := []string{"Account"}
		for _, c := range x.Classes {
			h = append(h, orDefault(c, "(untagged)"))
		}
		h = append(h, "Total")
		t := Table{Title: "Profit and Loss by class", Subtitle: x.From + " to " + x.To, Headers: h}
		for _, r := range x.Rows {
			row := []any{r.Type + ": " + strings.TrimSpace(r.Code+" "+r.Name)}
			for _, c := range x.Classes {
				row = append(row, r.ByClass[c])
			}
			t.Rows = append(t.Rows, append(row, r.Total))
		}
		prof := []any{"Net profit"}
		var all float64
		for _, c := range x.Classes {
			prof = append(prof, x.Totals[c].Profit)
			all += x.Totals[c].Profit
		}
		t.Rows = append(t.Rows, append(prof, rupees(paise(all))))
		return t, true
	case TableReport:
		return x.Tbl, true
	case TaxReturn:
		return x.Table(), true
	case HSNSummary:
		return x.Table(), true
	case TDSReport:
		return x.Table(), true
	case ITCReconciliation:
		return x.Table(), true
	}
	return Table{}, false
}

// writeTable sends t as csv, xlsx or pdf. It reports false for an unknown format.
func writeTable(w http.ResponseWriter, t Table, format, base string) bool {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, base)
	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, safe))
		_, _ = w.Write(t.CSV())
	case "xlsx":
		b, err := t.XLSX()
		if err != nil {
			httpErr(w, err)
			return true
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.xlsx"`, safe))
		_, _ = w.Write(b)
	case "pdf":
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, safe))
		_, _ = w.Write(t.PDF())
	default:
		return false
	}
	return true
}

// tableJSON is the generic shape the web app renders for any report. Links[i] is the
// account id behind row i (for drill-down to its journals) or "".
type tableJSON struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Headers  []string `json:"headers"`
	Rows     [][]any  `json:"rows"`
	Links    []string `json:"links"`
}

// accountLinks maps the "code name" label used in table rows to the account id.
func accountLinks(v any) map[string]string {
	m := map[string]string{}
	add := func(rs []ReportRow) {
		for _, r := range rs {
			m[strings.TrimSpace(r.Code+" "+r.Name)] = r.AccountID
		}
	}
	addC := func(rs []CompareRow) {
		for _, r := range rs {
			m[strings.TrimSpace(r.Code+" "+r.Name)] = r.AccountID
		}
	}
	switch x := v.(type) {
	case TrialBalance:
		add(x.Rows)
	case PnL:
		add(x.Income)
		add(x.Expenses)
	case PnLCompare:
		addC(x.Income)
		addC(x.Expenses)
	case BudgetVsActual:
		addC(x.Income)
		addC(x.Expenses)
	case BalanceSheet:
		add(x.Assets)
		add(x.Liabilities)
		add(x.Equity)
	case BalanceSheetCompare:
		addC(x.Rows)
	case CashFlow:
		for _, g := range x.Groups {
			add(g.Rows)
		}
	case GSTReport:
		add(x.Output)
		add(x.Input)
	case TableReport:
		for k, id := range x.Labels {
			m[k] = id
		}
	case ClassPnL:
		for _, r := range x.Rows {
			m[r.Type+": "+strings.TrimSpace(r.Code+" "+r.Name)] = r.AccountID
		}
	}
	return m
}

func tableAsJSON(v any, t Table) tableJSON {
	links := accountLinks(v)
	out := tableJSON{Title: t.Title, Subtitle: t.Subtitle, Headers: t.Headers, Rows: t.Rows, Links: make([]string, len(t.Rows))}
	for i, r := range t.Rows {
		if len(r) == 0 {
			continue
		}
		label, _ := r[0].(string)
		out.Links[i] = links[strings.TrimSpace(label)]
	}
	return out
}
