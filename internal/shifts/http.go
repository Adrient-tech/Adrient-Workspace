package shifts

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

const mount = "/api/v1/shifts"

// HTTPHandler serves /api/v1/shifts/*.
type HTTPHandler struct {
	repo *Repository
	auth opshttp.AuthFuncs
}

// NewHTTPHandler builds the handler.
func NewHTTPHandler(repo *Repository, auth opshttp.AuthFuncs) *HTTPHandler {
	return &HTTPHandler{repo: repo, auth: auth}
}

// Match reports whether path belongs to Shifts.
func (h *HTTPHandler) Match(path string) bool {
	return path == mount || strings.HasPrefix(path, mount+"/")
}

// CSVCell neutralises spreadsheet formulas in exported text.
func CSVCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.auth.OrgID(r)
	userID, ok2 := h.auth.UserID(r)
	if !ok || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	admin := h.auth.IsAdmin != nil && h.auth.IsAdmin(r)
	c, err := h.repo.CallerFor(r.Context(), orgID, userID, admin)
	if err != nil {
		opshttp.Fail(w, "shifts", err)
		return
	}
	ctx, m, q := r.Context(), r.Method, r.URL.Query()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, mount), "/"), "/")
	n := len(seg)
	rep := func(key string, v any, err error) { opshttp.Reply(w, "shifts", key, v, err) }
	from, to := q.Get("from"), q.Get("to")

	switch {
	case n == 1 && seg[0] == "me" && m == http.MethodGet:
		opshttp.JSON(w, 200, map[string]any{"employee_id": c.EmployeeID, "scheduler": c.Scheduler, "hr_admin": c.Actor.IsHR, "manager": c.Actor.IsManager})
	case n == 1 && seg[0] == "settings" && m == http.MethodGet:
		s, err := h.repo.GetSettings(ctx, orgID)
		rep("", s, err)
	case n == 1 && seg[0] == "settings" && m == http.MethodPut:
		var s Settings
		if !opshttp.Decode(w, r, &s) {
			return
		}
		v, err := h.repo.SaveSettings(ctx, c, s)
		rep("", v, err)

	case n == 1 && seg[0] == "staff" && m == http.MethodGet:
		v, err := h.repo.Staff(ctx, c, from, to)
		rep("staff", v, err)
	case n == 2 && seg[0] == "staff" && m == http.MethodPut:
		var in StaffInput
		if !opshttp.Decode(w, r, &in) {
			return
		}
		rep("", map[string]bool{"ok": true}, h.repo.SaveStaff(ctx, c, seg[1], in))

	case n == 1 && seg[0] == "shifts" && m == http.MethodGet:
		v, err := h.repo.ListShifts(ctx, c, from, to)
		rep("shifts", v, err)
	case n == 2 && seg[0] == "shifts" && m == http.MethodGet:
		v, err := h.repo.GetShift(ctx, c, seg[1])
		rep("", v, err)
	case n == 1 && seg[0] == "shifts" && m == http.MethodPost:
		var in Shift
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.SaveShift(ctx, c, "", in)
		rep("", v, err)
	case n == 2 && seg[0] == "shifts" && m == http.MethodPut:
		var in Shift
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.SaveShift(ctx, c, seg[1], in)
		rep("", v, err)
	case n == 2 && seg[0] == "shifts" && m == http.MethodDelete:
		rep("", map[string]bool{"ok": true}, h.repo.DeleteShift(ctx, c, seg[1]))
	case n == 3 && seg[0] == "shifts" && seg[2] == "suggest" && m == http.MethodGet:
		v, err := h.repo.Suggest(ctx, c, seg[1])
		rep("candidates", v, err)
	case n == 1 && seg[0] == "copy-week" && m == http.MethodPost:
		var in struct{ From, To string }
		if !opshttp.Decode(w, r, &in) {
			return
		}
		k, err := h.repo.CopyWeek(ctx, c, in.From, in.To)
		rep("", map[string]int{"copied": k}, err)
	case n == 1 && seg[0] == "publish" && m == http.MethodPost:
		var in struct {
			From      string `json:"from"`
			To        string `json:"to"`
			Published bool   `json:"published"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		k, err := h.repo.Publish(ctx, c, in.From, in.To, in.Published)
		rep("", map[string]int{"changed": k}, err)
	case n == 1 && seg[0] == "fill" && m == http.MethodGet:
		v, err := h.repo.FillPlan(ctx, c, from, to)
		rep("plan", v, err)
	case n == 1 && seg[0] == "fill" && m == http.MethodPost:
		var in struct {
			Assignments []Assignment `json:"assignments"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		k, problems, err := h.repo.ApplyFill(ctx, c, in.Assignments)
		rep("", map[string]any{"assigned": k, "problems": problems}, err)

	case n == 1 && seg[0] == "coverage" && m == http.MethodGet:
		v, err := h.repo.ListCoverage(ctx, orgID)
		rep("coverage", v, err)
	case n == 1 && seg[0] == "coverage" && m == http.MethodPost, n == 2 && seg[0] == "coverage" && m == http.MethodPut:
		var in Coverage
		if !opshttp.Decode(w, r, &in) {
			return
		}
		id := ""
		if n == 2 {
			id = seg[1]
		}
		v, err := h.repo.SaveCoverage(ctx, c, id, in)
		rep("", v, err)
	case n == 2 && seg[0] == "coverage" && m == http.MethodDelete:
		rep("", map[string]bool{"ok": true}, h.repo.DeleteCoverage(ctx, c, seg[1]))
	case n == 1 && seg[0] == "gaps" && m == http.MethodGet:
		v, err := h.repo.Gaps(ctx, c, from, to)
		rep("gaps", v, err)

	case n == 1 && seg[0] == "swaps" && m == http.MethodGet:
		v, err := h.repo.ListSwaps(ctx, c, q.Get("status"))
		rep("swaps", v, err)
	case n == 1 && seg[0] == "swaps" && m == http.MethodPost:
		var in struct {
			ShiftID string `json:"shift_id"`
			To      string `json:"to_employee_id"`
			Reason  string `json:"reason"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.RequestSwap(ctx, c, in.ShiftID, in.To, in.Reason)
		rep("", v, err)
	case n == 3 && seg[0] == "swaps" && seg[2] == "warnings" && m == http.MethodGet:
		v, err := h.repo.SwapWarnings(ctx, c, seg[1])
		rep("warnings", v, err)
	case n == 3 && seg[0] == "swaps" && m == http.MethodPost:
		v, err := h.repo.ResolveSwap(ctx, c, seg[1], seg[2])
		rep("", v, err)

	case n == 1 && seg[0] == "attendance" && m == http.MethodGet:
		v, err := h.repo.Attendance(ctx, c, from, to)
		rep("attendance", v, err)
	case n == 2 && seg[0] == "attendance" && m == http.MethodPut:
		var in Attendance
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.SetAttendance(ctx, c, seg[1], in)
		rep("", v, err)
	case n == 3 && seg[0] == "clock" && m == http.MethodPost:
		v, err := h.repo.Clock(ctx, c, seg[1], seg[2])
		rep("", v, err)

	case n == 1 && seg[0] == "timesheet" && m == http.MethodGet:
		rows, sum, err := h.repo.Timesheet(ctx, c, from, to)
		if err != nil {
			opshttp.Fail(w, "shifts", err)
			return
		}
		opshttp.JSON(w, 200, map[string]any{"rows": rows, "summary": sum})
	case n == 1 && seg[0] == "timesheet.csv" && m == http.MethodGet:
		rows, _, err := h.repo.Timesheet(ctx, c, from, to)
		if err != nil {
			opshttp.Fail(w, "shifts", err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="timesheet-`+from+`-`+to+`.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"Date", "Employee", "Role", "Start", "End", "Break minutes", "Scheduled hours", "Status", "Clock in", "Clock out", "Worked hours", "Minutes late", "Paid hours", "Hourly rate", "Pay"})
		f := func(p *float64) string {
			if p == nil {
				return ""
			}
			return strconv.FormatFloat(*p, 'f', 2, 64)
		}
		for _, x := range rows {
			_ = cw.Write([]string{x.Date, CSVCell(x.Employee), CSVCell(x.Role), x.Start, x.End, strconv.Itoa(x.Break), strconv.FormatFloat(x.Scheduled, 'f', 2, 64),
				x.Status, x.ClockIn, x.ClockOut, f(x.Worked), strconv.Itoa(x.Late), strconv.FormatFloat(x.Paid, 'f', 2, 64), f(x.Rate), f(x.Pay)})
		}
		cw.Flush()
	case n == 1 && seg[0] == "cost" && m == http.MethodGet:
		v, set, err := h.repo.Cost(ctx, c, from, to)
		if err != nil {
			opshttp.Fail(w, "shifts", err)
			return
		}
		opshttp.JSON(w, 200, map[string]any{"cost": v, "weekly_budget": set.WeeklyBudget, "currency": set.Currency})
	case n == 1 && seg[0] == "payroll-preview" && m == http.MethodGet:
		v, err := h.repo.PayrollLines(ctx, c, q.Get("period"))
		rep("lines", v, err)
	case n == 1 && seg[0] == "payroll-export" && m == http.MethodPost:
		var in struct {
			Period string `json:"period"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.ExportPayroll(ctx, c, in.Period)
		rep("", v, err)
	default:
		http.NotFound(w, r)
	}
}
