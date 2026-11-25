package audit

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

func TestAPIActionAndResource(t *testing.T) {
	cases := []struct {
		method, path, action, resType, resID string
	}{
		{"POST", "/api/v1/crm/deals", "create", "deals", ""},
		{"PUT", "/api/v1/crm/deals/d1", "update", "deals", "d1"},
		{"DELETE", "/api/v1/invoicing/documents/x9", "delete", "documents", "x9"},
		{"POST", "/api/v1/crm/deals/d1/stage", "stage", "deals", "d1"},
		{"POST", "/api/v1/accounting/bank/import", "import", "bank", "import"},
	}
	for _, c := range cases {
		mount := c.path[:strings.Index(c.path[len("/api/v1/"):], "/")+len("/api/v1/")]
		if got := apiAction(c.method, mount, c.path); got != c.action {
			t.Errorf("%s %s action = %q, want %q", c.method, c.path, got, c.action)
		}
		rt, id := apiResource(mount, c.path)
		if rt != c.resType || id != c.resID {
			t.Errorf("%s resource = (%q,%q), want (%q,%q)", c.path, rt, id, c.resType, c.resID)
		}
	}
}

func TestLogAPI_NilRecorderPassThrough(t *testing.T) {
	var rec *Recorder
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	rr := httptest.NewRecorder()
	rec.LogAPI("crm", "/api/v1/crm", nil, h).ServeHTTP(rr, httptest.NewRequest("POST", "/api/v1/crm/deals", nil))
	if rr.Code != 204 {
		t.Fatalf("code = %d", rr.Code)
	}
}

func TestDescribeRPC(t *testing.T) {
	_, id, d, ok := describeRPC("AdjustStock", &adrientv1.AdjustStockRequest{Id: "p1", Qty: -3, Reason: "remove", Ref: "R-1"}, nil)
	if !ok || id != "p1" || d["reason"] != "remove" || d["qty"] != "-3" || d["ref"] != "R-1" {
		t.Errorf("AdjustStock = %v %v %v", ok, id, d)
	}
	rt, id, d, _ := describeRPC("UpdateProduct", &adrientv1.SaveInvProductRequest{Id: "p2", Name: "Tea", Price: 12.5, Cost: 7}, nil)
	if rt != "product" || id != "p2" || d["price"] != "12.5" || d["cost"] != "7" {
		t.Errorf("UpdateProduct = %v %v %v", rt, id, d)
	}
	rt, id, d, _ = describeRPC("CompleteSale", &adrientv1.PosSaleInput{RegisterId: "r1"}, &adrientv1.PosSale{Id: "s1", Number: 42, Total: 99})
	if rt != "sale" || id != "s1" || d["number"] != "42" || d["total"] != "99" {
		t.Errorf("CompleteSale = %v %v %v", rt, id, d)
	}
	_, id, d, _ = describeRPC("RefundSale", &adrientv1.RefundPosSaleRequest{Id: "s1", Method: "cash", Restock: true, Lines: []*adrientv1.RefundLineInput{{}}}, nil)
	if id != "s1" || d["method"] != "cash" || d["lines"] != 1 {
		t.Errorf("RefundSale = %v %v", id, d)
	}
	_, id, d, _ = describeRPC("CloseShift", &adrientv1.ClosePosShiftRequest{Id: "sh1", CountedCash: 500}, nil)
	if id != "sh1" || d["counted_cash"] != "500" {
		t.Errorf("CloseShift = %v %v", id, d)
	}
	if _, _, _, ok := describeRPC("ListProducts", nil, nil); ok {
		t.Error("ListProducts should have no mapping")
	}
}

func TestIsMutating_StockAndPos(t *testing.T) {
	for _, m := range []string{"AdjustStock", "CompleteSale", "RefundSale", "CloseShift", "DeleteHeldSale", "UpdateProduct"} {
		if !isMutating(m) {
			t.Errorf("%s should be mutating", m)
		}
	}
	if isMutating("ListSales") || isMutating("Report") {
		t.Error("reads must not be mutating")
	}
}

// ---- database-backed (scratch DB via ADRIENT_TEST_DSN; never the live schema) ----

