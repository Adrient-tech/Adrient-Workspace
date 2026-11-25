package demoseed

import (
	"fmt"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contacts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/fieldservice"
	"github.com/Adrient-tech/Adrient-Workspace/internal/fleet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opsports"
	"github.com/Adrient-tech/Adrient-Workspace/internal/shifts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/timetracking"
)

// runV3 is seed step 3: the wave-6 operations apps (Shifts, Time tracking,
// Field service, Fleet). Dates are relative to today so the demo is always current.
func (s *seeder) runV3() {
	hrRepo := hr.NewRepository(s.pool)
	s.v3Shifts(hrRepo)
	s.v3Time(hrRepo)
	s.v3Field()
	s.v3Fleet()
}

type empRow struct{ id, user, name string }

func (s *seeder) v3Employees(limit int) []empRow {
	rows, err := s.pool.Query(s.ctx, `SELECT id::text, coalesce(user_id::text,''), trim(first_name||' '||last_name) FROM adrient.hr_employees
		WHERE org_id=$1 AND status='active' ORDER BY (user_id IS NULL), emp_code LIMIT $2`, s.org, limit)
	if err != nil {
		s.warn("ops employees", err)
		return nil
	}
	defer rows.Close()
	var out []empRow
	for rows.Next() {
		var e empRow
		if rows.Scan(&e.id, &e.user, &e.name) == nil {
			out = append(out, e)
		}
	}
	return out
}

func (s *seeder) weekStart() string {
	return shifts.StartOfWeek(day(s.today))
}

func (s *seeder) v3Shifts(hrRepo *hr.Repository) {
	repo := shifts.NewRepository(s.pool, hrRepo)
	c, err := repo.CallerFor(s.ctx, s.org, s.admin, true)
	if err != nil {
		s.warn("shifts caller", err)
		return
	}
	emps := s.v3Employees(8)
	if len(emps) < 3 {
		s.warn("shifts", fmt.Errorf("fewer than 3 HR employees; nothing to schedule"))
		return
	}
	if _, err := repo.SaveSettings(s.ctx, c, shifts.Settings{Rules: shifts.Rules{MinRestHours: 11, MaxDailyHours: 10, MaxConsecutive: 6}, Overtime: 40, OvertimeMult: 1.5, WeeklyBudget: 80000, Currency: "INR", GraceMinutes: 5}); err != nil {
		s.warn("shifts settings", err)
	}
	roles := []string{"Cashier", "Floor staff", "Stock keeper", "Supervisor"}
	for i, e := range emps {
		rate, target, max := float64(110+10*i), 40.0, 48.0
		if err := repo.SaveStaff(s.ctx, c, e.id, shifts.StaffInput{Role: roles[i%len(roles)], HourlyRate: &rate, TargetHours: &target, MaxHours: &max}); err != nil {
			s.warn("shifts staff", err)
		}
		s.count("shift_staff", 1)
	}
	for _, req := range []shifts.Coverage{{Role: "Cashier", Days: []int{1, 2, 3, 4, 5, 6}, Count: 2}, {Role: "Supervisor", Days: []int{1, 2, 3, 4, 5, 6}, Count: 1, Start: "09:00", End: "17:00"}} {
		if _, err := repo.SaveCoverage(s.ctx, c, "", req); err != nil {
			s.warn("shifts coverage", err)
		}
	}
	w0 := s.weekStart()
	var first string
	for week := 0; week < 2; week++ {
		ws := opshttp.AddDays(w0, 7*week)
		for d := 0; d < 6; d++ {
			date := opshttp.AddDays(ws, d)
			for i, e := range emps {
				if (d+i)%4 == 3 { // a rest day
					continue
				}
				start, end := "09:00", "17:00"
				if i%2 == 1 {
					start, end = "13:00", "21:00"
				}
				r, err := repo.SaveShift(s.ctx, c, "", shifts.Shift{Date: date, Start: start, End: end, BreakMinutes: 30, Role: roles[i%len(roles)], Location: "Main store", EmployeeID: e.id, Published: week == 0})
				if err != nil {
					continue
				}
				s.count("shifts", 1)
				if first == "" && week == 1 {
					first = r.Shift.ID
				}
			}
		}
		for _, d := range []int{4, 5} { // open shifts to fill
			if _, err := repo.SaveShift(s.ctx, c, "", shifts.Shift{Date: opshttp.AddDays(ws, d), Start: "17:00", End: "21:00", Role: "Cashier", Location: "Main store", Published: week == 0}); err == nil {
				s.count("shifts", 1)
			}
		}
	}
	// Attendance for the days already gone this week.
	cur, err := repo.ListShifts(s.ctx, c, w0, day(s.today.AddDate(0, 0, -1)))
	if err == nil {
		for i, sh := range cur {
			if sh.EmployeeID == "" {
				continue
			}
			a := shifts.Attendance{Status: "present", ClockIn: sh.Start, ClockOut: sh.End}
			switch {
			case i%11 == 5:
				a = shifts.Attendance{Status: "late", ClockIn: addMin(sh.Start, 17), ClockOut: sh.End}
			case i%17 == 7:
				a = shifts.Attendance{Status: "absent"}
			}
			if _, err := repo.SetAttendance(s.ctx, c, sh.ID, a); err != nil {
				s.warn("shifts attendance", err)
			}
		}
	}
	// A pending swap request for a published shift later this week.
	later, _ := repo.ListShifts(s.ctx, c, day(s.today), opshttp.AddDays(w0, 6))
	for _, sh := range later {
		if sh.EmployeeID == emps[0].id && sh.Published {
			if _, err := repo.RequestSwap(s.ctx, c, sh.ID, emps[2].id, "Family function"); err == nil {
				s.count("shift_swaps", 1)
			}
			break
		}
	}
	_ = first
}

