package authz

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Permissions enforced on the business modules.
const (
	AccountingRead  = "accounting:ledger:read"
	AccountingWrite = "accounting:ledger:write"

	InvoicingRead  = "invoicing:document:read"
	InvoicingWrite = "invoicing:document:write"

	CRMRead  = "crm:deal:read"
	CRMWrite = "crm:deal:write"

	InventoryRead   = "inventory:stock:read"
	InventoryWrite  = "inventory:product:write"
	InventoryAdjust = "inventory:stock:adjust"

	PosRead          = "pos:read"
	PosSell          = "pos:sell"
	PosShiftOpen     = "pos:shift:open"
	PosShiftClose    = "pos:shift:close"
	PosCashMove      = "pos:cash:move"
	PosRefund        = "pos:refund"
	PosSettingsWrite = "pos:settings:write"
	PosBalanceAdjust = "pos:balance:adjust"
	PosReport        = "pos:report:read"
)

func isWrite(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func rw(method, read, write string) string {
	if isWrite(method) {
		return write
	}
	return read
}

// PermForHTTP maps an API request to the permission it needs. guarded is false
// for paths outside the five business modules.
func PermForHTTP(method, path string) (perm string, guarded bool) {
	switch {
	case path == "/api/v1/accounting" || strings.HasPrefix(path, "/api/v1/accounting/"):
		return rw(method, AccountingRead, AccountingWrite), true
	case path == "/api/v1/invoicing" || strings.HasPrefix(path, "/api/v1/invoicing/"):
		return rw(method, InvoicingRead, InvoicingWrite), true
	case path == "/api/v1/crm" || strings.HasPrefix(path, "/api/v1/crm/"):
		return rw(method, CRMRead, CRMWrite), true
	case strings.HasPrefix(path, "/api/v1/inventory/"):
		if strings.HasSuffix(path, "/adjust") || strings.HasSuffix(path, "/transfer") {
			return InventoryAdjust, true
		}
		return rw(method, InventoryRead, InventoryWrite), true
	case path == "/api/v1/stock" || strings.HasPrefix(path, "/api/v1/stock/"):
		return rw(method, InventoryRead, InventoryAdjust), true
	case path == "/api/v1/purchasing" || strings.HasPrefix(path, "/api/v1/purchasing/"):
		return rw(method, InventoryRead, InventoryWrite), true
	case strings.HasPrefix(path, "/api/v1/pos/"):
		return posHTTP(method, strings.TrimPrefix(path, "/api/v1/pos/")), true
	}
	return "", false
}

func posHTTP(method, p string) string {
	seg := strings.Split(strings.Trim(p, "/"), "/")
	last := seg[len(seg)-1]
	if seg[0] == "x" {
		return posExtHTTP(method, seg[1:])
	}
	switch {
	case last == "refund":
		return PosRefund
	case seg[0] == "shifts" && last == "close":
		return PosShiftClose
	case seg[0] == "shifts" && last == "cash":
		return PosCashMove
	case seg[0] == "registers" && last == "shift":
		return PosShiftOpen
	case seg[0] == "sales" && isWrite(method):
		return PosSell
	case seg[0] == "balances" && last == "adjust":
		return PosBalanceAdjust
	case seg[0] == "report":
		return PosReport
	case isWrite(method): // registers, taxes, discounts, settings
		return PosSettingsWrite
	}
	return PosRead
}

// posExtHTTP maps the wave-3 routes under /api/v1/pos/x/. A cashier can take
// sales, void/refund with a manager PIN and sell gift cards; only managers set
// their own PIN or change settings.
func posExtHTTP(method string, seg []string) string {
	if len(seg) == 0 {
		return PosRead
	}
	switch seg[0] {
	case "sales":
		return PosSell
	case "zreport":
		return PosReport
	case "pin":
		if len(seg) == 2 && seg[1] == "verify" {
			return PosSell
		}
		if isWrite(method) {
			return PosRefund
		}
		return PosRead
	case "gift-cards":
		if len(seg) == 3 && seg[2] == "status" {
			return PosSettingsWrite
		}
		if isWrite(method) {
			return PosSell
		}
		return PosRead
	}
	if isWrite(method) { // settings, modifiers, favourites, stores
		return PosSettingsWrite
	}
	return PosRead
}

var rpcPerms = map[string]string{
	// POS
	"/adrient.v1.PosService/CloseShift":           PosShiftClose,
	"/adrient.v1.PosService/CashMove":             PosCashMove,
	"/adrient.v1.PosService/OpenShift":            PosShiftOpen,
	"/adrient.v1.PosService/HoldSale":             PosSell,
	"/adrient.v1.PosService/CompleteSale":         PosSell,
	"/adrient.v1.PosService/DeleteHeldSale":       PosSell,
	"/adrient.v1.PosService/RefundSale":           PosRefund,
	"/adrient.v1.PosService/CreateRegister":       PosSettingsWrite,
	"/adrient.v1.PosService/UpdateRegister":       PosSettingsWrite,
	"/adrient.v1.PosService/ArchiveRegister":      PosSettingsWrite,
	"/adrient.v1.PosService/CreateTaxRate":        PosSettingsWrite,
	"/adrient.v1.PosService/UpdateTaxRate":        PosSettingsWrite,
	"/adrient.v1.PosService/DeleteTaxRate":        PosSettingsWrite,
	"/adrient.v1.PosService/CreateDiscountPreset": PosSettingsWrite,
	"/adrient.v1.PosService/UpdateDiscountPreset": PosSettingsWrite,
	"/adrient.v1.PosService/DeleteDiscountPreset": PosSettingsWrite,
	"/adrient.v1.PosService/SaveSettings":         PosSettingsWrite,
	"/adrient.v1.PosService/AdjustBalance":        PosBalanceAdjust,
	"/adrient.v1.PosService/Report":               PosReport,
	// Inventory
	"/adrient.v1.InventoryService/AdjustStock":   InventoryAdjust,
	"/adrient.v1.InventoryService/TransferStock": InventoryAdjust,
}

// PermForRPC maps a gRPC full method to its permission. Pos and Inventory
// methods not listed fall back to read (List/Get) or write (anything else).
func PermForRPC(fullMethod string) (perm string, guarded bool) {
	if p, ok := rpcPerms[fullMethod]; ok {
		return p, true
	}
	var read, write string
	switch {
	case strings.HasPrefix(fullMethod, "/adrient.v1.PosService/"):
		read, write = PosRead, PosSettingsWrite
	case strings.HasPrefix(fullMethod, "/adrient.v1.InventoryService/"):
		read, write = InventoryRead, InventoryWrite
	default:
		return "", false
	}
	name := fullMethod[strings.LastIndex(fullMethod, "/")+1:]
	for _, pre := range []string{"List", "Get", "Reorder", "Report"} {
		if strings.HasPrefix(name, pre) {
			return read, true
		}
	}
	return write, true
}

// Denial describes one refused call, for the audit trail.
type Denial struct {
	Permission string
	Method     string // HTTP verb, or "GRPC"
	Target     string // URL path or full gRPC method
	IP         string
	UserAgent  string
}

var denyHook atomic.Pointer[func(ctx context.Context, d Denial)]

// SetDenyHook installs fn, called (synchronously, best-effort) for every
// refused guarded call. server.go wires it to the audit recorder.
func SetDenyHook(fn func(ctx context.Context, d Denial)) {
	if fn == nil {
		denyHook.Store(nil)
		return
	}
	denyHook.Store(&fn)
}

func reportDenied(ctx context.Context, d Denial) {
	if f := denyHook.Load(); f != nil {
		(*f)(ctx, d)
	}
}

func httpClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// WriteForbidden writes the 403 JSON body naming the missing permission.
func WriteForbidden(w http.ResponseWriter, perm string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":      "forbidden",
		"message":    "You do not have permission to do this.",
		"permission": perm,
	})
}

