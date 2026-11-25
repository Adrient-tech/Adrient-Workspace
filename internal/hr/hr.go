// Package hr is the Adrient HR module: employees, leave, attendance, payroll
// (India) and self-service. Every public Repository method takes an Actor and
// enforces the three roles itself (HR admin, manager, employee), so the rules
// hold for every caller, not only the HTTP layer.
package hr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned when no record matches within the org (or the
	// caller may not know it exists).
	ErrNotFound = errors.New("not found")
	// ErrInvalid wraps validation failures.
	ErrInvalid = errors.New("invalid")
	// ErrForbidden is returned when the caller's role does not allow the action.
	ErrForbidden = errors.New("forbidden")
	// ErrConflict is returned for duplicates and state conflicts.
	ErrConflict = errors.New("conflict")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}
func forbidden(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrForbidden, fmt.Sprintf(format, a...))
}
func conflict(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, a...))
}

// CalendarPort creates and removes the out-of-office items shown on Calendar.
type CalendarPort interface {
	CreateOutOfOffice(ctx context.Context, orgID, ownerUserID, title string, start, end time.Time, allDay bool) (eventID string, err error)
	DeleteEvent(ctx context.Context, orgID, eventID string) error
}

// Notifier delivers an in-app notification.
type Notifier interface {
	Notify(ctx context.Context, orgID, toUserID, actorUserID, typ, title, body, targetURL string) error
}

// BlobStore stores HR document bytes (the Drive blob store).
type BlobStore interface {
	Put(ctx context.Context, key, mimeType string, size int64, body io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error)
	Delete(ctx context.Context, key string) error
}

// DriveFiles records a Drive file row for an uploaded HR document.
type DriveFiles interface {
	CreateHRFile(ctx context.Context, orgID, ownerID, name, mimeType, storageKey string, size int64) (fileID string, err error)
}

// PayrollPoster posts the payroll journal to Accounting.
type PayrollPoster interface {
	PostPayroll(ctx context.Context, orgID, userID string, in PayrollJournal) (journalID string, err error)
	ListApprovedReports(ctx context.Context, orgID, submitterUserID string) ([]ExpenseClaim, error)
	MarkReportsReimbursed(ctx context.Context, orgID, userID string, reportIDs []string, date string) error
}

// PayrollJournal is the posting handed to Accounting for one locked run.
type PayrollJournal struct {
	RunID                              string
	Period                             string
	Date                               string
	Gross, EmployerContrib, Reimburse  float64
	PFPayable, ESIPayable, PTPayable   float64
	TDSPayable, OtherDed, NetSalaryDue float64
	Employees                          int
}

// ExpenseClaim is an approved Accounting expense report awaiting reimbursement.
type ExpenseClaim struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Amount float64 `json:"amount"`
}

// Repository is the Postgres store for HR. Every query is org-scoped.
type Repository struct {
	pool   *pgxpool.Pool
	cal    CalendarPort
	notify Notifier
	blobs  BlobStore
	files  DriveFiles
	pay    PayrollPoster
	now    func() time.Time
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, now: time.Now}
}

// WithCalendar wires Calendar out-of-office sync.
func (r *Repository) WithCalendar(c CalendarPort) *Repository { r.cal = c; return r }

// WithNotifier wires in-app notifications.
func (r *Repository) WithNotifier(n Notifier) *Repository { r.notify = n; return r }

// WithDocuments wires Drive blob storage for employee documents.
func (r *Repository) WithDocuments(b BlobStore, f DriveFiles) *Repository {
	r.blobs, r.files = b, f
	return r
}

// WithAccounting wires the payroll journal poster.
func (r *Repository) WithAccounting(p PayrollPoster) *Repository { r.pay = p; return r }

// WithClock overrides the clock (tests).
func (r *Repository) WithClock(now func() time.Time) *Repository { r.now = now; return r }

type db interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Actor is the resolved caller. Roles are derived from the database, never from
// request input.
type Actor struct {
	OrgID      string
	UserID     string
	EmployeeID string // the caller's own employee record, "" if none
	IsHR       bool   // org admin or HR admin grant
	IsManager  bool   // has at least one direct report
}

