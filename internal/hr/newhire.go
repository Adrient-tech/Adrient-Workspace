package hr

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// NewHireInput is what another app (Talent) hands over to create an employee.
type NewHireInput struct {
	Name         string
	Email        string
	Phone        string
	JobTitle     string
	Department   string
	StartDate    string // YYYY-MM-DD
	ManagerEmpID string // optional hr_employees id
	ManagerUser  string // optional account id; resolved to that user's employee record
	SourceRef    string // idempotency key, e.g. "talent:<application id>"
}

// CreateFromNewHire creates the employee for a hired candidate (HR only).
// It is idempotent: the same SourceRef, or an existing employee with the same
// work email, returns that employee with created=false. The work email links
// the account when a member has that address. makeContact runs only on create
// and returns the Contacts id to link (it may be nil or return "").
func (r *Repository) CreateFromNewHire(ctx context.Context, a Actor, in NewHireInput, makeContact func() (string, error)) (Employee, bool, error) {
	if err := a.requireHR(); err != nil {
		return Employee{}, false, err
	}
	in.Name, in.Email = strings.TrimSpace(in.Name), strings.ToLower(strings.TrimSpace(in.Email))
	if in.Name == "" {
		return Employee{}, false, invalid("the new hire needs a name")
	}
	var existing string
	err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.hr_employees WHERE org_id=$1 AND
		((source_ref<>'' AND source_ref=$2) OR ($3<>'' AND lower(work_email)=$3)) ORDER BY created_at LIMIT 1`, a.OrgID, in.SourceRef, in.Email).Scan(&existing)
	if err == nil {
		e, gerr := r.GetEmployee(ctx, a, existing)
		return e, false, gerr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, false, err
	}
	first, last := in.Name, ""
	if i := strings.LastIndex(in.Name, " "); i > 0 {
		first, last = strings.TrimSpace(in.Name[:i]), strings.TrimSpace(in.Name[i+1:])
	}
	e := Employee{FirstName: first, LastName: last, WorkEmail: in.Email, Phone: in.Phone, JoinDate: in.StartDate, ManagerID: in.ManagerEmpID}
	if e.JoinDate == "" {
		e.JoinDate = fmtDate(r.now())
	}
	if e.ManagerID == "" && in.ManagerUser != "" {
		_ = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.hr_employees WHERE org_id=$1 AND user_id=$2::uuid AND status<>'exited'`, a.OrgID, in.ManagerUser).Scan(&e.ManagerID)
	}
	if in.Department != "" {
		if e.DepartmentID, err = r.lookupOrCreate(ctx, a.OrgID, "departments", in.Department); err != nil {
			return Employee{}, false, err
		}
	}
	if in.JobTitle != "" {
		if e.DesignationID, err = r.lookupOrCreate(ctx, a.OrgID, "hr_designations", in.JobTitle); err != nil {
			return Employee{}, false, err
		}
	}
	if in.Email != "" {
		_ = r.pool.QueryRow(ctx, `SELECT u.id::text FROM adrient.users u WHERE u.org_id=$1 AND lower(u.email)=$2
			AND NOT EXISTS (SELECT 1 FROM adrient.hr_employees x WHERE x.org_id=$1 AND x.user_id=u.id) LIMIT 1`, a.OrgID, in.Email).Scan(&e.UserID)
	}
	if makeContact != nil {
		if cid, cerr := makeContact(); cerr == nil {
			e.ContactID = cid
		}
	}
	out, err := r.SaveEmployee(ctx, a, "", e)
	if err != nil {
		return Employee{}, false, err
	}
	if in.SourceRef != "" {
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.hr_employees SET source_ref=$3 WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, out.ID, in.SourceRef)
	}
	return out, true, nil
}
