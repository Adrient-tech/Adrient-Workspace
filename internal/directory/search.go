package directory

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
)

// SearchPath is where SearchHandler is mounted.
const SearchPath = "/api/v1/directory/search"

// MaxSearchLimit is the hard cap of one page.
const MaxSearchLimit = 50

// Entry is one picker row. Type is user, group or unit.
type Entry struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	Department string `json:"department,omitempty"`
	JobTitle   string `json:"job_title,omitempty"`
	EmployeeID string `json:"employee_id,omitempty"`
	UPN        string `json:"upn,omitempty"`
	Count      int    `json:"count,omitempty"`  // members of a group / unit
	Topics     int    `json:"topics,omitempty"` // groups: discussion topics
	Detail     string `json:"detail,omitempty"` // groups: description
	Status     string `json:"status,omitempty"`
}

// SearchHandler implements GET /api/v1/directory/search for async pickers:
//
//	q       text matched against name, email, employee id, UPN and department
//	type    user (default) | group | unit
//	limit   1..50 (default 20)
//	cursor  opaque keyset cursor from the previous page
//	ids     comma separated ids to hydrate (selected values), no paging
//	status  "any" also lists blocked users (default hides them)
//
// Results are always scoped to the caller's org and ordered by name then id,
// so the cursor stays stable when names repeat.
type SearchHandler struct {
	pool *pgxpool.Pool
	// Visible limits results for a restricted caller (a guest); nil = none.
	Visible func(ctx context.Context) (ids map[string]bool, restricted bool)
}

// NewSearchHandler constructs the handler.
func NewSearchHandler(pool *pgxpool.Pool) *SearchHandler { return &SearchHandler{pool: pool} }

func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := auth.UserFromContext(r.Context()); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	org, ok := auth.OrgFromContext(r.Context())
	if !ok {
		http.Error(w, "no org context", http.StatusInternalServerError)
		return
	}
	p := pagekit.Parse(r, 20, MaxSearchLimit, nil, "type", "status", "ids")
	var visible map[string]bool
	restricted := false
	if h.Visible != nil {
		visible, restricted = h.Visible(r.Context())
	}
	var (
		items []Entry
		next  string
		err   error
	)
	switch p.Filter["type"] {
	case "group":
		items, next, err = h.searchGroups(r.Context(), org.ID, p)
	case "unit":
		items, next, err = h.searchUnits(r.Context(), org.ID, p)
	case "", "user":
		items, next, err = h.searchUsers(r.Context(), org.ID, p, visible, restricted)
	default:
		http.Error(w, "unknown type", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []Entry{}
	}
	pagekit.Write(w, http.StatusOK, pagekit.Envelope{Items: items, NextCursor: next})
}

func splitIDs(s string) []string {
	var out []string
	for _, id := range strings.Split(s, ",") {
		id = strings.TrimSpace(id)
		if len(id) == 36 && len(out) < MaxSearchLimit {
			out = append(out, id)
		}
	}
	return out
}

const nameKey = `lower(COALESCE(NULLIF(u.display_name,''), u.email))`

