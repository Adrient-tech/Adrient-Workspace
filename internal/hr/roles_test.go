package hr

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRolesAndPrivacy(t *testing.T) {
	f := newFixture(t)
	hrA := f.team()
	asha, ravi, meera := f.actor("Asha", false), f.actor("Ravi", false), f.actor("Meera", false)
	if !hrA.IsHR || asha.IsHR || !meera.IsManager || asha.IsManager {
		t.Fatalf("roles wrong: hr=%+v asha=%+v meera=%+v", hrA, asha, meera)
	}
	nobody := f.actor("Nobody", false)
	if nobody.EmployeeID != "" {
		t.Fatal("unlinked user must not have an employee")
	}

	e, err := f.repo.GetEmployee(f.ctx, asha, f.emps["Ravi"])
	if err != nil {
		t.Fatal(err)
	}
	if e.Full || e.PAN != "" || e.BankAccount != "" || e.Phone == "" || e.Name == "" {
		t.Fatalf("peer view leaks or is empty: %+v", e)
	}
	if e, _ = f.repo.GetEmployee(f.ctx, meera, f.emps["Asha"]); e.Full || e.PAN != "" || e.BankAccount != "" {
		t.Fatalf("manager must not see sensitive fields: %+v", e)
	}
	if e, _ = f.repo.GetEmployee(f.ctx, asha, f.emps["Asha"]); !e.Full || e.PAN == "" {
		t.Fatal("employee sees their own full profile")
	}
	if e, _ = f.repo.GetEmployee(f.ctx, hrA, f.emps["Asha"]); !e.Full || e.BankAccount == "" {
		t.Fatal("HR sees the full profile")
	}
	list, _ := f.repo.ListEmployees(f.ctx, ravi, EmployeeFilter{})
	for _, x := range list {
		if x.ID != f.emps["Ravi"] && (x.PAN != "" || x.BankAccount != "" || x.DOB != "") {
			t.Fatalf("directory list leaks: %+v", x)
		}
	}

	_, err = f.repo.SaveEmployee(f.ctx, asha, f.emps["Asha"], Employee{FirstName: "X", JoinDate: "2022-01-01"})
	wantErr(t, "employee edits profile as HR", err, ErrForbidden)
	_, err = f.repo.ListEmployees(f.ctx, asha, EmployeeFilter{Status: "all"})
	wantErr(t, "list exited", err, ErrForbidden)
	wantErr(t, "delete", f.repo.DeleteEmployee(f.ctx, meera, f.emps["Asha"]), ErrForbidden)
	_, err = f.repo.HRDashboard(f.ctx, meera)
	wantErr(t, "hr dashboard", err, ErrForbidden)
	_, err = f.repo.ListRuns(f.ctx, meera)
	wantErr(t, "runs", err, ErrForbidden)
	_, err = f.repo.CreateRun(f.ctx, asha, "2026-05")
	wantErr(t, "create run", err, ErrForbidden)
	_, err = f.repo.ListStructures(f.ctx, asha)
	wantErr(t, "structures", err, ErrForbidden)
	_, err = f.repo.SaveSalary(f.ctx, asha, f.emps["Asha"], EmployeeSalary{})
	wantErr(t, "own salary write", err, ErrForbidden)
	_, err = f.repo.SaveSettings(f.ctx, meera, Settings{})
	wantErr(t, "settings", err, ErrForbidden)
	wantErr(t, "hr admin grant", f.repo.SetHRAdmin(f.ctx, meera, f.users["Meera"], true), ErrForbidden)
	_, err = f.repo.ImportEmployeesCSV(f.ctx, asha, strings.NewReader("first_name,join_date\nX,2020-01-01\n"))
	wantErr(t, "import", err, ErrForbidden)
	wantErr(t, "export", f.repo.ExportEmployeesCSV(f.ctx, asha, io.Discard), ErrForbidden)
	_, err = f.repo.ListHRAdmins(f.ctx, asha)
	wantErr(t, "hr admins", err, ErrForbidden)
	_, err = f.repo.ImportPunchesCSV(f.ctx, meera, strings.NewReader("emp_code,timestamp\n"))
	wantErr(t, "punch import", err, ErrForbidden)
	_, err = f.repo.SaveLookup(f.ctx, asha, "departments", "", Lookup{Name: "x"})
	wantErr(t, "lookup", err, ErrForbidden)

	lines := stdLines(t, 600000)
	for _, n := range []string{"Asha", "Ravi"} {
		if _, err := f.repo.SaveSalary(f.ctx, hrA, f.emps[n], EmployeeSalary{EffectiveFrom: "2026-01-01", Lines: lines, PFEnabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if s, err := f.repo.ListSalaries(f.ctx, asha, f.emps["Asha"]); err != nil || len(s) != 1 {
		t.Fatalf("own salary: %v %v", s, err)
	}
	_, err = f.repo.ListSalaries(f.ctx, asha, f.emps["Ravi"])
	wantErr(t, "peer salary", err, ErrForbidden)
	_, err = f.repo.ListSalaries(f.ctx, meera, f.emps["Asha"])
	wantErr(t, "manager reading a report's salary", err, ErrForbidden)
	_, err = f.repo.ListSalaries(f.ctx, nobody, f.emps["Asha"])
	wantErr(t, "unlinked user", err, ErrForbidden)
	if s, err := f.repo.ListSalaries(f.ctx, hrA, f.emps["Ravi"]); err != nil || len(s) != 1 {
		t.Fatal("HR reads salaries")
	}

	if _, err := f.repo.AddEmergencyContact(f.ctx, asha, f.emps["Asha"], EmergencyContact{Name: "Mum", Phone: "1"}); err != nil {
		t.Fatal(err)
	}
	_, err = f.repo.ListEmergencyContacts(f.ctx, ravi, f.emps["Asha"])
	wantErr(t, "peer emergency contacts", err, ErrForbidden)
	_, err = f.repo.ListEmergencyContacts(f.ctx, meera, f.emps["Asha"])
	wantErr(t, "manager emergency contacts", err, ErrForbidden)

	doc, err := f.repo.AddDocument(f.ctx, hrA, f.emps["Asha"], "offer_letter", "offer.txt", "text/plain", 5, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, rc, err := f.repo.OpenDocument(f.ctx, asha, doc.ID); err != nil {
		t.Fatal(err)
	} else {
		b, _ := io.ReadAll(rc)
		rc.Close()
		if string(b) != "hello" {
			t.Fatalf("doc bytes %q", b)
		}
	}
	for name, a := range map[string]Actor{"peer": ravi, "manager": meera, "unlinked": nobody} {
		if _, _, err := f.repo.OpenDocument(f.ctx, a, doc.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s opened another employee's document: %v", name, err)
		}
		if _, err := f.repo.ListDocuments(f.ctx, a, f.emps["Asha"]); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s listed documents", name)
		}
	}
	wantErr(t, "delete by peer", f.repo.DeleteDocument(f.ctx, ravi, doc.ID), ErrForbidden)
	wantErr(t, "delete HR doc by owner", f.repo.DeleteDocument(f.ctx, asha, doc.ID), ErrForbidden)
	_, err = f.repo.AddDocument(f.ctx, asha, f.emps["Asha"], "offer_letter", "x.txt", "text/plain", 1, strings.NewReader("x"))
	wantErr(t, "employee uploading an offer letter", err, ErrForbidden)
	if _, err := f.repo.AddDocument(f.ctx, asha, f.emps["Asha"], "id_proof", "id.txt", "text/plain", 2, strings.NewReader("id")); err != nil {
		t.Fatalf("employee may upload their own ID proof: %v", err)
	}
	_, err = f.repo.AddDocument(f.ctx, asha, f.emps["Ravi"], "id_proof", "id.txt", "text/plain", 2, strings.NewReader("id"))
	wantErr(t, "upload to a peer", err, ErrForbidden)

	_, err = f.repo.GetTimesheet(f.ctx, ravi, f.emps["Asha"], "2026-06")
	wantErr(t, "peer timesheet", err, ErrForbidden)
	if _, err := f.repo.GetTimesheet(f.ctx, meera, f.emps["Asha"], "2026-06"); err != nil {
		t.Fatalf("manager timesheet: %v", err)
	}
	_, err = f.repo.Balances(f.ctx, ravi, f.emps["Asha"])
	wantErr(t, "peer balances", err, ErrForbidden)
	_, err = f.repo.GetTimesheet(f.ctx, f.actor("Kabir", false), f.emps["Asha"], "2026-06")
	wantErr(t, "other manager's timesheet", err, ErrForbidden)
}

func TestHRAdminGrant(t *testing.T) {
	f := newFixture(t)
	hrA := f.team()
	if err := f.repo.SetHRAdmin(f.ctx, hrA, f.users["Meera"], true); err != nil {
		t.Fatal(err)
	}
	if !f.actor("Meera", false).IsHR {
		t.Fatal("granted user is HR")
	}
	if err := f.repo.SetHRAdmin(f.ctx, hrA, f.users["Meera"], false); err != nil {
		t.Fatal(err)
	}
	if f.actor("Meera", false).IsHR {
		t.Fatal("revoked user is no longer HR")
	}
	wantErr(t, "foreign user", f.repo.SetHRAdmin(f.ctx, hrA, "00000000-0000-0000-0000-000000000000", true), ErrInvalid)
}

func TestEmployeeValidationAndDirectorySync(t *testing.T) {
	f := newFixture(t)
	hrA := f.team()
	_, err := f.repo.SaveEmployee(f.ctx, hrA, "", Employee{FirstName: "", JoinDate: "2026-01-01"})
	wantErr(t, "missing name", err, ErrInvalid)
	_, err = f.repo.SaveEmployee(f.ctx, hrA, "", Employee{FirstName: "A", JoinDate: "01/01/2026"})
	wantErr(t, "bad date", err, ErrInvalid)
	_, err = f.repo.SaveEmployee(f.ctx, hrA, "", Employee{FirstName: "A", JoinDate: "2026-01-01", PAN: "bad"})
	wantErr(t, "bad PAN", err, ErrInvalid)
	_, err = f.repo.SaveEmployee(f.ctx, hrA, "", Employee{FirstName: "A", JoinDate: "2026-01-01", EmploymentType: "slave"})
	wantErr(t, "bad type", err, ErrInvalid)
	_, err = f.repo.SaveEmployee(f.ctx, hrA, "", Employee{FirstName: "A", JoinDate: "2026-01-01", EmpCode: "EMP0001"})
	wantErr(t, "duplicate code", err, ErrConflict)
	_, err = f.repo.SaveEmployee(f.ctx, hrA, "", Employee{FirstName: "Dup", JoinDate: "2026-01-01", UserID: f.users["Asha"]})
	wantErr(t, "user already linked", err, ErrConflict)
	meeraE, _ := f.repo.GetEmployee(f.ctx, hrA, f.emps["Meera"])
	meeraE.ManagerID = f.emps["Asha"]
	_, err = f.repo.SaveEmployee(f.ctx, hrA, f.emps["Meera"], meeraE)
	wantErr(t, "manager loop", err, ErrConflict)
	meeraE.ManagerID = f.emps["Meera"]
	_, err = f.repo.SaveEmployee(f.ctx, hrA, f.emps["Meera"], meeraE)
	wantErr(t, "self manager", err, ErrInvalid)

	dept, err := f.repo.SaveLookup(f.ctx, hrA, "departments", "", Lookup{Name: "Engineering"})
	if err != nil {
		t.Fatal(err)
	}
	des, _ := f.repo.SaveLookup(f.ctx, hrA, "designations", "", Lookup{Name: "Staff Engineer"})
	a, _ := f.repo.GetEmployee(f.ctx, hrA, f.emps["Asha"])
	a.DepartmentID, a.DesignationID = dept.ID, des.ID
	if _, err := f.repo.SaveEmployee(f.ctx, hrA, a.ID, a); err != nil {
		t.Fatal(err)
	}
	var title, mgr, dep string
	if err := f.pool.QueryRow(f.ctx, `SELECT job_title, coalesce(manager_id::text,''), coalesce(department_id::text,'') FROM adrient.users WHERE id=$1::uuid`, f.users["Asha"]).Scan(&title, &mgr, &dep); err != nil {
		t.Fatal(err)
	}
	if title != "Staff Engineer" || mgr != f.users["Meera"] || dep != dept.ID {
		t.Fatalf("directory not synced: %q %q %q", title, mgr, dep)
	}
	chart, err := f.repo.OrgChart(f.ctx, f.org)
	if err != nil || len(chart) != 5 {
		t.Fatalf("orgchart %d %v", len(chart), err)
	}
	if err := f.repo.DeleteEmployee(f.ctx, hrA, f.emps["Kabir"]); err != nil {
		t.Fatalf("delete clean employee: %v", err)
	}
}

func TestCSVImportExport(t *testing.T) {
	f := newFixture(t)
	hrA := f.team()
	csvIn := "emp_code,first_name,last_name,work_email,join_date,department,designation,location,manager_code,employment_type,dob\n" +
		"IMP001,Tara,Singh,tara@example.test,2025-04-01,Sales,Account Manager,Pune,EMP0001,full_time,1994-05-17\n" +
		"IMP002,Uday,Jain,,2025-05-01,Sales,Account Manager,Pune,IMP001,contract,\n" +
		",,,,,,,,,,\n" +
		"IMP003,,Bad,,2025-05-01,,,,,,\n" +
		"IMP004,Vik,Rao,,not-a-date,,,,,,\n" +
		"IMP005,Wen,Lee,,2025-05-01,,,,NOPE,,\n"
	res, err := f.repo.ImportEmployeesCSV(f.ctx, hrA, strings.NewReader(csvIn))
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 3 || len(res.Errors) != 3 {
		t.Fatalf("import result %+v", res)
	}
	tara, _ := f.repo.ListEmployees(f.ctx, hrA, EmployeeFilter{Q: "tara"})
	if len(tara) != 1 || tara[0].DepartmentName != "Sales" || tara[0].LocationName != "Pune" || tara[0].ManagerName != "Hana Test" {
		t.Fatalf("tara %+v", tara)
	}
	uday, _ := f.repo.ListEmployees(f.ctx, hrA, EmployeeFilter{Q: "uday"})
	if len(uday) != 1 || uday[0].ManagerName != "Tara Singh" || uday[0].EmploymentType != "contract" {
		t.Fatalf("uday %+v", uday)
	}
	res, _ = f.repo.ImportEmployeesCSV(f.ctx, hrA, strings.NewReader("emp_code,first_name,join_date\nIMP001,Tara,2025-04-01\n"))
	if res.Updated != 1 || res.Created != 0 {
		t.Fatalf("re-import %+v", res)
	}
	var buf bytes.Buffer
	if err := f.repo.ExportEmployeesCSV(f.ctx, hrA, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "IMP001") || !strings.Contains(buf.String(), "emp_code,first_name") {
		t.Fatalf("export: %s", buf.String())
	}
	if _, err := f.repo.ImportEmployeesCSV(f.ctx, hrA, strings.NewReader("name,joined\nx,y\n")); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing columns must be rejected")
	}
}
