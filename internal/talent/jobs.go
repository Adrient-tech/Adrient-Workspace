package talent

import (
	"context"
	"encoding/json"
	"strings"
)

// Job statuses and approval states.
const (
	JobDraft  = "draft"
	JobOpen   = "open"
	JobClosed = "closed"

	ApprovalNone     = "none"
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
)

// JobInput is the editable part of a job opening.
type JobInput struct {
	Title           string   `json:"title"`
	Department      string   `json:"department"`
	Location        string   `json:"location"`
	EmploymentType  string   `json:"employment_type"`
	Skills          []string `json:"skills"`
	Description     string   `json:"description"`
	SalaryMin       *float64 `json:"salary_min"`
	SalaryMax       *float64 `json:"salary_max"`
	Currency        string   `json:"currency"`
	Openings        int      `json:"openings"`
	RecruiterID     string   `json:"recruiter_id"`
	HiringManagerID string   `json:"hiring_manager_id"`
	TeamIDs         []string `json:"team_ids"`
}

var employmentTypes = []string{"full_time", "part_time", "contract", "intern", "temporary"}

// Validate normalises and checks a job.
func (j *JobInput) Validate() error {
	j.Title = clean(j.Title)
	if j.Title == "" || len(j.Title) > 160 {
		return invalid("title is required (max 160 characters)")
	}
	if j.EmploymentType == "" {
		j.EmploymentType = "full_time"
	}
	if !oneOf(j.EmploymentType, employmentTypes...) {
		return invalid("unknown employment type")
	}
	if j.Openings == 0 {
		j.Openings = 1
	}
	if j.Openings < 1 || j.Openings > 500 {
		return invalid("openings must be between 1 and 500")
	}
	if j.SalaryMin != nil && *j.SalaryMin < 0 || j.SalaryMax != nil && *j.SalaryMax < 0 {
		return invalid("salary cannot be negative")
	}
	if j.SalaryMin != nil && j.SalaryMax != nil && *j.SalaryMin > *j.SalaryMax {
		return invalid("salary minimum is above the maximum")
	}
	if j.Currency == "" {
		j.Currency = "INR"
	}
	sk := []string{}
	for _, s := range j.Skills {
		if s = clean(s); s != "" {
			sk = append(sk, s)
		}
	}
	j.Skills = sk
	j.TeamIDs = uuidList(j.TeamIDs)
	return nil
}

// CheckJobTransition applies the job status rules. HR may open any draft;
// anyone else needs an approved requisition.
func CheckJobTransition(isHR bool, approval, from, to string) error {
	switch {
	case from == to:
		return nil
	case to == JobOpen:
		if from == JobOpen {
			return nil
		}
		if !isHR && approval != ApprovalApproved {
			return invalid("this opening needs HR approval before it can be published")
		}
		if approval == ApprovalPending {
			return invalid("approval is still pending")
		}
		if approval == ApprovalRejected {
			return invalid("approval was rejected; edit and request approval again")
		}
	case to == JobClosed:
		if from == JobDraft {
			return invalid("a draft opening cannot be closed; delete it instead")
		}
	case to == JobDraft:
		if from == JobClosed {
			return invalid("a closed opening cannot go back to draft; reopen it")
		}
	default:
		return invalid("unknown status %q", to)
	}
	return nil
}

// jobVisible is the SQL predicate for jobs the actor may see ($1 org, $2 user, $3 hr).
const jobVisible = `(j.org_id=$1 AND ($3::boolean OR j.recruiter_id=$2::uuid OR j.hiring_manager_id=$2::uuid OR j.created_by=$2::uuid OR $2::uuid = ANY(j.team_ids)))`

const jobCols = `j.id::text AS id, j.title, j.department, j.location, j.employment_type, j.skills, j.description,
	j.salary_min::float8 AS salary_min, j.salary_max::float8 AS salary_max, j.currency, j.openings, j.status,
	j.approval_status, COALESCE(j.approver_id::text,'') AS approver_id, COALESCE(j.recruiter_id::text,'') AS recruiter_id,
	COALESCE(j.hiring_manager_id::text,'') AS hiring_manager_id, COALESCE(j.team_ids::text[], '{}') AS team_ids,
	COALESCE(j.created_by::text,'') AS created_by, j.created_at, j.published_at, j.closed_at`

