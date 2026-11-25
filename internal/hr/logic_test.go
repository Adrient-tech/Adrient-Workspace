package hr

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/pdf/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func d(s string) time.Time { return parseDate(s) }

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.005 {
		t.Errorf("%s = %.2f, want %.2f", name, got, want)
	}
}

// ---- leave ----

func TestLeaveYear(t *testing.T) {
	if LeaveYearOf(d("2026-03-31"), 4) != 2025 || LeaveYearOf(d("2026-04-01"), 4) != 2026 || LeaveYearOf(d("2026-12-31"), 1) != 2026 {
		t.Fatal("leave year labels wrong")
	}
	f, e := LeaveYearBounds(2025, 4)
	if fmtDate(f) != "2025-04-01" || fmtDate(e) != "2026-03-31" {
		t.Fatalf("bounds %s %s", fmtDate(f), fmtDate(e))
	}
}

func TestAccrualPeriods(t *testing.T) {
	// Joined on the 10th of March: accrues from March. Joined on the 20th: from April.
	p := AccrualPeriods(2026, 1, d("2026-03-10"), nil, d("2026-06-05"))
	if strings.Join(p, ",") != "2026-03,2026-04,2026-05,2026-06" {
		t.Fatalf("got %v", p)
	}
	p = AccrualPeriods(2026, 1, d("2026-03-20"), nil, d("2026-05-31"))
	if strings.Join(p, ",") != "2026-04,2026-05" {
		t.Fatalf("late joiner %v", p)
	}
	// Whole earlier history before the leave year starts is not in this year.
	p = AccrualPeriods(2026, 4, d("2020-01-01"), nil, d("2026-05-01"))
	if strings.Join(p, ",") != "2026-04,2026-05" {
		t.Fatalf("april year %v", p)
	}
	// Stops after the exit month.
	x := d("2026-02-15")
	p = AccrualPeriods(2026, 1, d("2025-01-01"), &x, d("2026-12-01"))
	if len(p) != 2 || p[1] != "2026-02" {
		t.Fatalf("exit %v", p)
	}
	// Never beyond the end of the leave year.
	if p = AccrualPeriods(2025, 1, d("2020-01-01"), nil, d("2026-08-01")); len(p) != 12 {
		t.Fatalf("year cap %d", len(p))
	}
}

func TestAccrualAmounts(t *testing.T) {
	near(t, "monthly 15", MonthlyAccrual(15), 1.25)
	near(t, "monthly 12", MonthlyAccrual(12), 1)
	near(t, "annual full", AnnualGrant(6, 2026, 1, d("2025-05-05")), 6)
	near(t, "annual joined Jul 1", AnnualGrant(12, 2026, 1, d("2026-07-01")), 6)
	near(t, "annual joined Jul 20 (from Aug)", AnnualGrant(12, 2026, 1, d("2026-07-20")), 5)
	max := 10.0
	near(t, "cap room", CapAccrual(9.5, 1.25, &max), 0.5)
	near(t, "cap none", CapAccrual(11, 1, &max), 0)
	near(t, "no cap", CapAccrual(100, 1, nil), 1)
}

func TestCarryForward(t *testing.T) {
	c, l := CarryForward(18, 10, true)
	near(t, "carry", c, 10)
	near(t, "lapse", l, 8)
	c, l = CarryForward(4, 10, true)
	near(t, "carry small", c, 4)
	near(t, "lapse small", l, 0)
	c, l = CarryForward(7, 10, false)
	near(t, "no carry", c, 0)
	near(t, "all lapse", l, 7)
	c, l = CarryForward(-2, 10, true)
	if c != 0 || l != 0 {
		t.Fatal("negative balance carries nothing")
	}
}

