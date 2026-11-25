package authz_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

// TestAppAccessPG runs the real tables: department subtree, group membership,
// policy editing through the admin API, 403 on a blocked app's API, App admin
// role assignment and the audited changes.
func TestAppAccessPG(t *testing.T) {
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
	var orgID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	admin := seedUser(t, pool, orgID, "adm")
	member := seedUser(t, pool, orgID, "mem")
	finUser := seedUser(t, pool, orgID, "fin")
	apUser := seedUser(t, pool, orgID, "ap")
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.org_admins (org_id,user_id) VALUES ($1,$2)`, orgID, admin); err != nil {
		t.Fatal(err)
	}
	mkDept := func(name, parent string) string {
		var id string
		var p any
		if parent != "" {
			p = parent
		}
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.departments (org_id,parent_id,name) VALUES ($1,$2::uuid,$3) RETURNING id::text`, orgID, p, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	fin := mkDept("Finance", "")
	ap := mkDept("Accounts payable", fin)
	ops := mkDept("Operations", "")
	setDept := func(user, dept string) {
		if _, err := pool.Exec(ctx, `UPDATE adrient.users SET department_id=$2::uuid WHERE id=$1::uuid`, user, dept); err != nil {
			t.Fatal(err)
		}
	}
	setDept(finUser, fin)
	setDept(apUser, ap)
	setDept(member, ops)

	roles := authz.NewService(authz.PGLoader{Pool: pool})
	authz.SetDefault(roles)
	apps := authz.NewAppService(authz.PGAppStore{Pool: pool}, roles)
	authz.SetDefaultApps(apps)
	t.Cleanup(func() { authz.SetDefaultApps(nil) })
	var audited []string
	h := &authz.AdminHTTP{Pool: pool, Roles: roles, Apps: apps,
		Audit: func(_ context.Context, service, action, _, _ string, _ map[string]any, _ *http.Request) {
			audited = append(audited, service+":"+action)
		}}
	var denied []authz.Denial
	authz.SetDenyHook(func(_ context.Context, d authz.Denial) { denied = append(denied, d) })
	t.Cleanup(func() { authz.SetDenyHook(nil) })

	cctx := func(id string) context.Context {
		c := auth.WithUser(ctx, users.User{ID: id, Email: id + "@x.test"})
		return auth.WithOrg(c, orgs.Org{ID: orgID})
	}
	call := func(user, method, path string, body any) *httptest.ResponseRecorder {
		var rd *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		} else {
			rd = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, rd).WithContext(cctx(user))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	guarded := authz.Guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	api := func(user, path string) int {
		rec := httptest.NewRecorder()
		guarded.ServeHTTP(rec, httptest.NewRequest("GET", path, nil).WithContext(cctx(user)))
		return rec.Code
	}

	// Non-admin cannot edit policies.
	if c := call(member, "PUT", "/api/v1/admin/app-access/accounting", map[string]any{"policies": []any{}}).Code; c != 403 {
		t.Fatalf("member edit policies: %d", c)
	}
	// Admin: Accounting -> Finance subtree only, Inventory -> Operations.
	put := func(app, typ, id string) {
		rec := call(admin, "PUT", "/api/v1/admin/app-access/"+app, map[string]any{"policies": []map[string]string{{"subject_type": typ, "subject_id": id, "effect": "allow"}}})
		if rec.Code != 200 {
			t.Fatalf("put %s: %d %s", app, rec.Code, rec.Body.String())
		}
	}
	put("accounting", "department", fin)
	put("inventory", "department", ops)

	if c := api(member, "/api/v1/accounting/accounts"); c != 403 {
		t.Fatalf("member (Ops) -> accounting: %d want 403", c)
	}
	if len(denied) != 1 || denied[0].Permission != "accounting:app:open" {
		t.Fatalf("denial not audited: %+v", denied)
	}
	if c := api(member, "/api/v1/inventory/products"); c != 204 {
		t.Fatalf("member -> inventory: %d", c)
	}
	if c := api(finUser, "/api/v1/accounting/accounts"); c == 403 {
		// finUser lacks accounting:ledger:read (member role); that is the R2 403,
		// not the app 403: it must not be reported as an app denial.
		if last := denied[len(denied)-1]; last.Permission == "accounting:app:open" {
			t.Fatal("finance user blocked at app level")
		}
	}
	// Department subtree: AP is a child of Finance and gets Accounting too.
	if c := api(apUser, "/api/v1/accounting/accounts"); c == 403 && denied[len(denied)-1].Permission == "accounting:app:open" {
		t.Fatal("child department must inherit Finance access")
	}
	if c := api(admin, "/api/v1/accounting/accounts"); c != 204 {
		t.Fatalf("admin: %d", c)
	}

	// /me/apps for the member lists accounting as denied.
	rec := call(member, "GET", "/api/v1/me/apps", nil)
	var me struct{ Allowed, Denied []string }
	_ = json.Unmarshal(rec.Body.Bytes(), &me)
	if rec.Code != 200 || !has(me.Denied, "accounting") || has(me.Denied, "inventory") || !has(me.Allowed, "inventory") {
		t.Fatalf("me/apps: %d %s", rec.Code, rec.Body.String())
	}

	// Assign the member 'App admin (accounting)': Accounting opens and ledger reads pass.
	var assignment struct{ ID string }
	rec = call(admin, "POST", "/api/v1/admin/roles/assignments", map[string]any{"subject_type": "user", "subject_id": member, "role_key": "app_admin", "scope_app": "accounting"})
	if rec.Code != 200 {
		t.Fatalf("assign: %d %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &assignment)
	roles.Invalidate()
	if c := api(member, "/api/v1/accounting/accounts"); c != 204 {
		t.Fatalf("app admin must open accounting: %d", c)
	}
	// app_admin without scope is rejected; non-admin cannot assign.
	if c := call(admin, "POST", "/api/v1/admin/roles/assignments", map[string]any{"subject_type": "user", "subject_id": member, "role_key": "app_admin"}).Code; c != 400 {
		t.Fatalf("unscoped app_admin: %d", c)
	}
	if c := call(member, "POST", "/api/v1/admin/roles/assignments", map[string]any{"subject_type": "user", "subject_id": member, "role_key": "admin"}).Code; c != 403 {
		t.Fatalf("self-promotion: %d", c)
	}
	// Preview as another user.
	rec = call(admin, "GET", "/api/v1/admin/app-access/preview?user_id="+finUser, nil)
	var pv struct{ Allowed, Denied []string }
	_ = json.Unmarshal(rec.Body.Bytes(), &pv)
	if rec.Code != 200 || !has(pv.Allowed, "accounting") || !has(pv.Denied, "inventory") {
		t.Fatalf("preview fin user: %d %s", rec.Code, rec.Body.String())
	}
	// Revoke -> back to blocked.
	if c := call(admin, "DELETE", "/api/v1/admin/roles/assignments/"+assignment.ID, nil).Code; c != 200 {
		t.Fatalf("revoke: %d", c)
	}
	roles.Invalidate()
	if c := api(member, "/api/v1/accounting/accounts"); c != 403 {
		t.Fatalf("after revoke: %d", c)
	}
	// Deny wins even for a user inside the allowed department.
	if rec := call(admin, "PUT", "/api/v1/admin/app-access/accounting", map[string]any{"policies": []map[string]string{
		{"subject_type": "department", "subject_id": fin, "effect": "allow"},
		{"subject_type": "user", "subject_id": apUser, "effect": "deny"},
	}}); rec.Code != 200 {
		t.Fatalf("put deny: %d", rec.Code)
	}
	if c := api(apUser, "/api/v1/accounting/accounts"); c != 403 || denied[len(denied)-1].Permission != "accounting:app:open" {
		t.Fatalf("deny must win: %d", c)
	}
	// Clearing the policies reopens the app.
	call(admin, "PUT", "/api/v1/admin/app-access/accounting", map[string]any{"policies": []any{}})
	rec = call(apUser, "GET", "/api/v1/me/apps", nil)
	me.Allowed, me.Denied = nil, nil
	_ = json.Unmarshal(rec.Body.Bytes(), &me)
	if has(me.Denied, "accounting") {
		t.Fatalf("cleared policy must reopen accounting: %s", rec.Body.String())
	}
	want := map[string]bool{"app-access:update": true, "roles:assign": true, "roles:revoke": true}
	for _, a := range audited {
		delete(want, a)
	}
	if len(want) != 0 {
		t.Fatalf("missing audit events %v (got %v)", want, audited)
	}
}

func has(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}
