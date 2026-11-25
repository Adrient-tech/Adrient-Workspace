package talent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ErrUnavailable is returned when a feature needs an integration that is not wired.
var ErrUnavailable = errors.New("not available")

// ErrDuplicate is returned when a candidate already applied to the same opening.
var ErrDuplicate = errors.New("already applied")

// ResumeFile is an uploaded resume.
type ResumeFile struct {
	Name string
	Mime string
	Data []byte
}

// ApplyInput creates a candidate and an application.
type ApplyInput struct {
	JobID        string
	Name         string
	Email        string
	Phone        string
	Location     string
	CoverNote    string
	Source       string
	SourceDetail string
	Consent      bool
	Resume       *ResumeFile
}

// Sources a candidate can come from.
var sources = []string{"career_page", "referral", "job_board", "linkedin", "agency", "walk_in", "campus", "social", "other", "talent_pool"}

// Validate normalises the input.
func (in *ApplyInput) Validate() error {
	in.Name = clean(in.Name)
	in.Email = strings.ToLower(clean(in.Email))
	in.Phone = clean(in.Phone)
	if in.Name == "" || len(in.Name) > 120 {
		return invalid("name is required (max 120 characters)")
	}
	if len(in.Email) > 200 {
		return invalid("email is too long")
	}
	a, err := mail.ParseAddress(in.Email)
	if err != nil || a.Address != in.Email || !strings.Contains(in.Email[strings.Index(in.Email, "@"):], ".") {
		return invalid("a valid email address is required")
	}
	if len(in.Phone) > 32 {
		return invalid("phone is too long")
	}
	if len(in.CoverNote) > 5000 {
		return invalid("cover note is too long (max 5000 characters)")
	}
	if in.Source == "" {
		in.Source = "career_page"
	}
	if !oneOf(in.Source, sources...) {
		return invalid("unknown source")
	}
	in.SourceDetail = clean(in.SourceDetail)
	if len(in.SourceDetail) > 200 {
		return invalid("source detail is too long")
	}
	return nil
}

// ValidateMove checks a pipeline move against the configured stages.
func ValidateMove(stages []Stage, status, to string) error {
	if status != "active" {
		return invalid("this application is already %s", status)
	}
	for _, s := range stages {
		if s.ID == to {
			return nil
		}
	}
	return invalid("unknown stage %q", to)
}

