// Package talent is the talent half of the HRMS: recruitment (ATS), onboarding
// and offboarding, performance, learning and policies, announcements and
// recognition. People are referenced by org user id only; the HR core in
// internal/hr is not a dependency.
package talent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned when no record matches within the org (or the
	// caller may not know it exists).
	ErrNotFound = errors.New("not found")
	// ErrInvalid wraps validation failures.
	ErrInvalid = errors.New("invalid")
	// ErrForbidden is returned when the caller lacks access to an existing record.
	ErrForbidden = errors.New("forbidden")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Actor is the authenticated caller.
type Actor struct {
	OrgID  string
	UserID string
	// IsHR is true for org admins and users listed as HR in talent settings.
	IsHR bool
}

// Repository is the Postgres store. Every query is org-scoped.
type Repository struct {
	pool *pgxpool.Pool
	Deps Deps
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Pool exposes the pool for integration tests.
func (r *Repository) Pool() *pgxpool.Pool { return r.pool }

// rows runs a SELECT and returns it as a JSON array.
func (r *Repository) rows(ctx context.Context, q string, args ...any) (json.RawMessage, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(json_agg(t),'[]'::json)::text FROM (`+q+`) t`, args...).Scan(&raw)
	return raw, err
}

// row runs a SELECT and returns the first row as a JSON object.
func (r *Repository) row(ctx context.Context, q string, args ...any) (json.RawMessage, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT row_to_json(t)::text FROM (`+q+`) t LIMIT 1`, args...).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return raw, err
}

func clean(s string) string { return strings.TrimSpace(s) }

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	return false
}

func contains(list []string, v string) bool {
	for _, a := range list {
		if a == v {
			return true
		}
	}
	return false
}

// nullUUID turns "" into nil for UUID columns.
func nullUUID(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func uuidList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (r *Repository) audit(ctx context.Context, a Actor, action, rtype, rid string) {
	if r.Deps.Audit != nil {
		r.Deps.Audit(ctx, a, action, rtype, rid)
	}
}

// ---------- settings ----------

// Stage is one open pipeline stage.
type Stage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Terminal pipeline outcomes.
const (
	StageHired    = "hired"
	StageRejected = "rejected"
)

// ValidateStages checks custom open stages: 2..10, unique non-empty ids, no terminals.
func ValidateStages(st []Stage) error {
	if len(st) < 2 || len(st) > 10 {
		return invalid("between 2 and 10 stages are required")
	}
	seen := map[string]bool{}
	for _, s := range st {
		id := strings.TrimSpace(s.ID)
		if id == "" || strings.TrimSpace(s.Name) == "" {
			return invalid("every stage needs an id and a name")
		}
		if id == StageHired || id == StageRejected {
			return invalid("%q is reserved", id)
		}
		if seen[id] {
			return invalid("duplicate stage id %q", id)
		}
		seen[id] = true
	}
	return nil
}

// Settings are per-org talent settings.
type Settings struct {
	Stages         []Stage  `json:"stages"`
	HRUserIDs      []string `json:"hr_user_ids"`
	CareerEnabled  bool     `json:"career_enabled"`
	CareerTitle    string   `json:"career_title"`
	CareerBlurb    string   `json:"career_blurb"`
	PrivacyText    string   `json:"privacy_text"`
	RetentionDays  int      `json:"retention_days"`
	Currency       string   `json:"currency"`
	OfferTemplate  string   `json:"offer_template"`
	ResumeFolderID string   `json:"resume_folder_id"`
}

// DefaultOfferTemplate is used when the org has not written its own.
const DefaultOfferTemplate = `Dear {{name}},

We are delighted to offer you the position of {{job_title}} in {{department}} at {{company}}.

Compensation: {{salary}} per year.
Proposed start date: {{start_date}}.

This offer is subject to the usual background and reference checks. Please accept it using the secure link in the email we sent with this letter.

Sincerely,
{{sender}}
{{company}}`

// GetSettings returns the org settings, creating the default row on first use.
func (r *Repository) GetSettings(ctx context.Context, org string) (Settings, error) {
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_settings (org_id) VALUES ($1) ON CONFLICT DO NOTHING`, org); err != nil {
		return Settings{}, err
	}
	var s Settings
	var raw []byte
	var hr []string
	err := r.pool.QueryRow(ctx, `SELECT stages, COALESCE(hr_user_ids::text[], '{}'), career_enabled, career_title, career_blurb,
		privacy_text, retention_days, currency, offer_template, COALESCE(resume_folder_id::text,'')
		FROM adrient.talent_settings WHERE org_id=$1`, org).Scan(&raw, &hr, &s.CareerEnabled, &s.CareerTitle,
		&s.CareerBlurb, &s.PrivacyText, &s.RetentionDays, &s.Currency, &s.OfferTemplate, &s.ResumeFolderID)
	if err != nil {
		return s, err
	}
	_ = json.Unmarshal(raw, &s.Stages)
	s.HRUserIDs = hr
	if s.HRUserIDs == nil {
		s.HRUserIDs = []string{}
	}
	return s, nil
}

// SaveSettings updates the org settings. HR only. Applications in a removed
// stage move to the first remaining stage.
func (r *Repository) SaveSettings(ctx context.Context, a Actor, s Settings) (Settings, error) {
	if !a.IsHR {
		return s, ErrForbidden
	}
	if err := ValidateStages(s.Stages); err != nil {
		return s, err
	}
	if s.Currency == "" {
		s.Currency = "INR"
	}
	if s.RetentionDays < 30 || s.RetentionDays > 3650 {
		return s, invalid("retention must be between 30 and 3650 days")
	}
	cur, err := r.GetSettings(ctx, a.OrgID)
	if err != nil {
		return s, err
	}
	keep := map[string]bool{}
	for _, st := range s.Stages {
		keep[st.ID] = true
	}
	for _, old := range cur.Stages {
		if !keep[old.ID] {
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.talent_applications SET stage=$3, stage_entered_at=now()
				WHERE org_id=$1 AND stage=$2 AND status='active'`, a.OrgID, old.ID, s.Stages[0].ID); err != nil {
				return s, err
			}
		}
	}
	b, _ := json.Marshal(s.Stages)
	_, err = r.pool.Exec(ctx, `UPDATE adrient.talent_settings SET stages=$2, hr_user_ids=$3::uuid[], career_enabled=$4,
		career_title=$5, career_blurb=$6, privacy_text=$7, retention_days=$8, currency=$9, offer_template=$10, updated_at=now()
		WHERE org_id=$1`, a.OrgID, b, uuidList(s.HRUserIDs), s.CareerEnabled, clean(s.CareerTitle), s.CareerBlurb,
		s.PrivacyText, s.RetentionDays, s.Currency, s.OfferTemplate)
	if err != nil {
		return s, err
	}
	r.audit(ctx, a, "settings.update", "talent_settings", a.OrgID)
	return r.GetSettings(ctx, a.OrgID)
}