func addMin(hhmm string, m int) string {
	var h, mm int
	fmt.Sscanf(hhmm, "%d:%d", &h, &mm)
	t := h*60 + mm + m
	return fmt.Sprintf("%02d:%02d", (t/60)%24, t%60)
}

func (s *seeder) v3Time(hrRepo *hr.Repository) {
	inv := opsports.Invoices{Repo: invoicing.NewRepository(s.pool)}
	dir := opsports.Contacts{Repo: contacts.NewRepository(s.pool)}
	repo := timetracking.NewRepository(s.pool, hrRepo, inv, dir)
	c, err := repo.CallerFor(s.ctx, s.org, s.admin, true)
	if err != nil {
		s.warn("time caller", err)
		return
	}
	set, _, _ := repo.GetSettings(s.ctx, s.org)
	set.DefaultRate, set.Currency, set.Rounding, set.TaxRate, set.TermsDays, set.PayrollRate = 1200, "INR", 15, 18, 15, 400
	if _, err := repo.SaveSettings(s.ctx, c, set); err != nil {
		s.warn("time settings", err)
	}
	r1, r2, rd := 1800.0, 1500.0, 15
	acme, e1 := repo.SaveClient(s.ctx, c, "", timetracking.Client{Name: "Kaveri Retail Pvt Ltd", Email: "accounts@kaveri.demo.adrient.example", Rate: &r1, Rounding: &rd, Color: "#0E7490"})
	nw, e2 := repo.SaveClient(s.ctx, c, "", timetracking.Client{Name: "Northwind Traders", Email: "billing@northwind.demo.adrient.example", Rate: &r2, Color: "#3B5BDB"})
	if e1 != nil || e2 != nil {
		s.warn("time clients", fmt.Errorf("%v %v", e1, e2))
		return
	}
	var projects []timetracking.Project
	for _, p := range []timetracking.Project{
		{Name: "Website redesign", ClientID: acme.ID, Billable: true, BudgetHours: 120},
		{Name: "Mobile app", ClientID: acme.ID, Billable: true, BudgetHours: 200},
		{Name: "Support retainer", ClientID: nw.ID, Billable: true, BudgetHours: 40},
		{Name: "Internal", Billable: false},
	} {
		saved, err := repo.SaveProject(s.ctx, c, "", p)
		if err != nil {
			s.warn("time project", err)
			continue
		}
		projects = append(projects, saved)
		s.count("time_projects", 1)
	}
	emps := s.v3Employees(4)
	var users []string
	for _, e := range emps {
		if e.user != "" {
			users = append(users, e.user)
		}
	}
	if len(users) == 0 {
		users = []string{s.admin}
	}
	tasks := []string{"Design review", "Development", "Client call", "Testing", "Planning", "Bug fixes"}
	for _, u := range users[:min(3, len(users))] {
		uc, _ := repo.CallerFor(s.ctx, s.org, u, u == s.admin)
		uc.Admin = true // the seeder logs on their behalf
		for back := 1; back <= 21; back++ {
			t := s.today.AddDate(0, 0, -back)
			if t.Weekday() == 0 || t.Weekday() == 6 {
				continue
			}
			for k := 0; k < 1+s.r.Intn(3); k++ {
				p := projects[s.r.Intn(len(projects))]
				sec := (2 + s.r.Intn(12)) * 15 * 60
				if _, err := repo.SaveEntry(s.ctx, uc, "", timetracking.Entry{UserID: u, ProjectID: p.ID, Date: day(t), Seconds: sec, Billable: true, Task: tasks[s.r.Intn(len(tasks))]}); err == nil {
					s.count("time_entries", 1)
				}
			}
		}
	}
	// Invoice the older Kaveri time so the demo shows a real invoice in Invoicing.
	if res, err := repo.CreateInvoice(s.ctx, c, acme.ID, "", day(s.today.AddDate(0, 0, -10)), nil); err != nil {
		s.warn("time invoice", err)
	} else {
		s.count("time_invoices", 1)
		_ = res
	}
}

