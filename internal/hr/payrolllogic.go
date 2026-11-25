package hr

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Pure payroll maths for India: salary structure expansion, PF, ESI,
// professional tax, a simple TDS estimate and loss-of-pay proration. Statutory
// rates here are defaults: verify them for your registration and financial year.

// Salary calc kinds.
const (
	CalcFixed    = "fixed"     // Value is a monthly amount
	CalcPctBasic = "pct_basic" // Value percent of Basic
	CalcPctGross = "pct_gross" // Value percent of monthly gross
	CalcBalance  = "balance"   // whatever remains of monthly gross
)

// SalaryLine is one component of a salary structure or of an employee's salary.
// In a structure Value + Calc describe the rule; in an employee salary Amount
// holds the resolved monthly amount.
type SalaryLine struct {
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	Kind    string  `json:"kind"` // earning | deduction
	Calc    string  `json:"calc"`
	Value   float64 `json:"value"`
	Amount  float64 `json:"amount"`
	PFWage  bool    `json:"pf_wage"`
	ESIWage bool    `json:"esi_wage"`
	Taxable bool    `json:"taxable"`
}

// ValidateLines checks a structure or salary definition.
func ValidateLines(lines []SalaryLine, structure bool) error {
	if len(lines) == 0 {
		return invalid("add at least one component")
	}
	seen := map[string]bool{}
	basic := false
	balances := 0
	for i := range lines {
		l := &lines[i]
		l.Code = strings.ToUpper(strings.TrimSpace(l.Code))
		l.Name = strings.TrimSpace(l.Name)
		if l.Code == "" || l.Name == "" {
			return invalid("every component needs a code and a name")
		}
		if seen[l.Code] {
			return invalid("duplicate component code %s", l.Code)
		}
		seen[l.Code] = true
		if l.Kind != "earning" && l.Kind != "deduction" {
			return invalid("component %s: kind must be earning or deduction", l.Code)
		}
		if l.Kind == "deduction" && (l.Calc == CalcBalance || l.PFWage || l.ESIWage) {
			return invalid("component %s: a deduction cannot be a balancing or wage component", l.Code)
		}
		if l.Code == "BASIC" {
			basic = true
			if structure && l.Calc != CalcFixed && l.Calc != CalcPctGross {
				return invalid("BASIC must be a fixed amount or a percent of gross")
			}
		}
		if l.Calc == CalcBalance {
			balances++
		}
		switch l.Calc {
		case CalcFixed, CalcPctBasic, CalcPctGross, CalcBalance:
		default:
			if structure {
				return invalid("component %s: unknown calc %q", l.Code, l.Calc)
			}
		}
		if l.Value < 0 || l.Amount < 0 || (l.Calc == CalcPctBasic || l.Calc == CalcPctGross) && l.Value > 100 {
			return invalid("component %s: amounts and percentages must be sensible and non-negative", l.Code)
		}
	}
	if structure && !basic {
		return invalid("a BASIC component is required (PF is computed on it)")
	}
	if balances > 1 {
		return invalid("only one balancing component is allowed")
	}
	return nil
}

// ExpandStructure resolves a structure into monthly amounts for an annual gross.
// The balancing component absorbs the remainder so earnings add up to exactly
// annual/12. Fails when the fixed parts exceed the gross.
func ExpandStructure(annualGross float64, lines []SalaryLine) ([]SalaryLine, error) {
	if annualGross <= 0 {
		return nil, invalid("annual gross must be positive")
	}
	if err := ValidateLines(lines, true); err != nil {
		return nil, err
	}
	monthly := round2(annualGross / 12)
	out := make([]SalaryLine, len(lines))
	copy(out, lines)
	var basic float64
	for i := range out {
		if out[i].Code == "BASIC" {
			if out[i].Calc == CalcFixed {
				out[i].Amount = round2(out[i].Value)
			} else {
				out[i].Amount = round2(monthly * out[i].Value / 100)
			}
			basic = out[i].Amount
		}
	}
	var earn float64
	balIdx := -1
	for i := range out {
		l := &out[i]
		if l.Code == "BASIC" && l.Kind == "earning" {
			earn += l.Amount
			continue
		}
		switch l.Calc {
		case CalcFixed:
			l.Amount = round2(l.Value)
		case CalcPctBasic:
			l.Amount = round2(basic * l.Value / 100)
		case CalcPctGross:
			l.Amount = round2(monthly * l.Value / 100)
		case CalcBalance:
			balIdx = i
			continue
		}
		if l.Kind == "earning" {
			earn += l.Amount
		}
	}
	if earn > monthly+0.005 {
		return nil, invalid("the components add up to %.2f a month, more than the gross %.2f", earn, monthly)
	}
	if balIdx >= 0 {
		out[balIdx].Amount = round2(monthly - earn)
	} else if math.Abs(monthly-earn) > 0.5 {
		return nil, invalid("the earnings add up to %.2f but the monthly gross is %.2f; add a balancing component", earn, monthly)
	}
	return out, nil
}

