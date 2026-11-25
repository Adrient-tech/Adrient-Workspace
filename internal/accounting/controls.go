package accounting

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ---- journal attachments (files live in Drive) ----

// AddJournalFile links a Drive file to a journal. Allowed on closed periods too (it is
// supporting evidence, not a change to the books) but it is audited.
func (r *Repository) AddJournalFile(ctx context.Context, orgID, userID, journalID string, f Attachment) ([]Attachment, error) {
	if _, err := r.GetJournal(ctx, orgID, journalID); err != nil {
		return nil, err
	}
	f.ID, f.Name = strings.TrimSpace(f.ID), strings.TrimSpace(f.Name)
	if f.ID == "" || len(f.ID) > 100 {
		return nil, invalid("file id is required")
	}
	var n int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.acct_journal_files WHERE journal_id=$1::uuid`, journalID).Scan(&n)
	if n >= 20 {
		return nil, invalid("a journal can have at most 20 attachments")
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.acct_journal_files (org_id, journal_id, file_id, name, added_by) VALUES ($1,$2::uuid,$3,$4,$5)
		ON CONFLICT (journal_id, file_id) DO NOTHING`, orgID, journalID, f.ID, f.Name, userID); err != nil {
		return nil, err
	}
	r.audit(ctx, orgID, userID, "attach", "journal", journalID, f.Name)
	return r.ListJournalFiles(ctx, orgID, journalID)
}

