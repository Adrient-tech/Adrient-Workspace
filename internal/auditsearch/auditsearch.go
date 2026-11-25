// Package auditsearch adds free-text and date-range search plus CSV export to
// the admin audit log (the plain list endpoint only filters by exact values).
package auditsearch

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Path is the endpoint (GET, admin only; add &format=csv to download).
const Path = "/api/v1/admin/audit/search"

// Handler searches adrient.audit_events of the caller's org.
type Handler struct {
	Pool *pgxpool.Pool
	// OnExport records that an export happened (best effort).
	OnExport func(ctx context.Context, orgID, actorID string, rows int)
}

type row struct {
	ID         string         `json:"id"`
	At         time.Time      `json:"created_at"`
	ActorEmail string         `json:"actor_email"`
	Service    string         `json:"service"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	Status     string         `json:"status"`
	IP         string         `json:"ip"`
	Detail     map[string]any `json:"detail"`
}

func parseTime(s string, endOfDay bool) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			if layout == "2006-01-02" && endOfDay {
				t = t.Add(24*time.Hour - time.Second)
			}
			return &t, nil
		}
	}
	return nil, fmt.Errorf("dates look like 2026-11-30")
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	u, ok2 := auth.UserFromContext(r.Context())
	if !ok || !ok2 || r.Method != http.MethodGet {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if !authz.IsAdmin(r.Context()) && !authz.Can(r.Context(), "admin:audit:read") {
		authz.WriteForbidden(w, "admin:audit:read")
		return
	}
	q := r.URL.Query()
	from, err1 := parseTime(q.Get("from"), false)
	to, err2 := parseTime(q.Get("to"), true)
	if err1 != nil || err2 != nil {
		http.Error(w, `{"error":"dates look like 2026-11-30"}`, http.StatusBadRequest)
		return
	}
	limit := 200
	csvOut := q.Get("format") == "csv"
	if csvOut {
		limit = 10000
	}
	args := []any{o.ID}
	where := "org_id::text=$1"
	add := func(cond string, v any) {
		args = append(args, v)
		where += " AND " + strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))) // the same value may be used several times
	}
	if t := strings.TrimSpace(q.Get("q")); t != "" {
		if len(t) > 100 {
			t = t[:100]
		}
		pat := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(t)) + "%"
		add(`(lower(actor_email) LIKE ? ESCAPE '\' OR lower(action) LIKE ? ESCAPE '\' OR lower(resource_id) LIKE ? ESCAPE '\' OR lower(detail::text) LIKE ? ESCAPE '\')`, pat)
	}
	if v := strings.TrimSpace(q.Get("service")); v != "" {
		add("service = ?", v)
	}
	if v := strings.TrimSpace(q.Get("actor")); v != "" {
		add("lower(actor_email) = ?", strings.ToLower(v))
	}
	if v := strings.TrimSpace(q.Get("action")); v != "" {
		add("action = ?", v)
	}
	if q.Get("denied") == "1" {
		where += " AND action = 'permission_denied'"
	}
	if from != nil {
		add("created_at >= ?", *from)
	}
	if to != nil {
		add("created_at <= ?", *to)
	}
	args = append(args, limit)
	rows, err := h.Pool.Query(r.Context(), `SELECT id::text, created_at, actor_email, service, action, resource_type || CASE WHEN resource_id<>'' THEN ':' || resource_id ELSE '' END,
status, ip, detail FROM adrient.audit_events WHERE `+where+fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args)), args...)
	if err != nil {
		http.Error(w, `{"error":"search failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []row{}
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.At, &x.ActorEmail, &x.Service, &x.Action, &x.Resource, &x.Status, &x.IP, &x.Detail); err != nil {
			http.Error(w, `{"error":"search failed"}`, http.StatusInternalServerError)
			return
		}
		out = append(out, x)
	}
	if csvOut {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="audit-log.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"time", "actor", "service", "action", "resource", "status", "ip", "detail"})
		for _, x := range out {
			d, _ := json.Marshal(x.Detail)
			_ = cw.Write([]string{x.At.UTC().Format(time.RFC3339), csvSafe(x.ActorEmail), x.Service, x.Action, csvSafe(x.Resource), x.Status, x.IP, csvSafe(string(d))})
		}
		cw.Flush()
		if h.OnExport != nil {
			h.OnExport(r.Context(), o.ID, u.ID, len(out))
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": out, "truncated": len(out) == limit})
}

// csvSafe stops spreadsheet formula injection: a cell that starts with =, +, -
// or @ gets a leading apostrophe.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
