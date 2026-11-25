package hr

import (
	"context"
	"sort"
	"time"
)

// NextOccurrence returns the next date on or after today that falls on the
// month and day of d. 29 February maps to 28 February in non-leap years.
func NextOccurrence(d, today time.Time) time.Time {
	for y := today.Year(); y <= today.Year()+1; y++ {
		day := d.Day()
		if d.Month() == time.February && day == 29 && !isLeap(y) {
			day = 28
		}
		t := time.Date(y, d.Month(), day, 0, 0, 0, 0, time.UTC)
		if !t.Before(today) {
			return t
		}
	}
	return today
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

// Celebration is a birthday or work anniversary.
type Celebration struct {
	EmployeeID string `json:"employee_id"`
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"` // birthday | anniversary
	Date       string `json:"date"`
	Years      int    `json:"years,omitempty"` // for anniversaries
}

// Celebrations lists birthdays and work anniversaries in the next `days` days.
// Visible to every member; only the name and the day are exposed (no birth year).
func (r *Repository) Celebrations(ctx context.Context, orgID string, days int) ([]Celebration, error) {
	today := r.today(ctx, orgID)
	rows, err := r.pool.Query(ctx, `SELECT id::text, coalesce(user_id::text,''), trim(first_name||' '||last_name), coalesce(to_char(dob,'YYYY-MM-DD'),''), to_char(join_date,'YYYY-MM-DD')
		FROM adrient.hr_employees WHERE org_id=$1 AND status<>'exited'`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	end := today.AddDate(0, 0, days)
	out := []Celebration{}
	for rows.Next() {
		var id, uid, name, dob, join string
		if err := rows.Scan(&id, &uid, &name, &dob, &join); err != nil {
			return nil, err
		}
		if dob != "" {
			if n := NextOccurrence(parseDate(dob), today); !n.After(end) {
				out = append(out, Celebration{EmployeeID: id, UserID: uid, Name: name, Kind: "birthday", Date: fmtDate(n)})
			}
		}
		j := parseDate(join)
		if n := NextOccurrence(j, today); !n.After(end) && n.Year() > j.Year() {
			out = append(out, Celebration{EmployeeID: id, UserID: uid, Name: name, Kind: "anniversary", Date: fmtDate(n), Years: n.Year() - j.Year()})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date < out[j].Date || (out[i].Date == out[j].Date && out[i].Name < out[j].Name)
	})
	return out, rows.Err()
}

// OnLeaveToday lists approved leave covering today, limited to what the caller
// may see (HR: everyone; others: their team, same rules as the team calendar).
func (r *Repository) OnLeaveToday(ctx context.Context, a Actor) ([]TeamLeave, error) {
	today := fmtDate(r.today(ctx, a.OrgID))
	m, err := r.TeamCalendar(ctx, a, today, today)
	if err != nil {
		return nil, err
	}
	out := []TeamLeave{}
	for _, l := range m["leaves"].([]TeamLeave) {
		if l.Status == "approved" {
			out = append(out, l)
		}
	}
	return out, nil
}

// Count is a labelled number.
type Count struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// HRDashboard is the HR admin overview.
type HRDashboard struct {
	Headcount     int           `json:"headcount"`
	OnNotice      int           `json:"on_notice"`
	ByDepartment  []Count       `json:"by_department"`
	ByType        []Count       `json:"by_type"`
	ByLocation    []Count       `json:"by_location"`
	Joiners       []Employee    `json:"joiners"`
	Leavers       []Employee    `json:"leavers"`
	AttritionPct  float64       `json:"attrition_pct"`
	Leavers12m    int           `json:"leavers_12m"`
	OnLeaveToday  []TeamLeave   `json:"on_leave_today"`
	PendingLeave  int           `json:"pending_leave"`
	PendingRegs   int           `json:"pending_regularisations"`
	Celebrations  []Celebration `json:"celebrations"`
	PayrollPeriod string        `json:"payroll_period"`
	PayrollStatus string        `json:"payroll_status"`
	NoSalary      int           `json:"employees_without_salary"`
}

func (r *Repository) counts(ctx context.Context, q string, args ...any) ([]Count, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Count{}
	for rows.Next() {
		var c Count
		if err := rows.Scan(&c.Label, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// HRDashboard builds the admin overview (HR only).
func (r *Repository) HRDashboard(ctx context.Context, a Actor) (HRDashboard, error) {
	var d HRDashboard
	if err := a.requireHR(); err != nil {
		return d, err
	}
	today := r.today(ctx, a.OrgID)
	t := fmtDate(today)
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status<>'exited'), count(*) FILTER (WHERE status='notice') FROM adrient.hr_employees WHERE org_id=$1`, a.OrgID).Scan(&d.Headcount, &d.OnNotice); err != nil {
		return d, err
	}
	var err error
	if d.ByDepartment, err = r.counts(ctx, `SELECT coalesce(dp.name,'No department'), count(*)::int FROM adrient.hr_employees e LEFT JOIN adrient.departments dp ON dp.id=e.department_id
		WHERE e.org_id=$1 AND e.status<>'exited' GROUP BY 1 ORDER BY 2 DESC, 1`, a.OrgID); err != nil {
		return d, err
	}
	if d.ByType, err = r.counts(ctx, `SELECT employment_type, count(*)::int FROM adrient.hr_employees WHERE org_id=$1 AND status<>'exited' GROUP BY 1 ORDER BY 2 DESC`, a.OrgID); err != nil {
		return d, err
	}
	if d.ByLocation, err = r.counts(ctx, `SELECT coalesce(l.name,'No location'), count(*)::int FROM adrient.hr_employees e LEFT JOIN adrient.hr_locations l ON l.id=e.location_id
		WHERE e.org_id=$1 AND e.status<>'exited' GROUP BY 1 ORDER BY 2 DESC, 1`, a.OrgID); err != nil {
		return d, err
	}
	d.Joiners, d.Leavers = []Employee{}, []Employee{}
	for _, q := range []struct {
		dst  *[]Employee
		cond string
	}{
		{&d.Joiners, `e.join_date >= $2::date - 30 AND e.join_date <= $2::date`},
		{&d.Leavers, `e.exit_date >= $2::date - 30 AND e.exit_date <= $2::date`},
	} {
		rows, err := r.pool.Query(ctx, `SELECT `+empCols+empFrom+` WHERE e.org_id=$1 AND `+q.cond+` ORDER BY e.first_name`, a.OrgID, t)
		if err != nil {
			return d, err
		}
		for rows.Next() {
			e, err := scanEmployee(rows)
			if err != nil {
				rows.Close()
				return d, err
			}
			*q.dst = append(*q.dst, redact(e))
		}
		rows.Close()
	}
	var before int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_employees WHERE org_id=$1 AND join_date <= $2::date - 365 AND (exit_date IS NULL OR exit_date > $2::date - 365)`, a.OrgID, t).Scan(&before)
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_employees WHERE org_id=$1 AND exit_date > $2::date - 365 AND exit_date <= $2::date`, a.OrgID, t).Scan(&d.Leavers12m)
	d.AttritionPct = AttritionPct(d.Leavers12m, before, d.Headcount)
	if d.OnLeaveToday, err = r.OnLeaveToday(ctx, a); err != nil {
		return d, err
	}
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_leave_requests WHERE org_id=$1 AND status='pending'`, a.OrgID).Scan(&d.PendingLeave)
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_regularisations WHERE org_id=$1 AND status='pending'`, a.OrgID).Scan(&d.PendingRegs)
	if d.Celebrations, err = r.Celebrations(ctx, a.OrgID, 30); err != nil {
		return d, err
	}
	d.PayrollPeriod = today.Format("2006-01")
	d.PayrollStatus = "not started"
	_ = r.pool.QueryRow(ctx, `SELECT status FROM adrient.hr_payroll_runs WHERE org_id=$1 AND period=$2`, a.OrgID, d.PayrollPeriod).Scan(&d.PayrollStatus)
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_employees e WHERE e.org_id=$1 AND e.status<>'exited' AND NOT EXISTS (SELECT 1 FROM adrient.hr_employee_salary s WHERE s.employee_id=e.id)`, a.OrgID).Scan(&d.NoSalary)
	return d, nil
}

// AttritionPct is leavers over the average headcount of the period, as a percent.
func AttritionPct(leavers, headStart, headEnd int) float64 {
	avg := float64(headStart+headEnd) / 2
	if avg <= 0 {
		return 0
	}
	return round2(float64(leavers) / avg * 100)
}

// ManagerDashboard is the team overview for a manager.
type ManagerDashboard struct {
	TeamSize     int         `json:"team_size"`
	Team         []Employee  `json:"team"`
	OnLeaveToday []TeamLeave `json:"on_leave_today"`
	PendingLeave int         `json:"pending_leave"`
	PendingRegs  int         `json:"pending_regularisations"`
	Present      int         `json:"present_today"`
	Absent       int         `json:"not_in_yet"`
}

// ManagerDashboard summarises the caller's direct reports.
func (r *Repository) ManagerDashboard(ctx context.Context, a Actor) (ManagerDashboard, error) {
	d := ManagerDashboard{Team: []Employee{}, OnLeaveToday: []TeamLeave{}}
	if !a.IsManager && !a.IsHR {
		return d, forbidden("managers and HR only")
	}
	if a.EmployeeID == "" {
		return d, nil
	}
	team, err := r.ListEmployees(ctx, a, EmployeeFilter{ManagerID: a.EmployeeID})
	if err != nil {
		return d, err
	}
	d.Team, d.TeamSize = team, len(team)
	today := r.today(ctx, a.OrgID)
	for _, e := range team {
		days, err := r.classifyRange(ctx, a.OrgID, e.ID, today, today)
		if err != nil || len(days) == 0 {
			continue
		}
		switch days[0].Status {
		case DayPresent, DayHalf, DayInProgress:
			if days[0].Punches > 0 {
				d.Present++
			} else {
				d.Absent++
			}
		case DayAbsent, DayMissingOut:
			d.Absent++
		}
	}
	for _, l := range func() []TeamLeave { l, _ := r.OnLeaveToday(ctx, a); return l }() {
		for _, e := range team {
			if e.ID == l.EmployeeID {
				d.OnLeaveToday = append(d.OnLeaveToday, l)
			}
		}
	}
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_leave_requests q JOIN adrient.hr_employees e ON e.id=q.employee_id WHERE q.org_id=$1 AND q.status='pending' AND e.manager_id=$2::uuid`, a.OrgID, a.EmployeeID).Scan(&d.PendingLeave)
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_regularisations g JOIN adrient.hr_employees e ON e.id=g.employee_id WHERE g.org_id=$1 AND g.status='pending' AND e.manager_id=$2::uuid`, a.OrgID, a.EmployeeID).Scan(&d.PendingRegs)
	return d, nil
}

