package payrollhours

import (
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
)

func TestMergeReplacesOwnLineOnly(t *testing.T) {
	ex := []hr.Adjustment{{Kind: "deduction", Name: "Advance", Amount: 500}, {Kind: "earning", Name: "Hours: shifts (10.00 h)", Amount: 1000}, {Kind: "earning", Name: "Hours: time tracking (3.00 h)", Amount: 300}}
	got := Merge(ex, "shifts", 12.5, 1250.456)
	if len(got) != 3 {
		t.Fatalf("want 3 adjustments, got %d: %+v", len(got), got)
	}
	if got[2].Name != "Hours: shifts (12.50 h)" || got[2].Amount != 1250.46 {
		t.Fatalf("bad new line: %+v", got[2])
	}
	if got[0].Name != "Advance" || got[1].Name != "Hours: time tracking (3.00 h)" {
		t.Fatalf("other lines changed: %+v", got)
	}
}

func TestMergeZeroRemoves(t *testing.T) {
	ex := []hr.Adjustment{{Kind: "earning", Name: "Hours: shifts (1.00 h)", Amount: 100}}
	if got := Merge(ex, "shifts", 0, 0); len(got) != 0 {
		t.Fatalf("zero amount must remove the line: %+v", got)
	}
}
