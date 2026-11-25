package sheets

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func wbJSON(t *testing.T, cells map[string]interface{}) string {
	t.Helper()
	b, err := json.Marshal(FsWorkbook{sheetFromCells(cells)})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func near(t *testing.T, got interface{}, want, tol float64) {
	t.Helper()
	n, ok := got.(float64)
	if !ok || math.Abs(n-want) > tol {
		t.Fatalf("got %v, want %g ± %g", got, want, tol)
	}
}

func TestDataTableOneVariable(t *testing.T) {
	data := wbJSON(t, map[string]interface{}{"A1": 0.05, "B1": 1000.0, "C1": "=B1*(1+A1)", "D1": "=C1-B1"})
	res, err := DataTable(DataTableRequest{Data: data, Formulas: []string{"C1", "D1"}, InputCell: "A1", Values: []float64{0, 0.1, 0.2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 3 || len(res.Results[0]) != 2 {
		t.Fatalf("shape %v", res.Results)
	}
	near(t, res.Results[0][0], 1000, 1e-9)
	near(t, res.Results[1][0], 1100, 1e-9)
	near(t, res.Results[2][1], 200, 1e-9)
}

func TestDataTableTwoVariables(t *testing.T) {
	data := wbJSON(t, map[string]interface{}{"A1": 1.0, "B1": 1.0, "C1": "=A1*B1"})
	res, err := DataTable(DataTableRequest{Data: data, Formulas: []string{"C1"}, RowInput: "A1", ColInput: "B1", RowValues: []float64{1, 2, 3}, ColValues: []float64{10, 20}})
	if err != nil {
		t.Fatal(err)
	}
	near(t, res.Results[2][1], 60, 1e-9)
	near(t, res.Results[0][0], 10, 1e-9)
}

func TestDataTableRejectsFormulaInput(t *testing.T) {
	data := wbJSON(t, map[string]interface{}{"A1": "=1", "C1": "=A1*2"})
	if _, err := DataTable(DataTableRequest{Data: data, Formulas: []string{"C1"}, InputCell: "A1", Values: []float64{1}}); !errors.Is(err, ErrGoalSeekChanging) {
		t.Fatalf("err = %v", err)
	}
	if _, err := DataTable(DataTableRequest{Data: data, Formulas: []string{"C1"}, InputCell: "A1", Values: make([]float64, 1000)}); err == nil {
		t.Fatal("expected a limit error")
	}
	if _, err := DataTable(DataTableRequest{Data: "nope", Formulas: []string{"C1"}}); !errors.Is(err, ErrNotWorkbook) {
		t.Fatalf("err = %v", err)
	}
}

func TestScenarios(t *testing.T) {
	data := wbJSON(t, map[string]interface{}{"A1": 100.0, "A2": 0.2, "B1": "=A1*(1-A2)"})
	res, err := Scenarios(ScenarioRequest{
		Data:    data,
		Results: []string{"B1"},
		Scenarios: []Scenario{
			{Name: "Best", Values: map[string]float64{"A1": 200, "A2": 0.1}},
			{Name: "Worst", Values: map[string]float64{"A1": 50}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 3 || res.Rows[0].Name != "Current values" {
		t.Fatalf("rows %+v", res.Rows)
	}
	near(t, res.Rows[0].Results[0], 80, 1e-9)
	near(t, res.Rows[1].Results[0], 180, 1e-9)
	near(t, res.Rows[2].Results[0], 40, 1e-9)
}

func TestSolverMinimiseQuadratic(t *testing.T) {
	// (x-3)^2 + (y+1)^2 is smallest at (3, -1).
	data := wbJSON(t, map[string]interface{}{"A1": 0.0, "A2": 0.0, "B1": "=(A1-3)^2+(A2+1)^2"})
	res, err := Solve(SolverRequest{Data: data, Objective: "B1", Goal: "min", Variables: []SolverVariable{{Cell: "A1"}, {Cell: "A2"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Feasible || math.Abs(res.Values["A1"]-3) > 1e-3 || math.Abs(res.Values["A2"]+1) > 1e-3 {
		t.Fatalf("res %+v", res)
	}
	if res.Method == "" {
		t.Fatal("method must be labelled")
	}
}

func TestSolverLinearProgramWithConstraints(t *testing.T) {
	// maximise 3x + 2y s.t. x + y <= 4, x + 3y <= 6, x,y >= 0 → x=4, y=0, 12.
	zero := 0.0
	data := wbJSON(t, map[string]interface{}{"A1": 1.0, "A2": 1.0, "B1": "=3*A1+2*A2", "B2": "=A1+A2", "B3": "=A1+3*A2"})
	res, err := Solve(SolverRequest{
		Data: data, Objective: "B1", Goal: "max",
		Variables:   []SolverVariable{{Cell: "A1", Min: &zero}, {Cell: "A2", Min: &zero}},
		Constraints: []SolverConstraint{{Cell: "B2", Op: "<=", Value: 4}, {Cell: "B3", Op: "<=", Value: 6}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Feasible {
		t.Fatalf("infeasible: %+v", res)
	}
	near(t, res.Objective, 12, 0.01)
}

func TestSolverReachValue(t *testing.T) {
	data := wbJSON(t, map[string]interface{}{"A1": 1.0, "B1": "=A1^3"})
	res, err := Solve(SolverRequest{Data: data, Objective: "B1", Goal: "value", Target: 27, Variables: []SolverVariable{{Cell: "A1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Values["A1"]-3) > 1e-3 {
		t.Fatalf("res %+v", res)
	}
}

func TestSolverReportsInfeasible(t *testing.T) {
	data := wbJSON(t, map[string]interface{}{"A1": 1.0, "B1": "=A1", "B2": "=A1"})
	res, err := Solve(SolverRequest{
		Data: data, Objective: "B1", Goal: "max", Variables: []SolverVariable{{Cell: "A1"}},
		Constraints: []SolverConstraint{{Cell: "B2", Op: ">=", Value: 10}, {Cell: "B2", Op: "<=", Value: 5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Feasible || len(res.Violations) == 0 {
		t.Fatalf("expected infeasible, got %+v", res)
	}
}

func TestSolverValidation(t *testing.T) {
	data := wbJSON(t, map[string]interface{}{"A1": 1.0, "B1": "=A1"})
	cases := []SolverRequest{
		{Data: data, Objective: "B1", Goal: "bogus", Variables: []SolverVariable{{Cell: "A1"}}},
		{Data: data, Objective: "A1", Goal: "max", Variables: []SolverVariable{{Cell: "A1"}}},
		{Data: data, Objective: "B1", Goal: "max"},
		{Data: data, Objective: "B1", Goal: "max", Variables: []SolverVariable{{Cell: "B1"}}},
		{Data: data, Objective: "B1", Goal: "max", Variables: []SolverVariable{{Cell: "A1"}}, Constraints: []SolverConstraint{{Cell: "A1", Op: "!="}}},
	}
	for i, c := range cases {
		if _, err := Solve(c); err == nil {
			t.Fatalf("case %d: expected an error", i)
		}
	}
}
