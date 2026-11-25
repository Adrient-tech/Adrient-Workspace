package contacts

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

// ExtrasHandler serves contact extras under /api/v1/cx/ (kept off the
// grpc-gateway /api/v1/contacts routes):
//
//	GET/PUT /api/v1/cx/{contactId}/extras
//	GET     /api/v1/cx/{contactId}/links
//	GET     /api/v1/cx/day-events?from=&to=
//	GET     /api/v1/cx/history
//	POST    /api/v1/cx/history/{id}/undo
type ExtrasHandler struct {
	repo *Repository
	a    ExtrasAuth
}

// NewExtrasHandler builds the handler.
func NewExtrasHandler(repo *Repository, a ExtrasAuth) *ExtrasHandler {
	return &ExtrasHandler{repo: repo, a: a}
}

const cxMount = "/api/v1/cx/"

// Match reports whether path belongs to this handler.
func (h *ExtrasHandler) Match(path string) bool { return strings.HasPrefix(path, cxMount) }

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

func (h *ExtrasHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok1 := h.a.OrgID(r)
	user, ok2 := h.a.UserID(r)
	if !ok1 || !ok2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, cxMount), "/"), "/")
	switch {
	case len(seg) == 1 && seg[0] == "page" && r.Method == http.MethodGet:
		h.servePage(w, r, org)
	case len(seg) == 1 && seg[0] == "day-events" && r.Method == http.MethodGet:
		from, e1 := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		to, e2 := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if e1 != nil || e2 != nil || !to.After(from) {
			http.Error(w, "from and to (RFC 3339) are required", 400)
			return
		}
		v, err := h.repo.DayEvents(ctx, org, user, from.UTC().Truncate(24*time.Hour), to.UTC())
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"events": v})
	case len(seg) == 1 && seg[0] == "history" && r.Method == http.MethodGet:
		v, err := h.repo.History(ctx, org, user)
		if err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]any{"history": v})
	case len(seg) == 3 && seg[0] == "history" && seg[2] == "undo" && r.Method == http.MethodPost:
		if err := h.repo.Undo(ctx, org, user, seg[1]); err != nil {
			fail(w, err)
			return
		}
		out(w, map[string]bool{"ok": true})
	case len(seg) == 2 && seg[1] == "extras" && r.Method == http.MethodGet:
		v, err := h.repo.GetExtras(ctx, org, seg[0])
		if err != nil {
			fail(w, err)
			return
		}
		out(w, v)
	case len(seg) == 2 && seg[1] == "extras" && r.Method == http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		var e Extras
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if err := h.repo.SaveExtras(ctx, org, seg[0], e); err != nil {
			fail(w, err)
			return
		}
		v, _ := h.repo.GetExtras(ctx, org, seg[0])
		out(w, v)
	case len(seg) == 2 && seg[1] == "links" && r.Method == http.MethodGet:
		v, err := h.repo.LinksFor(ctx, org, seg[0])
		if err != nil {
			fail(w, err)
			return
		}
		out(w, v)
	default:
		http.NotFound(w, r)
	}
}
