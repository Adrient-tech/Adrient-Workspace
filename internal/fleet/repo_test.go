package fleet

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opstest"
)

type fakeCal struct {
	events map[string]opshttp.CalEvent
	owners map[string]string
	n      int
}

func (f *fakeCal) Create(_ context.Context, _, owner string, e opshttp.CalEvent) (string, error) {
	f.n++
	id := fmt.Sprintf("ev%d", f.n)
	f.events[id], f.owners[id] = e, owner
	return id, nil
}
func (f *fakeCal) Update(_ context.Context, _, id string, e opshttp.CalEvent) error {
	if _, ok := f.events[id]; !ok {
		return opshttp.ErrNotFound
	}
	f.events[id] = e
	return nil
}
func (f *fakeCal) Delete(_ context.Context, _, id string) error { delete(f.events, id); return nil }

type fakeExp struct{ drafts []opshttp.ExpenseDraft }

func (f *fakeExp) Categories(context.Context, string) ([]opshttp.ExpenseAccount, error) {
	return []opshttp.ExpenseAccount{{ID: "00000000-0000-0000-0000-0000000000aa", Name: "Fuel"}}, nil
}
func (f *fakeExp) Record(_ context.Context, _, _ string, e opshttp.ExpenseDraft) (string, error) {
	f.drafts = append(f.drafts, e)
	return fmt.Sprintf("00000000-0000-0000-0000-00000000e%03d", len(f.drafts)), nil
}

