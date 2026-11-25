package sheets

// What-if tools beyond goal seek: data tables (one and two variables),
// scenarios, and "Solver-lite". All of them write trial values into a copy of
// the posted workbook and recompute only the formula cells they report, so the
// workbook itself is never modified.
//
// Solver-lite is deliberately small and says so in its result ("method"): one
// objective cell (maximise, minimise or reach a value), up to 16 changing
// cells with optional bounds, and constraints of the form <cell> <op> <number>.
// It runs a penalty-method Nelder-Mead search from the current values. That is
// a local, derivative-free method: it finds a good point for smooth models, it
// is not a guaranteed global optimum and has no integer constraints.

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	ErrWhatIfInput   = errors.New("what-if: invalid request")
	ErrWhatIfTooMany = errors.New("what-if: too many cells or values")
)

const (
	whatIfMaxValues = 200
	solverMaxVars   = 16
	solverMaxEvals  = 4000
)

// whatIfModel evaluates formula cells of a workbook with some input cells overridden.
type whatIfModel struct {
	wb   FsWorkbook
	base int
	now  time.Time
}

type wiCell struct {
	sheet int
	addr  cellAddr
}

func newWhatIfModel(data, sheet string) (*whatIfModel, error) {
	var wb FsWorkbook
	if err := json.Unmarshal([]byte(data), &wb); err != nil || len(wb) == 0 {
		return nil, ErrNotWorkbook
	}
	base := 0
	if sheet != "" {
		base = -1
		for i, s := range wb {
			if s.ID == sheet || s.Name == sheet {
				base = i
				break
			}
		}
		if base < 0 {
			return nil, ErrWhatIfInput
		}
	}
	return &whatIfModel{wb: wb, base: base, now: time.Now()}, nil
}

func (m *whatIfModel) cell(ref string) (wiCell, error) {
	i, a, ok := resolveSheetAddr(m.wb, m.base, strings.TrimSpace(ref))
	if !ok {
		return wiCell{}, ErrWhatIfInput
	}
	return wiCell{i, a}, nil
}

// inputCell resolves a changing cell: it must not hold a formula.
func (m *whatIfModel) inputCell(ref string) (wiCell, float64, error) {
	c, err := m.cell(ref)
	if err != nil {
		return c, 0, err
	}
	fc := findCell(m.wb[c.sheet].CellData, c.addr)
	if fc == nil {
		return c, 0, nil
	}
	if strings.HasPrefix(fc.F, "=") {
		return c, 0, ErrGoalSeekChanging
	}
	if n, ok := fc.V.(float64); ok {
		return c, n, nil
	}
	return c, 0, nil
}

// eval overrides the inputs and returns the values of the output cells.
func (m *whatIfModel) eval(inputs map[wiCell]float64, outputs []wiCell) []value {
	wb := make(FsWorkbook, len(m.wb))
	copy(wb, m.wb)
	bySheet := map[int]map[cellAddr]float64{}
	for c, v := range inputs {
		if bySheet[c.sheet] == nil {
			bySheet[c.sheet] = map[cellAddr]float64{}
		}
		bySheet[c.sheet][c.addr] = v
	}
	for si, set := range bySheet {
		src := m.wb[si].CellData
		data := make([]FsCellData, 0, len(src)+len(set))
		done := map[cellAddr]bool{}
		for _, cd := range src {
			a := cellAddr{cd.R, cd.C}
			if v, ok := set[a]; ok {
				nc := FsCell{}
				if cd.V != nil {
					nc = *cd.V
				}
				nc.F, nc.V, nc.M = "", v, ""
				cd.V = &nc
				done[a] = true
			}
			data = append(data, cd)
		}
		for a, v := range set {
			if !done[a] {
				data = append(data, FsCellData{R: a.row, C: a.col, V: &FsCell{V: v}})
			}
		}
		wb[si].CellData = data
	}
	ev := newWorkbookEvaluator(wb, m.now)
	out := make([]value, len(outputs))
	for k, o := range outputs {
		v := ev.ensureFormula(o.sheet, o.addr)
		if v.kind == kindArray && v.arr != nil && v.arr.rows > 0 && v.arr.cols > 0 {
			v = v.arr.cells[0][0]
		}
		out[k] = v
	}
	return out
}

func valueOut(v value) interface{} {
	if v.kind == kindNum || v.kind == kindBool {
		return v.num
	}
	return v.asInterface()
}

// ---- data table -------------------------------------------------------------