func scratchDB(t *testing.T) (*pgxpool.Pool, string) {
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
	var org string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	return pool, org
}

func TestAuditHooks_DB(t *testing.T) {
	pool, org := scratchDB(t)
	repo := NewRepository(pool)
	rec := NewRecorder(repo, func(context.Context) (Actor, bool) {
		return Actor{OrgID: org, Email: "m@x.test"}, true
	})
	ctx := context.Background()

	// Mutating HTTP call is recorded with resource, annotation and status.
	h := rec.LogAPI("crm", "/api/v1/crm", nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Annotate(r.Context(), "deal", "d1", "moved to won")
		w.WriteHeader(200)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/v1/crm/deals/d1/stage", nil))
	// A plain GET is not recorded.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/crm/deals", nil))
	// A refused call is recorded as permission_denied.
	rec.RecordDenied(ctx, "accounting:ledger:write", "POST", "/api/v1/accounting/journals", "1.2.3.4", "ua")
	// A rejected HTTP call (4xx) is recorded as error.
	rec.LogAPI("tickets", "/api/v1/tickets", nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400) })).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("DELETE", "/api/v1/tickets/tickets/t1", nil))

	// Export beacon: allowed for a permitted service, refused otherwise.
	perms := map[string]string{"crm": "crm:deal:read"}
	post := func(body string, can bool) int {
		rr := httptest.NewRecorder()
		rec.ClientEventHandler(perms, func(context.Context, string) bool { return can }).
			ServeHTTP(rr, httptest.NewRequest("POST", "/api/v1/audit/client-event", strings.NewReader(body)))
		return rr.Code
	}
	if c := post(`{"service":"crm","kind":"leads.csv","count":3}`, true); c != 200 {
		t.Errorf("export beacon = %d", c)
	}
	if c := post(`{"service":"crm","kind":"leads.csv"}`, false); c != 403 {
		t.Errorf("beacon without permission = %d, want 403", c)
	}
	if c := post(`{"service":"drive","kind":"x"}`, true); c != 400 {
		t.Errorf("beacon unknown service = %d, want 400", c)
	}

	all, err := repo.List(ctx, org, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("events = %d, want 4 (stage, denied, tickets delete, export): %+v", len(all), all)
	}
	byAction := map[string]Event{}
	for _, e := range all {
		byAction[e.Service+"/"+e.Action] = e
	}
	if e := byAction["crm/stage"]; e.ResourceType != "deal" || e.ResourceID != "d1" || e.Status != "ok" || e.ActorEmail != "m@x.test" || e.Detail["note"] == nil {
		t.Errorf("crm/stage = %+v", e)
	}
	if e := byAction["tickets/delete"]; e.Status != "error" || e.ResourceID != "t1" {
		t.Errorf("tickets/delete = %+v", e)
	}
	if e := byAction["crm/export"]; e.ResourceType != "leads.csv" {
		t.Errorf("crm/export = %+v", e)
	}
	denied, err := repo.List(ctx, org, Filter{Denied: true})
	if err != nil || len(denied) != 1 {
		t.Fatalf("denied filter = %d, %v", len(denied), err)
	}
	if d := denied[0]; d.Service != "accounting" || d.Action != ActionDenied || d.Detail["permission"] != "accounting:ledger:write" || d.IP != "1.2.3.4" {
		t.Errorf("denied = %+v", d)
	}

	// gRPC: a refund is recorded with its detail.
	ic := NewInterceptor(rec)
	info := &grpc.UnaryServerInfo{FullMethod: "/adrient.v1.PosService/RefundSale"}
	_, _ = ic(ctx, &adrientv1.RefundPosSaleRequest{Id: "s9", Method: "cash"}, info, func(context.Context, any) (any, error) { return nil, nil })
	ev, _ := repo.List(ctx, org, Filter{Service: "pos", Action: "refund"})
	if len(ev) != 1 || ev[0].ResourceType != "sale" || ev[0].ResourceID != "s9" || ev[0].Detail["method"] != "cash" {
		t.Errorf("refund event = %+v", ev)
	}
}
