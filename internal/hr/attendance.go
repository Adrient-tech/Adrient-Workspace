package hr

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Shift is a named work shift.
type Shift struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	ShiftDef
}

const shiftCols = `id::text, name, is_default, start_time, end_time, grace_minutes, early_minutes, full_day_mins, half_day_mins, ot_after_mins`

func scanShift(row pgx.Row) (Shift, error) {
	var s Shift
	err := row.Scan(&s.ID, &s.Name, &s.IsDefault, &s.Start, &s.End, &s.GraceMin, &s.EarlyMin, &s.FullMin, &s.HalfMin, &s.OTAfterMin)
	return s, err
}

// ListShifts lists shifts (any member).
func (r *Repository) ListShifts(ctx context.Context, orgID string) ([]Shift, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+shiftCols+` FROM adrient.hr_shifts WHERE org_id=$1 ORDER BY is_default DESC, name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Shift{}
	for rows.Next() {
		s, err := scanShift(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveShift creates or updates a shift (HR only).
func (r *Repository) SaveShift(ctx context.Context, a Actor, id string, s Shift) (Shift, error) {
	if err := a.requireHR(); err != nil {
		return s, err
	}
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return s, invalid("a shift name is required")
	}
	if !ValidHM(s.Start) || !ValidHM(s.End) {
		return s, invalid("start and end must be HH:MM")
	}
	st, _ := hm(s.Start)
	en, _ := hm(s.End)
	if en <= st {
		return s, invalid("overnight shifts are not supported: the end time must be after the start time")
	}
	if s.GraceMin < 0 || s.EarlyMin < 0 || s.FullMin <= 0 || s.HalfMin <= 0 || s.HalfMin > s.FullMin || s.OTAfterMin < s.FullMin {
		return s, invalid("check the grace, half-day, full-day and overtime minutes")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return s, err
	}
	defer tx.Rollback(ctx)
	if s.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE adrient.hr_shifts SET is_default=false WHERE org_id=$1`, a.OrgID); err != nil {
			return s, err
		}
	}
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO adrient.hr_shifts (org_id, name, start_time, end_time, grace_minutes, early_minutes, full_day_mins, half_day_mins, ot_after_mins, is_default)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id::text`, a.OrgID, s.Name, s.Start, s.End, s.GraceMin, s.EarlyMin, s.FullMin, s.HalfMin, s.OTAfterMin, s.IsDefault).Scan(&s.ID)
	} else {
		if !isUUID(id) {
			return s, ErrNotFound
		}
		var tag int64
		err = tx.QueryRow(ctx, `WITH u AS (UPDATE adrient.hr_shifts SET name=$3, start_time=$4, end_time=$5, grace_minutes=$6, early_minutes=$7, full_day_mins=$8, half_day_mins=$9,
			ot_after_mins=$10, is_default=$11 WHERE org_id=$1 AND id=$2::uuid RETURNING 1) SELECT count(*) FROM u`, a.OrgID, id, s.Name, s.Start, s.End, s.GraceMin, s.EarlyMin, s.FullMin, s.HalfMin, s.OTAfterMin, s.IsDefault).Scan(&tag)
		if err == nil && tag == 0 {
			return s, ErrNotFound
		}
		s.ID = id
	}
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return s, conflict("a shift named %q exists", s.Name)
		}
		return s, err
	}
	return s, tx.Commit(ctx)
}

// DeleteShift removes a shift (HR only). Employees on it fall back to the default.
func (r *Repository) DeleteShift(ctx context.Context, a Actor, id string) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	if !isUUID(id) {
		return ErrNotFound
	}
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.hr_employees SET shift_id=NULL WHERE org_id=$1 AND shift_id=$2::uuid`, a.OrgID, id)
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.hr_shifts WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id)
	return err
}

