// Package people is the org directory: user attributes (title, department,
// manager, phone, location), the department tree, manager-chain and
// department-subtree queries, and the HTTP API behind the shared PersonCard,
// the org chart and the admin People section.
package people

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound means the person or department does not exist in the org.
	ErrNotFound = errors.New("not found")
	// ErrCycle means the change would make a manager or department loop.
	ErrCycle = errors.New("change would create a cycle")
	// ErrInvalid wraps validation failures (bad reference, empty name).
	ErrInvalid = errors.New("invalid")
	// ErrDuplicate means a sibling department already has that name.
	ErrDuplicate = errors.New("a department with this name already exists here")
)

// maxDepth bounds recursive walks so corrupt data can never loop forever.
const maxDepth = 64

// Person is one org member with directory attributes.
type Person struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	JobTitle       string `json:"job_title"`
	Phone          string `json:"phone"`
	Location       string `json:"location"`
	EmployeeID     string `json:"employee_id"`
	StartDate      string `json:"start_date"`
	DepartmentID   string `json:"department_id"`
	DepartmentName string `json:"department_name"`
	ManagerID      string `json:"manager_id"`
	ManagerName    string `json:"manager_name"`
}

// Department is one node of the org's department tree.
type Department struct {
	ID          string `json:"id"`
	ParentID    string `json:"parent_id"`
	Name        string `json:"name"`
	HeadUserID  string `json:"head_user_id"`
	HeadName    string `json:"head_name"`
	MemberCount int    `json:"member_count"`
}

// Patch is a partial update of a user's directory attributes. Nil leaves a
// field unchanged; an empty string clears it.
type Patch struct {
	JobTitle     *string `json:"job_title"`
	Phone        *string `json:"phone"`
	Location     *string `json:"location"`
	EmployeeID   *string `json:"employee_id"`
	StartDate    *string `json:"start_date"`
	DepartmentID *string `json:"department_id"`
	ManagerID    *string `json:"manager_id"`
}

// Store reads and writes the directory.
type Store struct{ pool *pgxpool.Pool }

// NewStore constructs a Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const personSelect = `
SELECT u.id::text, COALESCE(NULLIF(u.display_name,''), u.email), u.email, u.job_title, u.phone, u.location,
       u.employee_id, u.start_date, COALESCE(u.department_id::text,''), COALESCE(d.name,''),
       COALESCE(u.manager_id::text,''), COALESCE(NULLIF(m.display_name,''), m.email, '')
FROM adrient.users u
LEFT JOIN adrient.departments d ON d.id = u.department_id
LEFT JOIN adrient.users m ON m.id = u.manager_id`

func scanPerson(row pgx.Row) (Person, error) {
	var p Person
	var sd *time.Time
	if err := row.Scan(&p.ID, &p.Name, &p.Email, &p.JobTitle, &p.Phone, &p.Location, &p.EmployeeID,
		&sd, &p.DepartmentID, &p.DepartmentName, &p.ManagerID, &p.ManagerName); err != nil {
		return Person{}, err
	}
	if sd != nil {
		p.StartDate = sd.Format("2006-01-02")
	}
	return p, nil
}

func (s *Store) people(ctx context.Context, where string, args ...any) ([]Person, error) {
	rows, err := s.pool.Query(ctx, personSelect+" WHERE "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns one person of the org.
func (s *Store) Get(ctx context.Context, orgID, id string) (Person, error) {
	if !isUUID(id) {
		return Person{}, ErrNotFound
	}
	p, err := scanPerson(s.pool.QueryRow(ctx, personSelect+" WHERE u.org_id=$1 AND u.id=$2", orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Person{}, ErrNotFound
	}
	return p, err
}

// Search finds people by name, email or title (empty query lists everyone).
func (s *Store) Search(ctx context.Context, orgID, query string, limit int) ([]Person, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := "%" + escapeLike(strings.TrimSpace(query)) + "%"
	return s.people(ctx, `u.org_id=$1 AND (u.display_name ILIKE $2 OR u.email ILIKE $2 OR u.job_title ILIKE $2)
		ORDER BY lower(COALESCE(NULLIF(u.display_name,''), u.email)) LIMIT `+fmt.Sprint(limit), orgID, q)
}

// All returns every person of the org (org chart input).
func (s *Store) All(ctx context.Context, orgID string) ([]Person, error) {
	return s.people(ctx, `u.org_id=$1 ORDER BY lower(COALESCE(NULLIF(u.display_name,''), u.email))`, orgID)
}

// DirectReports lists the people whose manager is id.
func (s *Store) DirectReports(ctx context.Context, orgID, id string) ([]Person, error) {
	if !isUUID(id) {
		return []Person{}, nil
	}
	return s.people(ctx, `u.org_id=$1 AND u.manager_id=$2 ORDER BY lower(COALESCE(NULLIF(u.display_name,''), u.email))`, orgID, id)
}

// ManagerChain returns id's managers, nearest first (manager, manager's
// manager, ...). It stops at a repeated user, so corrupt cyclic data cannot loop.
func (s *Store) ManagerChain(ctx context.Context, orgID, id string) ([]Person, error) {
	if !isUUID(id) {
		return []Person{}, nil
	}
	rows, err := s.pool.Query(ctx, `
