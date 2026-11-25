package hr

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- salary structures ----

// Structure is a reusable salary template.
type Structure struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Lines []SalaryLine `json:"lines"`
}

// ListStructures lists templates (HR only).
func (r *Repository) ListStructures(ctx context.Context, a Actor) ([]Structure, error) {
	if err := a.requireHR(); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, lines FROM adrient.hr_salary_structures WHERE org_id=$1 ORDER BY name`, a.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Structure{}
	for rows.Next() {
		var s Structure
		var l []byte
		if err := rows.Scan(&s.ID, &s.Name, &l); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(l, &s.Lines)
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveStructure creates or updates a template (HR only).
func (r *Repository) SaveStructure(ctx context.Context, a Actor, id string, s Structure) (Structure, error) {
	if err := a.requireHR(); err != nil {
		return s, err
	}
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return s, invalid("a structure name is required")
	}
	if err := ValidateLines(s.Lines, true); err != nil {
		return s, err
	}
	// Prove it resolves for a sample gross.
	if _, err := ExpandStructure(1200000, s.Lines); err != nil {
		return s, err
	}
	lj, _ := json.Marshal(s.Lines)
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_salary_structures (org_id, name, lines) VALUES ($1,$2,$3) RETURNING id::text`, a.OrgID, s.Name, lj).Scan(&s.ID)
	} else {
		if !isUUID(id) {
			return s, ErrNotFound
		}
		var n int64
		err = r.pool.QueryRow(ctx, `WITH u AS (UPDATE adrient.hr_salary_structures SET name=$3, lines=$4 WHERE org_id=$1 AND id=$2::uuid RETURNING 1) SELECT count(*) FROM u`, a.OrgID, id, s.Name, lj).Scan(&n)
		if err == nil && n == 0 {
			return s, ErrNotFound
		}
		s.ID = id
	}
	if err != nil && strings.Contains(err.Error(), "23505") {
		return s, conflict("a structure named %q exists", s.Name)
	}
	return s, err
}

// DeleteStructure removes a template; employee salaries keep their own copy.
func (r *Repository) DeleteStructure(ctx context.Context, a Actor, id string) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	if !isUUID(id) {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.hr_salary_structures WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id)
	return err
}

// PreviewStructure expands a template for an annual gross (HR only).
func (r *Repository) PreviewStructure(ctx context.Context, a Actor, lines []SalaryLine, annual float64) ([]SalaryLine, error) {
	if err := a.requireHR(); err != nil {
		return nil, err
	}
	return ExpandStructure(annual, lines)
}

// ---- employee salary (HR and the employee only) ----

// EmployeeSalary is a salary effective from a date.
type EmployeeSalary struct {
	ID            string       `json:"id"`
	EmployeeID    string       `json:"employee_id"`
	EffectiveFrom string       `json:"effective_from"`
	StructureID   string       `json:"structure_id"`
	AnnualGross   float64      `json:"annual_gross"`
	MonthlyGross  float64      `json:"monthly_gross"`
	Lines         []SalaryLine `json:"lines"`
	PFEnabled     bool         `json:"pf_enabled"`
	ESIEnabled    bool         `json:"esi_enabled"`
	PTEnabled     bool         `json:"pt_enabled"`
	TDSEnabled    bool         `json:"tds_enabled"`
}

func scanSalary(row pgx.Row) (EmployeeSalary, error) {
	var s EmployeeSalary
	var l []byte
	err := row.Scan(&s.ID, &s.EmployeeID, &s.EffectiveFrom, &s.StructureID, &s.AnnualGross, &l, &s.PFEnabled, &s.ESIEnabled, &s.PTEnabled, &s.TDSEnabled)
	_ = json.Unmarshal(l, &s.Lines)
	for _, x := range s.Lines {
		if x.Kind == "earning" {
			s.MonthlyGross += x.Amount
		}
	}
	s.MonthlyGross = round2(s.MonthlyGross)
	return s, err
}

const salCols = `id::text, employee_id::text, to_char(effective_from,'YYYY-MM-DD'), coalesce(structure_id::text,''), annual_gross::float8, lines, pf_enabled, esi_enabled, pt_enabled, tds_enabled`