// resumeOwner picks a user who owns the Drive copy of a resume.
func (r *Repository) resumeOwner(ctx context.Context, org string, f jobFacts) string {
	for _, id := range []string{f.Recruiter, f.Manager, f.CreatedBy} {
		if id != "" {
			return id
		}
	}
	var id string
	_ = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 ORDER BY created_at LIMIT 1`, org).Scan(&id)
	return id
}

// CreateApplication adds the candidate (deduplicated by email) and applies them
// to the job. byUser is the recruiter adding them (empty for the public form).
func (r *Repository) CreateApplication(ctx context.Context, org, byUser string, in ApplyInput) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	f, err := r.jobFacts(ctx, org, in.JobID)
	if err != nil {
		return "", err
	}
	if byUser == "" && f.Status != JobOpen {
		return "", ErrNotFound
	}
	st, err := r.GetSettings(ctx, org)
	if err != nil {
		return "", err
	}
	var resumeFile, resumeKey, resumeName, resumeMime, folder string
	if in.Resume != nil && len(in.Resume.Data) > 0 {
		if r.Deps.Resumes == nil {
			return "", fmt.Errorf("%w: resume storage", ErrUnavailable)
		}
		owner := r.resumeOwner(ctx, org, f)
		resumeFile, resumeKey, folder, err = r.Deps.Resumes.Save(ctx, org, owner, st.ResumeFolderID, in.Resume.Name, in.Resume.Mime, in.Resume.Data)
		if err != nil {
			return "", err
		}
		if folder != "" && folder != st.ResumeFolderID {
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.talent_settings SET resume_folder_id=$2 WHERE org_id=$1`, org, folder)
		}
		resumeName, resumeMime = in.Resume.Name, in.Resume.Mime
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var cand string
	err = tx.QueryRow(ctx, `INSERT INTO adrient.talent_candidates (org_id, name, email, phone, location, source, source_detail,
			resume_file_id, resume_key, resume_name, resume_mime, consent_at, consent_text)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, CASE WHEN $12 THEN now() END, CASE WHEN $12 THEN $13 ELSE '' END)
		ON CONFLICT (org_id, lower(email)) DO UPDATE SET
			phone = CASE WHEN adrient.talent_candidates.phone='' THEN EXCLUDED.phone ELSE adrient.talent_candidates.phone END,
			resume_file_id = COALESCE(EXCLUDED.resume_file_id, adrient.talent_candidates.resume_file_id),
			resume_key = CASE WHEN EXCLUDED.resume_key<>'' THEN EXCLUDED.resume_key ELSE adrient.talent_candidates.resume_key END,
			resume_name = CASE WHEN EXCLUDED.resume_name<>'' THEN EXCLUDED.resume_name ELSE adrient.talent_candidates.resume_name END,
			resume_mime = CASE WHEN EXCLUDED.resume_mime<>'' THEN EXCLUDED.resume_mime ELSE adrient.talent_candidates.resume_mime END,
			consent_at = COALESCE(adrient.talent_candidates.consent_at, EXCLUDED.consent_at),
			consent_text = CASE WHEN adrient.talent_candidates.consent_at IS NULL THEN EXCLUDED.consent_text ELSE adrient.talent_candidates.consent_text END
		RETURNING id::text`, org, in.Name, in.Email, in.Phone, clean(in.Location), in.Source, in.SourceDetail,
		nullUUID(resumeFile), resumeKey, resumeName, resumeMime, in.Consent, st.PrivacyText).Scan(&cand)
	if err != nil {
		return "", err
	}
	stage := st.Stages[0].ID
	var app string
	err = tx.QueryRow(ctx, `INSERT INTO adrient.talent_applications (org_id, job_id, candidate_id, stage, cover_note)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (job_id, candidate_id) DO NOTHING RETURNING id::text`,
		org, in.JobID, cand, stage, in.CoverNote).Scan(&app)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrDuplicate
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.talent_stage_history (application_id, org_id, from_stage, to_stage, by_user)
		VALUES ($1,$2,'',$3,$4)`, app, org, stage, nullUUID(byUser)); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return app, nil
}

type appFacts struct {
	ID, JobID, CandidateID, Stage, Status string
	Job                                   jobFacts
}

func (r *Repository) appFacts(ctx context.Context, org, id string) (appFacts, error) {
	var f appFacts
	err := r.pool.QueryRow(ctx, `SELECT id::text, job_id::text, candidate_id::text, stage, status
		FROM adrient.talent_applications WHERE org_id=$1 AND id=$2`, org, id).Scan(&f.ID, &f.JobID, &f.CandidateID, &f.Stage, &f.Status)
	if err != nil {
		return f, mapNoRows(err)
	}
	f.Job, err = r.jobFacts(ctx, org, f.JobID)
	return f, err
}

// accessApp loads an application the actor may see (ErrNotFound otherwise).
func (r *Repository) accessApp(ctx context.Context, a Actor, id string) (appFacts, error) {
	f, err := r.appFacts(ctx, a.OrgID, id)
	if err != nil {
		return f, err
	}
	if !f.Job.onTeam(a) {
		return f, ErrNotFound
	}
	return f, nil
}

// leadApp loads an application the actor may change (recruiter, hiring manager, HR).
func (r *Repository) leadApp(ctx context.Context, a Actor, id string) (appFacts, error) {
	f, err := r.accessApp(ctx, a, id)
	if err != nil {
		return f, err
	}
	if !f.Job.leads(a) {
		return f, ErrForbidden
	}
	return f, nil
}

const appCols = `ap.id::text AS id, ap.job_id::text AS job_id, ap.candidate_id::text AS candidate_id, ap.stage, ap.status,
	ap.reject_reason, ap.cover_note, ap.applied_at, ap.stage_entered_at, ap.closed_at,
	c.name AS candidate_name, c.email AS candidate_email, c.phone AS candidate_phone, c.source, c.source_detail,
	c.resume_name, (c.resume_key<>'') AS has_resume, c.talent_pool, j.title AS job_title`

const appFrom = ` FROM adrient.talent_applications ap JOIN adrient.talent_candidates c ON c.id=ap.candidate_id
	JOIN adrient.talent_jobs j ON j.id=ap.job_id `

const appVisible = ` ap.org_id=$1 AND ($3::boolean OR j.recruiter_id=$2::uuid OR j.hiring_manager_id=$2::uuid OR j.created_by=$2::uuid OR $2::uuid = ANY(j.team_ids)) `

// ListApplications returns the pipeline of one job, or every visible application when jobID is "".
func (r *Repository) ListApplications(ctx context.Context, a Actor, jobID string) (json.RawMessage, error) {
	q := `SELECT ` + appCols + `,
		(SELECT round(avg(s.overall) FILTER (WHERE s.overall>0),1)::float8 FROM adrient.talent_scorecards s WHERE s.application_id=ap.id) AS avg_score,
		(SELECT count(*) FROM adrient.talent_interviews i WHERE i.application_id=ap.id AND i.status='scheduled') AS interviews` + appFrom + `WHERE` + appVisible
	args := []any{a.OrgID, a.UserID, a.IsHR}
	if jobID != "" {
		q += ` AND ap.job_id=$4`
		args = append(args, jobID)
	}
	return r.rows(ctx, q+` ORDER BY ap.stage_entered_at DESC`, args...)
}

// GetApplication returns an application with history, scorecards, interviews and offers.
func (r *Repository) GetApplication(ctx context.Context, a Actor, id string) (json.RawMessage, error) {
	f, err := r.accessApp(ctx, a, id)
	if err != nil {
		return nil, err
	}
	base, err := r.row(ctx, `SELECT `+appCols+`, c.location AS candidate_location, c.consent_at, c.tags, c.notes AS candidate_notes`+appFrom+`WHERE ap.id=$1`, id)
	if err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	_ = json.Unmarshal(base, &out)
	out["history"], err = r.rows(ctx, `SELECT h.from_stage, h.to_stage, h.note, h.at, COALESCE(NULLIF(u.display_name,''),u.email,'') AS by_name
		FROM adrient.talent_stage_history h LEFT JOIN adrient.users u ON u.id=h.by_user WHERE h.application_id=$1 ORDER BY h.at`, id)
	if err != nil {
		return nil, err
	}
	out["interviews"], err = r.rows(ctx, `SELECT i.id::text AS id, i.title, i.starts_at, i.ends_at, i.status, i.meet_code, i.mail_sent,
		COALESCE(i.calendar_event_id::text,'') AS calendar_event_id, COALESCE(i.interviewer_ids::text[],'{}') AS interviewer_ids
		FROM adrient.talent_interviews i WHERE i.application_id=$1 ORDER BY i.starts_at`, id)
	if err != nil {
		return nil, err
	}
	out["scorecards"], err = r.scorecardsFor(ctx, a, f)
	if err != nil {
		return nil, err
	}
	if f.Job.leads(a) {
		out["offers"], err = r.rows(ctx, `SELECT id::text AS id, job_title, department, salary::float8 AS salary, currency, start_date, status,
			sent_at, responded_at, responded_name, expires_at, accept_token FROM adrient.talent_offers WHERE application_id=$1 ORDER BY created_at DESC`, id)
		if err != nil {
			return nil, err
		}
	} else {
		out["offers"] = json.RawMessage(`[]`)
	}
	b, _ := json.Marshal(out)
	return b, nil
}