// DataTableRequest: a one-variable table (RowInput or ColInput with Values)
// or a two-variable table (both inputs; RowValues × ColValues, one formula).
type DataTableRequest struct {
	Data     string   `json:"data"`
	Sheet    string   `json:"sheet,omitempty"`
	Formulas []string `json:"formulas"`
	// InputCell + Values: one-variable table (one row per value, one column per formula).
	InputCell string    `json:"inputCell,omitempty"`
	Values    []float64 `json:"values,omitempty"`
	// RowInput/ColInput with RowValues/ColValues: two-variable table over Formulas[0].
	RowInput  string    `json:"rowInput,omitempty"`
	ColInput  string    `json:"colInput,omitempty"`
	RowValues []float64 `json:"rowValues,omitempty"`
	ColValues []float64 `json:"colValues,omitempty"`
}

// DataTableResult: Results[i][j] is formula j at value i (one variable), or
// the formula at RowValues[i] × ColValues[j] (two variables).
type DataTableResult struct {
	Results [][]interface{} `json:"results"`
}

func DataTable(req DataTableRequest) (DataTableResult, error) {
	m, err := newWhatIfModel(req.Data, req.Sheet)
	if err != nil {
		return DataTableResult{}, err
	}
	if len(req.Formulas) == 0 || len(req.Formulas) > 20 {
		return DataTableResult{}, ErrWhatIfInput
	}
	outs := make([]wiCell, len(req.Formulas))
	for i, f := range req.Formulas {
		if outs[i], err = m.cell(f); err != nil {
			return DataTableResult{}, err
		}
	}
	var res DataTableResult
	if req.RowInput != "" && req.ColInput != "" {
		if len(req.RowValues)*len(req.ColValues) > whatIfMaxValues*10 || len(req.RowValues) == 0 || len(req.ColValues) == 0 {
			return res, ErrWhatIfTooMany
		}
		rc, _, err := m.inputCell(req.RowInput)
		if err != nil {
			return res, err
		}
		cc, _, err := m.inputCell(req.ColInput)
		if err != nil {
			return res, err
		}
		for _, rv := range req.RowValues {
			row := make([]interface{}, len(req.ColValues))
			for j, cv := range req.ColValues {
				row[j] = valueOut(m.eval(map[wiCell]float64{rc: rv, cc: cv}, outs[:1])[0])
			}
			res.Results = append(res.Results, row)
		}
		return res, nil
	}
	if req.InputCell == "" || len(req.Values) == 0 || len(req.Values) > whatIfMaxValues {
		return res, ErrWhatIfInput
	}
	ic, _, err := m.inputCell(req.InputCell)
	if err != nil {
		return res, err
	}
	for _, v := range req.Values {
		vals := m.eval(map[wiCell]float64{ic: v}, outs)
		row := make([]interface{}, len(vals))
		for j, x := range vals {
			row[j] = valueOut(x)
		}
		res.Results = append(res.Results, row)
	}
	return res, nil
}

// ---- scenarios ---------------------------------------------------------------

type Scenario struct {
	Name   string             `json:"name"`
	Values map[string]float64 `json:"values"`
}

// ScenarioRequest evaluates result cells under the current values and each scenario.
type ScenarioRequest struct {
	Data      string     `json:"data"`
	Sheet     string     `json:"sheet,omitempty"`
	Scenarios []Scenario `json:"scenarios"`
	Results   []string   `json:"results"`
}

type ScenarioRow struct {
	Name    string        `json:"name"`
	Results []interface{} `json:"results"`
}

// ScenarioResult: the first row is "Current values".
type ScenarioResult struct {
	Rows []ScenarioRow `json:"rows"`
}

func Scenarios(req ScenarioRequest) (ScenarioResult, error) {
	m, err := newWhatIfModel(req.Data, req.Sheet)
	if err != nil {
		return ScenarioResult{}, err
	}
	if len(req.Results) == 0 || len(req.Results) > 32 || len(req.Scenarios) > 64 {
		return ScenarioResult{}, ErrWhatIfTooMany
	}
	outs := make([]wiCell, len(req.Results))
	for i, f := range req.Results {
		if outs[i], err = m.cell(f); err != nil {
			return ScenarioResult{}, err
		}
	}
	row := func(name string, in map[wiCell]float64) ScenarioRow {
		vals := m.eval(in, outs)
		r := ScenarioRow{Name: name, Results: make([]interface{}, len(vals))}
		for i, v := range vals {
			r.Results[i] = valueOut(v)
		}
		return r
	}
	res := ScenarioResult{Rows: []ScenarioRow{row("Current values", nil)}}
	for _, sc := range req.Scenarios {
		if len(sc.Values) > 32 {
			return ScenarioResult{}, ErrWhatIfTooMany
		}
		in := map[wiCell]float64{}
		for ref, v := range sc.Values {
			c, _, err := m.inputCell(ref)
			if err != nil {
				return ScenarioResult{}, err
			}
			in[c] = v
		}
		res.Rows = append(res.Rows, row(sc.Name, in))
	}
	return res, nil
}

