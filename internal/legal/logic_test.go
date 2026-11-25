package legal

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	ok := map[string]int{"1.5": 90, "1,5": 90, "1:12": 72, "1h 12m": 72, "1h12": 72, "45m": 45, "45 min": 45, "2h": 120, "2 hrs": 120, ".3": 18, ".3h": 18, "0:30": 30, "1:29:45": 90, "90m": 90}
	for in, want := range ok {
		if got, good := ParseDuration(in); !good || got != want {
			t.Errorf("ParseDuration(%q) = %d,%v want %d", in, got, good, want)
		}
	}
	for _, bad := range []string{"", "abc", "1:75", "-2", "9999h", "1h 75mx"} {
		if _, good := ParseDuration(bad); good {
			t.Errorf("ParseDuration(%q) should fail", bad)
		}
	}
}

func TestMoneyMaths(t *testing.T) {
	if RoundUpMinutes(1, 6) != 6 || RoundUpMinutes(6, 6) != 6 || RoundUpMinutes(7, 6) != 12 || RoundUpMinutes(0, 6) != 0 || RoundUpMinutes(13, 15) != 15 {
		t.Error("RoundUpMinutes")
	}
	// 1.5 h at Rs 4,500/h = Rs 6,750.00; 7 min at Rs 5,000/h = 583.33; 1 min at 1.00/h = 1.67 paise -> 2.
	if TimeValue(90, 450000) != 675000 || TimeValue(7, 500000) != 58333 || TimeValue(1, 100) != 2 {
		t.Errorf("TimeValue: %d %d %d", TimeValue(90, 450000), TimeValue(7, 500000), TimeValue(1, 100))
	}
	// 2.5 x 120.50 = 301.25; 0.5 x 3.33 = 1.665 -> 1.67.
	if ExpenseValue(12050, 2500) != 30125 || ExpenseValue(333, 500) != 167 {
		t.Errorf("ExpenseValue %d %d", ExpenseValue(12050, 2500), ExpenseValue(333, 500))
	}
	if Rate(0, 0, 300000) != 300000 || Rate(0, 400000, 300000) != 400000 || Rate(500000, 400000, 300000) != 500000 {
		t.Error("rate precedence: matter, then client, then firm")
	}
	// Fees 100,000.00, expenses 5,000.00, 10% discount on fees, 18% tax on discounted fees:
	// discount 10,000; taxable 90,000; tax 16,200; total 100,000 + 5,000 - 10,000 + 16,200 = 111,200.
	lines := []Line{{Kind: "time", Amount: 10_000_000}, {Kind: "expense", Amount: 500_000}}
	bt := BillTotals(lines, 1000, 1800, false)
	if bt.Fees != 10_000_000 || bt.Discount != 1_000_000 || bt.Tax != 1_620_000 || bt.Total != 11_120_000 {
		t.Errorf("%+v", bt)
	}
	if bt := BillTotals(lines, 1000, 1800, true); bt.Tax != 1_710_000 { // taxable 95,000
		t.Errorf("tax on expenses %+v", bt)
	}
	// 3 lines of 33.33: fees 99.99, tax 17.9982 -> 18.00, total 117.99.
	three := []Line{{Kind: "time", Amount: 3333}, {Kind: "time", Amount: 3333}, {Kind: "time", Amount: 3333}}
	if bt := BillTotals(three, 0, 1800, false); bt.Tax != 1800 || bt.Total != 11799 {
		t.Errorf("%+v", bt)
	}
}

func TestNumbersAndWorkflow(t *testing.T) {
	now := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	if DisplayNumber(12, "number", "", "", now) != "00012" || DisplayNumber(12, "year-number", "", "", now) != "2026-00012" || DisplayNumber(12, "year-number", "", "2025-03-01", now) != "2025-00012" {
		t.Error("number formats")
	}
	if DisplayNumber(12, "number-client", "Maria Lopez", "", now) != "00012-Lopez" || DisplayNumber(12, "number-client", "", "", now) != "00012" {
		t.Error("number-client")
	}
	if !CanMove("intake", "open") || CanMove("intake", "pending") || !CanMove("closed", "open") || CanMove("closed", "pending") || !CanMove("on_hold", "pending") {
		t.Error("status workflow")
	}
}

func TestConflictScoring(t *testing.T) {
	c := Candidate{Name: "José Álvarez Pérez", Extra: []string{"jose@example.in", "+91 98765 43210"}}
	for q, want := range map[string]int{"jose alvarez perez": 100, "José": 70, "perez alvarez": 60, "perez, jose": 60, "98765-43210": 40, "9876543210": 40, "example.in": 40, "xyz": 0, "j": 0} {
		if got := ConflictScore(q, c); got != want {
			t.Errorf("ConflictScore(%q) = %d, want %d", q, got, want)
		}
	}
}