// ActorFor resolves the caller. orgAdmin comes from the platform's authz.
func (r *Repository) ActorFor(ctx context.Context, orgID, userID string, orgAdmin bool) (Actor, error) {
	a := Actor{OrgID: orgID, UserID: userID, IsHR: orgAdmin}
	if !a.IsHR {
		var n int
		if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_admins WHERE org_id=$1 AND user_id=$2::uuid`, orgID, userID).Scan(&n); err != nil {
			return a, err
		}
		a.IsHR = n > 0
	}
	err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.hr_employees WHERE org_id=$1 AND user_id=$2::uuid AND status<>'exited'`, orgID, userID).Scan(&a.EmployeeID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return a, err
	}
	if a.EmployeeID != "" {
		var n int
		if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_employees WHERE org_id=$1 AND manager_id=$2::uuid AND status<>'exited'`, orgID, a.EmployeeID).Scan(&n); err != nil {
			return a, err
		}
		a.IsManager = n > 0
	}
	return a, nil
}

func (a Actor) requireHR() error {
	if !a.IsHR {
		return forbidden("HR admin only")
	}
	return nil
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidRe.MatchString(s) }

func validDate(s string) (string, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return "", invalid("date %q must be YYYY-MM-DD", s)
	}
	return t.Format("2006-01-02"), nil
}

func parseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func fmtDate(t time.Time) string { return t.Format("2006-01-02") }

func round2(f float64) float64 {
	if f < 0 {
		return -float64(int64(-f*100+0.5)) / 100
	}
	return float64(int64(f*100+0.5)) / 100
}

func (r *Repository) audit(ctx context.Context, d db, orgID, actorID, action, entity, id, detail string) {
	var actor any
	if isUUID(actorID) {
		actor = actorID
	}
	_, _ = d.Exec(ctx, `INSERT INTO adrient.hr_audit (org_id, actor_id, action, entity, entity_id, detail) VALUES ($1,$2,$3,$4,$5,$6)`,
		orgID, actor, action, entity, id, detail)
}

func (r *Repository) tell(ctx context.Context, orgID, to, actor, typ, title, body, url string) {
	if r.notify == nil || to == "" || to == actor {
		return
	}
	_ = r.notify.Notify(ctx, orgID, to, actor, typ, title, body, url)
}

// Settings are the per-org HR settings.
type Settings struct {
	Timezone        string  `json:"timezone"`
	WeeklyOff       []int   `json:"weekly_off"`
	LeaveYearStart  int     `json:"leave_year_start"`
	GeofenceEnabled bool    `json:"geofence_enabled"`
	GeofenceLat     float64 `json:"geofence_lat"`
	GeofenceLng     float64 `json:"geofence_lng"`
	GeofenceRadiusM int     `json:"geofence_radius_m"`
	LOPMode         string  `json:"lop_mode"`
	OTEnabled       bool    `json:"ot_enabled"`
	PFWageCeiling   float64 `json:"pf_wage_ceiling"`
	PFCapped        bool    `json:"pf_capped"`
	ESIWageLimit    float64 `json:"esi_wage_limit"`
	CompanyName     string  `json:"company_name"`
	CompanyAddress  string  `json:"company_address"`
}

// GetSettings returns the org settings, creating defaults and the starter data
// (default shift, leave types, holiday calendar, PT slabs) on first use.
func (r *Repository) GetSettings(ctx context.Context, orgID string) (Settings, error) {
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.hr_settings (org_id) VALUES ($1) ON CONFLICT DO NOTHING`, orgID); err != nil {
		return Settings{}, err
	}
	var s Settings
	var wo []byte
	err := r.pool.QueryRow(ctx, `SELECT timezone, weekly_off, leave_year_start, geofence_enabled, geofence_lat, geofence_lng, geofence_radius_m,
		lop_mode, ot_enabled, pf_wage_ceiling::float8, pf_capped, esi_wage_limit::float8, company_name, company_address
		FROM adrient.hr_settings WHERE org_id=$1`, orgID).
		Scan(&s.Timezone, &wo, &s.LeaveYearStart, &s.GeofenceEnabled, &s.GeofenceLat, &s.GeofenceLng, &s.GeofenceRadiusM,
			&s.LOPMode, &s.OTEnabled, &s.PFWageCeiling, &s.PFCapped, &s.ESIWageLimit, &s.CompanyName, &s.CompanyAddress)
	if err != nil {
		return s, err
	}
	_ = json.Unmarshal(wo, &s.WeeklyOff)
	if s.WeeklyOff == nil {
		s.WeeklyOff = []int{}
	}
	return s, nil
}

