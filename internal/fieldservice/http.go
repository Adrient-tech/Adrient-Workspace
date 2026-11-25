package fieldservice

import (
	"context"
	"net/http"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

const mount = "/api/v1/fieldservice"

// HTTPHandler serves /api/v1/fieldservice/*.
type HTTPHandler struct {
	repo *Repository
	auth opshttp.AuthFuncs
}

// NewHTTPHandler builds the handler.
func NewHTTPHandler(repo *Repository, auth opshttp.AuthFuncs) *HTTPHandler {
	return &HTTPHandler{repo: repo, auth: auth}
}

// Match reports whether path belongs to Field service.
func (h *HTTPHandler) Match(path string) bool {
	return path == mount || strings.HasPrefix(path, mount+"/")
}

// TeamMember is a person who can be assigned to visits.
type TeamMember struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Team lists the organisation's users for assignment.
func (r *Repository) Team(ctx context.Context, c Caller) ([]TeamMember, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, display_name, email FROM adrient.users WHERE org_id=$1 ORDER BY display_name LIMIT 500`, c.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TeamMember{}
	for rows.Next() {
		var t TeamMember
		if err := rows.Scan(&t.ID, &t.Name, &t.Email); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok := h.auth.OrgID(r)
	user, ok2 := h.auth.UserID(r)
	if !ok || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	c := Caller{OrgID: org, UserID: user, Admin: h.auth.IsAdmin != nil && h.auth.IsAdmin(r)}
	ctx, m, q := r.Context(), r.Method, r.URL.Query()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, mount), "/"), "/")
	n := len(seg)
	rep := func(key string, v any, err error) { opshttp.Reply(w, "fieldservice", key, v, err) }
	post := m == http.MethodPost
	put := m == http.MethodPut

	switch {
	case n == 1 && seg[0] == "me" && m == http.MethodGet:
		opshttp.JSON(w, 200, map[string]any{"user_id": user, "admin": c.Admin})
	case n == 1 && seg[0] == "team" && m == http.MethodGet:
		v, err := h.repo.Team(ctx, c)
		rep("team", v, err)
	case n == 1 && seg[0] == "settings" && m == http.MethodGet:
		s, _, err := h.repo.GetSettings(ctx, org)
		rep("", s, err)
	case n == 1 && seg[0] == "settings" && put:
		var s Settings
		if !opshttp.Decode(w, r, &s) {
			return
		}
		v, err := h.repo.SaveSettings(ctx, c, s)
		rep("", v, err)
	case n == 1 && seg[0] == "dashboard" && m == http.MethodGet:
		v, err := h.repo.Dashboard(ctx, c)
		rep("", v, err)

	case n == 1 && seg[0] == "customers" && m == http.MethodGet:
		v, err := h.repo.Customers(ctx, c, q.Get("q"))
		rep("customers", v, err)
	case n == 1 && seg[0] == "customers" && post:
		var in opshttp.ContactRef
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.CreateCustomer(ctx, c, in)
		rep("", v, err)

	case n == 1 && seg[0] == "services" && m == http.MethodGet:
		v, err := h.repo.ListServices(ctx, c)
		rep("services", v, err)
	case (n == 1 && seg[0] == "services" && post) || (n == 2 && seg[0] == "services" && put):
		var in Service
		if !opshttp.Decode(w, r, &in) {
			return
		}
		id := ""
		if n == 2 {
			id = seg[1]
		}
		v, err := h.repo.SaveService(ctx, c, id, in)
		rep("", v, err)

	case n == 1 && seg[0] == "properties" && m == http.MethodGet:
		v, err := h.repo.ListProperties(ctx, c, q.Get("contact"))
		rep("properties", v, err)
	case (n == 1 && seg[0] == "properties" && post) || (n == 2 && seg[0] == "properties" && put):
		var in Property
		if !opshttp.Decode(w, r, &in) {
			return
		}
		id := ""
		if n == 2 {
			id = seg[1]
		}
		v, err := h.repo.SaveProperty(ctx, c, id, in)
		rep("", v, err)

	case n == 1 && seg[0] == "requests" && m == http.MethodGet:
		v, err := h.repo.ListRequests(ctx, c, q.Get("status"))
		rep("requests", v, err)
	case (n == 1 && seg[0] == "requests" && post) || (n == 2 && seg[0] == "requests" && put):
		var in Request
		if !opshttp.Decode(w, r, &in) {
			return
		}
		id := ""
		if n == 2 {
			id = seg[1]
		}
		v, err := h.repo.SaveRequest(ctx, c, id, in)
		rep("", v, err)
	case n == 3 && seg[0] == "requests" && seg[2] == "status" && post:
		var in struct {
			Status string `json:"status"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.MoveRequest(ctx, c, seg[1], in.Status)
		rep("", v, err)
	case n == 3 && seg[0] == "requests" && seg[2] == "quote" && post:
		v, err := h.repo.CreateQuote(ctx, c, seg[1])
		rep("", v, err)
	case n == 3 && seg[0] == "requests" && seg[2] == "quote" && m == http.MethodGet:
		d, err := h.repo.docs.Get(ctx, org, "request", seg[1])
		if err != nil {
			opshttp.Fail(w, "fieldservice", err)
			return
		}
		rq := h.repo.requestOf(ctx, org, d)
		opshttp.JSON(w, 200, map[string]any{"quote_id": rq.QuoteID, "quote_number": rq.QuoteNumber, "status": h.repo.QuoteStatus(ctx, c, rq)})
	case n == 3 && seg[0] == "requests" && seg[2] == "job" && post:
		var in struct {
			Schedule  Schedule `json:"schedule"`
			Assignees []string `json:"assignees"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.CreateJobFromRequest(ctx, c, seg[1], in.Schedule, in.Assignees)
		rep("", v, err)

	case n == 1 && seg[0] == "jobs" && m == http.MethodGet:
		v, err := h.repo.ListJobs(ctx, c, JobFilter{Status: q.Get("status"), Assignee: q.Get("assignee"), ContactID: q.Get("contact")})
		rep("jobs", v, err)
	case (n == 1 && seg[0] == "jobs" && post) || (n == 2 && seg[0] == "jobs" && put):
		var in Job
		if !opshttp.Decode(w, r, &in) {
			return
		}
		id := ""
		if n == 2 {
			id = seg[1]
		}
		v, err := h.repo.SaveJob(ctx, c, id, in)
		rep("", v, err)
	case n == 2 && seg[0] == "jobs" && m == http.MethodGet:
		j, err := h.repo.GetJob(ctx, c, seg[1])
		if err != nil {
			opshttp.Fail(w, "fieldservice", err)
			return
		}
		opshttp.JSON(w, 200, map[string]any{"job": j, "invoices": h.repo.InvoiceStatus(ctx, c, j), "property": h.repo.propertyLabel(ctx, org, j.PropertyID)})
	case n == 3 && seg[0] == "jobs" && seg[2] == "cancel" && post:
		v, err := h.repo.CancelJob(ctx, c, seg[1])
		rep("", v, err)
	case n == 3 && seg[0] == "jobs" && seg[2] == "invoice" && post:
		d, k, err := h.repo.InvoiceJob(ctx, c, seg[1])
		rep("", map[string]any{"document": d, "visits": k}, err)

	case n == 1 && seg[0] == "visits" && m == http.MethodGet:
		v, err := h.repo.ListVisits(ctx, c, q.Get("from"), q.Get("to"), q.Get("assignee"))
		rep("visits", v, err)
	case n == 2 && seg[0] == "visits" && put:
		var in VisitUpdate
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.UpdateVisit(ctx, c, seg[1], in)
		rep("", v, err)
	case n == 3 && seg[0] == "visits" && seg[2] == "complete" && post:
		v, d, err := h.repo.CompleteVisit(ctx, c, seg[1])
		rep("", map[string]any{"visit": v, "invoice": d}, err)
	case n == 3 && seg[0] == "visits" && seg[2] == "reopen" && post:
		v, err := h.repo.ReopenVisit(ctx, c, seg[1])
		rep("", v, err)
	case n == 3 && seg[0] == "visits" && seg[2] == "cancel" && post:
		v, err := h.repo.CancelVisit(ctx, c, seg[1])
		rep("", v, err)
	default:
		http.NotFound(w, r)
	}
}
