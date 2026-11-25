package accounting

import (
	"net/http"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// extra serves the wave-3 routes. It reports false when the path is not one of them.
func (h *HTTPHandler) extra(w http.ResponseWriter, r *http.Request, orgID, userID, userName string, seg []string) bool {
	ctx, m, q := r.Context(), r.Method, r.URL.Query()
	repo, n := h.repo, len(seg)
	switch {
	// ---- bank line matching (wave 6) ----
	case n == 2 && seg[0] == "bank" && seg[1] == "matches" && m == http.MethodGet:
		l, err := repo.SuggestMatches(ctx, orgID, q.Get("account_id"))
		replyKey(w, "matches", l, err)
	case n == 2 && seg[0] == "bank" && seg[1] == "candidates" && m == http.MethodGet:
		l, err := repo.MatchCandidates(ctx, orgID, q.Get("account_id"))
		replyKey(w, "candidates", l, err)
	case n == 4 && seg[0] == "bank" && seg[1] == "lines" && seg[3] == "match" && m == http.MethodPost:
		var b struct {
			JournalID string `json:"journal_id"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.MatchBankLine(ctx, orgID, userID, seg[2], b.JournalID))
		}

	// ---- exchange rates ----
	case n == 1 && seg[0] == "rates" && m == http.MethodGet:
		l, err := repo.ListRates(ctx, orgID, q.Get("currency"))
		replyKey(w, "rates", l, err)
	case n == 1 && seg[0] == "rates" && m == http.MethodPost:
		var x Rate
		if decode(w, r, &x) {
			reply(w)(repo.SaveRate(ctx, orgID, userID, x))
		}
	case n == 2 && seg[0] == "rates" && seg[1] == "fetch" && m == http.MethodPost:
		c, err := repo.FetchRates(ctx, orgID, userID, nil)
		reply(w)(map[string]int{"stored": c}, err)
	case n == 2 && seg[0] == "rates" && m == http.MethodDelete:
		replyOK(w, repo.DeleteRate(ctx, orgID, userID, seg[1]))
	case n == 1 && seg[0] == "revalue" && m == http.MethodPost:
		var b struct {
			Date string `json:"date"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.Revalue(ctx, orgID, userID, orDefault(b.Date, today())))
		}

	// ---- bills and vendor credits ----
	case n == 1 && seg[0] == "bills" && m == http.MethodGet:
		l, err := repo.ListBills(ctx, orgID, q.Get("kind"), q.Get("status"), q.Get("vendor_id"))
		replyKey(w, "bills", l, err)
	case n == 1 && seg[0] == "bills" && m == http.MethodPost:
		var in BillInput
		if decode(w, r, &in) {
			reply(w)(repo.SaveBill(ctx, orgID, userID, "", in))
		}
	case n == 2 && seg[0] == "bills" && m == http.MethodGet:
		reply(w)(repo.GetBill(ctx, orgID, seg[1]))
	case n == 2 && seg[0] == "bills" && m == http.MethodPut:
		var in BillInput
		if decode(w, r, &in) {
			reply(w)(repo.SaveBill(ctx, orgID, userID, seg[1], in))
		}
	case n == 3 && seg[0] == "bills" && seg[2] == "post" && m == http.MethodPost:
		reply(w)(repo.PostDraftBill(ctx, orgID, userID, seg[1]))
	case n == 3 && seg[0] == "bills" && (seg[2] == "approve" || seg[2] == "reject") && m == http.MethodPost:
		if !authz.Can(ctx, PermApprove) {
			http.Error(w, "forbidden: approving bills needs the accounting approver role", http.StatusForbidden)
			return true
		}
		var b struct {
			Note string `json:"note"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.ApproveBill(ctx, orgID, userID, userName, seg[1], seg[2] == "approve", b.Note))
		}
	case n == 3 && seg[0] == "bills" && seg[2] == "pay" && m == http.MethodPost:
		var p BillPayment
		if decode(w, r, &p) {
			p.Date = orDefault(p.Date, today())
			reply(w)(repo.PayVendorBill(ctx, orgID, userID, seg[1], p))
		}
	case n == 3 && seg[0] == "bills" && seg[2] == "apply-credit" && m == http.MethodPost:
		var b struct {
			CreditID string  `json:"credit_id"`
			Date     string  `json:"date"`
			Amount   float64 `json:"amount"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.ApplyCredit(ctx, orgID, userID, seg[1], b.CreditID, orDefault(b.Date, today()), b.Amount))
		}
	case n == 3 && seg[0] == "bills" && seg[2] == "write-off" && m == http.MethodPost:
		var b struct {
			Date string `json:"date"`
			Note string `json:"note"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.WriteOffBill(ctx, orgID, userID, seg[1], orDefault(b.Date, today()), b.Note))
		}
	case n == 3 && seg[0] == "bills" && seg[2] == "void" && m == http.MethodPost:
		reply(w)(repo.VoidBill(ctx, orgID, userID, seg[1]))
	case n == 4 && seg[0] == "bills" && seg[2] == "payments" && m == http.MethodDelete:
		reply(w)(repo.DeleteBillPayment(ctx, orgID, userID, seg[1], seg[3]))

	// ---- customer credit memos, write-offs ----
	case n == 1 && seg[0] == "credit-memos" && m == http.MethodGet:
		l, err := repo.ListCreditMemos(ctx, orgID)
		replyKey(w, "credit_memos", l, err)
	case n == 1 && seg[0] == "credit-memos" && m == http.MethodPost:
		var in CreditMemoInput
		if decode(w, r, &in) {
			in.Date = orDefault(in.Date, today())
			reply(w)(repo.CreateCreditMemo(ctx, orgID, userID, in))
		}
	case n == 3 && seg[0] == "credit-memos" && seg[2] == "refund" && m == http.MethodPost:
		var b struct {
			Date      string  `json:"date"`
			AccountID string  `json:"account_id"`
			Amount    float64 `json:"amount"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.RefundCreditMemo(ctx, orgID, userID, seg[1], orDefault(b.Date, today()), b.AccountID, b.Amount))
		}
	case n == 3 && seg[0] == "credit-memos" && seg[2] == "void" && m == http.MethodPost:
		reply(w)(repo.VoidCreditMemo(ctx, orgID, userID, seg[1]))
	case n == 1 && seg[0] == "write-off" && m == http.MethodPost:
		var b struct {
			CustomerID   string  `json:"customer_id"`
			CustomerName string  `json:"customer_name"`
			Date         string  `json:"date"`
			Amount       float64 `json:"amount"`
			Note         string  `json:"note"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.WriteOffReceivable(ctx, orgID, userID, b.CustomerID, b.CustomerName, orDefault(b.Date, today()), b.Amount, b.Note))
		}

	// ---- deposits ----
	case n == 1 && seg[0] == "undeposited" && m == http.MethodGet:
		l, err := repo.ListUndeposited(ctx, orgID)
		replyKey(w, "receipts", l, err)
	case n == 1 && seg[0] == "deposits" && m == http.MethodGet:
		l, err := repo.ListDeposits(ctx, orgID)
		replyKey(w, "deposits", l, err)
	case n == 1 && seg[0] == "deposits" && m == http.MethodPost:
		var b struct {
			Date      string   `json:"date"`
			AccountID string   `json:"account_id"`
			Reference string   `json:"reference"`
			LineIDs   []string `json:"line_ids"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.CreateDeposit(ctx, orgID, userID, orDefault(b.Date, today()), b.AccountID, b.Reference, b.LineIDs))
		}
	case n == 2 && seg[0] == "deposits" && m == http.MethodDelete:
		replyOK(w, repo.DeleteDeposit(ctx, orgID, userID, seg[1]))

	// ---- recurring ----
	case n == 1 && seg[0] == "recurring" && m == http.MethodGet:
		l, err := repo.ListRecurring(ctx, orgID)
		replyKey(w, "recurring", l, err)
	case n == 1 && seg[0] == "recurring" && m == http.MethodPost:
		var in Recurring
		if decode(w, r, &in) {
			reply(w)(repo.SaveRecurring(ctx, orgID, userID, "", in))
		}
	case n == 2 && seg[0] == "recurring" && seg[1] == "run" && m == http.MethodPost:
		reply(w)(repo.RunRecurring(ctx, orgID, userID, time.Now().In(time.FixedZone("IST", 5*3600+1800)).Format("2006-01-02")))
	case n == 2 && seg[0] == "recurring" && m == http.MethodPut:
		var in Recurring
		if decode(w, r, &in) {
			reply(w)(repo.SaveRecurring(ctx, orgID, userID, seg[1], in))
		}
	case n == 2 && seg[0] == "recurring" && m == http.MethodDelete:
		replyOK(w, repo.DeleteRecurring(ctx, orgID, userID, seg[1]))

	// ---- classes, budgets ----
	case n == 1 && seg[0] == "classes" && m == http.MethodGet:
		l, err := repo.ListClasses(ctx, orgID)
		replyKey(w, "classes", l, err)
	case n == 1 && seg[0] == "budgets" && m == http.MethodGet:
		fy := orDefault(q.Get("from"), today()[:4]+"-01")
		l, err := repo.ListBudgets(ctx, orgID, fy, orDefault(q.Get("to"), fy[:4]+"-12"))
		replyKey(w, "budgets", l, err)
	case n == 1 && seg[0] == "budgets" && m == http.MethodPut:
		var b struct {
			Cells []BudgetCell `json:"cells"`
		}
		if decode(w, r, &b) {
			c, err := repo.SaveBudgets(ctx, orgID, userID, b.Cells)
			reply(w)(map[string]int{"saved": c}, err)
		}

	// ---- taxes ----
	case n == 1 && seg[0] == "tax-rates" && m == http.MethodGet:
		l, err := repo.ListTaxRates(ctx, orgID)
		replyKey(w, "rates", l, err)
	case n == 1 && seg[0] == "tax-rates" && m == http.MethodPost:
		var t TaxRate
		if decode(w, r, &t) {
			reply(w)(repo.SaveTaxRate(ctx, orgID, userID, "", t))
		}
	case n == 2 && seg[0] == "tax-rates" && m == http.MethodPut:
		var t TaxRate
		if decode(w, r, &t) {
			reply(w)(repo.SaveTaxRate(ctx, orgID, userID, seg[1], t))
		}
	case n == 2 && seg[0] == "tax-rates" && m == http.MethodDelete:
		replyOK(w, repo.DeleteTaxRate(ctx, orgID, userID, seg[1]))
	case n == 1 && seg[0] == "gst-parties" && m == http.MethodGet:
		l, err := repo.ListGSTParties(ctx, orgID)
		replyKey(w, "parties", l, err)
	case n == 1 && seg[0] == "gst-parties" && m == http.MethodPost:
		var p GSTParty
		if decode(w, r, &p) {
			reply(w)(repo.SaveGSTParty(ctx, orgID, userID, p))
		}
	case n == 1 && seg[0] == "hsn-map" && m == http.MethodPost:
		var b struct {
			Match string `json:"match"`
			HSN   string `json:"hsn"`
		}
		if decode(w, r, &b) {
			replyOK(w, repo.SaveHSN(ctx, orgID, b.Match, b.HSN))
		}
	case n == 1 && seg[0] == "tds" && m == http.MethodPost:
		var e TDSEntry
		if decode(w, r, &e) {
			reply(w)(repo.RecordTDS(ctx, orgID, userID, e))
		}
	case n == 3 && seg[0] == "tds" && seg[2] == "settle" && m == http.MethodPost:
		var b struct {
			Date      string `json:"date"`
			AccountID string `json:"account_id"`
			Challan   string `json:"challan"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.SettleTDS(ctx, orgID, userID, seg[1], orDefault(b.Date, today()), b.AccountID, b.Challan))
		}
	case n == 2 && seg[0] == "gstr2b" && seg[1] == "import" && m == http.MethodPost:
		var b struct {
			Period string `json:"period"`
			CSV    string `json:"csv"`
		}
		if decode(w, r, &b) {
			c, err := repo.ImportGSTR2B(ctx, orgID, userID, b.Period, b.CSV)
			reply(w)(map[string]int{"imported": c}, err)
		}

	// ---- migration into Adrient, OFX ----
	case n == 2 && seg[0] == "import" && seg[1] == "columns" && m == http.MethodGet:
		writeJSON(w, http.StatusOK, ImportColumnHelp())
	case n == 2 && seg[0] == "import" && (seg[1] == "preview" || seg[1] == "run") && m == http.MethodPost:
		var req ImportRequest
		if decode(w, r, &req) {
			req.DryRun = seg[1] == "preview"
			reply(w)(repo.RunImport(ctx, orgID, userID, req))
		}
	case n == 2 && seg[0] == "bank" && seg[1] == "import-ofx" && m == http.MethodPost:
		var b struct {
			AccountID string `json:"account_id"`
			Data      string `json:"data"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.ImportOFX(ctx, orgID, userID, b.AccountID, b.Data))
		}

	// ---- controls ----
	case n == 1 && seg[0] == "close-checklist" && m == http.MethodGet:
		reply(w)(repo.CloseChecklist(ctx, orgID, orDefault(q.Get("date"), today())))
	case n == 1 && seg[0] == "close-period" && m == http.MethodPost:
		if !authz.Can(ctx, PermApprove) {
			http.Error(w, "forbidden: closing a period needs the accounting approver role", http.StatusForbidden)
			return true
		}
		var b struct {
			Date        string `json:"date"`
			Acknowledge bool   `json:"acknowledge"`
		}
		if decode(w, r, &b) {
			c, err := repo.ClosePeriod(ctx, orgID, userID, b.Date, b.Acknowledge)
			if err != nil {
				httpErr(w, err)
				return true
			}
			writeJSON(w, http.StatusOK, c)
		}
	case n == 3 && seg[0] == "journals" && seg[2] == "files" && m == http.MethodPost:
		var f Attachment
		if decode(w, r, &f) {
			l, err := repo.AddJournalFile(ctx, orgID, userID, seg[1], f)
			replyKey(w, "attachments", l, err)
		}
	case n == 4 && seg[0] == "journals" && seg[2] == "files" && m == http.MethodDelete:
		l, err := repo.RemoveJournalFile(ctx, orgID, userID, seg[1], seg[3])
		replyKey(w, "attachments", l, err)

	// ---- fixed assets ----
	case n == 1 && seg[0] == "assets" && m == http.MethodGet:
		l, err := repo.ListAssets(ctx, orgID)
		replyKey(w, "assets", l, err)
	case n == 1 && seg[0] == "assets" && m == http.MethodPost:
		var a Asset
		if decode(w, r, &a) {
			reply(w)(repo.CreateAsset(ctx, orgID, userID, a))
		}
	case n == 2 && seg[0] == "assets" && seg[1] == "depreciate" && m == http.MethodPost:
		var b struct {
			Period string `json:"period"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.RunDepreciation(ctx, orgID, userID, orDefault(b.Period, today()[:7])))
		}
	case n == 3 && seg[0] == "assets" && seg[2] == "schedule" && m == http.MethodGet:
		l, err := repo.AssetSchedule(ctx, orgID, seg[1])
		replyKey(w, "schedule", l, err)
	case n == 3 && seg[0] == "assets" && seg[2] == "dispose" && m == http.MethodPost:
		var b struct {
			Date      string  `json:"date"`
			Proceeds  float64 `json:"proceeds"`
			AccountID string  `json:"account_id"`
		}
		if decode(w, r, &b) {
			reply(w)(repo.DisposeAsset(ctx, orgID, userID, seg[1], orDefault(b.Date, today()), b.Proceeds, b.AccountID))
		}
	default:
		return false
	}
	return true
}
