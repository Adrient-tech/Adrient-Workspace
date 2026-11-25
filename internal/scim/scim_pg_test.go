package scim_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/scim"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

func TestParsePatch(t *testing.T) {
	// Entra sends booleans as strings and uses paths.
	req := scim.PatchRequest{Operations: []scim.PatchOp{
		{Op: "Replace", Path: "active", Value: json.RawMessage(`"False"`)},
		{Op: "Replace", Path: "name.givenName", Value: json.RawMessage(`"Ann"`)},
		{Op: "Replace", Path: `emails[type eq "work"].value`, Value: json.RawMessage(`"ann@x.test"`)},
	}}
	p, err := scim.ParseUserPatch(req)
	if err != nil || p.Active == nil || *p.Active || p.Given == nil || *p.Given != "Ann" || p.Email == nil || *p.Email != "ann@x.test" {
		t.Fatalf("%+v %v", p, err)
	}
	// Okta sends no path and a JSON bool.
	p, err = scim.ParseUserPatch(scim.PatchRequest{Operations: []scim.PatchOp{{Op: "replace", Value: json.RawMessage(`{"active":false,"displayName":"A B"}`)}}})
	if err != nil || p.Active == nil || *p.Active || *p.DisplayName != "A B" {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := scim.ParseUserPatch(scim.PatchRequest{Operations: []scim.PatchOp{{Op: "move", Path: "x"}}}); err == nil {
		t.Fatal("unknown op accepted")
	}
	if _, err := scim.ParseFilter(`userName eq "a@b.test" and active eq true`); err == nil {
		t.Fatal("compound filters are not supported and must say so")
	}
	f, err := scim.ParseFilter(`userName eq "a@b.test"`)
	if err != nil || !f.Match("A@B.test") {
		t.Fatalf("%v %v", f, err)
	}
}

// do sends one SCIM request and decodes the JSON answer.
func do(t *testing.T, h http.Handler, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/scim+json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestSCIMServerPG(t *testing.T) {
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
	var orgA, orgB string
	_ = pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&orgA)
	_ = pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ('b','Org B') RETURNING id::text`).Scan(&orgB)
	var owner string
	_ = pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'i','owner','owner@a.test','Owner') RETURNING id::text`, orgA).Scan(&owner)

	st := &scim.Store{Pool: pool}
	var audited []string
	srv := &scim.Server{Store: st,
		SeatCheck: func(ctx context.Context, org string) error {
			var n int
			_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.users WHERE org_id::text=$1 AND status='active' AND user_type='member'`, org).Scan(&n)
			if n >= 3 { // plan: 3 seats
				return http_err("no seats left")
			}
			return nil
		},
		Audit: func(_ context.Context, _ string, action string, _ map[string]any) { audited = append(audited, action) }}
	tokA, _, err := st.CreateToken(ctx, orgA, owner, "entra", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	tokB, _, _ := st.CreateToken(ctx, orgB, "", "other", 0, nil)

	// Auth.
	if c, _ := do(t, srv, "GET", "/scim/v2/Users", "", nil); c != 401 {
		t.Fatalf("no token: %d", c)
	}
	if c, _ := do(t, srv, "GET", "/scim/v2/Users", "scim_deadbeef", nil); c != 401 {
		t.Fatalf("bad token: %d", c)
	}
	if c, b := do(t, srv, "GET", "/scim/v2/ServiceProviderConfig", tokA, nil); c != 200 || b["patch"] == nil {
		t.Fatalf("spc %d %v", c, b)
	}

	// Create.
	c, u := do(t, srv, "POST", "/scim/v2/Users", tokA, map[string]any{
		"schemas": []string{scim.SchemaUser}, "userName": "Ann.Lee@A.test", "externalId": "ext-1",
		"name":   map[string]any{"givenName": "Ann", "familyName": "Lee"},
		"emails": []map[string]any{{"value": "ann.lee@a.test", "primary": true}}, "active": true, "title": "Engineer",
	})
	if c != 201 || u["userName"] != "ann.lee@a.test" || u["displayName"] != "Ann Lee" {
		t.Fatalf("create: %d %v", c, u)
	}
	id := u["id"].(string)
	if c, _ := do(t, srv, "POST", "/scim/v2/Users", tokA, map[string]any{"userName": "ann.lee@a.test"}); c != 409 {
		t.Fatalf("duplicate must be 409, got %d", c)
	}
	if c, _ := do(t, srv, "POST", "/scim/v2/Users", tokA, map[string]any{"userName": "not-an-email"}); c != 400 {
		t.Fatalf("bad userName must be 400, got %d", c)
	}

	// Filter + get + org isolation.
	c, l := do(t, srv, "GET", `/scim/v2/Users?filter=userName%20eq%20%22ANN.lee@a.test%22`, tokA, nil)
	if c != 200 || l["totalResults"].(float64) != 1 {
		t.Fatalf("filter: %d %v", c, l)
	}
	if c, l = do(t, srv, "GET", "/scim/v2/Users", tokB, nil); c != 200 || l["totalResults"].(float64) != 0 {
		t.Fatalf("org B must not see org A users: %v", l)
	}
	if c, _ := do(t, srv, "GET", "/scim/v2/Users/"+id, tokB, nil); c != 404 {
		t.Fatalf("cross-org get must be 404, got %d", c)
	}
	if c, _ := do(t, srv, "PATCH", "/scim/v2/Users/"+id, tokB, map[string]any{"Operations": []any{}}); c != 404 {
		t.Fatalf("cross-org patch must be 404, got %d", c)
	}

	// Patch: Entra style deactivate, then Okta style reactivate.
	c, u = do(t, srv, "PATCH", "/scim/v2/Users/"+id, tokA, map[string]any{"schemas": []string{scim.SchemaPatch},
		"Operations": []map[string]any{{"op": "Replace", "path": "active", "value": "False"}}})
	if c != 200 || u["active"] != false {
		t.Fatalf("deactivate: %d %v", c, u)
	}
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM adrient.users WHERE id::text=$1`, id).Scan(&status)
	if status != "blocked" {
		t.Fatalf("user status %q after active=false", status)
	}
	c, u = do(t, srv, "PATCH", "/scim/v2/Users/"+id, tokA, map[string]any{"Operations": []map[string]any{
		{"op": "replace", "value": map[string]any{"active": true, "title": "Staff Engineer"}}}})
	if c != 200 || u["active"] != true || u["title"] != "Staff Engineer" {
		t.Fatalf("reactivate: %d %v", c, u)
	}
	// PUT replaces.
	c, u = do(t, srv, "PUT", "/scim/v2/Users/"+id, tokA, map[string]any{"userName": "ann.lee@a.test", "displayName": "Ann Q Lee", "active": true})
	if c != 200 || u["displayName"] != "Ann Q Lee" {
		t.Fatalf("put: %d %v", c, u)
	}

	// Seats: one used (Ann), one left, then none.
	if c, _ := do(t, srv, "POST", "/scim/v2/Users", tokA, map[string]any{"userName": "bob@a.test"}); c != 201 {
		t.Fatalf("second user: %d", c)
	}
	if c, _ := do(t, srv, "POST", "/scim/v2/Users", tokA, map[string]any{"userName": "cy@a.test"}); c != 403 {
		t.Fatalf("no seats must be refused with 403, got %d", c)
	}

	// Groups.
	c, g := do(t, srv, "POST", "/scim/v2/Groups", tokA, map[string]any{"displayName": "Eng", "externalId": "g1",
		"members": []map[string]any{{"value": id}, {"value": "00000000-0000-0000-0000-000000000000"}}})
	if c != 201 {
		t.Fatalf("group create: %d %v", c, g)
	}
	gid := g["id"].(string)
	if m := g["members"].([]any); len(m) != 1 {
		t.Fatalf("unknown member ids must be dropped: %v", m)
	}
	c, g = do(t, srv, "PATCH", "/scim/v2/Groups/"+gid, tokA, map[string]any{"Operations": []map[string]any{
		{"op": "remove", "path": `members[value eq "` + id + `"]`}}})
	if c != 200 || len(g["members"].([]any)) != 0 {
		t.Fatalf("remove member: %d %v", c, g)
	}
	c, g = do(t, srv, "PATCH", "/scim/v2/Groups/"+gid, tokA, map[string]any{"Operations": []map[string]any{
		{"op": "add", "path": "members", "value": []map[string]any{{"value": id}}}}})
	if c != 200 || len(g["members"].([]any)) != 1 {
		t.Fatalf("add member: %d %v", c, g)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM adrient.group_members WHERE group_id::text=$1`, gid).Scan(&n)
	if n != 1 {
		t.Fatalf("group_members not synced: %d", n)
	}
	if c, _ := do(t, srv, "GET", "/scim/v2/Groups/"+gid, tokB, nil); c != 404 {
		t.Fatalf("cross-org group get: %d", c)
	}
	if c, _ := do(t, srv, "DELETE", "/scim/v2/Groups/"+gid, tokA, nil); c != 204 {
		t.Fatalf("group delete: %d", c)
	}

	// Delete user (no offboarder wired: deactivates).
	if c, _ := do(t, srv, "DELETE", "/scim/v2/Users/"+id, tokA, nil); c != 204 {
		t.Fatalf("user delete: %d", c)
	}
	_ = pool.QueryRow(ctx, `SELECT status FROM adrient.users WHERE id::text=$1`, id).Scan(&status)
	if status != "blocked" {
		t.Fatalf("deleted user status %q", status)
	}

	// Unsupported filter attribute -> 400 invalidFilter.
	if c, b := do(t, srv, "GET", `/scim/v2/Users?filter=title%20eq%20%22x%22`, tokA, nil); c != 400 || b["scimType"] != "invalidFilter" {
		t.Fatalf("filter on title: %d %v", c, b)
	}

	// Rate limit: a token limited to 10/min is cut off.
	lim, _, _ := st.CreateToken(ctx, orgA, owner, "slow", 10, nil)
	got429 := false
	for i := 0; i < 15; i++ {
		c, _ := do(t, srv, "GET", "/scim/v2/ResourceTypes", lim, nil)
		if c == 429 {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("rate limit not enforced")
	}

	// Revoked and expired tokens stop working.
	if ok, _ := st.RevokeToken(ctx, orgA, mustTokenID(t, st, orgA, "entra")); !ok {
		t.Fatal("revoke")
	}
	if c, _ := do(t, srv, "GET", "/scim/v2/Users", tokA, nil); c != 401 {
		t.Fatalf("revoked token still works: %d", c)
	}
	past := time.Now().Add(-time.Hour)
	exp, _, _ := st.CreateToken(ctx, orgA, owner, "old", 0, &past)
	if c, _ := do(t, srv, "GET", "/scim/v2/Users", exp, nil); c != 401 {
		t.Fatalf("expired token works: %d", c)
	}
	if len(audited) < 5 || !strings.Contains(strings.Join(audited, ","), "scim.user.create") {
		t.Fatalf("audit calls: %v", audited)
	}
}

type http_err string

func (e http_err) Error() string { return string(e) }

func mustTokenID(t *testing.T, st *scim.Store, org, name string) string {
	t.Helper()
	l, err := st.ListTokens(context.Background(), org)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range l {
		if x.Name == name {
			return x.ID
		}
	}
	t.Fatal("token not found")
	return ""
}