func (r *Repository) RemoveJournalFile(ctx context.Context, orgID, userID, journalID, fileID string) ([]Attachment, error) {
	if _, err := r.GetJournal(ctx, orgID, journalID); err != nil {
		return nil, err
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM adrient.acct_journal_files WHERE org_id=$1 AND journal_id=$2::uuid AND file_id=$3`, orgID, journalID, fileID); err != nil {
		return nil, err
	}
	r.audit(ctx, orgID, userID, "detach", "journal", journalID, fileID)
	return r.ListJournalFiles(ctx, orgID, journalID)
}

func (r *Repository) ListJournalFiles(ctx context.Context, orgID, journalID string) ([]Attachment, error) {
	rows, err := r.pool.Query(ctx, `SELECT file_id, name FROM adrient.acct_journal_files WHERE org_id=$1 AND journal_id=$2::uuid ORDER BY added_at`, orgID, journalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attachment{}
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- period close checklist ----

// CheckItem is one line of the close checklist. Status: ok, warn (review) or fail (blocks closing).
type CheckItem struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Count  int    `json:"count"`
	Detail string `json:"detail"`
}

// Checklist is the close checklist for a period end.
type Checklist struct {
	Date     string      `json:"date"`
	Items    []CheckItem `json:"items"`
	CanClose bool        `json:"can_close"`
	Closed   string      `json:"closed_through"`
}

// CloseChecklist runs the checks for closing the books through date.
func (r *Repository) CloseChecklist(ctx context.Context, orgID, date string) (Checklist, error) {
	d, err := validDate(date)
	if err != nil {
		return Checklist{}, err
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return Checklist{}, err
	}
	out := Checklist{Date: d, Items: []CheckItem{}, Closed: st.ClosingDate}
	add := func(id, label, status string, n int, detail string) {
		out.Items = append(out.Items, CheckItem{ID: id, Label: label, Status: status, Count: n, Detail: detail})
	}
	count := func(q string, args ...any) int {
		var n int
		_ = r.pool.QueryRow(ctx, q, args...).Scan(&n)
		return n
	}
	flag := func(n int, bad string) string {
		if n > 0 {
			return bad
		}
		return "ok"
	}

	accts, _ := r.ListAccounts(ctx, orgID)
	ps, _ := r.LoadPostings(ctx, orgID, d)
	tb := BuildTrialBalance(accts, ps, d)
	bal := "ok"
	detail := "Debits equal credits"
	if !tb.Balanced {
		bal, detail = "fail", fmt.Sprintf("Debits %.2f and credits %.2f differ", tb.TotalDebit, tb.TotalCredit)
	}
	add("trial_balance", "Trial balance is in balance", bal, 0, detail)

	n := count(`SELECT count(*) FROM adrient.acct_bank_lines WHERE org_id=$1 AND status='pending' AND line_date <= $2::date`, orgID, d)
	add("bank_lines", "Bank statement lines reviewed", flag(n, "fail"), n, fmt.Sprintf("%d imported lines are still waiting for review", n))

	n = count(`SELECT count(*) FROM adrient.acct_accounts a WHERE a.org_id=$1 AND a.subtype='bank' AND a.active
		AND EXISTS (SELECT 1 FROM adrient.acct_bank_lines b WHERE b.org_id=$1 AND b.account_id=a.id)
		AND NOT EXISTS (SELECT 1 FROM adrient.acct_reconciliations x WHERE x.org_id=$1 AND x.account_id=a.id AND x.statement_date >= $2::date - 31)`, orgID, d)
	add("reconciled", "Bank accounts reconciled this month", flag(n, "warn"), n, fmt.Sprintf("%d bank accounts have no reconciliation in the last month", n))

	n = count(`SELECT count(*) FROM adrient.acct_bills WHERE org_id=$1 AND status='pending_approval' AND bill_date <= $2::date`, orgID, d)
	add("bill_approvals", "Bills waiting for approval", flag(n, "fail"), n, fmt.Sprintf("%d bills over the approval limit are unapproved and not posted", n))

	n = count(`SELECT count(*) FROM adrient.acct_bills WHERE org_id=$1 AND status='draft' AND bill_date <= $2::date`, orgID, d)
	add("draft_bills", "Draft bills", flag(n, "warn"), n, fmt.Sprintf("%d draft bills are not in the books", n))

	n = count(`SELECT count(*) FROM adrient.acct_recurring_runs WHERE org_id=$1 AND status='failed' AND run_date <= $2::date`, orgID, d)
	add("recurring", "Recurring transactions created", flag(n, "fail"), n, fmt.Sprintf("%d scheduled entries failed to post", n))

	n = count(`SELECT count(*) FROM adrient.acct_assets a WHERE a.org_id=$1 AND a.status='active' AND a.dep_start <= $2::date
		AND NOT EXISTS (SELECT 1 FROM adrient.acct_asset_dep x WHERE x.asset_id=a.id AND x.period = to_char($2::date,'YYYY-MM'))
		AND (SELECT count(*) FROM adrient.acct_asset_dep y WHERE y.asset_id=a.id) < a.life_months`, orgID, d)
	add("depreciation", "Depreciation posted for the month", flag(n, "warn"), n, fmt.Sprintf("%d assets have no depreciation for %s", n, d[:7]))

	n = count(`SELECT count(DISTINCT currency) FROM adrient.acct_journals WHERE org_id=$1 AND currency<>'' AND NOT voided AND entry_date <= $2::date
		AND NOT EXISTS (SELECT 1 FROM adrient.acct_journals v WHERE v.org_id=$1 AND v.source='revaluation' AND NOT v.voided AND v.entry_date=$2::date)`, orgID, d)
	add("revaluation", "Foreign currency balances revalued", flag(n, "warn"), n, fmt.Sprintf("%d foreign currencies in use and no revaluation entry on %s", n, d))

	n = count(`SELECT count(*) FROM adrient.acct_tds_entries WHERE org_id=$1 AND status='pending' AND direction='payable' AND entry_date <= $2::date`, orgID, d)
	add("tds", "TDS deducted has been deposited", flag(n, "warn"), n, fmt.Sprintf("%d TDS payable entries not yet deposited", n))

	out.CanClose = true
	for _, it := range out.Items {
		if it.Status == "fail" {
			out.CanClose = false
		}
	}
	if st.ClosingDate != "" && d <= st.ClosingDate {
		out.Items = append(out.Items, CheckItem{ID: "already", Label: "Period is already closed", Status: "warn", Detail: "Books are closed through " + st.ClosingDate})
	}
	return out, nil
}

// ClosePeriod sets the closing date after the checklist has no failures. A date at or
// before the current closing date is rejected (use Settings to reopen). warn items are
// allowed but must be acknowledged with acknowledge=true.
func (r *Repository) ClosePeriod(ctx context.Context, orgID, userID, date string, acknowledge bool) (Checklist, error) {
	c, err := r.CloseChecklist(ctx, orgID, date)
	if err != nil {
		return c, err
	}
	if !c.CanClose {
		return c, invalid("fix the failed checks before closing the period")
	}
	for _, it := range c.Items {
		if it.Status == "warn" && it.ID != "already" && !acknowledge {
			return c, invalid("review the warnings and confirm to close anyway")
		}
	}
	if c.Closed != "" && c.Date <= c.Closed {
		return c, invalid("the books are already closed through %s", c.Closed)
	}
	if t, _ := time.Parse("2006-01-02", c.Date); t.After(time.Now().AddDate(0, 0, 1)) {
		return c, invalid("cannot close a period in the future")
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return c, err
	}
	st.ClosingDate = c.Date
	if _, err := r.SaveSettings(ctx, orgID, userID, st); err != nil {
		return c, err
	}
	c.Closed = c.Date
	return c, nil
}