// ListSalaries returns the salary history of an employee. Only HR and the
// employee themselves; a manager can never read it.
func (r *Repository) ListSalaries(ctx context.Context, a Actor, empID string) ([]EmployeeSalary, error) {
	if !a.ownsOrHR(empID) {
		return nil, forbidden("salary details are visible to HR and the employee only")
	}
	if !isUUID(empID) {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT `+salCols+` FROM adrient.hr_employee_salary WHERE org_id=$1 AND employee_id=$2::uuid ORDER BY effective_from DESC`, a.OrgID, empID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmployeeSalary{}
	for rows.Next() {
		s, err := scanSalary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveSalary sets a salary effective from a date (HR only). With a structure and
// annual gross the lines are expanded; explicit lines override.
func (r *Repository) SaveSalary(ctx context.Context, a Actor, empID string, in EmployeeSalary) (EmployeeSalary, error) {
	if err := a.requireHR(); err != nil {
		return in, err
	}
	if _, err := r.empLite(ctx, r.pool, a.OrgID, empID); err != nil {
		return in, err
	}
	var err error
	if in.EffectiveFrom, err = validDate(in.EffectiveFrom); err != nil {
		return in, err
	}
	if in.StructureID != "" {
		var n int
		if !isUUID(in.StructureID) || r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_salary_structures WHERE id=$1::uuid AND org_id=$2`, in.StructureID, a.OrgID).Scan(&n) != nil || n == 0 {
			return in, invalid("unknown salary structure")
		}
	}
	if len(in.Lines) == 0 {
		return in, invalid("salary components are required (preview and apply a structure first)")
	}
	if err := ValidateLines(in.Lines, false); err != nil {
		return in, err
	}
	var monthly float64
	for _, l := range in.Lines {
		if l.Kind == "earning" {
			monthly += l.Amount
		}
	}
	if monthly <= 0 {
		return in, invalid("the salary needs at least one earning")
	}
	in.AnnualGross = round2(monthly * 12)
	lj, _ := json.Marshal(in.Lines)
	var id string
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_employee_salary (org_id, employee_id, effective_from, structure_id, annual_gross, lines, pf_enabled, esi_enabled, pt_enabled, tds_enabled, created_by)
		VALUES ($1,$2::uuid,$3::date,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11::uuid)
		ON CONFLICT (employee_id, effective_from) DO UPDATE SET structure_id=EXCLUDED.structure_id, annual_gross=EXCLUDED.annual_gross, lines=EXCLUDED.lines,
			pf_enabled=EXCLUDED.pf_enabled, esi_enabled=EXCLUDED.esi_enabled, pt_enabled=EXCLUDED.pt_enabled, tds_enabled=EXCLUDED.tds_enabled RETURNING id::text`,
		a.OrgID, empID, in.EffectiveFrom, in.StructureID, in.AnnualGross, lj, in.PFEnabled, in.ESIEnabled, in.PTEnabled, in.TDSEnabled, a.UserID).Scan(&id)
	if err != nil {
		return in, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "set", "salary", empID, in.EffectiveFrom)
	return scanSalary(r.pool.QueryRow(ctx, `SELECT `+salCols+` FROM adrient.hr_employee_salary WHERE id=$1::uuid`, id))
}

func (r *Repository) salaryFor(ctx context.Context, orgID, empID string, asOf time.Time) (EmployeeSalary, error) {
	s, err := scanSalary(r.pool.QueryRow(ctx, `SELECT `+salCols+` FROM adrient.hr_employee_salary WHERE org_id=$1 AND employee_id=$2::uuid AND effective_from<=$3::date ORDER BY effective_from DESC LIMIT 1`,
		orgID, empID, fmtDate(asOf)))
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

// ---- professional tax slabs ----

// ListPTSlabs lists the editable state table (HR only).
func (r *Repository) ListPTSlabs(ctx context.Context, a Actor) ([]PTSlab, error) {
	if err := a.requireHR(); err != nil {
		return nil, err
	}
	return r.ptSlabs(ctx, a.OrgID)
}

func (r *Repository) ptSlabs(ctx context.Context, orgID string) ([]PTSlab, error) {
	rows, err := r.pool.Query(ctx, `SELECT state, min_salary::float8, max_salary::float8, amount::float8, month FROM adrient.hr_pt_slabs WHERE org_id=$1 ORDER BY state, month, min_salary`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PTSlab{}
	for rows.Next() {
		var s PTSlab
		if err := rows.Scan(&s.State, &s.Min, &s.Max, &s.Amount, &s.Month); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ReplacePTSlabs replaces the whole slab table (HR only). Replacing the full
// table keeps the editor simple and the table consistent.
func (r *Repository) ReplacePTSlabs(ctx context.Context, a Actor, slabs []PTSlab) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	if len(slabs) > 500 {
		return invalid("too many slabs")
	}
	for i := range slabs {
		s := &slabs[i]
		s.State = strings.TrimSpace(s.State)
		if s.State == "" || s.Min < 0 || s.Amount < 0 || s.Month < 0 || s.Month > 12 || (s.Max != nil && *s.Max < s.Min) {
			return invalid("slab %d: check the state, salary range, amount and month", i+1)
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.hr_pt_slabs WHERE org_id=$1`, a.OrgID); err != nil {
		return err
	}
	for _, s := range slabs {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.hr_pt_slabs (org_id, state, min_salary, max_salary, amount, month) VALUES ($1,$2,$3,$4,$5,$6)`,
			a.OrgID, s.State, s.Min, s.Max, s.Amount, s.Month); err != nil {
			return err
		}
	}
	r.audit(ctx, tx, a.OrgID, a.UserID, "replace", "pt_slabs", "", fmt.Sprintf("%d slabs", len(slabs)))
	return tx.Commit(ctx)
}

// ---- payroll runs ----

// Run is one month's payroll.
type Run struct {
	ID        string   `json:"id"`
	Period    string   `json:"period"`
	Status    string   `json:"status"`
	JournalID string   `json:"journal_id"`
	LockedAt  string   `json:"locked_at"`
	Employees int      `json:"employees"`
	Gross     float64  `json:"gross"`
	Net       float64  `json:"net_pay"`
	PF        float64  `json:"pf"`
	ESI       float64  `json:"esi"`
	PT        float64  `json:"prof_tax"`
	TDS       float64  `json:"tds"`
	Skipped   []string `json:"skipped,omitempty"`
}

const runCols = `r.id::text, r.period, r.status, r.journal_id, coalesce(to_char(r.locked_at,'YYYY-MM-DD'),''),
	(SELECT count(*) FROM adrient.hr_payslips p WHERE p.run_id=r.id),
	coalesce((SELECT sum(gross) FROM adrient.hr_payslips p WHERE p.run_id=r.id),0)::float8,
	coalesce((SELECT sum(net_pay) FROM adrient.hr_payslips p WHERE p.run_id=r.id),0)::float8,
	coalesce((SELECT sum(pf_employee+pf_employer) FROM adrient.hr_payslips p WHERE p.run_id=r.id),0)::float8,
	coalesce((SELECT sum(esi_employee+esi_employer) FROM adrient.hr_payslips p WHERE p.run_id=r.id),0)::float8,
	coalesce((SELECT sum(prof_tax) FROM adrient.hr_payslips p WHERE p.run_id=r.id),0)::float8,
	coalesce((SELECT sum(tds) FROM adrient.hr_payslips p WHERE p.run_id=r.id),0)::float8`

func scanRun(row pgx.Row) (Run, error) {
	var x Run
	err := row.Scan(&x.ID, &x.Period, &x.Status, &x.JournalID, &x.LockedAt, &x.Employees, &x.Gross, &x.Net, &x.PF, &x.ESI, &x.PT, &x.TDS)
	return x, err
}

// ListRuns lists payroll runs (HR only).
func (r *Repository) ListRuns(ctx context.Context, a Actor) ([]Run, error) {
	if err := a.requireHR(); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+runCols+` FROM adrient.hr_payroll_runs r WHERE r.org_id=$1 ORDER BY r.period DESC`, a.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		x, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// GetRun returns one run (HR only).
func (r *Repository) GetRun(ctx context.Context, a Actor, id string) (Run, error) {
	if err := a.requireHR(); err != nil {
		return Run{}, err
	}
	return r.getRun(ctx, a.OrgID, id)
}

func (r *Repository) getRun(ctx context.Context, orgID, id string) (Run, error) {
	if !isUUID(id) {
		return Run{}, ErrNotFound
	}
	x, err := scanRun(r.pool.QueryRow(ctx, `SELECT `+runCols+` FROM adrient.hr_payroll_runs r WHERE r.org_id=$1 AND r.id=$2::uuid`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, ErrNotFound
	}
	return x, err
}

// CreateRun opens a draft run for a month and computes every payslip (HR only).
func (r *Repository) CreateRun(ctx context.Context, a Actor, period string) (Run, error) {
	if err := a.requireHR(); err != nil {
		return Run{}, err
	}
	if _, _, err := monthRange(period); err != nil {
		return Run{}, err
	}
	var id string
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_payroll_runs (org_id, period, created_by) VALUES ($1,$2,$3::uuid) RETURNING id::text`, a.OrgID, period, a.UserID).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return Run{}, conflict("a payroll run for %s already exists", PeriodLabel(period))
		}
		return Run{}, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "create", "payroll_run", id, period)
	return r.recompute(ctx, a, id)
}

