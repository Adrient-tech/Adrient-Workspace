package authz

import (
	"net/http"
	"testing"
)

func TestDelegatedPerm(t *testing.T) {
	cases := []struct{ method, path, want string }{
		{"GET", "/api/v1/admin/users", "admin:users:read"},
		{"POST", "/api/v1/admin/users", "admin:users:write"},
		{"PATCH", "/api/v1/admin/users/abc", "admin:users:write"},
		{"POST", "/api/v1/admin/users/abc/password", "admin:users:reset"},
		{"POST", "/api/v1/admin/users/abc/admin", ""},     // granting admin is super admin only
		{"DELETE", "/api/v1/admin/users/abc/admin", ""},   // so is revoking it
		{"DELETE", "/api/v1/admin/users/abc/zitadel", ""}, // and deleting the identity
		{"DELETE", "/api/v1/admin/users/abc", ""},         // and removing a user outright
		{"GET", "/api/v1/admin/seats/usage", "admin:billing:read"},
		{"PUT", "/api/v1/admin/seats/plan", "admin:billing:write"},
		{"POST", "/api/v1/admin/lifecycle/users/u/delete", "admin:lifecycle:write"},
		{"GET", "/api/v1/admin/signin/history", "admin:sessions:read"},
		{"GET", "/api/v1/admin/whoami", "any"},
		{"GET", "/api/v1/admin/roles", ""},
		{"POST", "/api/v1/admin/admin-roles", ""},
		{"POST", "/api/v1/admin/admin-tokens", ""},
		{"GET", "/api/v1/drive/files", ""},
	}
	for _, c := range cases {
		if got := DelegatedPerm(c.method, c.path); got != c.want {
			t.Errorf("%s %s: got %q want %q", c.method, c.path, got, c.want)
		}
	}
	if targetOfPath("/api/v1/admin/users/abc/password") != "abc" || targetOfPath("/api/v1/admin/lifecycle/users/u1/delete") != "u1" || targetOfPath("/api/v1/admin/users") != "" {
		t.Error("targetOfPath")
	}
	_ = http.MethodGet
}

func TestDelegatedMarkerOnlyAffectsAdminCheck(t *testing.T) {
	s := NewService(fakeLoader{"help": {{RoleKey: "help_desk", Permissions: []string{"admin:users:read"}}}})
	ctx := WithDelegatedAdmin(callerCtx("help"))
	if s.Can(callerCtx("help"), PermOrgAdmin) {
		t.Fatal("help desk is not an admin by itself")
	}
	if !s.Can(ctx, PermOrgAdmin) || !s.CanAssigned(ctx, PermOrgAdmin) {
		t.Fatal("marker must satisfy the admin check")
	}
	if s.Can(ctx, "accounting:ledger:read") {
		t.Fatal("marker must not grant anything else")
	}
}