// MyHR is the employee self-service home.
type MyHR struct {
	Roles        map[string]bool `json:"roles"`
	Employee     *Employee       `json:"employee"`
	Balances     []Balance       `json:"balances"`
	Today        TodayState      `json:"today"`
	MyLeave      []LeaveRequest  `json:"my_leave"`
	Payslips     []Payslip       `json:"payslips"`
	Documents    []Document      `json:"documents"`
	Manager      *Employee       `json:"manager"`
	Peers        []Employee      `json:"team"`
	Reports      []Employee      `json:"reports"`
	Holidays     []Holiday       `json:"holidays"`
	Celebrations []Celebration   `json:"celebrations"`
	OnLeaveToday []TeamLeave     `json:"on_leave_today"`
	PendingToMe  int             `json:"pending_approvals"`
}

// MyHR assembles the caller's home page.
func (r *Repository) MyHR(ctx context.Context, a Actor) (MyHR, error) {
	m := MyHR{Roles: map[string]bool{"hr": a.IsHR, "manager": a.IsManager, "employee": a.EmployeeID != ""},
		Balances: []Balance{}, MyLeave: []LeaveRequest{}, Payslips: []Payslip{}, Documents: []Document{}, Peers: []Employee{}, Reports: []Employee{},
		Holidays: []Holiday{}, Celebrations: []Celebration{}, OnLeaveToday: []TeamLeave{}}
	var err error
	if m.Today, err = r.Today(ctx, a); err != nil {
		return m, err
	}
	if m.Celebrations, err = r.Celebrations(ctx, a.OrgID, 30); err != nil {
		return m, err
	}
	if a.IsManager || a.IsHR {
		var pl, pr int
		if a.IsHR {
			_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_leave_requests WHERE org_id=$1 AND status='pending'`, a.OrgID).Scan(&pl)
			_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_regularisations WHERE org_id=$1 AND status='pending'`, a.OrgID).Scan(&pr)
		} else {
			_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_leave_requests q JOIN adrient.hr_employees e ON e.id=q.employee_id WHERE q.org_id=$1 AND q.status='pending' AND e.manager_id=$2::uuid`, a.OrgID, a.EmployeeID).Scan(&pl)
			_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_regularisations g JOIN adrient.hr_employees e ON e.id=g.employee_id WHERE g.org_id=$1 AND g.status='pending' AND e.manager_id=$2::uuid`, a.OrgID, a.EmployeeID).Scan(&pr)
		}
		m.PendingToMe = pl + pr
	}
	if a.EmployeeID == "" {
		return m, nil
	}
	e, err := r.GetEmployee(ctx, a, a.EmployeeID)
	if err != nil {
		return m, err
	}
	m.Employee = &e
	if m.Balances, err = r.Balances(ctx, a, a.EmployeeID); err != nil {
		return m, err
	}
	if m.MyLeave, err = r.ListLeaveRequests(ctx, a, LeaveFilter{Scope: "mine"}); err != nil {
		return m, err
	}
	if len(m.MyLeave) > 8 {
		m.MyLeave = m.MyLeave[:8]
	}
	if m.Payslips, err = r.MyPayslips(ctx, a); err != nil {
		return m, err
	}
	if m.Documents, err = r.ListDocuments(ctx, a, a.EmployeeID); err != nil {
		return m, err
	}
	if e.ManagerID != "" {
		if mg, err := r.GetEmployee(ctx, a, e.ManagerID); err == nil {
			m.Manager = &mg
			if peers, err := r.ListEmployees(ctx, a, EmployeeFilter{ManagerID: e.ManagerID}); err == nil {
				for _, p := range peers {
					if p.ID != e.ID {
						m.Peers = append(m.Peers, p)
					}
				}
			}
		}
	}
	if a.IsManager {
		if m.Reports, err = r.ListEmployees(ctx, a, EmployeeFilter{ManagerID: a.EmployeeID}); err != nil {
			return m, err
		}
	}
	today := r.today(ctx, a.OrgID)
	if hol, err := r.holidaysFor(ctx, r.pool, a.OrgID, e.LocationID, today, today.AddDate(0, 0, 120)); err == nil {
		for d, n := range hol {
			m.Holidays = append(m.Holidays, Holiday{Day: d, Name: n})
		}
		sort.Slice(m.Holidays, func(i, j int) bool { return m.Holidays[i].Day < m.Holidays[j].Day })
		if len(m.Holidays) > 5 {
			m.Holidays = m.Holidays[:5]
		}
	}
	m.OnLeaveToday, _ = r.OnLeaveToday(ctx, a)
	return m, nil
}