func TestCountLeaveDays(t *testing.T) {
	sun := map[int]bool{0: true}
	hol := map[string]bool{"2026-08-15": true}
	// Mon 2026-08-10 .. Sun 2026-08-16: Mon-Sat = 6 days, minus Sat 15th holiday = 5.
	n, err := CountLeaveDays(d("2026-08-10"), d("2026-08-16"), "", sun, hol)
	if err != nil || n != 5 {
		t.Fatalf("n=%v err=%v", n, err)
	}
	if n, _ := CountLeaveDays(d("2026-08-10"), d("2026-08-10"), "first", sun, hol); n != 0.5 {
		t.Fatalf("half day %v", n)
	}
	if _, err := CountLeaveDays(d("2026-08-10"), d("2026-08-11"), "first", sun, hol); err == nil {
		t.Fatal("half day over two dates must fail")
	}
	if _, err := CountLeaveDays(d("2026-08-16"), d("2026-08-16"), "", sun, hol); err == nil {
		t.Fatal("a Sunday only range must fail")
	}
	if _, err := CountLeaveDays(d("2026-08-11"), d("2026-08-10"), "", sun, hol); err == nil {
		t.Fatal("reversed range must fail")
	}
}

func TestLeavesOverlap(t *testing.T) {
	if !leavesOverlap(d("2026-01-01"), d("2026-01-03"), "", d("2026-01-03"), d("2026-01-04"), "") {
		t.Error("touching ranges overlap")
	}
	if leavesOverlap(d("2026-01-01"), d("2026-01-01"), "first", d("2026-01-01"), d("2026-01-01"), "second") {
		t.Error("opposite halves must not overlap")
	}
	if !leavesOverlap(d("2026-01-01"), d("2026-01-01"), "first", d("2026-01-01"), d("2026-01-01"), "first") {
		t.Error("same half overlaps")
	}
}

// ---- attendance ----

func at(day, hm string) time.Time {
	t, _ := time.Parse("2006-01-02 15:04", day+" "+hm)
	return t
}

func TestClassifyDay(t *testing.T) {
	base := DayInput{Date: d("2026-06-10"), Employed: true, Shift: DefaultShift, Today: d("2026-06-30"), OTEnabled: true}
	cases := []struct {
		name   string
		mod    func(*DayInput)
		status string
		lop    float64
		late   bool
		ot     int
	}{
		{"full day on time", func(i *DayInput) { i.Punches = []time.Time{at("2026-06-10", "09:25"), at("2026-06-10", "18:35")} }, DayPresent, 0, false, 10},
		{"late arrival", func(i *DayInput) { i.Punches = []time.Time{at("2026-06-10", "10:05"), at("2026-06-10", "18:35")} }, DayPresent, 0, true, 0},
		{"within grace", func(i *DayInput) { i.Punches = []time.Time{at("2026-06-10", "09:40"), at("2026-06-10", "18:30")} }, DayPresent, 0, false, 0},
		{"half day", func(i *DayInput) { i.Punches = []time.Time{at("2026-06-10", "09:30"), at("2026-06-10", "14:00")} }, DayHalf, 0.5, false, 0},
		{"too short is absent", func(i *DayInput) { i.Punches = []time.Time{at("2026-06-10", "09:30"), at("2026-06-10", "10:30")} }, DayAbsent, 1, false, 0},
		{"no punches in the past", func(i *DayInput) {}, DayAbsent, 1, false, 0},
		{"single punch in the past", func(i *DayInput) { i.Punches = []time.Time{at("2026-06-10", "09:30")} }, DayMissingOut, 1, false, 0},
		{"overtime", func(i *DayInput) { i.Punches = []time.Time{at("2026-06-10", "09:00"), at("2026-06-10", "20:00")} }, DayPresent, 0, false, 120},
		{"weekly off", func(i *DayInput) { i.WeeklyOff = true }, DayWeeklyOff, 0, false, 0},
		{"holiday", func(i *DayInput) { i.Holiday = "Diwali" }, DayHoliday, 0, false, 0},
		{"full leave", func(i *DayInput) { i.Leave = "full" }, DayLeave, 0, false, 0},
		{"half leave + half day worked", func(i *DayInput) {
			i.Leave = "first"
			i.Punches = []time.Time{at("2026-06-10", "13:30"), at("2026-06-10", "18:30")}
		}, DayPresent, 0, true, 0},
		{"half leave + nothing else", func(i *DayInput) { i.Leave = "second" }, DayAbsent, 0.5, false, 0},
		{"future day", func(i *DayInput) { i.Date = d("2026-07-10") }, DayUpcoming, 0, false, 0},
		{"before joining", func(i *DayInput) { i.Employed = false }, DayNotEmployed, 0, false, 0},
		{"today with no punch yet", func(i *DayInput) { i.Today = d("2026-06-10") }, DayInProgress, 0, false, 0},
		{"today with one punch", func(i *DayInput) {
			i.Today = d("2026-06-10")
			i.Punches = []time.Time{at("2026-06-10", "09:30")}
		}, DayInProgress, 0, false, 0},
	}
	for _, c := range cases {
		in := base
		c.mod(&in)
		got := ClassifyDay(in)
		if got.Status != c.status || got.LOP != c.lop || got.Late != c.late || got.OTMin != c.ot {
			t.Errorf("%s: got status=%s lop=%v late=%v ot=%d, want %s %v %v %d", c.name, got.Status, got.LOP, got.Late, got.OTMin, c.status, c.lop, c.late, c.ot)
		}
	}
}

