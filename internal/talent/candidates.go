package talent

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/jackc/pgx/v5"
)

// MoveApplication moves an active application to another open stage.
func (r *Repository) MoveApplication(ctx context.Context, a Actor, id, stage, note string) (json.RawMessage, error) {
	f, err := r.leadApp(ctx, a, id)
	if err != nil {
		return nil, err
	}
	st, err := r.GetSettings(ctx, a.OrgID)
	if err != nil {
		return nil, err
	}
	if err := ValidateMove(st.Stages, f.Status, stage); err != nil {
		return nil, err
	}
	if stage != f.Stage {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		if _, err := tx.Exec(ctx, `UPDATE adrient.talent_applications SET stage=$3, stage_entered_at=now() WHERE org_id=$1 AND id=$2`, a.OrgID, id, stage); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.talent_stage_history (application_id, org_id, from_stage, to_stage, note, by_user) VALUES ($1,$2,$3,$4,$5,$6)`,
			id, a.OrgID, f.Stage, stage, note, a.UserID); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		r.audit(ctx, a, "application.move", "talent_application", id)
	}
	return r.GetApplication(ctx, a, id)
}

// RejectApplication closes an application as rejected. A reason is required.
func (r *Repository) RejectApplication(ctx context.Context, a Actor, id, reason string) (json.RawMessage, error) {
	f, err := r.leadApp(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if f.Status != "active" {
		return nil, invalid("this application is already %s", f.Status)
	}
	if clean(reason) == "" {
		return nil, invalid("give a reason for rejecting")
	}
	if err := r.closeApp(ctx, a, f, "rejected", StageRejected, clean(reason)); err != nil {
		return nil, err
	}
	return r.GetApplication(ctx, a, id)
}

func (r *Repository) closeApp(ctx context.Context, a Actor, f appFacts, status, stage, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE adrient.talent_applications SET status=$3, stage=$4, reject_reason=$5, closed_at=now(), stage_entered_at=now()
		WHERE org_id=$1 AND id=$2`, a.OrgID, f.ID, status, stage, reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.talent_stage_history (application_id, org_id, from_stage, to_stage, note, by_user) VALUES ($1,$2,$3,$4,$5,$6)`,
		f.ID, a.OrgID, f.Stage, stage, reason, a.UserID); err != nil {
		return err
	}
	if status == "hired" {
		// Close the opening once every position is filled.
		if _, err := tx.Exec(ctx, `UPDATE adrient.talent_jobs j SET status='closed', closed_at=now()
			WHERE j.id=$1 AND j.status='open' AND (SELECT count(*) FROM adrient.talent_applications x WHERE x.job_id=j.id AND x.status='hired') >= j.openings`, f.JobID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	r.audit(ctx, a, "application."+status, "talent_application", f.ID)
	return nil
}

// HireApplication marks an application hired.
func (r *Repository) HireApplication(ctx context.Context, a Actor, id string) (json.RawMessage, error) {
	f, err := r.leadApp(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if f.Status != "active" {
		return nil, invalid("this application is already %s", f.Status)
	}
	if err := r.closeApp(ctx, a, f, "hired", StageHired, ""); err != nil {
		return nil, err
	}
	return r.GetApplication(ctx, a, id)
}

// RestoreApplication reopens a rejected or withdrawn application at the first stage.
func (r *Repository) RestoreApplication(ctx context.Context, a Actor, id string) (json.RawMessage, error) {
	f, err := r.leadApp(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if f.Status != "rejected" && f.Status != "withdrawn" {
		return nil, invalid("only rejected or withdrawn applications can be restored")
	}
	st, err := r.GetSettings(ctx, a.OrgID)
	if err != nil {
		return nil, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.talent_applications SET status='active', stage=$3, reject_reason='', closed_at=NULL, stage_entered_at=now() WHERE org_id=$1 AND id=$2`,
		a.OrgID, id, st.Stages[0].ID); err != nil {
		return nil, err
	}
	_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.talent_stage_history (application_id, org_id, from_stage, to_stage, note, by_user) VALUES ($1,$2,$3,$4,'restored',$5)`,
		id, a.OrgID, f.Stage, st.Stages[0].ID, a.UserID)
	return r.GetApplication(ctx, a, id)
}

// AddCandidate lets the hiring team add a candidate by hand (referral, agency...).
func (r *Repository) AddCandidate(ctx context.Context, a Actor, in ApplyInput) (json.RawMessage, error) {
	f, err := r.jobFacts(ctx, a.OrgID, in.JobID)
	if err != nil {
		return nil, err
	}
	if !f.leads(a) {
		return nil, notFoundOrForbidden(f.onTeam(a))
	}
	if in.Source == "career_page" {
		in.Source = "other"
	}
	in.Consent = false
	id, err := r.CreateApplication(ctx, a.OrgID, a.UserID, in)
	if err != nil {
		return nil, err
	}
	r.audit(ctx, a, "application.create", "talent_application", id)
	return r.GetApplication(ctx, a, id)
}

// DuplicateMatch is an existing candidate with the same email.
type DuplicateMatch struct {
	CandidateID  string `json:"candidate_id"`
	Name         string `json:"name"`
	Applications int    `json:"applications"`
}

// CheckDuplicate reports whether the email is already a candidate.
func (r *Repository) CheckDuplicate(ctx context.Context, a Actor, email string) (*DuplicateMatch, error) {
	var d DuplicateMatch
	err := r.pool.QueryRow(ctx, `SELECT c.id::text, c.name, (SELECT count(*) FROM adrient.talent_applications x WHERE x.candidate_id=c.id)
		FROM adrient.talent_candidates c WHERE c.org_id=$1 AND lower(c.email)=lower($2)`, a.OrgID, clean(email)).Scan(&d.CandidateID, &d.Name, &d.Applications)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &d, err
}

// SetTalentPool adds or removes a candidate from the talent pool (HR, or a lead of one of their jobs).
func (r *Repository) SetTalentPool(ctx context.Context, a Actor, candidateID string, in bool, tags []string) error {
	if !a.IsHR {
		var ok bool
		_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.talent_applications ap JOIN adrient.talent_jobs j ON j.id=ap.job_id
			WHERE ap.org_id=$1 AND ap.candidate_id=$2 AND (j.recruiter_id=$3::uuid OR j.hiring_manager_id=$3::uuid OR j.created_by=$3::uuid))`,
			a.OrgID, candidateID, a.UserID).Scan(&ok)
		if !ok {
			return ErrNotFound
		}
	}
	ct, err := r.pool.Exec(ctx, `UPDATE adrient.talent_candidates SET talent_pool=$3, tags=CASE WHEN $4::text[] IS NULL THEN tags ELSE $4 END WHERE org_id=$1 AND id=$2`,
		a.OrgID, candidateID, in, tags)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	r.audit(ctx, a, "candidate.pool", "talent_candidate", candidateID)
	return nil
}