// Recompute rebuilds every payslip of a draft run, keeping HR adjustments.
func (r *Repository) Recompute(ctx context.Context, a Actor, id string) (Run, error) {
	if err := a.requireHR(); err != nil {
		return Run{}, err
	}
	return r.recompute(ctx, a, id)
}

func (r *Repository) recompute(ctx context.Context, a Actor, id string) (Run, error) {
	run, err := r.getRun(ctx, a.OrgID, id)
	if err != nil {
		return run, err
	}
	if run.Status != "draft" {
		return run, conflict("only a draft run can be recomputed; reopen it first")
	}
	from, to, _ := monthRange(run.Period)
	// Keep adjustments across a recompute.
	keep := map[string][]byte{}
	if rows, err := r.pool.Query(ctx, `SELECT employee_id::text, adjustments FROM adrient.hr_payslips WHERE run_id=$1::uuid`, id); err == nil {
		for rows.Next() {
			var e string
			var adj []byte
			if rows.Scan(&e, &adj) == nil {
				keep[e] = adj
			}
		}
		rows.Close()
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM adrient.hr_employees WHERE org_id=$1 AND join_date<=$3::date AND (exit_date IS NULL OR exit_date>=$2::date) ORDER BY emp_code`, a.OrgID, fmtDate(from), fmtDate(to))
	if err != nil {
		return run, err
	}
	var ids []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			rows.Close()
			return run, err
		}
		ids = append(ids, e)
	}
	rows.Close()
	if _, err := r.pool.Exec(ctx, `DELETE FROM adrient.hr_payslips WHERE run_id=$1::uuid`, id); err != nil {
		return run, err
	}
	var skipped []string
	for _, e := range ids {
		var ads []Adjustment
		_ = json.Unmarshal(keep[e], &ads)
		if err := r.buildPayslip(ctx, a.OrgID, run, e, ads); err != nil {
			if errors.Is(err, ErrNotFound) {
				skipped = append(skipped, e)
				continue
			}
			emp, _ := r.empLite(ctx, r.pool, a.OrgID, e)
			skipped = append(skipped, fmt.Sprintf("%s: %s", emp.Name, strings.TrimPrefix(err.Error(), "invalid: ")))
		}
	}
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.hr_payroll_runs SET updated_at=now() WHERE id=$1::uuid`, id)
	out, err := r.getRun(ctx, a.OrgID, id)
	// Name the employees that have no salary so HR can fix them.
	named := []string{}
	for _, s := range skipped {
		if isUUID(s) {
			emp, _ := r.empLite(ctx, r.pool, a.OrgID, s)
			named = append(named, emp.Name+": no salary set")
		} else {
			named = append(named, s)
		}
	}
	out.Skipped = named
	return out, err
}

