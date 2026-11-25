package authz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAppStore struct {
	pol    []Policy
	groups map[string][]string
	depts  map[string][]string
}

func (f fakeAppStore) Policies(context.Context, string) ([]Policy, error) { return f.pol, nil }
func (f fakeAppStore) Facts(_ context.Context, _, u string) ([]string, []string, error) {
	return f.groups[u], f.depts[u], nil
}

func TestResolvePolicies(t *testing.T) {
	member := Subject{UserID: "u1", RoleKeys: []string{RoleMember}}
	pol := func(app, typ, id, eff string) Policy {
		return Policy{AppID: app, SubjectType: typ, SubjectID: id, Effect: eff}
	}
	cases := []struct {
		name string
		pols []Policy
		sub  Subject
		app  string
		want bool
	}{
		{"no rows = everyone", nil, member, "crm", true},
		{"explicit everyone allow", []Policy{pol("crm", SubjectEveryone, "", EffectAllow)}, member, "crm", true},
		{"allow group, member in group", []Policy{pol("crm", SubjectGroup, "g1", EffectAllow)}, Subject{UserID: "u1", GroupIDs: []string{"g1"}}, "crm", true},
		{"allow group, member not in group", []Policy{pol("crm", SubjectGroup, "g1", EffectAllow)}, Subject{UserID: "u1", GroupIDs: []string{"g2"}}, "crm", false},
		{"department subtree: user dept chain includes policy dept", []Policy{pol("accounting", SubjectDepartment, "fin", EffectAllow)}, Subject{UserID: "u1", DeptIDs: []string{"fin-ap", "fin"}}, "accounting", true},
		{"department sibling does not match", []Policy{pol("accounting", SubjectDepartment, "fin", EffectAllow)}, Subject{UserID: "u1", DeptIDs: []string{"ops"}}, "accounting", false},
		{"role allow", []Policy{pol("pos", SubjectRole, RoleManager, EffectAllow)}, Subject{UserID: "u1", RoleKeys: []string{RoleManager}}, "pos", true},
		{"role allow, plain member", []Policy{pol("pos", SubjectRole, RoleManager, EffectAllow)}, member, "pos", false},
		{"explicit user allow", []Policy{pol("pos", SubjectUser, "u1", EffectAllow)}, member, "pos", true},
		{"explicit other user", []Policy{pol("pos", SubjectUser, "u2", EffectAllow)}, member, "pos", false},
		{"deny wins over group allow", []Policy{pol("crm", SubjectGroup, "g1", EffectAllow), pol("crm", SubjectUser, "u1", EffectDeny)}, Subject{UserID: "u1", GroupIDs: []string{"g1"}}, "crm", false},
		{"deny wins over everyone allow", []Policy{pol("crm", SubjectEveryone, "", EffectAllow), pol("crm", SubjectRole, RoleGuest, EffectDeny)}, Subject{UserID: "u1", RoleKeys: []string{RoleGuest}}, "crm", false},
		{"deny only: others still allowed", []Policy{pol("crm", SubjectGroup, "g9", EffectDeny)}, member, "crm", true},
		{"policy of another app ignored", []Policy{pol("pos", SubjectUser, "u2", EffectAllow)}, member, "crm", true},
		{"admin bypasses deny", []Policy{pol("crm", SubjectUser, "u1", EffectDeny)}, Subject{UserID: "u1", Admin: true}, "crm", true},
		{"app admin bypasses own app only", []Policy{pol("accounting", SubjectGroup, "g1", EffectAllow), pol("crm", SubjectGroup, "g1", EffectAllow)},
			Subject{UserID: "u1", AppAdmin: map[string]bool{"accounting": true}}, "accounting", true},
		{"app admin of another app", []Policy{pol("crm", SubjectGroup, "g1", EffectAllow)}, Subject{UserID: "u1", AppAdmin: map[string]bool{"accounting": true}}, "crm", false},
		{"core app ignores deny", []Policy{pol("settings", SubjectEveryone, "", EffectDeny)}, member, "settings", true},
	}
	for _, c := range cases {
		if got := Resolve(c.pols, c.app, c.sub).Allowed; got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestAppRouting(t *testing.T) {
	for path, want := range map[string]string{
		"/api/v1/accounting/accounts": "accounting", "/api/v1/crm": "crm", "/api/v1/inventory/products": "inventory",
		"/api/v1/pos/sales": "pos", "/api/v1/videos/1": "video", "/api/v1/forgejo/x": "git", "/api/v1/gamerooms/1": "games",
		"/api/v1/me/apps": "", "/api/v1/admin/roles": "", "/api/v1/people": "", "/api/v1/directory": "",
		"/api/v1/notifications": "", "/api/v1/search": "", "/healthz": "",
	} {
		if got := AppForHTTP(path); got != want {
			t.Errorf("AppForHTTP(%s)=%q want %q", path, got, want)
		}
	}
	for m, want := range map[string]string{
		"/adrient.v1.PosService/Refund": "pos", "/adrient.v1.InventoryService/ListProducts": "inventory",
		"/adrient.v1.WhiteboardsService/Get": "whiteboard", "/adrient.v1.AdminService/GetServiceSettings": "",
		"/adrient.v1.NotificationsService/List": "", "/adrient.v1.SearchService/Search": "", "/adrient.v1.AuthService/Me": "",
	} {
		if got := AppForRPC(m); got != want {
			t.Errorf("AppForRPC(%s)=%q want %q", m, got, want)
		}
	}
	for _, a := range httpApps {
		if !IsKnownApp(a) {
			t.Errorf("httpApps maps to unknown app %q", a)
		}
	}
	for _, a := range rpcApps {
		if !IsKnownApp(a) {
			t.Errorf("rpcApps maps to unknown app %q", a)
		}
	}
}

func TestBlockedAppReturns403(t *testing.T) {
	roles := newSvc() // member, acct (App admin accounting), admin
	store := fakeAppStore{
		pol: []Policy{
			{AppID: "accounting", SubjectType: SubjectDepartment, SubjectID: "fin", Effect: EffectAllow},
			{AppID: "inventory", SubjectType: SubjectDepartment, SubjectID: "ops", Effect: EffectAllow},
		},
		depts: map[string][]string{"member": {"ops"}, "acct": {"ops"}},
	}
	SetDefaultApps(NewAppService(store, roles))
	t.Cleanup(func() { SetDefaultApps(nil) })

	var denied []Denial
	SetDenyHook(func(_ context.Context, d Denial) { denied = append(denied, d) })
	t.Cleanup(func() { SetDenyHook(nil) })

	h := Guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	do := func(user, path string) int {
		req := httptest.NewRequest("GET", path, nil).WithContext(callerCtx(user))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	// Accounting is limited to Finance; the member is in Ops.
	if c := do("member", "/api/v1/accounting/accounts"); c != 403 {
		t.Fatalf("member -> accounting: %d want 403", c)
	}
	if len(denied) != 1 || denied[0].Permission != "accounting:app:open" {
		t.Fatalf("denial not reported as app denial: %+v", denied)
	}
	if c := do("member", "/api/v1/inventory/products"); c != 204 {
		t.Fatalf("member -> inventory: %d want 204", c)
	}
	if c := do("member", "/api/v1/people"); c != 204 {
		t.Fatalf("core route must stay open: %d", c)
	}
	if c := do("admin", "/api/v1/accounting/accounts"); c != 204 {
		t.Fatalf("admin always allowed: %d", c)
	}
	if c := do("acct", "/api/v1/accounting/accounts"); c != 204 {
		t.Fatalf("app admin(accounting) allowed: %d", c)
	}

	// gRPC interceptor returns PermissionDenied for a blocked app.
	ui := UnaryInterceptor()
	rpc := func(user, method string) error {
		_, err := ui(callerCtx(user), nil, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, any) (any, error) { return nil, nil })
		return err
	}
	store.pol = append(store.pol, Policy{AppID: "pos", SubjectType: SubjectGroup, SubjectID: "g1", Effect: EffectAllow})
	SetDefaultApps(NewAppService(store, roles))
	if err := rpc("member", "/adrient.v1.PosService/ListRegisters"); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("grpc pos blocked: %v", err)
	}
	if err := rpc("member", "/adrient.v1.InventoryService/ListProducts"); err != nil {
		t.Fatalf("grpc inventory: %v", err)
	}
	if err := rpc("member", "/adrient.v1.NotificationsService/List"); err != nil {
		t.Fatalf("grpc core: %v", err)
	}
}

func TestCanOpenAppNoServiceIsOpen(t *testing.T) {
	SetDefaultApps(nil)
	if !CanOpenApp(callerCtx("member"), "accounting") {
		t.Fatal("without an AppService no policies exist")
	}
}

func TestExternalUsersAreShutOutOfBusinessApps(t *testing.T) {
	ext := Subject{UserID: "u1", UserType: TypeExternal, GroupIDs: []string{"g1"}}
	mem := Subject{UserID: "u2", UserType: TypeMember}
	for _, app := range []string{"hr", "talent", "accounting", "inventory", "pos", "invoicing", "crm"} {
		if Resolve(nil, app, ext).Allowed {
			t.Errorf("external user opened %s with no rule", app)
		}
		if !Resolve(nil, app, mem).Allowed {
			t.Errorf("member lost %s", app)
		}
		// "everyone" allow rules do not admit externals.
		ev := []Policy{{AppID: app, SubjectType: SubjectEveryone, Effect: EffectAllow}}
		if Resolve(ev, app, ext).Allowed {
			t.Errorf("everyone rule admitted external to %s", app)
		}
		// naming their group does.
		gr := []Policy{{AppID: app, SubjectType: SubjectGroup, SubjectID: "g1", Effect: EffectAllow}}
		if !Resolve(gr, app, ext).Allowed {
			t.Errorf("group allow rule did not admit external to %s", app)
		}
	}
	if !Resolve(nil, "drive", ext).Allowed {
		t.Error("external lost drive")
	}
}