func TestOTDisabledAndEarlyOut(t *testing.T) {
	in := DayInput{Date: d("2026-06-10"), Employed: true, Shift: DefaultShift, Today: d("2026-06-30"),
		Punches: []time.Time{at("2026-06-10", "09:00"), at("2026-06-10", "20:00")}}
	if r := ClassifyDay(in); r.OTMin != 0 {
		t.Error("overtime must be zero when disabled")
	}
	in.Punches = []time.Time{at("2026-06-10", "09:30"), at("2026-06-10", "16:00")}
	if r := ClassifyDay(in); !r.EarlyOut || r.Status != DayHalf {
		t.Errorf("early out day: %+v", r)
	}
}

func TestSummarise(t *testing.T) {
	s := Summarise([]DayResult{{Status: DayPresent}, {Status: DayHalf, LOP: .5, Late: true}, {Status: DayAbsent, LOP: 1}, {Status: DayLeave}, {Status: DayHoliday}, {Status: DayWeeklyOff}})
	if s.Present != 1.5 || s.HalfDays != 1 || s.Absent != 1 || s.Leave != 1 || s.LateCount != 1 || s.LOP != 1.5 {
		t.Fatalf("%+v", s)
	}
}

func TestHaversine(t *testing.T) {
	// Roughly 111 km per degree of latitude.
	if m := HaversineMetres(12.9716, 77.5946, 13.9716, 77.5946); math.Abs(m-111195) > 600 {
		t.Fatalf("1 degree = %.0f m", m)
	}
	if HaversineMetres(18.5204, 73.8567, 18.5204, 73.8567) != 0 {
		t.Fatal("same point is zero")
	}
	if m := HaversineMetres(18.5204, 73.8567, 18.5214, 73.8567); m < 100 || m > 125 {
		t.Fatalf("~111 m expected, got %.0f", m)
	}
}

func TestParseLogTime(t *testing.T) {
	ist := time.FixedZone("IST", 5*3600+1800)
	for _, s := range []string{"2026-06-10 09:31:05", "10/06/2026 09:31", "2026-06-10T09:31:05+05:30", "2026-06-10 09:31"} {
		tm, err := ParseLogTime(s, ist)
		if err != nil || tm.In(ist).Format("15:04") != "09:31" {
			t.Errorf("%q -> %v %v", s, tm, err)
		}
	}
	if _, err := ParseLogTime("yesterday", ist); err == nil {
		t.Error("garbage must fail")
	}
}

