package accessreview

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Paths. Admins manage campaigns under /admin; any signed-in reviewer uses /me.
const (
	AdminBase = "/api/v1/admin/reviews"
	MeBase    = "/api/v1/me/reviews"
)

// Handler serves the access review API.
type Handler struct{ Svc *Service }

// Match reports whether path belongs to this handler.
func Match(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == AdminBase || strings.HasPrefix(p, AdminBase+"/") || p == MeBase || strings.HasPrefix(p, MeBase+"/")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	code, msg := http.StatusInternalServerError, "request failed"
	switch {
	case errors.Is(err, ErrBad):
		code, msg = http.StatusBadRequest, err.Error()
	case errors.Is(err, ErrNotFound):
		code, msg = http.StatusNotFound, "not found"
	case errors.Is(err, ErrForbid):
		code, msg = http.StatusForbidden, err.Error()
	}
	writeJSON(w, code, map[string]any{"error": msg})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	u, ok2 := auth.UserFromContext(r.Context())
	if !ok || !ok2 {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	ctx := r.Context()
	admin := authz.IsAdmin(ctx)
	path := strings.TrimRight(r.URL.Path, "/")
	decide := func(itemID string) {
		var b struct {
			Decision string `json:"decision"`
			Comment  string `json:"comment"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil {
			fail(w, ErrBad)
			return
		}
		if err := h.Svc.Decide(ctx, o.ID, u.ID, admin, itemID, b.Decision, b.Comment); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
	if strings.HasPrefix(path, MeBase) {
		seg := strings.Split(strings.Trim(strings.TrimPrefix(path, MeBase), "/"), "/")
		switch {
		case seg[0] == "" && r.Method == http.MethodGet:
			l, err := h.Svc.Mine(ctx, o.ID, u.ID)
			if err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": l})
		case len(seg) == 2 && seg[1] == "decision" && r.Method == http.MethodPost:
			// Reviewers are not admins: decide() lets only the assigned reviewer through.
			admin = false
			decide(seg[0])
		default:
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		}
		return
	}
	perm := "admin:reviews:read"
	if r.Method != http.MethodGet {
		perm = "admin:reviews:write"
	}
	if !admin && !authz.Can(ctx, perm) {
		authz.WriteForbidden(w, perm)
		return
	}
	seg := strings.Split(strings.Trim(strings.TrimPrefix(path, AdminBase), "/"), "/")
	switch {
	case seg[0] == "" && r.Method == http.MethodGet:
		l, err := h.Svc.List(ctx, o.ID)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"campaigns": l})
	case seg[0] == "" && r.Method == http.MethodPost:
		var in CreateInput
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil {
			fail(w, ErrBad)
			return
		}
		c, err := h.Svc.Create(ctx, o.ID, u.ID, in)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, c)
	case len(seg) == 1 && r.Method == http.MethodGet:
		c, err := h.Svc.Get(ctx, o.ID, seg[0])
		if err != nil {
			fail(w, err)
			return
		}
		qv := r.URL.Query()
		limit, _ := strconv.Atoi(qv.Get("limit"))
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		page, _ := strconv.Atoi(qv.Get("page"))
		if page < 1 {
			page = 1
		}
		items, err := h.Svc.ItemsPage(ctx, o.ID, seg[0], qv.Get("q"), qv.Get("decision"), limit+1, (page-1)*limit)
		if err != nil {
			fail(w, err)
			return
		}
		next := ""
		if len(items) > limit {
			items, next = items[:limit], strconv.Itoa(page+1)
		}
		writeJSON(w, http.StatusOK, map[string]any{"campaign": c, "items": items, "next_cursor": next})
	case len(seg) == 2 && seg[1] == "close" && r.Method == http.MethodPost:
		c, err := h.Svc.Close(ctx, o.ID, u.ID, seg[0])
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	case len(seg) == 2 && seg[1] == "export" && r.Method == http.MethodGet:
		csv, err := h.Svc.Export(ctx, o.ID, seg[0])
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="access-review.csv"`)
		_, _ = w.Write([]byte(csv))
	case len(seg) == 3 && seg[1] == "items" && r.Method == http.MethodPost:
		decide(seg[2])
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}
