package hr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// LeaveType is a kind of leave with its accrual policy.
type LeaveType struct {
	ID              string   `json:"id"`
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Color           string   `json:"color"`
	Paid            bool     `json:"paid"`
	Tracked         bool     `json:"tracked"`
	AnnualQuota     float64  `json:"annual_quota"`
	Accrual         string   `json:"accrual"`
	CarryForward    bool     `json:"carry_forward"`
	CarryCap        float64  `json:"carry_cap"`
	MaxBalance      *float64 `json:"max_balance"`
	AllowHalfDay    bool     `json:"allow_half_day"`
	EmploymentTypes []string `json:"employment_types"`
	Active          bool     `json:"active"`
}

const typeCols = `id::text, code, name, color, paid, tracked, annual_quota::float8, accrual, carry_forward, carry_cap::float8, max_balance::float8, allow_half_day, employment_types, active`

func scanType(row pgx.Row) (LeaveType, error) {
	var t LeaveType
	var et []byte
	err := row.Scan(&t.ID, &t.Code, &t.Name, &t.Color, &t.Paid, &t.Tracked, &t.AnnualQuota, &t.Accrual, &t.CarryForward, &t.CarryCap, &t.MaxBalance, &t.AllowHalfDay, &et, &t.Active)
	_ = json.Unmarshal(et, &t.EmploymentTypes)
	if t.EmploymentTypes == nil {
		t.EmploymentTypes = []string{}
	}
	return t, err
}

// ListLeaveTypes lists leave types (any member).
func (r *Repository) ListLeaveTypes(ctx context.Context, orgID string, includeInactive bool) ([]LeaveType, error) {
	q := `SELECT ` + typeCols + ` FROM adrient.hr_leave_types WHERE org_id=$1`
	if !includeInactive {
		q += ` AND active`
	}
	rows, err := r.pool.Query(ctx, q+` ORDER BY created_at, name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeaveType{}
	for rows.Next() {
		t, err := scanType(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SaveLeaveType creates or updates a leave type (HR only).
func (r *Repository) SaveLeaveType(ctx context.Context, a Actor, id string, t LeaveType) (LeaveType, error) {
	if err := a.requireHR(); err != nil {
		return t, err
	}
	t.Code, t.Name = strings.ToUpper(strings.TrimSpace(t.Code)), strings.TrimSpace(t.Name)
	if t.Code == "" || t.Name == "" || len(t.Code) > 12 {
		return t, invalid("a code (up to 12 characters) and a name are required")
	}
	if t.Accrual != "monthly" && t.Accrual != "annual" && t.Accrual != "none" {
		return t, invalid("accrual must be monthly, annual or none")
	}
	if t.AnnualQuota < 0 || t.AnnualQuota > 366 || t.CarryCap < 0 {
		return t, invalid("quota and carry-forward cap must be sensible, non-negative numbers")
	}
	if t.MaxBalance != nil && *t.MaxBalance < 0 {
		return t, invalid("max balance cannot be negative")
	}
	if t.Color == "" {
		t.Color = "#126C5A"
	}
	for _, et := range t.EmploymentTypes {
		if !employmentTypes[et] {
			return t, invalid("unknown employment type %q", et)
		}
	}
	et, _ := json.Marshal(t.EmploymentTypes)
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_leave_types (org_id, code, name, color, paid, tracked, annual_quota, accrual, carry_forward, carry_cap, max_balance, allow_half_day, employment_types, active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING `+typeCols,
			a.OrgID, t.Code, t.Name, t.Color, t.Paid, t.Tracked, t.AnnualQuota, t.Accrual, t.CarryForward, t.CarryCap, t.MaxBalance, t.AllowHalfDay, et, true).
			Scan(&t.ID, &t.Code, &t.Name, &t.Color, &t.Paid, &t.Tracked, &t.AnnualQuota, &t.Accrual, &t.CarryForward, &t.CarryCap, &t.MaxBalance, &t.AllowHalfDay, new([]byte), &t.Active)
	} else {
		if !isUUID(id) {
			return t, ErrNotFound
		}
		var tag int64
		err = r.pool.QueryRow(ctx, `WITH u AS (UPDATE adrient.hr_leave_types SET code=$3, name=$4, color=$5, paid=$6, tracked=$7, annual_quota=$8, accrual=$9, carry_forward=$10,
			carry_cap=$11, max_balance=$12, allow_half_day=$13, employment_types=$14, active=$15 WHERE org_id=$1 AND id=$2::uuid RETURNING 1) SELECT count(*) FROM u`,
			a.OrgID, id, t.Code, t.Name, t.Color, t.Paid, t.Tracked, t.AnnualQuota, t.Accrual, t.CarryForward, t.CarryCap, t.MaxBalance, t.AllowHalfDay, et, t.Active).Scan(&tag)
		if err == nil && tag == 0 {
			return t, ErrNotFound
		}
		t.ID = id
	}
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return t, conflict("a leave type with code %s already exists", t.Code)
		}
		return t, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "save", "leave_type", t.ID, t.Code)
	return t, nil
}

// ---- holidays ----

// HolidayCalendar is a named set of holidays for a location (or the default).
type HolidayCalendar struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	LocationID string    `json:"location_id"`
	Holidays   []Holiday `json:"holidays"`
}

// Holiday is one day off.
type Holiday struct {
	ID       string `json:"id"`
	Day      string `json:"day"`
	Name     string `json:"name"`
	Optional bool   `json:"optional"`
}

