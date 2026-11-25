package accounting

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AuthFuncs supplies the caller's identity from the request context.
type AuthFuncs struct {
	UserID   func(r *http.Request) (string, bool)
	UserName func(r *http.Request) string
	OrgID    func(r *http.Request) (string, bool)
}

// HTTPHandler serves /api/v1/accounting/* inside the auth wall.
type HTTPHandler struct {
	repo *Repository
	auth AuthFuncs
	pos  PosSource
	inv  InvoiceSource
}

// NewHTTPHandler constructs the handler. pos and inv may be nil (sync is then skipped for that app).
func NewHTTPHandler(repo *Repository, auth AuthFuncs, pos PosSource, inv InvoiceSource) *HTTPHandler {
	return &HTTPHandler{repo: repo, auth: auth, pos: pos, inv: inv}
}

const mount = "/api/v1/accounting"

// Match reports whether path belongs to the authenticated surface.
func (h *HTTPHandler) Match(path string) bool {
	return path == mount || strings.HasPrefix(path, mount+"/")
}

func today() string { return time.Now().In(time.FixedZone("IST", 5*3600+1800)).Format("2006-01-02") }

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.auth.OrgID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, _ := h.auth.UserID(r)
	userName := ""
	if h.auth.UserName != nil {
		userName = h.auth.UserName(r)
	}
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, mount), "/"), "/")
	ctx, m, q := r.Context(), r.Method, r.URL.Query()
	n := len(seg)
	repo := h.repo

	switch {
	case n == 1 && seg[0] == "settings" && m == http.MethodGet:
		reply(w)(repo.GetSettings(ctx, orgID))
	case n == 1 && seg[0] == "settings" && m == http.MethodPut:
		var s Settings
		if decode(w, r, &s) {
			reply(w)(repo.SaveSettings(ctx, orgID, userID, s))
		}
	case n == 1 && seg[0] == "capabilities" && m == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"pos": h.pos != nil, "invoicing": h.inv != nil})
	case n == 1 && seg[0] == "summary" && m == http.MethodGet:
		reply(w)(h.summary(ctx, orgID))

	case n == 1 && seg[0] == "accounts" && m == http.MethodGet:
		a, err := repo.ListAccounts(ctx, orgID)
		replyKey(w, "accounts", a, err)
	case n == 1 && seg[0] == "accounts" && m == http.MethodPost:
		var a Account
		if decode(w, r, &a) {
			reply(w)(repo.SaveAccount(ctx, orgID, userID, "", a))
		}
	case n == 2 && seg[0] == "accounts" && m == http.MethodPut:
		var a Account
		if decode(w, r, &a) {
			reply(w)(repo.SaveAccount(ctx, orgID, userID, seg[1], a))
		}
	case n == 2 && seg[0] == "accounts" && m == http.MethodDelete:
		replyOK(w, repo.DeleteAccount(ctx, orgID, userID, seg[1]))
	case n == 3 && seg[0] == "accounts" && seg[2] == "register" && m == http.MethodGet:
		h.register(w, r, orgID, seg[1])

	case n == 1 && seg[0] == "journals" && m == http.MethodGet:
		h.journalPage(w, r, orgID)
	case n == 1 && seg[0] == "journals" && m == http.MethodPost:
		var in JournalInput
		if decode(w, r, &in) {
			in.Source, in.SourceID, in.SourceHash = "manual", "", ""
			j, _, err := repo.PostJournal(ctx, orgID, userID, in)
			reply(w)(j, err)
		}
	case n == 2 && seg[0] == "journals" && m == http.MethodGet:
		reply(w)(repo.GetJournal(ctx, orgID, seg[1]))
	case n == 2 && seg[0] == "journals" && m == http.MethodPut:
		var in JournalInput
		if decode(w, r, &in) {
			reply(w)(repo.UpdateManual(ctx, orgID, userID, seg[1], in))
		}
	case n == 2 && seg[0] == "journals" && m == http.MethodDelete:
		replyOK(w, repo.DeleteJournal(ctx, orgID, userID, seg[1]))

	case n == 1 && seg[0] == "sync" && m == http.MethodPost:
		res, err := repo.Sync(ctx, orgID, userID, h.pos, h.inv, q.Get("full") == "1")
		reply(w)(res, err)

	case n == 2 && seg[0] == "reports" && m == http.MethodGet:
		h.report(w, r, orgID, seg[1])

	case n == 2 && seg[0] == "bank" && seg[1] == "lines" && m == http.MethodGet:
		h.bankLinesPage(w, r, orgID)
	case n == 2 && seg[0] == "bank" && seg[1] == "import" && m == http.MethodPost:
		var b struct {
			AccountID string `json:"account_id"`
			CSV       string `json:"csv"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.ImportCSV(ctx, orgID, userID, b.AccountID, b.CSV))
		}
	case n == 2 && seg[0] == "bank" && seg[1] == "apply-rules" && m == http.MethodPost:
		c, a, err := repo.ApplyRules(ctx, orgID, userID)
		reply(w)(map[string]int{"categorized": c, "added": a}, err)
	case n == 4 && seg[0] == "bank" && seg[1] == "lines" && seg[3] == "add" && m == http.MethodPost:
		var b struct {
			CategoryID string `json:"category_id"`
			Memo       string `json:"memo"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.AddBankLine(ctx, orgID, userID, seg[2], b.CategoryID, b.Memo))
		}
	case n == 4 && seg[0] == "bank" && seg[1] == "lines" && seg[3] == "exclude" && m == http.MethodPost:
		replyOK(w, repo.ExcludeBankLine(ctx, orgID, userID, seg[2]))
	case n == 4 && seg[0] == "bank" && seg[1] == "lines" && seg[3] == "undo" && m == http.MethodPost:
		replyOK(w, repo.UndoBankLine(ctx, orgID, userID, seg[2]))
	case n == 4 && seg[0] == "bank" && seg[1] == "lines" && seg[3] == "category" && m == http.MethodPut:
		var b struct {
			CategoryID string `json:"category_id"`
		}
		if decode(w, r, &b) {
			_, err := repo.pool.Exec(ctx, `UPDATE adrient.acct_bank_lines SET category_id=NULLIF($3,'')::uuid WHERE org_id=$1 AND id=$2::uuid AND status='pending'`, orgID, seg[2], b.CategoryID)
			replyOK(w, err)
		}

	case n == 2 && seg[0] == "bank" && seg[1] == "rules" && m == http.MethodGet:
		l, err := repo.ListRules(ctx, orgID)
		replyKey(w, "rules", l, err)
	case n == 2 && seg[0] == "bank" && seg[1] == "rules" && m == http.MethodPost:
		var x Rule
		if decode(w, r, &x) {
			reply(w)(repo.SaveRule(ctx, orgID, userID, "", x))
		}
	case n == 3 && seg[0] == "bank" && seg[1] == "rules" && m == http.MethodPut:
		var x Rule
		if decode(w, r, &x) {
			reply(w)(repo.SaveRule(ctx, orgID, userID, seg[2], x))
		}
	case n == 3 && seg[0] == "bank" && seg[1] == "rules" && m == http.MethodDelete:
		replyOK(w, repo.DeleteRule(ctx, orgID, userID, seg[2]))

	case n == 1 && seg[0] == "reconcile" && m == http.MethodGet:
		reply(w)(repo.ReconcileDraft(ctx, orgID, q.Get("account_id"), q.Get("statement_date")))
	case n == 1 && seg[0] == "reconcile" && m == http.MethodPost:
		var b struct {
			AccountID     string   `json:"account_id"`
			StatementDate string   `json:"statement_date"`
			Ending        float64  `json:"ending_balance"`
			LineIDs       []string `json:"line_ids"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.FinishReconciliation(ctx, orgID, userID, b.AccountID, b.StatementDate, b.Ending, b.LineIDs))
		}
	case n == 2 && seg[0] == "reconcile" && seg[1] == "history" && m == http.MethodGet:
		l, err := repo.ReconciliationHistory(ctx, orgID, q.Get("account_id"))
		replyKey(w, "history", l, err)
	case n == 3 && seg[0] == "reconcile" && seg[2] == "undo" && m == http.MethodPost:
		replyOK(w, repo.UndoReconciliation(ctx, orgID, userID, seg[1]))

	case n == 1 && seg[0] == "expenses" && m == http.MethodGet:
		l, err := repo.ListExpenses(ctx, orgID, q.Get("report_id"), q.Get("from"), q.Get("to"), q.Get("q"))
		replyKey(w, "expenses", l, err)
	case n == 1 && seg[0] == "expenses" && m == http.MethodPost:
		var e Expense
		if decode(w, r, &e) {
			reply(w)(repo.SaveExpense(ctx, orgID, userID, "", e))
		}
	case n == 2 && seg[0] == "expenses" && m == http.MethodPut:
		var e Expense
		if decode(w, r, &e) {
			reply(w)(repo.SaveExpense(ctx, orgID, userID, seg[1], e))
		}
	case n == 2 && seg[0] == "expenses" && m == http.MethodDelete:
		replyOK(w, repo.DeleteExpense(ctx, orgID, userID, seg[1]))
	case n == 3 && seg[0] == "expenses" && seg[2] == "pay" && m == http.MethodPost:
		var b struct {
			AccountID string `json:"account_id"`
			Date      string `json:"date"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.PayBill(ctx, orgID, userID, seg[1], b.AccountID, orDefault(b.Date, today())))
		}

	case n == 1 && seg[0] == "expense-reports" && m == http.MethodGet:
		l, err := repo.ListReports(ctx, orgID)
		replyKey(w, "reports", l, err)
	case n == 1 && seg[0] == "expense-reports" && m == http.MethodPost:
		var x ExpenseReport
		if decode(w, r, &x) {
			reply(w)(repo.SaveReport(ctx, orgID, userID, userName, "", x))
		}
	case n == 2 && seg[0] == "expense-reports" && m == http.MethodGet:
		reply(w)(repo.GetReport(ctx, orgID, seg[1]))
	case n == 2 && seg[0] == "expense-reports" && m == http.MethodPut:
		var x ExpenseReport
		if decode(w, r, &x) {
			reply(w)(repo.SaveReport(ctx, orgID, userID, userName, seg[1], x))
		}
	case n == 2 && seg[0] == "expense-reports" && m == http.MethodDelete:
		replyOK(w, repo.DeleteReport(ctx, orgID, userID, seg[1]))
	case n == 3 && seg[0] == "expense-reports" && m == http.MethodPost:
		var b struct {
			Note      string `json:"note"`
			AccountID string `json:"account_id"`
			Date      string `json:"date"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&b)
		reply(w)(repo.ReportAction(ctx, orgID, userID, seg[1], seg[2], b.Note, b.AccountID, orDefault(b.Date, today())))

	case n == 1 && seg[0] == "audit" && m == http.MethodGet:
		lim, _ := strconv.Atoi(q.Get("limit"))
		l, err := repo.ListAudit(ctx, orgID, lim)
		replyKey(w, "audit", l, err)
	case n == 1 && seg[0] == "search" && m == http.MethodGet:
		h.search(w, r, orgID, strings.TrimSpace(q.Get("q")))
	default:
		if h.extra(w, r, orgID, userID, userName, seg) {
			return
		}
		http.NotFound(w, r)
	}
}

func (h *HTTPHandler) report(w http.ResponseWriter, r *http.Request, orgID, name string) {
	ctx, q := r.Context(), r.URL.Query()
	st, err := h.repo.GetSettings(ctx, orgID)
	if err != nil {
		httpErr(w, err)
		return
	}
	asOf := orDefault(q.Get("as_of"), orDefault(q.Get("to"), today()))
	from := q.Get("from")
	to := orDefault(q.Get("to"), asOf)
	if from == "" {
		from = FYStart(to, st.FYStartMonth)
	}
	for _, d := range []string{asOf, from, to} {
		if _, err := validDate(d); err != nil {
			httpErr(w, err)
			return
		}
	}
	accts, err := h.repo.ListAccounts(ctx, orgID)
	if err != nil {
		httpErr(w, err)
		return
	}
	last := asOf
	if to > last {
		last = to
	}
	ps, err := h.repo.LoadPostings(ctx, orgID, last)
	if err != nil {
		httpErr(w, err)
		return
	}
	var v any
	cmp := q.Get("compare")
	switch name {
	case "trial-balance":
		v = BuildTrialBalance(accts, ps, asOf)
	case "pnl":
		switch cmp {
		case "":
			v = BuildPnL(accts, ps, from, to)
		case "budget":
			cells, err := h.repo.ListBudgets(ctx, orgID, from[:7], to[:7])
			if err != nil {
				httpErr(w, err)
				return
			}
			b := BuildBudgetVsActual(accts, ps, cells, from, to)
			v = PnLCompare{Mode: "budget", From: from, To: to, CompareFrom: from, CompareTo: to, Income: b.Income, Expenses: b.Expenses,
				TotalIncome: sumCompare("Total income", b.Income), TotalExpenses: sumCompare("Total expenses", b.Expenses),
				NetProfit: CompareRow{Name: "Net profit", Current: b.NetActual, Compare: b.NetBudget, Change: rupees(paise(b.NetActual) - paise(b.NetBudget))}}
		default:
			c, err := BuildPnLCompare(accts, ps, from, to, cmp)
			if err != nil {
				httpErr(w, err)
				return
			}
			v = c
		}
	case "balance-sheet":
		if cmp == "prev_year" {
			c, err := BuildBalanceSheetCompare(accts, ps, asOf, st.FYStartMonth)
			if err != nil {
				httpErr(w, err)
				return
			}
			v = c
		} else {
			v = BuildBalanceSheet(accts, ps, asOf, st.FYStartMonth)
		}
	case "cash-flow":
		v = BuildCashFlow(accts, ps, from, to)
	case "aged-receivables":
		v = BuildAging(accts, ps, asOf, "receivable")
	case "aged-payables":
		v = BuildAging(accts, ps, asOf, "payable")
	case "gst":
		v = BuildGST(accts, ps, from, to)
	case "general-ledger":
		v = BuildGeneralLedger(accts, ps, from, to)
	case "journal":
		v = BuildJournalReport(accts, ps, from, to)
	case "transaction-list":
		v = BuildTransactionList(ps, from, to)
	case "account-list":
		v = BuildAccountList(accts, ps, asOf)
	case "customer-balances":
		v = BuildContactBalances(accts, ps, asOf, "receivable")
	case "vendor-balances":
		v = BuildContactBalances(accts, ps, asOf, "payable")
	case "sales-by-customer":
		v = BuildByContact(accts, ps, from, to, "sales")
	case "expenses-by-vendor":
		v = BuildByContact(accts, ps, from, to, "expenses")
	case "pnl-columns":
		c, err := BuildPnLColumns(accts, ps, from, to, orDefault(q.Get("by"), "month"), st.FYStartMonth)
		if err != nil {
			httpErr(w, err)
			return
		}
		v = c
	case "pnl-by-class":
		v = BuildPnLByClass(accts, ps, from, to)
	case "budget-vs-actual":
		cells, err := h.repo.ListBudgets(ctx, orgID, from[:7], to[:7])
		if err != nil {
			httpErr(w, err)
			return
		}
		v = BuildBudgetVsActual(accts, ps, cells, from, to)
	case "gstr1":
		g, err := h.repo.BuildGSTR1(ctx, orgID, from, to, h.inv, h.pos)
		if err != nil {
			httpErr(w, err)
			return
		}
		v = g
	case "gstr3b":
		v = BuildGSTR3B(accts, ps, st, from, to)
	case "hsn-summary":
		x, err := h.repo.BuildHSNSummary(ctx, orgID, from, to, h.inv)
		if err != nil {
			httpErr(w, err)
			return
		}
		v = x
	case "tds":
		x, err := h.repo.BuildTDSReport(ctx, orgID, q.Get("direction"), from, to)
		if err != nil {
			httpErr(w, err)
			return
		}
		v = x
	case "itc-reconciliation":
		x, err := h.repo.ReconcileITC(ctx, orgID, orDefault(q.Get("period"), to[:7]))
		if err != nil {
			httpErr(w, err)
			return
		}
		v = x
	default:
		http.NotFound(w, r)
		return
	}
	if q.Get("format") == "table" {
		t, ok := TableOf(v)
		if !ok {
			http.Error(w, "this report has no table view", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, tableAsJSON(v, t))
		return
	}
	if f := q.Get("format"); f != "" && f != "json" {
		t, ok := TableOf(v)
		if !ok || !writeTable(w, t, f, name+"-"+to) {
			http.Error(w, "unsupported export format", http.StatusBadRequest)
		}
		return
	}
	if tr, ok := v.(TableReport); ok { // wave-6 reports are tables; plain JSON gets the same shape as format=table
		writeJSON(w, http.StatusOK, tableAsJSON(v, tr.Tbl))
		return
	}
	reply(w)(v, nil)
}

func (h *HTTPHandler) register(w http.ResponseWriter, r *http.Request, orgID, id string) {
	ctx, q := r.Context(), r.URL.Query()
	accts, err := h.repo.ListAccounts(ctx, orgID)
	if err != nil {
		httpErr(w, err)
		return
	}
	var acct *Account
	for i := range accts {
		if accts[i].ID == id {
			acct = &accts[i]
		}
	}
	if acct == nil {
		httpErr(w, ErrNotFound)
		return
	}
	ps, err := h.repo.LoadPostings(ctx, orgID, q.Get("to"))
	if err != nil {
		httpErr(w, err)
		return
	}
	rows, opening := BuildRegister(*acct, ps, q.Get("from"), q.Get("to"))
	writeJSON(w, http.StatusOK, map[string]any{"account": acct, "opening": opening, "rows": rows})
}

// Summary feeds the home screen.
type Summary struct {
	Cash         float64 `json:"cash"`
	Receivables  float64 `json:"receivables"`
	Payables     float64 `json:"payables"`
	GSTPayable   float64 `json:"gst_payable"`
	MonthIncome  float64 `json:"month_income"`
	MonthExpense float64 `json:"month_expense"`
	YearProfit   float64 `json:"year_profit"`
	Pending      int     `json:"pending_bank_lines"`
	Journals     int     `json:"journals"`
	Balanced     bool    `json:"balanced"`
}

func (h *HTTPHandler) summary(ctx context.Context, orgID string) (Summary, error) {
	var s Summary
	st, err := h.repo.GetSettings(ctx, orgID)
	if err != nil {
		return s, err
	}
	accts, err := h.repo.ListAccounts(ctx, orgID)
	if err != nil {
		return s, err
	}
	ps, err := h.repo.LoadPostings(ctx, orgID, "")
	if err != nil {
		return s, err
	}
	t := today()
	monthStart := t[:8] + "01"
	for _, a := range accts {
		switch {
		case isCash(a):
			s.Cash += a.Balance
		case a.Subtype == "receivable":
			s.Receivables += a.Balance
		case a.Subtype == "payable":
			s.Payables += a.Balance
		}
	}
	g := BuildGST(accts, ps, "", t)
	s.GSTPayable = g.NetPayable
	s.MonthIncome = BuildPnL(accts, ps, monthStart, t).TotalIncome
	s.MonthExpense = BuildPnL(accts, ps, monthStart, t).TotalExpenses
	s.YearProfit = BuildPnL(accts, ps, FYStart(t, st.FYStartMonth), t).NetProfit
	s.Balanced = BuildTrialBalance(accts, ps, "").Balanced
	_ = h.repo.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.acct_bank_lines WHERE org_id=$1 AND status='pending'`, orgID).Scan(&s.Pending)
	_ = h.repo.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.acct_journals WHERE org_id=$1 AND NOT voided`, orgID).Scan(&s.Journals)
	return s, nil
}

func (h *HTTPHandler) search(w http.ResponseWriter, r *http.Request, orgID, q string) {
	ctx := r.Context()
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"journals": []Journal{}, "accounts": []Account{}, "expenses": []Expense{}})
		return
	}
	js, err := h.repo.ListJournals(ctx, orgID, JournalFilter{Query: q, Limit: 25})
	if err != nil {
		httpErr(w, err)
		return
	}
	accts, err := h.repo.ListAccounts(ctx, orgID)
	if err != nil {
		httpErr(w, err)
		return
	}
	lq := strings.ToLower(q)
	matched := []Account{}
	for _, a := range accts {
		if strings.Contains(strings.ToLower(a.Name), lq) || strings.HasPrefix(a.Code, q) {
			matched = append(matched, a)
		}
	}
	ex, err := h.repo.ListExpenses(ctx, orgID, "", "", "", q)
	if err != nil {
		httpErr(w, err)
		return
	}
	if len(ex) > 25 {
		ex = ex[:25]
	}
	writeJSON(w, http.StatusOK, map[string]any{"journals": js, "accounts": matched, "expenses": ex})
}

// ---- helpers ----

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(v); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func httpErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrLocked):
		http.Error(w, strings.TrimPrefix(err.Error(), "locked: "), http.StatusConflict)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid: "), http.StatusBadRequest)
	default:
		slog.Error("accounting", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func reply(w http.ResponseWriter) func(v any, err error) {
	return func(v any, err error) {
		if err != nil {
			httpErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func replyKey(w http.ResponseWriter, key string, v any, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{key: v})
}

func replyOK(w http.ResponseWriter, err error) {
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func sumCompare(label string, rows []CompareRow) CompareRow {
	var c, b int64
	for _, r := range rows {
		c += paise(r.Current)
		b += paise(r.Compare)
	}
	out := CompareRow{Name: label, Current: rupees(c), Compare: rupees(b)}
	out.Change, out.ChangePct = mkCompare(out.Current, out.Compare)
	return out
}