// ListJobs returns the jobs the actor may see, with applicant counts.
func (r *Repository) ListJobs(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT `+jobCols+`,
		(SELECT count(*) FROM adrient.talent_applications x WHERE x.job_id=j.id AND x.status='active') AS active_applicants,
		(SELECT count(*) FROM adrient.talent_applications x WHERE x.job_id=j.id) AS total_applicants,
		(SELECT count(*) FROM adrient.talent_applications x WHERE x.job_id=j.id AND x.status='hired') AS hired
		FROM adrient.talent_jobs j WHERE `+jobVisible+` ORDER BY j.created_at DESC`, a.OrgID, a.UserID, a.IsHR)
}

// GetJob returns one visible job. Jobs the actor cannot see look like missing ones.
func (r *Repository) GetJob(ctx context.Context, a Actor, id string) (json.RawMessage, error) {
	return r.row(ctx, `SELECT `+jobCols+` FROM adrient.talent_jobs j WHERE j.id=$4 AND `+jobVisible, a.OrgID, a.UserID, a.IsHR, id)
}

// jobAccess loads minimal job facts and reports whether the actor is on the hiring team.
type jobFacts struct {
	ID, Status, Approval, Title, Department, Currency string
	Recruiter, Manager, CreatedBy, Owner              string
	Team                                              []string
	Openings                                          int
}

func (f jobFacts) onTeam(a Actor) bool {
	if a.IsHR {
		return true
	}
	return a.UserID != "" && (a.UserID == f.Recruiter || a.UserID == f.Manager || a.UserID == f.CreatedBy || contains(f.Team, a.UserID))
}

// leads reports whether the actor runs the opening (recruiter, hiring manager, creator or HR).
func (f jobFacts) leads(a Actor) bool {
	return a.IsHR || a.UserID != "" && (a.UserID == f.Recruiter || a.UserID == f.Manager || a.UserID == f.CreatedBy)
}

func (r *Repository) jobFacts(ctx context.Context, org, id string) (jobFacts, error) {
	var f jobFacts
	err := r.pool.QueryRow(ctx, `SELECT id::text, status, approval_status, title, department, currency,
		COALESCE(recruiter_id::text,''), COALESCE(hiring_manager_id::text,''), COALESCE(created_by::text,''),
		COALESCE(team_ids::text[],'{}'), openings FROM adrient.talent_jobs WHERE org_id=$1 AND id=$2`, org, id).
		Scan(&f.ID, &f.Status, &f.Approval, &f.Title, &f.Department, &f.Currency, &f.Recruiter, &f.Manager, &f.CreatedBy, &f.Team, &f.Openings)
	if err != nil {
		return f, mapNoRows(err)
	}
	return f, nil
}

// SaveJob creates (id=="") or updates a job. Updating needs lead access.
func (r *Repository) SaveJob(ctx context.Context, a Actor, id string, in JobInput) (json.RawMessage, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if id == "" {
		var nid string
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_jobs (org_id, title, department, location, employment_type, skills,
			description, salary_min, salary_max, currency, openings, recruiter_id, hiring_manager_id, team_ids, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::uuid[],$15) RETURNING id::text`,
			a.OrgID, in.Title, clean(in.Department), clean(in.Location), in.EmploymentType, in.Skills, in.Description,
			in.SalaryMin, in.SalaryMax, in.Currency, in.Openings, nullUUID(in.RecruiterID), nullUUID(in.HiringManagerID), in.TeamIDs, a.UserID).Scan(&nid)
		if err != nil {
			return nil, err
		}
		r.audit(ctx, a, "job.create", "talent_job", nid)
		return r.GetJob(ctx, a, nid)
	}
	f, err := r.jobFacts(ctx, a.OrgID, id)
	if err != nil {
		return nil, err
	}
	if !f.leads(a) {
		return nil, notFoundOrForbidden(f.onTeam(a))
	}
	// An edit of an approved job by a non-HR user needs a fresh approval.
	approval := f.Approval
	if !a.IsHR && approval == ApprovalApproved && f.Status != JobOpen {
		approval = ApprovalNone
	}
	_, err = r.pool.Exec(ctx, `UPDATE adrient.talent_jobs SET title=$3, department=$4, location=$5, employment_type=$6, skills=$7,
		description=$8, salary_min=$9, salary_max=$10, currency=$11, openings=$12, recruiter_id=$13, hiring_manager_id=$14,
		team_ids=$15::uuid[], approval_status=$16 WHERE org_id=$1 AND id=$2`,
		a.OrgID, id, in.Title, clean(in.Department), clean(in.Location), in.EmploymentType, in.Skills, in.Description,
		in.SalaryMin, in.SalaryMax, in.Currency, in.Openings, nullUUID(in.RecruiterID), nullUUID(in.HiringManagerID), in.TeamIDs, approval)
	if err != nil {
		return nil, err
	}
	r.audit(ctx, a, "job.update", "talent_job", id)
	return r.GetJob(ctx, a, id)
}