// lopFor returns loss-of-pay days (unpaid leave and, when configured,
// attendance) and the days of the month outside the employment period.
func (r *Repository) lopFor(ctx context.Context, orgID string, e empLite, from, to time.Time, st Settings) (lop, outside float64, err error) {
	join := parseDate(e.JoinDate)
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if d.Before(join) {
			outside++
		}
		if e.ExitDate != "" && d.After(parseDate(e.ExitDate)) {
			outside++
		}
	}
	hol, err := r.holidaysFor(ctx, r.pool, orgID, e.LocationID, from, to)
	if err != nil {
		return 0, 0, err
	}
	wo := weeklyOffSet(st)
	rows, err := r.pool.Query(ctx, `SELECT to_char(q.from_date,'YYYY-MM-DD'), to_char(q.to_date,'YYYY-MM-DD'), q.half_day FROM adrient.hr_leave_requests q JOIN adrient.hr_leave_types t ON t.id=q.type_id
		WHERE q.employee_id=$1::uuid AND q.status='approved' AND NOT t.paid AND q.from_date<=$3::date AND q.to_date>=$2::date`, e.ID, fmtDate(from), fmtDate(to))
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var f, t, h string
		if err := rows.Scan(&f, &t, &h); err != nil {
			return 0, 0, err
		}
		a, b := parseDate(f), parseDate(t)
		if a.Before(from) {
			a = from
		}
		if b.After(to) {
			b = to
		}
		half := h
		if !a.Equal(b) {
			half = ""
		}
		if n, err := CountLeaveDays(a, b, half, wo, holidaySet(hol)); err == nil {
			lop += n
		}
	}
	rows.Close()
	if st.LOPMode == "attendance_and_leave" {
		days, err := r.classifyRange(ctx, orgID, e.ID, from, to)
		if err != nil {
			return 0, 0, err
		}
		lop += Summarise(days).LOP
	}
	return lop, outside, nil
}