func (h *SearchHandler) searchUsers(ctx context.Context, orgID string, p pagekit.Params, visible map[string]bool, restricted bool) ([]Entry, string, error) {
	args := []any{orgID}
	where := "u.org_id = $1"
	if p.Filter["ids"] != "" {
		p.Limit = MaxSearchLimit
	}
	add := func(sql string, v any) int {
		args = append(args, v)
		return len(args)
	}
	if ids := splitIDs(p.Filter["ids"]); p.Filter["ids"] != "" {
		n := add("", ids)
		where += " AND u.id = ANY($" + itoa(n) + "::uuid[])"
	} else {
		if p.Filter["status"] != "any" {
			where += " AND u.status <> 'blocked'"
		}
		if p.Q != "" {
			n := add("", "%"+pagekit.EscapeLike(p.Q)+"%")
			where += " AND (u.display_name ILIKE $" + itoa(n) + " OR u.email ILIKE $" + itoa(n) +
				" OR u.employee_id ILIKE $" + itoa(n) + " OR u.upn ILIKE $" + itoa(n)
			// Departments are matched by id so the planner can combine the
			// trigram indexes with the department index in one bitmap OR.
			if depIDs := h.matchingUnits(ctx, orgID, p.Q); len(depIDs) > 0 {
				d := add("", depIDs)
				where += " OR u.department_id = ANY($" + itoa(d) + "::uuid[])"
			}
			where += ")"
		}
		if len(p.Cursor) == 2 {
			a := add("", p.Cursor[0])
			b := add("", p.Cursor[1])
			where += " AND (" + nameKey + ", u.id) > ($" + itoa(a) + ", $" + itoa(b) + "::uuid)"
		}
	}
	if restricted {
		ids := make([]string, 0, len(visible))
		for id := range visible {
			ids = append(ids, id)
		}
		n := add("", ids)
		where += " AND u.id = ANY($" + itoa(n) + "::uuid[])"
	}
	n := add("", p.Limit+1)
	rows, err := h.pool.Query(ctx, `SELECT u.id::text, COALESCE(NULLIF(u.display_name,''), u.email), u.email,
			COALESCE(d.name,''), u.job_title, u.employee_id, COALESCE(u.upn,''), u.status, `+nameKey+`
		FROM adrient.users u LEFT JOIN adrient.departments d ON d.id = u.department_id
		WHERE `+where+` ORDER BY `+nameKey+`, u.id LIMIT $`+itoa(n), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []Entry
	var keys []string
	for rows.Next() {
		e := Entry{Type: "user"}
		var key string
		if err := rows.Scan(&e.ID, &e.Name, &e.Email, &e.Department, &e.JobTitle, &e.EmployeeID, &e.UPN, &e.Status, &key); err != nil {
			return nil, "", err
		}
		out = append(out, e)
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return trim(out, keys, p.Limit, p.Filter["ids"] != "")
}

func (h *SearchHandler) searchGroups(ctx context.Context, orgID string, p pagekit.Params) ([]Entry, string, error) {
	args := []any{orgID}
	where := "g.org_id = $1"
	if p.Q != "" {
		args = append(args, "%"+pagekit.EscapeLike(p.Q)+"%")
		where += " AND (g.name ILIKE $" + itoa(len(args)) + " OR g.email ILIKE $" + itoa(len(args)) + ")"
	}
	if len(p.Cursor) == 2 {
		args = append(args, p.Cursor[0], p.Cursor[1])
		where += " AND (lower(g.name), g.id) > ($" + itoa(len(args)-1) + ", $" + itoa(len(args)) + "::uuid)"
	}
	args = append(args, p.Limit+1)
	rows, err := h.pool.Query(ctx, `SELECT g.id::text, g.name, g.email, jsonb_array_length(g.member_ids), lower(g.name),
			g.description, (SELECT count(*) FROM adrient.group_topics t WHERE t.group_id = g.id)
		FROM adrient.groups g WHERE `+where+` ORDER BY lower(g.name), g.id LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []Entry
	var keys []string
	for rows.Next() {
		e := Entry{Type: "group"}
		var key string
		if err := rows.Scan(&e.ID, &e.Name, &e.Email, &e.Count, &key, &e.Detail, &e.Topics); err != nil {
			return nil, "", err
		}
		out = append(out, e)
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return trim(out, keys, p.Limit, false)
}

func (h *SearchHandler) searchUnits(ctx context.Context, orgID string, p pagekit.Params) ([]Entry, string, error) {
	args := []any{orgID}
	where := "d.org_id = $1"
	if p.Q != "" {
		args = append(args, "%"+pagekit.EscapeLike(p.Q)+"%")
		where += " AND d.name ILIKE $" + itoa(len(args))
	}
	if len(p.Cursor) == 2 {
		args = append(args, p.Cursor[0], p.Cursor[1])
		where += " AND (lower(d.name), d.id) > ($" + itoa(len(args)-1) + ", $" + itoa(len(args)) + "::uuid)"
	}
	args = append(args, p.Limit+1)
	rows, err := h.pool.Query(ctx, `SELECT d.id::text, d.name, COALESCE(pd.name,''), lower(d.name)
		FROM adrient.departments d LEFT JOIN adrient.departments pd ON pd.id = d.parent_id
		WHERE `+where+` ORDER BY lower(d.name), d.id LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []Entry
	var keys []string
	for rows.Next() {
		e := Entry{Type: "unit"}
		var key string
		if err := rows.Scan(&e.ID, &e.Name, &e.Department, &key); err != nil {
			return nil, "", err
		}
		out = append(out, e)
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return trim(out, keys, p.Limit, false)
}

// trim drops the look-ahead row and builds the next cursor from the last
// returned row.
func trim(out []Entry, keys []string, limit int, noPaging bool) ([]Entry, string, error) {
	if noPaging || len(out) <= limit {
		return out, "", nil
	}
	out, keys = out[:limit], keys[:limit]
	return out, pagekit.EncodeCursor(keys[limit-1], out[limit-1].ID), nil
}

func itoa(n int) string {
	const digits = "0123456789"
	if n < 10 {
		return digits[n : n+1]
	}
	return itoa(n/10) + digits[n%10:n%10+1]
}

// matchingUnits returns the ids of up to 100 org units whose name contains q.
func (h *SearchHandler) matchingUnits(ctx context.Context, orgID, q string) []string {
	rows, err := h.pool.Query(ctx, `SELECT id::text FROM adrient.departments WHERE org_id = $1 AND name ILIKE $2 LIMIT 100`,
		orgID, "%"+pagekit.EscapeLike(q)+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
