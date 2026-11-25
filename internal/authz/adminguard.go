package authz

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"net/http"
	"strings"

	"google.golang.org/grpc/peer"
)

type ipKey struct{}

// WithClientIP stores the caller's IP on ctx for control and policy conditions.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ipKey{}, ip)
}

// ClientIP returns the caller IP set by the request guard (or the gRPC peer).
func ClientIP(ctx context.Context) string {
	if v, ok := ctx.Value(ipKey{}).(string); ok {
		return v
	}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		a := p.Addr.String()
		if i := strings.LastIndex(a, ":"); i > 0 {
			return strings.Trim(a[:i], "[]")
		}
		return a
	}
	return ""
}

// IsAdminPath reports whether path is an admin control-plane route.
func IsAdminPath(path string) bool {
	for _, pre := range []string{"/api/v1/admin", "/api/v1/orgadmin"} {
		if path == pre || strings.HasPrefix(path, pre+"/") {
			return true
		}
	}
	return false
}

// AdminPathGuard refuses every /api/v1/admin/* and /api/v1/orgadmin/* request
// from a guest before any handler runs, so no individual handler (including the
// non-gated whoami) can answer a guest with 200. It must run inside the auth
// middleware.
func AdminPathGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsAdminPath(r.URL.Path) && IsGuest(r.Context()) {
			reportDenied(r.Context(), Denial{Permission: PermOrgAdmin, Method: r.Method, Target: r.URL.Path, IP: httpClientIP(r), UserAgent: r.UserAgent()})
			WriteForbidden(w, PermOrgAdmin)
			return
		}
		ctx := r.Context()
		// Delegated admins (user admin, billing admin, help desk, read-only): a
		// caller who is not a super admin passes only for the request whose
		// mapped permission they hold, and only for people in their scope.
		if IsAdminPath(r.URL.Path) && !Can(ctx, PermOrgAdmin) {
			need := DelegatedPerm(r.Method, r.URL.Path)
			if need == "any" && anyAdminPerm(ctx) || need != "" && need != "any" && Can(ctx, need) {
				if t := targetOfPath(r.URL.Path); t != "" && DelegatedScope != nil && r.Method != http.MethodGet {
					u, _ := auth.UserFromContext(ctx)
					o, _ := auth.OrgFromContext(ctx)
					if !DelegatedScope(ctx, o.ID, u.ID, t) {
						reportDenied(ctx, Denial{Permission: need + " (outside your scope)", Method: r.Method, Target: r.URL.Path, IP: httpClientIP(r), UserAgent: r.UserAgent()})
						WriteForbidden(w, need)
						return
					}
				}
				ctx = WithDelegatedAdmin(ctx)
			}
		}
		// Every auth-wrapped request carries the caller IP for control conditions.
		next.ServeHTTP(w, r.WithContext(WithClientIP(ctx, httpClientIP(r))))
	})
}
