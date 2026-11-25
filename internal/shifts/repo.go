package shifts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
	"github.com/Adrient-tech/Adrient-Workspace/internal/payrollhours"
)

// Caller is the signed-in person with the roles Shifts derives from HR.
type Caller struct {
	OrgID      string
	UserID     string
	Actor      hr.Actor
	Scheduler  bool // org admin, HR admin or a manager with direct reports
	EmployeeID string
}

// Settings are the organisation-wide rota settings.
type Settings struct {
	Rules        Rules    `json:"rules"`
	Overtime     float64  `json:"overtime_threshold_hours"`
	OvertimeMult float64  `json:"overtime_multiplier"`
	WeeklyBudget float64  `json:"weekly_budget"`
	Currency     string   `json:"currency"`
	GraceMinutes int      `json:"grace_minutes"`
	DefaultRoles []string `json:"roles"`
}

// Swap is a request to hand one shift to a colleague.
type Swap struct {
	ID           string         `json:"id"`
	ShiftID      string         `json:"shift_id"`
	FromEmployee string         `json:"from_employee_id"`
	FromName     string         `json:"from_name"`
	ToEmployee   string         `json:"to_employee_id"`
	ToName       string         `json:"to_name"`
	Reason       string         `json:"reason"`
	Status       string         `json:"status"`
	Snapshot     map[string]any `json:"snapshot"`
	CreatedAt    string         `json:"created_at"`
	Warnings     []string       `json:"warnings,omitempty"`
	CanResolve   bool           `json:"can_resolve"`
	CanCancel    bool           `json:"can_cancel"`
}

// Repository is the Postgres store for Shifts.
type Repository struct {
	pool *pgxpool.Pool
	hr   *hr.Repository
	now  func() time.Time
}

// NewRepository builds the store; hrRepo supplies employees, roles and leave.
func NewRepository(pool *pgxpool.Pool, hrRepo *hr.Repository) *Repository {
	return &Repository{pool: pool, hr: hrRepo, now: time.Now}
}

// CallerFor resolves the roles of a user. orgAdmin comes from authz.
func (r *Repository) CallerFor(ctx context.Context, orgID, userID string, orgAdmin bool) (Caller, error) {
	a, err := r.hr.ActorFor(ctx, orgID, userID, orgAdmin)
	if err != nil {
		return Caller{}, err
	}
	return Caller{OrgID: orgID, UserID: userID, Actor: a, EmployeeID: a.EmployeeID, Scheduler: orgAdmin || a.IsHR || a.IsManager}, nil
}

func (c Caller) needScheduler() error {
	if !c.Scheduler {
		return opshttp.Forbidden("only managers and HR admins can change the rota")
	}
	return nil
}

// ---- settings ----

func defaultSettings() Settings {
	return Settings{Currency: "INR", GraceMinutes: 5, DefaultRoles: []string{}}
}

// GetSettings returns the rota settings.
func (r *Repository) GetSettings(ctx context.Context, orgID string) (Settings, error) {
	s := defaultSettings()
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT data FROM adrient.shf_settings WHERE org_id=$1`, orgID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	_ = json.Unmarshal(raw, &s)
	return s, nil
}

// SaveSettings validates and stores the settings (schedulers only).
func (r *Repository) SaveSettings(ctx context.Context, c Caller, s Settings) (Settings, error) {
	if err := c.needScheduler(); err != nil {
		return s, err
	}
	if s.Rules.MinRestHours < 0 || s.Rules.MinRestHours > 24 || s.Rules.MaxDailyHours < 0 || s.Rules.MaxDailyHours > 24 || s.Rules.MaxConsecutive < 0 || s.Rules.MaxConsecutive > 60 {
		return s, opshttp.Invalid("working-time limits are out of range")
	}
	if s.Overtime < 0 || s.Overtime > 168 || (s.OvertimeMult != 0 && (s.OvertimeMult < 1 || s.OvertimeMult > 5)) || s.WeeklyBudget < 0 {
		return s, opshttp.Invalid("overtime or budget values are out of range")
	}
	if s.GraceMinutes < 0 || s.GraceMinutes > 120 {
		return s, opshttp.Invalid("grace minutes must be 0-120")
	}
	if s.Currency == "" {
		s.Currency = "INR"
	}
	raw, _ := json.Marshal(s)
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.shf_settings (org_id, data) VALUES ($1,$2::jsonb)
		ON CONFLICT (org_id) DO UPDATE SET data=EXCLUDED.data, updated_at=now()`, c.OrgID, raw)
	return s, err
}

// ---- staff ----

type profile struct {
	role   string
	color  string
	rate   *float64
	target *float64
	max    *float64
	avail  Availability
}

var palette = []string{"#126C5A", "#3B5BDB", "#C2410C", "#7C3AED", "#0E7490", "#B45309", "#BE185D", "#4D7C0F"}

