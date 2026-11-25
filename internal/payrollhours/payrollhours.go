// Package payrollhours pushes worked hours (from Shifts or Time tracking) into
// an HR payroll run as one earning adjustment per employee. It only uses HR's
// public Repository methods, so HR's own rules (HR admin only, draft runs only)
// apply. Re-running an export for the same period and source replaces the
// earlier line instead of adding a second one.
package payrollhours

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
)

// Line is the pay for one employee for the period.
type Line struct {
	EmployeeID string  `json:"employee_id"`
	Hours      float64 `json:"hours"`
	Amount     float64 `json:"amount"`
}

// Applied reports one payslip that received the line.
type Applied struct {
	EmployeeID string  `json:"employee_id"`
	Name       string  `json:"name"`
	Hours      float64 `json:"hours"`
	Amount     float64 `json:"amount"`
}

// Skipped explains why an employee's hours were not exported.
type Skipped struct {
	EmployeeID string `json:"employee_id"`
	Reason     string `json:"reason"`
}

// Result is the outcome of an export.
type Result struct {
	RunID      string    `json:"run_id"`
	Period     string    `json:"period"`
	CreatedRun bool      `json:"created_run"`
	Applied    []Applied `json:"applied"`
	Skipped    []Skipped `json:"skipped"`
}

// Tag is the prefix identifying the lines a source owns on a payslip.
func Tag(source string) string { return "Hours: " + source }

// Merge returns the adjustments with this source's line replaced (or removed
// when amount is zero). Other adjustments are kept untouched.
func Merge(existing []hr.Adjustment, source string, hours, amount float64) []hr.Adjustment {
	tag := Tag(source)
	out := make([]hr.Adjustment, 0, len(existing)+1)
	for _, a := range existing {
		if strings.HasPrefix(a.Name, tag) {
			continue
		}
		out = append(out, a)
	}
	if amount > 0 {
		out = append(out, hr.Adjustment{Kind: "earning", Name: fmt.Sprintf("%s (%.2f h)", tag, hours), Amount: round2(amount)})
	}
	return out
}

func round2(f float64) float64 { return float64(int64(f*100+0.5)) / 100 }

// Export applies lines to the draft payroll run of period (YYYY-MM), creating
// the draft run first when none exists.
func Export(ctx context.Context, repo *hr.Repository, a hr.Actor, period, source string, lines []Line) (Result, error) {
	res := Result{Period: period, Applied: []Applied{}, Skipped: []Skipped{}}
	runs, err := repo.ListRuns(ctx, a)
	if err != nil {
		return res, err
	}
	var run *hr.Run
	for i := range runs {
		if runs[i].Period == period {
			run = &runs[i]
		}
	}
	if run == nil {
		r, err := repo.CreateRun(ctx, a, period)
		if err != nil {
			return res, err
		}
		run, res.CreatedRun = &r, true
	}
	if run.Status != "draft" {
		return res, hr.ErrConflict
	}
	res.RunID = run.ID
	slips, err := repo.RunPayslips(ctx, a, run.ID)
	if err != nil {
		return res, err
	}
	byEmp := map[string]hr.Payslip{}
	for _, p := range slips {
		byEmp[p.EmployeeID] = p
	}
	for _, l := range lines {
		p, ok := byEmp[l.EmployeeID]
		if !ok {
			res.Skipped = append(res.Skipped, Skipped{EmployeeID: l.EmployeeID, Reason: "no payslip in this run (employee has no salary structure in HR)"})
			continue
		}
		merged := Merge(p.Adjustments, source, l.Hours, l.Amount)
		if _, err := repo.SetAdjustments(ctx, a, p.ID, merged); err != nil {
			if errors.Is(err, hr.ErrInvalid) {
				res.Skipped = append(res.Skipped, Skipped{EmployeeID: l.EmployeeID, Reason: err.Error()})
				continue
			}
			return res, err
		}
		res.Applied = append(res.Applied, Applied{EmployeeID: l.EmployeeID, Name: p.EmpName, Hours: l.Hours, Amount: l.Amount})
	}
	return res, nil
}