func TestRepositoryFlow(t *testing.T) {
	pool, org, admin := opstest.DB(t)
	ctx := context.Background()
	cal := &fakeCal{events: map[string]opshttp.CalEvent{}, owners: map[string]string{}}
	exp := &fakeExp{}
	repo := NewRepository(pool, cal, exp)
	repo.now = func() time.Time { return time.Date(2099, 3, 1, 9, 0, 0, 0, time.Local) }
	adm := Caller{OrgID: org, UserID: admin, Admin: true}
	driver := Caller{OrgID: org, UserID: opstest.User(t, pool, org, "driver")}

	if _, err := repo.SaveVehicle(ctx, driver, "", Vehicle{Name: "x"}); !errors.Is(err, opshttp.ErrForbidden) {
		t.Fatalf("drivers cannot add vehicles: %v", err)
	}
	if _, err := repo.SaveVehicle(ctx, adm, "", Vehicle{Name: "Van", VIN: "1M8GDM9AXKP04278"}); !errors.Is(err, opshttp.ErrInvalid) {
		t.Fatalf("bad VIN shape: %v", err)
	}
	res, err := repo.SaveVehicle(ctx, adm, "", Vehicle{Name: "Van 1", VIN: "1m8gdm9axkp042788", Plate: "ka01 ab 1234", Meter: 50000, AssigneeID: driver.UserID})
	if err != nil || res.Vehicle.Year != 1989 || len(res.Warnings) != 0 {
		t.Fatalf("vehicle %+v %v", res, err)
	}
	v := res.Vehicle
	if _, err := repo.SaveVehicle(ctx, adm, "", Vehicle{Name: "Copy", VIN: "1M8GDM9AXKP042788"}); !errors.Is(err, opshttp.ErrConflict) {
		t.Fatalf("duplicate VIN refused: %v", err)
	}

	// Meter readings: drivers may add them, lower ones need confirmation.
	if _, _, err := repo.AddMeter(ctx, driver, v.ID, 49000, "2099-03-01", "", false); !errors.Is(err, opshttp.ErrConflict) {
		t.Fatalf("lower reading refused: %v", err)
	}
	if got, _, err := repo.AddMeter(ctx, driver, v.ID, 50400, "2099-03-01", "", false); err != nil || got.Meter != 50400 {
		t.Fatalf("meter %+v %v", got, err)
	}

	// Fuel with economy between full tanks, and posting the cost to Accounting once.
	if _, _, err := repo.AddFuel(ctx, driver, Fuel{VehicleID: v.ID, Date: "2099-03-01", Quantity: 40, Total: 4000, Meter: 50400, FullTank: true, Vendor: "Shell"}); err != nil {
		t.Fatal(err)
	}
	f2, _, err := repo.AddFuel(ctx, driver, Fuel{VehicleID: v.ID, Date: "2099-03-10", Quantity: 30, Total: 3000, Meter: 50850, FullTank: true})
	if err != nil {
		t.Fatal(err)
	}
	fl, _ := repo.ListFuel(ctx, driver, v.ID)
	if len(fl) != 2 || fl[0].Economy != 15 {
		t.Fatalf("economy %+v", fl)
	}
	if _, err := repo.PostExpense(ctx, driver, "fuel", f2.ID, "00000000-0000-0000-0000-0000000000aa"); !errors.Is(err, opshttp.ErrForbidden) {
		t.Fatalf("only admins post expenses: %v", err)
	}
	if _, err := repo.PostExpense(ctx, adm, "fuel", f2.ID, "00000000-0000-0000-0000-0000000000aa"); err != nil || len(exp.drafts) != 1 || exp.drafts[0].Amount != 3000 {
		t.Fatalf("expense %+v %v", exp.drafts, err)
	}
	if _, err := repo.PostExpense(ctx, adm, "fuel", f2.ID, "00000000-0000-0000-0000-0000000000aa"); !errors.Is(err, opshttp.ErrConflict) {
		t.Fatalf("no double posting: %v", err)
	}

	// Reminder: every 6 months or 10,000 km. Creating it puts a Calendar item on the due date, assigned to the driver.
	rem, err := repo.SaveReminder(ctx, adm, "", Reminder{VehicleID: v.ID, Title: "Oil change", Interval: Interval{Every: 6, Unit: "months"}, MeterInterval: 10000, LastDate: "2098-10-01", LastMeter: 45000})
	if err != nil || rem.NextDate != "2099-04-01" || rem.Status != "upcoming" {
		t.Fatalf("reminder %+v %v", rem, err)
	}
	if len(cal.events) != 1 || cal.owners[rem.EventID] != driver.UserID || !cal.events[rem.EventID].AllDay {
		t.Fatalf("calendar %+v %+v", cal.events, cal.owners)
	}
	// A service entry completes it: the next due date moves and so does the Calendar item.
	if _, err := repo.AddService(ctx, adm, Service{VehicleID: v.ID, Date: "2099-03-02", Meter: 50900, Description: "Oil and filter", Cost: 2500, Vendor: "Garage", ReminderIDs: []string{rem.ID}}); err != nil {
		t.Fatal(err)
	}
	rl, _ := repo.ListReminders(ctx, adm, v.ID)
	if len(rl) != 1 || rl[0].NextDate != "2099-09-02" || rl[0].Status != "upcoming" || len(rl[0].History) != 1 || !rl[0].History[0].OnTime {
		t.Fatalf("after service %+v", rl)
	}
	if cal.events[rl[0].EventID].Start.Format("2006-01-02") != "2099-09-02" || len(cal.events) != 1 {
		t.Fatalf("calendar must follow the due date: %+v", cal.events)
	}
	if err := repo.DeleteReminder(ctx, adm, rem.ID); err != nil || len(cal.events) != 0 {
		t.Fatalf("deleting a reminder removes its Calendar item: %v %d", err, len(cal.events))
	}

	// Renewal rolls from the due date.
	rn, err := repo.SaveRenewal(ctx, adm, "", Renewal{VehicleID: v.ID, Kind: "insurance", Title: "Insurance", DueDate: "2099-03-10", Repeat: Interval{Every: 1, Unit: "years"}})
	if err != nil || rn.Status != "due_soon" || len(cal.events) != 1 {
		t.Fatalf("renewal %+v %v", rn, err)
	}
	if rn, err = repo.CompleteRenewal(ctx, adm, rn.ID, "2099-03-05", 15000, ""); err != nil || rn.DueDate != "2100-03-10" || rn.Status != "upcoming" {
		t.Fatalf("renewed %+v %v", rn, err)
	}

	// An inspection with a failed item opens a high-priority issue.
	insp, err := repo.SubmitInspection(ctx, driver, Inspection{VehicleID: v.ID, Meter: 51000, Items: []InspectionItem{{Label: "Brakes", Result: "pass"}, {Label: "Tyres", Result: "fail", Note: "Left front worn"}}})
	if err != nil || insp.Failed != 1 || len(insp.IssueIDs) != 1 {
		t.Fatalf("inspection %+v %v", insp, err)
	}
	issues, _ := repo.ListIssues(ctx, driver, v.ID, "open")
	if len(issues) != 1 || issues[0].Priority != "high" || issues[0].Source != "inspection" {
		t.Fatalf("issues %+v", issues)
	}
	if _, err := repo.SaveIssue(ctx, driver, issues[0].ID, Issue{VehicleID: v.ID, Title: "x", Status: "closed"}); !errors.Is(err, opshttp.ErrForbidden) {
		t.Fatalf("only admins close issues: %v", err)
	}
	if _, err := repo.SaveIssue(ctx, adm, issues[0].ID, Issue{VehicleID: v.ID, Title: issues[0].Title, Status: "resolved", Resolution: "New tyre"}); err != nil {
		t.Fatal(err)
	}

	d, err := repo.Dashboard(ctx, adm)
	if err != nil || d.Vehicles != 1 || d.OpenIssues != 0 || d.FailedLast30d != 1 || len(d.Months) != 6 || d.Months[5].Fuel != 7000 {
		t.Fatalf("dashboard %+v %v", d, err)
	}
	costs, _ := repo.Costs(ctx, adm, v.ID)
	if costs.Total != 9500 || costs.Distance != 1000 {
		t.Fatalf("costs %+v", costs)
	}
}