func (r *Repository) profiles(ctx context.Context, orgID string) (map[string]profile, error) {
	rows, err := r.pool.Query(ctx, `SELECT employee_id::text, role, color, hourly_rate::float8, target_hours::float8, max_hours::float8, availability FROM adrient.shf_staff WHERE org_id=$1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]profile{}
	for rows.Next() {
		var id string
		var p profile
		var raw []byte
		if err := rows.Scan(&id, &p.role, &p.color, &p.rate, &p.target, &p.max, &raw); err != nil {
			return nil, err
		}
		p.avail = DefaultAvailability()
		var a Availability
		if json.Unmarshal(raw, &a) == nil && len(a.Days) == 7 {
			p.avail.Days = a.Days
		}
		if a.Exceptions != nil {
			p.avail.Exceptions = a.Exceptions
		}
		out[id] = p
	}
	return out, rows.Err()
}

// Staff lists active employees with their scheduling profile. Approved HR
// leave in from..to is added to each person's time off (Source "leave").
// Pay rates are only filled in for schedulers.
func (r *Repository) Staff(ctx context.Context, c Caller, from, to string) ([]Staff, error) {
	emps, err := r.hr.ListEmployees(ctx, c.Actor, hr.EmployeeFilter{})
	if err != nil {
		return nil, err
	}
	prof, err := r.profiles(ctx, c.OrgID)
	if err != nil {
		return nil, err
	}
	var leave []hr.LeaveSpan
	if opshttp.ValidDate(from) && opshttp.ValidDate(to) {
		if leave, err = r.hr.ApprovedLeave(ctx, c.OrgID, from, to); err != nil {
			return nil, err
		}
	}
	out := make([]Staff, 0, len(emps))
	for i, e := range emps {
		p, ok := prof[e.ID]
		if !ok {
			p = profile{avail: DefaultAvailability()}
		}
		st := Staff{EmployeeID: e.ID, Name: e.Name, EmpCode: e.EmpCode, Department: e.DepartmentName, Role: p.role, Color: p.color,
			TargetHours: p.target, MaxHours: p.max, Availability: p.avail, Active: e.Status != "exited"}
		if st.Role == "" {
			st.Role = e.DesignationName
		}
		if st.Color == "" {
			st.Color = palette[i%len(palette)]
		}
		if c.Scheduler {
			st.HourlyRate = p.rate
		}
		ex := append([]TimeOff{}, st.Availability.Exceptions...)
		for _, l := range leave {
			if l.EmployeeID == e.ID {
				ex = append(ex, TimeOff{From: l.From, To: l.To, Note: l.TypeName, Source: "leave"})
			}
		}
		st.Availability.Exceptions = ex
		out = append(out, st)
	}
	return out, nil
}

// StaffInput is the editable part of a staff profile.
type StaffInput struct {
	Role         string       `json:"role"`
	Color        string       `json:"color"`
	HourlyRate   *float64     `json:"hourly_rate"`
	TargetHours  *float64     `json:"target_hours"`
	MaxHours     *float64     `json:"max_hours"`
	Availability Availability `json:"availability"`
}

// SaveStaff stores one employee's scheduling profile (schedulers only).
func (r *Repository) SaveStaff(ctx context.Context, c Caller, employeeID string, in StaffInput) error {
	if err := c.needScheduler(); err != nil {
		return err
	}
	if !opshttp.IsUUID(employeeID) {
		return opshttp.ErrNotFound
	}
	if in.HourlyRate != nil && (*in.HourlyRate < 0 || *in.HourlyRate > 1e6) {
		return opshttp.Invalid("hourly rate is out of range")
	}
	if (in.TargetHours != nil && (*in.TargetHours < 0 || *in.TargetHours > 168)) || (in.MaxHours != nil && (*in.MaxHours <= 0 || *in.MaxHours > 168)) {
		return opshttp.Invalid("weekly hours must be between 0 and 168")
	}
	if len(in.Availability.Days) != 7 {
		in.Availability = DefaultAvailability()
	}
	for _, d := range in.Availability.Days {
		if !d.Off && (!opshttp.ValidTime(d.Start) || !opshttp.ValidTime(d.End)) {
			return opshttp.Invalid("availability times must be HH:MM")
		}
	}
	manual := []TimeOff{}
	for _, t := range in.Availability.Exceptions {
		if t.Source == "leave" {
			continue // comes from HR
		}
		if !opshttp.ValidDate(t.From) || !opshttp.ValidDate(t.To) || t.To < t.From {
			return opshttp.Invalid("time off needs a valid date range")
		}
		manual = append(manual, TimeOff{From: t.From, To: t.To, Note: strings.TrimSpace(t.Note)})
	}
	in.Availability.Exceptions = manual
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_employees WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, employeeID).Scan(&n); err != nil || n == 0 {
		return opshttp.ErrNotFound
	}
	av, _ := json.Marshal(in.Availability)
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.shf_staff (org_id, employee_id, role, color, hourly_rate, target_hours, max_hours, availability)
		VALUES ($1,$2::uuid,$3,$4,$5,$6,$7,$8::jsonb)
		ON CONFLICT (org_id, employee_id) DO UPDATE SET role=EXCLUDED.role, color=EXCLUDED.color, hourly_rate=EXCLUDED.hourly_rate,
		  target_hours=EXCLUDED.target_hours, max_hours=EXCLUDED.max_hours, availability=EXCLUDED.availability`,
		c.OrgID, employeeID, strings.TrimSpace(in.Role), in.Color, in.HourlyRate, in.TargetHours, in.MaxHours, av)
	return err
}

// ---- shifts ----

const shiftCols = `id::text, to_char(shift_date,'YYYY-MM-DD'), start_time, end_time, break_minutes, role, location, notes, coalesce(employee_id::text,''), published`

func scanShift(row pgx.Row) (Shift, error) {
	var s Shift
	err := row.Scan(&s.ID, &s.Date, &s.Start, &s.End, &s.BreakMinutes, &s.Role, &s.Location, &s.Notes, &s.EmployeeID, &s.Published)
	return s, err
}

func (r *Repository) shiftsIn(ctx context.Context, orgID, from, to string, publishedOnly bool) ([]Shift, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+shiftCols+` FROM adrient.shf_shifts WHERE org_id=$1 AND shift_date BETWEEN $2::date AND $3::date
		AND (NOT $4 OR published) ORDER BY shift_date, start_time, id LIMIT 5000`, orgID, from, to, publishedOnly)
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

func checkRange(from, to string) error {
	if !opshttp.ValidDate(from) || !opshttp.ValidDate(to) || to < from {
		return opshttp.Invalid("from and to must be dates (YYYY-MM-DD) with to on or after from")
	}
	if d := dayNumber(to) - dayNumber(from); d > 370 {
		return opshttp.Invalid("the range is limited to one year")
	}
	return nil
}

