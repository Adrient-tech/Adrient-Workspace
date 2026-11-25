package audit

import (
	"context"
	"encoding/json"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// mountPath is the public path this handler answers on.
const mountPath = "/api/v1/admin/audit"

// EmailResolver returns the caller's adrient email + org id from the request
// context, and whether a caller is present. server.go supplies a closure backed
// by auth.UserFromContext / auth.OrgFromContext, keeping this package free of
// internal/auth (and its gen/ dep) — same pattern as internal/adminusers.
type EmailResolver func(ctx context.Context) (email, orgID string, ok bool)

// AdminChecker reports whether the caller (resolved from the request context)
// holds a per-org admin role (an org_admins grant). server.go injects a closure
// backed by the org_admins repo + auth context. Optional: kept decoupled from
// gen/ exactly like EmailResolver.
type AdminChecker func(ctx context.Context) bool

// Handler serves GET /api/v1/admin/audit, an admin-gated JSON listing of the
// caller's org's audit events. Authorization (see docs/rbac-design.md): the
// caller's email must be in ADRIENT_ADMIN_EMAILS (bootstrap super-admins) OR they
// must hold an org_admins grant (via the injected AdminChecker). There is NO open
// fallback — an empty allowlist no longer grants every member access.
type Handler struct {
	repo        *Repository
	adminEmails map[string]struct{}
	resolve     EmailResolver
	isOrgAdmin  AdminChecker
}

// NewHandler constructs the audit handler. adminEmails is the raw
// ADRIENT_ADMIN_EMAILS value (comma-separated); "" leaves the allowlist empty.
func NewHandler(repo *Repository, adminEmails string) *Handler {
	allow := make(map[string]struct{})
	for _, e := range strings.Split(adminEmails, ",") {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			allow[e] = struct{}{}
		}
	}
	return &Handler{repo: repo, adminEmails: allow}
}

// WithResolver injects the caller-identity resolver and returns the handler for
// chaining. Without a resolver every request is treated as unauthenticated.
func (h *Handler) WithResolver(r EmailResolver) *Handler {
	h.resolve = r
	return h
}

// WithAdminChecker injects the per-org admin predicate and returns the handler
// for chaining. server.go calls this with a closure backed by the org_admins repo.
func (h *Handler) WithAdminChecker(c AdminChecker) *Handler {
	h.isOrgAdmin = c
	return h
}

// eventOut is the JSON shape returned to the Admin viewer.
type eventOut struct {
	ID           string         `json:"id"`
	ActorEmail   string         `json:"actor_email"`
	ActorID      string         `json:"actor_id,omitempty"`
	Service      string         `json:"service"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Method       string         `json:"method"`
	Status       string         `json:"status"`
	Detail       map[string]any `json:"detail,omitempty"`
	IP           string         `json:"ip"`
	UserAgent    string         `json:"user_agent"`
	CreatedAt    string         `json:"created_at"`
}

// ServeHTTP authorizes the caller, then lists events for their org filtered by
// the query string: ?service=&actor=&action=&denied=1&limit=&before=.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	email, orgID, ok := "", "", false
	if h.resolve != nil {
		email, orgID, ok = h.resolve(r.Context())
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no session"})
		return
	}
	// Admin iff email is in the bootstrap allowlist OR an org_admins grant exists
	// (via the injected AdminChecker). No open fallback (see docs/rbac-design.md).
	_, inAllowlist := h.adminEmails[strings.ToLower(strings.TrimSpace(email))]
	if !inAllowlist && !(h.isOrgAdmin != nil && h.isOrgAdmin(r.Context())) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin privileges required"})
		return
	}
	if orgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no org context"})
		return
	}

	q := r.URL.Query()
	f := Filter{
		Service:    strings.TrimSpace(q.Get("service")),
		ActorEmail: strings.TrimSpace(q.Get("actor")),
		ActorID:    validUUID(q.Get("actor_id")),
		Action:     strings.TrimSpace(q.Get("action")),
		Status:     strings.TrimSpace(q.Get("status")),
		ResourceID: strings.TrimSpace(q.Get("resource_id")),
		Denied:     q.Get("denied") == "1" || q.Get("denied") == "true",
	}
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			f.Limit = n
		}
	}
	parseTime := func(key string) time.Time {
		t, _ := time.Parse(time.RFC3339Nano, q.Get(key))
		return t
	}
	f.Before, f.From, f.To = parseTime("before"), parseTime("from"), parseTime("to")
	// "at" jumps to a date: show events at or before that moment.
	if at := parseTime("at"); !at.IsZero() {
		f.To = at
	}
	if c := q.Get("cursor"); c != "" {
		if parts, err := pagekit.DecodeCursor(c); err == nil && len(parts) == 2 {
			if t, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
				f.CursorTime, f.CursorID = t, validUUID(parts[1])
			}
		}
	}
	// Live tail: only events newer than the given cursor, oldest first.
	if q.Get("tail") == "1" && !f.CursorTime.IsZero() {
		f.Asc = true
	}

	if q.Get("format") == "csv" {
		cw := pagekit.NewCSV(w, "audit-log.csv", []string{"time", "actor", "service", "action", "resource_type", "resource_id", "status", "method", "ip"})
		_ = h.repo.Each(r.Context(), orgID, f, 1000000, func(e Event) error {
			cw.Row(e.CreatedAt.UTC().Format(time.RFC3339), e.ActorEmail, e.Service, e.Action, e.ResourceType, e.ResourceID, e.Status, e.Method, e.IP)
			return r.Context().Err()
		})
		cw.Close()
		return
	}

	// One extra row tells whether another page exists.
	want := f.Limit
	if want <= 0 {
		want = 100
	}
	if want > 500 {
		want = 500
	}
	f.Limit = want + 1
	events, err := h.repo.List(r.Context(), orgID, f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	next := ""
	if len(events) > want {
		events = events[:want]
		last := events[len(events)-1]
		next = pagekit.EncodeCursor(last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID)
	}
	if f.Asc && len(events) > 0 { // tail: hand rows back newest-first
		for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
			events[i], events[j] = events[j], events[i]
		}
		last := events[0]
		next = pagekit.EncodeCursor(last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID)
	}

	out := make([]eventOut, 0, len(events))
	for _, e := range events {
		out = append(out, eventOut{
			ID:           e.ID,
			ActorEmail:   e.ActorEmail,
			ActorID:      e.ActorID,
			Service:      e.Service,
			Action:       e.Action,
			ResourceType: e.ResourceType,
			ResourceID:   e.ResourceID,
			Method:       e.Method,
			Status:       e.Status,
			Detail:       e.Detail,
			IP:           e.IP,
			UserAgent:    e.UserAgent,
			CreatedAt:    e.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	resp := map[string]any{"events": out, "items": out, "next_cursor": next}
	// The total is only worth computing on the first page of a view.
	if f.CursorTime.IsZero() && q.Get("count") != "0" {
		cf := f
		cf.Before = time.Time{}
		if n, capped, err := h.repo.CountCapped(r.Context(), orgID, cf, pagekit.CountCap); err == nil {
			resp["total_estimate"], resp["total_capped"] = n, capped
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// validUUID returns s when it is a canonical UUID string, else "".
func validUUID(s string) string {
	s = strings.TrimSpace(s)
	if len(s) != 36 {
		return ""
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return ""
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return ""
		}
	}
	return s
}

// writeJSON writes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
