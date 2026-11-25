package projects

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// ExtrasAuth extracts the caller from the request.
type ExtrasAuth struct {
	UserID func(r *http.Request) (string, bool)
	OrgID  func(r *http.Request) (string, bool)
}

// ExtrasHandler serves plan extras under /api/v1/px/ (kept off the
// grpc-gateway /api/v1/projects routes):
//
//	GET  /px/extras                              dates, estimates, prerequisites of all issues
//	PUT  /px/issues/{id}/extras                  save them for one issue
//	POST /px/issues/{id}/log {hours}             log time
//	GET  /px/milestones?project=   POST /px/projects/{id}/milestones   PUT/DELETE /px/milestones/{id}
//	GET/PUT /px/projects/{id}/brief
//	GET/POST /px/views   DELETE /px/views/{id}
//	GET  /px/templates   POST /px/from-template
//	GET/POST /px/projects/{id}/links   DELETE /px/links/{id}   GET /px/pick?kind=&q=
type ExtrasHandler struct {
	repo *Repository
	a    ExtrasAuth
}

// NewExtrasHandler builds the handler.
func NewExtrasHandler(repo *Repository, a ExtrasAuth) *ExtrasHandler {
	return &ExtrasHandler{repo: repo, a: a}
}

const pxMount = "/api/v1/px/"

// Match reports whether path belongs to this handler.
func (h *ExtrasHandler) Match(path string) bool { return strings.HasPrefix(path, pxMount) }

func out(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid input: "), http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(v); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

func (h *ExtrasHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok1 := h.a.OrgID(r)
	user, ok2 := h.a.UserID(r)
	if !ok1 || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, pxMount), "/"), "/")
	get, post, put, del := r.Method == http.MethodGet, r.Method == http.MethodPost, r.Method == http.MethodPut, r.Method == http.MethodDelete
	switch {
	case len(seg) == 1 && seg[0] == "extras" && get:
		v, err := h.repo.ListExtras(ctx, org)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"extras": v})
	case len(seg) == 3 && seg[0] == "issues" && seg[2] == "extras" && put:
		var in IssueExtras
		if !decode(w, r, &in) {
			return
		}
		if err := h.repo.SaveIssueExtras(ctx, org, seg[1], in); err != nil {
			fail(w, err)
			return
		}
		out(w, in)
	case len(seg) == 1 && seg[0] == "archive" && post:
		var in struct {
			IDs      []string `json:"ids"`
			Archived bool     `json:"archived"`
		}
		if !decode(w, r, &in) {
			return
		}
		n, err := h.repo.SetArchived(ctx, org, in.IDs, in.Archived)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]int{"changed": n})
	case len(seg) == 1 && seg[0] == "wip" && get:
		v, err := h.repo.ListWip(ctx, org, r.URL.Query().Get("team"))
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"limits": v})
	case len(seg) == 1 && seg[0] == "wip" && put:
		var in struct {
			TeamID string `json:"team_id"`
			Status string `json:"status"`
			Max    int    `json:"max"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := h.repo.SetWip(ctx, org, in.TeamID, in.Status, in.Max); err != nil {
			fail(w, err)
			return
		}
		out(w, in)
	case len(seg) == 3 && seg[0] == "issues" && seg[2] == "log" && post:
		var in struct {
			Hours float64 `json:"hours"`
		}
		if !decode(w, r, &in) {
			return
		}
		total, err := h.repo.LogHours(ctx, org, seg[1], in.Hours)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]float64{"logged_hours": total})
	case len(seg) == 1 && seg[0] == "milestones" && get:
		v, err := h.repo.ListMilestones(ctx, org, r.URL.Query().Get("project"))
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"milestones": v})
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "milestones" && post:
		var m Milestone
		if !decode(w, r, &m) {
			return
		}
		m.ID = ""
		v, err := h.repo.SaveMilestone(ctx, org, seg[1], m)
		if err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		out(w, v)
	case len(seg) == 2 && seg[0] == "milestones" && put:
		var in struct {
			Milestone
		}
		if !decode(w, r, &in) {
			return
		}
		in.ID = seg[1]
		v, err := h.repo.SaveMilestone(ctx, org, in.ProjectID, in.Milestone)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, v)
	case len(seg) == 2 && seg[0] == "milestones" && del:
		if err := h.repo.DeleteMilestone(ctx, org, seg[1]); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "brief" && get:
		b, err := h.repo.GetBrief(ctx, org, seg[1])
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]string{"body": b})
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "brief" && put:
		var in struct {
			Body string `json:"body"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := h.repo.SaveBrief(ctx, org, user, seg[1], in.Body); err != nil {
			fail(w, err)
			return
		}
		out(w, in)
	case len(seg) == 1 && seg[0] == "views" && get:
		v, err := h.repo.ListViews(ctx, org, user)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"views": v})
	case len(seg) == 1 && seg[0] == "views" && post:
		var v SavedView
		if !decode(w, r, &v) {
			return
		}
		v, err := h.repo.CreateView(ctx, org, user, v)
		if err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		out(w, v)
	case len(seg) == 2 && seg[0] == "views" && del:
		if err := h.repo.DeleteView(ctx, org, user, seg[1]); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case len(seg) == 1 && seg[0] == "templates" && get:
		var list []map[string]any
		for _, x := range Templates() {
			list = append(list, map[string]any{"id": x.ID, "name": x.Name, "description": x.Description, "issues": len(x.Issues), "milestones": len(x.Milestones)})
		}
		out(w, map[string]any{"templates": list})
	case len(seg) == 1 && seg[0] == "from-template" && post:
		var in struct {
			Template string `json:"template"`
			Name     string `json:"name"`
			TeamID   string `json:"team_id"`
			Start    string `json:"start_date"`
		}
		if !decode(w, r, &in) {
			return
		}
		p, err := h.repo.FromTemplate(ctx, org, user, in.Template, in.Name, in.TeamID, in.Start)
		if err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		out(w, map[string]any{"id": p.ID, "name": p.Name})
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "links" && get:
		v, err := h.repo.ListLinks(ctx, org, seg[1])
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"links": v})
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "links" && post:
		var in struct {
			Kind  string `json:"kind"`
			RefID string `json:"ref_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := h.repo.AddLink(ctx, org, seg[1], in.Kind, in.RefID); err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]bool{"ok": true})
	case len(seg) == 2 && seg[0] == "links" && del:
		if err := h.repo.DeleteLink(ctx, org, seg[1]); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case len(seg) == 1 && seg[0] == "pick" && get:
		v, err := h.repo.Pick(ctx, org, r.URL.Query().Get("kind"), r.URL.Query().Get("q"))
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"items": v})
	default:
		http.NotFound(w, r)
	}
}
