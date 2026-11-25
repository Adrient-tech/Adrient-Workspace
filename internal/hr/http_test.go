package hr

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func (f *fixture) server() http.Handler {
	return NewHTTPHandler(f.repo, AuthFuncs{
		UserID:  func(r *http.Request) (string, bool) { u := r.Header.Get("X-Test-User"); return f.users[u], u != "" },
		OrgID:   func(r *http.Request) (string, bool) { return f.org, true },
		IsAdmin: func(r *http.Request) bool { return r.Header.Get("X-Test-Admin") == "1" },
	})
}

func do(h http.Handler, user string, admin bool, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Test-User", user)
	if admin {
		req.Header.Set("X-Test-Admin", "1")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHTTPEnforcesRoles(t *testing.T) {
	f := newFixture(t)
	f.team()
	h := f.server()
	hrA := f.actor("Hana", true)
	if _, err := f.repo.SaveSalary(f.ctx, hrA, f.emps["Asha"], EmployeeSalary{EffectiveFrom: "2026-01-01", Lines: stdLines(t, 600000)}); err != nil {
		t.Fatal(err)
	}
	run, _ := f.repo.CreateRun(f.ctx, hrA, "2026-05")
	slips, _ := f.repo.RunPayslips(f.ctx, hrA, run.ID)
	asha := f.emps["Asha"]
	slip := "/api/v1/hr/payroll/payslips/" + slips[0].ID

	cases := []struct {
		name         string
		user         string
		admin        bool
		method, path string
		body         string
		want         int
	}{
		{"unauthenticated", "", false, "GET", "/api/v1/hr/me", "", 401},
		{"my home", "Asha", false, "GET", "/api/v1/hr/me", "", 200},
		{"peer salary", "Ravi", false, "GET", "/api/v1/hr/employees/" + asha + "/salary", "", 403},
		{"manager salary", "Meera", false, "GET", "/api/v1/hr/employees/" + asha + "/salary", "", 403},
		{"own salary", "Asha", false, "GET", "/api/v1/hr/employees/" + asha + "/salary", "", 200},
		{"hr salary", "Hana", true, "GET", "/api/v1/hr/employees/" + asha + "/salary", "", 200},
		{"set salary as employee", "Asha", false, "POST", "/api/v1/hr/employees/" + asha + "/salary", `{"effective_from":"2026-01-01"}`, 403},
		{"payroll runs as manager", "Meera", false, "GET", "/api/v1/hr/payroll/runs", "", 403},
		{"payroll runs as hr", "Hana", true, "GET", "/api/v1/hr/payroll/runs", "", 200},
		{"create run as employee", "Asha", false, "POST", "/api/v1/hr/payroll/runs", `{"period":"2026-04"}`, 403},
		{"draft payslip by owner", "Asha", false, "GET", slip, "", 404},
		{"payslip pdf by peer", "Ravi", false, "GET", slip + "/pdf", "", 404},
		{"payslip pdf by hr", "Hana", true, "GET", slip + "/pdf", "", 200},
		{"structures as employee", "Asha", false, "GET", "/api/v1/hr/payroll/structures", "", 403},
		{"pt slabs as employee", "Asha", false, "PUT", "/api/v1/hr/payroll/pt-slabs", `{"slabs":[]}`, 403},
		{"export as employee", "Asha", false, "GET", "/api/v1/hr/employees/export.csv", "", 403},
		{"export as hr", "Hana", true, "GET", "/api/v1/hr/employees/export.csv", "", 200},
		{"create employee as manager", "Meera", false, "POST", "/api/v1/hr/employees", `{"first_name":"X","join_date":"2026-01-01"}`, 403},
		{"dashboard as employee", "Asha", false, "GET", "/api/v1/hr/dashboard", "", 403},
		{"manager dashboard as employee", "Asha", false, "GET", "/api/v1/hr/dashboard/manager", "", 403},
		{"manager dashboard as manager", "Meera", false, "GET", "/api/v1/hr/dashboard/manager", "", 200},
		{"peer documents", "Ravi", false, "GET", "/api/v1/hr/employees/" + asha + "/documents", "", 403},
		{"peer timesheet", "Ravi", false, "GET", "/api/v1/hr/attendance/timesheet?employee_id=" + asha + "&month=2026-06", "", 403},
		{"manager timesheet", "Meera", false, "GET", "/api/v1/hr/attendance/timesheet?employee_id=" + asha + "&month=2026-06", "", 200},
		{"peer balances", "Ravi", false, "GET", "/api/v1/hr/employees/" + asha + "/balances", "", 403},
		{"directory", "Ravi", false, "GET", "/api/v1/hr/employees?q=asha", "", 200},
		{"former employees", "Ravi", false, "GET", "/api/v1/hr/employees?status=all", "", 403},
		{"punch import by manager", "Meera", false, "POST", "/api/v1/hr/attendance/import", "emp_code,timestamp\n", 403},
		{"accrual run by manager", "Meera", false, "POST", "/api/v1/hr/leave/accrual/run", "", 403},
		{"settings write by employee", "Asha", false, "PUT", "/api/v1/hr/settings", `{}`, 403},
		{"settings read by employee", "Asha", false, "GET", "/api/v1/hr/settings", "", 200},
		{"leave type write by employee", "Asha", false, "POST", "/api/v1/hr/leave-types", `{"code":"X","name":"X","accrual":"none"}`, 403},
		{"bad json", "Hana", true, "POST", "/api/v1/hr/employees", `{`, 400},
		{"unknown path", "Hana", true, "GET", "/api/v1/hr/nope", "", 404},
		{"org chart open to members", "Ravi", false, "GET", "/api/v1/hr/orgchart", "", 200},
	}
	for _, tc := range cases {
		rec := do(h, tc.user, tc.admin, tc.method, tc.path, tc.body)
		if rec.Code != tc.want {
			t.Errorf("%s: %s %s as %q = %d, want %d (%s)", tc.name, tc.method, tc.path, tc.user, rec.Code, tc.want, strings.TrimSpace(rec.Body.String()))
		}
	}
	for _, u := range []string{"Ravi", "Meera"} {
		body := do(h, u, false, "GET", "/api/v1/hr/employees/"+asha, "").Body.String()
		for _, secret := range []string{"ABCDE1234F", "9999Asha", "HDFC0001234", "annual_gross"} {
			if strings.Contains(body, secret) {
				t.Errorf("%s's view of an employee leaked %q", u, secret)
			}
		}
	}
	rec := do(h, "Hana", true, "GET", slip+"/pdf", "")
	if rec.Header().Get("Content-Type") != "application/pdf" || !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF")) {
		t.Errorf("pdf response headers %v", rec.Header())
	}
}
