package hr

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Employee list paging. The sort keys map to fixed SQL so user input never
// reaches the ORDER BY; the id tiebreak keeps pages stable.
var employeeSort = map[string]string{
	"name":       "lower(e.first_name), lower(e.last_name)",
	"code":       "lower(e.emp_code)",
	"department": "lower(coalesce(d.name,''))",
	"join_date":  "e.join_date",
	"status":     "e.status",
}

// MaxEmployeePage bounds one page of the employee list.
const MaxEmployeePage = 200

func (f EmployeeFilter) orderLimit() string {
	expr, ok := employeeSort[f.Sort]
	if !ok {
		expr = employeeSort["name"]
	}
	dir := "ASC"
	if f.Desc {
		dir = "DESC"
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > MaxEmployeePage+1 {
		limit = MaxEmployeePage + 1
	}
	off := f.Offset
	if off < 0 {
		off = 0
	}
	return fmt.Sprintf(" ORDER BY %s %s, e.id %s LIMIT %d OFFSET %d", expr, dir, dir, limit, off)
}

// pageArgs reads ?limit= (default def, max cap) and ?page= (1-based).
func pageArgs(r *http.Request, def, limitCap int) (limit, page int) {
	q := r.URL.Query()
	limit, _ = strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = def
	}
	if limit > limitCap {
		limit = limitCap
	}
	page, _ = strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	return limit, page
}

// RunPayslipsPage is one window of a run's payslips (HR only), by employee
// code. q matches the name or code.
func (r *Repository) RunPayslipsPage(ctx context.Context, a Actor, runID, q string, limit, offset int) ([]Payslip, error) {
	if err := a.requireHR(); err != nil {
		return nil, err
	}
	if !isUUID(runID) {
		return nil, ErrNotFound
	}
	if limit <= 0 || limit > 501 {
		limit = 100
	}
	like := "%" + escapeLike(strings.ToLower(strings.TrimSpace(q))) + "%"
	return r.queryPayslips(ctx, fmt.Sprintf(`WHERE p.org_id=$1 AND p.run_id=$2::uuid AND (lower(p.emp_name) LIKE $3 OR lower(p.emp_code) LIKE $3)
		ORDER BY p.emp_code, p.id LIMIT %d OFFSET %d`, limit, max(offset, 0)), a.OrgID, runID, like)
}