// PTSlab is one professional tax slab for a state.
type PTSlab struct {
	State  string   `json:"state"`
	Min    float64  `json:"min_salary"`
	Max    *float64 `json:"max_salary"`
	Amount float64  `json:"amount"`
	Month  int      `json:"month"` // 0 = every month; 1-12 = that month only (overrides)
}

// ProfessionalTax returns the PT for a monthly gross: a slab specific to the
// month wins over the every-month slab.
func ProfessionalTax(slabs []PTSlab, state string, gross float64, month int) float64 {
	pick := func(m int) (float64, bool) {
		for _, s := range slabs {
			if !strings.EqualFold(s.State, state) || s.Month != m {
				continue
			}
			if gross >= s.Min && (s.Max == nil || gross <= *s.Max) {
				return s.Amount, true
			}
		}
		return 0, false
	}
	if v, ok := pick(month); ok && month != 0 {
		return v
	}
	v, _ := pick(0)
	return v
}

// Rate constants (defaults).
const (
	PFEmployeeRate  = 0.12
	PFEmployerRate  = 0.12
	ESIEmployeeRate = 0.0075
	ESIEmployerRate = 0.0325
	StandardDeduct  = 75000.0
)

// PF returns the employee and employer contributions on PF wages (sum of
// PF-wage components after loss of pay), capped at the ceiling when capped.
func PF(pfWages float64, capped bool, ceiling float64) (ee, er float64) {
	w := pfWages
	if capped && w > ceiling {
		w = ceiling
	}
	if w <= 0 {
		return 0, 0
	}
	return math.Round(w * PFEmployeeRate), math.Round(w * PFEmployerRate)
}

// ESI applies when the contracted monthly ESI wages are within the limit.
// Contributions round up to the next rupee, as ESIC does.
func ESI(contractedWages, paidWages, limit float64) (ee, er float64) {
	if contractedWages <= 0 || contractedWages > limit || paidWages <= 0 {
		return 0, 0
	}
	return math.Ceil(paidWages*ESIEmployeeRate - 1e-9), math.Ceil(paidWages*ESIEmployerRate - 1e-9)
}

// NewRegimeTax estimates annual income tax under the new regime (FY 2025-26
// slabs, 87A rebate with marginal relief, 4% cess) on taxable income after the
// standard deduction. It is an estimate, not a filing.
func NewRegimeTax(taxable float64) float64 {
	if taxable <= 0 {
		return 0
	}
	slabs := []struct{ upto, rate float64 }{{400000, 0}, {800000, 0.05}, {1200000, 0.10}, {1600000, 0.15}, {2000000, 0.20}, {2400000, 0.25}, {math.MaxFloat64, 0.30}}
	var tax, prev float64
	for _, s := range slabs {
		if taxable > prev {
			top := math.Min(taxable, s.upto)
			tax += (top - prev) * s.rate
		}
		prev = s.upto
	}
	if taxable <= 1200000 {
		tax = 0
	} else if taxable <= 1275000 && tax > taxable-1200000 {
		tax = taxable - 1200000 // marginal relief
	}
	return math.Round(tax * 1.04)
}

// MonthlyTDS spreads the estimated annual tax evenly over 12 months.
func MonthlyTDS(monthlyGross float64) float64 {
	annual := monthlyGross*12 - StandardDeduct
	return math.Round(NewRegimeTax(annual) / 12)
}

// Adjustment is an HR one-off on a payslip.
type Adjustment struct {
	Kind            string  `json:"kind"` // earning | deduction | reimbursement
	Name            string  `json:"name"`
	Amount          float64 `json:"amount"`
	ExpenseReportID string  `json:"expense_report_id,omitempty"`
}