// buildPayslip computes and stores one employee's payslip for a run.
func (r *Repository) buildPayslip(ctx context.Context, orgID string, run Run, empID string, ads []Adjustment) error {
	from, to, _ := monthRange(run.Period)
	e, err := r.empLite(ctx, r.pool, orgID, empID)
	if err != nil {
		return err
	}
	sal, err := r.salaryFor(ctx, orgID, empID, to)
	if err != nil {
		return err
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return err
	}
	lop, outside, err := r.lopFor(ctx, orgID, e, from, to, st)
	if err != nil {
		return err
	}
	var state string
	_ = r.pool.QueryRow(ctx, `SELECT coalesce(l.state,'') FROM adrient.hr_employees x LEFT JOIN adrient.hr_locations l ON l.id=x.location_id WHERE x.id=$1::uuid`, empID).Scan(&state)
	slabs, err := r.ptSlabs(ctx, orgID)
	if err != nil {
		return err
	}
	res, err := ComputePayslip(PayrollInput{Lines: sal.Lines, DaysInMonth: to.Day(), LOPDays: lop, OutsideDays: outside, PFEnabled: sal.PFEnabled,
		ESIEnabled: sal.ESIEnabled, PTEnabled: sal.PTEnabled, TDSEnabled: sal.TDSEnabled, PFCapped: st.PFCapped, PFCeiling: st.PFWageCeiling,
		ESILimit: st.ESIWageLimit, State: state, Month: int(from.Month()), PTSlabs: slabs, Adjustments: ads})
	if err != nil {
		return err
	}
	emp, err := r.getEmployee(ctx, r.pool, orgID, empID)
	if err != nil {
		return err
	}
	ej, _ := json.Marshal(res.Earnings)
	dj, _ := json.Marshal(res.Deductions)
	if ads == nil {
		ads = []Adjustment{}
	}
	aj, _ := json.Marshal(ads)
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.hr_payslips (org_id, run_id, employee_id, emp_code, emp_name, designation, department, pan, uan, bank_name, bank_account, bank_ifsc,
		days_in_month, lop_days, paid_days, earnings, deductions, adjustments, gross, pf_employee, pf_employer, esi_employee, esi_employer, prof_tax, tds, other_deductions, reimbursements, net_pay)
		VALUES ($1,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)
		ON CONFLICT (run_id, employee_id) DO UPDATE SET lop_days=EXCLUDED.lop_days, paid_days=EXCLUDED.paid_days, earnings=EXCLUDED.earnings, deductions=EXCLUDED.deductions,
		adjustments=EXCLUDED.adjustments, gross=EXCLUDED.gross, pf_employee=EXCLUDED.pf_employee, pf_employer=EXCLUDED.pf_employer, esi_employee=EXCLUDED.esi_employee,
		esi_employer=EXCLUDED.esi_employer, prof_tax=EXCLUDED.prof_tax, tds=EXCLUDED.tds, other_deductions=EXCLUDED.other_deductions, reimbursements=EXCLUDED.reimbursements, net_pay=EXCLUDED.net_pay`,
		orgID, run.ID, empID, emp.EmpCode, emp.Name, emp.DesignationName, emp.DepartmentName, emp.PAN, emp.UAN, emp.BankName, emp.BankAccount, emp.BankIFSC,
		to.Day(), round2(lop), res.PaidDays, ej, dj, aj, res.Gross, res.PFEmployee, res.PFEmployer, res.ESIEmployee, res.ESIEmployer, res.ProfTax, res.TDS, res.OtherDed, res.Reimburse, res.Net)
	return err
}

// Payslips lists a run's payslips (HR only).
func (r *Repository) RunPayslips(ctx context.Context, a Actor, runID string) ([]Payslip, error) {
	if err := a.requireHR(); err != nil {
		return nil, err
	}
	if !isUUID(runID) {
		return nil, ErrNotFound
	}
	return r.queryPayslips(ctx, `WHERE p.org_id=$1 AND p.run_id=$2::uuid ORDER BY p.emp_code`, a.OrgID, runID)
}

const slipCols = `p.id::text, p.run_id::text, r.period, r.status, p.employee_id::text, p.emp_code, p.emp_name, p.designation, p.department, p.pan, p.uan, p.bank_name, p.bank_account, p.bank_ifsc,
	p.days_in_month, p.lop_days::float8, p.paid_days::float8, p.earnings, p.deductions, p.adjustments, p.gross::float8, p.pf_employee::float8, p.pf_employer::float8, p.esi_employee::float8,
	p.esi_employer::float8, p.prof_tax::float8, p.tds::float8, p.other_deductions::float8, p.reimbursements::float8, p.net_pay::float8`

func (r *Repository) queryPayslips(ctx context.Context, where string, args ...any) ([]Payslip, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+slipCols+` FROM adrient.hr_payslips p JOIN adrient.hr_payroll_runs r ON r.id=p.run_id `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Payslip{}
	for rows.Next() {
		var p Payslip
		var e, d, ad []byte
		if err := rows.Scan(&p.ID, &p.RunID, &p.Period, &p.RunStatus, &p.EmployeeID, &p.EmpCode, &p.EmpName, &p.Designation, &p.Department, &p.PAN, &p.UAN, &p.BankName,
			&p.BankAccount, &p.BankIFSC, &p.DaysInMonth, &p.LOPDays, &p.PaidDays, &e, &d, &ad, &p.Gross, &p.PFEmployee, &p.PFEmployer, &p.ESIEmployee, &p.ESIEmployer,
			&p.ProfTax, &p.TDS, &p.OtherDed, &p.Reimburse, &p.Net); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(e, &p.Earnings)
		_ = json.Unmarshal(d, &p.Deductions)
		_ = json.Unmarshal(ad, &p.Adjustments)
		out = append(out, p)
	}
	return out, rows.Err()
}

// MyPayslips lists the caller's own payslips of locked runs.
func (r *Repository) MyPayslips(ctx context.Context, a Actor) ([]Payslip, error) {
	if a.EmployeeID == "" {
		return []Payslip{}, nil
	}
	return r.queryPayslips(ctx, `WHERE p.org_id=$1 AND p.employee_id=$2::uuid AND r.status='locked' ORDER BY r.period DESC`, a.OrgID, a.EmployeeID)
}

// GetPayslip returns one payslip: HR any, an employee only their own once the
// run is locked. Anyone else gets "not found".
func (r *Repository) GetPayslip(ctx context.Context, a Actor, id string) (Payslip, error) {
	if !isUUID(id) {
		return Payslip{}, ErrNotFound
	}
	list, err := r.queryPayslips(ctx, `WHERE p.org_id=$1 AND p.id=$2::uuid`, a.OrgID, id)
	if err != nil {
		return Payslip{}, err
	}
	if len(list) == 0 {
		return Payslip{}, ErrNotFound
	}
	p := list[0]
	if a.IsHR {
		return p, nil
	}
	if a.EmployeeID != "" && p.EmployeeID == a.EmployeeID && p.RunStatus == "locked" {
		return p, nil
	}
	return Payslip{}, ErrNotFound
}

// PayslipPDF renders a payslip the caller may read.
func (r *Repository) PayslipPDF(ctx context.Context, a Actor, id string) ([]byte, string, error) {
	p, err := r.GetPayslip(ctx, a, id)
	if err != nil {
		return nil, "", err
	}
	st, _ := r.GetSettings(ctx, a.OrgID)
	name := strings.NewReplacer(" ", "_", "/", "-").Replace(p.EmpCode + "_" + p.Period)
	return RenderPayslipPDF(p, st.CompanyName, st.CompanyAddress), "payslip_" + name + ".pdf", nil
}

// SetAdjustments replaces the one-off adjustments of a payslip in a draft run
// and recomputes it (HR only). Reimbursements must reference an approved
// Accounting expense report of that employee.
func (r *Repository) SetAdjustments(ctx context.Context, a Actor, payslipID string, ads []Adjustment) (Payslip, error) {
	if err := a.requireHR(); err != nil {
		return Payslip{}, err
	}
	p, err := r.GetPayslip(ctx, a, payslipID)
	if err != nil {
		return p, err
	}
	if p.RunStatus != "draft" {
		return p, conflict("adjustments can only be changed while the run is a draft")
	}
	if err := ValidateAdjustments(ads); err != nil {
		return p, err
	}
	var userID string
	_ = r.pool.QueryRow(ctx, `SELECT coalesce(user_id::text,'') FROM adrient.hr_employees WHERE id=$1::uuid`, p.EmployeeID).Scan(&userID)
	var claims []ExpenseClaim
	for i := range ads {
		if ads[i].Kind != "reimbursement" || ads[i].ExpenseReportID == "" {
			ads[i].ExpenseReportID = ""
			continue
		}
		if r.pay == nil || userID == "" {
			return p, invalid("expense reports can only be linked for employees with an account when Accounting is available")
		}
		if claims == nil {
			if claims, err = r.pay.ListApprovedReports(ctx, a.OrgID, userID); err != nil {
				return p, err
			}
		}
		found := false
		for _, c := range claims {
			if c.ID == ads[i].ExpenseReportID {
				ads[i].Amount, found = c.Amount, true
				if ads[i].Name == "" {
					ads[i].Name = c.Title
				}
			}
		}
		if !found {
			return p, invalid("expense report is not an approved report of this employee awaiting reimbursement")
		}
		var used int
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_payslips WHERE org_id=$1 AND id<>$2::uuid AND adjustments @> jsonb_build_array(jsonb_build_object('expense_report_id',$3::text))`,
			a.OrgID, payslipID, ads[i].ExpenseReportID).Scan(&used)
		if used > 0 {
			return p, conflict("that expense report is already included in another payslip")
		}
	}
	run, err := r.getRun(ctx, a.OrgID, p.RunID)
	if err != nil {
		return p, err
	}
	if err := r.buildPayslip(ctx, a.OrgID, run, p.EmployeeID, ads); err != nil {
		return p, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "adjust", "payslip", payslipID, fmt.Sprintf("%d adjustments", len(ads)))
	return r.GetPayslip(ctx, a, payslipID)
}