// ListHolidayCalendars returns every calendar with its holidays (any member).
func (r *Repository) ListHolidayCalendars(ctx context.Context, orgID string) ([]HolidayCalendar, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, coalesce(location_id::text,'') FROM adrient.hr_holiday_calendars WHERE org_id=$1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	cals := []HolidayCalendar{}
	for rows.Next() {
		var c HolidayCalendar
		if err := rows.Scan(&c.ID, &c.Name, &c.LocationID); err != nil {
			rows.Close()
			return nil, err
		}
		c.Holidays = []Holiday{}
		cals = append(cals, c)
	}
	rows.Close()
	for i := range cals {
		hr, err := r.pool.Query(ctx, `SELECT id::text, to_char(day,'YYYY-MM-DD'), name, optional FROM adrient.hr_holidays WHERE calendar_id=$1::uuid ORDER BY day`, cals[i].ID)
		if err != nil {
			return nil, err
		}
		for hr.Next() {
			var h Holiday
			if err := hr.Scan(&h.ID, &h.Day, &h.Name, &h.Optional); err != nil {
				hr.Close()
				return nil, err
			}
			cals[i].Holidays = append(cals[i].Holidays, h)
		}
		hr.Close()
	}
	return cals, nil
}

// SaveHolidayCalendar creates or renames a calendar (HR only).
func (r *Repository) SaveHolidayCalendar(ctx context.Context, a Actor, id string, c HolidayCalendar) (HolidayCalendar, error) {
	if err := a.requireHR(); err != nil {
		return c, err
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return c, invalid("a name is required")
	}
	if c.LocationID != "" {
		var ok bool
		if !isUUID(c.LocationID) || r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.hr_locations WHERE id=$1::uuid AND org_id=$2)`, c.LocationID, a.OrgID).Scan(&ok) != nil || !ok {
			return c, invalid("unknown location")
		}
	}
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_holiday_calendars (org_id, name, location_id) VALUES ($1,$2,NULLIF($3,'')::uuid) RETURNING id::text`, a.OrgID, c.Name, c.LocationID).Scan(&c.ID)
	} else {
		_, err = r.pool.Exec(ctx, `UPDATE adrient.hr_holiday_calendars SET name=$3, location_id=NULLIF($4,'')::uuid WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id, c.Name, c.LocationID)
		c.ID = id
	}
	if err != nil && strings.Contains(err.Error(), "23505") {
		return c, conflict("a calendar named %q exists", c.Name)
	}
	return c, err
}

// DeleteHolidayCalendar removes a calendar and its holidays (HR only).
func (r *Repository) DeleteHolidayCalendar(ctx context.Context, a Actor, id string) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	if !isUUID(id) {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.hr_holiday_calendars WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id)
	return err
}

// AddHoliday adds or renames a holiday on a calendar (HR only).
func (r *Repository) AddHoliday(ctx context.Context, a Actor, calID string, h Holiday) (Holiday, error) {
	if err := a.requireHR(); err != nil {
		return h, err
	}
	var err error
	if h.Day, err = validDate(h.Day); err != nil {
		return h, err
	}
	h.Name = strings.TrimSpace(h.Name)
	if h.Name == "" {
		return h, invalid("a holiday name is required")
	}
	if !isUUID(calID) {
		return h, ErrNotFound
	}
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_holidays (org_id, calendar_id, day, name, optional)
		SELECT $1, c.id, $3::date, $4, $5 FROM adrient.hr_holiday_calendars c WHERE c.id=$2::uuid AND c.org_id=$1
		ON CONFLICT (calendar_id, day) DO UPDATE SET name=EXCLUDED.name, optional=EXCLUDED.optional RETURNING id::text`, a.OrgID, calID, h.Day, h.Name, h.Optional).Scan(&h.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, ErrNotFound
	}
	return h, err
}

// DeleteHoliday removes one holiday (HR only).
func (r *Repository) DeleteHoliday(ctx context.Context, a Actor, id string) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	if !isUUID(id) {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.hr_holidays WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id)
	return err
}

// holidaysFor returns the non-optional holiday dates (date -> name) that apply
// to a location: its own calendar if it has one, else the default calendars.
func (r *Repository) holidaysFor(ctx context.Context, d db, orgID, locationID string, from, to time.Time) (map[string]string, error) {
	rows, err := d.Query(ctx, `SELECT to_char(h.day,'YYYY-MM-DD'), h.name FROM adrient.hr_holidays h JOIN adrient.hr_holiday_calendars c ON c.id=h.calendar_id
		WHERE h.org_id=$1 AND NOT h.optional AND h.day BETWEEN $2::date AND $3::date
		AND (c.location_id = NULLIF($4,'')::uuid OR (c.location_id IS NULL AND NOT EXISTS (
			SELECT 1 FROM adrient.hr_holiday_calendars x WHERE x.org_id=$1 AND x.location_id = NULLIF($4,'')::uuid)))`,
		orgID, fmtDate(from), fmtDate(to), locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var day, name string
		if err := rows.Scan(&day, &name); err != nil {
			return nil, err
		}
		m[day] = name
	}
	return m, rows.Err()
}

func holidaySet(m map[string]string) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

func weeklyOffSet(s Settings) map[int]bool {
	m := map[int]bool{}
	for _, d := range s.WeeklyOff {
		m[d] = true
	}
	return m
}

// ---- balances ----

// Balance is one leave type's position for the current leave year.
type Balance struct {
	TypeID   string  `json:"type_id"`
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	Color    string  `json:"color"`
	Paid     bool    `json:"paid"`
	Tracked  bool    `json:"tracked"`
	Credited float64 `json:"credited"`
	Used     float64 `json:"used"`
	Pending  float64 `json:"pending"`
	Balance  float64 `json:"balance"`
	Year     int     `json:"year"`
}

type empLite struct {
	ID, UserID, Name, JoinDate, ExitDate, EmpType, LocationID, ManagerID, Status string
}