// ---- solver-lite ----------------------------------------------------------------

type SolverConstraint struct {
	Cell  string  `json:"cell"`
	Op    string  `json:"op"` // "<=", ">=", "="
	Value float64 `json:"value"`
}

type SolverVariable struct {
	Cell string   `json:"cell"`
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
}

type SolverRequest struct {
	Data        string             `json:"data"`
	Sheet       string             `json:"sheet,omitempty"`
	Objective   string             `json:"objective"`
	Goal        string             `json:"goal"` // "max", "min", "value"
	Target      float64            `json:"target,omitempty"`
	Variables   []SolverVariable   `json:"variables"`
	Constraints []SolverConstraint `json:"constraints,omitempty"`
}

type SolverResult struct {
	// Feasible is true when every constraint holds within 1e-6 (relative).
	Feasible  bool               `json:"feasible"`
	Objective interface{}        `json:"objective"`
	Values    map[string]float64 `json:"values"`
	// Violations lists constraints still broken at the returned point.
	Violations  []string `json:"violations,omitempty"`
	Evaluations int      `json:"evaluations"`
	Method      string   `json:"method"`
}

func Solve(req SolverRequest) (SolverResult, error) {
	m, err := newWhatIfModel(req.Data, req.Sheet)
	if err != nil {
		return SolverResult{}, err
	}
	if len(req.Variables) == 0 || len(req.Variables) > solverMaxVars || len(req.Constraints) > 64 {
		return SolverResult{}, ErrWhatIfTooMany
	}
	switch req.Goal {
	case "max", "min", "value":
	default:
		return SolverResult{}, ErrWhatIfInput
	}
	obj, err := m.cell(req.Objective)
	if err != nil {
		return SolverResult{}, err
	}
	if fc := findCell(m.wb[obj.sheet].CellData, obj.addr); fc == nil || !strings.HasPrefix(fc.F, "=") {
		return SolverResult{}, ErrGoalSeekFormula
	}
	n := len(req.Variables)
	vars := make([]wiCell, n)
	x0 := make([]float64, n)
	lo := make([]float64, n)
	hi := make([]float64, n)
	for i, v := range req.Variables {
		if vars[i], x0[i], err = m.inputCell(v.Cell); err != nil {
			return SolverResult{}, err
		}
		lo[i], hi[i] = math.Inf(-1), math.Inf(1)
		if v.Min != nil {
			lo[i] = *v.Min
		}
		if v.Max != nil {
			hi[i] = *v.Max
		}
		if lo[i] > hi[i] {
			return SolverResult{}, ErrWhatIfInput
		}
	}
	outs := []wiCell{obj}
	for _, c := range req.Constraints {
		if c.Op != "<=" && c.Op != ">=" && c.Op != "=" {
			return SolverResult{}, ErrWhatIfInput
		}
		cc, err := m.cell(c.Cell)
		if err != nil {
			return SolverResult{}, err
		}
		outs = append(outs, cc)
	}
	clamp := func(x []float64) []float64 {
		y := make([]float64, n)
		for i := range x {
			y[i] = math.Max(lo[i], math.Min(hi[i], x[i]))
		}
		return y
	}
	evals := 0
	measure := func(x []float64) (objV float64, viol float64, ok bool, vals []value) {
		evals++
		in := make(map[wiCell]float64, n)
		for i, c := range vars {
			in[c] = x[i]
		}
		vals = m.eval(in, outs)
		if vals[0].kind != kindNum && vals[0].kind != kindBool {
			return 0, 0, false, vals
		}
		objV = vals[0].num
		for k, c := range req.Constraints {
			v := vals[k+1]
			if v.kind != kindNum && v.kind != kindBool {
				viol += 1e6
				continue
			}
			d := 0.0
			switch c.Op {
			case "<=":
				d = math.Max(0, v.num-c.Value)
			case ">=":
				d = math.Max(0, c.Value-v.num)
			case "=":
				d = math.Abs(v.num - c.Value)
			}
			viol += d / math.Max(1, math.Abs(c.Value))
		}
		return objV, viol, true, vals
	}
	// Penalised cost: lower is better.
	weight := 1e3
	cost := func(x []float64) float64 {
		o, v, ok, _ := measure(clamp(x))
		if !ok {
			return math.Inf(1)
		}
		var c float64
		switch req.Goal {
		case "max":
			c = -o
		case "min":
			c = o
		default:
			c = math.Abs(o - req.Target)
		}
		return c + weight*math.Max(1, math.Abs(c))*v + weight*v
	}
	best := clamp(x0)
	for round := 0; round < 4 && evals < solverMaxEvals; round++ {
		best = clamp(nelderMead(cost, best, solverMaxEvals/4, &evals))
		weight *= 10
	}
	o, viol, ok, _ := measure(best)
	res := SolverResult{Values: map[string]float64{}, Evaluations: evals, Method: "Solver-lite: penalty Nelder-Mead (local search, no integer constraints)"}
	for i, v := range req.Variables {
		res.Values[strings.ToUpper(strings.TrimSpace(v.Cell))] = roundSig(best[i])
	}
	if ok {
		res.Objective = o
	}
	res.Feasible = ok && viol < 1e-6
	if !res.Feasible {
		_, _, _, vals := measure(best)
		for k, c := range req.Constraints {
			v := vals[k+1]
			holds := v.kind == kindNum && ((c.Op == "<=" && v.num <= c.Value+1e-6) || (c.Op == ">=" && v.num >= c.Value-1e-6) || (c.Op == "=" && math.Abs(v.num-c.Value) <= 1e-6*math.Max(1, math.Abs(c.Value))))
			if !holds {
				res.Violations = append(res.Violations, c.Cell+" "+c.Op)
			}
		}
	}
	return res, nil
}