// ReimbursementCandidates lists approved expense reports awaiting repayment for
// the payslip's employee (HR only).
func (r *Repository) ReimbursementCandidates(ctx context.Context, a Actor, payslipID string) ([]ExpenseClaim, error) {
	if err := a.requireHR(); err != nil {
		return nil, err
	}
	p, err := r.GetPayslip(ctx, a, payslipID)
	if err != nil {
		return nil, err
	}
	var userID string
	_ = r.pool.QueryRow(ctx, `SELECT coalesce(user_id::text,'') FROM adrient.hr_employees WHERE id=$1::uuid`, p.EmployeeID).Scan(&userID)
	if r.pay == nil || userID == "" {
		return []ExpenseClaim{}, nil
	}
	return r.pay.ListApprovedReports(ctx, a.OrgID, userID)
}

// MarkReviewed moves a draft run to reviewed (HR only); reopen goes back.
func (r *Repository) MarkReviewed(ctx context.Context, a Actor, id string) (Run, error) {
	if err := a.requireHR(); err != nil {
		return Run{}, err
	}
	run, err := r.getRun(ctx, a.OrgID, id)
	if err != nil {
		return run, err
	}
	if run.Status != "draft" {
		return run, conflict("the run is %s", run.Status)
	}
	if run.Employees == 0 {
		return run, invalid("the run has no payslips; set salaries for employees first")
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.hr_payroll_runs SET status='reviewed', reviewed_by=$3::uuid, updated_at=now() WHERE org_id=$1 AND id=$2::uuid AND status='draft'`, a.OrgID, id, a.UserID); err != nil {
		return run, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "review", "payroll_run", id, run.Period)
	return r.getRun(ctx, a.OrgID, id)
}

// ReopenRun returns a reviewed run to draft (HR only). Locked runs are final.
func (r *Repository) ReopenRun(ctx context.Context, a Actor, id string) (Run, error) {
	if err := a.requireHR(); err != nil {
		return Run{}, err
	}
	run, err := r.getRun(ctx, a.OrgID, id)
	if err != nil {
		return run, err
	}
	if run.Status != "reviewed" {
		return run, conflict("only a reviewed run can be reopened (a locked run is final)")
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.hr_payroll_runs SET status='draft', updated_at=now() WHERE org_id=$1 AND id=$2::uuid AND status='reviewed'`, a.OrgID, id); err != nil {
		return run, err
	}
	return r.getRun(ctx, a.OrgID, id)
}