// ---- payroll ----

func stdLines(t *testing.T, annual float64) []SalaryLine {
	t.Helper()
	l, err := ExpandStructure(annual, DefaultStructure)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestExpandStructure(t *testing.T) {
	l := stdLines(t, 600000) // 50,000 / month
	m := map[string]float64{}
	var sum float64
	for _, x := range l {
		m[x.Code] = x.Amount
		sum += x.Amount
	}
	near(t, "basic", m["BASIC"], 20000)
	near(t, "hra", m["HRA"], 10000)
	near(t, "special", m["SPECIAL"], 20000)
	near(t, "sum", sum, 50000)
	// Fixed parts above the gross are refused.
	_, err := ExpandStructure(60000, []SalaryLine{{Code: "BASIC", Name: "Basic", Kind: "earning", Calc: CalcPctGross, Value: 50}, {Code: "X", Name: "X", Kind: "earning", Calc: CalcFixed, Value: 9000}, {Code: "S", Name: "S", Kind: "earning", Calc: CalcBalance}})
	if err == nil {
		t.Error("overspent structure must fail")
	}
	// Earnings that do not reach the gross need a balancing line.
	if _, err := ExpandStructure(600000, []SalaryLine{{Code: "BASIC", Name: "Basic", Kind: "earning", Calc: CalcPctGross, Value: 40}}); err == nil {
		t.Error("underspent structure without a balance line must fail")
	}
	if _, err := ExpandStructure(600000, []SalaryLine{{Code: "HRA", Name: "H", Kind: "earning", Calc: CalcFixed, Value: 1}}); err == nil {
		t.Error("BASIC is required")
	}
}

func TestPF(t *testing.T) {
	ee, er := PF(20000, true, 15000)
	near(t, "capped ee", ee, 1800)
	near(t, "capped er", er, 1800)
	ee, _ = PF(20000, false, 15000)
	near(t, "uncapped", ee, 2400)
	ee, _ = PF(12345, true, 15000)
	near(t, "rounded", ee, 1481) // 1481.4
	if ee, er = PF(0, true, 15000); ee != 0 || er != 0 {
		t.Error("no wages no PF")
	}
}

func TestESI(t *testing.T) {
	ee, er := ESI(20000, 20000, 21000)
	near(t, "ee", ee, 150) // 0.75% of 20000
	near(t, "er", er, 650) // 3.25%
	ee, er = ESI(21001, 21001, 21000)
	if ee != 0 || er != 0 {
		t.Error("above the limit means no ESI")
	}
	ee, _ = ESI(18000, 17333, 21000) // rounds up: 129.9975 -> 130
	near(t, "round up", ee, 130)
}

func TestProfessionalTax(t *testing.T) {
	ptSlabs := DefaultPTSlabs
	near(t, "mh low", ProfessionalTax(ptSlabs, "Maharashtra", 7000, 5), 0)
	near(t, "mh mid", ProfessionalTax(ptSlabs, "Maharashtra", 9000, 5), 175)
	near(t, "mh high", ProfessionalTax(ptSlabs, "Maharashtra", 50000, 5), 200)
	near(t, "mh february", ProfessionalTax(ptSlabs, "Maharashtra", 50000, 2), 300)
	near(t, "ka", ProfessionalTax(ptSlabs, "karnataka", 30000, 1), 200)
	near(t, "ka below", ProfessionalTax(ptSlabs, "Karnataka", 20000, 1), 0)
	near(t, "unknown state", ProfessionalTax(ptSlabs, "Goa", 90000, 1), 0)
}

func TestNewRegimeTax(t *testing.T) {
	near(t, "up to 12L is nil", NewRegimeTax(1200000), 0)
	near(t, "4L", NewRegimeTax(400000), 0)
	// 15L taxable: 0-4 nil, 4-8 5% = 20000, 8-12 10% = 40000, 12-15 15% = 45000 => 105000 + 4% = 109200
	near(t, "15L", NewRegimeTax(1500000), 109200)
	// Marginal relief just above 12L: tax = excess over 12L (+cess).
	near(t, "12.1L marginal relief", NewRegimeTax(1210000), math.Round(10000*1.04))
	near(t, "monthly TDS for 50k/month", MonthlyTDS(50000), 0)
	if MonthlyTDS(200000) <= 0 {
		t.Error("high earners pay TDS")
	}
}

func TestComputePayslip(t *testing.T) {
	lines := stdLines(t, 600000) // 50k gross, basic 20k
	slabs := DefaultPTSlabs
	in := PayrollInput{Lines: lines, DaysInMonth: 30, PFEnabled: true, ESIEnabled: true, PTEnabled: true, PFCapped: true, PFCeiling: 15000, ESILimit: 21000,
		State: "Maharashtra", Month: 6, PTSlabs: slabs}
	r, err := ComputePayslip(in)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "gross", r.Gross, 50000)
	near(t, "pf ee (capped 15000)", r.PFEmployee, 1800)
	near(t, "esi none above limit", r.ESIEmployee, 0)
	near(t, "pt", r.ProfTax, 200)
	near(t, "net", r.Net, 50000-1800-200)
	near(t, "paid days", r.PaidDays, 30)

	// 3 days loss of pay out of 30: everything prorates by 27/30.
	in.LOPDays = 3
	r, _ = ComputePayslip(in)
	near(t, "lop gross", r.Gross, 45000)
	near(t, "lop paid days", r.PaidDays, 27)
	near(t, "lop pf (18000 wage capped at 15000)", r.PFEmployee, 1800)

	// Joined mid month: 10 days outside employment.
	in.LOPDays, in.OutsideDays = 0, 10
	r, _ = ComputePayslip(in)
	near(t, "partial month gross", r.Gross, 50000*20/30.0)

	// Low salary: ESI applies and PF is on the actual basic.
	low := stdLines(t, 240000) // 20,000 gross, basic 8000
	r, _ = ComputePayslip(PayrollInput{Lines: low, DaysInMonth: 31, PFEnabled: true, ESIEnabled: true, PFCapped: true, PFCeiling: 15000, ESILimit: 21000})
	near(t, "low pf", r.PFEmployee, 960)
	near(t, "low esi ee", r.ESIEmployee, 150)
	near(t, "low esi er", r.ESIEmployer, 650)
	near(t, "low net", r.Net, 20000-960-150)

	// Adjustments: bonus is earned, deduction taken, reimbursement added to net only.
	in = PayrollInput{Lines: lines, DaysInMonth: 30, Adjustments: []Adjustment{{Kind: "earning", Name: "Bonus", Amount: 5000}, {Kind: "deduction", Name: "Advance", Amount: 2000}, {Kind: "reimbursement", Name: "Travel", Amount: 1500}}}
	r, _ = ComputePayslip(in)
	near(t, "adj gross", r.Gross, 55000)
	near(t, "adj other", r.OtherDed, 2000)
	near(t, "adj reimb", r.Reimburse, 1500)
	near(t, "adj net", r.Net, 55000-2000+1500)

	// All pay lost: net must not go negative silently.
	in = PayrollInput{Lines: lines, DaysInMonth: 30, LOPDays: 30, Adjustments: []Adjustment{{Kind: "deduction", Name: "Advance", Amount: 2000}}}
	if _, err := ComputePayslip(in); err == nil {
		t.Error("negative net must be an error")
	}
}

