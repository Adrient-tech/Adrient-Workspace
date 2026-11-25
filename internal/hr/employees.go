package hr

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Employee is one HR record. Sensitive fields (personal data, IDs, bank) are
// blank unless the caller is HR or the employee themselves.
type Employee struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	ContactID       string `json:"contact_id"`
	EmpCode         string `json:"emp_code"`
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Name            string `json:"name"`
	WorkEmail       string `json:"work_email"`
	PersonalEmail   string `json:"personal_email"`
	Phone           string `json:"phone"`
	DOB             string `json:"dob"`
	Gender          string `json:"gender"`
	MaritalStatus   string `json:"marital_status"`
	BloodGroup      string `json:"blood_group"`
	Address         string `json:"address"`
	JoinDate        string `json:"join_date"`
	ProbationEnd    string `json:"probation_end"`
	ExitDate        string `json:"exit_date"`
	ExitReason      string `json:"exit_reason"`
	Status          string `json:"status"`
	EmploymentType  string `json:"employment_type"`
	DepartmentID    string `json:"department_id"`
	DepartmentName  string `json:"department_name"`
	DesignationID   string `json:"designation_id"`
	DesignationName string `json:"designation_name"`
	LocationID      string `json:"location_id"`
	LocationName    string `json:"location_name"`
	ManagerID       string `json:"manager_id"`
	ManagerName     string `json:"manager_name"`
	ShiftID         string `json:"shift_id"`
	PAN             string `json:"pan"`
	AadhaarLast4    string `json:"aadhaar_last4"`
	UAN             string `json:"uan"`
	ESINumber       string `json:"esi_number"`
	BankName        string `json:"bank_name"`
	BankAccount     string `json:"bank_account"`
	BankIFSC        string `json:"bank_ifsc"`
	// Full is true when sensitive fields are included.
	Full bool `json:"full"`
}

const empCols = `e.id::text, coalesce(e.user_id::text,''), coalesce(e.contact_id::text,''), e.emp_code, e.first_name, e.last_name,
	e.work_email, e.personal_email, e.phone, coalesce(to_char(e.dob,'YYYY-MM-DD'),''), e.gender, e.marital_status, e.blood_group, e.address,
	to_char(e.join_date,'YYYY-MM-DD'), coalesce(to_char(e.probation_end,'YYYY-MM-DD'),''), coalesce(to_char(e.exit_date,'YYYY-MM-DD'),''),
	e.exit_reason, e.status, e.employment_type, coalesce(e.department_id::text,''), coalesce(d.name,''), coalesce(e.designation_id::text,''),
	coalesce(g.name,''), coalesce(e.location_id::text,''), coalesce(l.name,''), coalesce(e.manager_id::text,''),
	coalesce(nullif(trim(m.first_name||' '||m.last_name),''),''), coalesce(e.shift_id::text,''),
	e.pan, e.aadhaar_last4, e.uan, e.esi_number, e.bank_name, e.bank_account, e.bank_ifsc`

const empFrom = ` FROM adrient.hr_employees e
	LEFT JOIN adrient.departments d ON d.id=e.department_id
	LEFT JOIN adrient.hr_designations g ON g.id=e.designation_id
	LEFT JOIN adrient.hr_locations l ON l.id=e.location_id
	LEFT JOIN adrient.hr_employees m ON m.id=e.manager_id`

func scanEmployee(row pgx.Row) (Employee, error) {
	var e Employee
	err := row.Scan(&e.ID, &e.UserID, &e.ContactID, &e.EmpCode, &e.FirstName, &e.LastName, &e.WorkEmail, &e.PersonalEmail, &e.Phone,
		&e.DOB, &e.Gender, &e.MaritalStatus, &e.BloodGroup, &e.Address, &e.JoinDate, &e.ProbationEnd, &e.ExitDate, &e.ExitReason,
		&e.Status, &e.EmploymentType, &e.DepartmentID, &e.DepartmentName, &e.DesignationID, &e.DesignationName, &e.LocationID,
		&e.LocationName, &e.ManagerID, &e.ManagerName, &e.ShiftID, &e.PAN, &e.AadhaarLast4, &e.UAN, &e.ESINumber, &e.BankName,
		&e.BankAccount, &e.BankIFSC)
	e.Name = strings.TrimSpace(e.FirstName + " " + e.LastName)
	return e, err
}

// redact strips everything but the directory-level fields.
func redact(e Employee) Employee {
	e.PersonalEmail, e.DOB, e.Gender, e.MaritalStatus, e.BloodGroup, e.Address = "", "", "", "", "", ""
	e.ProbationEnd, e.ExitReason, e.PAN, e.AadhaarLast4, e.UAN, e.ESINumber = "", "", "", "", "", ""
	e.BankName, e.BankAccount, e.BankIFSC, e.ContactID = "", "", "", ""
	e.Full = false
	return e
}

// shape applies the visibility rule for the caller.
func (a Actor) shape(e Employee) Employee {
	if a.IsHR || (a.EmployeeID != "" && a.EmployeeID == e.ID) {
		e.Full = true
		return e
	}
	return redact(e)
}

// EmployeeFilter narrows ListEmployees.
type EmployeeFilter struct {
	Q            string
	Status       string
	DepartmentID string
	LocationID   string
	ManagerID    string
	// Paging (see page.go). Limit 0 means the default cap of 100.
	Limit  int
	Offset int
	Sort   string
	Desc   bool
}

// ListEmployees is the directory search. Every member can search active
// employees (directory fields only); HR sees everything incl. exited.
func (r *Repository) ListEmployees(ctx context.Context, a Actor, f EmployeeFilter) ([]Employee, error) {
	q := `SELECT ` + empCols + empFrom + ` WHERE e.org_id=$1`
	args := []any{a.OrgID}
	add := func(cond string, v any) {
		args = append(args, v)
		q += fmt.Sprintf(" AND "+cond, len(args))
	}
	if f.Status != "" {
		if f.Status != "all" {
			add("e.status=$%d", f.Status)
		}
		if !a.IsHR && f.Status != "active" && f.Status != "notice" {
			return nil, forbidden("only HR can list former employees")
		}
	} else {
		q += " AND e.status<>'exited'"
	}
	if !a.IsHR && f.Status == "all" {
		return nil, forbidden("only HR can list former employees")
	}
	if f.Q != "" {
		args = append(args, "%"+escapeLike(strings.ToLower(f.Q))+"%")
		q += fmt.Sprintf(` AND (lower(e.first_name||' '||e.last_name) LIKE $%[1]d OR lower(e.emp_code) LIKE $%[1]d OR lower(e.work_email) LIKE $%[1]d
			OR lower(coalesce(g.name,'')) LIKE $%[1]d OR lower(coalesce(d.name,'')) LIKE $%[1]d)`, len(args))
	}
	if isUUID(f.DepartmentID) {
		add("e.department_id=$%d::uuid", f.DepartmentID)
	}
	if isUUID(f.LocationID) {
		add("e.location_id=$%d::uuid", f.LocationID)
	}
	if isUUID(f.ManagerID) {
		add("e.manager_id=$%d::uuid", f.ManagerID)
	}
	q += f.orderLimit()
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Employee{}
	for rows.Next() {
		e, err := scanEmployee(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a.shape(e))
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *Repository) getEmployee(ctx context.Context, d db, orgID, id string) (Employee, error) {
	if !isUUID(id) {
		return Employee{}, ErrNotFound
	}
	e, err := scanEmployee(d.QueryRow(ctx, `SELECT `+empCols+empFrom+` WHERE e.org_id=$1 AND e.id=$2::uuid`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

// GetEmployee returns one employee shaped for the caller.
func (r *Repository) GetEmployee(ctx context.Context, a Actor, id string) (Employee, error) {
	e, err := r.getEmployee(ctx, r.pool, a.OrgID, id)
	if err != nil {
		return e, err
	}
	if e.Status == "exited" && !a.IsHR && a.EmployeeID != e.ID {
		return Employee{}, ErrNotFound
	}
	return a.shape(e), nil
}

// isManagerOf reports whether the caller is the direct manager of employee id.
func (r *Repository) isManagerOf(ctx context.Context, a Actor, empID string) bool {
	if a.EmployeeID == "" || !isUUID(empID) {
		return false
	}
	var ok bool
	_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.hr_employees WHERE org_id=$1 AND id=$2::uuid AND manager_id=$3::uuid)`,
		a.OrgID, empID, a.EmployeeID).Scan(&ok)
	return ok
}

// canSee reports whether the caller may act on the employee's HR data
// (HR, the employee, or their direct manager).
func (r *Repository) canSee(ctx context.Context, a Actor, empID string) bool {
	return a.IsHR || (a.EmployeeID != "" && a.EmployeeID == empID) || r.isManagerOf(ctx, a, empID)
}

var employmentTypes = map[string]bool{"full_time": true, "part_time": true, "contract": true, "intern": true, "consultant": true}

func (r *Repository) nextEmpCode(ctx context.Context, d db, orgID string) (string, error) {
	var n int
	err := d.QueryRow(ctx, `SELECT coalesce(max(substring(emp_code from '^EMP([0-9]+)$')::int),0)+1 FROM adrient.hr_employees WHERE org_id=$1`, orgID).Scan(&n)
	return fmt.Sprintf("EMP%04d", n), err
}

func (r *Repository) validateEmployee(ctx context.Context, d db, orgID, id string, e *Employee) error {
	e.FirstName, e.LastName = strings.TrimSpace(e.FirstName), strings.TrimSpace(e.LastName)
	if e.FirstName == "" {
		return invalid("first name is required")
	}
	if len(e.FirstName) > 100 || len(e.LastName) > 100 {
		return invalid("name is too long")
	}
	var err error
	if e.JoinDate, err = validDate(e.JoinDate); err != nil {
		return invalid("join date is required (YYYY-MM-DD)")
	}
	for _, p := range []*string{&e.DOB, &e.ProbationEnd, &e.ExitDate} {
		if *p != "" {
			if *p, err = validDate(*p); err != nil {
				return err
			}
		}
	}
	if e.Status == "" {
		e.Status = "active"
	}
	if e.Status != "active" && e.Status != "notice" && e.Status != "exited" {
		return invalid("status must be active, notice or exited")
	}
	if e.Status == "exited" && e.ExitDate == "" {
		return invalid("an exit date is required for an exited employee")
	}
	if e.ExitDate != "" && e.ExitDate < e.JoinDate {
		return invalid("exit date is before the join date")
	}
	if e.EmploymentType == "" {
		e.EmploymentType = "full_time"
	}
	if !employmentTypes[e.EmploymentType] {
		return invalid("unknown employment type %q", e.EmploymentType)
	}
	e.PAN = strings.ToUpper(strings.TrimSpace(e.PAN))
	if e.PAN != "" && !panRe.MatchString(e.PAN) {
		return invalid("PAN must look like ABCDE1234F")
	}
	e.BankIFSC = strings.ToUpper(strings.TrimSpace(e.BankIFSC))
	if e.BankIFSC != "" && !ifscRe.MatchString(e.BankIFSC) {
		return invalid("IFSC must look like HDFC0001234")
	}
	e.AadhaarLast4 = strings.TrimSpace(e.AadhaarLast4)
	if e.AadhaarLast4 != "" && !last4Re.MatchString(e.AadhaarLast4) {
		return invalid("store only the last 4 digits of Aadhaar")
	}
	// Reference checks, all org scoped.
	check := func(label, table, val string) error {
		if val == "" {
			return nil
		}
		if !isUUID(val) {
			return invalid("bad %s", label)
		}
		var ok bool
		if err := d.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.`+table+` WHERE id=$1::uuid AND org_id=$2)`, val, orgID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return invalid("unknown %s", label)
		}
		return nil
	}
	if err := check("department", "departments", e.DepartmentID); err != nil {
		return err
	}
	if err := check("designation", "hr_designations", e.DesignationID); err != nil {
		return err
	}
	if err := check("location", "hr_locations", e.LocationID); err != nil {
		return err
	}
	if err := check("shift", "hr_shifts", e.ShiftID); err != nil {
		return err
	}
	if e.UserID != "" {
		if err := check("user", "users", e.UserID); err != nil {
			return err
		}
		var other string
		err := d.QueryRow(ctx, `SELECT id::text FROM adrient.hr_employees WHERE org_id=$1 AND user_id=$2::uuid AND id<>coalesce(nullif($3,'')::uuid,'00000000-0000-0000-0000-000000000000')`,
			orgID, e.UserID, id).Scan(&other)
		if err == nil {
			return conflict("that user account is already linked to another employee")
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	if e.ManagerID != "" {
		if err := check("manager", "hr_employees", e.ManagerID); err != nil {
			return err
		}
		if id != "" {
			if e.ManagerID == id {
				return invalid("an employee cannot manage themselves")
			}
			// Walk up from the proposed manager; reaching this employee is a cycle.
			cur := e.ManagerID
			for i := 0; i < 64 && cur != ""; i++ {
				var next string
				if err := d.QueryRow(ctx, `SELECT coalesce(manager_id::text,'') FROM adrient.hr_employees WHERE id=$1::uuid`, cur).Scan(&next); err != nil {
					break
				}
				if next == id {
					return conflict("that manager choice would create a reporting loop")
				}
				cur = next
			}
		}
	}
	if e.ContactID != "" {
		if err := check("contact", "contacts", e.ContactID); err != nil {
			return err
		}
	}
	return nil
}

// SaveEmployee creates (id == "") or updates an employee (HR only).
func (r *Repository) SaveEmployee(ctx context.Context, a Actor, id string, e Employee) (Employee, error) {
	if err := a.requireHR(); err != nil {
		return Employee{}, err
	}
	if id != "" {
		if _, err := r.getEmployee(ctx, r.pool, a.OrgID, id); err != nil {
			return Employee{}, err
		}
	}
	if err := r.validateEmployee(ctx, r.pool, a.OrgID, id, &e); err != nil {
		return Employee{}, err
	}
	e.EmpCode = strings.TrimSpace(e.EmpCode)
	args := []any{a.OrgID, e.UserID, e.ContactID, e.FirstName, e.LastName, e.WorkEmail, e.PersonalEmail, e.Phone, e.DOB, e.Gender, e.MaritalStatus,
		e.BloodGroup, e.Address, e.JoinDate, e.ProbationEnd, e.ExitDate, e.ExitReason, e.Status, e.EmploymentType, e.DepartmentID, e.DesignationID,
		e.LocationID, e.ManagerID, e.ShiftID, e.PAN, e.AadhaarLast4, e.UAN, e.ESINumber, e.BankName, e.BankAccount, e.BankIFSC}
	var out string
	if id == "" {
		if e.EmpCode == "" {
			c, err := r.nextEmpCode(ctx, r.pool, a.OrgID)
			if err != nil {
				return Employee{}, err
			}
			e.EmpCode = c
		}
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_employees (org_id, user_id, contact_id, first_name, last_name, work_email, personal_email, phone, dob,
			gender, marital_status, blood_group, address, join_date, probation_end, exit_date, exit_reason, status, employment_type, department_id,
			designation_id, location_id, manager_id, shift_id, pan, aadhaar_last4, uan, esi_number, bank_name, bank_account, bank_ifsc, emp_code)
			VALUES ($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,NULLIF($9,'')::date,$10,$11,$12,$13,$14::date,NULLIF($15,'')::date,
			NULLIF($16,'')::date,$17,$18,$19,NULLIF($20,'')::uuid,NULLIF($21,'')::uuid,NULLIF($22,'')::uuid,NULLIF($23,'')::uuid,NULLIF($24,'')::uuid,
			$25,$26,$27,$28,$29,$30,$31,$32) RETURNING id::text`, append(args, e.EmpCode)...).Scan(&out)
		if err != nil {
			return Employee{}, mapPgErr(err)
		}
		r.audit(ctx, r.pool, a.OrgID, a.UserID, "create", "employee", out, e.EmpCode)
	} else {
		if e.EmpCode == "" {
			return Employee{}, invalid("employee code is required")
		}
		_, err := r.pool.Exec(ctx, `UPDATE adrient.hr_employees SET user_id=NULLIF($2,'')::uuid, contact_id=NULLIF($3,'')::uuid, first_name=$4, last_name=$5,
			work_email=$6, personal_email=$7, phone=$8, dob=NULLIF($9,'')::date, gender=$10, marital_status=$11, blood_group=$12, address=$13,
			join_date=$14::date, probation_end=NULLIF($15,'')::date, exit_date=NULLIF($16,'')::date, exit_reason=$17, status=$18, employment_type=$19,
			department_id=NULLIF($20,'')::uuid, designation_id=NULLIF($21,'')::uuid, location_id=NULLIF($22,'')::uuid, manager_id=NULLIF($23,'')::uuid,
			shift_id=NULLIF($24,'')::uuid, pan=$25, aadhaar_last4=$26, uan=$27, esi_number=$28, bank_name=$29, bank_account=$30, bank_ifsc=$31,
			emp_code=$32, updated_at=now() WHERE org_id=$1 AND id=$33::uuid`, append(args, e.EmpCode, id)...)
		if err != nil {
			return Employee{}, mapPgErr(err)
		}
		out = id
		r.audit(ctx, r.pool, a.OrgID, a.UserID, "update", "employee", id, e.EmpCode)
	}
	r.syncDirectory(ctx, a.OrgID, out)
	return r.GetEmployee(ctx, a, out)
}

func mapPgErr(err error) error {
	if err != nil && strings.Contains(err.Error(), "23505") {
		return conflict("an employee with that code already exists")
	}
	return err
}

// syncDirectory mirrors job attributes onto the linked user so the directory,
// PersonCard and the org chart agree with HR.
func (r *Repository) syncDirectory(ctx context.Context, orgID, empID string) {
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.users u SET
			job_title = coalesce(g.name,''),
			department_id = e.department_id,
			manager_id = (SELECT mu.user_id FROM adrient.hr_employees mu WHERE mu.id=e.manager_id),
			employee_id = e.emp_code,
			start_date = e.join_date,
			updated_at = now()
		FROM adrient.hr_employees e LEFT JOIN adrient.hr_designations g ON g.id=e.designation_id
		WHERE e.id=$1::uuid AND e.org_id=$2 AND u.id=e.user_id AND u.org_id=$2`, empID, orgID)
	// Reports of this employee whose manager account just became known.
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.users u SET manager_id=(SELECT user_id FROM adrient.hr_employees WHERE id=$1::uuid)
		FROM adrient.hr_employees e WHERE e.manager_id=$1::uuid AND e.org_id=$2 AND u.id=e.user_id AND u.org_id=$2`, empID, orgID)
}

// SaveOwnProfile lets an employee edit the fields they may change themselves.
func (r *Repository) SaveOwnProfile(ctx context.Context, a Actor, e Employee) (Employee, error) {
	if a.EmployeeID == "" {
		return Employee{}, forbidden("no employee record is linked to your account")
	}
	_, err := r.pool.Exec(ctx, `UPDATE adrient.hr_employees SET phone=$3, personal_email=$4, address=$5, marital_status=$6, blood_group=$7, updated_at=now()
		WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, a.EmployeeID, strings.TrimSpace(e.Phone), strings.TrimSpace(e.PersonalEmail),
		strings.TrimSpace(e.Address), strings.TrimSpace(e.MaritalStatus), strings.TrimSpace(e.BloodGroup))
	if err != nil {
		return Employee{}, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "update_own", "employee", a.EmployeeID, "")
	return r.GetEmployee(ctx, a, a.EmployeeID)
}

// DeleteEmployee removes an employee with no payroll history (HR only);
// otherwise mark them exited instead.
func (r *Repository) DeleteEmployee(ctx context.Context, a Actor, id string) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	if _, err := r.getEmployee(ctx, r.pool, a.OrgID, id); err != nil {
		return err
	}
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM adrient.hr_payslips WHERE employee_id=$1::uuid)+(SELECT count(*) FROM adrient.hr_leave_requests WHERE employee_id=$1::uuid)`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return conflict("this employee has payroll or leave history; mark them as exited instead")
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.hr_employees WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id)
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "delete", "employee", id, "")
	return err
}

// ---- emergency contacts ----

// EmergencyContact is a person to call for an employee.
type EmergencyContact struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	Phone        string `json:"phone"`
}

// ListEmergencyContacts is limited to HR and the employee.
func (r *Repository) ListEmergencyContacts(ctx context.Context, a Actor, empID string) ([]EmergencyContact, error) {
	if !(a.IsHR || (a.EmployeeID != "" && a.EmployeeID == empID)) {
		return nil, forbidden("not allowed")
	}
	if !isUUID(empID) {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, relationship, phone FROM adrient.hr_emergency_contacts WHERE org_id=$1 AND employee_id=$2::uuid ORDER BY created_at`, a.OrgID, empID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmergencyContact{}
	for rows.Next() {
		var c EmergencyContact
		if err := rows.Scan(&c.ID, &c.Name, &c.Relationship, &c.Phone); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddEmergencyContact is limited to HR and the employee.
func (r *Repository) AddEmergencyContact(ctx context.Context, a Actor, empID string, c EmergencyContact) (EmergencyContact, error) {
	if !(a.IsHR || (a.EmployeeID != "" && a.EmployeeID == empID)) {
		return c, forbidden("not allowed")
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || strings.TrimSpace(c.Phone) == "" {
		return c, invalid("name and phone are required")
	}
	if _, err := r.getEmployee(ctx, r.pool, a.OrgID, empID); err != nil {
		return c, err
	}
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_emergency_contacts (org_id, employee_id, name, relationship, phone) VALUES ($1,$2::uuid,$3,$4,$5) RETURNING id::text`,
		a.OrgID, empID, c.Name, strings.TrimSpace(c.Relationship), strings.TrimSpace(c.Phone)).Scan(&c.ID)
	return c, err
}

// DeleteEmergencyContact is limited to HR and the employee.
func (r *Repository) DeleteEmergencyContact(ctx context.Context, a Actor, empID, id string) error {
	if !(a.IsHR || (a.EmployeeID != "" && a.EmployeeID == empID)) {
		return forbidden("not allowed")
	}
	if !isUUID(id) || !isUUID(empID) {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.hr_emergency_contacts WHERE org_id=$1 AND employee_id=$2::uuid AND id=$3::uuid`, a.OrgID, empID, id)
	return err
}

// ---- departments, designations, locations ----

// Lookup is a simple named reference row.
type Lookup struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	State    string `json:"state,omitempty"`
	Address  string `json:"address,omitempty"`
	ParentID string `json:"parent_id,omitempty"`
	Count    int    `json:"count"`
}

// ListLookups returns "departments", "designations" or "locations" (any member).
func (r *Repository) ListLookups(ctx context.Context, orgID, kind string) ([]Lookup, error) {
	var q string
	switch kind {
	case "departments":
		q = `SELECT d.id::text, d.name, '', '', coalesce(d.parent_id::text,''), (SELECT count(*) FROM adrient.hr_employees e WHERE e.department_id=d.id AND e.status<>'exited')
			FROM adrient.departments d WHERE d.org_id=$1 ORDER BY d.name`
	case "designations":
		q = `SELECT g.id::text, g.name, '', '', '', (SELECT count(*) FROM adrient.hr_employees e WHERE e.designation_id=g.id AND e.status<>'exited')
			FROM adrient.hr_designations g WHERE g.org_id=$1 ORDER BY g.name`
	case "locations":
		q = `SELECT l.id::text, l.name, l.state, l.address, '', (SELECT count(*) FROM adrient.hr_employees e WHERE e.location_id=l.id AND e.status<>'exited')
			FROM adrient.hr_locations l WHERE l.org_id=$1 ORDER BY l.name`
	default:
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lookup{}
	for rows.Next() {
		var l Lookup
		if err := rows.Scan(&l.ID, &l.Name, &l.State, &l.Address, &l.ParentID, &l.Count); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SaveLookup creates or updates a department, designation or location (HR only).
func (r *Repository) SaveLookup(ctx context.Context, a Actor, kind, id string, l Lookup) (Lookup, error) {
	if err := a.requireHR(); err != nil {
		return l, err
	}
	l.Name = strings.TrimSpace(l.Name)
	if l.Name == "" || len(l.Name) > 120 {
		return l, invalid("a name of up to 120 characters is required")
	}
	var err error
	switch kind {
	case "departments":
		if id == "" {
			err = r.pool.QueryRow(ctx, `INSERT INTO adrient.departments (org_id, name, parent_id) VALUES ($1,$2,NULLIF($3,'')::uuid) RETURNING id::text`, a.OrgID, l.Name, l.ParentID).Scan(&l.ID)
		} else {
			if l.ParentID == id {
				return l, invalid("a department cannot be its own parent")
			}
			_, err = r.pool.Exec(ctx, `UPDATE adrient.departments SET name=$3, parent_id=NULLIF($4,'')::uuid, updated_at=now() WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id, l.Name, l.ParentID)
			l.ID = id
		}
	case "designations":
		if id == "" {
			err = r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_designations (org_id, name) VALUES ($1,$2) RETURNING id::text`, a.OrgID, l.Name).Scan(&l.ID)
		} else {
			_, err = r.pool.Exec(ctx, `UPDATE adrient.hr_designations SET name=$3 WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id, l.Name)
			l.ID = id
		}
	case "locations":
		l.State = strings.TrimSpace(l.State)
		if id == "" {
			err = r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_locations (org_id, name, state, address) VALUES ($1,$2,$3,$4) RETURNING id::text`, a.OrgID, l.Name, l.State, l.Address).Scan(&l.ID)
		} else {
			_, err = r.pool.Exec(ctx, `UPDATE adrient.hr_locations SET name=$3, state=$4, address=$5 WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id, l.Name, l.State, l.Address)
			l.ID = id
		}
	default:
		return l, ErrNotFound
	}
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return l, conflict("%q already exists", l.Name)
		}
		return l, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "save", kind, l.ID, l.Name)
	return l, nil
}

// DeleteLookup removes an unused lookup row (HR only).
func (r *Repository) DeleteLookup(ctx context.Context, a Actor, kind, id string) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	if !isUUID(id) {
		return ErrNotFound
	}
	table := map[string]string{"departments": "departments", "designations": "hr_designations", "locations": "hr_locations"}[kind]
	if table == "" {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.`+table+` WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id)
	return err
}

// ---- CSV import / export ----

var csvHeader = []string{"emp_code", "first_name", "last_name", "work_email", "phone", "join_date", "dob", "gender", "employment_type", "status",
	"department", "designation", "location", "manager_code", "pan", "uan", "bank_name", "bank_account", "bank_ifsc"}

// ExportEmployeesCSV writes every employee (HR only).
func (r *Repository) ExportEmployeesCSV(ctx context.Context, a Actor, w io.Writer) error {
	if err := a.requireHR(); err != nil {
		return err
	}
	list, err := r.ListEmployees(ctx, a, EmployeeFilter{Status: "all"})
	if err != nil {
		return err
	}
	codes := map[string]string{}
	for _, e := range list {
		codes[e.ID] = e.EmpCode
	}
	cw := csv.NewWriter(w)
	_ = cw.Write(csvHeader)
	for _, e := range list {
		_ = cw.Write([]string{csvSafe(e.EmpCode), csvSafe(e.FirstName), csvSafe(e.LastName), csvSafe(e.WorkEmail), csvSafe(e.Phone), e.JoinDate, e.DOB,
			e.Gender, e.EmploymentType, e.Status, csvSafe(e.DepartmentName), csvSafe(e.DesignationName), csvSafe(e.LocationName), codes[e.ManagerID],
			e.PAN, csvSafe(e.UAN), csvSafe(e.BankName), csvSafe(e.BankAccount), e.BankIFSC})
	}
	cw.Flush()
	return cw.Error()
}

// csvSafe defuses spreadsheet formula injection in exported text.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// ImportRowError describes a rejected CSV row.
type ImportRowError struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

// ImportResult summarises a CSV import.
type ImportResult struct {
	Created int              `json:"created"`
	Updated int              `json:"updated"`
	Errors  []ImportRowError `json:"errors"`
}

// ImportEmployeesCSV upserts employees by emp_code (HR only). Missing
// departments, designations and locations are created. Managers are resolved
// after all rows are loaded so row order does not matter.
func (r *Repository) ImportEmployeesCSV(ctx context.Context, a Actor, in io.Reader) (ImportResult, error) {
	res := ImportResult{Errors: []ImportRowError{}}
	if err := a.requireHR(); err != nil {
		return res, err
	}
	cr := csv.NewReader(in)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	head, err := cr.Read()
	if err != nil {
		return res, invalid("the file is empty or not CSV")
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))] = i
	}
	for _, need := range []string{"first_name", "join_date"} {
		if _, ok := col[need]; !ok {
			return res, invalid("missing required column %q", need)
		}
	}
	get := func(rec []string, k string) string {
		if i, ok := col[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	type pending struct {
		row         int
		empID, code string
		managerCode string
	}
	var mgrs []pending
	row := 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		row++
		if err != nil {
			res.Errors = append(res.Errors, ImportRowError{row, "unreadable row: " + err.Error()})
			continue
		}
		if len(res.Errors) > 200 || row > 5001 {
			res.Errors = append(res.Errors, ImportRowError{row, "stopped: too many rows or errors"})
			break
		}
		e := Employee{EmpCode: get(rec, "emp_code"), FirstName: get(rec, "first_name"), LastName: get(rec, "last_name"),
			WorkEmail: get(rec, "work_email"), Phone: get(rec, "phone"), JoinDate: get(rec, "join_date"), DOB: get(rec, "dob"),
			Gender: get(rec, "gender"), EmploymentType: strings.ToLower(strings.ReplaceAll(get(rec, "employment_type"), " ", "_")),
			Status: strings.ToLower(get(rec, "status")), PAN: get(rec, "pan"), UAN: get(rec, "uan"), BankName: get(rec, "bank_name"),
			BankAccount: get(rec, "bank_account"), BankIFSC: get(rec, "bank_ifsc")}
		if e.FirstName == "" && e.JoinDate == "" && e.EmpCode == "" {
			continue // blank line
		}
		if e.Status == "exited" && get(rec, "exit_date") == "" {
			e.ExitDate = e.JoinDate
		}
		for _, p := range []struct {
			k, table string
			dst      *string
		}{{"department", "departments", &e.DepartmentID}, {"designation", "hr_designations", &e.DesignationID}, {"location", "hr_locations", &e.LocationID}} {
			name := get(rec, p.k)
			if name == "" {
				continue
			}
			id, err := r.lookupOrCreate(ctx, a.OrgID, p.table, name)
			if err != nil {
				res.Errors = append(res.Errors, ImportRowError{row, err.Error()})
				e.FirstName = ""
				break
			}
			*p.dst = id
		}
		if e.FirstName == "" {
			if len(res.Errors) == 0 || res.Errors[len(res.Errors)-1].Row != row {
				res.Errors = append(res.Errors, ImportRowError{row, "first name is required"})
			}
			continue
		}
		if e.WorkEmail != "" {
			_ = r.pool.QueryRow(ctx, `SELECT u.id::text FROM adrient.users u WHERE u.org_id=$1 AND lower(u.email)=lower($2)
				AND NOT EXISTS (SELECT 1 FROM adrient.hr_employees x WHERE x.user_id=u.id AND lower(x.emp_code)<>lower($3)) LIMIT 1`, a.OrgID, e.WorkEmail, e.EmpCode).Scan(&e.UserID)
		}
		var existing string
		if e.EmpCode != "" {
			_ = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.hr_employees WHERE org_id=$1 AND lower(emp_code)=lower($2)`, a.OrgID, e.EmpCode).Scan(&existing)
		}
		saved, err := r.SaveEmployee(ctx, a, existing, e)
		if err != nil {
			res.Errors = append(res.Errors, ImportRowError{row, strings.TrimPrefix(err.Error(), "invalid: ")})
			continue
		}
		if existing != "" {
			res.Updated++
		} else {
			res.Created++
		}
		if mc := get(rec, "manager_code"); mc != "" {
			mgrs = append(mgrs, pending{row, saved.ID, saved.EmpCode, mc})
		}
	}
	for _, p := range mgrs {
		var mid string
		if err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.hr_employees WHERE org_id=$1 AND lower(emp_code)=lower($2)`, a.OrgID, p.managerCode).Scan(&mid); err != nil {
			res.Errors = append(res.Errors, ImportRowError{p.row, fmt.Sprintf("manager code %q not found", p.managerCode)})
			continue
		}
		cur, err := r.getEmployee(ctx, r.pool, a.OrgID, p.empID)
		if err != nil {
			continue
		}
		cur.ManagerID = mid
		if _, err := r.SaveEmployee(ctx, a, p.empID, cur); err != nil {
			res.Errors = append(res.Errors, ImportRowError{p.row, strings.TrimPrefix(err.Error(), "conflict: ")})
		}
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "import", "employees", "", fmt.Sprintf("%d created, %d updated, %d errors", res.Created, res.Updated, len(res.Errors)))
	return res, nil
}

func (r *Repository) lookupOrCreate(ctx context.Context, orgID, table, name string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.`+table+` WHERE org_id=$1 AND lower(name)=lower($2)`+
		map[bool]string{true: ` AND parent_id IS NULL`, false: ``}[table == "departments"]+` LIMIT 1`, orgID, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.`+table+` (org_id, name) VALUES ($1,$2) RETURNING id::text`, orgID, name).Scan(&id)
	return id, err
}