// ListShifts returns the shifts in a range. People who cannot edit the rota
// see published shifts only.
func (r *Repository) ListShifts(ctx context.Context, c Caller, from, to string) ([]Shift, error) {
	if err := checkRange(from, to); err != nil {
		return nil, err
	}
	return r.shiftsIn(ctx, c.OrgID, from, to, !c.Scheduler)
}

// GetShift returns one shift (unpublished ones only to schedulers).
func (r *Repository) GetShift(ctx context.Context, c Caller, id string) (Shift, error) {
	if !opshttp.IsUUID(id) {
		return Shift{}, opshttp.ErrNotFound
	}
	s, err := scanShift(r.pool.QueryRow(ctx, `SELECT `+shiftCols+` FROM adrient.shf_shifts WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, id))
	if err != nil || (!s.Published && !c.Scheduler) {
		return Shift{}, opshttp.ErrNotFound
	}
	return s, nil
}

// SaveResult is a stored shift with the non-blocking warnings of its assignment.
type SaveResult struct {
	Shift    Shift    `json:"shift"`
	Warnings []string `json:"warnings"`
}

func (r *Repository) staffMember(ctx context.Context, c Caller, id, date string) (Staff, error) {
	st, err := r.Staff(ctx, c, date, opshttp.AddDays(date, 1))
	if err != nil {
		return Staff{}, err
	}
	for _, s := range st {
		if s.EmployeeID == id {
			return s, nil
		}
	}
	return Staff{}, opshttp.Invalid("that person is not an active employee")
}

func (r *Repository) weekShifts(ctx context.Context, orgID, date string) ([]Shift, error) {
	w := StartOfWeek(date)
	return r.shiftsIn(ctx, orgID, opshttp.AddDays(w, -2), opshttp.AddDays(w, 9), false)
}

// SaveShift creates (id == "") or updates a shift and checks the assignment.
func (r *Repository) SaveShift(ctx context.Context, c Caller, id string, in Shift) (SaveResult, error) {
	if err := c.needScheduler(); err != nil {
		return SaveResult{}, err
	}
	in.Role, in.Location, in.Notes = strings.TrimSpace(in.Role), strings.TrimSpace(in.Location), strings.TrimSpace(in.Notes)
	if errs := ValidateShift(in); len(errs) > 0 {
		return SaveResult{}, opshttp.Invalid("%s", strings.Join(errs, " "))
	}
	if len(in.Role) > 80 || len(in.Location) > 120 || len(in.Notes) > 1000 {
		return SaveResult{}, opshttp.Invalid("role, location or notes are too long")
	}
	in.ID = id
	var warnings []string
	if in.EmployeeID != "" {
		st, err := r.staffMember(ctx, c, in.EmployeeID, in.Date)
		if err != nil {
			return SaveResult{}, err
		}
		set, _ := r.GetSettings(ctx, c.OrgID)
		all, err := r.weekShifts(ctx, c.OrgID, in.Date)
		if err != nil {
			return SaveResult{}, err
		}
		blocked, w := CheckAssign(in, st, all, set.Rules)
		if blocked != "" {
			return SaveResult{}, opshttp.Conflict("%s", blocked)
		}
		warnings = w
	}
	emp := in.EmployeeID
	var out Shift
	var err error
	if id == "" {
		out, err = scanShift(r.pool.QueryRow(ctx, `INSERT INTO adrient.shf_shifts (org_id, shift_date, start_time, end_time, break_minutes, role, location, notes, employee_id, published, created_by)
			VALUES ($1,$2::date,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid,$10,$11::uuid) RETURNING `+shiftCols,
			c.OrgID, in.Date, in.Start, in.End, in.BreakMinutes, in.Role, in.Location, in.Notes, emp, in.Published, c.UserID))
	} else {
		if !opshttp.IsUUID(id) {
			return SaveResult{}, opshttp.ErrNotFound
		}
		out, err = scanShift(r.pool.QueryRow(ctx, `UPDATE adrient.shf_shifts SET shift_date=$3::date, start_time=$4, end_time=$5, break_minutes=$6, role=$7, location=$8, notes=$9,
			employee_id=NULLIF($10,'')::uuid, published=$11, updated_at=now() WHERE org_id=$1 AND id=$2::uuid RETURNING `+shiftCols,
			c.OrgID, id, in.Date, in.Start, in.End, in.BreakMinutes, in.Role, in.Location, in.Notes, emp, in.Published))
		if errors.Is(err, pgx.ErrNoRows) {
			return SaveResult{}, opshttp.ErrNotFound
		}
	}
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{Shift: out, Warnings: warnings}, nil
}

// DeleteShift removes a shift (and its attendance).
func (r *Repository) DeleteShift(ctx context.Context, c Caller, id string) error {
	if err := c.needScheduler(); err != nil {
		return err
	}
	if !opshttp.IsUUID(id) {
		return opshttp.ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.shf_shifts WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return opshttp.ErrNotFound
	}
	return nil
}

// CopyWeek copies the shifts of the week starting fromWeek into the week
// starting toWeek, skipping ones already there. Copies are unpublished and
// keep their assignee unless that would overlap (then they stay open).
func (r *Repository) CopyWeek(ctx context.Context, c Caller, fromWeek, toWeek string) (int, error) {
	if err := c.needScheduler(); err != nil {
		return 0, err
	}
	if !opshttp.ValidDate(fromWeek) || !opshttp.ValidDate(toWeek) {
		return 0, opshttp.Invalid("weeks must be dates")
	}
	fromWeek, toWeek = StartOfWeek(fromWeek), StartOfWeek(toWeek)
	if fromWeek == toWeek {
		return 0, opshttp.Invalid("choose a different week to copy into")
	}
	src, err := r.shiftsIn(ctx, c.OrgID, fromWeek, opshttp.AddDays(fromWeek, 6), false)
	if err != nil {
		return 0, err
	}
	dst, err := r.shiftsIn(ctx, c.OrgID, opshttp.AddDays(toWeek, -1), opshttp.AddDays(toWeek, 7), false)
	if err != nil {
		return 0, err
	}
	shift := dayNumber(toWeek) - dayNumber(fromWeek)
	n := 0
	for _, s := range src {
		ns := s
		ns.ID, ns.Published = "", false
		ns.Date = opshttp.AddDays(s.Date, shift)
		dup := false
		for _, d := range dst {
			if d.Date == ns.Date && d.Start == ns.Start && d.End == ns.End && d.Role == ns.Role && d.EmployeeID == ns.EmployeeID {
				dup = true
			}
		}
		if dup {
			continue
		}
		if ns.EmployeeID != "" && len(FindConflicts(ns, dst, ns.EmployeeID)) > 0 {
			ns.EmployeeID = ""
		}
		saved, err := scanShift(r.pool.QueryRow(ctx, `INSERT INTO adrient.shf_shifts (org_id, shift_date, start_time, end_time, break_minutes, role, location, notes, employee_id, published, created_by)
			VALUES ($1,$2::date,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid,false,$10::uuid) RETURNING `+shiftCols,
			c.OrgID, ns.Date, ns.Start, ns.End, ns.BreakMinutes, ns.Role, ns.Location, ns.Notes, ns.EmployeeID, c.UserID))
		if err != nil {
			return n, err
		}
		dst = append(dst, saved)
		n++
	}
	return n, nil
}

// Publish sets the published flag on every shift in the range.
func (r *Repository) Publish(ctx context.Context, c Caller, from, to string, published bool) (int, error) {
	if err := c.needScheduler(); err != nil {
		return 0, err
	}
	if err := checkRange(from, to); err != nil {
		return 0, err
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.shf_shifts SET published=$4, updated_at=now() WHERE org_id=$1 AND shift_date BETWEEN $2::date AND $3::date AND published<>$4`, c.OrgID, from, to, published)
	return int(tag.RowsAffected()), err
}

// Suggest ranks people for one shift.
func (r *Repository) Suggest(ctx context.Context, c Caller, shiftID string) ([]Candidate, error) {
	if err := c.needScheduler(); err != nil {
		return nil, err
	}
	s, err := scanShift(r.pool.QueryRow(ctx, `SELECT `+shiftCols+` FROM adrient.shf_shifts WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, shiftID))
	if err != nil {
		return nil, opshttp.ErrNotFound
	}
	staff, err := r.Staff(ctx, c, s.Date, opshttp.AddDays(s.Date, 1))
	if err != nil {
		return nil, err
	}
	all, err := r.weekShifts(ctx, c.OrgID, s.Date)
	if err != nil {
		return nil, err
	}
	set, _ := r.GetSettings(ctx, c.OrgID)
	return SuggestAssignees(s, staff, all, set.Rules, 5), nil
}

// FillPlan proposes people for the open shifts in the range.
func (r *Repository) FillPlan(ctx context.Context, c Caller, from, to string) ([]FillProposal, error) {
	if err := c.needScheduler(); err != nil {
		return nil, err
	}
	if err := checkRange(from, to); err != nil {
		return nil, err
	}
	staff, err := r.Staff(ctx, c, from, to)
	if err != nil {
		return nil, err
	}
	all, err := r.shiftsIn(ctx, c.OrgID, opshttp.AddDays(from, -2), opshttp.AddDays(to, 9), false)
	if err != nil {
		return nil, err
	}
	set, _ := r.GetSettings(ctx, c.OrgID)
	var dates []string
	for d := from; d <= to; d = opshttp.AddDays(d, 1) {
		dates = append(dates, d)
	}
	return PlanFill(all, staff, set.Rules, dates), nil
}

// Assignment is one shift to hand to one person.
type Assignment struct {
	ShiftID    string `json:"shift_id"`
	EmployeeID string `json:"employee_id"`
}

// ApplyFill assigns several shifts at once; each is re-checked.
func (r *Repository) ApplyFill(ctx context.Context, c Caller, as []Assignment) (int, []string, error) {
	if err := c.needScheduler(); err != nil {
		return 0, nil, err
	}
	n, problems := 0, []string{}
	for _, a := range as {
		s, err := scanShift(r.pool.QueryRow(ctx, `SELECT `+shiftCols+` FROM adrient.shf_shifts WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, a.ShiftID))
		if err != nil {
			problems = append(problems, "a shift no longer exists")
			continue
		}
		s.EmployeeID = a.EmployeeID
		if _, err := r.SaveShift(ctx, c, s.ID, s); err != nil {
			problems = append(problems, fmt.Sprintf("%s %s: %s", s.Date, s.Start, strings.TrimPrefix(err.Error(), "conflict: ")))
			continue
		}
		n++
	}
	return n, problems, nil
}