func TestJournalBalances(t *testing.T) {
	tot := RunTotals{Employees: 2, Gross: 100000, PFEE: 3600, PFER: 3600, ESIEE: 150, ESIER: 650, PT: 400, TDS: 1000, Other: 2000, Reimb: 1500}
	tot.Net = tot.Gross - tot.PFEE - tot.ESIEE - tot.PT - tot.TDS - tot.Other + tot.Reimb
	j := tot.BuildJournal("run", "2026-06", "2026-06-30")
	near(t, "balance", j.Balanced(), 0)
	near(t, "employer", j.EmployerContrib, 4250)
	near(t, "pf payable", j.PFPayable, 7200)
}

func TestAmountInWords(t *testing.T) {
	cases := map[float64]string{0: "Zero Rupees Only", 1: "One Rupees Only", 48750: "Forty Eight Thousand Seven Hundred Fifty Rupees Only",
		125000: "One Lakh Twenty Five Thousand Rupees Only", 10000000: "One Crore Rupees Only", 1050.50: "One Thousand Fifty Rupees and Fifty Paise Only"}
	for in, want := range cases {
		if got := AmountInWords(in); got != want {
			t.Errorf("%v: %q want %q", in, got, want)
		}
	}
}

func TestPayslipPDFIsValid(t *testing.T) {
	p := Payslip{Period: "2026-06", EmpCode: "EMP0001", EmpName: "Asha Rao (test) \\ ₹", Designation: "Engineer", Department: "Tech", PAN: "ABCDE1234F", BankName: "HDFC", BankAccount: "123456789012",
		BankIFSC: "HDFC0001234", DaysInMonth: 30, LOPDays: 1.5, PaidDays: 28.5, Earnings: []Amt{{"BASIC", "Basic", 20000}, {"HRA", "HRA", 10000}}, Gross: 30000, PFEmployee: 1800, ProfTax: 200, Net: 28000}
	b := RenderPayslipPDF(p, "Adrient Labs", "Pune")
	if !bytes.HasPrefix(b, []byte("%PDF-1.4")) || !bytes.HasSuffix(bytes.TrimSpace(b), []byte("%%EOF")) {
		t.Fatal("not a PDF")
	}
	g := pdf.New()
	if err := g.ValidatePDF(bytes.NewReader(b)); err != nil {
		t.Fatalf("pdfcpu rejects the payslip: %v", err)
	}
	if n, err := api.PageCount(bytes.NewReader(b), model.NewDefaultConfiguration()); err != nil || n != 1 {
		t.Fatalf("pages %d %v", n, err)
	}
}