func (r *Repository) empLite(ctx context.Context, d db, orgID, id string) (empLite, error) {
	var e empLite
	if !isUUID(id) {
		return e, ErrNotFound
	}
	err := d.QueryRow(ctx, `SELECT id::text, coalesce(user_id::text,''), trim(first_name||' '||last_name), to_char(join_date,'YYYY-MM-DD'), coalesce(to_char(exit_date,'YYYY-MM-DD'),''),
		employment_type, coalesce(location_id::text,''), coalesce(manager_id::text,''), status FROM adrient.hr_employees WHERE org_id=$1 AND id=$2::uuid`, orgID, id).
		Scan(&e.ID, &e.UserID, &e.Name, &e.JoinDate, &e.ExitDate, &e.EmpType, &e.LocationID, &e.ManagerID, &e.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

func (t LeaveType) appliesTo(empType string) bool {
	if len(t.EmploymentTypes) == 0 {
		return true
	}
	for _, x := range t.EmploymentTypes {
		if x == empType {
			return true
		}
	}
	return false
}

func (r *Repository) ledgerBalance(ctx context.Context, d db, empID, typeID string, year int) (float64, error) {
	var b float64
	err := d.QueryRow(ctx, `SELECT coalesce(sum(days),0)::float8 FROM adrient.hr_leave_ledger WHERE employee_id=$1::uuid AND type_id=$2::uuid AND leave_year=$3`, empID, typeID, year).Scan(&b)
	return b, err
}

// EnsureAccrual credits everything due up to asOf for one employee. It is
// idempotent: a (type, year, month) accrual is written once.
func (r *Repository) EnsureAccrual(ctx context.Context, orgID, empID string, asOf time.Time) error {
	e, err := r.empLite(ctx, r.pool, orgID, empID)
	if err != nil {
		return err
	}
	if e.Status == "exited" && e.ExitDate == "" {
		return nil
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return err
	}
	types, err := r.ListLeaveTypes(ctx, orgID, false)
	if err != nil {
		return err
	}
	join := parseDate(e.JoinDate)
	var exit *time.Time
	if e.ExitDate != "" {
		x := parseDate(e.ExitDate)
		exit = &x
	}
	cur := LeaveYearOf(asOf, st.LeaveYearStart)
	firstYear := LeaveYearOf(join, st.LeaveYearStart)
	for _, t := range types {
		if !t.Tracked || t.Accrual == "none" || !t.appliesTo(e.EmpType) {
			continue
		}
		for y := firstYear; y <= cur; y++ {
			if y > firstYear {
				// Close the previous year: carry forward what the policy allows.
				prev, err := r.ledgerBalance(ctx, r.pool, empID, t.ID, y-1)
				if err != nil {
					return err
				}
				carry, lapse := CarryForward(prev, t.CarryCap, t.CarryForward)
				if carry > 0 {
					if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.hr_leave_ledger (org_id, employee_id, type_id, leave_year, kind, period, days, note)
						VALUES ($1,$2::uuid,$3::uuid,$4,'carry_forward','cf',$5,$6) ON CONFLICT DO NOTHING`, orgID, empID, t.ID, y, carry, fmt.Sprintf("carried from %d", y-1)); err != nil {
						return err
					}
				}
				if lapse > 0 {
					if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.hr_leave_ledger (org_id, employee_id, type_id, leave_year, kind, period, days, note)
						VALUES ($1,$2::uuid,$3::uuid,$4,'lapse','lapse',$5,$6) ON CONFLICT DO NOTHING`, orgID, empID, t.ID, y-1, -lapse, "lapsed at year end"); err != nil {
						return err
					}
				}
			}
			switch t.Accrual {
			case "annual":
				period := fmt.Sprintf("%d-annual", y)
				g := AnnualGrant(t.AnnualQuota, y, st.LeaveYearStart, join)
				if exit != nil {
					continue
				}
				bal, _ := r.ledgerBalance(ctx, r.pool, empID, t.ID, y)
				g = CapAccrual(bal, g, t.MaxBalance)
				if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.hr_leave_ledger (org_id, employee_id, type_id, leave_year, kind, period, days, note)
					VALUES ($1,$2::uuid,$3::uuid,$4,'accrual',$5,$6,'annual grant') ON CONFLICT DO NOTHING`, orgID, empID, t.ID, y, period, g); err != nil {
					return err
				}
			case "monthly":
				periods := AccrualPeriods(y, st.LeaveYearStart, join, exit, asOf)
				bal, _ := r.ledgerBalance(ctx, r.pool, empID, t.ID, y)
				m := MonthlyAccrual(t.AnnualQuota)
				for _, p := range periods {
					var have bool
					if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.hr_leave_ledger WHERE employee_id=$1::uuid AND type_id=$2::uuid AND leave_year=$3 AND kind='accrual' AND period=$4)`,
						empID, t.ID, y, p).Scan(&have); err != nil {
						return err
					}
					if have {
						continue
					}
					add := CapAccrual(bal, m, t.MaxBalance)
					if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.hr_leave_ledger (org_id, employee_id, type_id, leave_year, kind, period, days, note)
						VALUES ($1,$2::uuid,$3::uuid,$4,'accrual',$5,$6,'monthly accrual') ON CONFLICT DO NOTHING`, orgID, empID, t.ID, y, p, add); err != nil {
						return err
					}
					bal += add
				}
			}
		}
	}
	return nil
}

// RunAccrual runs EnsureAccrual for every active employee (HR only). Returns the
// number of employees processed.
func (r *Repository) RunAccrual(ctx context.Context, a Actor) (int, error) {
	if err := a.requireHR(); err != nil {
		return 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM adrient.hr_employees WHERE org_id=$1 AND status<>'exited'`, a.OrgID)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	today := r.today(ctx, a.OrgID)
	for _, id := range ids {
		if err := r.EnsureAccrual(ctx, a.OrgID, id, today); err != nil {
			return 0, err
		}
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "run", "leave_accrual", "", fmt.Sprintf("%d employees", len(ids)))
	return len(ids), nil
}

// Balances returns the employee's balances for the current leave year.
func (r *Repository) Balances(ctx context.Context, a Actor, empID string) ([]Balance, error) {
	if !r.canSee(ctx, a, empID) {
		return nil, forbidden("not allowed")
	}
	e, err := r.empLite(ctx, r.pool, a.OrgID, empID)
	if err != nil {
		return nil, err
	}
	today := r.today(ctx, a.OrgID)
	if err := r.EnsureAccrual(ctx, a.OrgID, empID, today); err != nil {
		return nil, err
	}
	st, _ := r.GetSettings(ctx, a.OrgID)
	year := LeaveYearOf(today, st.LeaveYearStart)
	types, err := r.ListLeaveTypes(ctx, a.OrgID, false)
	if err != nil {
		return nil, err
	}
	out := []Balance{}
	for _, t := range types {
		if !t.appliesTo(e.EmpType) {
			continue
		}
		b := Balance{TypeID: t.ID, Code: t.Code, Name: t.Name, Color: t.Color, Paid: t.Paid, Tracked: t.Tracked, Year: year}
		if err := r.pool.QueryRow(ctx, `SELECT coalesce(sum(days) FILTER (WHERE days>0),0)::float8, coalesce(-sum(days) FILTER (WHERE kind IN ('used','reversal','lapse') ),0)::float8
			FROM adrient.hr_leave_ledger WHERE employee_id=$1::uuid AND type_id=$2::uuid AND leave_year=$3`, empID, t.ID, year).Scan(&b.Credited, &b.Used); err != nil {
			return nil, err
		}
		// Credited excludes reversals (they restore used days) so used stays net.
		var rev float64
		_ = r.pool.QueryRow(ctx, `SELECT coalesce(sum(days),0)::float8 FROM adrient.hr_leave_ledger WHERE employee_id=$1::uuid AND type_id=$2::uuid AND leave_year=$3 AND kind='reversal'`, empID, t.ID, year).Scan(&rev)
		b.Credited -= rev
		b.Used = round2(b.Used)
		bal, _ := r.ledgerBalance(ctx, r.pool, empID, t.ID, year)
		b.Balance = round2(bal)
		_ = r.pool.QueryRow(ctx, `SELECT coalesce(sum(days),0)::float8 FROM adrient.hr_leave_requests WHERE employee_id=$1::uuid AND type_id=$2::uuid AND status='pending'
			AND from_date >= $3::date AND from_date <= $4::date`, empID, t.ID, fmtDate(time.Date(year, time.Month(st.LeaveYearStart), 1, 0, 0, 0, 0, time.UTC)),
			fmtDate(time.Date(year+1, time.Month(st.LeaveYearStart), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1))).Scan(&b.Pending)
		b.Credited = round2(b.Credited)
		out = append(out, b)
	}
	return out, nil
}

// AdjustBalance adds or removes days with a note (HR only).
func (r *Repository) AdjustBalance(ctx context.Context, a Actor, empID, typeID string, days float64, note string) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	if days == 0 || days > 366 || days < -366 {
		return invalid("days must be a non-zero number between -366 and 366")
	}
	if strings.TrimSpace(note) == "" {
		return invalid("a note explaining the adjustment is required")
	}
	if _, err := r.empLite(ctx, r.pool, a.OrgID, empID); err != nil {
		return err
	}
	var ok bool
	if !isUUID(typeID) || r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.hr_leave_types WHERE id=$1::uuid AND org_id=$2 AND tracked)`, typeID, a.OrgID).Scan(&ok) != nil || !ok {
		return invalid("unknown or untracked leave type")
	}
	st, _ := r.GetSettings(ctx, a.OrgID)
	if err := r.EnsureAccrual(ctx, a.OrgID, empID, r.today(ctx, a.OrgID)); err != nil {
		return err
	}
	year := LeaveYearOf(r.today(ctx, a.OrgID), st.LeaveYearStart)
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.hr_leave_ledger (org_id, employee_id, type_id, leave_year, kind, days, note, created_by) VALUES ($1,$2::uuid,$3::uuid,$4,'adjustment',$5,$6,$7::uuid)`,
		a.OrgID, empID, typeID, year, days, strings.TrimSpace(note), a.UserID)
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "adjust", "leave_balance", empID, fmt.Sprintf("%+.2f %s", days, note))
	return err
}

