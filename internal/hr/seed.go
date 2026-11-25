package hr

import (
	"context"
	"encoding/json"
)

func f64(v float64) *float64 { return &v }

// DefaultPTSlabs are starter professional tax tables for a few states. They are
// editable and only a starting point: state rules change, so verify them.
var DefaultPTSlabs = []PTSlab{
	{State: "Maharashtra", Min: 0, Max: f64(7500), Amount: 0},
	{State: "Maharashtra", Min: 7500.01, Max: f64(10000), Amount: 175},
	{State: "Maharashtra", Min: 10000.01, Amount: 200},
	{State: "Maharashtra", Min: 10000.01, Amount: 300, Month: 2},
	{State: "Karnataka", Min: 0, Max: f64(24999.99), Amount: 0},
	{State: "Karnataka", Min: 25000, Amount: 200},
	{State: "Telangana", Min: 0, Max: f64(15000), Amount: 0},
	{State: "Telangana", Min: 15000.01, Max: f64(20000), Amount: 150},
	{State: "Telangana", Min: 20000.01, Amount: 200},
	{State: "West Bengal", Min: 0, Max: f64(10000), Amount: 0},
	{State: "West Bengal", Min: 10000.01, Max: f64(15000), Amount: 110},
	{State: "West Bengal", Min: 15000.01, Max: f64(25000), Amount: 130},
	{State: "West Bengal", Min: 25000.01, Max: f64(40000), Amount: 150},
	{State: "West Bengal", Min: 40000.01, Amount: 200},
	{State: "Gujarat", Min: 0, Max: f64(12000), Amount: 0},
	{State: "Gujarat", Min: 12000.01, Amount: 200},
	{State: "Andhra Pradesh", Min: 0, Max: f64(15000), Amount: 0},
	{State: "Andhra Pradesh", Min: 15000.01, Max: f64(20000), Amount: 150},
	{State: "Andhra Pradesh", Min: 20000.01, Amount: 200},
}

// DefaultStructure is a typical Indian CTC split.
var DefaultStructure = []SalaryLine{
	{Code: "BASIC", Name: "Basic", Kind: "earning", Calc: CalcPctGross, Value: 40, PFWage: true, ESIWage: true, Taxable: true},
	{Code: "HRA", Name: "House Rent Allowance", Kind: "earning", Calc: CalcPctBasic, Value: 50, ESIWage: true, Taxable: true},
	{Code: "SPECIAL", Name: "Special Allowance", Kind: "earning", Calc: CalcBalance, ESIWage: true, Taxable: true},
}

// EnsureSeed creates the starter data for an org once: settings, a default
// shift, leave types, a holiday calendar, a salary structure and PT slabs.
// Safe to call repeatedly.
func (r *Repository) EnsureSeed(ctx context.Context, orgID string) error {
	if _, err := r.GetSettings(ctx, orgID); err != nil {
		return err
	}
	var seeded bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.hr_leave_types WHERE org_id=$1) OR EXISTS(SELECT 1 FROM adrient.hr_shifts WHERE org_id=$1)`, orgID).Scan(&seeded); err != nil {
		return err
	}
	if seeded {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "hr-seed:"+orgID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.hr_shifts (org_id, name, is_default) VALUES ($1,'General',true) ON CONFLICT DO NOTHING`, orgID); err != nil {
		return err
	}
	type lt struct {
		code, name, color string
		paid, tracked     bool
		quota             float64
		accrual           string
		carry             bool
		cap               float64
	}
	for _, t := range []lt{
		{"CL", "Casual Leave", "#2E86C1", true, true, 12, "monthly", false, 0},
		{"SL", "Sick Leave", "#C0392B", true, true, 6, "annual", false, 0},
		{"EL", "Earned Leave", "#126C5A", true, true, 15, "monthly", true, 30},
		{"LOP", "Loss of Pay", "#7F8C8D", false, false, 0, "none", false, 0},
	} {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.hr_leave_types (org_id, code, name, color, paid, tracked, annual_quota, accrual, carry_forward, carry_cap)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`, orgID, t.code, t.name, t.color, t.paid, t.tracked, t.quota, t.accrual, t.carry, t.cap); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.hr_holiday_calendars (org_id, name) VALUES ($1,'Company holidays') ON CONFLICT DO NOTHING`, orgID); err != nil {
		return err
	}
	lj, _ := json.Marshal(DefaultStructure)
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.hr_salary_structures (org_id, name, lines) VALUES ($1,'Standard (40% basic, 50% HRA)',$2) ON CONFLICT DO NOTHING`, orgID, lj); err != nil {
		return err
	}
	for _, s := range DefaultPTSlabs {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.hr_pt_slabs (org_id, state, min_salary, max_salary, amount, month) VALUES ($1,$2,$3,$4,$5,$6)`,
			orgID, s.State, s.Min, s.Max, s.Amount, s.Month); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