// Amt is a labelled amount on a payslip.
type Amt struct {
	Code   string  `json:"code"`
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

// PayrollInput is everything needed to compute one payslip.
type PayrollInput struct {
	Lines       []SalaryLine
	DaysInMonth int
	LOPDays     float64
	OutsideDays float64 // days before joining / after exit in the month
	PFEnabled   bool
	ESIEnabled  bool
	PTEnabled   bool
	TDSEnabled  bool
	PFCapped    bool
	PFCeiling   float64
	ESILimit    float64
	State       string
	Month       int
	PTSlabs     []PTSlab
	Adjustments []Adjustment
}

// PayrollResult is one computed payslip.
type PayrollResult struct {
	PaidDays    float64 `json:"paid_days"`
	Earnings    []Amt   `json:"earnings"`
	Deductions  []Amt   `json:"deductions"`
	Gross       float64 `json:"gross"`
	PFEmployee  float64 `json:"pf_employee"`
	PFEmployer  float64 `json:"pf_employer"`
	ESIEmployee float64 `json:"esi_employee"`
	ESIEmployer float64 `json:"esi_employer"`
	ProfTax     float64 `json:"prof_tax"`
	TDS         float64 `json:"tds"`
	OtherDed    float64 `json:"other_deductions"`
	Reimburse   float64 `json:"reimbursements"`
	Net         float64 `json:"net_pay"`
}

// ComputePayslip applies loss of pay and the statutory deductions.
func ComputePayslip(in PayrollInput) (PayrollResult, error) {
	var res PayrollResult
	if in.DaysInMonth < 28 || in.DaysInMonth > 31 {
		return res, invalid("days in month must be 28 to 31")
	}
	payable := float64(in.DaysInMonth) - in.OutsideDays - in.LOPDays
	if payable < 0 {
		payable = 0
	}
	res.PaidDays = round2(payable)
	factor := payable / float64(in.DaysInMonth)
	var pfWages, esiPaid, esiContract, contractGross float64
	for _, l := range in.Lines {
		if l.Kind != "earning" {
			continue
		}
		amt := round2(l.Amount * factor)
		res.Earnings = append(res.Earnings, Amt{l.Code, l.Name, amt})
		res.Gross += amt
		contractGross += l.Amount
		if l.PFWage {
			pfWages += amt
		}
		if l.ESIWage {
			esiPaid += amt
			esiContract += l.Amount
		}
	}
	for _, ad := range in.Adjustments {
		if ad.Kind == "earning" && ad.Amount > 0 {
			res.Earnings = append(res.Earnings, Amt{"ADJ", ad.Name, round2(ad.Amount)})
			res.Gross += round2(ad.Amount)
		}
	}
	res.Gross = round2(res.Gross)
	if in.PFEnabled {
		res.PFEmployee, res.PFEmployer = PF(pfWages, in.PFCapped, in.PFCeiling)
	}
	if in.ESIEnabled {
		res.ESIEmployee, res.ESIEmployer = ESI(esiContract, esiPaid, in.ESILimit)
	}
	if in.PTEnabled && in.State != "" {
		res.ProfTax = ProfessionalTax(in.PTSlabs, in.State, res.Gross, in.Month)
	}
	if in.TDSEnabled {
		res.TDS = MonthlyTDS(contractGross)
	}
	for _, l := range in.Lines {
		if l.Kind == "deduction" && l.Amount > 0 {
			res.Deductions = append(res.Deductions, Amt{l.Code, l.Name, round2(l.Amount)})
			res.OtherDed += round2(l.Amount)
		}
	}
	for _, ad := range in.Adjustments {
		switch ad.Kind {
		case "deduction":
			if ad.Amount > 0 {
				res.Deductions = append(res.Deductions, Amt{"ADJ", ad.Name, round2(ad.Amount)})
				res.OtherDed += round2(ad.Amount)
			}
		case "reimbursement":
			if ad.Amount > 0 {
				res.Reimburse += round2(ad.Amount)
			}
		}
	}
	res.OtherDed, res.Reimburse = round2(res.OtherDed), round2(res.Reimburse)
	res.Net = round2(res.Gross - res.PFEmployee - res.ESIEmployee - res.ProfTax - res.TDS - res.OtherDed + res.Reimburse)
	if res.Net < 0 {
		return res, invalid("deductions exceed the pay (net %.2f)", res.Net)
	}
	return res, nil
}

// ValidateAdjustments checks HR one-offs.
func ValidateAdjustments(ads []Adjustment) error {
	for i := range ads {
		ads[i].Name = strings.TrimSpace(ads[i].Name)
		if ads[i].Kind != "earning" && ads[i].Kind != "deduction" && ads[i].Kind != "reimbursement" {
			return invalid("adjustment kind must be earning, deduction or reimbursement")
		}
		if ads[i].Name == "" || ads[i].Amount <= 0 || ads[i].Amount > 100000000 {
			return invalid("every adjustment needs a name and a positive amount")
		}
	}
	return nil
}

// RunTotals sums payslip results for the journal and the run header.
type RunTotals struct {
	Employees                                                   int
	Gross, PFEE, PFER, ESIEE, ESIER, PT, TDS, Other, Reimb, Net float64
}

// BuildJournal derives the accounting posting from run totals. Debits: salaries
// (gross), employer contributions, reimbursements settled through payroll.
// Credits: statutory payables, other deductions and salary payable (net).
func (t RunTotals) BuildJournal(runID, period, date string) PayrollJournal {
	return PayrollJournal{RunID: runID, Period: period, Date: date, Employees: t.Employees,
		Gross: round2(t.Gross), EmployerContrib: round2(t.PFER + t.ESIER), Reimburse: round2(t.Reimb),
		PFPayable: round2(t.PFEE + t.PFER), ESIPayable: round2(t.ESIEE + t.ESIER), PTPayable: round2(t.PT),
		TDSPayable: round2(t.TDS), OtherDed: round2(t.Other), NetSalaryDue: round2(t.Net)}
}

// Balanced reports debits minus credits of a payroll journal (should be 0).
func (j PayrollJournal) Balanced() float64 {
	dr := j.Gross + j.EmployerContrib + j.Reimburse
	cr := j.PFPayable + j.ESIPayable + j.PTPayable + j.TDSPayable + j.OtherDed + j.NetSalaryDue
	return round2(dr - cr)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func money(f float64) string { return fmt.Sprintf("%.2f", f) }