// ---- requests ----

// LeaveRequest is one leave application.
type LeaveRequest struct {
	ID           string  `json:"id"`
	EmployeeID   string  `json:"employee_id"`
	EmployeeName string  `json:"employee_name"`
	EmpCode      string  `json:"emp_code"`
	TypeID       string  `json:"type_id"`
	TypeName     string  `json:"type_name"`
	TypeColor    string  `json:"type_color"`
	From         string  `json:"from_date"`
	To           string  `json:"to_date"`
	HalfDay      string  `json:"half_day"`
	Days         float64 `json:"days"`
	Reason       string  `json:"reason"`
	Status       string  `json:"status"`
	DecidedBy    string  `json:"decided_by"`
	DecisionNote string  `json:"decision_note"`
	CreatedAt    string  `json:"created_at"`
	CanDecide    bool    `json:"can_decide"`
	CanCancel    bool    `json:"can_cancel"`
}

const reqCols = `q.id::text, q.employee_id::text, trim(e.first_name||' '||e.last_name), e.emp_code, q.type_id::text, t.name, t.color,
	to_char(q.from_date,'YYYY-MM-DD'), to_char(q.to_date,'YYYY-MM-DD'), q.half_day, q.days::float8, q.reason, q.status,
	coalesce(du.display_name,''), q.decision_note, to_char(q.created_at,'YYYY-MM-DD')`