WITH RECURSIVE chain(id, manager_id, depth, path) AS (
    SELECT u.id, u.manager_id, 0, ARRAY[u.id]
    FROM adrient.users u WHERE u.org_id=$1 AND u.id=$2
  UNION ALL
    SELECT m.id, m.manager_id, c.depth+1, c.path || m.id
    FROM chain c JOIN adrient.users m ON m.id = c.manager_id AND m.org_id=$1
    WHERE NOT m.id = ANY(c.path) AND c.depth < $3
)
SELECT u.id::text, COALESCE(NULLIF(u.display_name,''), u.email), u.email, u.job_title, u.phone, u.location,
       u.employee_id, u.start_date, COALESCE(u.department_id::text,''), COALESCE(d.name,''),
       COALESCE(u.manager_id::text,''), COALESCE(NULLIF(m.display_name,''), m.email, '')
FROM chain c
JOIN adrient.users u ON u.id = c.id
LEFT JOIN adrient.departments d ON d.id = u.department_id
LEFT JOIN adrient.users m ON m.id = u.manager_id
WHERE c.depth > 0 ORDER BY c.depth`, orgID, id, maxDepth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DepartmentSubtreeIDs returns deptID and all of its descendants.
func (s *Store) DepartmentSubtreeIDs(ctx context.Context, orgID, deptID string) ([]string, error) {
	if !isUUID(deptID) {
		return []string{}, nil
	}
	rows, err := s.pool.Query(ctx, `
WITH RECURSIVE sub(id, depth, path) AS (
    SELECT d.id, 0, ARRAY[d.id] FROM adrient.departments d WHERE d.org_id=$1 AND d.id=$2
  UNION ALL
    SELECT c.id, s.depth+1, s.path || c.id
    FROM sub s JOIN adrient.departments c ON c.parent_id = s.id AND c.org_id=$1
    WHERE NOT c.id = ANY(s.path) AND s.depth < $3
)
SELECT id::text FROM sub ORDER BY depth, id`, orgID, deptID, maxDepth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// PeopleInDepartmentSubtree lists everyone in deptID or any department below it.
func (s *Store) PeopleInDepartmentSubtree(ctx context.Context, orgID, deptID string) ([]Person, error) {
	ids, err := s.DepartmentSubtreeIDs(ctx, orgID, deptID)
	if err != nil || len(ids) == 0 {
		return []Person{}, err
	}
	return s.people(ctx, `u.org_id=$1 AND u.department_id = ANY($2::uuid[]) ORDER BY lower(COALESCE(NULLIF(u.display_name,''), u.email))`, orgID, ids)
}

// Departments lists the org's departments with head names and member counts.
func (s *Store) Departments(ctx context.Context, orgID string) ([]Department, error) {
	rows, err := s.pool.Query(ctx, `
