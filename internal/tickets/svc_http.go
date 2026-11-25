package tickets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// serveSvc handles the service-desk routes under /api/v1/ts/. It reports false for paths it
// does not own.
//
//	GET  projects/{id}/list?view&q&group&assignee&limit&cursor   paginated tickets + view counts
//	GET/PUT projects/{id}/statuses                             workflow (admin to write)
//	GET/POST projects/{id}/groups, DELETE groups/{id}            teams and routing (admin to write)
//	PUT  me/ooo                                                  out of office
//	GET/POST calendars, DELETE calendars/{id}                    business hours and holidays
//	GET/POST projects/{id}/sla-rules, DELETE sla-rules/{id}
//	GET/POST projects/{id}/automations, DELETE automations/{id}, POST projects/{id}/automations/test
//	GET/POST projects/{id}/forms, DELETE forms/{id}
//	GET  projects/{id}/report2, report.csv; GET/POST projects/{id}/saved-reports, DELETE saved-reports/{id}
//	GET  projects/{id}/kb/suggest?q, kb/stats
//	GET  major, approvals/mine
//	tickets/{id}/: sla, presence, draft, time, snooze, parent, major, fields, approvals, approvals/decide, detail2
//	POST sweep                                                   run scheduled work now (admin)
func (h *ExtrasHandler) serveSvc(w http.ResponseWriter, r *http.Request, org, user string, seg []string) bool {
	m := r.Method
	get, post, put, del := m == http.MethodGet, m == http.MethodPost, m == http.MethodPut, m == http.MethodDelete
	name := ""
	if h.auth.UserName != nil {
		name = h.auth.UserName(r)
	}
	ctx := WithActor(r.Context(), Actor{ID: user, Name: name})
	admin := func() bool {
		if h.auth.IsAdmin != nil && h.auth.IsAdmin(r) {
			return true
		}
		http.Error(w, "only admins can change the service desk configuration", http.StatusForbidden)
		return false
	}
	fail := func(err error) bool { failHTTP(w, err); return true }
	q := r.URL.Query()
	n := len(seg)
	switch {
	case n == 3 && seg[0] == "projects" && seg[2] == "list" && get:
		limit, _ := strconv.Atoi(q.Get("limit"))
		items, next, err := h.repo.ListPage(ctx, org, ListOpts{ProjectID: seg[1], View: q.Get("view"), Q: strings.TrimSpace(q.Get("q")), Group: q.Get("group"),
			Assignee: q.Get("assignee"), Limit: limit, Cursor: q.Get("cursor"), Me: user})
		if err != nil {
			return fail(err)
		}
		resp := map[string]any{"tickets": items, "next": next, "types": TicketTypes}
		if q.Get("cursor") == "" {
			views, err := h.repo.ViewCountsSQL(ctx, org, seg[1], user)
			if err != nil {
				return fail(err)
			}
			resp["views"] = views
		}
		outJSON(w, resp)
	case n == 3 && seg[0] == "projects" && seg[2] == "statuses" && get:
		v, err := h.repo.StatusDefs(ctx, org, seg[1])
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"statuses": v, "categories": StatusCategories})
	case n == 3 && seg[0] == "projects" && seg[2] == "statuses" && put:
		if !admin() {
			return true
		}
		var in struct {
			Statuses []StatusDef `json:"statuses"`
		}
		if !decode(w, r, &in) {
			return true
		}
		v, err := h.repo.SaveStatusDefs(ctx, org, seg[1], in.Statuses)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"statuses": v})
	case n == 3 && seg[0] == "projects" && seg[2] == "groups" && get:
		v, err := h.repo.ListGroups(ctx, org, seg[1])
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"groups": v, "strategies": []string{StrategyManual, StrategyRoundRobin, StrategyLoad}})
	case n == 3 && seg[0] == "projects" && seg[2] == "groups" && post:
		if !admin() {
			return true
		}
		var g Group
		if !decode(w, r, &g) {
			return true
		}
		v, err := h.repo.SaveGroup(ctx, org, seg[1], g)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 2 && seg[0] == "groups" && del:
		if !admin() {
			return true
		}
		if err := h.repo.DeleteGroup(ctx, org, seg[1]); err != nil {
			return fail(err)
		}
		w.WriteHeader(http.StatusNoContent)
	case n == 2 && seg[0] == "me" && seg[1] == "ooo" && put:
		var in struct {
			Until *time.Time `json:"until"`
		}
		if !decode(w, r, &in) {
			return true
		}
		if err := h.repo.SetOOO(ctx, org, user, in.Until); err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"until": in.Until})
	case n == 1 && seg[0] == "calendars" && get:
		v, err := h.repo.ListCalendars(ctx, org)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"calendars": v})
	case n == 1 && seg[0] == "calendars" && post:
		if !admin() {
			return true
		}
		var c CalendarRow
		if !decode(w, r, &c) {
			return true
		}
		v, err := h.repo.SaveCalendar(ctx, org, c)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 2 && seg[0] == "calendars" && del:
		if !admin() {
			return true
		}
		if err := h.repo.DeleteCalendar(ctx, org, seg[1]); err != nil {
			return fail(err)
		}
		w.WriteHeader(http.StatusNoContent)
	case n == 3 && seg[0] == "projects" && seg[2] == "sla-rules" && get:
		v, err := h.repo.ListSLARules(ctx, org, seg[1])
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"rules": v})
	case n == 3 && seg[0] == "projects" && seg[2] == "sla-rules" && post:
		if !admin() {
			return true
		}
		var s SLARule
		if !decode(w, r, &s) {
			return true
		}
		v, err := h.repo.SaveSLARule(ctx, org, seg[1], s)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 2 && seg[0] == "sla-rules" && del:
		if !admin() {
			return true
		}
		if err := h.repo.DeleteSLARule(ctx, org, seg[1]); err != nil {
			return fail(err)
		}
		w.WriteHeader(http.StatusNoContent)
	case n == 3 && seg[0] == "projects" && seg[2] == "automations" && get:
		v, err := h.repo.ListAutomations(ctx, org, seg[1])
		if err != nil {
			return fail(err)
		}
		out := make([]map[string]any, 0, len(v))
		for _, a := range v {
			out = append(out, map[string]any{"rule": a, "description": DescribeAutomation(a)})
		}
		outJSON(w, map[string]any{"automations": out, "condition_fields": ConditionFields, "action_types": actionTypeList()})
	case n == 3 && seg[0] == "projects" && seg[2] == "automations" && post:
		if !admin() {
			return true
		}
		var a Automation
		if !decode(w, r, &a) {
			return true
		}
		v, err := h.repo.SaveAutomation(ctx, org, user, seg[1], a)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 4 && seg[0] == "projects" && seg[2] == "automations" && seg[3] == "test" && post:
		var in struct {
			Automation Automation `json:"automation"`
			TicketID   string     `json:"ticket_id"`
			Event      string     `json:"event"`
		}
		if !decode(w, r, &in) {
			return true
		}
		v, err := h.repo.TestAutomation(ctx, org, in.TicketID, in.Automation, in.Event)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 2 && seg[0] == "automations" && del:
		if !admin() {
			return true
		}
		if err := h.repo.DeleteAutomation(ctx, org, seg[1]); err != nil {
			return fail(err)
		}
		w.WriteHeader(http.StatusNoContent)
	case n == 3 && seg[0] == "projects" && seg[2] == "fields" && get:
		v, err := h.repo.ListFieldDefs(ctx, org, seg[1])
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"fields": v})
	case n == 3 && seg[0] == "projects" && seg[2] == "fields" && post:
		if !admin() {
			return true
		}
		var d FieldDef
		if !decode(w, r, &d) {
			return true
		}
		v, err := h.repo.SaveFieldDef(ctx, org, seg[1], d)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 2 && seg[0] == "fields" && del:
		if !admin() {
			return true
		}
		if err := h.repo.DeleteFieldDef(ctx, org, seg[1]); err != nil {
			return fail(err)
		}
		w.WriteHeader(http.StatusNoContent)
	case n == 2 && seg[0] == "me" && seg[1] == "ooo" && get:
		u, err := h.repo.GetOOO(ctx, org, user)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"until": u})
	case n == 3 && seg[0] == "projects" && seg[2] == "forms" && get:
		v, err := h.repo.ListForms(ctx, org, seg[1], false)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"forms": v})
	case n == 3 && seg[0] == "projects" && seg[2] == "forms" && post:
		if !admin() {
			return true
		}
		var f RequestForm
		if !decode(w, r, &f) {
			return true
		}
		v, err := h.repo.SaveForm(ctx, org, seg[1], f)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 2 && seg[0] == "forms" && del:
		if !admin() {
			return true
		}
		if err := h.repo.DeleteForm(ctx, org, seg[1]); err != nil {
			return fail(err)
		}
		w.WriteHeader(http.StatusNoContent)
	case n == 3 && seg[0] == "projects" && (seg[2] == "report2" || seg[2] == "report.csv") && get:
		days, _ := strconv.Atoi(q.Get("days"))
		o := ReportOpts{Days: days, From: q.Get("from"), To: q.Get("to"), Group: q.Get("group"), Type: q.Get("type"), Priority: q.Get("priority")}
		if seg[2] == "report.csv" {
			b, err := h.repo.ExportCSV(ctx, org, seg[1], o, time.Now())
			if err != nil {
				return fail(err)
			}
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="tickets.csv"`)
			_, _ = w.Write(b)
			return true
		}
		v, err := h.repo.ReportV2(ctx, org, seg[1], o, time.Now())
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 3 && seg[0] == "projects" && seg[2] == "saved-reports" && get:
		v, err := h.repo.ListSavedReports(ctx, org, seg[1])
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"reports": v})
	case n == 3 && seg[0] == "projects" && seg[2] == "saved-reports" && post:
		var s SavedReport
		if !decode(w, r, &s) {
			return true
		}
		v, err := h.repo.SaveReport(ctx, org, user, seg[1], s)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 2 && seg[0] == "saved-reports" && del:
		if err := h.repo.DeleteSavedReport(ctx, org, seg[1]); err != nil {
			return fail(err)
		}
		w.WriteHeader(http.StatusNoContent)
	case n == 4 && seg[0] == "projects" && seg[2] == "kb" && seg[3] == "suggest" && get:
		v, err := h.repo.SuggestArticles(ctx, org, seg[1], q.Get("q"), false)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"articles": v})
	case n == 4 && seg[0] == "projects" && seg[2] == "kb" && seg[3] == "stats" && get:
		v, err := h.repo.KBStats(ctx, org, seg[1])
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 1 && seg[0] == "major" && get:
		v, err := h.repo.MajorIncidents(ctx, org)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"incidents": v})
	case n == 2 && seg[0] == "approvals" && seg[1] == "mine" && get:
		v, err := h.repo.MyApprovals(ctx, org, user)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"tickets": v})
	case n == 2 && seg[0] == "tickets" && seg[1] == "from-source" && post:
		var in struct {
			Source    string `json:"source"`
			MessageID string `json:"message_id"`
			ProjectID string `json:"project_id"`
		}
		if !decode(w, r, &in) {
			return true
		}
		t, err := h.repo.FromSource(ctx, org, user, in.Source, in.MessageID, in.ProjectID)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"id": t.ID, "ref": fmt.Sprintf("%s-%d", t.ProjectKey, t.Number), "href": "/tickets?ticket=" + t.ID})
	case n == 3 && seg[0] == "tickets" && seg[2] == "issue" && post:
		var in struct {
			TeamID string `json:"team_id"`
		}
		_ = decodeOptional(r, &in)
		id, label, err := h.repo.CreateIssueFor(ctx, org, user, seg[1], in.TeamID)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"id": id, "key": label, "href": "/projects?issue=" + id})
	case n == 2 && seg[0] == "inbound" && seg[1] == "mail" && post:
		if !admin() {
			return true
		}
		var m InboundMail
		if !decode(w, r, &m) {
			return true
		}
		m.OrgID = org
		v, err := h.repo.IngestMail(ctx, m)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n == 1 && seg[0] == "sweep" && post:
		if !admin() {
			return true
		}
		v, err := h.repo.Sweep(ctx, time.Now())
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case n >= 3 && seg[0] == "tickets":
		return h.serveTicketSvc(w, r, ctx, org, user, name, seg)
	default:
		return false
	}
	return true
}

func actionTypeList() []string {
	return []string{"set_status", "set_priority", "set_type", "set_group", "set_assignee", "set_category", "require_skill", "add_tag", "remove_tag", "mark_major", "notify", "webhook", "create_task", "add_note"}
}

func (h *ExtrasHandler) serveTicketSvc(w http.ResponseWriter, r *http.Request, ctx context.Context, org, user, name string, seg []string) bool {
	m := r.Method
	get, post, put, del := m == http.MethodGet, m == http.MethodPost, m == http.MethodPut, m == http.MethodDelete
	id, sub := seg[1], seg[2]
	fail := func(err error) bool { failHTTP(w, err); return true }
	switch {
	case len(seg) == 3 && sub == "sla" && get:
		v, err := h.repo.SLAFor(ctx, org, id, time.Now())
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case len(seg) == 3 && sub == "scheduled" && get:
		v, err := h.repo.ListScheduled(ctx, org, id)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"scheduled": v})
	case len(seg) == 3 && sub == "scheduled" && post:
		var s Scheduled
		if !decode(w, r, &s) {
			return true
		}
		v, err := h.repo.ScheduleReply(ctx, org, user, name, id, s)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case len(seg) == 4 && sub == "scheduled" && del:
		admin := h.auth.IsAdmin != nil && h.auth.IsAdmin(r)
		if err := h.repo.CancelScheduled(ctx, org, user, admin, seg[3]); err != nil {
			return fail(err)
		}
		w.WriteHeader(http.StatusNoContent)
	case len(seg) == 3 && sub == "attachments" && get:
		v, err := h.repo.ListAttachments(ctx, org, id)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"attachments": v})
	case len(seg) == 3 && sub == "attachments" && post:
		var in struct {
			FileID string `json:"file_id"`
		}
		if !decode(w, r, &in) {
			return true
		}
		v, err := h.repo.AttachFile(ctx, org, user, id, in.FileID)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case len(seg) == 4 && sub == "attachments" && del:
		if err := h.repo.RemoveAttachment(ctx, org, seg[3]); err != nil {
			return fail(err)
		}
		w.WriteHeader(http.StatusNoContent)
	case len(seg) == 3 && sub == "ai" && post:
		var in struct {
			Kind string `json:"kind"`
		}
		if !decode(w, r, &in) {
			return true
		}
		var run AIRun
		if h.auth.AI != nil {
			run = h.auth.AI(r)
		}
		v, err := h.repo.RunAI(ctx, run, org, user, name, id, in.Kind)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case len(seg) == 4 && sub == "ai" && seg[3] == "apply" && post:
		var c Classification
		if !decode(w, r, &c) {
			return true
		}
		v, err := h.repo.ApplyClassification(ctx, org, id, c)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case len(seg) == 3 && sub == "presence" && post:
		var in struct {
			Mode string `json:"mode"`
		}
		_ = decodeOptional(r, &in)
		v, err := h.repo.Heartbeat(ctx, org, id, user, name, in.Mode)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"others": v})
	case len(seg) == 3 && sub == "draft" && get:
		v, err := h.repo.GetDraft(ctx, org, id, user)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case len(seg) == 3 && sub == "draft" && put:
		var d Draft
		if !decode(w, r, &d) {
			return true
		}
		if err := h.repo.SaveDraft(ctx, org, id, user, d); err != nil {
			return fail(err)
		}
		outJSON(w, map[string]bool{"saved": true})
	case len(seg) == 3 && sub == "time" && get:
		v, total, err := h.repo.ListTime(ctx, org, id)
		if err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"entries": v, "total_min": total})
	case len(seg) == 3 && sub == "time" && post:
		var e TimeEntry
		if !decode(w, r, &e) {
			return true
		}
		v, err := h.repo.AddTime(ctx, org, user, name, id, e)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case len(seg) == 4 && sub == "time" && del:
		admin := h.auth.IsAdmin != nil && h.auth.IsAdmin(r)
		if err := h.repo.DeleteTime(ctx, org, user, admin, seg[3]); err != nil {
			return fail(err)
		}
		w.WriteHeader(http.StatusNoContent)
	case len(seg) == 3 && sub == "snooze" && put:
		var in struct {
			Until *time.Time `json:"until"`
		}
		if !decode(w, r, &in) {
			return true
		}
		if err := h.repo.Snooze(ctx, org, id, in.Until); err != nil {
			return fail(err)
		}
		outJSON(w, map[string]any{"until": in.Until})
	case len(seg) == 3 && sub == "parent" && put:
		var in struct {
			ParentID string `json:"parent_id"`
		}
		if !decode(w, r, &in) {
			return true
		}
		if err := h.repo.SetParent(ctx, org, id, in.ParentID); err != nil {
			return fail(err)
		}
		outJSON(w, map[string]string{"parent_id": in.ParentID})
	case len(seg) == 3 && sub == "major" && put:
		var in struct {
			On bool `json:"on"`
		}
		if !decode(w, r, &in) {
			return true
		}
		if err := h.repo.SetMajor(ctx, org, id, in.On); err != nil {
			return fail(err)
		}
		outJSON(w, map[string]bool{"major": in.On})
	case len(seg) == 3 && sub == "fields" && put:
		var f FieldsPatch
		if !decode(w, r, &f) {
			return true
		}
		v, err := h.repo.SetFields(ctx, org, id, f)
		if err != nil {
			return fail(err)
		}
		outJSON(w, v)
	case len(seg) == 3 && sub == "detail2" && get:
		kids, err := h.repo.Children(ctx, org, id)
		if err != nil {
			return fail(err)
		}
		ap, state, err := h.repo.Approvals(ctx, org, id)
		if err != nil {
			return fail(err)
		}
		te, total, err := h.repo.ListTime(ctx, org, id)
		if err != nil {
			return fail(err)
		}
		sla, _ := h.repo.SLAFor(ctx, org, id, time.Now())
		outJSON(w, map[string]any{"children": kids, "approvals": ap, "approval_state": state, "time": te, "time_total_min": total, "sla": sla})
	case len(seg) == 3 && sub == "approvals" && post:
		var in struct {
			Steps [][]string `json:"steps"`
		}
		if !decode(w, r, &in) {
			return true
		}
		if err := h.repo.RequestApprovals(ctx, org, id, in.Steps); err != nil {
			return fail(err)
		}
		outJSON(w, map[string]bool{"requested": true})
	case len(seg) == 4 && sub == "approvals" && seg[3] == "decide" && post:
		var in struct {
			Decision string `json:"decision"`
			Comment  string `json:"comment"`
		}
		if !decode(w, r, &in) {
			return true
		}
		if err := h.repo.Decide(ctx, org, id, user, in.Decision, in.Comment); err != nil {
			return fail(err)
		}
		outJSON(w, map[string]bool{"ok": true})
	default:
		return false
	}
	return true
}

// decodeOptional reads a small JSON body when there is one.
func decodeOptional(r *http.Request, v any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 16<<10)
	return json.NewDecoder(r.Body).Decode(v)
}
