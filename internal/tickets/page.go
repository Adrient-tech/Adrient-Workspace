package tickets

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
)

// ListTicketsPage returns up to limit tickets of a project, newest first,
// strictly after the keyset (beforeTime, beforeID). q matches the title,
// requester and "#number".
func (r *Repository) ListTicketsPage(ctx context.Context, orgID, projectID, status, q string, limit int, beforeTime time.Time, beforeID string) ([]Ticket, error) {
	args := []any{projectID, orgID}
	where := `t.project_id = $1 AND t.org_id = $2`
	if strings.TrimSpace(status) != "" {
		args = append(args, status)
		where += fmt.Sprintf(` AND t.status = $%d`, len(args))
	}
	if q = strings.TrimSpace(q); q != "" {
		args = append(args, "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)+"%", strings.TrimPrefix(q, "#"))
		where += fmt.Sprintf(` AND (t.title ILIKE $%[1]d OR t.requester_name ILIKE $%[1]d OR t.requester_email ILIKE $%[1]d OR t.number::text = $%[2]d)`, len(args)-1, len(args))
	}
	if !beforeTime.IsZero() && beforeID != "" {
		args = append(args, beforeTime, beforeID)
		where += fmt.Sprintf(` AND (t.created_at, t.id) < ($%d, $%d::uuid)`, len(args)-1, len(args))
	}
	rows, err := r.pool.Query(ctx, `SELECT t.id::text, t.project_id::text, t.org_id::text, p.key, t.number, t.title, t.body,
	             t.status, t.priority, t.requester_user_id::text, t.requester_name, t.requester_email,
	             t.assignee_user_id::text, t.source, t.created_at, t.updated_at
	        FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id = t.project_id
	       WHERE `+where+fmt.Sprintf(` ORDER BY t.created_at DESC, t.id DESC LIMIT %d`, limit), args...)
	if err != nil {
		return nil, fmt.Errorf("tickets.ListTicketsPage: %w", err)
	}
	defer rows.Close()
	var out []Ticket
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ticketsPage answers GET /projects/{id}/tickets when ?limit= or ?cursor= is
// given: {tickets, items, next_cursor}, limit default 50, cap 100.
func (h *HTTPHandler) ticketsPage(w http.ResponseWriter, r *http.Request, orgID, projectID string) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	var bt time.Time
	var bid string
	if parts, err := pagekit.DecodeCursor(q.Get("cursor")); err == nil && len(parts) == 2 {
		if t, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
			bt, bid = t, parts[1]
		}
	}
	list, err := h.repo.ListTicketsPage(r.Context(), orgID, projectID, q.Get("status"), q.Get("q"), limit+1, bt, bid)
	if err != nil {
		httpErr(w, err)
		return
	}
	next := ""
	if len(list) > limit {
		list = list[:limit]
		last := list[limit-1]
		next = pagekit.EncodeCursor(last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID)
	}
	out := make([]map[string]any, 0, len(list))
	for _, t := range list {
		out = append(out, ticketJSON(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": out, "items": out, "next_cursor": next})
}