// SetRoster assigns a shift (or a day off when shiftID is "off") over a date
// range for employees. HR may roster anyone; a manager their direct reports.
func (r *Repository) SetRoster(ctx context.Context, a Actor, empIDs []string, from, to, shiftID string) (int, error) {
	var err error
	if from, err = validDate(from); err != nil {
		return 0, err
	}
	if to, err = validDate(to); err != nil {
		return 0, err
	}
	if to < from || parseDate(to).Sub(parseDate(from)) > 92*24*time.Hour {
		return 0, invalid("choose a range of up to 92 days")
	}
	if len(empIDs) == 0 || len(empIDs) > 500 {
		return 0, invalid("choose between 1 and 500 employees")
	}
	if shiftID != "off" && shiftID != "clear" {
		var ok bool
		if !isUUID(shiftID) || r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.hr_shifts WHERE id=$1::uuid AND org_id=$2)`, shiftID, a.OrgID).Scan(&ok) != nil || !ok {
			return 0, invalid("unknown shift")
		}
	}
	n := 0
	for _, id := range empIDs {
		if !a.IsHR && !r.isManagerOf(ctx, a, id) {
			return n, forbidden("you can only roster your direct reports")
		}
		if _, err := r.empLite(ctx, r.pool, a.OrgID, id); err != nil {
			return n, err
		}
		for d := parseDate(from); !d.After(parseDate(to)); d = d.AddDate(0, 0, 1) {
			switch shiftID {
			case "clear":
				_, err = r.pool.Exec(ctx, `DELETE FROM adrient.hr_roster WHERE employee_id=$1::uuid AND day=$2::date`, id, fmtDate(d))
			case "off":
				_, err = r.pool.Exec(ctx, `INSERT INTO adrient.hr_roster (org_id, employee_id, day, shift_id) VALUES ($1,$2::uuid,$3::date,NULL)
					ON CONFLICT (employee_id, day) DO UPDATE SET shift_id=NULL`, a.OrgID, id, fmtDate(d))
			default:
				_, err = r.pool.Exec(ctx, `INSERT INTO adrient.hr_roster (org_id, employee_id, day, shift_id) VALUES ($1,$2::uuid,$3::date,$4::uuid)
					ON CONFLICT (employee_id, day) DO UPDATE SET shift_id=EXCLUDED.shift_id`, a.OrgID, id, fmtDate(d), shiftID)
			}
			if err != nil {
				return n, err
			}
			n++
		}
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "roster", "roster", "", fmt.Sprintf("%d employees %s..%s %s", len(empIDs), from, to, shiftID))
	return n, nil
}

// RosterRow is one roster override.
type RosterRow struct {
	EmployeeID string `json:"employee_id"`
	Day        string `json:"day"`
	ShiftID    string `json:"shift_id"` // "" = day off
}

// ListRoster returns overrides in a window for employees the caller may see.
func (r *Repository) ListRoster(ctx context.Context, a Actor, from, to string) ([]RosterRow, error) {
	var err error
	if from, err = validDate(from); err != nil {
		return nil, err
	}
	if to, err = validDate(to); err != nil {
		return nil, err
	}
	q := `SELECT r.employee_id::text, to_char(r.day,'YYYY-MM-DD'), coalesce(r.shift_id::text,'') FROM adrient.hr_roster r JOIN adrient.hr_employees e ON e.id=r.employee_id
		WHERE r.org_id=$1 AND r.day BETWEEN $2::date AND $3::date`
	args := []any{a.OrgID, from, to}
	if !a.IsHR {
		if a.EmployeeID == "" {
			return []RosterRow{}, nil
		}
		args = append(args, a.EmployeeID)
		q += ` AND (r.employee_id=$4::uuid OR e.manager_id=$4::uuid)`
	}
	rows, err := r.pool.Query(ctx, q+` ORDER BY r.day LIMIT 5000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RosterRow{}
	for rows.Next() {
		var x RosterRow
		if err := rows.Scan(&x.EmployeeID, &x.Day, &x.ShiftID); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// shiftFor resolves the shift for one employee date: roster, employee shift, default.
// ok=false means a rostered day off.
func (r *Repository) shiftFor(ctx context.Context, orgID, empID, day string) (ShiftDef, bool, error) {
	var shiftID *string
	var found bool
	err := r.pool.QueryRow(ctx, `SELECT true, shift_id::text FROM adrient.hr_roster WHERE employee_id=$1::uuid AND day=$2::date`, empID, day).Scan(&found, &shiftID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return DefaultShift, true, err
	}
	if found && shiftID == nil {
		return DefaultShift, false, nil
	}
	id := ""
	if found {
		id = *shiftID
	}
	var s Shift
	var q string
	var arg []any
	switch {
	case id != "":
		q, arg = `SELECT `+shiftCols+` FROM adrient.hr_shifts WHERE id=$1::uuid`, []any{id}
	default:
		q, arg = `SELECT `+shiftCols+` FROM adrient.hr_shifts WHERE id=coalesce((SELECT shift_id FROM adrient.hr_employees WHERE id=$2::uuid),
			(SELECT id FROM adrient.hr_shifts WHERE org_id=$1 AND is_default LIMIT 1))`, []any{orgID, empID}
	}
	s, err = scanShift(r.pool.QueryRow(ctx, q, arg...))
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultShift, true, nil
	}
	return s.ShiftDef, true, err
}

// ---- punches ----

// Punch is one raw attendance event.
type Punch struct {
	At     string `json:"at"`
	Source string `json:"source"`
}

// TodayState is what the web check-in widget shows.
type TodayState struct {
	Date      string    `json:"date"`
	Punches   []Punch   `json:"punches"`
	CheckedIn bool      `json:"checked_in"`
	Day       DayResult `json:"day"`
	Geofence  bool      `json:"geofence"`
	Shift     ShiftDef  `json:"shift"`
}

// CheckInOut records a web punch for the caller: the first of the day is the
// check-in, then they alternate. When the geofence is enabled the position must
// be supplied and inside the radius.
func (r *Repository) CheckInOut(ctx context.Context, a Actor, lat, lng *float64) (TodayState, error) {
	if a.EmployeeID == "" {
		return TodayState{}, forbidden("no employee record is linked to your account")
	}
	st, err := r.GetSettings(ctx, a.OrgID)
	if err != nil {
		return TodayState{}, err
	}
	if st.GeofenceEnabled {
		if lat == nil || lng == nil {
			return TodayState{}, invalid("your location is required to check in (allow location access)")
		}
		if d := HaversineMetres(*lat, *lng, st.GeofenceLat, st.GeofenceLng); d > float64(st.GeofenceRadiusM) {
			return TodayState{}, forbidden("you are %.0f m from the office; check-in is allowed within %d m", d, st.GeofenceRadiusM)
		}
	}
	loc := r.location(ctx, a.OrgID)
	now := r.now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var last *time.Time
	_ = r.pool.QueryRow(ctx, `SELECT max(punched_at) FROM adrient.hr_punches WHERE employee_id=$1::uuid AND day=$2::date`, a.EmployeeID, fmtDate(day)).Scan(&last)
	if last != nil && now.Sub(*last) < 30*time.Second {
		return TodayState{}, conflict("you just punched; wait a moment")
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.hr_punches (org_id, employee_id, punched_at, day, source, lat, lng) VALUES ($1,$2::uuid,$3,$4::date,'web',$5,$6)
		ON CONFLICT DO NOTHING`, a.OrgID, a.EmployeeID, now.UTC().Truncate(time.Second), fmtDate(day), lat, lng); err != nil {
		return TodayState{}, err
	}
	return r.Today(ctx, a)
}

// Today returns the caller's punches and classified day.
func (r *Repository) Today(ctx context.Context, a Actor) (TodayState, error) {
	var ts TodayState
	st, err := r.GetSettings(ctx, a.OrgID)
	if err != nil {
		return ts, err
	}
	ts.Geofence = st.GeofenceEnabled
	today := r.today(ctx, a.OrgID)
	ts.Date = fmtDate(today)
	ts.Punches = []Punch{}
	if a.EmployeeID == "" {
		return ts, nil
	}
	days, err := r.classifyRange(ctx, a.OrgID, a.EmployeeID, today, today)
	if err != nil {
		return ts, err
	}
	if len(days) == 1 {
		ts.Day = days[0]
	}
	loc := r.location(ctx, a.OrgID)
	rows, err := r.pool.Query(ctx, `SELECT punched_at, source FROM adrient.hr_punches WHERE employee_id=$1::uuid AND day=$2::date ORDER BY punched_at`, a.EmployeeID, ts.Date)
	if err != nil {
		return ts, err
	}
	defer rows.Close()
	for rows.Next() {
		var t time.Time
		var src string
		if err := rows.Scan(&t, &src); err != nil {
			return ts, err
		}
		ts.Punches = append(ts.Punches, Punch{At: t.In(loc).Format("15:04:05"), Source: src})
	}
	ts.CheckedIn = len(ts.Punches)%2 == 1
	sh, _, _ := r.shiftFor(ctx, a.OrgID, a.EmployeeID, ts.Date)
	ts.Shift = sh
	return ts, rows.Err()
}

// classifyRange classifies every date in [from, to] for one employee.
func (r *Repository) classifyRange(ctx context.Context, orgID, empID string, from, to time.Time) ([]DayResult, error) {
	e, err := r.empLite(ctx, r.pool, orgID, empID)
	if err != nil {
		return nil, err
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	loc := r.location(ctx, orgID)
	today := r.today(ctx, orgID)
	hol, err := r.holidaysFor(ctx, r.pool, orgID, e.LocationID, from, to)
	if err != nil {
		return nil, err
	}
	wo := weeklyOffSet(st)
	// punches by day
	punches := map[string][]time.Time{}
	rows, err := r.pool.Query(ctx, `SELECT to_char(day,'YYYY-MM-DD'), punched_at FROM adrient.hr_punches WHERE employee_id=$1::uuid AND day BETWEEN $2::date AND $3::date`, empID, fmtDate(from), fmtDate(to))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d string
		var t time.Time
		if err := rows.Scan(&d, &t); err != nil {
			rows.Close()
			return nil, err
		}
		punches[d] = append(punches[d], t.In(loc))
	}
	rows.Close()
	// approved leave by day
	leave := map[string]string{}
	lrows, err := r.pool.Query(ctx, `SELECT to_char(from_date,'YYYY-MM-DD'), to_char(to_date,'YYYY-MM-DD'), half_day FROM adrient.hr_leave_requests
		WHERE employee_id=$1::uuid AND status='approved' AND from_date<=$3::date AND to_date>=$2::date`, empID, fmtDate(from), fmtDate(to))
	if err != nil {
		return nil, err
	}
	for lrows.Next() {
		var f, t, h string
		if err := lrows.Scan(&f, &t, &h); err != nil {
			lrows.Close()
			return nil, err
		}
		for d := parseDate(f); !d.After(parseDate(t)); d = d.AddDate(0, 0, 1) {
			switch h {
			case "":
				leave[fmtDate(d)] = "full"
			default:
				if leave[fmtDate(d)] == "" {
					leave[fmtDate(d)] = h
				} else {
					leave[fmtDate(d)] = "full"
				}
			}
		}
	}
	lrows.Close()
	join := parseDate(e.JoinDate)
	var exit *time.Time
	if e.ExitDate != "" {
		x := parseDate(e.ExitDate)
		exit = &x
	}
	out := []DayResult{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		k := fmtDate(d)
		sh, working, err := r.shiftFor(ctx, orgID, empID, k)
		if err != nil {
			return nil, err
		}
		in := DayInput{Date: d, Employed: !d.Before(join) && (exit == nil || !d.After(*exit)), Holiday: hol[k], Leave: leave[k],
			Punches: punches[k], Shift: sh, Today: today, OTEnabled: st.OTEnabled, Location: loc}
		// A rostered working day on a weekly off is honoured; otherwise the weekly off applies.
		in.WeeklyOff = !working || (wo[int(d.Weekday())] && !r.rosteredOn(ctx, empID, k))
		out = append(out, ClassifyDay(in))
	}
	return out, nil
}

func (r *Repository) rosteredOn(ctx context.Context, empID, day string) bool {
	var ok bool
	_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.hr_roster WHERE employee_id=$1::uuid AND day=$2::date AND shift_id IS NOT NULL)`, empID, day).Scan(&ok)
	return ok
}

func monthRange(period string) (time.Time, time.Time, error) {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return t, t, invalid("month must be YYYY-MM")
	}
	return t, t.AddDate(0, 1, -1), nil
}

