package accounting

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// PayrollPosting is one locked HR payroll run, ready to post as a single
// balanced journal (source "payroll", source id = run id, so posting twice
// never double counts).
type PayrollPosting struct {
	RunID, Period, Date                          string
	Gross, EmployerContrib, Reimburse            float64
	PFPayable, ESIPayable, PTPayable, TDSPayable float64
	OtherDed, NetSalaryDue                       float64
	Employees                                    int
}

// ExpenseClaimRow is an approved expense report awaiting reimbursement.
type ExpenseClaimRow struct {
	ID     string
	Title  string
	Amount float64
}

type payrollAccount struct{ key, code, name, typ, subtype string }

var payrollAccounts = []payrollAccount{
	{"salaries", "6010", "Salaries & Wages", "expense", "opex"},
	{"hr_employer_contrib", "6015", "Employer PF & ESI Contribution", "expense", "opex"},
	{"hr_pf_payable", "2410", "PF Payable", "liability", "other"},
	{"hr_esi_payable", "2420", "ESI Payable", "liability", "other"},
	{"hr_pt_payable", "2430", "Professional Tax Payable", "liability", "other"},
	{"tds_payable", "2400", "TDS Payable", "liability", "other"},
	{"hr_other_deductions", "2440", "Other Payroll Deductions Payable", "liability", "other"},
	{"hr_salary_payable", "2450", "Salary Payable", "liability", "other"},
}

// ensurePayrollAccounts makes sure the payroll accounts exist and carry their
// system keys, adopting the starter chart's accounts by code when present.
func (r *Repository) ensurePayrollAccounts(ctx context.Context, orgID string) (map[string]string, error) {
	if err := r.EnsureSeeded(ctx, orgID); err != nil {
		return nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "acct-payroll-accounts:"+orgID); err != nil {
		return nil, err
	}
	for _, a := range payrollAccounts {
		var id string
		err := tx.QueryRow(ctx, `SELECT id::text FROM adrient.acct_accounts WHERE org_id=$1 AND system_key=$2`, orgID, a.key).Scan(&id)
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		tag, err := tx.Exec(ctx, `UPDATE adrient.acct_accounts SET system_key=$3 WHERE id=(SELECT id FROM adrient.acct_accounts WHERE org_id=$1 AND code=$2 AND system_key='' AND type=$4 ORDER BY created_at LIMIT 1)`,
			orgID, a.code, a.key, a.typ)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO adrient.acct_accounts (org_id, code, name, type, subtype, system_key) VALUES ($1,$2,$3,$4,$5,$6)`,
				orgID, a.code, a.name, a.typ, a.subtype, a.key); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.accountKeys(ctx, r.pool, orgID)
}

// PostPayroll posts the run's journal and returns its id. Debits: salary
// expense, employer contributions, and employee reimbursements settled through
// payroll. Credits: PF, ESI, PT, TDS and other payables and the net salary due.
func (r *Repository) PostPayroll(ctx context.Context, orgID, userID string, p PayrollPosting) (string, error) {
	keys, err := r.ensurePayrollAccounts(ctx, orgID)
	if err != nil {
		return "", err
	}
	memo := "Payroll " + p.Period
	dr := func(key string, amt float64) Line { return Line{AccountID: keys[key], Debit: amt, Memo: memo} }
	cr := func(key string, amt float64) Line { return Line{AccountID: keys[key], Credit: amt, Memo: memo} }
	lines := []Line{
		dr("salaries", p.Gross), dr("hr_employer_contrib", p.EmployerContrib), dr("emp_payable", p.Reimburse),
		cr("hr_pf_payable", p.PFPayable), cr("hr_esi_payable", p.ESIPayable), cr("hr_pt_payable", p.PTPayable),
		cr("tds_payable", p.TDSPayable), cr("hr_other_deductions", p.OtherDed), cr("hr_salary_payable", p.NetSalaryDue),
	}
	kept := lines[:0]
	for _, l := range lines {
		if paise(l.Debit) != 0 || paise(l.Credit) != 0 {
			kept = append(kept, l)
		}
	}
	if len(kept) == 0 {
		return "", invalid("the payroll run has nothing to post")
	}
	in := JournalInput{Date: p.Date, Memo: memo + fmt.Sprintf(" (%d employees)", p.Employees), Ref: "PAYROLL-" + p.Period, Kind: "journal",
		Source: "payroll", SourceID: p.RunID, Lines: kept,
		SourceHash: hashOf(p.Gross, p.EmployerContrib, p.Reimburse, p.PFPayable, p.ESIPayable, p.PTPayable, p.TDSPayable, p.OtherDed, p.NetSalaryDue, p.Date)}
	j, _, err := r.PostJournal(ctx, orgID, userID, in)
	if err != nil {
		return "", err
	}
	return j.ID, nil
}

// ApprovedClaimsFor lists approved expense reports of a user with out-of-pocket
// amounts still to be repaid.
func (r *Repository) ApprovedClaimsFor(ctx context.Context, orgID, submitterUserID string) ([]ExpenseClaimRow, error) {
	if !isUUID(submitterUserID) {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT r.id::text, r.title, sum(e.total)::float8 FROM adrient.acct_expense_reports r JOIN adrient.acct_expenses e ON e.report_id=r.id
		WHERE r.org_id=$1 AND r.submitter_id=$2::uuid AND r.status='approved' AND e.out_of_pocket GROUP BY r.id, r.title HAVING sum(e.total) > 0 ORDER BY r.created_at`, orgID, submitterUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpenseClaimRow
	for rows.Next() {
		var c ExpenseClaimRow
		if err := rows.Scan(&c.ID, &c.Title, &c.Amount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MarkReportsReimbursedByPayroll flags expense reports as repaid because their
// amount was settled through a payroll run (the payroll journal debits
// Employee Reimbursements Payable, so the report must not be paid again).
func (r *Repository) MarkReportsReimbursedByPayroll(ctx context.Context, orgID, userID string, ids []string, date string) error {
	date, err := validDate(date)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !isUUID(id) {
			continue
		}
		tag, err := r.pool.Exec(ctx, `UPDATE adrient.acct_expense_reports SET status='reimbursed', reimbursed_at=$3::date, updated_at=now() WHERE org_id=$1 AND id=$2::uuid AND status='approved'`, orgID, id, date)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			r.audit(ctx, orgID, userID, "reimburse", "expense_report", id, "settled through payroll")
		}
	}
	return nil
}