func TestNextOccurrenceAndAttrition(t *testing.T) {
	today := d("2026-11-18")
	if got := fmtDate(NextOccurrence(d("1990-11-20"), today)); got != "2026-11-20" {
		t.Error(got)
	}
	if got := fmtDate(NextOccurrence(d("1990-11-10"), today)); got != "2027-11-10" {
		t.Error(got)
	}
	if got := fmtDate(NextOccurrence(d("1992-02-29"), d("2026-11-18"))); got != "2027-02-28" {
		t.Errorf("leap day %s", got)
	}
	near(t, "attrition", AttritionPct(6, 40, 50), 13.33)
	near(t, "attrition zero", AttritionPct(1, 0, 0), 0)
}

func TestCSVSafe(t *testing.T) {
	for _, s := range []string{"=1+1", "+x", "-x", "@SUM(A1)"} {
		if !strings.HasPrefix(csvSafe(s), "'") {
			t.Errorf("%q not defused", s)
		}
	}
	if csvSafe("Asha") != "Asha" {
		t.Error("plain text unchanged")
	}
}

func TestValidateLines(t *testing.T) {
	if err := ValidateLines([]SalaryLine{{Code: "BASIC", Name: "B", Kind: "earning", Calc: CalcFixed, Amount: 1}, {Code: "BASIC", Name: "B2", Kind: "earning", Calc: CalcFixed}}, false); err == nil {
		t.Error("duplicate code")
	}
	if err := ValidateLines([]SalaryLine{{Code: "X", Name: "X", Kind: "deduction", PFWage: true}}, false); err == nil {
		t.Error("deduction cannot be PF wage")
	}
}