// ---- coverage ----

// ListCoverage returns the coverage requirements.
func (r *Repository) ListCoverage(ctx context.Context, orgID string) ([]Coverage, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, role, days, min_count, start_time, end_time FROM adrient.shf_coverage WHERE org_id=$1 ORDER BY role, id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Coverage{}
	for rows.Next() {
		var v Coverage
		var raw []byte
		if err := rows.Scan(&v.ID, &v.Role, &raw, &v.Count, &v.Start, &v.End); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &v.Days)
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveCoverage creates or updates a requirement.
func (r *Repository) SaveCoverage(ctx context.Context, c Caller, id string, v Coverage) (Coverage, error) {
	if err := c.needScheduler(); err != nil {
		return v, err
	}
	v.Role = strings.TrimSpace(v.Role)
	if v.Role == "" || v.Count < 1 || v.Count > 500 || len(v.Days) == 0 {
		return v, opshttp.Invalid("a requirement needs a role, at least one day and a headcount of 1 or more")
	}
	for _, d := range v.Days {
		if d < 0 || d > 6 {
			return v, opshttp.Invalid("days are 0 (Sunday) to 6 (Saturday)")
		}
	}
	if (v.Start != "" && !opshttp.ValidTime(v.Start)) || (v.End != "" && !opshttp.ValidTime(v.End)) || (v.Start == "") != (v.End == "") {
		return v, opshttp.Invalid("give both a start and an end time, or neither")
	}
	days, _ := json.Marshal(v.Days)
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.shf_coverage (org_id, role, days, min_count, start_time, end_time) VALUES ($1,$2,$3::jsonb,$4,$5,$6) RETURNING id::text`,
			c.OrgID, v.Role, days, v.Count, v.Start, v.End).Scan(&v.ID)
	} else {
		v.ID = id
		var tag int64
		t, e := r.pool.Exec(ctx, `UPDATE adrient.shf_coverage SET role=$3, days=$4::jsonb, min_count=$5, start_time=$6, end_time=$7 WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, id, v.Role, days, v.Count, v.Start, v.End)
		err = e
		if e == nil {
			tag = t.RowsAffected()
			if tag == 0 {
				err = opshttp.ErrNotFound
			}
		}
	}
	return v, err
}