// DeleteRun removes a draft run (HR only).
func (r *Repository) DeleteRun(ctx context.Context, a Actor, id string) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	run, err := r.getRun(ctx, a.OrgID, id)
	if err != nil {
		return err
	}
	if run.Status != "draft" {
		return conflict("only a draft run can be deleted")
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.hr_payroll_runs WHERE org_id=$1 AND id=$2::uuid AND status='draft'`, a.OrgID, id)
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "delete", "payroll_run", id, run.Period)
	return err
}

// LockRun finalises a reviewed run: posts the payroll journal to Accounting
// (once per run, idempotent), settles linked expense reports and makes the
// payslips visible to employees.
func (r *Repository) LockRun(ctx context.Context, a Actor, id string) (Run, error) {
	if err := a.requireHR(); err != nil {
		return Run{}, err
	}
	run, err := r.getRun(ctx, a.OrgID, id)
	if err != nil {
		return run, err
	}
	if run.Status != "reviewed" {
		return run, conflict("review the run before locking it (status is %s)", run.Status)
	}
	slips, err := r.queryPayslips(ctx, `WHERE p.org_id=$1 AND p.run_id=$2::uuid`, a.OrgID, id)
	if err != nil {
		return run, err
	}
	var t RunTotals
	var reports []string
	for _, p := range slips {
		t.Employees++
		t.Gross += p.Gross
		t.PFEE += p.PFEmployee
		t.PFER += p.PFEmployer
		t.ESIEE += p.ESIEmployee
		t.ESIER += p.ESIEmployer
		t.PT += p.ProfTax
		t.TDS += p.TDS
		t.Other += p.OtherDed
		t.Reimb += p.Reimburse
		t.Net += p.Net
		for _, ad := range p.Adjustments {
			if ad.ExpenseReportID != "" {
				reports = append(reports, ad.ExpenseReportID)
			}
		}
	}
	_, to, _ := monthRange(run.Period)
	j := t.BuildJournal(id, run.Period, fmtDate(to))
	if d := j.Balanced(); d > 0.05 || d < -0.05 {
		return run, fmt.Errorf("payroll journal does not balance (difference %.2f)", d)
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.hr_payroll_runs SET status='locked', locked_by=$3::uuid, locked_at=now(), updated_at=now() WHERE org_id=$1 AND id=$2::uuid AND status='reviewed'`, a.OrgID, id, a.UserID)
	if err != nil {
		return run, err
	}
	if tag.RowsAffected() == 0 {
		return run, conflict("the run was already locked")
	}
	journalID := ""
	if r.pay != nil {
		journalID, err = r.pay.PostPayroll(ctx, a.OrgID, a.UserID, j)
		if err == nil && len(reports) > 0 {
			err = r.pay.MarkReportsReimbursed(ctx, a.OrgID, a.UserID, reports, fmtDate(to))
		}
		if err != nil {
			// Undo the lock so HR can fix the books (for example a closed period) and retry.
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.hr_payroll_runs SET status='reviewed', locked_by=NULL, locked_at=NULL WHERE id=$1::uuid`, id)
			return run, fmt.Errorf("posting to Accounting failed: %w", err)
		}
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.hr_payroll_runs SET journal_id=$2 WHERE id=$1::uuid`, id, journalID)
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "lock", "payroll_run", id, fmt.Sprintf("%s journal %s", run.Period, journalID))
	for _, p := range slips {
		var uid string
		if r.pool.QueryRow(ctx, `SELECT coalesce(user_id::text,'') FROM adrient.hr_employees WHERE id=$1::uuid`, p.EmployeeID).Scan(&uid) == nil {
			r.tell(ctx, a.OrgID, uid, a.UserID, "hr_payslip", "Payslip available", "Your payslip for "+PeriodLabel(run.Period)+" is ready.", "/hr/payslips")
		}
	}
	slog.Info("hr: payroll locked", "run", id, "period", run.Period, "journal", journalID)
	return r.getRun(ctx, a.OrgID, id)
}

