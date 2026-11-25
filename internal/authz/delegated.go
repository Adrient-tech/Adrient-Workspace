package authz

import (
	"context"
	"net/http"
	"strings"
)

// Delegated admin roles (user admin, billing admin, help desk, read-only) hold
// permissions such as "admin:users:write". AdminPathGuard maps every admin API
// request to one of those permissions; a caller who holds it is let through for
// that one request. Paths that have no mapping (roles, tokens, service
// settings...) stay super-admin only.

type delegatedKey struct{}

// WithDelegatedAdmin marks ctx as holding a delegated admin permission for the
// current request. IsAdmin then reports true for that request only.
func WithDelegatedAdmin(ctx context.Context) context.Context {
	return context.WithValue(ctx, delegatedKey{}, true)
}

func delegatedFrom(ctx context.Context) bool {
	v, _ := ctx.Value(delegatedKey{}).(bool)
	return v
}

// DelegatedPerm returns the permission a delegated admin needs for an admin API
// request, "any" for the whoami probe, or "" when only a super admin may call it.
func DelegatedPerm(method, path string) string {
	p := strings.TrimRight(path, "/")
	for _, pre := range []string{"/api/v1/admin/", "/api/v1/orgadmin/"} {
		if strings.HasPrefix(p+"/", pre) && p+"/" == pre {
			return ""
		}
	}
	rest := ""
	switch {
	case strings.HasPrefix(p, "/api/v1/admin/"):
		rest = strings.TrimPrefix(p, "/api/v1/admin/")
	case strings.HasPrefix(p, "/api/v1/orgadmin/"):
		rest = "settings/" + strings.TrimPrefix(p, "/api/v1/orgadmin/")
	default:
		return ""
	}
	seg := strings.Split(rest, "/")
	write := method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
	area := ""
	switch seg[0] {
	case "whoami":
		return "any"
	case "users":
		area = "users"
		if len(seg) >= 3 && (seg[2] == "admin" || seg[2] == "zitadel" || seg[2] == "org") {
			return "" // granting admin, deleting the identity: super admin only
		}
		if method == http.MethodDelete && len(seg) == 2 {
			return "" // removing a user outright: super admin only (suspend and delete live under lifecycle)
		}
		if write && len(seg) >= 3 && seg[2] == "password" {
			return "admin:users:reset"
		}
	case "groups":
		area = "groups"
	case "sessions":
		area = "sessions"
	case "signin":
		area = "sessions"
	case "audit", "analytics":
		area = "audit"
	case "seats":
		area = "billing"
	case "lifecycle":
		area = "lifecycle"
	case "reviews":
		area = "reviews"
	case "sso", "scim":
		area = "identity"
	case "security":
		area = "security"
	case "access", "controls", "apps", "app-access", "service-settings":
		area = "apps"
	case "external":
		area = "external"
	case "identity", "domains", "invitations":
		area = "identity"
	case "settings", "org":
		area = "settings"
	default:
		return "" // roles, admin-roles, admin-tokens, geo, honeypot... super admin only
	}
	if write {
		return "admin:" + area + ":write"
	}
	return "admin:" + area + ":read"
}

// anyAdminPerm reports whether the caller holds at least one delegated admin permission.
func anyAdminPerm(ctx context.Context) bool {
	s := std.Load()
	if s == nil {
		return false
	}
	set, err := s.Resolve(ctx)
	if err != nil {
		return false
	}
	for _, p := range set.Patterns() {
		if strings.HasPrefix(p, "admin:") {
			return true
		}
	}
	return false
}

// DelegatedScope decides whether a department-scoped admin may act on a target
// person ("" target = not about one person). nil = no scoping. The server sets it.
var DelegatedScope func(ctx context.Context, orgID, adminUserID, target string) bool

// targetOfPath extracts the person an admin path is about (a Zitadel subject or
// an Adrient user id), or "".
func targetOfPath(path string) string {
	p := strings.Trim(strings.TrimRight(path, "/"), "/")
	seg := strings.Split(p, "/")
	// api/v1/admin/users/{id}[/...]  and  api/v1/admin/lifecycle/users/{id}/...
	for i := 0; i+1 < len(seg); i++ {
		if seg[i] == "users" && i >= 3 {
			return seg[i+1]
		}
	}
	return ""
}