SELECT d.id::text, COALESCE(d.parent_id::text,''), d.name, COALESCE(d.head_user_id::text,''),
       COALESCE(NULLIF(h.display_name,''), h.email, ''),
       (SELECT count(*) FROM adrient.users u WHERE u.department_id = d.id)::int
FROM adrient.departments d LEFT JOIN adrient.users h ON h.id = d.head_user_id
WHERE d.org_id=$1 ORDER BY lower(d.name)`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Department{}
	for rows.Next() {
		var d Department
		if err := rows.Scan(&d.ID, &d.ParentID, &d.Name, &d.HeadUserID, &d.HeadName, &d.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDepartment returns one department.
func (s *Store) GetDepartment(ctx context.Context, orgID, id string) (Department, error) {
	ds, err := s.Departments(ctx, orgID)
	if err != nil {
		return Department{}, err
	}
	for _, d := range ds {
		if d.ID == id {
			return d, nil
		}
	}
	return Department{}, ErrNotFound
}

func (s *Store) checkUser(ctx context.Context, orgID, id string) error {
	if !isUUID(id) {
		return fmt.Errorf("%w: unknown user", ErrInvalid)
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE org_id=$1 AND id=$2)`, orgID, id).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: unknown user", ErrInvalid)
	}
	return nil
}

