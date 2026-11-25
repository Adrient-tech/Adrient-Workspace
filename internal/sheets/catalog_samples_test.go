package sheets

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// catalogSamples is written by web/app/scripts/dump-fn-cases.ts: one sample
// formula per function in the Insert Function catalog, plus the fixture cells
// they refer to. The browser probe (scripts/sheet-fn-probe.cjs) reads the same
// file, so the engine and the editor are judged on identical input.
type catalogSamples struct {
	Cases []struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Formula  string `json:"formula"`
	} `json:"cases"`
	Fixture    [][]interface{} `json:"fixture"`
	FormulaCol int             `json:"formulaCol"`
}

func loadCatalogSamples(t *testing.T) catalogSamples {
	t.Helper()
	path := os.Getenv("SHEET_FN_CASES")
	if path == "" {
		path = "testdata/function_samples.json"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no sample file: %v", err)
	}
	var s catalogSamples
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s catalogSamples) workbook(formula string) string {
	var cells []map[string]interface{}
	for _, f := range s.Fixture {
		v := map[string]interface{}{"v": f[2]}
		if str, ok := f[2].(string); ok && strings.HasPrefix(str, "=") {
			v = map[string]interface{}{"f": str}
		}
		cells = append(cells, map[string]interface{}{"r": f[0], "c": f[1], "v": v})
	}
	if formula != "" {
		cells = append(cells, map[string]interface{}{"r": 0, "c": s.FormulaCol, "v": map[string]interface{}{"f": "=" + formula}})
	}
	b, _ := json.Marshal([]interface{}{map[string]interface{}{"name": "Sheet1", "id": "sheet1", "celldata": cells}})
	return string(b)
}

// catalogKnownErrors are samples whose correct result is an error value (the
// sample only shows the call shape) or that depend on the outside world.
var catalogKnownErrors = map[string]string{
	"NA":           "returns #N/A by definition",
	"GETPIVOTDATA": "needs a pivot table on the sheet; #REF! is the right answer without one",
	"STOCKHISTORY": "needs a data feed",
}

// TestCatalogSamplesEvaluate runs every sample alone through RecalcWorkbook
// and fails when the engine answers with an error value or nothing.
func TestCatalogSamplesEvaluate(t *testing.T) {
	s := loadCatalogSamples(t)
	out := map[string]map[string]interface{}{}
	for _, c := range s.Cases {
		cells, err := RecalcWorkbook(s.workbook(c.Formula))
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		var top *RecalcCell
		for i := range cells {
			if cells[i].R == 0 && cells[i].C == s.FormulaCol && !cells[i].Spill {
				top = &cells[i]
			}
		}
		if top == nil {
			t.Errorf("%s: %s gave no value", c.Name, c.Formula)
			continue
		}
		out[c.Name] = map[string]interface{}{"formula": c.Formula, "m": top.M, "v": top.V}
		if _, ok := catalogKnownErrors[c.Name]; ok {
			continue
		}
		if strings.HasPrefix(top.M, "#") {
			t.Errorf("%s: %s = %s", c.Name, c.Formula, top.M)
		}
	}
	if p := os.Getenv("SHEET_FN_OUT"); p != "" {
		b, _ := json.MarshalIndent(out, "", " ")
		_ = os.WriteFile(p, b, 0o644)
	}
}