func (s *seeder) v3Field() {
	dir := opsports.Contacts{Repo: contacts.NewRepository(s.pool)}
	inv := opsports.Invoices{Repo: invoicing.NewRepository(s.pool)}
	cal := opsports.Calendar{Repo: calendar.NewRepository(s.pool)}
	repo := fieldservice.NewRepository(s.pool, dir, inv, cal)
	c := fieldservice.Caller{OrgID: s.org, UserID: s.admin, Admin: true}
	if _, err := repo.SaveSettings(s.ctx, c, fieldservice.Settings{TaxRate: 18, Currency: "INR", TermsDays: 15, DefaultBilling: "on_completion"}); err != nil {
		s.warn("field settings", err)
	}
	for _, sv := range []fieldservice.Service{
		{Name: "Lawn mowing", UnitPrice: 1200, UnitCost: 500, Taxable: true}, {Name: "Hedge trimming", UnitPrice: 1800, UnitCost: 700, Taxable: true},
		{Name: "AC service", UnitPrice: 2500, UnitCost: 900, Taxable: true}, {Name: "Deep cleaning (per room)", UnitPrice: 1500, UnitCost: 600, Taxable: true},
		{Name: "Plumbing call-out", UnitPrice: 800, UnitCost: 200, Taxable: true},
	} {
		if _, err := repo.SaveService(s.ctx, c, "", sv); err == nil {
			s.count("field_services", 1)
		}
	}
	var team []string
	if rows, err := s.pool.Query(s.ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 ORDER BY created_at LIMIT 3`, s.org); err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				team = append(team, id)
			}
		}
		rows.Close()
	}
	type cust struct{ name, addr string }
	var made []struct {
		ref  opshttp.ContactRef
		prop fieldservice.Property
	}
	for _, x := range []cust{{"Meera Iyer", "14 Lake View Road, Pune"}, {"Sundaram Apartments", "Plot 7, Baner, Pune"}, {"Dr. Rohan Kulkarni", "22 Garden Street, Pune"}} {
		ref, err := repo.CreateCustomer(s.ctx, c, opshttp.ContactRef{Name: x.name, Email: slug(x.name) + "@" + demoDomain, Phone: s.r.phone()})
		if err != nil {
			s.warn("field customer", err)
			continue
		}
		p, err := repo.SaveProperty(s.ctx, c, "", fieldservice.Property{ContactID: ref.ID, Name: "Main", Address: x.addr})
		if err != nil {
			s.warn("field property", err)
			continue
		}
		made = append(made, struct {
			ref  opshttp.ContactRef
			prop fieldservice.Property
		}{ref, p})
		s.count("field_customers", 1)
	}
	if len(made) < 3 {
		return
	}
	line := func(n string, q, p, cst float64) fieldservice.Item {
		return fieldservice.Item{Name: n, Qty: q, UnitPrice: p, UnitCost: cst, Taxable: true}
	}
	// 1. A new request waiting for a quote.
	if _, err := repo.SaveRequest(s.ctx, c, "", fieldservice.Request{ContactID: made[2].ref.ID, PropertyID: made[2].prop.ID, Title: "Gutter cleaning and roof check", Details: "Leaves blocking the downpipe on the north side.",
		Preferred: day(s.today.AddDate(0, 0, 5)), Items: []fieldservice.Item{line("Plumbing call-out", 1, 800, 200), line("Deep cleaning (per room)", 2, 1500, 600)}}); err == nil {
		s.count("field_requests", 1)
	}
	// 2. A quoted request (the quote is a draft estimate in Invoicing).
	if q, err := repo.SaveRequest(s.ctx, c, "", fieldservice.Request{ContactID: made[1].ref.ID, PropertyID: made[1].prop.ID, Title: "Annual AC service, 12 units", Details: "Common areas and clubhouse.",
		Preferred: day(s.today.AddDate(0, 0, 9)), Items: []fieldservice.Item{line("AC service", 12, 2500, 900)}}); err == nil {
		s.count("field_requests", 1)
		if _, err := repo.CreateQuote(s.ctx, c, q.ID); err != nil {
			s.warn("field quote", err)
		}
	}
	// 3. A request that became a weekly job, with last week's visits done and invoiced.
	if q, err := repo.SaveRequest(s.ctx, c, "", fieldservice.Request{ContactID: made[0].ref.ID, PropertyID: made[0].prop.ID, Title: "Weekly garden care", Details: "Mow, edge and clear clippings.",
		Preferred: day(s.today.AddDate(0, 0, -14)), Items: []fieldservice.Item{line("Lawn mowing", 1, 1200, 500), line("Hedge trimming", 1, 1800, 700)}}); err == nil {
		s.count("field_requests", 1)
		_, _ = repo.CreateQuote(s.ctx, c, q.ID)
		job, err := repo.CreateJobFromRequest(s.ctx, c, q.ID, fieldservice.Schedule{Start: day(s.today.AddDate(0, 0, -14)), Repeat: "weekly", Interval: 1, EndType: "count", EndCount: 8, StartTime: "08:00", EndTime: "10:00"}, team[:min(2, len(team))])
		if err != nil {
			s.warn("field job", err)
		} else {
			s.count("field_jobs", 1)
			for _, v := range job.Visits {
				if v.Date < day(s.today) {
					if _, _, err := repo.CompleteVisit(s.ctx, c, v.ID); err != nil {
						s.warn("field visit", err)
					}
				}
			}
			if _, _, err := repo.InvoiceJob(s.ctx, c, job.ID); err != nil {
				s.warn("field invoice", err)
			}
		}
	}
	// 4. A one-off job in two days.
	if _, err := repo.SaveJob(s.ctx, c, "", fieldservice.Job{ContactID: made[2].ref.ID, ClientName: made[2].ref.Name, PropertyID: made[2].prop.ID, Title: "Kitchen tap replacement", Instructions: "Customer supplies the tap. Bring a sealing kit.",
		Schedule: fieldservice.Schedule{Start: day(s.today.AddDate(0, 0, 2)), StartTime: "11:00", EndTime: "12:30"}, Assignees: team[:min(1, len(team))], Items: []fieldservice.Item{line("Plumbing call-out", 1, 800, 200)}, TaxRate: 18}); err == nil {
		s.count("field_jobs", 1)
	}
}

func (s *seeder) v3Fleet() {
	cal := opsports.Calendar{Repo: calendar.NewRepository(s.pool)}
	exp := opsports.Expenses{Repo: accounting.NewRepository(s.pool)}
	repo := fleet.NewRepository(s.pool, cal, exp)
	c := fleet.Caller{OrgID: s.org, UserID: s.admin, Admin: true}
	var drivers []string
	if rows, err := s.pool.Query(s.ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 ORDER BY created_at LIMIT 3`, s.org); err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				drivers = append(drivers, id)
			}
		}
		rows.Close()
	}
	driver := func(i int) string {
		if len(drivers) == 0 {
			return ""
		}
		return drivers[i%len(drivers)]
	}
	type seedV struct {
		v     fleet.Vehicle
		meter float64
	}
	var made []fleet.Vehicle
	for i, x := range []seedV{
		{fleet.Vehicle{Name: "Delivery van 1", VIN: "1M8GDM9AXKP042788", Plate: "MH12 AB 1234", Make: "Tata", Model: "Ace Gold", Type: "Van", FuelType: "Diesel", Group: "Delivery", Meter: 41200, PurchasePrice: 650000}, 41200},
		{fleet.Vehicle{Name: "Delivery van 2", VIN: "MA1TA2GA5K2A12345", Plate: "MH12 CD 5678", Make: "Mahindra", Model: "Supro", Type: "Van", FuelType: "Diesel", Group: "Delivery", Meter: 78300, PurchasePrice: 590000}, 78300},
		{fleet.Vehicle{Name: "Site pickup", Plate: "MH14 EF 9012", Make: "Isuzu", Model: "D-Max", Type: "Pickup", FuelType: "Diesel", Group: "Field", Meter: 22800, PurchasePrice: 1450000}, 22800},
		{fleet.Vehicle{Name: "Warehouse forklift", Make: "Toyota", Model: "8FD25", Type: "Forklift", FuelType: "Diesel", Group: "Warehouse", Status: "maintenance", Meter: 3150, PurchasePrice: 1100000}, 3150},
	} {
		x.v.AssigneeID = driver(i)
		x.v.Status = map[bool]string{true: "maintenance", false: "active"}[x.v.Status == "maintenance"]
		res, err := repo.SaveVehicle(s.ctx, c, "", x.v)
		if err != nil {
			s.warn("fleet vehicle", err)
			continue
		}
		made = append(made, res.Vehicle)
		s.count("fleet_vehicles", 1)
	}
	today := day(s.today)
	var fuelIDs []string
	for i, v := range made {
		meter := v.Meter
		for k := 8; k >= 1; k-- {
			meter += float64(280 + s.r.Intn(260))
			qty := float64(28 + s.r.Intn(14))
			f, _, err := repo.AddFuel(s.ctx, c, fleet.Fuel{VehicleID: v.ID, Date: day(s.today.AddDate(0, 0, -k*8-i)), Quantity: qty, Total: qty * 92, Meter: meter, FullTank: true, Vendor: "HP Petrol Pump"})
			if err != nil {
				s.warn("fleet fuel", err)
				continue
			}
			fuelIDs = append(fuelIDs, f.ID)
			s.count("fleet_fuel", 1)
		}
		// Service history: the first van's oil change is overdue, the second's is due soon.
		last := s.today.AddDate(0, -7+i*2, -s.r.Intn(5))
		if _, err := repo.AddService(s.ctx, c, fleet.Service{VehicleID: v.ID, Date: day(last), Meter: v.Meter, Description: "Engine oil and filter", Vendor: "Shree Auto Works", Cost: float64(3200 + 400*i)}); err != nil {
			s.warn("fleet service", err)
		}
		rem, err := repo.SaveReminder(s.ctx, c, "", fleet.Reminder{VehicleID: v.ID, Title: "Oil change", Interval: fleet.Interval{Every: 6, Unit: "months"}, MeterInterval: 10000, LastDate: day(last), LastMeter: v.Meter})
		if err != nil {
			s.warn("fleet reminder", err)
		} else {
			_ = rem
			s.count("fleet_reminders", 1)
		}
		if _, err := repo.SaveRenewal(s.ctx, c, "", fleet.Renewal{VehicleID: v.ID, Kind: "insurance", Title: "Motor insurance", DueDate: day(s.today.AddDate(0, 0, 8+i*40)), Repeat: fleet.Interval{Every: 1, Unit: "years"}, Cost: 18000}); err == nil {
			s.count("fleet_renewals", 1)
		}
		if _, err := repo.SaveRenewal(s.ctx, c, "", fleet.Renewal{VehicleID: v.ID, Kind: "registration", Title: "Fitness certificate", DueDate: day(s.today.AddDate(0, 0, 90+i*30)), Repeat: fleet.Interval{Every: 2, Unit: "years"}}); err == nil {
			s.count("fleet_renewals", 1)
		}
	}
	if len(made) > 0 {
		if _, err := repo.SubmitInspection(s.ctx, c, fleet.Inspection{VehicleID: made[0].ID, Date: today, Items: []fleet.InspectionItem{
			{Label: "Tyres and wheels", Result: "fail", Note: "Rear left tyre worn to the wear bars"}, {Label: "Brakes", Result: "pass"}, {Label: "Lights and indicators", Result: "pass"}, {Label: "Fluid levels (oil, coolant)", Result: "pass"}}}); err != nil {
			s.warn("fleet inspection", err)
		} else {
			s.count("fleet_inspections", 1)
		}
		if len(made) > 1 {
			if _, err := repo.SaveIssue(s.ctx, c, "", fleet.Issue{VehicleID: made[1].ID, Title: "AC not cooling", Description: "Driver reports weak airflow.", Priority: "low"}); err == nil {
				s.count("fleet_issues", 1)
			}
		}
	}
	// Post two fuel bills to Accounting so the link is visible.
	if cats, err := repo.ExpenseCategories(s.ctx, c); err == nil && len(cats) > 0 {
		cat := cats[0].ID
		for _, cc := range cats {
			if strings.Contains(strings.ToLower(cc.Name), "fuel") || strings.Contains(strings.ToLower(cc.Name), "vehicle") {
				cat = cc.ID
				break
			}
		}
		for _, id := range fuelIDs[:min(2, len(fuelIDs))] {
			if _, err := repo.PostExpense(s.ctx, c, "fuel", id, cat); err != nil {
				s.warn("fleet expense", err)
			}
		}
	}
}