func (s *Store) checkDept(ctx context.Context, orgID, id string) error {
	if !isUUID(id) {
		return fmt.Errorf("%w: unknown department", ErrInvalid)
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.departments WHERE org_id=$1 AND id=$2)`, orgID, id).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: unknown department", ErrInvalid)
	}
	return nil
}

// UpdateUser applies p to user id. Callers enforce who may set which field.
// A manager change that would create a loop returns ErrCycle.
func (s *Store) UpdateUser(ctx context.Context, orgID, id string, p Patch) (Person, error) {
	if _, err := s.Get(ctx, orgID, id); err != nil {
		return Person{}, err
	}
	if p.ManagerID != nil && *p.ManagerID != "" {
		if err := s.checkUser(ctx, orgID, *p.ManagerID); err != nil {
			return Person{}, err
		}
		if *p.ManagerID == id {
			return Person{}, ErrCycle
		}
		chain, err := s.ManagerChain(ctx, orgID, *p.ManagerID)
		if err != nil {
			return Person{}, err
		}
		for _, m := range chain {
			if m.ID == id {
				return Person{}, ErrCycle
			}
		}
	}
	if p.DepartmentID != nil && *p.DepartmentID != "" {
		if err := s.checkDept(ctx, orgID, *p.DepartmentID); err != nil {
			return Person{}, err
		}
	}
	if p.StartDate != nil && *p.StartDate != "" {
		if _, err := time.Parse("2006-01-02", *p.StartDate); err != nil {
			return Person{}, fmt.Errorf("%w: start_date must be YYYY-MM-DD", ErrInvalid)
		}
	}
	str := func(v *string) any {
		if v == nil {
			return nil
		}
		return strings.TrimSpace(*v)
	}
	// nullable: $n_set flag distinguishes "leave" from "clear".
	nul := func(v *string) (bool, any) {
		if v == nil {
			return false, nil
		}
		if strings.TrimSpace(*v) == "" {
			return true, nil
		}
		return true, strings.TrimSpace(*v)
	}
	dSet, dVal := nul(p.DepartmentID)
	mSet, mVal := nul(p.ManagerID)
	sSet, sVal := nul(p.StartDate)
	_, err := s.pool.Exec(ctx, `
UPDATE adrient.users SET
  job_title   = COALESCE($3, job_title),
  phone       = COALESCE($4, phone),
  location    = COALESCE($5, location),
  employee_id = COALESCE($6, employee_id),
  department_id = CASE WHEN $7 THEN $8::uuid ELSE department_id END,
  manager_id    = CASE WHEN $9 THEN $10::uuid ELSE manager_id END,
  start_date    = CASE WHEN $11 THEN $12::date ELSE start_date END,
  updated_at = now()
WHERE org_id=$1 AND id=$2`,
		orgID, id, str(p.JobTitle), str(p.Phone), str(p.Location), str(p.EmployeeID),
		dSet, dVal, mSet, mVal, sSet, sVal)
	if err != nil {
		return Person{}, err
	}
	return s.Get(ctx, orgID, id)
}

// CreateDepartment adds a department under parentID ("" for a root).
func (s *Store) CreateDepartment(ctx context.Context, orgID, name, parentID, headUserID string) (Department, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Department{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if parentID != "" {
		if err := s.checkDept(ctx, orgID, parentID); err != nil {
			return Department{}, err
		}
	}
	if headUserID != "" {
		if err := s.checkUser(ctx, orgID, headUserID); err != nil {
			return Department{}, err
		}
	}
	var id string
	err := s.pool.QueryRow(ctx, `INSERT INTO adrient.departments (org_id, parent_id, name, head_user_id)
		VALUES ($1, NULLIF($2,'')::uuid, $3, NULLIF($4,'')::uuid) RETURNING id::text`, orgID, parentID, name, headUserID).Scan(&id)
	if err != nil {
		if isUnique(err) {
			return Department{}, ErrDuplicate
		}
		return Department{}, err
	}
	return s.GetDepartment(ctx, orgID, id)
}

// DepartmentPatch is a partial department update; nil leaves a field unchanged.
type DepartmentPatch struct {
	Name       *string `json:"name"`
	ParentID   *string `json:"parent_id"`
	HeadUserID *string `json:"head_user_id"`
}

// UpdateDepartment renames, re-parents or changes the head of a department.
// Moving a department under itself or one of its descendants returns ErrCycle.
func (s *Store) UpdateDepartment(ctx context.Context, orgID, id string, p DepartmentPatch) (Department, error) {
	if _, err := s.GetDepartment(ctx, orgID, id); err != nil {
		return Department{}, err
	}
	if p.Name != nil && strings.TrimSpace(*p.Name) == "" {
		return Department{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if p.ParentID != nil && *p.ParentID != "" {
		if err := s.checkDept(ctx, orgID, *p.ParentID); err != nil {
			return Department{}, err
		}
		sub, err := s.DepartmentSubtreeIDs(ctx, orgID, id)
		if err != nil {
			return Department{}, err
		}
		for _, d := range sub {
			if d == *p.ParentID {
				return Department{}, ErrCycle
			}
		}
	}
	if p.HeadUserID != nil && *p.HeadUserID != "" {
		if err := s.checkUser(ctx, orgID, *p.HeadUserID); err != nil {
			return Department{}, err
		}
	}
	var name any
	if p.Name != nil {
		name = strings.TrimSpace(*p.Name)
	}
	pSet, pVal := false, any(nil)
	if p.ParentID != nil {
		pSet = true
		if *p.ParentID != "" {
			pVal = *p.ParentID
		}
	}
	hSet, hVal := false, any(nil)
	if p.HeadUserID != nil {
		hSet = true
		if *p.HeadUserID != "" {
			hVal = *p.HeadUserID
		}
	}
	_, err := s.pool.Exec(ctx, `UPDATE adrient.departments SET
		name = COALESCE($3, name),
		parent_id = CASE WHEN $4 THEN $5::uuid ELSE parent_id END,
		head_user_id = CASE WHEN $6 THEN $7::uuid ELSE head_user_id END,
		updated_at = now() WHERE org_id=$1 AND id=$2`, orgID, id, name, pSet, pVal, hSet, hVal)
	if err != nil {
		if isUnique(err) {
			return Department{}, ErrDuplicate
		}
		return Department{}, err
	}
	return s.GetDepartment(ctx, orgID, id)
}

// DeleteDepartment removes a department. Its children become roots and its
// members lose their department (foreign keys use ON DELETE SET NULL).
func (s *Store) DeleteDepartment(ctx context.Context, orgID, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM adrient.departments WHERE org_id=$1 AND id=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
				return false
			}
		}
	}
	return true
}
