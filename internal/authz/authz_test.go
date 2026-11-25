package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

var memberPerms = []string{"org:member", "crm:*", "invoicing:*", "inventory:*", "pos:read", "pos:sell", "pos:shift:open"}

type fakeLoader map[string][]Assignment

func (f fakeLoader) Load(_ context.Context, _, userID string) ([]Assignment, error) {
	return f[userID], nil
}

func callerCtx(userID string) context.Context {
	ctx := auth.WithUser(context.Background(), users.User{ID: userID, Email: userID + "@x.test"})
	return auth.WithOrg(ctx, orgs.Org{ID: "org1"})
}

func newSvc() *Service {
	s := NewService(fakeLoader{
		"owner":   {{RoleKey: RoleOwner, Permissions: []string{"*", "org:owner"}}},
		"admin":   {{RoleKey: RoleAdmin, Permissions: []string{"*"}}},
		"member":  {{RoleKey: RoleMember, Permissions: memberPerms}},
		"manager": {{RoleKey: RoleMember, Permissions: memberPerms}, {RoleKey: RoleManager, Permissions: []string{"org:manager", "pos:*"}}},
		"acct":    {{RoleKey: RoleMember, Permissions: memberPerms}, {RoleKey: RoleAppAdmin, ScopeApp: "accounting"}},
		"bad":     {{RoleKey: RoleAppAdmin}}, // unscoped app admin grants nothing
	})
	SetDefault(s)
	return s
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pat, perm string
		want      bool
	}{
		{"*", "accounting:ledger:read", true},
		{"*", PermOrgOwner, false},
		{PermOrgOwner, PermOrgOwner, true},
		{"crm:*", "crm:deal:write", true},
		{"crm:*", "crmx:deal:write", false},
		{"pos:sell", "pos:refund", false},
		{"pos:read", "pos:read", true},
		{"", "pos:read", false},
	}
	for _, c := range cases {
		if got := Match(c.pat, c.perm); got != c.want {
			t.Errorf("Match(%q,%q)=%v want %v", c.pat, c.perm, got, c.want)
		}
	}
}

func TestCanByRole(t *testing.T) {
	newSvc()
	type row struct {
		user, perm string
		want       bool
	}
	for _, c := range []row{
		{"member", CRMWrite, true},
		{"member", InvoicingWrite, true},
		{"member", InventoryAdjust, true},
		{"member", PosSell, true},
		{"member", PosRefund, false},
		{"member", PosShiftClose, false},
		{"member", AccountingRead, false},
		{"member", PermOrgAdmin, false},
		{"manager", PosRefund, true},
		{"manager", PosShiftClose, true},
		{"manager", AccountingRead, false},
		{"acct", AccountingRead, true},
		{"acct", AccountingWrite, true},
		{"acct", PosRefund, false},
		{"admin", AccountingRead, true},
		{"admin", PosRefund, true},
		{"admin", PermOrgAdmin, true},
		{"admin", PermOrgOwner, false},
		{"owner", PermOrgOwner, true},
		{"owner", PermOrgAdmin, true},
		{"bad", AccountingRead, false},
		{"nobody", CRMRead, false},
	} {
		if got := Can(callerCtx(c.user), c.perm); got != c.want {
			t.Errorf("Can(%s,%s)=%v want %v", c.user, c.perm, got, c.want)
		}
	}
	if err := Require(callerCtx("member"), AccountingRead); err == nil {
		t.Fatal("Require should fail for member")
	} else if de, ok := err.(*DeniedError); !ok || de.Permission != AccountingRead {
		t.Fatalf("unexpected error %v", err)
	}
	if err := Require(callerCtx("admin"), AccountingRead); err != nil {
		t.Fatal(err)
	}
	if IsAdmin(callerCtx("member")) || !IsAdmin(callerCtx("admin")) || !IsAdmin(callerCtx("owner")) {
		t.Fatal("IsAdmin wrong")
	}
}

func TestFailClosed(t *testing.T) {
	SetDefault(nil)
	if Can(callerCtx("admin"), CRMRead) {
		t.Fatal("no default service must deny")
	}
	newSvc()
	if Can(context.Background(), CRMRead) {
		t.Fatal("no caller must deny")
	}
}

