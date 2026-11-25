package shifts

import (
	"context"
	"errors"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opstest"
)

func TestRepositoryFlow(t *testing.T) {
	pool, org, admin := opstest.DB(t)
	ctx := context.Background()
	hrRepo := hr.NewRepository(pool)
	repo := NewRepository(pool, hrRepo)

	uMgr := opstest.User(t, pool, org, "meera")
	uAsha := opstest.User(t, pool, org, "asha")
	uBen := opstest.User(t, pool, org, "ben")
	eMgr := opstest.Employee(t, pool, org, uMgr, "Meera", "")
	eAsha := opstest.Employee(t, pool, org, uAsha, "Asha", eMgr)
	eBen := opstest.Employee(t, pool, org, uBen, "Ben", eMgr)

	sched, err := repo.CallerFor(ctx, org, admin, true)
	if err != nil || !sched.Scheduler {
		t.Fatalf("admin must be a scheduler: %+v %v", sched, err)
	}
	mgr, _ := repo.CallerFor(ctx, org, uMgr, false)
	asha, _ := repo.CallerFor(ctx, org, uAsha, false)
	if !mgr.Scheduler || asha.Scheduler {
		t.Fatalf("manager schedules, employee does not: %v %v", mgr.Scheduler, asha.Scheduler)
	}

	rate := 100.0
	if err := repo.SaveStaff(ctx, mgr, eAsha, StaffInput{Role: "Cashier", HourlyRate: &rate, MaxHours: ptr(20)}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveStaff(ctx, asha, eAsha, StaffInput{}); !errors.Is(err, opshttp.ErrForbidden) {
		t.Fatalf("employees cannot edit profiles: %v", err)
	}

	// An approved HR leave shows up as time off.
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.hr_leave_types (org_id, code, name) VALUES ($1,'CL','Casual') ON CONFLICT DO NOTHING`, org); err != nil {
		t.Skipf("leave type table shape differs: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.hr_leave_requests (org_id, employee_id, type_id, from_date, to_date, days, status)
		SELECT $1,$2::uuid,id,'2099-01-06','2099-01-06',1,'approved' FROM adrient.hr_leave_types WHERE org_id=$1 AND code='CL'`, org, eBen); err != nil {
		t.Fatal(err)
	}
	staff, err := repo.Staff(ctx, sched, "2099-01-05", "2099-01-11")
	if err != nil {
		t.Fatal(err)
	}
	var ben Staff
	for _, s := range staff {
		if s.EmployeeID == eBen {
			ben = s
		}
	}
	if TimeOffOn(ben.Availability, "2099-01-06") == nil {
		t.Fatalf("approved leave must show as time off: %+v", ben.Availability)
	}

	// Create shifts; warn on leave; block overlaps.
	res, err := repo.SaveShift(ctx, mgr, "", Shift{Date: "2099-01-06", Start: "09:00", End: "17:00", Role: "Cashier", EmployeeID: eBen})
	if err != nil || len(res.Warnings) == 0 {
		t.Fatalf("leave must warn: %+v %v", res, err)
	}
	s1, err := repo.SaveShift(ctx, mgr, "", Shift{Date: "2099-01-05", Start: "09:00", End: "17:00", BreakMinutes: 60, Role: "Cashier", EmployeeID: eAsha})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveShift(ctx, mgr, "", Shift{Date: "2099-01-05", Start: "16:00", End: "20:00", Role: "Cashier", EmployeeID: eAsha}); !errors.Is(err, opshttp.ErrConflict) {
		t.Fatalf("overlap must be refused: %v", err)
	}
	if _, err := repo.SaveShift(ctx, asha, "", Shift{Date: "2099-01-07", Start: "09:00", End: "17:00", Role: "x"}); !errors.Is(err, opshttp.ErrForbidden) {
		t.Fatalf("employee cannot create shifts: %v", err)
	}
	open, _ := repo.SaveShift(ctx, mgr, "", Shift{Date: "2099-01-07", Start: "09:00", End: "13:00", Role: "Cashier"})

	// Drafts are hidden from employees until published.
	got, _ := repo.ListShifts(ctx, asha, "2099-01-05", "2099-01-11")
	if len(got) != 0 {
		t.Fatalf("drafts must be hidden from employees: %d", len(got))
	}
	if n, err := repo.Publish(ctx, mgr, "2099-01-05", "2099-01-11", true); err != nil || n != 3 {
		t.Fatalf("publish: %d %v", n, err)
	}
	got, _ = repo.ListShifts(ctx, asha, "2099-01-05", "2099-01-11")
	if len(got) != 3 {
		t.Fatalf("published shifts visible: %d", len(got))
	}

	// Suggestions: Ben is on leave only on the 6th; Asha has max 20h.
	c, err := repo.Suggest(ctx, mgr, open.Shift.ID)
	if err != nil || len(c) != 3 {
		t.Fatalf("suggest: %+v %v", c, err)
	}

	// Swap: Asha asks Ben; the manager approves; the shift moves.
	sw, err := repo.RequestSwap(ctx, asha, s1.Shift.ID, eBen, "dentist")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolveSwap(ctx, asha, sw.ID, "approve"); !errors.Is(err, opshttp.ErrForbidden) {
		t.Fatalf("employee cannot approve: %v", err)
	}
	if _, err := repo.ResolveSwap(ctx, mgr, sw.ID, "approve"); err != nil {
		t.Fatal(err)
	}
	moved, _ := repo.GetShift(ctx, mgr, s1.Shift.ID)
	if moved.EmployeeID != eBen {
		t.Fatalf("shift must move to Ben: %+v", moved)
	}
	if _, err := repo.ResolveSwap(ctx, mgr, sw.ID, "decline"); !errors.Is(err, opshttp.ErrConflict) {
		t.Fatalf("resolved swap cannot change: %v", err)
	}

	// Attendance, timesheet and payroll lines.
	if _, err := repo.SetAttendance(ctx, mgr, s1.Shift.ID, Attendance{Status: "late", ClockIn: "09:30", ClockOut: "17:00"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveStaff(ctx, mgr, eBen, StaffInput{Role: "Cashier", HourlyRate: &rate}); err != nil {
		t.Fatal(err)
	}
	rows, sum, err := repo.Timesheet(ctx, mgr, "2099-01-01", "2099-01-31")
	if err != nil || len(rows) != 2 || len(sum) != 1 {
		t.Fatalf("timesheet: %d rows %d summary %v", len(rows), len(sum), err)
	}
	lines, err := repo.PayrollLines(ctx, mgr, "2099-01")
	if err != nil || len(lines) != 1 {
		t.Fatalf("payroll lines: %+v %v", lines, err)
	}
	// 7.5h clocked minus the 60 min break = 6.5h, plus the 8h leave-day shift (no break) = 14.5h at 100/h.
	if lines[0].Hours != 14.5 || lines[0].Amount != 1450 {
		t.Fatalf("payroll line = %+v", lines[0])
	}
	if _, err := repo.ExportPayroll(ctx, mgr, "2099-01"); !errors.Is(err, opshttp.ErrForbidden) {
		t.Fatalf("only HR admins export: %v", err)
	}

	// Copy week skips duplicates.
	if n, err := repo.CopyWeek(ctx, mgr, "2099-01-05", "2099-01-12"); err != nil || n != 3 {
		t.Fatalf("copy week: %d %v", n, err)
	}
	if n, _ := repo.CopyWeek(ctx, mgr, "2099-01-05", "2099-01-12"); n != 0 {
		t.Fatalf("second copy must add nothing, added %d", n)
	}
}

func ptr(f float64) *float64 { return &f }