// Timesheet is a month of classified days for one employee.
type Timesheet struct {
	EmployeeID string       `json:"employee_id"`
	Name       string       `json:"name"`
	Period     string       `json:"period"`
	Days       []DayResult  `json:"days"`
	Summary    MonthSummary `json:"summary"`
}

// GetTimesheet returns the daily timesheet (HR, the employee, or their manager).
func (r *Repository) GetTimesheet(ctx context.Context, a Actor, empID, period string) (Timesheet, error) {
	var ts Timesheet
	if !r.canSee(ctx, a, empID) {
		return ts, forbidden("not allowed")
	}
	from, to, err := monthRange(period)
	if err != nil {
		return ts, err
	}
	e, err := r.empLite(ctx, r.pool, a.OrgID, empID)
	if err != nil {
		return ts, err
	}
	days, err := r.classifyRange(ctx, a.OrgID, empID, from, to)
	if err != nil {
		return ts, err
	}
	return Timesheet{EmployeeID: empID, Name: e.Name, Period: period, Days: days, Summary: Summarise(days)}, nil
}

// MonthlyRow is one employee in the monthly attendance report.
type MonthlyRow struct {
	EmployeeID string  `json:"employee_id"`
	EmpCode    string  `json:"emp_code"`
	Name       string  `json:"name"`
	Present    float64 `json:"present"`
	HalfDays   float64 `json:"half_days"`
	Absent     float64 `json:"absent"`
	Leave      float64 `json:"leave"`
	Holidays   float64 `json:"holidays"`
	WeeklyOffs float64 `json:"weekly_offs"`
	Late       int     `json:"late"`
	EarlyOut   int     `json:"early_out"`
	WorkedHrs  float64 `json:"worked_hours"`
	OTHrs      float64 `json:"ot_hours"`
	LOP        float64 `json:"lop"`
}