const reqFrom = ` FROM adrient.hr_leave_requests q JOIN adrient.hr_employees e ON e.id=q.employee_id JOIN adrient.hr_leave_types t ON t.id=q.type_id
	LEFT JOIN adrient.users du ON du.id=q.decided_by`

func scanReq(row pgx.Row) (LeaveRequest, error) {
	var q LeaveRequest
	err := row.Scan(&q.ID, &q.EmployeeID, &q.EmployeeName, &q.EmpCode, &q.TypeID, &q.TypeName, &q.TypeColor, &q.From, &q.To, &q.HalfDay, &q.Days, &q.Reason,
		&q.Status, &q.DecidedBy, &q.DecisionNote, &q.CreatedAt)
	return q, err
}

// LeaveFilter narrows ListLeaveRequests.
type LeaveFilter struct {
	Scope      string // mine | team | all
	Status     string
	EmployeeID string
	From, To   string
	Limit      int // 0 = 500 (the old cap); HTTP paging is in page.go
	Offset     int
}

// ListLeaveRequests lists requests the caller may see. scope=mine is the
// caller's own, team the direct reports (HR: everyone), all requires HR.
func (r *Repository) ListLeaveRequests(ctx context.Context, a Actor, f LeaveFilter) ([]LeaveRequest, error) {
	q := `SELECT ` + reqCols + reqFrom + ` WHERE q.org_id=$1`
	args := []any{a.OrgID}
	switch f.Scope {
	case "", "mine":
		if a.EmployeeID == "" {
			return []LeaveRequest{}, nil
		}
		args = append(args, a.EmployeeID)
		q += fmt.Sprintf(" AND q.employee_id=$%d::uuid", len(args))
	case "team":
		if a.IsHR {
			break
		}
		if a.EmployeeID == "" || !a.IsManager {
			return []LeaveRequest{}, nil
		}
		args = append(args, a.EmployeeID)
		q += fmt.Sprintf(" AND e.manager_id=$%d::uuid", len(args))
	case "all":
		if !a.IsHR {
			return nil, forbidden("HR admin only")
		}
	default:
		return nil, invalid("unknown scope")
	}
	if f.Status != "" {
		args = append(args, f.Status)
		q += fmt.Sprintf(" AND q.status=$%d", len(args))
	}
	if isUUID(f.EmployeeID) {
		if !r.canSee(ctx, a, f.EmployeeID) {
			return nil, forbidden("not allowed")
		}
		args = append(args, f.EmployeeID)
		q += fmt.Sprintf(" AND q.employee_id=$%d::uuid", len(args))
	}
	if f.From != "" {
		args = append(args, f.From)
		q += fmt.Sprintf(" AND q.to_date >= $%d::date", len(args))
	}
	if f.To != "" {
		args = append(args, f.To)
		q += fmt.Sprintf(" AND q.from_date <= $%d::date", len(args))
	}
	lim := f.Limit
	if lim <= 0 || lim > 501 {
		lim = 500
	}
	rows, err := r.pool.Query(ctx, q+fmt.Sprintf(" ORDER BY q.from_date DESC, q.created_at DESC, q.id DESC LIMIT %d OFFSET %d", lim, max(f.Offset, 0)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeaveRequest{}
	for rows.Next() {
		x, err := scanReq(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].CanDecide, out[i].CanCancel = r.leaveActions(ctx, a, out[i])
	}
	return out, nil
}

// leaveActions reports what the caller may do with a request.
func (r *Repository) leaveActions(ctx context.Context, a Actor, q LeaveRequest) (decide, cancel bool) {
	if q.Status == "pending" {
		decide = r.mayDecide(ctx, a, q.EmployeeID)
	}
	if q.Status == "pending" || (q.Status == "approved" && q.To >= fmtDate(r.today(ctx, a.OrgID))) {
		cancel = a.IsHR || (a.EmployeeID != "" && a.EmployeeID == q.EmployeeID)
	}
	if q.Status == "approved" && a.IsHR {
		cancel = true
	}
	return
}

// mayDecide: the direct manager, or HR (override). Nobody decides their own
// request, except HR for an employee who has no manager at all.
func (r *Repository) mayDecide(ctx context.Context, a Actor, empID string) bool {
	if a.EmployeeID != "" && a.EmployeeID == empID {
		if !a.IsHR {
			return false
		}
		var hasMgr bool
		_ = r.pool.QueryRow(ctx, `SELECT manager_id IS NOT NULL FROM adrient.hr_employees WHERE id=$1::uuid`, empID).Scan(&hasMgr)
		return !hasMgr
	}
	return a.IsHR || r.isManagerOf(ctx, a, empID)
}

// LeaveInput is an application for leave.
type LeaveInput struct {
	EmployeeID string `json:"employee_id"` // empty = the caller; HR may apply for anyone
	TypeID     string `json:"type_id"`
	From       string `json:"from_date"`
	To         string `json:"to_date"`
	HalfDay    string `json:"half_day"`
	Reason     string `json:"reason"`
}

// ApplyLeave validates and records a pending request and notifies the approver.
func (r *Repository) ApplyLeave(ctx context.Context, a Actor, in LeaveInput) (LeaveRequest, error) {
	var out LeaveRequest
	empID := in.EmployeeID
	if empID == "" {
		empID = a.EmployeeID
	}
	if empID == "" {
		return out, forbidden("no employee record is linked to your account")
	}
	if empID != a.EmployeeID && !a.IsHR {
		return out, forbidden("you can only apply for your own leave")
	}
	e, err := r.empLite(ctx, r.pool, a.OrgID, empID)
	if err != nil {
		return out, err
	}
	if e.Status == "exited" {
		return out, invalid("this employee has left the organisation")
	}
	from, err := validDate(in.From)
	if err != nil {
		return out, err
	}
	if in.To == "" {
		in.To = from
	}
	to, err := validDate(in.To)
	if err != nil {
		return out, err
	}
	if in.HalfDay != "" && in.HalfDay != "first" && in.HalfDay != "second" {
		return out, invalid("half_day must be first or second")
	}
	var t LeaveType
	if !isUUID(in.TypeID) {
		return out, invalid("choose a leave type")
	}
	t, err = scanType(r.pool.QueryRow(ctx, `SELECT `+typeCols+` FROM adrient.hr_leave_types WHERE org_id=$1 AND id=$2::uuid AND active`, a.OrgID, in.TypeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return out, invalid("unknown leave type")
	}
	if err != nil {
		return out, err
	}
	if !t.appliesTo(e.EmpType) {
		return out, invalid("%s leave does not apply to this employment type", t.Name)
	}
	if in.HalfDay != "" && !t.AllowHalfDay {
		return out, invalid("%s leave cannot be taken as a half day", t.Name)
	}
	st, _ := r.GetSettings(ctx, a.OrgID)
	fd, td := parseDate(from), parseDate(to)
	hol, err := r.holidaysFor(ctx, r.pool, a.OrgID, e.LocationID, fd, td)
	if err != nil {
		return out, err
	}
	days, err := CountLeaveDays(fd, td, in.HalfDay, weeklyOffSet(st), holidaySet(hol))
	if err != nil {
		return out, err
	}
	if from < fmtDate(parseDate(e.JoinDate)) {
		return out, invalid("leave cannot start before the join date")
	}
	// Overlap with an open or approved request.
	rows, err := r.pool.Query(ctx, `SELECT to_char(from_date,'YYYY-MM-DD'), to_char(to_date,'YYYY-MM-DD'), half_day FROM adrient.hr_leave_requests
		WHERE employee_id=$1::uuid AND status IN ('pending','approved') AND from_date<=$3::date AND to_date>=$2::date`, empID, from, to)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var f2, t2, h2 string
		if err := rows.Scan(&f2, &t2, &h2); err != nil {
			rows.Close()
			return out, err
		}
		if leavesOverlap(fd, td, in.HalfDay, parseDate(f2), parseDate(t2), h2) {
			rows.Close()
			return out, conflict("you already have leave from %s to %s that overlaps these dates", f2, t2)
		}
	}
	rows.Close()
	if t.Tracked {
		if err := r.EnsureAccrual(ctx, a.OrgID, empID, r.today(ctx, a.OrgID)); err != nil {
			return out, err
		}
		year := LeaveYearOf(fd, st.LeaveYearStart)
		bal, _ := r.ledgerBalance(ctx, r.pool, empID, t.ID, year)
		var pending float64
		_ = r.pool.QueryRow(ctx, `SELECT coalesce(sum(days),0)::float8 FROM adrient.hr_leave_requests WHERE employee_id=$1::uuid AND type_id=$2::uuid AND status='pending'`, empID, t.ID).Scan(&pending)
		if bal-pending < days {
			return out, conflict("insufficient %s balance: %.2f available, %.2f requested", t.Name, bal-pending, days)
		}
	}
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_leave_requests (org_id, employee_id, type_id, from_date, to_date, half_day, days, reason)
		VALUES ($1,$2::uuid,$3::uuid,$4::date,$5::date,$6,$7,$8) RETURNING id::text`, a.OrgID, empID, t.ID, from, to, in.HalfDay, days, strings.TrimSpace(in.Reason)).Scan(&id); err != nil {
		return out, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "apply", "leave", id, fmt.Sprintf("%s %s..%s %.1f", t.Code, from, to, days))
	// Tell the approver.
	title := fmt.Sprintf("Leave request: %s", e.Name)
	body := fmt.Sprintf("%s, %s to %s (%.1f day(s))", t.Name, from, to, days)
	for _, u := range r.approverUsers(ctx, a.OrgID, e) {
		r.tell(ctx, a.OrgID, u, a.UserID, "hr_leave_request", title, body, "/hr/approvals")
	}
	return r.getRequest(ctx, a, id)
}

// approverUsers are the user ids to notify: the manager's account, or every HR
// admin when the employee has no manager.
func (r *Repository) approverUsers(ctx context.Context, orgID string, e empLite) []string {
	var out []string
	if e.ManagerID != "" {
		var u string
		if r.pool.QueryRow(ctx, `SELECT coalesce(user_id::text,'') FROM adrient.hr_employees WHERE id=$1::uuid`, e.ManagerID).Scan(&u) == nil && u != "" {
			return []string{u}
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT user_id::text FROM adrient.hr_admins WHERE org_id=$1 UNION SELECT user_id::text FROM adrient.org_admins WHERE org_id=$1`, orgID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			out = append(out, u)
		}
	}
	return out
}

