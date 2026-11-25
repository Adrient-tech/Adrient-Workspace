package tasks

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// ExtrasAuth extracts the caller from the request.
type ExtrasAuth struct {
	UserID func(r *http.Request) (string, bool)
	OrgID  func(r *http.Request) (string, bool)
}

// ExtrasHandler serves task extras under /api/v1/tx/ (kept off the
// grpc-gateway /api/v1/tasks routes):
//
//	GET  /tx/lists/{id}/extras                    extras of every task in a list
//	GET  /tx/lists/{id}/shares   POST add   DELETE /tx/lists/{id}/shares/{userId}
//	GET  /tx/shared-lists                         lists shared with me
//	GET  /tx/assigned                             open tasks assigned to me
//	PUT  /tx/tasks/{id}/extras                    assignee, priority, repeat, reminder, board column
//	POST /tx/quick                                make a task from Mail or Chat
type ExtrasHandler struct {
	repo *Repository
	a    ExtrasAuth
	// OnAssigned is called after a task is assigned to someone else.
	OnAssigned func(org, from, to, taskID, title string)
}

// NewExtrasHandler builds the handler.
func NewExtrasHandler(repo *Repository, a ExtrasAuth) *ExtrasHandler {
	return &ExtrasHandler{repo: repo, a: a}
}

const txMount = "/api/v1/tx/"

// Match reports whether path belongs to this handler.
func (h *ExtrasHandler) Match(path string) bool { return strings.HasPrefix(path, txMount) }

func out(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrForbidden):
		http.Error(w, "you do not have access to this list", http.StatusForbidden)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid input: "), http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

type taskJSON struct {
	ID       string    `json:"id"`
	ListID   string    `json:"list_id"`
	ListName string    `json:"list_name,omitempty"`
	Title    string    `json:"title"`
	Notes    string    `json:"notes"`
	DueAt    string    `json:"due_at"`
	Extras   *Extras   `json:"extras,omitempty"`
	Created  time.Time `json:"created_at"`
}

func toJSON(t TaskItem) taskJSON {
	j := taskJSON{ID: t.ID, ListID: t.ListID, Title: t.Title, Notes: t.Notes, Created: t.CreatedAt}
	if t.DueAt != nil {
		j.DueAt = t.DueAt.UTC().Format(time.RFC3339)
	}
	return j
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
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, txMount), "/"), "/")
	switch {
	case len(seg) == 1 && seg[0] == "feed" && r.Method == http.MethodGet, len(seg) == 2 && seg[0] == "feed" && seg[1] == "rotate" && r.Method == http.MethodPost:
		t, err := h.repo.FeedToken(ctx, org, user, len(seg) == 2)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]string{"token": t, "path": "/api/v1/public/tasks/feed/" + t + ".ics"})
	case len(seg) == 3 && seg[0] == "lists" && seg[2] == "public" && (r.Method == http.MethodGet || r.Method == http.MethodPost || r.Method == http.MethodDelete):
		var t string
		var err error
		switch r.Method {
		case http.MethodGet:
			t, err = h.repo.PublicLink(ctx, org, user, seg[1])
		case http.MethodPost:
			t, err = h.repo.CreatePublicLink(ctx, org, user, seg[1], r.URL.Query().Get("rotate") == "1")
		default:
			err = h.repo.RevokePublicLink(ctx, org, user, seg[1])
		}
		if err != nil {
			fail(w, err)
			return
		}
		path := ""
		if t != "" {
			path = "/api/v1/public/tasks/list/" + t
		}
		out(w, map[string]string{"token": t, "path": path})
	case len(seg) == 1 && seg[0] == "tags" && r.Method == http.MethodGet:
		v, err := h.repo.Tags(ctx, org, user)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"tags": v})
	case len(seg) == 1 && seg[0] == "export.ics" && r.Method == http.MethodGet:
		v, err := h.repo.OpenDated(ctx, org, user)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="adrient-tasks.ics"`)
		host := r.Host
		if i := strings.Index(host, ":"); i > 0 {
			host = host[:i]
		}
		_, _ = w.Write([]byte(BuildICS(v, host)))
	case len(seg) == 1 && seg[0] == "shared-lists" && r.Method == http.MethodGet:
		v, err := h.repo.SharedWithMe(ctx, org, user)
		if err != nil {
			fail(w, err)
			return
		}
		res := make([]map[string]string, 0, len(v))
		for _, s := range v {
			res = append(res, map[string]string{"id": s.ID, "name": s.Name, "owner_name": s.OwnerName, "role": s.Role})
		}
		out(w, map[string]any{"lists": res})
	case len(seg) == 1 && seg[0] == "assigned" && r.Method == http.MethodGet:
		v, err := h.repo.Assigned(ctx, org, user)
		if err != nil {
			fail(w, err)
			return
		}
		res := make([]taskJSON, 0, len(v))
		for _, a := range v {
			j := toJSON(a.TaskItem)
			j.ListName = a.ListName
			e := a.Extras
			j.Extras = &e
			res = append(res, j)
		}
		out(w, map[string]any{"tasks": res})
	case len(seg) == 1 && seg[0] == "quick" && r.Method == http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var in struct {
			Title string `json:"title"`
			Notes string `json:"notes"`
			Link  string `json:"link"`
			Due   string `json:"due_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		var due *time.Time
		if in.Due != "" {
			d, err := time.Parse(time.RFC3339, in.Due)
			if err != nil {
				http.Error(w, "due_at must be RFC 3339", 400)
				return
			}
			due = &d
		}
		t, err := h.repo.Quick(ctx, org, user, in.Title, in.Notes, in.Link, due)
		if err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		out(w, toJSON(t))
	case len(seg) == 3 && seg[0] == "lists" && seg[2] == "extras" && r.Method == http.MethodGet:
		if err := h.repo.Require(ctx, org, user, seg[1], false); err != nil {
			fail(w, err)
			return
		}
		v, err := h.repo.ListExtras(ctx, org, seg[1])
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"extras": v})
	case len(seg) == 3 && seg[0] == "lists" && seg[2] == "shares" && r.Method == http.MethodGet:
		v, err := h.repo.Shares(ctx, org, user, seg[1])
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"shares": v})
	case len(seg) == 3 && seg[0] == "lists" && seg[2] == "shares" && r.Method == http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		var in struct{ Email, Role string }
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if in.Role == "" {
			in.Role = RoleEditor
		}
		s, err := h.repo.AddShare(ctx, org, user, seg[1], in.Email, in.Role)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, s)
	case len(seg) == 4 && seg[0] == "lists" && seg[2] == "shares" && r.Method == http.MethodDelete:
		if err := h.repo.RemoveShare(ctx, org, user, seg[1], seg[3]); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case len(seg) == 3 && seg[0] == "tasks" && seg[2] == "extras" && r.Method == http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		var e Extras
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		changed, err := h.repo.SaveExtras(ctx, org, user, seg[1], e)
		if err != nil {
			fail(w, err)
			return
		}
		if changed && e.AssigneeID != user && h.OnAssigned != nil {
			if t, err := h.repo.GetTask(ctx, org, seg[1]); err == nil {
				h.OnAssigned(org, user, e.AssigneeID, t.ID, t.Title)
			}
		}
		e.TaskID = seg[1]
		out(w, e)
	default:
		http.NotFound(w, r)
	}
}
