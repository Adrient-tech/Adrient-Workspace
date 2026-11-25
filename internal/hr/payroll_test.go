package hr

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestAttendanceFlow(t *testing.T) {
	f := newFixture(t)
	hrA := f.team()
	asha, meera, ravi := f.actor("Asha", false), f.actor("Meera", false), f.actor("Ravi", false)
	ist := f.clock.Location()

	*f.clock = time.Date(2026, 6, 15, 9, 20, 0, 0, ist)
	s, err := f.repo.CheckInOut(f.ctx, asha, nil, nil)
	if err != nil || !s.CheckedIn || len(s.Punches) != 1 {
		t.Fatalf("check in: %+v %v", s, err)
	}
	_, err = f.repo.CheckInOut(f.ctx, asha, nil, nil)
	wantErr(t, "debounce", err, ErrConflict)
	*f.clock = time.Date(2026, 6, 15, 18, 40, 0, 0, ist)
	s, err = f.repo.CheckInOut(f.ctx, asha, nil, nil)
	if err != nil || s.CheckedIn || s.Day.Status != DayPresent {
		t.Fatalf("check out: %+v %v", s, err)
	}
	_, err = f.repo.CheckInOut(f.ctx, f.actor("Nobody", false), nil, nil)
	wantErr(t, "no employee", err, ErrForbidden)

	st, _ := f.repo.GetSettings(f.ctx, f.org)
	if st.GeofenceEnabled {
		t.Fatal("geofence must default to off")
	}
	st.GeofenceEnabled, st.GeofenceLat, st.GeofenceLng, st.GeofenceRadiusM = true, 18.5204, 73.8567, 200
	if _, err := f.repo.SaveSettings(f.ctx, hrA, st); err != nil {
		t.Fatal(err)
	}
	*f.clock = time.Date(2026, 6, 16, 9, 20, 0, 0, ist)
	_, err = f.repo.CheckInOut(f.ctx, asha, nil, nil)
	wantErr(t, "geofence needs a position", err, ErrInvalid)
	far, near1, lng := 19.0, 18.5210, 73.8567
	_, err = f.repo.CheckInOut(f.ctx, asha, &far, &lng)
	wantErr(t, "outside geofence", err, ErrForbidden)
	if _, err := f.repo.CheckInOut(f.ctx, asha, &near1, &lng); err != nil {
		t.Fatalf("inside geofence: %v", err)
	}
	st.GeofenceEnabled = false
	_, _ = f.repo.SaveSettings(f.ctx, hrA, st)

	// Codes: Hana EMP0001, Meera 0002, Asha 0003, Ravi 0004.
	csvIn := "emp_code,timestamp\nEMP0004,2026-06-10 09:30:00\nEMP0004,2026-06-10 18:45:00\nEMP0004,2026-06-10 18:45:00\nNOPE,2026-06-10 09:30:00\nEMP0004,garbage\n"
	res, err := f.repo.ImportPunchesCSV(f.ctx, hrA, strings.NewReader(csvIn))
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 2 || res.Duplicates != 1 || len(res.Errors) != 2 {
		t.Fatalf("import %+v", res)
	}
	ts, err := f.repo.GetTimesheet(f.ctx, ravi, f.emps["Ravi"], "2026-06")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ts.Days {
		if d.Date == "2026-06-10" && (d.Status != DayPresent || d.FirstIn != "09:30" || d.LastOut != "18:45") {
			t.Fatalf("imported day %+v", d)
		}
	}

	reg, err := f.repo.RequestRegularisation(f.ctx, ravi, "2026-06-11", "09:30", "18:30", "forgot to punch")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.repo.RequestRegularisation(f.ctx, ravi, "2026-06-11", "09:30", "18:30", "again")
	wantErr(t, "duplicate regularisation", err, ErrConflict)
	_, err = f.repo.RequestRegularisation(f.ctx, ravi, "2026-06-20", "09:30", "18:30", "future")
	wantErr(t, "future regularisation", err, ErrInvalid)
	_, err = f.repo.DecideRegularisation(f.ctx, asha, reg.ID, true, "")
	wantErr(t, "peer decides", err, ErrNotFound)
	_, err = f.repo.DecideRegularisation(f.ctx, ravi, reg.ID, true, "")
	wantErr(t, "self decides", err, ErrForbidden)
	if _, err := f.repo.DecideRegularisation(f.ctx, meera, reg.ID, true, "ok"); err != nil {
		t.Fatal(err)
	}
	ts, _ = f.repo.GetTimesheet(f.ctx, meera, f.emps["Ravi"], "2026-06")
	for _, d := range ts.Days {
		if d.Date == "2026-06-11" && d.Status != DayPresent {
			t.Fatalf("regularised day %+v", d)
		}
	}

	rows, err := f.repo.MonthlyReport(f.ctx, hrA, "2026-06")
	if err != nil || len(rows) != 5 {
		t.Fatalf("hr report %d %v", len(rows), err)
	}
	rows, err = f.repo.MonthlyReport(f.ctx, meera, "2026-06")
	if err != nil || len(rows) != 2 {
		t.Fatalf("manager report %d %v", len(rows), err)
	}
	_, err = f.repo.MonthlyReport(f.ctx, asha, "2026-06")
	wantErr(t, "employee report", err, ErrForbidden)
	var buf bytes.Buffer
	if err := f.repo.MonthlyReportCSV(f.ctx, hrA, "2026-06", &buf); err != nil || !strings.Contains(buf.String(), "lop_days") {
		t.Fatal("csv report")
	}

	if _, err := f.repo.SetRoster(f.ctx, meera, []string{f.emps["Asha"]}, "2026-06-12", "2026-06-12", "off"); err != nil {
		t.Fatal(err)
	}
	_, err = f.repo.SetRoster(f.ctx, meera, []string{f.emps["Kabir"]}, "2026-06-12", "2026-06-12", "off")
	wantErr(t, "roster outside the team", err, ErrForbidden)
	ts, _ = f.repo.GetTimesheet(f.ctx, hrA, f.emps["Asha"], "2026-06")
	for _, d := range ts.Days {
		if (d.Date == "2026-06-12" || d.Date == "2026-06-14") && d.Status != DayWeeklyOff {
			t.Fatalf("expected a day off on %s: %+v", d.Date, d)
		}
	}
}