// MonthlyReport covers HR: every active employee; a manager: their reports.
func (r *Repository) MonthlyReport(ctx context.Context, a Actor, period string) ([]MonthlyRow, error) {
	return r.MonthlyReportPage(ctx, a, period, "", 0, 0)
}

// MonthlyReportPage is MonthlyReport for one window of employees (by name);
// limit 0 = everyone. q matches the name or code.
func (r *Repository) MonthlyReportPage(ctx context.Context, a Actor, period, search string, limit, offset int) ([]MonthlyRow, error) {
	from, to, err := monthRange(period)
	if err != nil {
		return nil, err
	}
	q := `SELECT id::text, emp_code, trim(first_name||' '||last_name) FROM adrient.hr_employees WHERE org_id=$1 AND join_date<=$3::date AND (exit_date IS NULL OR exit_date>=$2::date)`
	args := []any{a.OrgID, fmtDate(from), fmtDate(to)}
	if !a.IsHR {
		if !a.IsManager {
			return nil, forbidden("managers and HR only")
		}
		args = append(args, a.EmployeeID)
		q += ` AND manager_id=$4::uuid`
	}
	if s := strings.TrimSpace(search); s != "" {
		args = append(args, "%"+escapeLike(strings.ToLower(s))+"%")
		q += fmt.Sprintf(` AND (lower(first_name||' '||last_name) LIKE $%[1]d OR lower(emp_code) LIKE $%[1]d)`, len(args))
	}
	page := ""
	if limit > 0 {
		page = fmt.Sprintf(` LIMIT %d OFFSET %d`, limit, max(offset, 0))
	}
	rows, err := r.pool.Query(ctx, q+` ORDER BY first_name, last_name, id`+page, args...)
	if err != nil {
		return nil, err
	}
	var emps []MonthlyRow
	for rows.Next() {
		var m MonthlyRow
		if err := rows.Scan(&m.EmployeeID, &m.EmpCode, &m.Name); err != nil {
			rows.Close()
			return nil, err
		}
		emps = append(emps, m)
	}
	rows.Close()
	out := []MonthlyRow{}
	for _, m := range emps {
		days, err := r.classifyRange(ctx, a.OrgID, m.EmployeeID, from, to)
		if err != nil {
			return nil, err
		}
		s := Summarise(days)
		m.Present, m.HalfDays, m.Absent, m.Leave, m.Holidays, m.WeeklyOffs = s.Present, s.HalfDays, s.Absent, s.Leave, s.Holidays, s.WeeklyOffs
		m.Late, m.EarlyOut, m.WorkedHrs, m.OTHrs, m.LOP = s.LateCount, s.EarlyCount, round2(float64(s.WorkedMin)/60), round2(float64(s.OTMin)/60), s.LOP
		out = append(out, m)
	}
	return out, nil
}