// DeleteCoverage removes a requirement.
func (r *Repository) DeleteCoverage(ctx context.Context, c Caller, id string) error {
	if err := c.needScheduler(); err != nil {
		return err
	}
	if !opshttp.IsUUID(id) {
		return opshttp.ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.shf_coverage WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, id)
	return err
}

// Gaps returns the unmet coverage in a range.
func (r *Repository) Gaps(ctx context.Context, c Caller, from, to string) ([]Gap, error) {
	if err := checkRange(from, to); err != nil {
		return nil, err
	}
	reqs, err := r.ListCoverage(ctx, c.OrgID)
	if err != nil {
		return nil, err
	}
	sh, err := r.shiftsIn(ctx, c.OrgID, opshttp.AddDays(from, -1), to, !c.Scheduler)
	if err != nil {
		return nil, err
	}
	var dates []string
	for d := from; d <= to; d = opshttp.AddDays(d, 1) {
		dates = append(dates, d)
	}
	g := CoverageGaps(sh, reqs, dates)
	if g == nil {
		g = []Gap{}
	}
	return g, nil
}

// ---- swaps ----

func (r *Repository) names(ctx context.Context, orgID string, ids ...string) map[string]string {
	out := map[string]string{}
	rows, err := r.pool.Query(ctx, `SELECT id::text, trim(first_name||' '||last_name) FROM adrient.hr_employees WHERE org_id=$1 AND id = ANY($2::uuid[])`, orgID, ids)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, n string
		if rows.Scan(&id, &n) == nil {
			out[id] = n
		}
	}
	return out
}