// SaveSettings validates and stores settings (HR only).
func (r *Repository) SaveSettings(ctx context.Context, a Actor, s Settings) (Settings, error) {
	if err := a.requireHR(); err != nil {
		return s, err
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil || s.Timezone == "" {
		return s, invalid("unknown timezone %q", s.Timezone)
	}
	for _, d := range s.WeeklyOff {
		if d < 0 || d > 6 {
			return s, invalid("weekly_off values are weekday numbers 0 (Sunday) to 6")
		}
	}
	if s.LeaveYearStart < 1 || s.LeaveYearStart > 12 {
		return s, invalid("leave_year_start must be 1-12")
	}
	if s.LOPMode != "leave_only" && s.LOPMode != "attendance_and_leave" {
		return s, invalid("lop_mode must be leave_only or attendance_and_leave")
	}
	if s.GeofenceEnabled {
		if s.GeofenceRadiusM < 20 || s.GeofenceRadiusM > 50000 {
			return s, invalid("geofence radius must be 20 to 50000 metres")
		}
		if s.GeofenceLat < -90 || s.GeofenceLat > 90 || s.GeofenceLng < -180 || s.GeofenceLng > 180 || (s.GeofenceLat == 0 && s.GeofenceLng == 0) {
			return s, invalid("set a valid geofence latitude and longitude")
		}
	}
	if s.PFWageCeiling <= 0 || s.ESIWageLimit <= 0 {
		return s, invalid("PF wage ceiling and ESI wage limit must be positive")
	}
	if _, err := r.GetSettings(ctx, a.OrgID); err != nil {
		return s, err
	}
	wo, _ := json.Marshal(s.WeeklyOff)
	_, err := r.pool.Exec(ctx, `UPDATE adrient.hr_settings SET timezone=$2, weekly_off=$3, leave_year_start=$4, geofence_enabled=$5, geofence_lat=$6,
		geofence_lng=$7, geofence_radius_m=$8, lop_mode=$9, ot_enabled=$10, pf_wage_ceiling=$11, pf_capped=$12, esi_wage_limit=$13,
		company_name=$14, company_address=$15, updated_at=now() WHERE org_id=$1`,
		a.OrgID, s.Timezone, wo, s.LeaveYearStart, s.GeofenceEnabled, s.GeofenceLat, s.GeofenceLng, s.GeofenceRadiusM,
		s.LOPMode, s.OTEnabled, s.PFWageCeiling, s.PFCapped, s.ESIWageLimit, strings.TrimSpace(s.CompanyName), strings.TrimSpace(s.CompanyAddress))
	if err != nil {
		return s, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "update", "settings", a.OrgID, "")
	return r.GetSettings(ctx, a.OrgID)
}

func (r *Repository) location(ctx context.Context, orgID string) *time.Location {
	s, err := r.GetSettings(ctx, orgID)
	if err == nil {
		if loc, e := time.LoadLocation(s.Timezone); e == nil {
			return loc
		}
	}
	return time.FixedZone("IST", 5*3600+1800)
}

// today returns the org-local calendar date.
func (r *Repository) today(ctx context.Context, orgID string) time.Time {
	loc := r.location(ctx, orgID)
	n := r.now().In(loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// ---- HR admins ----

// ListHRAdmins lists explicit HR admin grants.
func (r *Repository) ListHRAdmins(ctx context.Context, a Actor) ([]map[string]string, error) {
	if err := a.requireHR(); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT u.id::text, u.display_name, u.email FROM adrient.hr_admins h JOIN adrient.users u ON u.id=h.user_id
		WHERE h.org_id=$1 ORDER BY u.display_name`, a.OrgID)
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
		out = append(out, map[string]string{"user_id": id, "name": n, "email": e})
	}
	return out, rows.Err()
}

// SetHRAdmin grants or revokes the HR admin role for a user.
func (r *Repository) SetHRAdmin(ctx context.Context, a Actor, userID string, on bool) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	if !isUUID(userID) {
		return invalid("bad user id")
	}
	var ok bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE id=$1::uuid AND org_id=$2)`, userID, a.OrgID).Scan(&ok); err != nil || !ok {
		return invalid("user is not in this organisation")
	}
	if on {
		_, err := r.pool.Exec(ctx, `INSERT INTO adrient.hr_admins (org_id, user_id) VALUES ($1,$2::uuid) ON CONFLICT DO NOTHING`, a.OrgID, userID)
		r.audit(ctx, r.pool, a.OrgID, a.UserID, "grant", "hr_admin", userID, "")
		return err
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.hr_admins WHERE org_id=$1 AND user_id=$2::uuid`, a.OrgID, userID)
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "revoke", "hr_admin", userID, "")
	return err
}
