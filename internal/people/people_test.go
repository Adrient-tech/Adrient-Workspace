package people

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

type fixture struct {
	pool  *pgxpool.Pool
	store *Store
	org   string
	ids   map[string]string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := testguard.DropSchema(ctx, t, pool); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	f := &fixture{pool: pool, store: NewStore(pool), ids: map[string]string{}}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&f.org); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"ceo", "vp", "lead", "dev", "other"} {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
			VALUES ($1,'https://issuer.test',$2,$2||'@x.test',$2) RETURNING id::text`, f.org, n).Scan(&id); err != nil {
			t.Fatal(err)
		}
		f.ids[n] = id
	}
	return f
}

func (f *fixture) setManager(t *testing.T, user, mgr string) {
	t.Helper()
	m := f.ids[mgr]
	if _, err := f.store.UpdateUser(context.Background(), f.org, f.ids[user], Patch{ManagerID: &m}); err != nil {
		t.Fatal(err)
	}
}

func names(ps []Person) string {
	var n []string
	for _, p := range ps {
		n = append(n, p.Name)
	}
	return strings.Join(n, ",")
}

func TestManagerChainAndReports(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.setManager(t, "dev", "lead")
	f.setManager(t, "lead", "vp")
	f.setManager(t, "vp", "ceo")

	chain, err := f.store.ManagerChain(ctx, f.org, f.ids["dev"])
	if err != nil {
		t.Fatal(err)
	}
	if got := names(chain); got != "lead,vp,ceo" {
		t.Fatalf("chain = %s", got)
	}
	if c, _ := f.store.ManagerChain(ctx, f.org, f.ids["ceo"]); len(c) != 0 {
		t.Fatalf("ceo chain should be empty: %v", c)
	}
	reports, _ := f.store.DirectReports(ctx, f.org, f.ids["lead"])
	if names(reports) != "dev" {
		t.Fatalf("reports = %s", names(reports))
	}
	// Loop is rejected, self-manager is rejected.
	ceo := f.ids["dev"]
	if _, err := f.store.UpdateUser(ctx, f.org, f.ids["ceo"], Patch{ManagerID: &ceo}); !errors.Is(err, ErrCycle) {
		t.Fatalf("want ErrCycle, got %v", err)
	}
	self := f.ids["dev"]
	if _, err := f.store.UpdateUser(ctx, f.org, f.ids["dev"], Patch{ManagerID: &self}); !errors.Is(err, ErrCycle) {
		t.Fatalf("self manager: %v", err)
	}
	// Corrupt data (a raw loop) must not hang the chain query.
	if _, err := f.pool.Exec(ctx, `UPDATE adrient.users SET manager_id=$1 WHERE id=$2`, f.ids["dev"], f.ids["ceo"]); err != nil {
		t.Fatal(err)
	}
	chain, err = f.store.ManagerChain(ctx, f.org, f.ids["dev"])
	if err != nil || len(chain) != 3 {
		t.Fatalf("cyclic chain: %v %v", names(chain), err)
	}
	// Clearing a manager.
	empty := ""
	p, err := f.store.UpdateUser(ctx, f.org, f.ids["dev"], Patch{ManagerID: &empty})
	if err != nil || p.ManagerID != "" {
		t.Fatalf("clear manager: %+v %v", p, err)
	}
}

func TestDepartmentSubtree(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	eng, err := f.store.CreateDepartment(ctx, f.org, "Engineering", "", f.ids["vp"])
	if err != nil {
		t.Fatal(err)
	}
	be, _ := f.store.CreateDepartment(ctx, f.org, "Backend", eng.ID, "")
	fe, _ := f.store.CreateDepartment(ctx, f.org, "Frontend", eng.ID, "")
	infra, _ := f.store.CreateDepartment(ctx, f.org, "Infra", be.ID, "")
	sales, _ := f.store.CreateDepartment(ctx, f.org, "Sales", "", "")
	if _, err := f.store.CreateDepartment(ctx, f.org, "backend", eng.ID, ""); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("dup sibling: %v", err)
	}
	for u, d := range map[string]string{"dev": infra.ID, "lead": be.ID, "vp": eng.ID, "other": sales.ID, "ceo": fe.ID} {
		d := d
		if _, err := f.store.UpdateUser(ctx, f.org, f.ids[u], Patch{DepartmentID: &d}); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := f.store.DepartmentSubtreeIDs(ctx, f.org, eng.ID)
	if err != nil || len(ids) != 4 {
		t.Fatalf("subtree ids = %v %v", ids, err)
	}
	ids, _ = f.store.DepartmentSubtreeIDs(ctx, f.org, be.ID)
	if len(ids) != 2 {
		t.Fatalf("backend subtree = %v", ids)
	}
	ps, _ := f.store.PeopleInDepartmentSubtree(ctx, f.org, eng.ID)
	if names(ps) != "ceo,dev,lead,vp" {
		t.Fatalf("eng people = %s", names(ps))
	}
	ps, _ = f.store.PeopleInDepartmentSubtree(ctx, f.org, sales.ID)
	if names(ps) != "other" {
		t.Fatalf("sales people = %s", names(ps))
	}
	// Moving Engineering under Infra (its own descendant) is a cycle.
	if _, err := f.store.UpdateDepartment(ctx, f.org, eng.ID, DepartmentPatch{ParentID: &infra.ID}); !errors.Is(err, ErrCycle) {
		t.Fatalf("dept cycle: %v", err)
	}
	// Re-parent Sales under Engineering.
	if _, err := f.store.UpdateDepartment(ctx, f.org, sales.ID, DepartmentPatch{ParentID: &eng.ID}); err != nil {
		t.Fatal(err)
	}
	ids, _ = f.store.DepartmentSubtreeIDs(ctx, f.org, eng.ID)
	if len(ids) != 5 {
		t.Fatalf("after reparent = %d", len(ids))
	}
	// Delete the middle node: children become roots, members lose the dept.
	if err := f.store.DeleteDepartment(ctx, f.org, be.ID); err != nil {
		t.Fatal(err)
	}
	if d, _ := f.store.GetDepartment(ctx, f.org, infra.ID); d.ParentID != "" {
		t.Fatalf("infra should be root: %+v", d)
	}
	if p, _ := f.store.Get(ctx, f.org, f.ids["lead"]); p.DepartmentID != "" {
		t.Fatal("lead should have no department")
	}
}

func TestGroupMembersBackfillAndSync(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var gid string
	if err := f.pool.QueryRow(ctx, `INSERT INTO adrient.groups (org_id, owner_id, name, member_ids)
		VALUES ($1,$2,'G',jsonb_build_array($3::text,$4::text,'not-a-uuid')) RETURNING id::text`,
		f.org, f.ids["ceo"], f.ids["dev"], f.ids["lead"]).Scan(&gid); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		var n int
		_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.group_members WHERE group_id=$1`, gid).Scan(&n)
		return n
	}
	if count() != 2 {
		t.Fatalf("insert sync = %d", count())
	}
	if _, err := f.pool.Exec(ctx, `UPDATE adrient.groups SET member_ids=jsonb_build_array($2::text) WHERE id=$1`, gid, f.ids["vp"]); err != nil {
		t.Fatal(err)
	}
	if count() != 1 {
		t.Fatalf("update sync = %d", count())
	}
	// The migration's backfill statement is idempotent over existing rows.
	if _, err := f.pool.Exec(ctx, `INSERT INTO adrient.group_members (group_id, user_id, org_id)
		SELECT g.id, u.id, g.org_id FROM adrient.groups g CROSS JOIN LATERAL jsonb_array_elements_text(g.member_ids) m
		JOIN adrient.users u ON u.id = m::uuid ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if count() != 1 {
		t.Fatalf("backfill = %d", count())
	}
}

func apiFor(f *fixture, caller string, admin bool) *Handler {
	return NewHandler(f.store, AuthFuncs{
		OrgID:   func(*http.Request) (string, bool) { return f.org, true },
		UserID:  func(*http.Request) (string, bool) { return f.ids[caller], true },
		IsAdmin: func(*http.Request) bool { return admin },
	})
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAPI(t *testing.T) {
	f := setup(t)
	admin := apiFor(f, "ceo", true)
	member := apiFor(f, "dev", false)

	// Admin builds a department and sets dev's title/department/manager.
	rec := do(admin, "POST", "/api/v1/departments", `{"name":"Eng"}`)
	if rec.Code != 201 {
		t.Fatalf("create dept: %d %s", rec.Code, rec.Body)
	}
	var dep struct{ Department Department }
	_ = json.Unmarshal(rec.Body.Bytes(), &dep)
	if rec := do(admin, "POST", "/api/v1/departments", `{"name":"eng"}`); rec.Code != 409 {
		t.Fatalf("dup dept = %d", rec.Code)
	}
	body := `{"job_title":"Engineer","department_id":"` + dep.Department.ID + `","manager_id":"` + f.ids["lead"] + `","start_date":"2024-03-01"}`
	if rec := do(admin, "PATCH", "/api/v1/people/"+f.ids["dev"], body); rec.Code != 200 {
		t.Fatalf("admin patch: %d %s", rec.Code, rec.Body)
	}
	do(admin, "PATCH", "/api/v1/people/"+f.ids["lead"], `{"manager_id":"`+f.ids["vp"]+`"}`)

	rec = do(member, "GET", "/api/v1/people/"+f.ids["dev"], "")
	if rec.Code != 200 {
		t.Fatalf("get person: %d", rec.Code)
	}
	var got struct {
		Person     Person   `json:"person"`
		Manager    Person   `json:"manager"`
		Chain      []Person `json:"manager_chain"`
		Reports    []Person `json:"direct_reports"`
		Department Department
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Person.JobTitle != "Engineer" || got.Manager.Name != "lead" || len(got.Chain) != 2 ||
		got.Department.Name != "Eng" || got.Person.StartDate != "2024-03-01" {
		t.Fatalf("profile = %+v", got)
	}
	rec = do(member, "GET", "/api/v1/people/"+f.ids["lead"], "")
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Reports) != 1 || got.Reports[0].Name != "dev" {
		t.Fatalf("reports = %+v", got.Reports)
	}

	// Search.
	rec = do(member, "GET", "/api/v1/people?query=ENGIN", "")
	var sr struct{ People []Person }
	_ = json.Unmarshal(rec.Body.Bytes(), &sr)
	if len(sr.People) != 1 || sr.People[0].Name != "dev" {
		t.Fatalf("search = %+v", sr.People)
	}
	rec = do(member, "GET", "/api/v1/people?query=%25", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &sr)
	if len(sr.People) != 0 {
		t.Fatalf("wildcard must be escaped: %d", len(sr.People))
	}

	// Member: own phone/location OK; title, others, departments are forbidden.
	if rec := do(member, "PATCH", "/api/v1/people/"+f.ids["dev"], `{"phone":"+1 555","location":"Pune"}`); rec.Code != 200 {
		t.Fatalf("self phone: %d %s", rec.Code, rec.Body)
	}
	if rec := do(member, "PATCH", "/api/v1/people/"+f.ids["dev"], `{"job_title":"CTO"}`); rec.Code != 403 {
		t.Fatalf("self title = %d", rec.Code)
	}
	if rec := do(member, "PATCH", "/api/v1/people/"+f.ids["dev"], `{"manager_id":""}`); rec.Code != 403 {
		t.Fatalf("self manager = %d", rec.Code)
	}
	if rec := do(member, "PATCH", "/api/v1/people/"+f.ids["lead"], `{"phone":"1"}`); rec.Code != 403 {
		t.Fatalf("other phone = %d", rec.Code)
	}
	if rec := do(member, "POST", "/api/v1/departments", `{"name":"X"}`); rec.Code != 403 {
		t.Fatalf("member dept create = %d", rec.Code)
	}
	if rec := do(member, "DELETE", "/api/v1/departments/"+dep.Department.ID, ""); rec.Code != 403 {
		t.Fatalf("member dept delete = %d", rec.Code)
	}
	// Cycle and bad refs.
	if rec := do(admin, "PATCH", "/api/v1/people/"+f.ids["vp"], `{"manager_id":"`+f.ids["dev"]+`"}`); rec.Code != 409 {
		t.Fatalf("cycle = %d", rec.Code)
	}
	if rec := do(admin, "PATCH", "/api/v1/people/"+f.ids["vp"], `{"department_id":"nope"}`); rec.Code != 400 {
		t.Fatalf("bad dept = %d", rec.Code)
	}
	if rec := do(admin, "GET", "/api/v1/people/00000000-0000-0000-0000-000000000000", ""); rec.Code != 404 {
		t.Fatalf("missing = %d", rec.Code)
	}
	// Org chart payload.
	rec = do(member, "GET", "/api/v1/people/orgchart", "")
	var oc struct {
		People      []Person
		Departments []Department
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &oc)
	if rec.Code != 200 || len(oc.People) != 5 || len(oc.Departments) != 1 || oc.Departments[0].MemberCount != 1 {
		t.Fatalf("orgchart: %d %+v", rec.Code, oc)
	}
	// Admin deletes the department.
	if rec := do(admin, "DELETE", "/api/v1/departments/"+dep.Department.ID, ""); rec.Code != 200 {
		t.Fatalf("delete dept = %d", rec.Code)
	}
}

func TestMatch(t *testing.T) {
	for p, want := range map[string]bool{
		"/api/v1/people": true, "/api/v1/people/x": true, "/api/v1/departments": true,
		"/api/v1/peoplex": false, "/api/v1/directory": false,
	} {
		if Match(p) != want {
			t.Errorf("Match(%q) != %v", p, want)
		}
	}
}