// Guard wraps next and enforces CanOpenApp (AppGuard) and then PermForHTTP on
// every guarded business route.
// It must run inside the auth middleware (it reads the caller from context).
func Guard(next http.Handler) http.Handler {
	return AppGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if perm, guarded := PermForHTTP(r.Method, r.URL.Path); guarded && !Can(r.Context(), perm) {
			reportDenied(r.Context(), Denial{Permission: perm, Method: r.Method, Target: r.URL.Path, IP: httpClientIP(r), UserAgent: r.UserAgent()})
			WriteForbidden(w, perm)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// UnaryInterceptor enforces PermForRPC on Pos and Inventory gRPC calls.
func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if app := AppForRPC(info.FullMethod); app != "" && !CanOpenApp(ctx, app) {
			reportDenied(ctx, Denial{Permission: appPerm(app), Method: "GRPC", Target: info.FullMethod})
			return nil, status.Error(codes.PermissionDenied, "forbidden: no access to app "+app)
		}
		if perm, guarded := PermForRPC(info.FullMethod); guarded && !Can(ctx, perm) {
			reportDenied(ctx, Denial{Permission: perm, Method: "GRPC", Target: info.FullMethod})
			return nil, status.Error(codes.PermissionDenied, "forbidden: missing permission "+perm)
		}
		return h(ctx, req)
	}
}

// MeHandler serves GET /api/v1/me/permissions for the UI.
func MeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s := Default()
	if s == nil {
		http.Error(w, "authz unavailable", http.StatusServiceUnavailable)
		return
	}
	set, err := s.Resolve(r.Context())
	if err != nil {
		if err == errNoCaller {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.Error(w, "authz error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"roles":       nonNil(set.Roles),
		"permissions": nonNil(set.Patterns()),
		"admin":       set.Can(PermOrgAdmin),
	})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
