package contacts

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
)

// PageFilter is one window of the contact list.
type PageFilter struct {
	ListFilter
	Query  string
	Limit  int
	Offset int
}

// ListPage returns one window of contacts (same order as List) and whether
// another window follows. Query matches names, company, emails and phones.
func (r *Repository) ListPage(ctx context.Context, orgID string, f PageFilter) ([]Contact, bool, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	args := []any{orgID}
	where := "c.org_id=$1 AND c.trashed_at IS NULL"
	from := "adrient.contacts c"
	if f.GroupID != "" {
		args = append(args, f.GroupID)
		from += fmt.Sprintf(" JOIN adrient.contact_group_members m ON m.contact_id = c.id AND m.group_id=$%d", len(args))
	}
	if f.StarredOnly {
		where += " AND c.starred"
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)+"%")
		n := len(args)
		where += fmt.Sprintf(` AND (c.display_name ILIKE $%[1]d OR c.first_name ILIKE $%[1]d OR c.last_name ILIKE $%[1]d
			OR c.company ILIKE $%[1]d OR c.emails::text ILIKE $%[1]d OR c.phones::text ILIKE $%[1]d)`, n)
	}
	q := `SELECT ` + qualify(columns) + ` FROM ` + from + ` WHERE ` + where +
		fmt.Sprintf(` ORDER BY c.starred DESC, lower(NULLIF(c.display_name,'')) NULLS LAST, lower(c.first_name), lower(c.last_name), c.id LIMIT %d OFFSET %d`, f.Limit+1, f.Offset)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, false, fmt.Errorf("contacts.ListPage: %w", err)
	}
	defer rows.Close()
	var out []Contact
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, false, fmt.Errorf("contacts.ListPage scan: %w", err)
		}
		out = append(out, c)
	}
	more := len(out) > f.Limit
	if more {
		out = out[:f.Limit]
	}
	return out, more, rows.Err()
}

// qualify prefixes every column of a comma list with "c.".
func qualify(list string) string {
	parts := strings.Split(list, ",")
	for i, p := range parts {
		parts[i] = "c." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

// servePage answers GET /api/v1/cx/page?q=&group_id=&starred=1&limit=&page=
// with {items, next_cursor} (next_cursor is the next page number).
func (h *ExtrasHandler) servePage(w http.ResponseWriter, r *http.Request, org string) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	list, more, err := h.repo.ListPage(r.Context(), org, PageFilter{
		ListFilter: ListFilter{GroupID: q.Get("group_id"), StarredOnly: q.Get("starred") == "1"},
		Query:      q.Get("q"), Limit: limit, Offset: (page - 1) * limit,
	})
	if err != nil {
		fail(w, err)
		return
	}
	mo := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i, c := range list {
		if i > 0 {
			b.WriteString(",")
		}
		j, err := mo.Marshal(toProto(c))
		if err != nil {
			fail(w, err)
			return
		}
		b.Write(j)
	}
	next := ""
	if more {
		next = strconv.Itoa(page + 1)
	}
	fmt.Fprintf(&b, `],"next_cursor":%q}`, next)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(b.String()))
}