// roundSig trims float noise (1999.9999999997 → 2000) to 10 significant digits.
func roundSig(x float64) float64 {
	if x == 0 || math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	p := math.Pow(10, 9-math.Floor(math.Log10(math.Abs(x))))
	return math.Round(x*p) / p
}

// nelderMead minimises f from x0 (Lagarias et al. standard coefficients).
func nelderMead(f func([]float64) float64, x0 []float64, maxEval int, evals *int) []float64 {
	n := len(x0)
	type pt struct {
		x []float64
		f float64
	}
	start := *evals
	simplex := make([]pt, n+1)
	simplex[0] = pt{append([]float64(nil), x0...), f(x0)}
	for i := 0; i < n; i++ {
		x := append([]float64(nil), x0...)
		step := 0.1 * math.Abs(x[i])
		if step < 1 {
			step = 1
		}
		x[i] += step
		simplex[i+1] = pt{x, f(x)}
	}
	for *evals-start < maxEval {
		sort.Slice(simplex, func(a, b int) bool { return simplex[a].f < simplex[b].f })
		spread := math.Abs(simplex[n].f - simplex[0].f)
		size := 0.0
		for i := 1; i <= n; i++ {
			for j := 0; j < n; j++ {
				size = math.Max(size, math.Abs(simplex[i].x[j]-simplex[0].x[j]))
			}
		}
		if spread <= 1e-12*math.Max(1, math.Abs(simplex[0].f)) && size < 1e-9 {
			break
		}
		cen := make([]float64, n)
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				cen[j] += simplex[i].x[j] / float64(n)
			}
		}
		at := func(t float64) []float64 {
			x := make([]float64, n)
			for j := range x {
				x[j] = cen[j] + t*(simplex[n].x[j]-cen[j])
			}
			return x
		}
		xr := at(-1)
		fr := f(xr)
		switch {
		case fr < simplex[0].f:
			xe := at(-2)
			if fe := f(xe); fe < fr {
				simplex[n] = pt{xe, fe}
			} else {
				simplex[n] = pt{xr, fr}
			}
		case fr < simplex[n-1].f:
			simplex[n] = pt{xr, fr}
		default:
			t := 0.5
			if fr < simplex[n].f {
				t = -0.5
			}
			xc := at(t)
			if fc := f(xc); fc < math.Min(fr, simplex[n].f) {
				simplex[n] = pt{xc, fc}
			} else {
				for i := 1; i <= n; i++ {
					for j := 0; j < n; j++ {
						simplex[i].x[j] = simplex[0].x[j] + 0.5*(simplex[i].x[j]-simplex[0].x[j])
					}
					simplex[i].f = f(simplex[i].x)
				}
			}
		}
	}
	sort.Slice(simplex, func(a, b int) bool { return simplex[a].f < simplex[b].f })
	return simplex[0].x
}
