package authz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

// Every admin / orgadmin route registered anywhere in the server.
var adminRoutes = []string{
	"/api/v1/admin/analytics", "/api/v1/admin/app-access", "/api/v1/admin/app-access/drive",
	"/api/v1/admin/app-access/preview", "/api/v1/admin/audit", "/api/v1/admin/backup",
	"/api/v1/admin/domains", "/api/v1/admin/domains/x/verify", "/api/v1/admin/export-all-users",
	"/api/v1/admin/geo", "/api/v1/admin/honeypot", "/api/v1/admin/identity/run-expiry",
	"/api/v1/admin/identity/users", "/api/v1/admin/identity/users/u1", "/api/v1/admin/invitations",
	"/api/v1/admin/invitations/i1", "/api/v1/admin/org", "/api/v1/admin/org/branding",
	"/api/v1/admin/org/branding/logo", "/api/v1/admin/org/settings", "/api/v1/admin/ratelimit",
	"/api/v1/admin/roles", "/api/v1/admin/roles/assignments", "/api/v1/admin/roles/assignments/a1",
	"/api/v1/admin/security/password", "/api/v1/admin/security/policies", "/api/v1/admin/sessions",
	"/api/v1/admin/sessions/abc/revoke", "/api/v1/admin/subjects", "/api/v1/admin/users",
	"/api/v1/admin/users/u1", "/api/v1/admin/users/u1/admin", "/api/v1/admin/users/u1/zitadel",
	"/api/v1/admin/whoami", "/api/v1/admin/controls", "/api/v1/admin/controls/rules",
	"/api/v1/admin/access/effective", "/api/v1/admin/external/policy", "/api/v1/admin/external/partners",
	"/api/v1/orgadmin", "/api/v1/orgadmin/anything", "/api/v1/orgadmin/users",
}

// A user who was admin and is then converted to guest must never keep admin
// powers from the permission cache or from a stale admin role row, and the
// admin guard answers 403 on every admin route, every time, concurrently.
func TestGuestNeverGets200OnAdminRoutes(t *testing.T) {
	var asGuest atomic.Bool
	loader := loaderFunc(func(_ context.Context, _, _ string) ([]Assignment, error) {
		// stale: the admin role row is still present after conversion
		return []Assignment{{RoleKey: RoleAdmin, Permissions: []string{"*"}}, {RoleKey: RoleGuest}}, nil
	})
	svc := NewService(loader).WithBreakGlass(func(context.Context) bool { return true })
	SetDefault(svc)
	h := AdminPathGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			WriteForbidden(w, PermOrgAdmin)
			return
		}
		w.WriteHeader(200)
	}))
	// warm the cache as a member-typed admin first
	mctx := auth.WithOrg(auth.WithUser(context.Background(), users.User{ID: "g1", Email: "g@x.test"}), orgs.Org{ID: "o"})
	if !svc.Can(mctx, PermOrgAdmin) {
		t.Fatal("setup: admin member should be admin")
	}
	asGuest.Store(true)
	var wg sync.WaitGroup
	var bad atomic.Int32
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				for _, p := range adminRoutes {
					for _, m := range []string{"GET", "POST", "PUT", "DELETE"} {
						ctx := auth.WithOrg(auth.WithUser(context.Background(), users.User{ID: "g1", Email: "g@x.test", UserType: "guest"}), orgs.Org{ID: "o"})
						rec := httptest.NewRecorder()
						h.ServeHTTP(rec, httptest.NewRequest(m, p, nil).WithContext(ctx))
						if rec.Code != 403 {
							bad.Add(1)
						}
					}
				}
			}
		}()
	}
	wg.Wait()
	if bad.Load() != 0 {
		t.Fatalf("%d guest requests did not get 403", bad.Load())
	}
	gctx := auth.WithOrg(auth.WithUser(context.Background(), users.User{ID: "g1", UserType: "guest"}), orgs.Org{ID: "o"})
	if svc.Can(gctx, PermOrgAdmin) {
		t.Fatal("guest holds org:admin through stale role row / break-glass")
	}
}

type loaderFunc func(ctx context.Context, orgID, userID string) ([]Assignment, error)

func (f loaderFunc) Load(ctx context.Context, o, u string) ([]Assignment, error) { return f(ctx, o, u) }