func TestPayrollRunJournalAndPayslips(t *testing.T) {
	f := newFixture(t)
	hrA := f.team()
	asha, ravi, meera := f.actor("Asha", false), f.actor("Ravi", false), f.actor("Meera", false)

	loc, err := f.repo.SaveLookup(f.ctx, hrA, "locations", "", Lookup{Name: "Pune HQ", State: "Maharashtra"})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Asha", "Ravi"} {
		e, _ := f.repo.GetEmployee(f.ctx, hrA, f.emps[n])
		e.LocationID = loc.ID
		if _, err := f.repo.SaveEmployee(f.ctx, hrA, e.ID, e); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.repo.SaveSalary(f.ctx, hrA, f.emps["Asha"], EmployeeSalary{EffectiveFrom: "2026-01-01", Lines: stdLines(t, 600000), PFEnabled: true, ESIEnabled: true, PTEnabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SaveSalary(f.ctx, hrA, f.emps["Ravi"], EmployeeSalary{EffectiveFrom: "2026-01-01", Lines: stdLines(t, 240000), PFEnabled: true, ESIEnabled: true, PTEnabled: true}); err != nil {
		t.Fatal(err)
	}
	r1, err := f.repo.ApplyLeave(f.ctx, ravi, LeaveInput{TypeID: f.leaveType("LOP"), From: "2026-05-05", To: "2026-05-07"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.DecideLeave(f.ctx, meera, r1.ID, true, ""); err != nil {
		t.Fatal(err)
	}

	run, err := f.repo.CreateRun(f.ctx, hrA, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if run.Employees != 2 || run.Status != "draft" || len(run.Skipped) == 0 {
		t.Fatalf("run %+v", run)
	}
	_, err = f.repo.CreateRun(f.ctx, hrA, "2026-05")
	wantErr(t, "duplicate run", err, ErrConflict)
	slips, _ := f.repo.RunPayslips(f.ctx, hrA, run.ID)
	by := map[string]Payslip{}
	for _, s := range slips {
		by[s.EmpCode] = s
	}
	a, rv := by["EMP0003"], by["EMP0004"]
	near(t, "asha gross", a.Gross, 50000)
	near(t, "asha pf", a.PFEmployee, 1800)
	near(t, "asha pt (Maharashtra, May)", a.ProfTax, 200)
	near(t, "ravi lop", rv.LOPDays, 3)
	near(t, "ravi paid days", rv.PaidDays, 28)
	near(t, "ravi gross", rv.Gross, round2(20000*28/31.0))
	if rv.ESIEmployee <= 0 || rv.ESIEmployer <= 0 {
		t.Fatalf("ravi should have ESI: %+v", rv)
	}

	adj, err := f.repo.SetAdjustments(f.ctx, hrA, a.ID, []Adjustment{{Kind: "earning", Name: "Bonus", Amount: 5000}, {Kind: "deduction", Name: "Advance", Amount: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	near(t, "bonus gross", adj.Gross, 55000)
	if _, err := f.repo.Recompute(f.ctx, hrA, run.ID); err != nil {
		t.Fatal(err)
	}
	slips, _ = f.repo.RunPayslips(f.ctx, hrA, run.ID)
	for _, s := range slips {
		if s.EmpCode == "EMP0003" {
			near(t, "bonus kept after recompute", s.Gross, 55000)
			a = s
		}
	}
	_, err = f.repo.SetAdjustments(f.ctx, asha, a.ID, []Adjustment{{Kind: "earning", Name: "Bonus", Amount: 99999}})
	wantErr(t, "employee edits own payslip", err, ErrForbidden)

	if _, err := f.repo.GetPayslip(f.ctx, asha, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("draft payslip visible to employee: %v", err)
	}
	if mine, _ := f.repo.MyPayslips(f.ctx, asha); len(mine) != 0 {
		t.Fatal("no payslips before lock")
	}
	_, err = f.repo.LockRun(f.ctx, hrA, run.ID)
	wantErr(t, "lock before review", err, ErrConflict)
	_, err = f.repo.MarkReviewed(f.ctx, meera, run.ID)
	wantErr(t, "review by manager", err, ErrForbidden)
	if _, err := f.repo.MarkReviewed(f.ctx, hrA, run.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.repo.SetAdjustments(f.ctx, hrA, a.ID, nil)
	wantErr(t, "adjust after review", err, ErrConflict)
	_, err = f.repo.Recompute(f.ctx, hrA, run.ID)
	wantErr(t, "recompute after review", err, ErrConflict)
	var adv bytes.Buffer
	if err := f.repo.BankAdviceCSV(f.ctx, hrA, run.ID, &adv); err != nil || !strings.Contains(adv.String(), "EMP0003") || !strings.Contains(adv.String(), "Salary May 2026") {
		t.Fatalf("bank advice: %v %s", err, adv.String())
	}
	wantErr(t, "bank advice by employee", f.repo.BankAdviceCSV(f.ctx, asha, run.ID, io.Discard), ErrForbidden)

	locked, err := f.repo.LockRun(f.ctx, hrA, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if locked.Status != "locked" || locked.JournalID == "" {
		t.Fatalf("locked %+v", locked)
	}
	_, err = f.repo.LockRun(f.ctx, hrA, run.ID)
	wantErr(t, "lock twice", err, ErrConflict)
	_, err = f.repo.ReopenRun(f.ctx, hrA, run.ID)
	wantErr(t, "reopen locked", err, ErrConflict)
	wantErr(t, "delete locked", f.repo.DeleteRun(f.ctx, hrA, run.ID), ErrConflict)

	var n int
	var srcID string
	var dr, cr float64
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*), max(source_id) FROM adrient.acct_journals WHERE org_id=$1 AND source='payroll'`, f.org).Scan(&n, &srcID); err != nil {
		t.Fatal(err)
	}
	if n != 1 || srcID != run.ID {
		t.Fatalf("journals: %d %s", n, srcID)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT sum(debit)::float8, sum(credit)::float8 FROM adrient.acct_lines WHERE journal_id=$1::uuid`, locked.JournalID).Scan(&dr, &cr); err != nil {
		t.Fatal(err)
	}
	near(t, "balanced", dr-cr, 0)
	var salaryDebit float64
	_ = f.pool.QueryRow(f.ctx, `SELECT coalesce(sum(l.debit),0)::float8 FROM adrient.acct_lines l JOIN adrient.acct_accounts a ON a.id=l.account_id WHERE l.journal_id=$1::uuid AND a.system_key='salaries'`, locked.JournalID).Scan(&salaryDebit)
	near(t, "salary expense debit = total gross", salaryDebit, 55000+round2(20000*28/31.0))

	// Posting the same run again returns the same journal.
	slips, _ = f.repo.RunPayslips(f.ctx, hrA, run.ID)
	var tot RunTotals
	for _, s := range slips {
		tot.Employees++
		tot.Gross += s.Gross
		tot.PFEE += s.PFEmployee
		tot.PFER += s.PFEmployer
		tot.ESIEE += s.ESIEmployee
		tot.ESIER += s.ESIEmployer
		tot.PT += s.ProfTax
		tot.TDS += s.TDS
		tot.Other += s.OtherDed
		tot.Reimb += s.Reimburse
		tot.Net += s.Net
	}
	again, err := AccountingAdapter{Repo: f.acct}.PostPayroll(f.ctx, f.org, f.users["Hana"], tot.BuildJournal(run.ID, run.Period, "2026-05-31"))
	if err != nil || again != locked.JournalID {
		t.Fatalf("idempotent post: %q vs %q (%v)", again, locked.JournalID, err)
	}
	_ = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM adrient.acct_journals WHERE org_id=$1 AND source='payroll'`, f.org).Scan(&n)
	if n != 1 {
		t.Fatalf("%d payroll journals after re-post", n)
	}

	if !f.notif.has(f.users["Asha"], "Payslip available") {
		t.Fatal("payslip notification missing")
	}
	mine, err := f.repo.MyPayslips(f.ctx, asha)
	if err != nil || len(mine) != 1 || mine[0].EmpCode != "EMP0003" {
		t.Fatalf("my payslips %+v %v", mine, err)
	}
	if _, err := f.repo.GetPayslip(f.ctx, asha, a.ID); err != nil {
		t.Fatalf("own payslip: %v", err)
	}
	if _, err := f.repo.GetPayslip(f.ctx, asha, rv.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another employee's payslip must be hidden: %v", err)
	}
	if _, _, err := f.repo.PayslipPDF(f.ctx, asha, rv.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another employee's payslip PDF must be hidden: %v", err)
	}
	if _, _, err := f.repo.PayslipPDF(f.ctx, meera, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a manager must not read a report's payslip: %v", err)
	}
	pdfBytes, name, err := f.repo.PayslipPDF(f.ctx, asha, a.ID)
	if err != nil || !bytes.HasPrefix(pdfBytes, []byte("%PDF")) || !strings.HasSuffix(name, ".pdf") {
		t.Fatalf("pdf: %v %s", err, name)
	}
	if _, err := f.repo.RunPayslips(f.ctx, asha, run.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("run payslip list is HR only")
	}
}
