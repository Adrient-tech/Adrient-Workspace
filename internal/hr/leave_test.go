package hr

import (
	"strings"
	"testing"
	"time"
)

func balanceOf(bs []Balance, code string) Balance {
	for _, b := range bs {
		if b.Code == code {
			return b
		}
	}
	return Balance{}
}

func TestLeaveLifecycle(t *testing.T) {
	f := newFixture(t)
	hrA := f.team()
	asha, meera, ravi, kabirA := f.actor("Asha", false), f.actor("Meera", false), f.actor("Ravi", false), f.actor("Kabir", false)
	cl := f.leaveType("CL")
	bal := func() []Balance {
		b, err := f.repo.Balances(f.ctx, asha, f.emps["Asha"])
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	near(t, "CL after Jan-Jun", balanceOf(bal(), "CL").Balance, 6)
	near(t, "SL annual grant", balanceOf(bal(), "SL").Balance, 6)
	near(t, "EL: 30 carried (cap) + 1.25 x 6", balanceOf(bal(), "EL").Balance, 37.5)
	if _, err := f.repo.RunAccrual(f.ctx, hrA); err != nil {
		t.Fatal(err)
	}
	near(t, "CL unchanged after re-run", balanceOf(bal(), "CL").Balance, 6)

	req, err := f.repo.ApplyLeave(f.ctx, asha, LeaveInput{TypeID: cl, From: "2026-06-22", To: "2026-06-23", Reason: "family"})
	if err != nil {
		t.Fatal(err)
	}
	near(t, "days", req.Days, 2)
	if req.Status != "pending" || req.CanDecide {
		t.Fatalf("request %+v", req)
	}
	if !f.notif.has(f.users["Meera"], "Leave request") {
		t.Fatal("manager was not notified")
	}
	_, err = f.repo.ApplyLeave(f.ctx, asha, LeaveInput{TypeID: cl, From: "2026-06-23", To: "2026-06-24"})
	wantErr(t, "overlap", err, ErrConflict)
	_, err = f.repo.ApplyLeave(f.ctx, asha, LeaveInput{TypeID: cl, From: "2026-07-06", To: "2026-07-17"})
	wantErr(t, "balance", err, ErrConflict)
	_, err = f.repo.ApplyLeave(f.ctx, asha, LeaveInput{TypeID: cl, From: "2026-06-28", To: "2026-06-28"})
	wantErr(t, "sunday only", err, ErrInvalid)
	_, err = f.repo.ApplyLeave(f.ctx, asha, LeaveInput{EmployeeID: f.emps["Ravi"], TypeID: cl, From: "2026-07-06"})
	wantErr(t, "apply for a peer", err, ErrForbidden)

	_, err = f.repo.DecideLeave(f.ctx, asha, req.ID, true, "")
	wantErr(t, "self approval", err, ErrForbidden)
	_, err = f.repo.DecideLeave(f.ctx, ravi, req.ID, true, "")
	wantErr(t, "peer approval", err, ErrNotFound)
	_, err = f.repo.DecideLeave(f.ctx, kabirA, req.ID, true, "")
	wantErr(t, "other manager approval", err, ErrNotFound)

	got, err := f.repo.DecideLeave(f.ctx, meera, req.ID, true, "enjoy")
	if err != nil || got.Status != "approved" {
		t.Fatalf("approve: %+v %v", got, err)
	}
	near(t, "CL after approval", balanceOf(bal(), "CL").Balance, 4)
	near(t, "CL used", balanceOf(bal(), "CL").Used, 2)
	if len(f.cal.events) != 1 {
		t.Fatalf("calendar events: %v", f.cal.events)
	}
	for _, ev := range f.cal.events {
		if ev[0] != f.users["Asha"] || !strings.Contains(ev[1], "Out of office") || !strings.HasPrefix(ev[2], "2026-06-22T00:00:00") || !strings.HasPrefix(ev[3], "2026-06-24T00:00:00") {
			t.Fatalf("event %v", ev)
		}
	}
	if !f.notif.has(f.users["Asha"], "Leave approved") {
		t.Fatal("employee was not notified")
	}
	_, err = f.repo.DecideLeave(f.ctx, meera, req.ID, false, "")
	wantErr(t, "deciding twice", err, ErrConflict)

	tc, err := f.repo.TeamCalendar(f.ctx, ravi, "2026-06-01", "2026-06-30")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(tc["leaves"].([]TeamLeave)); n != 1 {
		t.Fatalf("peer sees %d leaves", n)
	}
	tc, _ = f.repo.TeamCalendar(f.ctx, kabirA, "2026-06-01", "2026-06-30")
	if n := len(tc["leaves"].([]TeamLeave)); n != 0 {
		t.Fatalf("outsider sees %d leaves", n)
	}

	_, err = f.repo.CancelLeave(f.ctx, ravi, req.ID)
	wantErr(t, "peer cancel", err, ErrNotFound)
	c, err := f.repo.CancelLeave(f.ctx, asha, req.ID)
	if err != nil || c.Status != "cancelled" {
		t.Fatalf("cancel: %+v %v", c, err)
	}
	near(t, "CL restored", balanceOf(bal(), "CL").Balance, 6)
	if len(f.cal.events) != 0 || len(f.cal.deleted) != 1 {
		t.Fatalf("calendar after cancel: %v %v", f.cal.events, f.cal.deleted)
	}
	if _, err = f.repo.CancelLeave(f.ctx, asha, req.ID); err == nil {
		t.Fatal("cancelling twice must fail")
	}

	h, err := f.repo.ApplyLeave(f.ctx, asha, LeaveInput{TypeID: cl, From: "2026-06-25", HalfDay: "second"})
	if err != nil {
		t.Fatal(err)
	}
	near(t, "half day", h.Days, 0.5)
	if _, err := f.repo.DecideLeave(f.ctx, hrA, h.ID, true, ""); err != nil {
		t.Fatalf("HR override: %v", err)
	}
	for _, ev := range f.cal.events {
		if !strings.Contains(ev[1], "second half") {
			t.Fatalf("half-day event title %q", ev[1])
		}
	}
	other, err := f.repo.ApplyLeave(f.ctx, asha, LeaveInput{TypeID: cl, From: "2026-06-25", HalfDay: "first"})
	if err != nil {
		t.Fatalf("opposite half on the same day is allowed: %v", err)
	}
	rj, err := f.repo.DecideLeave(f.ctx, meera, other.ID, false, "busy")
	if err != nil || rj.Status != "rejected" {
		t.Fatalf("reject %v %v", rj, err)
	}
	near(t, "CL after half day", balanceOf(bal(), "CL").Balance, 5.5)

	if _, err := f.repo.ApplyLeave(f.ctx, asha, LeaveInput{TypeID: f.leaveType("LOP"), From: "2026-08-03", To: "2026-08-14"}); err != nil {
		t.Fatalf("LOP: %v", err)
	}
	wantErr(t, "adjust without note", f.repo.AdjustBalance(f.ctx, hrA, f.emps["Asha"], cl, 2, ""), ErrInvalid)
	if err := f.repo.AdjustBalance(f.ctx, hrA, f.emps["Asha"], cl, 2, "comp-off"); err != nil {
		t.Fatal(err)
	}
	wantErr(t, "adjust by manager", f.repo.AdjustBalance(f.ctx, meera, f.emps["Asha"], cl, 2, "x"), ErrForbidden)
	lst, _ := f.repo.ListLeaveRequests(f.ctx, meera, LeaveFilter{Scope: "team", Status: "pending"})
	for _, q := range lst {
		if q.EmployeeID == f.emps["Kabir"] {
			t.Fatal("manager sees someone else's team")
		}
	}
	_, err = f.repo.ListLeaveRequests(f.ctx, asha, LeaveFilter{Scope: "all"})
	wantErr(t, "scope all", err, ErrForbidden)
}

func TestHolidaysReduceLeaveDays(t *testing.T) {
	f := newFixture(t)
	hrA := f.team()
	asha := f.actor("Asha", false)
	cals, _ := f.repo.ListHolidayCalendars(f.ctx, f.org)
	if _, err := f.repo.AddHoliday(f.ctx, hrA, cals[0].ID, Holiday{Day: "2026-06-23", Name: "Founders day"}); err != nil {
		t.Fatal(err)
	}
	r, err := f.repo.ApplyLeave(f.ctx, asha, LeaveInput{TypeID: f.leaveType("CL"), From: "2026-06-22", To: "2026-06-24"})
	if err != nil {
		t.Fatal(err)
	}
	near(t, "holiday excluded", r.Days, 2)
	_, err = f.repo.AddHoliday(f.ctx, asha, cals[0].ID, Holiday{Day: "2026-06-24", Name: "x"})
	wantErr(t, "employee adds holiday", err, ErrForbidden)
}

func TestCarryForwardAcrossYears(t *testing.T) {
	f := newFixture(t)
	hrA := f.team()
	f.employee(hrA, "Zed", "2025-01-01", "", false)
	read := func() []Balance {
		b, err := f.repo.Balances(f.ctx, hrA, f.emps["Zed"])
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	*f.clock = time.Date(2025, 12, 20, 10, 0, 0, 0, f.clock.Location())
	b := read()
	near(t, "EL 2025", balanceOf(b, "EL").Balance, 15)
	near(t, "CL 2025", balanceOf(b, "CL").Balance, 12)
	near(t, "SL 2025", balanceOf(b, "SL").Balance, 6)
	*f.clock = time.Date(2026, 2, 10, 10, 0, 0, 0, f.clock.Location())
	b = read()
	near(t, "EL 2026 = 15 carried + 2 x 1.25", balanceOf(b, "EL").Balance, 17.5)
	near(t, "CL 2026 = 2 months, nothing carried", balanceOf(b, "CL").Balance, 2)
	near(t, "SL 2026 = new grant", balanceOf(b, "SL").Balance, 6)
	near(t, "EL stable on re-read", balanceOf(read(), "EL").Balance, 17.5)

	capDays := 3.0
	if _, err := f.repo.SaveLeaveType(f.ctx, hrA, f.leaveType("CL"), LeaveType{Code: "CL", Name: "Casual Leave", Paid: true, Tracked: true, AnnualQuota: 12, Accrual: "monthly", MaxBalance: &capDays, AllowHalfDay: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	*f.clock = time.Date(2026, 6, 10, 10, 0, 0, 0, f.clock.Location())
	near(t, "CL capped at 3", balanceOf(read(), "CL").Balance, 3)
}