// OrgChartNode is one person on the HR org chart.
type OrgChartNode struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Designation  string `json:"designation"`
	Department   string `json:"department"`
	DepartmentID string `json:"department_id"`
	ManagerID    string `json:"manager_id"`
	Location     string `json:"location"`
}

// OrgChart lists active employees with their managers (any member). Directory
// fields only.
func (r *Repository) OrgChart(ctx context.Context, orgID string) ([]OrgChartNode, error) {
	rows, err := r.pool.Query(ctx, `SELECT e.id::text, coalesce(e.user_id::text,''), trim(e.first_name||' '||e.last_name), e.work_email, coalesce(g.name,''), coalesce(d.name,''),
		coalesce(e.department_id::text,''), coalesce(e.manager_id::text,''), coalesce(l.name,'')
		FROM adrient.hr_employees e LEFT JOIN adrient.hr_designations g ON g.id=e.designation_id LEFT JOIN adrient.departments d ON d.id=e.department_id
		LEFT JOIN adrient.hr_locations l ON l.id=e.location_id WHERE e.org_id=$1 AND e.status<>'exited' ORDER BY e.first_name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgChartNode{}
	for rows.Next() {
		var n OrgChartNode
		if err := rows.Scan(&n.ID, &n.UserID, &n.Name, &n.Email, &n.Designation, &n.Department, &n.DepartmentID, &n.ManagerID, &n.Location); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// AvailableUsers lists org users with no employee record yet (HR only), for linking.
func (r *Repository) AvailableUsers(ctx context.Context, a Actor) ([]map[string]string, error) {
	if err := a.requireHR(); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT u.id::text, u.display_name, u.email FROM adrient.users u WHERE u.org_id=$1 AND u.user_type IN ('member','external')
		AND NOT EXISTS (SELECT 1 FROM adrient.hr_employees e WHERE e.user_id=u.id) ORDER BY u.display_name LIMIT 500`, a.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var id, n, e string
		if err := rows.Scan(&id, &n, &e); err != nil {
			return nil, err
		}
		out = append(out, map[string]string{"id": id, "name": n, "email": e})
	}
	return out, rows.Err()
}
