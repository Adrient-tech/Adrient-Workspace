package accounting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Receipt references a Drive file (photo or PDF); no OCR.
type Receipt struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Expense is a purchase, bill or out-of-pocket claim.
type Expense struct {
	ID           string    `json:"id"`
	Date         string    `json:"date"`
	VendorID     string    `json:"vendor_id"`
	VendorName   string    `json:"vendor_name"`
	CategoryID   string    `json:"category_id"`
	CategoryName string    `json:"category_name"`
	Amount       float64   `json:"amount"`   // before GST
	GSTRate      float64   `json:"gst_rate"` // percent
	GSTAmount    float64   `json:"gst_amount"`
	Total        float64   `json:"total"`
	PaidFromID   string    `json:"paid_from_id"`
	OutOfPocket  bool      `json:"out_of_pocket"`
	Paid         bool      `json:"paid"`
	Notes        string    `json:"notes"`
	Receipts     []Receipt `json:"receipts"`
	ReportID     string    `json:"report_id"`
	ReportTitle  string    `json:"report_title"`
	Posted       bool      `json:"posted"`
}

const expenseCols = `e.id::text, to_char(e.expense_date,'YYYY-MM-DD'), e.vendor_id, e.vendor_name, e.category_id::text, a.name, e.amount::float8,
	e.gst_rate::float8, e.gst_amount::float8, e.total::float8, coalesce(e.paid_from_id::text,''), e.out_of_pocket, e.paid, e.notes, e.receipts,
	coalesce(e.report_id::text,''), coalesce(r.title,''), e.journal_id IS NOT NULL`

const expenseFrom = ` FROM adrient.acct_expenses e JOIN adrient.acct_accounts a ON a.id=e.category_id
	LEFT JOIN adrient.acct_expense_reports r ON r.id=e.report_id`

func scanExpense(row pgx.Row) (Expense, error) {
	var e Expense
	var raw []byte
	err := row.Scan(&e.ID, &e.Date, &e.VendorID, &e.VendorName, &e.CategoryID, &e.CategoryName, &e.Amount, &e.GSTRate, &e.GSTAmount, &e.Total,
		&e.PaidFromID, &e.OutOfPocket, &e.Paid, &e.Notes, &raw, &e.ReportID, &e.ReportTitle, &e.Posted)
	e.Receipts = []Receipt{}
	_ = json.Unmarshal(raw, &e.Receipts)
	return e, err
}

