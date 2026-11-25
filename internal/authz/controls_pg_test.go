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

// TestControlsPG covers rule CRUD through the admin API (admin 200, member 403,
// guest 403), user_type rules, an IP condition, the app-access deny on user
// type guest and parity between the effective access viewer and enforcement.
func TestControlsPG(t *testing.T) {
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
	admin := seedUser(t, pool, orgID, "cadm")
	member := seedUser(t, pool, orgID, "cmem")
	guest := seedUser(t, pool, orgID, "cgst")
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.org_admins (org_id,user_id) VALUES ($1,$2)`, orgID, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE adrient.users SET user_type='guest', guest_apps='{drive,chat}' WHERE id=$1::uuid`, guest); err != nil {
		t.Fatal(err)
	}

	roles := authz.NewService(authz.PGLoader{Pool: pool})
	authz.SetDefault(roles)
	apps := authz.NewAppService(authz.PGAppStore{Pool: pool}, roles)
	authz.SetDefaultApps(apps)
	ctl := authz.NewControlService(authz.PGControlStore{Pool: pool}, authz.PGAppStore{Pool: pool}, roles)
	authz.SetDefaultControls(ctl)
	t.Cleanup(func() { authz.SetDefaultApps(nil); authz.SetDefaultControls(nil) })
	var audited []string
	h := &authz.ControlsHTTP{Pool: pool, Roles: roles, Apps: apps, Controls: ctl,
		Audit: func(_ context.Context, service, action, _, _ string, _ map[string]any, _ *http.Request) {
			audited = append(audited, service+":"+action)
		}}
	guarded := authz.AdminPathGuard(h)

	ctxFor := func(id, typ, ip string) context.Context {
		c := auth.WithUser(ctx, users.User{ID: id, Email: id + "@x.test", UserType: typ, GuestApps: []string{"drive", "chat"}})
		c = authz.WithClientIP(c, ip)
		return auth.WithOrg(c, orgs.Org{ID: orgID})
	}
	call := func(c context.Context, method, path string, body any) (int, map[string]any) {
		var rd *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		} else {
			rd = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, rd).WithContext(c)
		rec := httptest.NewRecorder()
		guarded.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	// Authorization of the controls API itself.
	if code, _ := call(ctxFor(member, "member", "1.1.1.1"), "GET", "/api/v1/admin/controls", nil); code != 403 {
		t.Fatalf("member got %d", code)
	}
	if code, _ := call(ctxFor(guest, "guest", "1.1.1.1"), "GET", "/api/v1/admin/access/effective?user_id="+guest, nil); code != 403 {
		t.Fatalf("guest got %d", code)
	}
	if code, body := call(ctxFor(admin, "member", "1.1.1.1"), "GET", "/api/v1/admin/controls", nil); code != 200 || body["catalog"] == nil {
		t.Fatalf("admin list: %d %v", code, body)
	}
	// Unknown key and bad value are rejected.
	if code, _ := call(ctxFor(admin, "member", ""), "POST", "/api/v1/admin/controls/rules", map[string]any{"control": "nope.x", "value": true, "subject_type": "everyone"}); code != 400 {
		t.Fatalf("unknown key got %d", code)
	}
	if code, _ := call(ctxFor(admin, "member", ""), "POST", "/api/v1/admin/controls/rules", map[string]any{"control": "drive.external_sharing", "value": "banana", "subject_type": "everyone"}); code != 400 {
		t.Fatalf("bad enum got %d", code)
	}

	// Defaults: a guest cannot download or reshare; external sharing is "domains".
	if authz.ControlBool(ctxFor(guest, "guest", ""), "drive.guest_download") {
		t.Fatal("guest download default should be off")
	}
	if got := authz.ControlEnum(ctxFor(member, "member", ""), "drive.external_sharing"); got != "domains" {
		t.Fatalf("default sharing %q", got)
	}

	// Rule: external sharing off for guests only.
	code, body := call(ctxFor(admin, "member", ""), "POST", "/api/v1/admin/controls/rules",
		map[string]any{"control": "drive.external_sharing", "value": "off", "subject_type": "user_type", "subject_id": "guest"})
	if code != 200 {
		t.Fatalf("create rule: %d %v", code, body)
	}
	ruleID, _ := body["id"].(string)
	ctl.Invalidate()
	if got := authz.ControlEnum(ctxFor(guest, "guest", ""), "drive.external_sharing"); got != "off" {
		t.Fatalf("guest sharing %q want off", got)
	}
	if got := authz.ControlEnum(ctxFor(member, "member", ""), "drive.external_sharing"); got != "domains" {
		t.Fatalf("member sharing %q want domains", got)
	}

	// IP condition rule: recording off from 8.8.8.0/24 only.
	if code, b := call(ctxFor(admin, "member", ""), "POST", "/api/v1/admin/controls/rules",
		map[string]any{"control": "meet.recording", "value": "off", "subject_type": "everyone", "condition": map[string]any{"ip_cidrs": []string{"8.8.8.0/24"}}}); code != 200 {
		t.Fatalf("ip rule: %d %v", code, b)
	}
	ctl.Invalidate()
	if got := authz.ControlEnum(ctxFor(member, "member", "8.8.8.8"), "meet.recording"); got != "off" {
		t.Fatalf("in-range recording %q", got)
	}
	if got := authz.ControlEnum(ctxFor(member, "member", "9.9.9.9"), "meet.recording"); got != "host" {
		t.Fatalf("out-of-range recording %q", got)
	}

	// Effective viewer matches enforcement for guest and member, and what-if IP flips.
	for _, c := range []struct{ id, typ string }{{guest, "guest"}, {member, "member"}} {
		code, eff := call(ctxFor(admin, "member", ""), "GET", "/api/v1/admin/access/effective?user_id="+c.id+"&ip=8.8.8.8", nil)
		if code != 200 {
			t.Fatalf("effective %s: %d %v", c.typ, code, eff)
		}
		viewer := map[string]any{}
		for _, a := range eff["apps"].([]any) {
			for _, cr := range a.(map[string]any)["controls"].([]any) {
				m := cr.(map[string]any)
				viewer[m["key"].(string)] = m["value"]
			}
		}
		for _, d := range authz.Controls {
			if d.App == "*" {
				continue
			}
			real := authz.Control(ctxFor(c.id, c.typ, "8.8.8.8"), d.Key).Value
			if viewer[d.Key] != real {
				t.Errorf("%s %s: viewer %v enforcement %v", c.typ, d.Key, viewer[d.Key], real)
			}
		}
	}
	_, eff := call(ctxFor(admin, "member", ""), "GET", "/api/v1/admin/access/effective?user_id="+member+"&app=meet&ip=9.9.9.9", nil)
	rec := eff["apps"].([]any)[0].(map[string]any)["controls"].([]any)
	for _, cr := range rec {
		m := cr.(map[string]any)
		if m["key"] == "meet.recording" && m["value"] != "host" {
			t.Errorf("what-if IP outside should be host, got %v", m["value"])
		}
	}

	// D2: a deny for user_type=guest closes an app a guest has on their invitation.
	hApps := &authz.AdminHTTP{Pool: pool, Roles: roles, Apps: apps}
	req := httptest.NewRequest("PUT", "/api/v1/admin/app-access/chat", bytes.NewReader([]byte(`{"policies":[{"subject_type":"user_type","subject_id":"guest","effect":"deny"}]}`))).WithContext(ctxFor(admin, "member", ""))
	rr := httptest.NewRecorder()
	hApps.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("put policy %d %s", rr.Code, rr.Body.String())
	}
	apps.Invalidate()
	if authz.CanOpenApp(ctxFor(guest, "guest", ""), "chat") {
		t.Error("guest should be denied chat")
	}
	if !authz.CanOpenApp(ctxFor(guest, "guest", ""), "drive") {
		t.Error("guest should still open drive")
	}
	if !authz.CanOpenApp(ctxFor(member, "member", ""), "chat") {
		t.Error("member must still open chat")
	}

	// Delete the rule: the guest returns to the default; audit rows were written.
	if code, _ := call(ctxFor(admin, "member", ""), "DELETE", "/api/v1/admin/controls/rules/"+ruleID, nil); code != 200 {
		t.Fatalf("delete %d", code)
	}
	ctl.Invalidate()
	if got := authz.ControlEnum(ctxFor(guest, "guest", ""), "drive.external_sharing"); got != "domains" {
		t.Fatalf("after delete %q", got)
	}
	if len(audited) < 3 {
		t.Fatalf("audit rows %v", audited)
	}
}
