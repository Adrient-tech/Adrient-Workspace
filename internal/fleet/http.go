package fleet

import (
	"net/http"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

const mount = "/api/v1/fleet"

// HTTPHandler serves /api/v1/fleet/*.
type HTTPHandler struct {
	repo *Repository
	auth opshttp.AuthFuncs
}

// NewHTTPHandler builds the handler.
func NewHTTPHandler(repo *Repository, auth opshttp.AuthFuncs) *HTTPHandler {
	return &HTTPHandler{repo: repo, auth: auth}
}

// Match reports whether path belongs to Fleet.
func (h *HTTPHandler) Match(path string) bool {
	return path == mount || strings.HasPrefix(path, mount+"/")
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
	rep := func(key string, v any, err error) { opshttp.Reply(w, "fleet", key, v, err) }
	post, put, del := m == http.MethodPost, m == http.MethodPut, m == http.MethodDelete
	idOf := func() string {
		if n == 2 {
			return seg[1]
		}
		return ""
	}

	switch {
	case n == 1 && seg[0] == "me" && m == http.MethodGet:
		opshttp.JSON(w, 200, map[string]any{"user_id": user, "admin": c.Admin})
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
	case n == 2 && seg[0] == "vin" && seg[1] == "check" && m == http.MethodGet:
		vin := CleanVIN(q.Get("vin"))
		shape, check := ValidVIN(vin)
		opshttp.JSON(w, 200, map[string]any{"vin": vin, "valid": shape, "check_digit_ok": check, "year": ModelYear(vin)})
	case n == 1 && seg[0] == "team" && m == http.MethodGet:
		rows, err := h.repo.pool.Query(ctx, `SELECT id::text, display_name, email FROM adrient.users WHERE org_id=$1 ORDER BY display_name LIMIT 500`, org)
		if err != nil {
			opshttp.Fail(w, "fleet", err)
			return
		}
		defer rows.Close()
		type tm struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		out := []tm{}
		for rows.Next() {
			var t tm
			if rows.Scan(&t.ID, &t.Name, &t.Email) == nil {
				out = append(out, t)
			}
		}
		opshttp.JSON(w, 200, map[string]any{"team": out})
	case n == 1 && seg[0] == "checklist" && m == http.MethodGet:
		opshttp.JSON(w, 200, map[string]any{"items": h.repo.DefaultChecklist(ctx, org)})

	case n == 1 && seg[0] == "vehicles" && m == http.MethodGet:
		v, err := h.repo.ListVehicles(ctx, c, q.Get("status"), q.Get("q"), q.Get("archived") == "1")
		rep("vehicles", v, err)
	case (n == 1 && seg[0] == "vehicles" && post) || (n == 2 && seg[0] == "vehicles" && put):
		var in Vehicle
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.SaveVehicle(ctx, c, idOf(), in)
		rep("", v, err)
	case n == 2 && seg[0] == "vehicles" && m == http.MethodGet:
		v, err := h.repo.GetVehicle(ctx, c, seg[1])
		if err != nil {
			opshttp.Fail(w, "fleet", err)
			return
		}
		costs, _ := h.repo.Costs(ctx, c, seg[1])
		opshttp.JSON(w, 200, map[string]any{"vehicle": v, "costs": map[string]float64{"fuel": costs.Fuel, "service": costs.Service, "total": costs.Total, "distance": costs.Distance, "per_distance": costs.PerDistance}})

	case n == 1 && seg[0] == "meter" && m == http.MethodGet:
		v, err := h.repo.ListMeter(ctx, c, q.Get("vehicle"))
		rep("readings", v, err)
	case n == 1 && seg[0] == "meter" && post:
		var in struct {
			VehicleID string  `json:"vehicle_id"`
			Value     float64 `json:"value"`
			Date      string  `json:"date"`
			Note      string  `json:"note"`
			Force     bool    `json:"force"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, warn, err := h.repo.AddMeter(ctx, c, in.VehicleID, in.Value, in.Date, in.Note, in.Force)
		rep("", map[string]any{"vehicle": v, "warnings": warn}, err)

	case n == 1 && seg[0] == "fuel" && m == http.MethodGet:
		v, err := h.repo.ListFuel(ctx, c, q.Get("vehicle"))
		rep("fuel", v, err)
	case n == 1 && seg[0] == "fuel" && post:
		var in Fuel
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, warn, err := h.repo.AddFuel(ctx, c, in)
		rep("", map[string]any{"fuel": v, "warnings": warn}, err)
	case n == 1 && seg[0] == "service" && m == http.MethodGet:
		v, err := h.repo.ListService(ctx, c, q.Get("vehicle"))
		rep("service", v, err)
	case n == 1 && seg[0] == "service" && post:
		var in Service
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.AddService(ctx, c, in)
		rep("", v, err)

	case n == 1 && seg[0] == "reminders" && m == http.MethodGet:
		v, err := h.repo.ListReminders(ctx, c, q.Get("vehicle"))
		rep("reminders", v, err)
	case (n == 1 && seg[0] == "reminders" && post) || (n == 2 && seg[0] == "reminders" && put):
		var in Reminder
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.SaveReminder(ctx, c, idOf(), in)
		rep("", v, err)
	case n == 3 && seg[0] == "reminders" && seg[2] == "complete" && post:
		var in struct {
			Date  string  `json:"date"`
			Meter float64 `json:"meter"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.CompleteReminder(ctx, c, seg[1], in.Date, in.Meter)
		rep("", v, err)
	case n == 3 && seg[0] == "reminders" && seg[2] == "snooze" && post:
		var in struct {
			Until string `json:"until"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.SnoozeReminder(ctx, c, seg[1], in.Until)
		rep("", v, err)
	case n == 2 && seg[0] == "reminders" && del:
		rep("", map[string]bool{"ok": true}, h.repo.DeleteReminder(ctx, c, seg[1]))

	case n == 1 && seg[0] == "renewals" && m == http.MethodGet:
		v, err := h.repo.ListRenewals(ctx, c, q.Get("vehicle"))
		rep("renewals", v, err)
	case (n == 1 && seg[0] == "renewals" && post) || (n == 2 && seg[0] == "renewals" && put):
		var in Renewal
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.SaveRenewal(ctx, c, idOf(), in)
		rep("", v, err)
	case n == 3 && seg[0] == "renewals" && seg[2] == "complete" && post:
		var in struct {
			Date    string  `json:"date"`
			Cost    float64 `json:"cost"`
			NextDue string  `json:"next_due"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.CompleteRenewal(ctx, c, seg[1], in.Date, in.Cost, in.NextDue)
		rep("", v, err)

	case n == 1 && seg[0] == "issues" && m == http.MethodGet:
		v, err := h.repo.ListIssues(ctx, c, q.Get("vehicle"), q.Get("status"))
		rep("issues", v, err)
	case (n == 1 && seg[0] == "issues" && post) || (n == 2 && seg[0] == "issues" && put):
		var in Issue
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.SaveIssue(ctx, c, idOf(), in)
		rep("", v, err)

	case n == 1 && seg[0] == "inspections" && m == http.MethodGet:
		v, err := h.repo.ListInspections(ctx, c, q.Get("vehicle"))
		rep("inspections", v, err)
	case n == 1 && seg[0] == "inspections" && post:
		var in Inspection
		if !opshttp.Decode(w, r, &in) {
			return
		}
		v, err := h.repo.SubmitInspection(ctx, c, in)
		rep("", v, err)

	case n == 2 && seg[0] == "expenses" && seg[1] == "categories" && m == http.MethodGet:
		v, err := h.repo.ExpenseCategories(ctx, c)
		rep("categories", v, err)
	case n == 1 && seg[0] == "expenses" && post:
		var in struct {
			Kind       string `json:"kind"`
			ID         string `json:"id"`
			CategoryID string `json:"category_id"`
		}
		if !opshttp.Decode(w, r, &in) {
			return
		}
		id, err := h.repo.PostExpense(ctx, c, in.Kind, in.ID, in.CategoryID)
		rep("", map[string]string{"expense_id": id}, err)
	default:
		http.NotFound(w, r)
	}
}
