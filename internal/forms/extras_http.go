package forms

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// ExtrasAuth extracts the caller from the request.
type ExtrasAuth struct {
	UserID   func(r *http.Request) (string, bool)
	OrgID    func(r *http.Request) (string, bool)
	UserName func(r *http.Request) string
}

// ExtrasHandler serves form extras under /api/v1/fx/:
//
//	GET/PUT /fx/{id}/extras   theme colours and owner notification
//	POST    /fx/{id}/send     {emails, group_id, message} email the form link
type ExtrasHandler struct {
	repo *Repository
	a    ExtrasAuth
	Send SendDeps
}

// NewExtrasHandler builds the handler.
func NewExtrasHandler(repo *Repository, a ExtrasAuth, d SendDeps) *ExtrasHandler {
	return &ExtrasHandler{repo: repo, a: a, Send: d}
}

const fxMount = "/api/v1/fx/"

// Match reports whether path belongs to this handler.
func (h *ExtrasHandler) Match(path string) bool { return strings.HasPrefix(path, fxMount) }

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
	case errors.Is(err, ErrMailOff):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
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
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, fxMount), "/"), "/")
	switch {
	case len(seg) == 2 && seg[1] == "extras" && r.Method == http.MethodGet:
		v, err := h.repo.GetExtras(ctx, org, user, seg[0])
		if err != nil {
			fail(w, err)
			return
		}
		out(w, v)
	case len(seg) == 2 && seg[1] == "extras" && r.Method == http.MethodPut:
		var e Extras
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&e); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if err := h.repo.SaveExtras(ctx, org, user, seg[0], e); err != nil {
			fail(w, err)
			return
		}
		out(w, e)
	case len(seg) == 2 && seg[1] == "send" && r.Method == http.MethodPost:
		var in struct {
			Emails  []string `json:"emails"`
			GroupID string   `json:"group_id"`
			Message string   `json:"message"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		n, err := h.repo.SendInvites(ctx, h.Send, org, user, h.a.UserName(r), seg[0], in.Emails, in.GroupID, in.Message)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]int{"sent": n})
	default:
		http.NotFound(w, r)
	}
}
