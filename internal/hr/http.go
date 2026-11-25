package hr

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// AuthFuncs supplies the caller's identity from the request context.
type AuthFuncs struct {
	UserID  func(r *http.Request) (string, bool)
	OrgID   func(r *http.Request) (string, bool)
	IsAdmin func(r *http.Request) bool // org admin / owner
}

// HTTPHandler serves /api/v1/hr/*.
type HTTPHandler struct {
	repo   *Repository
	auth   AuthFuncs
	seeded sync.Map
}

// NewHTTPHandler constructs the authenticated handler.
func NewHTTPHandler(repo *Repository, auth AuthFuncs) *HTTPHandler {
	return &HTTPHandler{repo: repo, auth: auth}
}

const mount = "/api/v1/hr"

// Match reports whether path belongs to the HR API.
func (h *HTTPHandler) Match(path string) bool {
	return path == mount || strings.HasPrefix(path, mount+"/")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrForbidden):
		http.Error(w, strings.TrimPrefix(err.Error(), "forbidden: "), http.StatusForbidden)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid: "), http.StatusBadRequest)
	case errors.Is(err, ErrConflict):
		http.Error(w, strings.TrimPrefix(err.Error(), "conflict: "), http.StatusConflict)
	default:
		slog.Error("hr: request failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func reply(w http.ResponseWriter, key string, v any, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	if key == "" {
		writeJSON(w, http.StatusOK, v)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{key: v})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func csvResponse(w http.ResponseWriter, name string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok := h.auth.OrgID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	uid, ok := h.auth.UserID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	if _, done := h.seeded.Load(org); !done {
		if err := h.repo.EnsureSeed(ctx, org); err != nil {
			fail(w, err)
			return
		}
		h.seeded.Store(org, true)
	}
	a, err := h.repo.ActorFor(ctx, org, uid, h.auth.IsAdmin != nil && h.auth.IsAdmin(r))
	if err != nil {
		fail(w, err)
		return
	}
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, mount), "/"), "/")
	m := r.Method
	n := len(seg)
	q := r.URL.Query()
	id := ""
	if n >= 2 {
		id = seg[1]
	}
	sub := ""
	if n >= 3 {
		sub = seg[2]
	}
	notFound := func() { http.NotFound(w, r) }

	switch seg[0] {
	case "me":
		if n == 1 && m == http.MethodGet {
			v, err := h.repo.MyHR(ctx, a)
			reply(w, "", v, err)
			return
		}
	case "dashboard":
		if n == 1 && m == http.MethodGet {
			v, err := h.repo.HRDashboard(ctx, a)
			reply(w, "", v, err)
			return
		}
		if n == 2 && id == "manager" && m == http.MethodGet {
			v, err := h.repo.ManagerDashboard(ctx, a)
			reply(w, "", v, err)
			return
		}
	case "celebrations":
		if m == http.MethodGet {
			v, err := h.repo.Celebrations(ctx, org, 45)
			reply(w, "celebrations", v, err)
			return
		}
	case "orgchart":
		if m == http.MethodGet {
			v, err := h.repo.OrgChart(ctx, org)
			reply(w, "people", v, err)
			return
		}
	case "settings":
		switch m {
		case http.MethodGet:
			v, err := h.repo.GetSettings(ctx, org)
			reply(w, "", v, err)
			return
		case http.MethodPut:
			var s Settings
			if decode(w, r, &s) {
				v, err := h.repo.SaveSettings(ctx, a, s)
				reply(w, "", v, err)
			}
			return
		}
	case "hr-admins":
		switch {
		case n == 1 && m == http.MethodGet:
			v, err := h.repo.ListHRAdmins(ctx, a)
			reply(w, "admins", v, err)
			return
		case n == 2 && m == http.MethodPut:
			reply(w, "", map[string]bool{"ok": true}, h.repo.SetHRAdmin(ctx, a, id, true))
			return
		case n == 2 && m == http.MethodDelete:
			reply(w, "", map[string]bool{"ok": true}, h.repo.SetHRAdmin(ctx, a, id, false))
			return
		}
	case "available-users":
		if m == http.MethodGet {
			v, err := h.repo.AvailableUsers(ctx, a)
			reply(w, "users", v, err)
			return
		}
	case "departments", "designations", "locations":
		kind := seg[0]
		switch {
		case n == 1 && m == http.MethodGet:
			v, err := h.repo.ListLookups(ctx, org, kind)
			reply(w, kind, v, err)
			return
		case n == 1 && m == http.MethodPost, n == 2 && m == http.MethodPut:
			var l Lookup
			if decode(w, r, &l) {
				v, err := h.repo.SaveLookup(ctx, a, kind, id, l)
				reply(w, "", v, err)
			}
			return
		case n == 2 && m == http.MethodDelete:
			reply(w, "", map[string]bool{"ok": true}, h.repo.DeleteLookup(ctx, a, kind, id))
			return
		}
	case "employees":
		h.employees(w, r, a, seg)
		return
	case "documents":
		switch {
		case n == 2 && m == http.MethodGet:
			d, rc, err := h.repo.OpenDocument(ctx, a, id)
			if err != nil {
				fail(w, err)
				return
			}
			defer rc.Close()
			w.Header().Set("Content-Type", d.MimeType)
			w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeFilename(d.Name)+`"`)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "private, no-store")
			_, _ = io.Copy(w, rc)
			return
		case n == 2 && m == http.MethodDelete:
			reply(w, "", map[string]bool{"ok": true}, h.repo.DeleteDocument(ctx, a, id))
			return
		}
	case "leave-types":
		switch {
		case n == 1 && m == http.MethodGet:
			v, err := h.repo.ListLeaveTypes(ctx, org, a.IsHR && q.Get("all") == "1")
			reply(w, "leave_types", v, err)
			return
		case n == 1 && m == http.MethodPost, n == 2 && m == http.MethodPut:
			var t LeaveType
			t.Active = true
			if decode(w, r, &t) {
				v, err := h.repo.SaveLeaveType(ctx, a, id, t)
				reply(w, "", v, err)
			}
			return
		}
	case "holiday-calendars":
		switch {
		case n == 1 && m == http.MethodGet:
			v, err := h.repo.ListHolidayCalendars(ctx, org)
			reply(w, "calendars", v, err)
			return
		case n == 1 && m == http.MethodPost, n == 2 && m == http.MethodPut:
			var c HolidayCalendar
			if decode(w, r, &c) {
				v, err := h.repo.SaveHolidayCalendar(ctx, a, id, c)
				reply(w, "", v, err)
			}
			return
		case n == 2 && m == http.MethodDelete:
			reply(w, "", map[string]bool{"ok": true}, h.repo.DeleteHolidayCalendar(ctx, a, id))
			return
		case n == 3 && sub == "holidays" && m == http.MethodPost:
			var hd Holiday
			if decode(w, r, &hd) {
				v, err := h.repo.AddHoliday(ctx, a, id, hd)
				reply(w, "", v, err)
			}
			return
		}
	case "holidays":
		if n == 2 && m == http.MethodDelete {
			reply(w, "", map[string]bool{"ok": true}, h.repo.DeleteHoliday(ctx, a, id))
			return
		}
	case "leave":
		h.leave(w, r, a, seg)
		return
	case "attendance", "shifts", "roster", "regularisations":
		h.attendance(w, r, a, seg)
		return
	case "payroll":
		h.payroll(w, r, a, seg)
		return
	}
	notFound()
}

