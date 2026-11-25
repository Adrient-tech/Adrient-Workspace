package people

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
)

// PagePath lists people with server-side search, sort and paging.
const PagePath = PeoplePrefix + "/page"

// sortSQL maps the sortable column keys to ORDER BY expressions; the id
// tiebreak keeps pages stable when values repeat.
var sortSQL = map[string]string{
	"name":        "lower(COALESCE(NULLIF(u.display_name,''), u.email))",
	"title":       "lower(u.job_title)",
	"department":  "lower(COALESCE(d.name,''))",
	"employee_id": "lower(u.employee_id)",
	"email":       "lower(u.email)",
}

// PageQuery is one list request.
type PageQuery struct {
	Q            string
	DepartmentID string
	Sort         string
	Desc         bool
	Limit        int
	Offset       int
	deptIDs      []string // departments whose name matches Q (resolved by Page)
}

func (q PageQuery) where(orgID string) (string, []any) {
	args := []any{orgID}
	where := "u.org_id = $1"
	if s := strings.TrimSpace(q.Q); s != "" {
		args = append(args, "%"+escapeLike(s)+"%")
		n := fmt.Sprint(len(args))
		where += " AND (u.display_name ILIKE $" + n + " OR u.email ILIKE $" + n + " OR u.job_title ILIKE $" + n +
			" OR u.employee_id ILIKE $" + n
		if len(q.deptIDs) > 0 {
			args = append(args, q.deptIDs)
			where += fmt.Sprintf(" OR u.department_id = ANY($%d::uuid[])", len(args))
		}
		where += ")"
	}
	if isUUID(q.DepartmentID) {
		args = append(args, q.DepartmentID)
		where += fmt.Sprintf(" AND u.department_id = $%d", len(args))
	}
	return where, args
}

// Page returns one page of people plus a count capped at pagekit.CountCap.
// hasMore reports whether a following page exists.
func (s *Store) Page(ctx context.Context, orgID string, q PageQuery) (people []Person, total int64, capped, hasMore bool, err error) {
	if q.Limit <= 0 || q.Limit > pagekit.MaxLimit {
		q.Limit = pagekit.DefaultLimit
	}
	expr, ok := sortSQL[q.Sort]
	if !ok {
		expr = sortSQL["name"]
	}
	dir := "ASC"
	if q.Desc {
		dir = "DESC"
	}
	if strings.TrimSpace(q.Q) != "" {
		q.deptIDs = s.departmentIDsLike(ctx, orgID, q.Q)
	}
	where, args := q.where(orgID)
	rows, err := s.pool.Query(ctx, personSelect+" WHERE "+where+
		fmt.Sprintf(" ORDER BY %s %s, u.id %s LIMIT %d OFFSET %d", expr, dir, dir, q.Limit+1, q.Offset), args...)
	if err != nil {
		return nil, 0, false, false, err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, 0, false, false, err
		}
		people = append(people, p)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, false, false, err
	}
	if len(people) > q.Limit {
		people, hasMore = people[:q.Limit], true
	}
	total, capped, err = pagekit.CappedCount(ctx, func(ctx context.Context, sql string, a ...any) (int64, error) {
		var n int64
		return n, s.pool.QueryRow(ctx, sql, a...).Scan(&n)
	}, "FROM adrient.users u WHERE "+where, args...)
	return people, total, capped, hasMore, err
}

// pageHandler serves PagePath: ?q=&department=&sort=&order=&page=&limit=&format=csv.
func (h *Handler) pageHandler(w http.ResponseWriter, r *http.Request, orgID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.Visible != nil {
		if _, restricted := h.Visible(r.Context()); restricted {
			// A guest only sees the people shared with them: the capped search is enough.
			list, err := h.store.Search(r.Context(), orgID, r.URL.Query().Get("q"), 50)
			list = h.filter(r.Context(), list)
			h.reply(w, err, map[string]any{"items": list, "people": list})
			return
		}
	}
	p := pagekit.Parse(r, 50, pagekit.MaxLimit, []string{"name", "title", "department", "employee_id", "email"}, "department")
	q := PageQuery{Q: p.Q, DepartmentID: p.Filter["department"], Sort: p.Sort, Desc: p.Desc, Limit: p.Limit, Offset: p.Offset()}
	if r.URL.Query().Get("format") == "csv" {
		cw := pagekit.NewCSV(w, "people.csv", []string{"name", "email", "job_title", "department", "manager", "employee_id", "location"})
		q.Limit = 500
		for off := 0; off < 200000; off += q.Limit {
			q.Offset = off
			list, _, _, more, err := h.store.Page(r.Context(), orgID, q)
			if err != nil {
				break
			}
			for _, x := range list {
				cw.Row(x.Name, x.Email, x.JobTitle, x.DepartmentName, x.ManagerName, x.EmployeeID, x.Location)
			}
			if !more {
				break
			}
		}
		cw.Close()
		return
	}
	list, total, capped, more, err := h.store.Page(r.Context(), orgID, q)
	if err != nil {
		h.reply(w, err, nil)
		return
	}
	env := pagekit.Envelope{Items: list}
	if more {
		env.NextCursor = fmt.Sprint(p.Page + 1)
	}
	env.SetTotal(total, capped)
	pagekit.Write(w, http.StatusOK, env)
}

// departmentIDsLike returns up to 100 department ids whose name contains q.
func (s *Store) departmentIDsLike(ctx context.Context, orgID, q string) []string {
	rows, err := s.pool.Query(ctx, `SELECT id::text FROM adrient.departments WHERE org_id = $1 AND name ILIKE $2 LIMIT 100`,
		orgID, "%"+escapeLike(strings.TrimSpace(q))+"%")
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
