package timetracking

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

const mount = "/api/v1/timetracking"

// HTTPHandler serves /api/v1/timetracking/*.
type HTTPHandler struct {
	repo *Repository
	auth opshttp.AuthFuncs
}

// NewHTTPHandler builds the handler.
func NewHTTPHandler(repo *Repository, auth opshttp.AuthFuncs) *HTTPHandler {
	return &HTTPHandler{repo: repo, auth: auth}
}

// Match reports whether path belongs to Time tracking.
func (h *HTTPHandler) Match(path string) bool {
	return path == mount || strings.HasPrefix(path, mount+"/")
}

func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok := h.auth.OrgID(r)
	user, ok2 := h.auth.UserID(r)
	if !ok || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	c, err := h.repo.CallerFor(r.Context(), org, user, h.auth.IsAdmin != nil && h.auth.IsAdmin(r))
	if err != nil {
		opshttp.Fail(w, "timetracking", err)
		return
	}
	ctx, m, q := r.Context(), r.Method, r.URL.Query()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, mount), "/"), "/")
	n := len(seg)
	rep := func(key string, v any, err error) { opshttp.Reply(w, "timetracking", key, v, err) }
	filter := EntryFilter{From: q.Get("from"), To: q.Get("to"), UserID: q.Get("user"), ProjectID: q.Get("project"), ClientID: q.Get("client"), Unbilled: q.Get("unbilled") == "1"}

	switch {
	case n == 1 && seg[0] == "me" && m == http.MethodGet:
		opshttp.JSON(w, 200, map[string]any{"user_id": user, "admin": c.Admin, "hr_admin": c.HRAdmin, "employee_id": c.EmployeeID})
	case n == 1 && seg[0] == "settings" && m == http.MethodGet:
		s, _, err := h.repo.GetSettings(ctx, org)
		if err == nil && !c.Admin {
			s = Settings{Currency: s.Currency, WarnLongDayHrs: s.WarnLongDayHrs}
		}
		rep("", s, err)
	case n == 1 && seg[0] == "settings" && m == http.MethodPut:
		var s Settings
		if !opshttp.Decode(w, r, &s) {
			return
		}
		v, err := h.repo.SaveSettings(ctx, c, s)
		rep("", v, err)

	case n == 1 && seg[0] == "clients" && m == http.MethodGet:
		v, err := h.repo.ListClients(ctx, c, q.Get("archived") == "1")
		rep("clients", v, err)
	case (n == 1 && seg[0] == "clients" && m == http.MethodPost) || (n == 2 && seg[0] == "clients" && m == http.MethodPut):
		var in Client
		if !opshttp.Decode(w, r, &in) {
			return
		}
		id := ""
		if n == 2 {
			id = seg[1]
		}
		v, err := h.repo.SaveClient(ctx, c, id, in)
		rep("", v, err)
	case n == 1 && seg[0] == "projects" && m == http.MethodGet:
		v, err := h.repo.ListProjects(ctx, c, q.Get("archived") == "1")
		rep("projects", v, err)
	case (n == 1 && seg[0] == "projects" && m == http.MethodPost) || (n == 2 && seg[0] == "projects" && m == http.MethodPut):
		var in Project
		if !opshttp.Decode(w, r, &in) {
			return
		}
		id := ""
		if n == 2 {
			id = seg[1]
		}
		v, err := h.repo.SaveProject(ctx, c, id, in)
		rep("", v, err)
	case n == 1 && seg[0] == "contacts" && m == http.MethodGet:
		v, err := h.repo.Contacts(ctx, c, q.Get("q"))
		rep("contacts", v, err)

	case n == 1 && seg[0] == "entries" && m == http.MethodGet:
		v, err := h.repo.ListEntries(ctx, c, filter)
		rep("entries", v, err)
	case (n == 1 && seg[0] == "entries" && m == http.MethodPost) || (n == 2 && seg[0] == "entries" && m == http.MethodPut):
		var in Entry
		if !opshttp.Decode(w, r, &in) {
			return
		}
		id := ""
		if n == 2 {
			id = seg[1]
		}
		v, err := h.repo.SaveEntry(ctx, c, id, in)
		rep("", v, err)
	case n == 2 && seg[0] == "entries" && seg[1] == "cell" && m == http.MethodPost:
		var in struct {
			ProjectID string `json:"project_id"`
			Date      string `json:"date"`
			Seconds   int    `json:"seconds"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.SetCell(ctx, c, in.ProjectID, in.Date, in.Seconds)
		rep("", map[string]int{"seconds": v}, err)
	case n == 2 && seg[0] == "entries" && m == http.MethodDelete:
		rep("", map[string]bool{"ok": true}, h.repo.DeleteEntry(ctx, c, seg[1]))

	case n == 1 && seg[0] == "timer" && m == http.MethodGet:
		v, err := h.repo.GetTimer(ctx, c)
		rep("", map[string]any{"timer": v}, err)
	case n == 2 && seg[0] == "timer" && seg[1] == "start" && m == http.MethodPost:
		var in struct {
			ProjectID string `json:"project_id"`
			Task      string `json:"task"`
			Billable  *bool  `json:"billable"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		b := in.Billable == nil || *in.Billable
		t, saved, err := h.repo.StartTimer(ctx, c, in.ProjectID, in.Task, b)
		rep("", map[string]any{"timer": t, "saved": saved}, err)
	case n == 2 && seg[0] == "timer" && seg[1] == "stop" && m == http.MethodPost:
		v, err := h.repo.StopTimer(ctx, c)
		rep("", v, err)
	case n == 1 && seg[0] == "timer" && m == http.MethodDelete:
		rep("", map[string]bool{"ok": true}, h.repo.DiscardTimer(ctx, c))

	case n == 1 && seg[0] == "report" && m == http.MethodGet:
		v, err := h.repo.Report(ctx, c, filter, q.Get("group"))
		rep("rows", v, err)
	case n == 1 && seg[0] == "report.csv" && m == http.MethodGet:
		v, err := h.repo.Report(ctx, c, filter, q.Get("group"))
		if err != nil {
			opshttp.Fail(w, "timetracking", err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="time-report.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"Group", "Entries", "Hours", "Billable hours", "Amount"})
		for _, x := range v {
			amt := ""
			if x.Amount != nil {
				amt = strconv.FormatFloat(*x.Amount, 'f', 2, 64)
			}
			_ = cw.Write([]string{cell(x.Label), strconv.Itoa(x.Entries), strconv.FormatFloat(Hours(x.Seconds), 'f', 2, 64), strconv.FormatFloat(Hours(x.Billed), 'f', 2, 64), amt})
		}
		cw.Flush()
	case n == 1 && seg[0] == "entries.csv" && m == http.MethodGet:
		v, err := h.repo.ListEntries(ctx, c, filter)
		if err != nil {
			opshttp.Fail(w, "timetracking", err)
			return
		}
		pl, _ := h.repo.ListProjects(ctx, c, true)
		pn := map[string]string{}
		for _, p := range pl {
			pn[p.ID] = p.Name
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="time-entries.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"Date", "Person", "Project", "Task", "Hours", "Billable", "Note"})
		for _, e := range v {
			b := "no"
			if e.Billable {
				b = "yes"
			}
			_ = cw.Write([]string{e.Date, cell(e.UserName), cell(pn[e.ProjectID]), cell(e.Task), strconv.FormatFloat(Hours(e.Seconds), 'f', 2, 64), b, cell(e.Note)})
		}
		cw.Flush()

	case n == 1 && seg[0] == "invoices" && m == http.MethodGet:
		v, err := h.repo.Invoices(ctx, c)
		rep("invoices", v, err)
	case n == 1 && seg[0] == "invoices" && m == http.MethodPost:
		var in struct {
			ClientID string   `json:"client_id"`
			From     string   `json:"from"`
			To       string   `json:"to"`
			Entries  []string `json:"entry_ids"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.CreateInvoice(ctx, c, in.ClientID, in.From, in.To, in.Entries)
		rep("", v, err)

	case n == 1 && seg[0] == "payroll-preview" && m == http.MethodGet:
		l, sk, err := h.repo.PayrollLines(ctx, c, q.Get("period"))
		rep("", map[string]any{"lines": l, "skipped": sk}, err)
	case n == 1 && seg[0] == "payroll-export" && m == http.MethodPost:
		var in struct {
			Period string `json:"period"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, sk, err := h.repo.ExportPayroll(ctx, c, in.Period)
		rep("", map[string]any{"result": v, "skipped_people": sk}, err)
	default:
		http.NotFound(w, r)
	}
}