// BankAdviceCSV writes the transfer file for a reviewed or locked run (HR only).
func (r *Repository) BankAdviceCSV(ctx context.Context, a Actor, runID string, w io.Writer) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	run, err := r.getRun(ctx, a.OrgID, runID)
	if err != nil {
		return err
	}
	if run.Status == "draft" {
		return conflict("review the run before generating the bank advice")
	}
	slips, err := r.queryPayslips(ctx, `WHERE p.org_id=$1 AND p.run_id=$2::uuid ORDER BY p.emp_code`, a.OrgID, runID)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"emp_code", "beneficiary_name", "bank_name", "account_number", "ifsc", "amount", "narration", "details_missing"})
	for _, p := range slips {
		if p.Net <= 0 {
			continue
		}
		missing := ""
		if p.BankAccount == "" || p.BankIFSC == "" {
			missing = "yes"
		}
		_ = cw.Write([]string{csvSafe(p.EmpCode), csvSafe(p.EmpName), csvSafe(p.BankName), csvSafe(p.BankAccount), csvSafe(p.BankIFSC), money(p.Net), "Salary " + PeriodLabel(run.Period), missing})
	}
	cw.Flush()
	return cw.Error()
}

// StatutoryCSV exports the PF / ESI / PT / TDS register of a run (HR only).
func (r *Repository) StatutoryCSV(ctx context.Context, a Actor, runID string, w io.Writer) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	if _, err := r.getRun(ctx, a.OrgID, runID); err != nil {
		return err
	}
	slips, err := r.queryPayslips(ctx, `WHERE p.org_id=$1 AND p.run_id=$2::uuid ORDER BY p.emp_code`, a.OrgID, runID)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"emp_code", "name", "uan", "pan", "gross", "pf_employee", "pf_employer", "esi_employee", "esi_employer", "professional_tax", "tds", "net_pay"})
	for _, p := range slips {
		_ = cw.Write([]string{csvSafe(p.EmpCode), csvSafe(p.EmpName), csvSafe(p.UAN), csvSafe(p.PAN), money(p.Gross), money(p.PFEmployee), money(p.PFEmployer),
			money(p.ESIEmployee), money(p.ESIEmployer), money(p.ProfTax), money(p.TDS), money(p.Net)})
	}
	cw.Flush()
	return cw.Error()
}