// RequestApproval asks HR to approve a draft opening.
func (r *Repository) RequestApproval(ctx context.Context, a Actor, id string) (json.RawMessage, error) {
	f, err := r.jobFacts(ctx, a.OrgID, id)
	if err != nil {
		return nil, err
	}
	if !f.leads(a) {
		return nil, notFoundOrForbidden(f.onTeam(a))
	}
	if f.Status != JobDraft {
		return nil, invalid("only a draft can be sent for approval")
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.talent_jobs SET approval_status='pending' WHERE org_id=$1 AND id=$2`, a.OrgID, id); err != nil {
		return nil, err
	}
	r.audit(ctx, a, "job.approval_requested", "talent_job", id)
	return r.GetJob(ctx, a, id)
}

// DecideApproval lets HR approve or reject a pending opening.
func (r *Repository) DecideApproval(ctx context.Context, a Actor, id string, approve bool) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	f, err := r.jobFacts(ctx, a.OrgID, id)
	if err != nil {
		return nil, err
	}
	if f.Approval != ApprovalPending {
		return nil, invalid("this opening is not waiting for approval")
	}
	st := ApprovalRejected
	if approve {
		st = ApprovalApproved
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.talent_jobs SET approval_status=$3, approver_id=$4 WHERE org_id=$1 AND id=$2`, a.OrgID, id, st, a.UserID); err != nil {
		return nil, err
	}
	r.audit(ctx, a, "job.approval_"+st, "talent_job", id)
	return r.GetJob(ctx, a, id)
}

// SetJobStatus opens, closes or reopens an opening.
func (r *Repository) SetJobStatus(ctx context.Context, a Actor, id, to string) (json.RawMessage, error) {
	f, err := r.jobFacts(ctx, a.OrgID, id)
	if err != nil {
		return nil, err
	}
	if !f.leads(a) {
		return nil, notFoundOrForbidden(f.onTeam(a))
	}
	if err := CheckJobTransition(a.IsHR, f.Approval, f.Status, to); err != nil {
		return nil, err
	}
	_, err = r.pool.Exec(ctx, `UPDATE adrient.talent_jobs SET status=$3,
		published_at = CASE WHEN $3='open' AND published_at IS NULL THEN now() ELSE published_at END,
		closed_at = CASE WHEN $3='closed' THEN now() ELSE NULL END
		WHERE org_id=$1 AND id=$2`, a.OrgID, id, to)
	if err != nil {
		return nil, err
	}
	r.audit(ctx, a, "job.status_"+to, "talent_job", id)
	return r.GetJob(ctx, a, id)
}

// DeleteJob removes a draft opening (anything with applicants must be closed instead).
func (r *Repository) DeleteJob(ctx context.Context, a Actor, id string) error {
	f, err := r.jobFacts(ctx, a.OrgID, id)
	if err != nil {
		return err
	}
	if !f.leads(a) {
		return notFoundOrForbidden(f.onTeam(a))
	}
	var n int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.talent_applications WHERE job_id=$1`, id).Scan(&n)
	if n > 0 {
		return invalid("this opening has applicants; close it instead")
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.talent_jobs WHERE org_id=$1 AND id=$2`, a.OrgID, id)
	r.audit(ctx, a, "job.delete", "talent_job", id)
	return err
}

func notFoundOrForbidden(onTeam bool) error {
	if onTeam {
		return ErrForbidden
	}
	return ErrNotFound
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }
