package demoseed

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contacts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
)

// teamMember is a named person of the demo company's leadership team. Each one
// gets a sign-in-less account (so Mail, Chat and Projects can show real names)
// and an HR employee record linked to that account.
type teamMember struct {
	First, Last, Title, Dept string
	UserID                   string
	EmpID                    string
}

var team = []*teamMember{
	{First: "Rohan", Last: "Deshpande", Title: "Head of Sales", Dept: "Sales"},
	{First: "Priya", Last: "Nair", Title: "Finance Controller", Dept: "Finance"},
	{First: "Imran", Last: "Sheikh", Title: "Engineering Manager", Dept: "Engineering"},
	{First: "Neha", Last: "Kapoor", Title: "HR Manager", Dept: "People and Culture"},
	{First: "Arjun", Last: "Patil", Title: "Warehouse Supervisor", Dept: "Operations"},
	{First: "Divya", Last: "Iyer", Title: "Customer Success Lead", Dept: "Sales"},
	{First: "Karan", Last: "Malhotra", Title: "Procurement Manager", Dept: "Operations"},
	{First: "Sneha", Last: "Kulkarni", Title: "Marketing Lead", Dept: "Sales"},
	{First: "Faisal", Last: "Khan", Title: "IT Administrator", Dept: "Operations"},
}

func (m *teamMember) name() string  { return m.First + " " + m.Last }
func (m *teamMember) email() string { return demoEmail(m.First, m.Last) }

// teamByFirst returns a member by first name (nil when missing).
func teamByFirst(first string) *teamMember {
	for _, m := range team {
		if m.First == first {
			return m
		}
	}
	return nil
}

// adminName is the demo persona the signed-in admin is linked to in HR.
const (
	adminFirst = "Asha"
	adminLast  = "Menon"
	adminTitle = "Head of Operations"
)

// v2Users creates the team's accounts. The accounts use a private issuer, so
// nobody can sign in as them.
func (s *seeder) v2Users() {
	n := 0
	for _, m := range team {
		err := s.pool.QueryRow(s.ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 AND lower(email)=lower($2)`, s.org, m.email()).Scan(&m.UserID)
		if err == nil {
			continue
		}
		err = s.pool.QueryRow(s.ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, job_title)
			VALUES ($1,'demo.adrient.example',$2,$3,$4,$5) RETURNING id::text`, s.org, "demo:"+slug(m.name()), m.email(), m.name(), m.Title).Scan(&m.UserID)
		if err != nil {
			s.warn("team user "+m.name(), err)
			continue
		}
		n++
	}
	s.count("team_users", n)
}

// v2HR links the admin account to an employee ("Asha Menon, Head of
// Operations"), adds the leadership team as employees, and gives the admin a
// team, leave balances and a payslip.
func (s *seeder) v2HR(acct *accounting.Repository) {
	repo := hr.NewRepository(s.pool).WithAccounting(hr.AccountingAdapter{Repo: acct})
	a, err := repo.ActorFor(s.ctx, s.org, s.admin, true)
	if err != nil {
		s.warn("hr actor", err)
		return
	}
	if err := repo.EnsureSeed(s.ctx, s.org); err != nil {
		s.warn("hr settings", err)
	}
	var adminEmail string
	if err := s.pool.QueryRow(s.ctx, `SELECT email FROM adrient.users WHERE id=$1::uuid`, s.admin).Scan(&adminEmail); err != nil {
		s.warn("hr admin email", err)
		return
	}
	start := s.today.AddDate(-3, -2, 0)
	var created []string
	ensure := func(first, last, title, dept, email, mgr string, years int) string {
		in := hr.NewHireInput{Name: first + " " + last, Email: email, Phone: s.r.phone(), JobTitle: title, Department: dept,
			StartDate: day(start.AddDate(years, -s.r.Intn(6), 0)), ManagerEmpID: mgr, SourceRef: "demo:team:" + slug(first+" "+last)}
		e, isNew, err := repo.CreateFromNewHire(s.ctx, a, in, nil)
		if err != nil {
			s.warn("hr hire "+first, err)
			return ""
		}
		if isNew {
			created = append(created, e.ID)
		}
		return e.ID
	}
	ashaID := ensure(adminFirst, adminLast, adminTitle, "Operations", adminEmail, "", 0)
	if ashaID == "" {
		return
	}
	for _, m := range team {
		mgr := ""
		if m.Dept == "Operations" {
			mgr = ashaID
		}
		m.EmpID = ensure(m.First, m.Last, m.Title, m.Dept, m.email(), mgr, 1)
	}
	s.count("hr_team_employees", len(created))
	// The existing Operations people report to Asha too: a real team.
	ops, _ := repo.ListLookups(s.ctx, s.org, "departments")
	opsID := ""
	for _, d := range ops {
		if d.Name == "Operations" {
			opsID = d.ID
		}
	}
	moved := 0
	if opsID != "" {
		list, _ := repo.ListEmployees(s.ctx, a, hr.EmployeeFilter{DepartmentID: opsID})
		for _, e := range list {
			if e.ID == ashaID || e.ManagerID == ashaID || strings.Contains(e.DesignationName, "Head of") {
				continue
			}
			full, err := repo.GetEmployee(s.ctx, a, e.ID)
			if err != nil {
				continue
			}
			full.ManagerID = ashaID
			if _, err := repo.SaveEmployee(s.ctx, a, e.ID, full); err != nil {
				s.warn("hr manager", err)
				continue
			}
			moved++
		}
	}
	s.count("hr_reports_to_admin", moved)
	// Salaries for the new people.
	structs, err := repo.ListStructures(s.ctx, a)
	if err != nil || len(structs) == 0 {
		s.warn("hr structures", errors.Join(err, errors.New("no salary structure")))
		return
	}
	all := append([]string{ashaID}, func() []string {
		var ids []string
		for _, m := range team {
			if m.EmpID != "" {
				ids = append(ids, m.EmpID)
			}
		}
		return ids
	}()...)
	for i, id := range all {
		annual := float64(780000 + (i%5)*120000)
		if i == 0 {
			annual = 1860000
		}
		if sal, _ := repo.ListSalaries(s.ctx, a, id); len(sal) > 0 {
			continue
		}
		lines, err := repo.PreviewStructure(s.ctx, a, structs[0].Lines, annual)
		if err != nil {
			s.warn("hr preview", err)
			break
		}
		if _, err := repo.SaveSalary(s.ctx, a, id, hr.EmployeeSalary{EffectiveFrom: day(s.today.AddDate(-1, 0, 0)), StructureID: structs[0].ID, AnnualGross: annual, Lines: lines, PFEnabled: true, PTEnabled: true, TDSEnabled: true}); err != nil {
			s.warn("hr salary", err)
		}
	}
	// Attendance for the last 20 days.
	codes := map[string]string{}
	if list, err := repo.ListEmployees(s.ctx, a, hr.EmployeeFilter{}); err == nil {
		for _, e := range list {
			codes[e.ID] = e.EmpCode
		}
	}
	var csv strings.Builder
	csv.WriteString("emp_code,timestamp\n")
	punches := 0
	for d := 1; d <= 20; d++ {
		t := s.today.AddDate(0, 0, -d)
		if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			continue
		}
		for i, id := range all {
			if (d+i)%9 == 0 || codes[id] == "" {
				continue
			}
			in := time.Date(t.Year(), t.Month(), t.Day(), 9, (i*3+d)%35, 0, 0, time.Local)
			out := time.Date(t.Year(), t.Month(), t.Day(), 18, (i*7+d)%45, 0, 0, time.Local)
			csv.WriteString(codes[id] + "," + in.Format("2006-01-02 15:04:05") + "\n" + codes[id] + "," + out.Format("2006-01-02 15:04:05") + "\n")
			punches += 2
		}
	}
	if res, err := repo.ImportPunchesCSV(s.ctx, a, strings.NewReader(csv.String())); err != nil {
		s.warn("hr attendance v2", err)
	} else {
		s.count("hr_punches", res.Imported)
	}
	// Leave: balances come from the accrual; add used and pending requests.
	_ = repo.EnsureAccrual(s.ctx, s.org, ashaID, s.today)
	types, _ := repo.ListLeaveTypes(s.ctx, s.org, false)
	var paid []hr.LeaveType
	for _, t := range types {
		if t.Paid {
			paid = append(paid, t)
		}
	}
	nextWorkday := func(t time.Time) time.Time {
		for t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			t = t.AddDate(0, 0, 1)
		}
		return t
	}
	if len(paid) > 0 {
		// Asha: one approved leave last month, one pending next month.
		past := nextWorkday(s.today.AddDate(0, -1, -3))
		if req, err := repo.ApplyLeave(s.ctx, a, hr.LeaveInput{EmployeeID: ashaID, TypeID: paid[0].ID, From: day(past), To: day(past.AddDate(0, 0, 1)), Reason: "Family wedding"}); err != nil {
			s.warn("hr leave asha", err)
		} else if _, err := repo.DecideLeave(s.ctx, a, req.ID, true, ""); err != nil {
			s.warn("hr leave asha decide", err)
		}
		fut := nextWorkday(s.today.AddDate(0, 1, 6))
		if _, err := repo.ApplyLeave(s.ctx, a, hr.LeaveInput{EmployeeID: ashaID, TypeID: paid[0].ID, From: day(fut), To: day(fut.AddDate(0, 0, 2)), Reason: "Annual trip"}); err != nil {
			s.warn("hr leave asha pending", err)
		}
		// Her team: requests waiting for her approval.
		nreq := 0
		for i, m := range team {
			if m.Dept != "Operations" || m.EmpID == "" {
				continue
			}
			from := nextWorkday(s.today.AddDate(0, 0, 4+i*2))
			if _, err := repo.ApplyLeave(s.ctx, a, hr.LeaveInput{EmployeeID: m.EmpID, TypeID: paid[i%len(paid)].ID, From: day(from), To: day(from), Reason: []string{"Medical appointment", "Personal work", "Festival travel"}[i%3]}); err != nil {
				s.warn("hr team leave", err)
				continue
			}
			nreq++
		}
		s.count("hr_pending_leave", nreq)
	}
	// Payroll for the current month, locked so every payslip is final.
	period := s.today.Format("2006-01")
	run, err := repo.CreateRun(s.ctx, a, period)
	if err != nil {
		s.warn("hr payroll run v2", err)
		return
	}
	if run, err = repo.MarkReviewed(s.ctx, a, run.ID); err != nil {
		s.warn("hr payroll review v2", err)
		return
	}
	if _, err = repo.LockRun(s.ctx, a, run.ID); err != nil {
		s.warn("hr payroll lock v2", err)
		return
	}
	s.count("hr_payroll_runs", 1)
}

// v2Contacts adds 60 contacts to the admin's address book.
func (s *seeder) v2Contacts() {
	repo := contacts.NewRepository(s.pool)
	groups := []struct {
		label  string
		titles []string
	}{
		{"Clients", []string{"Purchase Head", "Owner", "Accounts Manager", "Store Manager"}},
		{"Suppliers", []string{"Sales Manager", "Key Account Manager", "Dispatch Coordinator"}},
		{"Partners", []string{"Director", "Business Development Manager", "Channel Lead"}},
		{"Team", []string{"Consultant", "Chartered Accountant", "Legal Adviser", "Auditor"}},
	}
	seen := map[string]bool{}
	made := 0
	for i := 0; made < 60 && i < 400; i++ {
		f := firstNames[i%len(firstNames)]
		l := lastNames[(i*7+i/len(firstNames))%len(lastNames)]
		if seen[f+l] {
			continue
		}
		seen[f+l] = true
		g := groups[made%len(groups)]
		co := coPrefix[(made*3)%len(coPrefix)] + " " + coSuffix[(made*5)%len(coSuffix)]
		c := contacts.Fields{DisplayName: f + " " + l, FirstName: f, LastName: l, Company: co, JobTitle: g.titles[made%len(g.titles)],
			Emails: []string{fmt.Sprintf("%s.%s@%s.example", slug(f), slug(l), slug(co))}, Phones: []string{s.r.phone()},
			Labels: []string{g.label}, Notes: "Met at the " + s.r.pick(cities) + " trade fair.", Starred: made%9 == 0}
		if _, err := repo.Create(s.ctx, s.org, s.admin, c); err != nil {
			s.warn("contacts", err)
			continue
		}
		made++
	}
	s.count("contacts", made)
}