// MonthlyReportCSV renders the report.
func (r *Repository) MonthlyReportCSV(ctx context.Context, a Actor, period string, w io.Writer) error {
	rows, err := r.MonthlyReport(ctx, a, period)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"emp_code", "name", "present", "half_days", "absent", "leave", "holidays", "weekly_offs", "late", "early_out", "worked_hours", "ot_hours", "lop_days"})
	f := func(v float64) string { return fmt.Sprintf("%.2f", v) }
	for _, m := range rows {
		_ = cw.Write([]string{csvSafe(m.EmpCode), csvSafe(m.Name), f(m.Present), f(m.HalfDays), f(m.Absent), f(m.Leave), f(m.Holidays), f(m.WeeklyOffs),
			fmt.Sprint(m.Late), fmt.Sprint(m.EarlyOut), f(m.WorkedHrs), f(m.OTHrs), f(m.LOP)})
	}
	cw.Flush()
	return cw.Error()
}

// ---- import of biometric logs ----

// PunchImportResult summarises a biometric CSV import.
type PunchImportResult struct {
	Imported   int              `json:"imported"`
	Duplicates int              `json:"duplicates"`
	Errors     []ImportRowError `json:"errors"`
}

// ImportPunchesCSV loads device logs: columns emp_code and timestamp (extra
// columns are ignored). HR only.
func (r *Repository) ImportPunchesCSV(ctx context.Context, a Actor, in io.Reader) (PunchImportResult, error) {
	res := PunchImportResult{Errors: []ImportRowError{}}
	if err := a.requireHR(); err != nil {
		return res, err
	}
	cr := csv.NewReader(in)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return res, invalid("the file is empty or not CSV")
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))] = i
	}
	ci, ok1 := col["emp_code"]
	ti, ok2 := col["timestamp"]
	if !ok2 {
		ti, ok2 = col["datetime"]
	}
	if !ok1 || !ok2 {
		return res, invalid("the file needs emp_code and timestamp columns")
	}
	loc := r.location(ctx, a.OrgID)
	codes := map[string]string{}
	row := 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		row++
		if row > 100001 {
			res.Errors = append(res.Errors, ImportRowError{row, "stopped after 100000 rows"})
			break
		}
		if err != nil || len(rec) <= ci || len(rec) <= ti {
			res.Errors = append(res.Errors, ImportRowError{row, "unreadable row"})
			continue
		}
		code := strings.ToLower(strings.TrimSpace(rec[ci]))
		id, seen := codes[code]
		if !seen {
			_ = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.hr_employees WHERE org_id=$1 AND lower(emp_code)=$2`, a.OrgID, code).Scan(&id)
			codes[code] = id
		}
		if id == "" {
			if len(res.Errors) < 100 {
				res.Errors = append(res.Errors, ImportRowError{row, fmt.Sprintf("unknown employee code %q", rec[ci])})
			}
			continue
		}
		t, err := ParseLogTime(rec[ti], loc)
		if err != nil {
			if len(res.Errors) < 100 {
				res.Errors = append(res.Errors, ImportRowError{row, err.Error()})
			}
			continue
		}
		l := t.In(loc)
		day := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
		tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.hr_punches (org_id, employee_id, punched_at, day, source) VALUES ($1,$2::uuid,$3,$4::date,'biometric') ON CONFLICT DO NOTHING`,
			a.OrgID, id, t.UTC().Truncate(time.Second), fmtDate(day))
		if err != nil {
			return res, err
		}
		if tag.RowsAffected() == 0 {
			res.Duplicates++
		} else {
			res.Imported++
		}
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "import", "punches", "", fmt.Sprintf("%d imported, %d duplicates", res.Imported, res.Duplicates))
	return res, nil
}