func TestBreakGlass(t *testing.T) {
	s := newSvc().WithBreakGlass(func(ctx context.Context) bool {
		u, _ := auth.UserFromContext(ctx)
		return u.Email == "member@x.test"
	})
	if !s.Can(callerCtx("member"), AccountingRead) {
		t.Fatal("break-glass caller must be admin")
	}
}

func TestPermForHTTP(t *testing.T) {
	cases := []struct{ method, path, want string }{
		{"GET", "/api/v1/accounting/accounts", AccountingRead},
		{"POST", "/api/v1/accounting/journals", AccountingWrite},
		{"GET", "/api/v1/crm/deals", CRMRead},
		{"PUT", "/api/v1/crm/deals/1", CRMWrite},
		{"POST", "/api/v1/invoicing/documents", InvoicingWrite},
		{"POST", "/api/v1/inventory/products/p1/adjust", InventoryAdjust},
		{"GET", "/api/v1/inventory/products", InventoryRead},
		{"POST", "/api/v1/pos/sales/s1/refund", PosRefund},
		{"POST", "/api/v1/pos/shifts/s1/close", PosShiftClose},
		{"POST", "/api/v1/pos/sales/complete", PosSell},
		{"GET", "/api/v1/pos/sales", PosRead},
		{"POST", "/api/v1/pos/registers/r1/shift", PosShiftOpen},
		{"PUT", "/api/v1/pos/settings", PosSettingsWrite},
		{"GET", "/api/v1/pos/report", PosReport},
	}
	for _, c := range cases {
		got, ok := PermForHTTP(c.method, c.path)
		if !ok || got != c.want {
			t.Errorf("%s %s = %q,%v want %q", c.method, c.path, got, ok, c.want)
		}
	}
	if _, ok := PermForHTTP("GET", "/api/v1/drive/files"); ok {
		t.Error("drive must not be guarded")
	}
}

func TestGuard(t *testing.T) {
	newSvc()
	h := Guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	do := func(user, method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil).WithContext(callerCtx(user))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := do("member", "GET", "/api/v1/accounting/accounts")
	if rec.Code != 403 {
		t.Fatalf("member ledger read: %d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["permission"] != AccountingRead {
		t.Fatalf("body %v", body)
	}
	if c := do("member", "POST", "/api/v1/pos/sales/s1/refund").Code; c != 403 {
		t.Fatalf("member refund: %d", c)
	}
	for _, p := range []string{"/api/v1/pos/sales/s1/refund", "/api/v1/accounting/accounts"} {
		if c := do("admin", "POST", p).Code; c != 204 {
			t.Fatalf("admin %s: %d", p, c)
		}
	}
	if c := do("member", "POST", "/api/v1/pos/sales/complete").Code; c != 204 {
		t.Fatalf("member sell: %d", c)
	}
	if c := do("member", "POST", "/api/v1/crm/deals").Code; c != 204 {
		t.Fatalf("member crm: %d", c)
	}
}

func TestPermForRPC(t *testing.T) {
	for m, want := range map[string]string{
		"/adrient.v1.PosService/RefundSale":          PosRefund,
		"/adrient.v1.PosService/CloseShift":          PosShiftClose,
		"/adrient.v1.PosService/ListSales":           PosRead,
		"/adrient.v1.InventoryService/AdjustStock":   InventoryAdjust,
		"/adrient.v1.InventoryService/ListProducts":  InventoryRead,
		"/adrient.v1.InventoryService/CreateProduct": InventoryWrite,
	} {
		if got, ok := PermForRPC(m); !ok || got != want {
			t.Errorf("%s=%q,%v want %q", m, got, ok, want)
		}
	}
	if _, ok := PermForRPC("/adrient.v1.DriveService/List"); ok {
		t.Error("drive must not be guarded")
	}
}

func TestMeHandler(t *testing.T) {
	newSvc()
	req := httptest.NewRequest("GET", "/api/v1/me/permissions", nil).WithContext(callerCtx("acct"))
	rec := httptest.NewRecorder()
	MeHandler(rec, req)
	var body struct {
		Roles       []string `json:"roles"`
		Permissions []string `json:"permissions"`
		Admin       bool     `json:"admin"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 {
		t.Fatalf("%d %v", rec.Code, err)
	}
	if body.Admin || len(body.Roles) != 2 || body.Roles[0] != "app_admin:accounting" {
		t.Fatalf("%+v", body)
	}
}