// IsHRUser reports whether the user is an HR user per settings.
func (r *Repository) IsHRUser(ctx context.Context, org, user string) bool {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT COALESCE($2::uuid = ANY(hr_user_ids), false) FROM adrient.talent_settings WHERE org_id=$1`, org, user).Scan(&ok)
	return err == nil && ok
}

// UserBrief is minimal directory info on a user.
type UserBrief struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	JobTitle   string `json:"job_title"`
	Phone      string `json:"phone"`
	Department string `json:"department"`
	ManagerID  string `json:"manager_id"`
}

// Directory lists org users (any member may read the directory).
func (r *Repository) Directory(ctx context.Context, org string) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT u.id::text AS id, COALESCE(NULLIF(u.display_name,''),u.email) AS name, u.email,
		u.job_title, u.phone, COALESCE(d.name,'') AS department, COALESCE(u.manager_id::text,'') AS manager_id
		FROM adrient.users u LEFT JOIN adrient.departments d ON d.id=u.department_id
		WHERE u.org_id=$1 AND COALESCE(u.status,'active')='active' ORDER BY 2 LIMIT 100`, org)
}

func (r *Repository) userBrief(ctx context.Context, org, id string) (UserBrief, error) {
	var u UserBrief
	err := r.pool.QueryRow(ctx, `SELECT u.id::text, COALESCE(NULLIF(u.display_name,''),u.email), u.email, u.job_title, u.phone,
		COALESCE(d.name,''), COALESCE(u.manager_id::text,'')
		FROM adrient.users u LEFT JOIN adrient.departments d ON d.id=u.department_id
		WHERE u.org_id=$1 AND u.id=$2`, org, id).Scan(&u.ID, &u.Name, &u.Email, &u.JobTitle, &u.Phone, &u.Department, &u.ManagerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// managerOf returns the manager id of a user ("" when none).
func (r *Repository) managerOf(ctx context.Context, org, user string) string {
	var m string
	_ = r.pool.QueryRow(ctx, `SELECT COALESCE(manager_id::text,'') FROM adrient.users WHERE org_id=$1 AND id=$2`, org, user).Scan(&m)
	return m
}

// managesChain reports whether mgr is a (direct or indirect) manager of user.
func (r *Repository) managesChain(ctx context.Context, org, mgr, user string) bool {
	if mgr == "" || user == "" || mgr == user {
		return false
	}
	var ok bool
	err := r.pool.QueryRow(ctx, `WITH RECURSIVE chain(id, depth) AS (
		SELECT manager_id, 1 FROM adrient.users WHERE org_id=$1 AND id=$3
		UNION ALL
		SELECT u.manager_id, c.depth+1 FROM adrient.users u JOIN chain c ON u.id=c.id WHERE u.org_id=$1 AND c.depth<32)
		SELECT EXISTS(SELECT 1 FROM chain WHERE id=$2::uuid)`, org, mgr, user).Scan(&ok)
	return err == nil && ok
}

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