func (r *Repository) getRequest(ctx context.Context, a Actor, id string) (LeaveRequest, error) {
	if !isUUID(id) {
		return LeaveRequest{}, ErrNotFound
	}
	q, err := scanReq(r.pool.QueryRow(ctx, `SELECT `+reqCols+reqFrom+` WHERE q.org_id=$1 AND q.id=$2::uuid`, a.OrgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return q, ErrNotFound
	}
	if err != nil {
		return q, err
	}
	if !r.canSee(ctx, a, q.EmployeeID) {
		return LeaveRequest{}, ErrNotFound
	}
	q.CanDecide, q.CanCancel = r.leaveActions(ctx, a, q)
	return q, nil
}

// DecideLeave approves or rejects a pending request.
func (r *Repository) DecideLeave(ctx context.Context, a Actor, id string, approve bool, note string) (LeaveRequest, error) {
	q, err := r.getRequest(ctx, a, id)
	if err != nil {
		return q, err
	}
	if q.Status != "pending" {
		return q, conflict("this request is already %s", q.Status)
	}
	if !r.mayDecide(ctx, a, q.EmployeeID) {
		return q, forbidden("only the employee's manager or HR can decide this request")
	}
	e, err := r.empLite(ctx, r.pool, a.OrgID, q.EmployeeID)
	if err != nil {
		return q, err
	}
	note = strings.TrimSpace(note)
	if !approve {
		tag, err := r.pool.Exec(ctx, `UPDATE adrient.hr_leave_requests SET status='rejected', decided_by=$3::uuid, decided_at=now(), decision_note=$4 WHERE org_id=$1 AND id=$2::uuid AND status='pending'`,
			a.OrgID, id, a.UserID, note)
		if err != nil {
			return q, err
		}
		if tag.RowsAffected() == 0 {
			return q, conflict("this request was already decided")
		}
		r.audit(ctx, r.pool, a.OrgID, a.UserID, "reject", "leave", id, note)
		r.tell(ctx, a.OrgID, e.UserID, a.UserID, "hr_leave_decision", "Leave rejected", fmt.Sprintf("%s %s to %s was rejected. %s", q.TypeName, q.From, q.To, note), "/hr")
		return r.getRequest(ctx, a, id)
	}
	st, _ := r.GetSettings(ctx, a.OrgID)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return q, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE adrient.hr_leave_requests SET status='approved', decided_by=$3::uuid, decided_at=now(), decision_note=$4 WHERE org_id=$1 AND id=$2::uuid AND status='pending'`,
		a.OrgID, id, a.UserID, note)
	if err != nil {
		return q, err
	}
	if tag.RowsAffected() == 0 {
		return q, conflict("this request was already decided")
	}
	var tracked bool
	if err := tx.QueryRow(ctx, `SELECT tracked FROM adrient.hr_leave_types WHERE id=$1::uuid`, q.TypeID).Scan(&tracked); err != nil {
		return q, err
	}
	if tracked {
		year := LeaveYearOf(parseDate(q.From), st.LeaveYearStart)
		bal, err := r.ledgerBalance(ctx, tx, q.EmployeeID, q.TypeID, year)
		if err != nil {
			return q, err
		}
		if bal < q.Days {
			return q, conflict("insufficient balance now (%.2f left); adjust the balance or reject", bal)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.hr_leave_ledger (org_id, employee_id, type_id, leave_year, kind, days, request_id, note, created_by)
			VALUES ($1,$2::uuid,$3::uuid,$4,'used',$5,$6::uuid,$7,$8::uuid)`, a.OrgID, q.EmployeeID, q.TypeID, year, -q.Days, id, q.From+".."+q.To, a.UserID); err != nil {
			return q, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return q, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "approve", "leave", id, note)
	// Calendar out-of-office. A Calendar failure never undoes the approval.
	if r.cal != nil && e.UserID != "" {
		evID, err := r.createOOO(ctx, a.OrgID, e, q, st)
		if err != nil {
			slog.Warn("hr: calendar out-of-office failed", "request", id, "err", err)
		} else if evID != "" {
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.hr_leave_requests SET calendar_event_id=$2 WHERE id=$1::uuid`, id, evID)
		}
	}
	r.tell(ctx, a.OrgID, e.UserID, a.UserID, "hr_leave_decision", "Leave approved", fmt.Sprintf("%s %s to %s was approved.", q.TypeName, q.From, q.To), "/calendar")
	return r.getRequest(ctx, a, id)
}

func (r *Repository) createOOO(ctx context.Context, orgID string, e empLite, q LeaveRequest, st Settings) (string, error) {
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		loc = time.FixedZone("IST", 5*3600+1800)
	}
	f, t := parseDate(q.From), parseDate(q.To)
	title := "Out of office: " + q.TypeName
	if q.HalfDay == "first" {
		s := time.Date(f.Year(), f.Month(), f.Day(), 9, 0, 0, 0, loc)
		return r.cal.CreateOutOfOffice(ctx, orgID, e.UserID, title+" (first half)", s, s.Add(4*time.Hour), false)
	}
	if q.HalfDay == "second" {
		s := time.Date(f.Year(), f.Month(), f.Day(), 14, 0, 0, 0, loc)
		return r.cal.CreateOutOfOffice(ctx, orgID, e.UserID, title+" (second half)", s, s.Add(4*time.Hour), false)
	}
	s := time.Date(f.Year(), f.Month(), f.Day(), 0, 0, 0, 0, loc)
	end := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
	return r.cal.CreateOutOfOffice(ctx, orgID, e.UserID, title, s, end, true)
}

// CancelLeave cancels a pending or approved request, restoring balance and
// removing the Calendar entry.
func (r *Repository) CancelLeave(ctx context.Context, a Actor, id string) (LeaveRequest, error) {
	q, err := r.getRequest(ctx, a, id)
	if err != nil {
		return q, err
	}
	if !q.CanCancel {
		if q.Status == "cancelled" || q.Status == "rejected" {
			return q, conflict("this request is already %s", q.Status)
		}
		return q, forbidden("you cannot cancel this request")
	}
	st, _ := r.GetSettings(ctx, a.OrgID)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return q, err
	}
	defer tx.Rollback(ctx)
	var prev, evID string
	if err := tx.QueryRow(ctx, `SELECT status, calendar_event_id FROM adrient.hr_leave_requests WHERE id=$1::uuid FOR UPDATE`, id).Scan(&prev, &evID); err != nil {
		return q, err
	}
	if prev != "pending" && prev != "approved" {
		return q, conflict("this request is already %s", prev)
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.hr_leave_requests SET status='cancelled', calendar_event_id='' WHERE id=$1::uuid`, id); err != nil {
		return q, err
	}
	if prev == "approved" {
		var tracked bool
		_ = tx.QueryRow(ctx, `SELECT tracked FROM adrient.hr_leave_types WHERE id=$1::uuid`, q.TypeID).Scan(&tracked)
		if tracked {
			year := LeaveYearOf(parseDate(q.From), st.LeaveYearStart)
			if _, err := tx.Exec(ctx, `INSERT INTO adrient.hr_leave_ledger (org_id, employee_id, type_id, leave_year, kind, days, request_id, note, created_by)
				VALUES ($1,$2::uuid,$3::uuid,$4,'reversal',$5,$6::uuid,'leave cancelled',$7::uuid)`, a.OrgID, q.EmployeeID, q.TypeID, year, q.Days, id, a.UserID); err != nil {
				return q, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return q, err
	}
	if evID != "" && r.cal != nil {
		if err := r.cal.DeleteEvent(ctx, a.OrgID, evID); err != nil {
			slog.Warn("hr: calendar delete failed", "event", evID, "err", err)
		}
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "cancel", "leave", id, "")
	if e, err := r.empLite(ctx, r.pool, a.OrgID, q.EmployeeID); err == nil {
		title, body := "Leave cancelled: "+e.Name, fmt.Sprintf("%s %s to %s", q.TypeName, q.From, q.To)
		if a.EmployeeID == q.EmployeeID {
			for _, u := range r.approverUsers(ctx, a.OrgID, e) {
				r.tell(ctx, a.OrgID, u, a.UserID, "hr_leave_cancel", title, body, "/hr/approvals")
			}
		} else {
			r.tell(ctx, a.OrgID, e.UserID, a.UserID, "hr_leave_cancel", "Your leave was cancelled by HR", body, "/hr")
		}
	}
	return r.getRequest(ctx, a, id)
}

// ---- team calendar ----

// TeamLeave is one leave bar on the team calendar.
type TeamLeave struct {
	EmployeeID string `json:"employee_id"`
	Name       string `json:"name"`
	TypeName   string `json:"type_name"`
	Color      string `json:"color"`
	From       string `json:"from_date"`
	To         string `json:"to_date"`
	HalfDay    string `json:"half_day"`
	Status     string `json:"status"`
}

// TeamCalendar returns approved (and, for approvers, pending) leave in a window
// plus holidays. HR sees everyone, a manager their reports and themselves, an
// employee their own team (same manager).
func (r *Repository) TeamCalendar(ctx context.Context, a Actor, from, to string) (map[string]any, error) {
	var err error
	if from, err = validDate(from); err != nil {
		return nil, err
	}
	if to, err = validDate(to); err != nil {
		return nil, err
	}
	if parseDate(to).Sub(parseDate(from)) > 400*24*time.Hour {
		return nil, invalid("window too large")
	}
	q := `SELECT q.employee_id::text, trim(e.first_name||' '||e.last_name), t.name, t.color, to_char(q.from_date,'YYYY-MM-DD'), to_char(q.to_date,'YYYY-MM-DD'), q.half_day, q.status
		FROM adrient.hr_leave_requests q JOIN adrient.hr_employees e ON e.id=q.employee_id JOIN adrient.hr_leave_types t ON t.id=q.type_id
		WHERE q.org_id=$1 AND q.from_date<=$3::date AND q.to_date>=$2::date AND q.status IN ('pending','approved')`
	args := []any{a.OrgID, from, to}
	if !a.IsHR {
		if a.EmployeeID == "" {
			return map[string]any{"leaves": []TeamLeave{}, "holidays": []Holiday{}}, nil
		}
		args = append(args, a.EmployeeID)
		q += ` AND (q.employee_id=$4::uuid OR e.manager_id=$4::uuid OR e.manager_id=(SELECT manager_id FROM adrient.hr_employees WHERE id=$4::uuid) OR e.id=(SELECT manager_id FROM adrient.hr_employees WHERE id=$4::uuid))`
	}
	rows, err := r.pool.Query(ctx, q+` ORDER BY q.from_date`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	leaves := []TeamLeave{}
	for rows.Next() {
		var l TeamLeave
		if err := rows.Scan(&l.EmployeeID, &l.Name, &l.TypeName, &l.Color, &l.From, &l.To, &l.HalfDay, &l.Status); err != nil {
			return nil, err
		}
		// Pending requests are only shown to the employee, their manager and HR.
		if l.Status == "pending" && !(a.IsHR || l.EmployeeID == a.EmployeeID || r.isManagerOf(ctx, a, l.EmployeeID)) {
			continue
		}
		leaves = append(leaves, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var loc string
	if a.EmployeeID != "" {
		_ = r.pool.QueryRow(ctx, `SELECT coalesce(location_id::text,'') FROM adrient.hr_employees WHERE id=$1::uuid`, a.EmployeeID).Scan(&loc)
	}
	hol, err := r.holidaysFor(ctx, r.pool, a.OrgID, loc, parseDate(from), parseDate(to))
	if err != nil {
		return nil, err
	}
	hs := []Holiday{}
	for d, n := range hol {
		hs = append(hs, Holiday{Day: d, Name: n})
	}
	return map[string]any{"leaves": leaves, "holidays": hs}, nil
}