// ListPool returns the talent pool (HR only).
func (r *Repository) ListPool(ctx context.Context, a Actor) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	return r.rows(ctx, `SELECT c.id::text AS id, c.name, c.email, c.phone, c.source, c.tags, c.resume_name, (c.resume_key<>'') AS has_resume, c.created_at,
		(SELECT count(*) FROM adrient.talent_applications x WHERE x.candidate_id=c.id) AS applications
		FROM adrient.talent_candidates c WHERE c.org_id=$1 AND c.talent_pool ORDER BY c.created_at DESC`, a.OrgID)
}

// PoolToJob applies a talent-pool candidate to an opening (HR only).
func (r *Repository) PoolToJob(ctx context.Context, a Actor, candidateID, jobID string) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	if _, err := r.jobFacts(ctx, a.OrgID, jobID); err != nil {
		return nil, err
	}
	var name, email, phone string
	if err := r.pool.QueryRow(ctx, `SELECT name,email,phone FROM adrient.talent_candidates WHERE org_id=$1 AND id=$2`, a.OrgID, candidateID).Scan(&name, &email, &phone); err != nil {
		return nil, mapNoRows(err)
	}
	id, err := r.CreateApplication(ctx, a.OrgID, a.UserID, ApplyInput{JobID: jobID, Name: name, Email: email, Phone: phone, Source: "talent_pool"})
	if err != nil {
		return nil, err
	}
	return r.GetApplication(ctx, a, id)
}

// OpenResume returns the resume stream of a candidate the actor may see.
func (r *Repository) OpenResume(ctx context.Context, a Actor, candidateID string) (string, io.ReadCloser, string, error) {
	var key, name, mime string
	err := r.pool.QueryRow(ctx, `SELECT c.resume_key, c.resume_name, c.resume_mime FROM adrient.talent_candidates c
		WHERE c.org_id=$1 AND c.id=$2 AND ($3::boolean OR EXISTS(SELECT 1 FROM adrient.talent_applications ap JOIN adrient.talent_jobs j ON j.id=ap.job_id
			WHERE ap.candidate_id=c.id AND (j.recruiter_id=$4::uuid OR j.hiring_manager_id=$4::uuid OR j.created_by=$4::uuid OR $4::uuid = ANY(j.team_ids))))`,
		a.OrgID, candidateID, a.IsHR, a.UserID).Scan(&key, &name, &mime)
	if err != nil {
		return "", nil, "", mapNoRows(err)
	}
	if key == "" {
		return "", nil, "", ErrNotFound
	}
	if r.Deps.Resumes == nil {
		return "", nil, "", ErrUnavailable
	}
	body, m, _, err := r.Deps.Resumes.Open(ctx, key)
	if err != nil {
		return "", nil, "", err
	}
	if mime == "" {
		mime = m
	}
	return name, body, mime, nil
}

// DeleteCandidate erases a candidate and their applications (GDPR-style erasure). HR only.
func (r *Repository) DeleteCandidate(ctx context.Context, a Actor, id string) error {
	if !a.IsHR {
		return ErrForbidden
	}
	ct, err := r.pool.Exec(ctx, `DELETE FROM adrient.talent_candidates WHERE org_id=$1 AND id=$2`, a.OrgID, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	r.audit(ctx, a, "candidate.erase", "talent_candidate", id)
	return nil
}