// RequestSwap asks to hand one of your shifts to a colleague. Schedulers may
// file one for anybody.
func (r *Repository) RequestSwap(ctx context.Context, c Caller, shiftID, toEmployee, reason string) (Swap, error) {
	s, err := scanShift(r.pool.QueryRow(ctx, `SELECT `+shiftCols+` FROM adrient.shf_shifts WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, shiftID))
	if err != nil {
		return Swap{}, opshttp.ErrNotFound
	}
	if s.EmployeeID == "" {
		return Swap{}, opshttp.Invalid("only assigned shifts can be swapped")
	}
	if !c.Scheduler && s.EmployeeID != c.EmployeeID {
		return Swap{}, opshttp.Forbidden("you can only swap your own shifts")
	}
	if toEmployee == "" || toEmployee == s.EmployeeID {
		return Swap{}, opshttp.Invalid("choose a different colleague to take the shift")
	}
	if len(reason) > 500 {
		return Swap{}, opshttp.Invalid("reason is too long")
	}
	var dup int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.shf_swaps WHERE org_id=$1 AND shift_id=$2::uuid AND status='pending'`, c.OrgID, shiftID).Scan(&dup)
	if dup > 0 {
		return Swap{}, opshttp.Conflict("this shift already has a pending swap request")
	}
	if _, err := r.staffMember(ctx, c, toEmployee, s.Date); err != nil {
		return Swap{}, err
	}
	snap, _ := json.Marshal(map[string]any{"date": s.Date, "start": s.Start, "end": s.End, "role": s.Role})
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.shf_swaps (org_id, shift_id, from_employee_id, to_employee_id, reason, snapshot, requested_by)
		VALUES ($1,$2::uuid,$3::uuid,$4::uuid,$5,$6::jsonb,$7::uuid) RETURNING id::text`, c.OrgID, shiftID, s.EmployeeID, toEmployee, strings.TrimSpace(reason), snap, c.UserID).Scan(&id); err != nil {
		return Swap{}, err
	}
	return r.getSwap(ctx, c, id)
}

func (r *Repository) swapRows(ctx context.Context, c Caller, where string, args ...any) ([]Swap, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, coalesce(shift_id::text,''), from_employee_id::text, to_employee_id::text, reason, status, snapshot, to_char(created_at,'YYYY-MM-DD HH24:MI')
		FROM adrient.shf_swaps WHERE org_id=$1 `+where+` ORDER BY created_at DESC LIMIT 200`, append([]any{c.OrgID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Swap
	for rows.Next() {
		var w Swap
		var raw []byte
		if err := rows.Scan(&w.ID, &w.ShiftID, &w.FromEmployee, &w.ToEmployee, &w.Reason, &w.Status, &raw, &w.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &w.Snapshot)
		out = append(out, w)
	}
	rows.Close()
	ids := []string{}
	for _, w := range out {
		ids = append(ids, w.FromEmployee, w.ToEmployee)
	}
	nm := r.names(ctx, c.OrgID, ids...)
	for i := range out {
		w := &out[i]
		w.FromName, w.ToName = nm[w.FromEmployee], nm[w.ToEmployee]
		w.CanResolve = c.Scheduler && w.Status == "pending"
		w.CanCancel = w.Status == "pending" && (c.Scheduler || w.FromEmployee == c.EmployeeID)
	}
	if out == nil {
		out = []Swap{}
	}
	return out, nil
}

func (r *Repository) getSwap(ctx context.Context, c Caller, id string) (Swap, error) {
	if !opshttp.IsUUID(id) {
		return Swap{}, opshttp.ErrNotFound
	}
	l, err := r.swapRows(ctx, c, `AND id=$2::uuid`, id)
	if err != nil {
		return Swap{}, err
	}
	if len(l) == 0 {
		return Swap{}, opshttp.ErrNotFound
	}
	return l[0], nil
}

// ListSwaps returns swap requests: all for schedulers, your own otherwise.
func (r *Repository) ListSwaps(ctx context.Context, c Caller, status string) ([]Swap, error) {
	if c.Scheduler {
		return r.swapRows(ctx, c, `AND ($2='' OR status=$2)`, status)
	}
	if c.EmployeeID == "" {
		return []Swap{}, nil
	}
	return r.swapRows(ctx, c, `AND ($2='' OR status=$2) AND (from_employee_id=$3::uuid OR to_employee_id=$3::uuid)`, status, c.EmployeeID)
}

// SwapWarnings previews the non-blocking warnings of approving a swap.
func (r *Repository) SwapWarnings(ctx context.Context, c Caller, id string) ([]string, error) {
	w, err := r.getSwap(ctx, c, id)
	if err != nil {
		return nil, err
	}
	s, err := scanShift(r.pool.QueryRow(ctx, `SELECT `+shiftCols+` FROM adrient.shf_shifts WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, w.ShiftID))
	if err != nil {
		return nil, opshttp.Conflict("the shift in this request no longer exists")
	}
	to, err := r.staffMember(ctx, c, w.ToEmployee, s.Date)
	if err != nil {
		return nil, err
	}
	all, _ := r.weekShifts(ctx, c.OrgID, s.Date)
	set, _ := r.GetSettings(ctx, c.OrgID)
	var n int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.shf_attendance WHERE shift_id=$1::uuid`, s.ID).Scan(&n)
	out := EvaluateSwap(s, to, all, set.Rules, n > 0)
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// ResolveSwap approves or declines (schedulers) or cancels (the requester).
func (r *Repository) ResolveSwap(ctx context.Context, c Caller, id, action string) (Swap, error) {
	w, err := r.getSwap(ctx, c, id)
	if err != nil {
		return w, err
	}
	if w.Status != "pending" {
		return w, opshttp.Conflict("this request has already been resolved")
	}
	status := map[string]string{"approve": "approved", "decline": "declined", "cancel": "cancelled"}[action]
	if status == "" {
		return w, opshttp.Invalid("action must be approve, decline or cancel")
	}
	if action == "cancel" {
		if !w.CanCancel {
			return w, opshttp.Forbidden("only the requester or a manager can cancel")
		}
	} else if !c.Scheduler {
		return w, opshttp.Forbidden("only managers and HR admins can decide swaps")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return w, err
	}
	defer tx.Rollback(ctx)
	if action == "approve" {
		s, err := scanShift(tx.QueryRow(ctx, `SELECT `+shiftCols+` FROM adrient.shf_shifts WHERE org_id=$1 AND id=$2::uuid FOR UPDATE`, c.OrgID, w.ShiftID))
		if err != nil {
			return w, opshttp.Conflict("the shift in this request no longer exists")
		}
		if s.EmployeeID != w.FromEmployee {
			return w, opshttp.Conflict("the shift has been reassigned since this request was made")
		}
		all, _ := r.weekShifts(ctx, c.OrgID, s.Date)
		if len(FindConflicts(s, all, w.ToEmployee)) > 0 {
			return w, opshttp.Conflict("%s already has an overlapping shift then", w.ToName)
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.shf_shifts SET employee_id=$3::uuid, updated_at=now() WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, w.ShiftID, w.ToEmployee); err != nil {
			return w, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.shf_swaps SET status=$3, resolved_by=$4::uuid, resolved_at=now() WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, id, status, c.UserID); err != nil {
		return w, err
	}
	if err := tx.Commit(ctx); err != nil {
		return w, err
	}
	return r.getSwap(ctx, c, id)
}

// ---- attendance and timesheet ----

// SetAttendance records attendance for a shift (schedulers), or a clock event
// for your own shift (clockIn/clockOut with an empty status).
func (r *Repository) SetAttendance(ctx context.Context, c Caller, shiftID string, a Attendance) (Attendance, error) {
	s, err := scanShift(r.pool.QueryRow(ctx, `SELECT `+shiftCols+` FROM adrient.shf_shifts WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, shiftID))
	if err != nil {
		return a, opshttp.ErrNotFound
	}
	if !c.Scheduler {
		return a, opshttp.Forbidden("only managers and HR admins can record attendance")
	}
	if a.Status == "" {
		a.Status = "present"
	}
	if a.Status != "present" && a.Status != "late" && a.Status != "absent" {
		return a, opshttp.Invalid("status must be present, late or absent")
	}
	if (a.ClockIn != "" && !opshttp.ValidTime(a.ClockIn)) || (a.ClockOut != "" && !opshttp.ValidTime(a.ClockOut)) {
		return a, opshttp.Invalid("clock times must be HH:MM")
	}
	if s.EmployeeID == "" {
		return a, opshttp.Invalid("an open shift has no attendance")
	}
	if a.Status == "absent" {
		a.ClockIn, a.ClockOut = "", ""
	}
	a.ShiftID = shiftID
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.shf_attendance (shift_id, org_id, status, clock_in, clock_out, note) VALUES ($1::uuid,$2,$3,$4,$5,$6)
		ON CONFLICT (shift_id) DO UPDATE SET status=EXCLUDED.status, clock_in=EXCLUDED.clock_in, clock_out=EXCLUDED.clock_out, note=EXCLUDED.note, updated_at=now()`,
		shiftID, c.OrgID, a.Status, a.ClockIn, a.ClockOut, strings.TrimSpace(a.Note))
	return a, err
}

// Clock records clock-in or clock-out on your own published shift of today
// (or yesterday for overnight shifts).
func (r *Repository) Clock(ctx context.Context, c Caller, shiftID, dir string) (Attendance, error) {
	s, err := scanShift(r.pool.QueryRow(ctx, `SELECT `+shiftCols+` FROM adrient.shf_shifts WHERE org_id=$1 AND id=$2::uuid`, c.OrgID, shiftID))
	if err != nil {
		return Attendance{}, opshttp.ErrNotFound
	}
	if s.EmployeeID == "" || s.EmployeeID != c.EmployeeID || !s.Published {
		return Attendance{}, opshttp.Forbidden("you can only clock on your own published shifts")
	}
	now := r.now()
	today := now.Format("2006-01-02")
	if s.Date != today && s.Date != opshttp.AddDays(today, -1) {
		return Attendance{}, opshttp.Invalid("you can only clock on a shift of today (or last night's)")
	}
	hhmm := now.Format("15:04")
	cur := r.attendanceFor(ctx, c.OrgID, []string{shiftID})[shiftID]
	set, _ := r.GetSettings(ctx, c.OrgID)
	switch dir {
	case "in":
		if cur.ClockIn != "" {
			return cur, opshttp.Conflict("already clocked in")
		}
		cur = Attendance{ShiftID: shiftID, Status: "present", ClockIn: hhmm}
		if LateMinutes(s, hhmm) > set.GraceMinutes {
			cur.Status = "late"
		}
	case "out":
		if cur.ClockIn == "" || cur.ClockOut != "" {
			return cur, opshttp.Conflict("clock in first, and only once")
		}
		cur.ClockOut = hhmm
	default:
		return cur, opshttp.Invalid("direction must be in or out")
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.shf_attendance (shift_id, org_id, status, clock_in, clock_out, note) VALUES ($1::uuid,$2,$3,$4,$5,$6)
		ON CONFLICT (shift_id) DO UPDATE SET status=EXCLUDED.status, clock_in=EXCLUDED.clock_in, clock_out=EXCLUDED.clock_out, updated_at=now()`,
		shiftID, c.OrgID, cur.Status, cur.ClockIn, cur.ClockOut, cur.Note)
	return cur, err
}

func (r *Repository) attendanceFor(ctx context.Context, orgID string, ids []string) map[string]Attendance {
	out := map[string]Attendance{}
	rows, err := r.pool.Query(ctx, `SELECT shift_id::text, status, clock_in, clock_out, note FROM adrient.shf_attendance WHERE org_id=$1 AND shift_id = ANY($2::uuid[])`, orgID, ids)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var a Attendance
		if rows.Scan(&a.ShiftID, &a.Status, &a.ClockIn, &a.ClockOut, &a.Note) == nil {
			out[a.ShiftID] = a
		}
	}
	return out
}

// Attendance lists the records of the shifts in a range.
func (r *Repository) Attendance(ctx context.Context, c Caller, from, to string) ([]Attendance, error) {
	sh, err := r.ListShifts(ctx, c, from, to)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(sh))
	for _, s := range sh {
		ids = append(ids, s.ID)
	}
	m := r.attendanceFor(ctx, c.OrgID, ids)
	out := make([]Attendance, 0, len(m))
	for _, a := range m {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ShiftID < out[j].ShiftID })
	return out, nil
}

// TimesheetRow is one assigned shift with what was worked and paid.
type TimesheetRow struct {
	ShiftID    string   `json:"shift_id"`
	Date       string   `json:"date"`
	EmployeeID string   `json:"employee_id"`
	Employee   string   `json:"employee"`
	Role       string   `json:"role"`
	Start      string   `json:"start"`
	End        string   `json:"end"`
	Break      int      `json:"break_minutes"`
	Scheduled  float64  `json:"scheduled_hours"`
	Status     string   `json:"status"`
	ClockIn    string   `json:"clock_in"`
	ClockOut   string   `json:"clock_out"`
	Worked     *float64 `json:"worked_hours"`
	Late       int      `json:"minutes_late"`
	Paid       float64  `json:"paid_hours"`
	Rate       *float64 `json:"hourly_rate"`
	Pay        *float64 `json:"pay"`
}

// TimesheetSummary totals one employee.
type TimesheetSummary struct {
	EmployeeID  string   `json:"employee_id"`
	Employee    string   `json:"employee"`
	Shifts      int      `json:"shifts"`
	Scheduled   float64  `json:"scheduled_hours"`
	Worked      float64  `json:"worked_hours"`
	Paid        float64  `json:"paid_hours"`
	Present     int      `json:"present"`
	Late        int      `json:"late"`
	Absent      int      `json:"absent"`
	NotRecorded int      `json:"not_recorded"`
	Pay         *float64 `json:"pay"`
}

func hrs(min int) float64 { return opshttp.Round2(float64(min) / 60) }

// Timesheet computes the rows and per-employee summary for a range. Pay is
// shown to schedulers only; other people see their own rows.
func (r *Repository) Timesheet(ctx context.Context, c Caller, from, to string) ([]TimesheetRow, []TimesheetSummary, error) {
	sh, err := r.ListShifts(ctx, c, from, to)
	if err != nil {
		return nil, nil, err
	}
	staff, err := r.Staff(ctx, c, from, to)
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]Staff{}
	for _, s := range staff {
		byID[s.EmployeeID] = s
	}
	ids := make([]string, 0, len(sh))
	for _, s := range sh {
		ids = append(ids, s.ID)
	}
	att := r.attendanceFor(ctx, c.OrgID, ids)
	rows := []TimesheetRow{}
	for _, s := range sh {
		if s.EmployeeID == "" || (!c.Scheduler && s.EmployeeID != c.EmployeeID) {
			continue
		}
		e := byID[s.EmployeeID]
		a, has := att[s.ID]
		var ap *Attendance
		if has {
			ap = &a
		}
		row := TimesheetRow{ShiftID: s.ID, Date: s.Date, EmployeeID: s.EmployeeID, Employee: e.Name, Role: s.Role, Start: s.Start, End: s.End, Break: s.BreakMinutes,
			Scheduled: hrs(NetMinutes(s)), Status: "not recorded", Paid: hrs(PaidMinutes(s, ap)), Rate: e.HourlyRate}
		if row.Employee == "" {
			row.Employee = "Former employee"
		}
		if has {
			row.Status = a.Status
			if a.Status != "absent" {
				row.ClockIn, row.ClockOut = a.ClockIn, a.ClockOut
				row.Late = LateMinutes(s, a.ClockIn)
				if w, ok := DurationMinutes(a.ClockIn, a.ClockOut); ok {
					v := hrs(max(0, w-s.BreakMinutes))
					row.Worked = &v
				}
			}
		}
		if e.HourlyRate != nil {
			p := opshttp.Round2(row.Paid * *e.HourlyRate)
			row.Pay = &p
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		if rows[i].Start != rows[j].Start {
			return rows[i].Start < rows[j].Start
		}
		return rows[i].Employee < rows[j].Employee
	})
	sum := map[string]*TimesheetSummary{}
	for _, w := range rows {
		t := sum[w.EmployeeID]
		if t == nil {
			t = &TimesheetSummary{EmployeeID: w.EmployeeID, Employee: w.Employee}
			sum[w.EmployeeID] = t
		}
		t.Shifts++
		t.Scheduled = opshttp.Round2(t.Scheduled + w.Scheduled)
		t.Paid = opshttp.Round2(t.Paid + w.Paid)
		if w.Worked != nil {
			t.Worked = opshttp.Round2(t.Worked + *w.Worked)
		}
		if w.Pay != nil {
			v := opshttp.Round2(*w.Pay)
			if t.Pay != nil {
				v += *t.Pay
			}
			t.Pay = &v
		}
		switch w.Status {
		case "present":
			t.Present++
		case "late":
			t.Late++
		case "absent":
			t.Absent++
		default:
			t.NotRecorded++
		}
	}
	out := make([]TimesheetSummary, 0, len(sum))
	for _, t := range sum {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Employee < out[j].Employee })
	return rows, out, nil
}

// Cost prices the assigned shifts in a range (schedulers only).
func (r *Repository) Cost(ctx context.Context, c Caller, from, to string) (Cost, Settings, error) {
	set, _ := r.GetSettings(ctx, c.OrgID)
	if err := c.needScheduler(); err != nil {
		return Cost{}, set, err
	}
	if err := checkRange(from, to); err != nil {
		return Cost{}, set, err
	}
	staff, err := r.Staff(ctx, c, from, to)
	if err != nil {
		return Cost{}, set, err
	}
	sh, err := r.shiftsIn(ctx, c.OrgID, StartOfWeek(from), opshttp.AddDays(StartOfWeek(to), 6), false)
	if err != nil {
		return Cost{}, set, err
	}
	var dates []string
	for d := from; d <= to; d = opshttp.AddDays(d, 1) {
		dates = append(dates, d)
	}
	return LaborCost(sh, staff, dates, CostOptions{ThresholdHours: set.Overtime, Multiplier: set.OvertimeMult}), set, nil
}

// PayrollLines turns a month of shifts into one pay line per employee: paid
// hours (attendance when recorded, else scheduled) times the hourly rate.
func (r *Repository) PayrollLines(ctx context.Context, c Caller, period string) ([]payrollhours.Line, error) {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return nil, opshttp.Invalid("period must be YYYY-MM")
	}
	from := t.Format("2006-01-02")
	to := t.AddDate(0, 1, -1).Format("2006-01-02")
	rows, _, err := r.Timesheet(ctx, c, from, to)
	if err != nil {
		return nil, err
	}
	acc := map[string]*payrollhours.Line{}
	order := []string{}
	for _, w := range rows {
		if w.Rate == nil || w.EmployeeID == "" {
			continue
		}
		l := acc[w.EmployeeID]
		if l == nil {
			l = &payrollhours.Line{EmployeeID: w.EmployeeID}
			acc[w.EmployeeID] = l
			order = append(order, w.EmployeeID)
		}
		l.Hours = opshttp.Round2(l.Hours + w.Paid)
		l.Amount = opshttp.Round2(l.Amount + w.Paid**w.Rate)
	}
	out := make([]payrollhours.Line, 0, len(order))
	for _, id := range order {
		out = append(out, *acc[id])
	}
	return out, nil
}

// ExportPayroll adds the month's shift hours to the draft HR payroll run
// (HR admins only; HR enforces that again).
func (r *Repository) ExportPayroll(ctx context.Context, c Caller, period string) (payrollhours.Result, error) {
	if !c.Actor.IsHR {
		return payrollhours.Result{}, opshttp.Forbidden("only HR admins can export hours to payroll")
	}
	lines, err := r.PayrollLines(ctx, c, period)
	if err != nil {
		return payrollhours.Result{}, err
	}
	if len(lines) == 0 {
		return payrollhours.Result{}, opshttp.Invalid("no shifts with a pay rate in that month")
	}
	res, err := payrollhours.Export(ctx, r.hr, c.Actor, period, "shifts", lines)
	if errors.Is(err, hr.ErrConflict) {
		return res, opshttp.Conflict("the payroll run for that month is not a draft any more")
	}
	if errors.Is(err, hr.ErrInvalid) {
		return res, opshttp.Invalid("%s", strings.TrimPrefix(err.Error(), "invalid: "))
	}
	return res, err
}