// ---- regularisation ----

// Regularisation is a request to correct a day's punches.
type Regularisation struct {
	ID           string `json:"id"`
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
	Day          string `json:"day"`
	In           string `json:"in_time"`
	Out          string `json:"out_time"`
	Reason       string `json:"reason"`
	Status       string `json:"status"`
	DecisionNote string `json:"decision_note"`
	CanDecide    bool   `json:"can_decide"`
	CanCancel    bool   `json:"can_cancel"`
}

const regCols = `g.id::text, g.employee_id::text, trim(e.first_name||' '||e.last_name), to_char(g.day,'YYYY-MM-DD'), g.in_time, g.out_time, g.reason, g.status, g.decision_note`
const regFrom = ` FROM adrient.hr_regularisations g JOIN adrient.hr_employees e ON e.id=g.employee_id`

func scanReg(row pgx.Row) (Regularisation, error) {
	var g Regularisation
	err := row.Scan(&g.ID, &g.EmployeeID, &g.EmployeeName, &g.Day, &g.In, &g.Out, &g.Reason, &g.Status, &g.DecisionNote)
	return g, err
}

// RequestRegularisation files a correction for the caller's own past day.
func (r *Repository) RequestRegularisation(ctx context.Context, a Actor, day, in, out, reason string) (Regularisation, error) {
	var g Regularisation
	if a.EmployeeID == "" {
		return g, forbidden("no employee record is linked to your account")
	}
	var err error
	if day, err = validDate(day); err != nil {
		return g, err
	}
	today := r.today(ctx, a.OrgID)
	if parseDate(day).After(today) {
		return g, invalid("you cannot regularise a future date")
	}
	if today.Sub(parseDate(day)) > 62*24*time.Hour {
		return g, invalid("regularisation is limited to the last 62 days")
	}
	if !ValidHM(in) || !ValidHM(out) {
		return g, invalid("in and out times must be HH:MM")
	}
	a1, _ := hm(in)
	a2, _ := hm(out)
	if a2 <= a1 {
		return g, invalid("out time must be after in time")
	}
	if strings.TrimSpace(reason) == "" {
		return g, invalid("a reason is required")
	}
	var dup bool
	_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.hr_regularisations WHERE employee_id=$1::uuid AND day=$2::date AND status='pending')`, a.EmployeeID, day).Scan(&dup)
	if dup {
		return g, conflict("a regularisation for %s is already pending", day)
	}
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_regularisations (org_id, employee_id, day, in_time, out_time, reason) VALUES ($1,$2::uuid,$3::date,$4,$5,$6) RETURNING id::text`,
		a.OrgID, a.EmployeeID, day, in, out, strings.TrimSpace(reason)).Scan(&id); err != nil {
		return g, err
	}
	if e, err := r.empLite(ctx, r.pool, a.OrgID, a.EmployeeID); err == nil {
		for _, u := range r.approverUsers(ctx, a.OrgID, e) {
			r.tell(ctx, a.OrgID, u, a.UserID, "hr_regularisation", "Attendance regularisation: "+e.Name, fmt.Sprintf("%s, %s to %s", day, in, out), "/hr/approvals")
		}
	}
	return r.getReg(ctx, a, id)
}