func (r *Repository) GetExpense(ctx context.Context, orgID, id string) (Expense, error) {
	if !isUUID(id) {
		return Expense{}, ErrNotFound
	}
	e, err := scanExpense(r.pool.QueryRow(ctx, `SELECT `+expenseCols+expenseFrom+` WHERE e.org_id=$1 AND e.id=$2::uuid`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

func (r *Repository) ListExpenses(ctx context.Context, orgID, reportID, from, to, q string) ([]Expense, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+expenseCols+expenseFrom+` WHERE e.org_id=$1 AND ($2='' OR e.report_id=$2::uuid)
		AND ($3='' OR e.expense_date >= $3::date) AND ($4='' OR e.expense_date <= $4::date)
		AND ($5='' OR e.vendor_name ILIKE '%'||$5||'%' OR e.notes ILIKE '%'||$5||'%' OR a.name ILIKE '%'||$5||'%')
		ORDER BY e.expense_date DESC, e.created_at DESC LIMIT 1000`, orgID, reportID, from, to, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Expense{}
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ExpenseJournal builds the posting for an expense (pure, unit-tested).
func ExpenseJournal(keys map[string]string, e Expense, intrastate bool) JournalInput {
	amt, gst := paise(e.Amount), paise(e.GSTAmount)
	total := amt + gst
	memo := "Expense: " + orDefault(e.VendorName, e.CategoryName)
	in := JournalInput{Date: e.Date, Memo: memo, Kind: "expense", Source: "expense", SourceID: e.ID, ContactID: e.VendorID, ContactName: e.VendorName}
	in.Lines = []Line{{AccountID: e.CategoryID, Debit: rupees(amt), Memo: memo}}
	in.Lines = append(in.Lines, taxLines(keys, gst, intrastate, false, memo)...)
	credit := keys["ap"]
	switch {
	case e.OutOfPocket:
		credit = keys["emp_payable"]
		in.Kind = "reimbursement"
	case e.Paid && e.PaidFromID != "":
		credit = e.PaidFromID
	default:
		in.Kind = "bill"
	}
	in.Lines = append(in.Lines, Line{AccountID: credit, Credit: rupees(total), Memo: memo})
	in.SourceHash = hashOf(amt, gst, e.Date, e.CategoryID, credit, e.VendorID, e.VendorName)
	return in
}

func normalizeExpense(e *Expense) error {
	if _, err := validDate(e.Date); err != nil {
		return err
	}
	if !isUUID(e.CategoryID) {
		return invalid("choose an expense category")
	}
	if paise(e.Amount) <= 0 {
		return invalid("amount must be greater than zero")
	}
	if e.GSTRate < 0 || e.GSTRate > 100 {
		return invalid("GST rate must be between 0 and 100")
	}
	e.Amount = rupees(paise(e.Amount))
	e.GSTAmount = rupees(int64(float64(paise(e.Amount))*e.GSTRate/100 + 0.5))
	e.Total = rupees(paise(e.Amount) + paise(e.GSTAmount))
	e.VendorName = strings.TrimSpace(e.VendorName)
	if e.OutOfPocket {
		e.Paid, e.PaidFromID = false, ""
	} else if e.Paid && e.PaidFromID == "" {
		return invalid("choose the account the expense was paid from, or mark it unpaid (a bill)")
	}
	if !e.Paid {
		e.PaidFromID = ""
	}
	if e.Receipts == nil {
		e.Receipts = []Receipt{}
	}
	return nil
}

// SaveExpense creates or updates an expense and (re)posts its journal unless it
// belongs to a report that has not been approved yet.
func (r *Repository) SaveExpense(ctx context.Context, orgID, userID, id string, e Expense) (Expense, error) {
	if err := normalizeExpense(&e); err != nil {
		return Expense{}, err
	}
	if e.OutOfPocket && e.ReportID == "" {
		return Expense{}, invalid("an out-of-pocket expense must be added to an expense report")
	}
	if e.ReportID != "" {
		var st string
		if err := r.pool.QueryRow(ctx, `SELECT status FROM adrient.acct_expense_reports WHERE org_id=$1 AND id=$2::uuid`, orgID, e.ReportID).Scan(&st); err != nil {
			return Expense{}, invalid("expense report not found")
		}
		if st != "draft" && st != "rejected" {
			return Expense{}, invalid("a %s report cannot be changed", st)
		}
	}
	rc, _ := json.Marshal(e.Receipts)
	created := id == ""
	if id == "" {
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_expenses (org_id, expense_date, vendor_id, vendor_name, category_id, amount, gst_rate, gst_amount, total,
			paid_from_id, out_of_pocket, paid, notes, receipts, report_id, created_by)
			VALUES ($1,$2::date,$3,$4,$5::uuid,$6,$7,$8,$9,NULLIF($10,'')::uuid,$11,$12,$13,$14,NULLIF($15,'')::uuid,NULLIF($16,'')::uuid) RETURNING id::text`,
			orgID, e.Date, e.VendorID, e.VendorName, e.CategoryID, e.Amount, e.GSTRate, e.GSTAmount, e.Total, e.PaidFromID, e.OutOfPocket, e.Paid,
			e.Notes, rc, e.ReportID, userID).Scan(&id)
		if err != nil {
			return Expense{}, err
		}
	} else {
		cur, err := r.GetExpense(ctx, orgID, id)
		if err != nil {
			return Expense{}, err
		}
		if cur.ReportID != "" && cur.ReportID != e.ReportID {
			return Expense{}, invalid("move an expense between reports by creating it again")
		}
		tag, err := r.pool.Exec(ctx, `UPDATE adrient.acct_expenses SET expense_date=$3::date, vendor_id=$4, vendor_name=$5, category_id=$6::uuid, amount=$7, gst_rate=$8,
			gst_amount=$9, total=$10, paid_from_id=NULLIF($11,'')::uuid, out_of_pocket=$12, paid=$13, notes=$14, receipts=$15 WHERE org_id=$1 AND id=$2::uuid`,
			orgID, id, e.Date, e.VendorID, e.VendorName, e.CategoryID, e.Amount, e.GSTRate, e.GSTAmount, e.Total, e.PaidFromID, e.OutOfPocket, e.Paid, e.Notes, rc)
		if err != nil {
			return Expense{}, err
		}
		if tag.RowsAffected() == 0 {
			return Expense{}, ErrNotFound
		}
	}
	saved, err := r.GetExpense(ctx, orgID, id)
	if err != nil {
		return saved, err
	}
	if saved.ReportID == "" {
		if err := r.postExpense(ctx, orgID, userID, saved); err != nil {
			if created {
				_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.acct_expenses WHERE org_id=$1 AND id=$2::uuid AND journal_id IS NULL`, orgID, id)
			}
			return Expense{}, err
		}
	}
	r.audit(ctx, orgID, userID, "save", "expense", id, fmt.Sprintf("%s %.2f", saved.VendorName, saved.Total))
	return r.GetExpense(ctx, orgID, id)
}

func (r *Repository) postExpense(ctx context.Context, orgID, userID string, e Expense) error {
	var paidViaBill bool
	_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.acct_journals WHERE org_id=$1 AND source='expense_payment' AND source_id=$2 AND NOT voided)`, orgID, e.ID).Scan(&paidViaBill)
	if paidViaBill { // a separate payment journal settles the payable
		e.Paid, e.PaidFromID = false, ""
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return err
	}
	keys, err := r.accountKeys(ctx, r.pool, orgID)
	if err != nil {
		return err
	}
	j, _, err := r.PostJournal(ctx, orgID, userID, ExpenseJournal(keys, e, st.Intrastate))
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `UPDATE adrient.acct_expenses SET journal_id=$3::uuid WHERE org_id=$1 AND id=$2::uuid`, orgID, e.ID, j.ID)
	return err
}

func (r *Repository) DeleteExpense(ctx context.Context, orgID, userID, id string) error {
	e, err := r.GetExpense(ctx, orgID, id)
	if err != nil {
		return err
	}
	if e.ReportID != "" {
		var st string
		_ = r.pool.QueryRow(ctx, `SELECT status FROM adrient.acct_expense_reports WHERE id=$1::uuid`, e.ReportID).Scan(&st)
		if st != "draft" && st != "rejected" {
			return invalid("a %s report cannot be changed", st)
		}
	}
	if err := r.deleteBySource(ctx, orgID, userID, "expense", id); err != nil {
		return err
	}
	_ = r.deleteBySource(ctx, orgID, userID, "expense_payment", id)
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.acct_expenses WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	if err == nil {
		r.audit(ctx, orgID, userID, "delete", "expense", id, e.VendorName)
	}
	return err
}

// PayBill settles an unpaid bill: Dr Accounts Payable, Cr the paying account.
func (r *Repository) PayBill(ctx context.Context, orgID, userID, id, fromID, date string) (Expense, error) {
	e, err := r.GetExpense(ctx, orgID, id)
	if err != nil {
		return e, err
	}
	if e.Paid || e.OutOfPocket || !e.Posted {
		return e, invalid("only an unpaid bill can be paid")
	}
	if _, err := r.bankAccount(ctx, orgID, fromID); err != nil {
		return e, err
	}
	if date, err = validDate(date); err != nil {
		return e, err
	}
	keys, err := r.accountKeys(ctx, r.pool, orgID)
	if err != nil {
		return e, err
	}
	memo := "Bill payment: " + orDefault(e.VendorName, e.CategoryName)
	t := paise(e.Total)
	_, _, err = r.PostJournal(ctx, orgID, userID, JournalInput{Date: date, Memo: memo, Kind: "payment", Source: "expense_payment", SourceID: id,
		SourceHash: hashOf(t, fromID, date), ContactID: e.VendorID, ContactName: e.VendorName,
		Lines: []Line{{AccountID: keys["ap"], Debit: rupees(t), Memo: memo}, {AccountID: fromID, Credit: rupees(t), Memo: memo}}})
	if err != nil {
		return e, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_expenses SET paid=true, paid_from_id=$3::uuid WHERE org_id=$1 AND id=$2::uuid`, orgID, id, fromID); err != nil {
		return e, err
	}
	return r.GetExpense(ctx, orgID, id)
}