func sanitizeFilename(s string) string {
	return strings.NewReplacer(`"`, "", "\r", "", "\n", "", `\`, "").Replace(s)
}

func (h *HTTPHandler) employees(w http.ResponseWriter, r *http.Request, a Actor, seg []string) {
	ctx := r.Context()
	m, n := r.Method, len(seg)
	q := r.URL.Query()
	id, sub := "", ""
	if n >= 2 {
		id = seg[1]
	}
	if n >= 3 {
		sub = seg[2]
	}
	switch {
	case n == 1 && m == http.MethodGet:
		f := EmployeeFilter{Q: q.Get("q"), Status: q.Get("status"), DepartmentID: q.Get("department_id"), LocationID: q.Get("location_id"), ManagerID: q.Get("manager_id"),
			Sort: q.Get("sort"), Desc: q.Get("order") == "desc"}
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 || limit > MaxEmployeePage {
			limit = 100
		}
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		f.Limit, f.Offset = limit+1, (page-1)*limit
		v, err := h.repo.ListEmployees(ctx, a, f)
		if err != nil {
			reply(w, "employees", v, err)
			return
		}
		next := ""
		if len(v) > limit {
			v, next = v[:limit], strconv.Itoa(page+1)
		}
		writeJSON(w, http.StatusOK, map[string]any{"employees": v, "items": v, "next_cursor": next})
	case n == 1 && m == http.MethodPost:
		var e Employee
		if decode(w, r, &e) {
			v, err := h.repo.SaveEmployee(ctx, a, "", e)
			reply(w, "", v, err)
		}
	case n == 2 && id == "me" && m == http.MethodPut:
		var e Employee
		if decode(w, r, &e) {
			v, err := h.repo.SaveOwnProfile(ctx, a, e)
			reply(w, "", v, err)
		}
	case n == 2 && id == "export.csv" && m == http.MethodGet:
		if err := a.requireHR(); err != nil {
			fail(w, err)
			return
		}
		csvResponse(w, "employees.csv")
		if err := h.repo.ExportEmployeesCSV(ctx, a, w); err != nil {
			slog.Error("hr: export", "err", err)
		}
	case n == 2 && id == "import" && m == http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
		var src io.Reader = r.Body
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			f, _, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "attach the CSV as the file field", http.StatusBadRequest)
				return
			}
			defer f.Close()
			src = f
		}
		v, err := h.repo.ImportEmployeesCSV(ctx, a, src)
		reply(w, "", v, err)
	case n == 2 && m == http.MethodGet:
		v, err := h.repo.GetEmployee(ctx, a, id)
		reply(w, "", v, err)
	case n == 2 && m == http.MethodPut:
		var e Employee
		if decode(w, r, &e) {
			v, err := h.repo.SaveEmployee(ctx, a, id, e)
			reply(w, "", v, err)
		}
	case n == 2 && m == http.MethodDelete:
		reply(w, "", map[string]bool{"ok": true}, h.repo.DeleteEmployee(ctx, a, id))
	case n == 3 && sub == "emergency" && m == http.MethodGet:
		v, err := h.repo.ListEmergencyContacts(ctx, a, id)
		reply(w, "contacts", v, err)
	case n == 3 && sub == "emergency" && m == http.MethodPost:
		var c EmergencyContact
		if decode(w, r, &c) {
			v, err := h.repo.AddEmergencyContact(ctx, a, id, c)
			reply(w, "", v, err)
		}
	case n == 4 && sub == "emergency" && m == http.MethodDelete:
		reply(w, "", map[string]bool{"ok": true}, h.repo.DeleteEmergencyContact(ctx, a, id, seg[3]))
	case n == 3 && sub == "documents" && m == http.MethodGet:
		v, err := h.repo.ListDocuments(ctx, a, id)
		reply(w, "documents", v, err)
	case n == 3 && sub == "documents" && m == http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, MaxDocumentBytes+(1<<20))
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			http.Error(w, "upload too large or malformed", http.StatusBadRequest)
			return
		}
		f, hd, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "attach a file", http.StatusBadRequest)
			return
		}
		defer f.Close()
		v, err := h.repo.AddDocument(ctx, a, id, r.FormValue("category"), hd.Filename, hd.Header.Get("Content-Type"), hd.Size, f)
		reply(w, "", v, err)
	case n == 3 && sub == "salary" && m == http.MethodGet:
		v, err := h.repo.ListSalaries(ctx, a, id)
		reply(w, "salaries", v, err)
	case n == 3 && sub == "salary" && m == http.MethodPost:
		var s EmployeeSalary
		s.PFEnabled, s.ESIEnabled, s.PTEnabled = true, true, true
		if decode(w, r, &s) {
			v, err := h.repo.SaveSalary(ctx, a, id, s)
			reply(w, "", v, err)
		}
	case n == 3 && sub == "balances" && m == http.MethodGet:
		v, err := h.repo.Balances(ctx, a, id)
		reply(w, "balances", v, err)
	case n == 4 && sub == "balances" && seg[3] == "adjust" && m == http.MethodPost:
		var b struct {
			TypeID string  `json:"type_id"`
			Days   float64 `json:"days"`
			Note   string  `json:"note"`
		}
		if decode(w, r, &b) {
			reply(w, "", map[string]bool{"ok": true}, h.repo.AdjustBalance(ctx, a, id, b.TypeID, b.Days, b.Note))
		}
	case n == 3 && sub == "timesheet" && m == http.MethodGet:
		v, err := h.repo.GetTimesheet(ctx, a, id, q.Get("month"))
		reply(w, "", v, err)
	default:
		http.NotFound(w, r)
	}
}

func (h *HTTPHandler) leave(w http.ResponseWriter, r *http.Request, a Actor, seg []string) {
	ctx := r.Context()
	m, n := r.Method, len(seg)
	q := r.URL.Query()
	switch {
	case n == 2 && seg[1] == "requests" && m == http.MethodGet:
		limit, page := pageArgs(r, 100, 500)
		v, err := h.repo.ListLeaveRequests(ctx, a, LeaveFilter{Scope: q.Get("scope"), Status: q.Get("status"), EmployeeID: q.Get("employee_id"), From: q.Get("from"), To: q.Get("to"),
			Limit: limit + 1, Offset: (page - 1) * limit})
		if err != nil {
			reply(w, "requests", v, err)
			return
		}
		next := ""
		if len(v) > limit {
			v, next = v[:limit], strconv.Itoa(page+1)
		}
		writeJSON(w, http.StatusOK, map[string]any{"requests": v, "items": v, "next_cursor": next})
	case n == 2 && seg[1] == "requests" && m == http.MethodPost:
		var in LeaveInput
		if decode(w, r, &in) {
			v, err := h.repo.ApplyLeave(ctx, a, in)
			reply(w, "", v, err)
		}
	case n == 4 && seg[1] == "requests" && m == http.MethodPost:
		switch seg[3] {
		case "approve", "reject":
			var b struct {
				Note string `json:"note"`
			}
			if r.ContentLength != 0 && !decode(w, r, &b) {
				return
			}
			v, err := h.repo.DecideLeave(ctx, a, seg[2], seg[3] == "approve", b.Note)
			reply(w, "", v, err)
		case "cancel":
			v, err := h.repo.CancelLeave(ctx, a, seg[2])
			reply(w, "", v, err)
		default:
			http.NotFound(w, r)
		}
	case n == 2 && seg[1] == "team-calendar" && m == http.MethodGet:
		v, err := h.repo.TeamCalendar(ctx, a, q.Get("from"), q.Get("to"))
		reply(w, "", v, err)
	case n == 3 && seg[1] == "accrual" && seg[2] == "run" && m == http.MethodPost:
		c, err := h.repo.RunAccrual(ctx, a)
		reply(w, "", map[string]int{"employees": c}, err)
	default:
		http.NotFound(w, r)
	}
}

func (h *HTTPHandler) attendance(w http.ResponseWriter, r *http.Request, a Actor, seg []string) {
	ctx := r.Context()
	m, n := r.Method, len(seg)
	q := r.URL.Query()
	switch seg[0] {
	case "attendance":
		switch {
		case n == 2 && seg[1] == "today" && m == http.MethodGet:
			v, err := h.repo.Today(ctx, a)
			reply(w, "", v, err)
		case n == 2 && seg[1] == "punch" && m == http.MethodPost:
			var b struct {
				Lat *float64 `json:"lat"`
				Lng *float64 `json:"lng"`
			}
			if r.ContentLength != 0 && !decode(w, r, &b) {
				return
			}
			v, err := h.repo.CheckInOut(ctx, a, b.Lat, b.Lng)
			reply(w, "", v, err)
		case n == 2 && seg[1] == "timesheet" && m == http.MethodGet:
			emp := q.Get("employee_id")
			if emp == "" {
				emp = a.EmployeeID
			}
			v, err := h.repo.GetTimesheet(ctx, a, emp, q.Get("month"))
			reply(w, "", v, err)
		case n == 2 && seg[1] == "report" && m == http.MethodGet:
			if q.Get("format") == "csv" {
				if _, err := h.repo.MonthlyReport(ctx, a, q.Get("month")); err != nil {
					fail(w, err)
					return
				}
				csvResponse(w, "attendance_"+q.Get("month")+".csv")
				_ = h.repo.MonthlyReportCSV(ctx, a, q.Get("month"), w)
				return
			}
			limit, page := pageArgs(r, 100, 200)
			v, err := h.repo.MonthlyReportPage(ctx, a, q.Get("month"), q.Get("q"), limit+1, (page-1)*limit)
			if err != nil {
				reply(w, "rows", v, err)
				return
			}
			next := ""
			if len(v) > limit {
				v, next = v[:limit], strconv.Itoa(page+1)
			}
			writeJSON(w, http.StatusOK, map[string]any{"rows": v, "items": v, "next_cursor": next})
		case n == 2 && seg[1] == "import" && m == http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
			var src io.Reader = r.Body
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
				f, _, err := r.FormFile("file")
				if err != nil {
					http.Error(w, "attach the CSV as the file field", http.StatusBadRequest)
					return
				}
				defer f.Close()
				src = f
			}
			v, err := h.repo.ImportPunchesCSV(ctx, a, src)
			reply(w, "", v, err)
		default:
			http.NotFound(w, r)
		}
	case "shifts":
		switch {
		case n == 1 && m == http.MethodGet:
			v, err := h.repo.ListShifts(ctx, a.OrgID)
			reply(w, "shifts", v, err)
		case n == 1 && m == http.MethodPost, n == 2 && m == http.MethodPut:
			s := Shift{ShiftDef: DefaultShift}
			if decode(w, r, &s) {
				id := ""
				if n == 2 {
					id = seg[1]
				}
				v, err := h.repo.SaveShift(ctx, a, id, s)
				reply(w, "", v, err)
			}
		case n == 2 && m == http.MethodDelete:
			reply(w, "", map[string]bool{"ok": true}, h.repo.DeleteShift(ctx, a, seg[1]))
		default:
			http.NotFound(w, r)
		}
	case "roster":
		switch {
		case n == 1 && m == http.MethodGet:
			v, err := h.repo.ListRoster(ctx, a, q.Get("from"), q.Get("to"))
			reply(w, "roster", v, err)
		case n == 1 && m == http.MethodPost:
			var b struct {
				EmployeeIDs []string `json:"employee_ids"`
				From        string   `json:"from"`
				To          string   `json:"to"`
				ShiftID     string   `json:"shift_id"`
			}
			if decode(w, r, &b) {
				c, err := h.repo.SetRoster(ctx, a, b.EmployeeIDs, b.From, b.To, b.ShiftID)
				reply(w, "", map[string]int{"days": c}, err)
			}
		default:
			http.NotFound(w, r)
		}
	case "regularisations":
		switch {
		case n == 1 && m == http.MethodGet:
			v, err := h.repo.ListRegularisations(ctx, a, q.Get("scope"), q.Get("status"))
			reply(w, "regularisations", v, err)
		case n == 1 && m == http.MethodPost:
			var b struct {
				Day    string `json:"day"`
				In     string `json:"in_time"`
				Out    string `json:"out_time"`
				Reason string `json:"reason"`
			}
			if decode(w, r, &b) {
				v, err := h.repo.RequestRegularisation(ctx, a, b.Day, b.In, b.Out, b.Reason)
				reply(w, "", v, err)
			}
		case n == 3 && m == http.MethodPost && (seg[2] == "approve" || seg[2] == "reject"):
			var b struct {
				Note string `json:"note"`
			}
			if r.ContentLength != 0 && !decode(w, r, &b) {
				return
			}
			v, err := h.repo.DecideRegularisation(ctx, a, seg[1], seg[2] == "approve", b.Note)
			reply(w, "", v, err)
		case n == 3 && m == http.MethodPost && seg[2] == "cancel":
			reply(w, "", map[string]bool{"ok": true}, h.repo.CancelRegularisation(ctx, a, seg[1]))
		default:
			http.NotFound(w, r)
		}
	}
}

func (h *HTTPHandler) payroll(w http.ResponseWriter, r *http.Request, a Actor, seg []string) {
	ctx := r.Context()
	m, n := r.Method, len(seg)
	id, sub := "", ""
	if n >= 3 {
		id = seg[2]
	}
	if n >= 4 {
		sub = seg[3]
	}
	if n < 2 {
		http.NotFound(w, r)
		return
	}
	switch seg[1] {
	case "structures":
		switch {
		case n == 2 && m == http.MethodGet:
			v, err := h.repo.ListStructures(ctx, a)
			reply(w, "structures", v, err)
		case n == 2 && m == http.MethodPost, n == 3 && m == http.MethodPut && id != "preview":
			var s Structure
			if decode(w, r, &s) {
				v, err := h.repo.SaveStructure(ctx, a, id, s)
				reply(w, "", v, err)
			}
		case n == 3 && id == "preview" && m == http.MethodPost:
			var b struct {
				AnnualGross float64      `json:"annual_gross"`
				Lines       []SalaryLine `json:"lines"`
			}
			if decode(w, r, &b) {
				v, err := h.repo.PreviewStructure(ctx, a, b.Lines, b.AnnualGross)
				reply(w, "lines", v, err)
			}
		case n == 3 && m == http.MethodDelete:
			reply(w, "", map[string]bool{"ok": true}, h.repo.DeleteStructure(ctx, a, id))
		default:
			http.NotFound(w, r)
		}
	case "pt-slabs":
		switch {
		case n == 2 && m == http.MethodGet:
			v, err := h.repo.ListPTSlabs(ctx, a)
			reply(w, "slabs", v, err)
		case n == 2 && m == http.MethodPut:
			var b struct {
				Slabs []PTSlab `json:"slabs"`
			}
			if decode(w, r, &b) {
				reply(w, "", map[string]bool{"ok": true}, h.repo.ReplacePTSlabs(ctx, a, b.Slabs))
			}
		default:
			http.NotFound(w, r)
		}
	case "runs":
		switch {
		case n == 2 && m == http.MethodGet:
			v, err := h.repo.ListRuns(ctx, a)
			reply(w, "runs", v, err)
		case n == 2 && m == http.MethodPost:
			var b struct {
				Period string `json:"period"`
			}
			if decode(w, r, &b) {
				v, err := h.repo.CreateRun(ctx, a, b.Period)
				reply(w, "", v, err)
			}
		case n == 3 && m == http.MethodGet:
			v, err := h.repo.GetRun(ctx, a, id)
			reply(w, "", v, err)
		case n == 3 && m == http.MethodDelete:
			reply(w, "", map[string]bool{"ok": true}, h.repo.DeleteRun(ctx, a, id))
		case n == 4 && m == http.MethodPost && sub == "recompute":
			v, err := h.repo.Recompute(ctx, a, id)
			reply(w, "", v, err)
		case n == 4 && m == http.MethodPost && sub == "review":
			v, err := h.repo.MarkReviewed(ctx, a, id)
			reply(w, "", v, err)
		case n == 4 && m == http.MethodPost && sub == "reopen":
			v, err := h.repo.ReopenRun(ctx, a, id)
			reply(w, "", v, err)
		case n == 4 && m == http.MethodPost && sub == "lock":
			v, err := h.repo.LockRun(ctx, a, id)
			reply(w, "", v, err)
		case n == 4 && m == http.MethodGet && sub == "payslips":
			limit, page := pageArgs(r, 100, 500)
			v, err := h.repo.RunPayslipsPage(ctx, a, id, r.URL.Query().Get("q"), limit+1, (page-1)*limit)
			if err != nil {
				reply(w, "payslips", v, err)
				return
			}
			next := ""
			if len(v) > limit {
				v, next = v[:limit], strconv.Itoa(page+1)
			}
			writeJSON(w, http.StatusOK, map[string]any{"payslips": v, "items": v, "next_cursor": next})
		case n == 4 && m == http.MethodGet && (sub == "bank-advice.csv" || sub == "statutory.csv"):
			if err := a.requireHR(); err != nil {
				fail(w, err)
				return
			}
			var buf strings.Builder
			var err error
			if sub == "bank-advice.csv" {
				err = h.repo.BankAdviceCSV(ctx, a, id, &buf)
			} else {
				err = h.repo.StatutoryCSV(ctx, a, id, &buf)
			}
			if err != nil {
				fail(w, err)
				return
			}
			csvResponse(w, sub)
			_, _ = io.WriteString(w, buf.String())
		default:
			http.NotFound(w, r)
		}
	case "payslips":
		switch {
		case n == 3 && id == "mine" && m == http.MethodGet:
			v, err := h.repo.MyPayslips(ctx, a)
			reply(w, "payslips", v, err)
		case n == 3 && m == http.MethodGet:
			v, err := h.repo.GetPayslip(ctx, a, id)
			reply(w, "", v, err)
		case n == 4 && m == http.MethodGet && sub == "pdf":
			b, name, err := h.repo.PayslipPDF(ctx, a, id)
			if err != nil {
				fail(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeFilename(name)+`"`)
			w.Header().Set("Cache-Control", "private, no-store")
			_, _ = w.Write(b)
		case n == 4 && m == http.MethodPut && sub == "adjustments":
			var b struct {
				Adjustments []Adjustment `json:"adjustments"`
			}
			if decode(w, r, &b) {
				v, err := h.repo.SetAdjustments(ctx, a, id, b.Adjustments)
				reply(w, "", v, err)
			}
		case n == 4 && m == http.MethodGet && sub == "reimbursements":
			v, err := h.repo.ReimbursementCandidates(ctx, a, id)
			reply(w, "reports", v, err)
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}