func (r *Repository) getReg(ctx context.Context, a Actor, id string) (Regularisation, error) {
	if !isUUID(id) {
		return Regularisation{}, ErrNotFound
	}
	g, err := scanReg(r.pool.QueryRow(ctx, `SELECT `+regCols+regFrom+` WHERE g.org_id=$1 AND g.id=$2::uuid`, a.OrgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	if err != nil {
		return g, err
	}
	if !r.canSee(ctx, a, g.EmployeeID) {
		return Regularisation{}, ErrNotFound
	}
	g.CanDecide = g.Status == "pending" && r.mayDecide(ctx, a, g.EmployeeID)
	g.CanCancel = g.Status == "pending" && a.EmployeeID == g.EmployeeID
	return g, nil
}

// ListRegularisations: scope mine | team (manager's reports, HR everyone).
func (r *Repository) ListRegularisations(ctx context.Context, a Actor, scope, status string) ([]Regularisation, error) {
	q := `SELECT ` + regCols + regFrom + ` WHERE g.org_id=$1`
	args := []any{a.OrgID}
	switch scope {
	case "", "mine":
		if a.EmployeeID == "" {
			return []Regularisation{}, nil
		}
		args = append(args, a.EmployeeID)
		q += ` AND g.employee_id=$2::uuid`
	case "team":
		if !a.IsHR {
			if !a.IsManager {
				return []Regularisation{}, nil
			}
			args = append(args, a.EmployeeID)
			q += ` AND e.manager_id=$2::uuid`
		}
	default:
		return nil, invalid("unknown scope")
	}
	if status != "" {
		args = append(args, status)
		q += fmt.Sprintf(` AND g.status=$%d`, len(args))
	}
	rows, err := r.pool.Query(ctx, q+` ORDER BY g.day DESC, g.created_at DESC LIMIT 300`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Regularisation{}
	for rows.Next() {
		g, err := scanReg(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].CanDecide = out[i].Status == "pending" && r.mayDecide(ctx, a, out[i].EmployeeID)
		out[i].CanCancel = out[i].Status == "pending" && a.EmployeeID == out[i].EmployeeID
	}
	return out, nil
}

// DecideRegularisation approves (adding the punches) or rejects a request.
func (r *Repository) DecideRegularisation(ctx context.Context, a Actor, id string, approve bool, note string) (Regularisation, error) {
	g, err := r.getReg(ctx, a, id)
	if err != nil {
		return g, err
	}
	if g.Status != "pending" {
		return g, conflict("this request is already %s", g.Status)
	}
	if !r.mayDecide(ctx, a, g.EmployeeID) {
		return g, forbidden("only the employee's manager or HR can decide this request")
	}
	status := "rejected"
	if approve {
		status = "approved"
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return g, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE adrient.hr_regularisations SET status=$3, decided_by=$4::uuid, decided_at=now(), decision_note=$5 WHERE org_id=$1 AND id=$2::uuid AND status='pending'`,
		a.OrgID, id, status, a.UserID, strings.TrimSpace(note))
	if err != nil {
		return g, err
	}
	if tag.RowsAffected() == 0 {
		return g, conflict("this request was already decided")
	}
	if approve {
		loc := r.location(ctx, a.OrgID)
		d := parseDate(g.Day)
		for _, hhmm := range []string{g.In, g.Out} {
			m, _ := hm(hhmm)
			t := time.Date(d.Year(), d.Month(), d.Day(), m/60, m%60, 0, 0, loc)
			if _, err := tx.Exec(ctx, `INSERT INTO adrient.hr_punches (org_id, employee_id, punched_at, day, source) VALUES ($1,$2::uuid,$3,$4::date,'regularised') ON CONFLICT DO NOTHING`,
				a.OrgID, g.EmployeeID, t.UTC(), g.Day); err != nil {
				return g, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return g, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, status, "regularisation", id, note)
	if e, err := r.empLite(ctx, r.pool, a.OrgID, g.EmployeeID); err == nil {
		r.tell(ctx, a.OrgID, e.UserID, a.UserID, "hr_regularisation_decision", "Attendance regularisation "+status, g.Day+" "+g.In+"-"+g.Out, "/hr/attendance")
	}
	return r.getReg(ctx, a, id)
}

// CancelRegularisation withdraws the caller's own pending request.
func (r *Repository) CancelRegularisation(ctx context.Context, a Actor, id string) error {
	g, err := r.getReg(ctx, a, id)
	if err != nil {
		return err
	}
	if !g.CanCancel {
		return forbidden("only your own pending request can be cancelled")
	}
	_, err = r.pool.Exec(ctx, `UPDATE adrient.hr_regularisations SET status='cancelled' WHERE id=$1::uuid AND status='pending'`, id)
	return err
}