// ---- expense reports ----

type ExpenseReport struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	Status        string  `json:"status"`
	SubmitterID   string  `json:"submitter_id"`
	SubmitterName string  `json:"submitter_name"`
	Notes         string  `json:"notes"`
	DecisionNote  string  `json:"decision_note"`
	ReimbursedAt  string  `json:"reimbursed_at"`
	CreatedAt     string  `json:"created_at"`
	Count         int     `json:"count"`
	Total         float64 `json:"total"`
}

const reportCols = `r.id::text, r.title, r.status, coalesce(r.submitter_id::text,''), r.submitter_name, r.notes, r.decision_note,
	coalesce(to_char(r.reimbursed_at,'YYYY-MM-DD'),''), to_char(r.created_at,'YYYY-MM-DD'),
	(SELECT count(*) FROM adrient.acct_expenses e WHERE e.report_id=r.id), (SELECT coalesce(sum(e.total),0)::float8 FROM adrient.acct_expenses e WHERE e.report_id=r.id)`

func (r *Repository) ListReports(ctx context.Context, orgID string) ([]ExpenseReport, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+reportCols+` FROM adrient.acct_expense_reports r WHERE r.org_id=$1 ORDER BY r.created_at DESC LIMIT 500`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExpenseReport{}
	for rows.Next() {
		var x ExpenseReport
		if err := rows.Scan(&x.ID, &x.Title, &x.Status, &x.SubmitterID, &x.SubmitterName, &x.Notes, &x.DecisionNote, &x.ReimbursedAt, &x.CreatedAt, &x.Count, &x.Total); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) GetReport(ctx context.Context, orgID, id string) (ExpenseReport, error) {
	var x ExpenseReport
	if !isUUID(id) {
		return x, ErrNotFound
	}
	err := r.pool.QueryRow(ctx, `SELECT `+reportCols+` FROM adrient.acct_expense_reports r WHERE r.org_id=$1 AND r.id=$2::uuid`, orgID, id).
		Scan(&x.ID, &x.Title, &x.Status, &x.SubmitterID, &x.SubmitterName, &x.Notes, &x.DecisionNote, &x.ReimbursedAt, &x.CreatedAt, &x.Count, &x.Total)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, ErrNotFound
	}
	return x, err
}

func (r *Repository) SaveReport(ctx context.Context, orgID, userID, userName, id string, x ExpenseReport) (ExpenseReport, error) {
	x.Title = strings.TrimSpace(x.Title)
	if x.Title == "" {
		return x, invalid("a report needs a title")
	}
	if id == "" {
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_expense_reports (org_id, title, notes, submitter_id, submitter_name)
			VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5) RETURNING id::text`, orgID, x.Title, x.Notes, userID, userName).Scan(&id)
		if err != nil {
			return x, err
		}
	} else {
		cur, err := r.GetReport(ctx, orgID, id)
		if err != nil {
			return x, err
		}
		if cur.Status != "draft" && cur.Status != "rejected" {
			return x, invalid("a %s report cannot be changed", cur.Status)
		}
		if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_expense_reports SET title=$3, notes=$4, updated_at=now() WHERE org_id=$1 AND id=$2::uuid`, orgID, id, x.Title, x.Notes); err != nil {
			return x, err
		}
	}
	r.audit(ctx, orgID, userID, "save", "expense_report", id, x.Title)
	return r.GetReport(ctx, orgID, id)
}

func (r *Repository) DeleteReport(ctx context.Context, orgID, userID, id string) error {
	cur, err := r.GetReport(ctx, orgID, id)
	if err != nil {
		return err
	}
	if cur.Status != "draft" && cur.Status != "rejected" {
		return invalid("a %s report cannot be deleted", cur.Status)
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM adrient.acct_expenses WHERE org_id=$1 AND report_id=$2::uuid`, orgID, id); err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.acct_expense_reports WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	if err == nil {
		r.audit(ctx, orgID, userID, "delete", "expense_report", id, cur.Title)
	}
	return err
}

// ReportAction moves a report through submit -> approve|reject -> reimburse.
// Approving posts the report's expenses (credit: Employee Reimbursements Payable);
// reimbursing pays that liability from a bank or cash account.
func (r *Repository) ReportAction(ctx context.Context, orgID, userID, id, action, note, fromID, date string) (ExpenseReport, error) {
	cur, err := r.GetReport(ctx, orgID, id)
	if err != nil {
		return cur, err
	}
	setStatus := func(st string, extra string, args ...any) error {
		q := `UPDATE adrient.acct_expense_reports SET status=$3, updated_at=now()` + extra + ` WHERE org_id=$1 AND id=$2::uuid`
		_, err := r.pool.Exec(ctx, q, append([]any{orgID, id, st}, args...)...)
		return err
	}
	switch action {
	case "submit":
		if cur.Status != "draft" && cur.Status != "rejected" {
			return cur, invalid("only a draft report can be submitted")
		}
		if cur.Count == 0 {
			return cur, invalid("add at least one expense before submitting")
		}
		err = setStatus("submitted", ", decision_note=''")
	case "reject":
		if cur.Status != "submitted" {
			return cur, invalid("only a submitted report can be rejected")
		}
		err = setStatus("rejected", ", decision_note=$4", note)
	case "approve":
		if cur.Status != "submitted" {
			return cur, invalid("only a submitted report can be approved")
		}
		exps, e := r.ListExpenses(ctx, orgID, id, "", "", "")
		if e != nil {
			return cur, e
		}
		for _, x := range exps {
			if e := r.postExpense(ctx, orgID, userID, x); e != nil {
				return cur, e
			}
		}
		err = setStatus("approved", ", decision_note=$4", note)
	case "reimburse":
		if cur.Status != "approved" {
			return cur, invalid("only an approved report can be reimbursed")
		}
		if _, e := r.bankAccount(ctx, orgID, fromID); e != nil {
			return cur, e
		}
		if date, err = validDate(date); err != nil {
			return cur, err
		}
		keys, e := r.accountKeys(ctx, r.pool, orgID)
		if e != nil {
			return cur, e
		}
		var oop float64
		_ = r.pool.QueryRow(ctx, `SELECT coalesce(sum(total),0)::float8 FROM adrient.acct_expenses WHERE org_id=$1 AND report_id=$2::uuid AND out_of_pocket`, orgID, id).Scan(&oop)
		if t := paise(oop); t > 0 {
			memo := "Reimbursement: " + cur.Title
			if _, _, e := r.PostJournal(ctx, orgID, userID, JournalInput{Date: date, Memo: memo, Kind: "reimbursement", Source: "report", SourceID: id,
				SourceHash: hashOf(t, fromID, date), ContactName: cur.SubmitterName,
				Lines: []Line{{AccountID: keys["emp_payable"], Debit: rupees(t), Memo: memo}, {AccountID: fromID, Credit: rupees(t), Memo: memo}}}); e != nil {
				return cur, e
			}
		}
		err = setStatus("reimbursed", ", reimbursed_at=$4::date", date)
	default:
		return cur, invalid("unknown action %q", action)
	}
	if err != nil {
		return cur, err
	}
	r.audit(ctx, orgID, userID, action, "expense_report", id, cur.Title)
	return r.GetReport(ctx, orgID, id)
}
