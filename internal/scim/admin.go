package scim

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Admin API paths (admin console).
const (
	PathTokens = "/api/v1/admin/scim/tokens"
	PathEvents = "/api/v1/admin/scim/events"
	PathInfo   = "/api/v1/admin/scim/info"
)

// Admin serves token management for org admins.
type Admin struct {
	Store   *Store
	BaseURL string
	Audit   func(r *http.Request, action string, detail map[string]any)
}

// MatchAdmin reports whether path belongs to the admin API.
func MatchAdmin(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == PathTokens || strings.HasPrefix(p, PathTokens+"/") || p == PathEvents || p == PathInfo
}

func (a *Admin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	u, ok2 := auth.UserFromContext(r.Context())
	if !ok || !ok2 {
		writeJSONPlain(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if !authz.IsAdmin(r.Context()) {
		authz.WriteForbidden(w, authz.PermOrgAdmin)
		return
	}
	path := strings.TrimRight(r.URL.Path, "/")
	switch {
	case path == PathInfo && r.Method == http.MethodGet:
		base := a.BaseURL
		if base == "" {
			proto := r.Header.Get("X-Forwarded-Proto")
			if proto == "" {
				proto = "http"
				if r.TLS != nil {
					proto = "https"
				}
			}
			host := r.Header.Get("X-Forwarded-Host")
			if host == "" {
				host = r.Host
			}
			base = proto + "://" + host
		}
		writeJSONPlain(w, http.StatusOK, map[string]any{"base_url": strings.TrimRight(base, "/") + Mount,
			"user_name_attribute": "userName (the user's e-mail address)", "auth": "Bearer token"})
	case path == PathEvents && r.Method == http.MethodGet:
		before, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
		ev, err := a.Store.EventsBefore(r.Context(), o.ID, 101, before)
		if err != nil {
			writeJSONPlain(w, http.StatusInternalServerError, map[string]any{"error": "read failed"})
			return
		}
		next := ""
		if len(ev) > 100 {
			ev = ev[:100]
			next = strconv.FormatInt(ev[99].ID, 10)
		}
		writeJSONPlain(w, http.StatusOK, map[string]any{"events": ev, "items": ev, "next_cursor": next})
	case path == PathTokens && r.Method == http.MethodGet:
		l, err := a.Store.ListTokens(r.Context(), o.ID)
		if err != nil {
			writeJSONPlain(w, http.StatusInternalServerError, map[string]any{"error": "read failed"})
			return
		}
		writeJSONPlain(w, http.StatusOK, map[string]any{"tokens": l})
	case path == PathTokens && r.Method == http.MethodPost:
		var in struct {
			Name       string `json:"name"`
			RatePerMin int    `json:"rate_per_min"`
			ExpireDays int    `json:"expire_days"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
			writeJSONPlain(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		var exp *time.Time
		if in.ExpireDays > 0 {
			t := time.Now().AddDate(0, 0, in.ExpireDays)
			exp = &t
		}
		plain, t, err := a.Store.CreateToken(r.Context(), o.ID, u.ID, in.Name, in.RatePerMin, exp)
		if err != nil {
			writeJSONPlain(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if a.Audit != nil {
			a.Audit(r, "scim.token.create", map[string]any{"token_id": t.ID, "name": t.Name})
		}
		writeJSONPlain(w, http.StatusCreated, map[string]any{"token": plain, "meta": t,
			"note": "Copy the token now. It is not shown again."})
	case strings.HasPrefix(path, PathTokens+"/") && r.Method == http.MethodDelete:
		id := strings.TrimPrefix(path, PathTokens+"/")
		done, err := a.Store.RevokeToken(r.Context(), o.ID, id)
		if err != nil || !done {
			writeJSONPlain(w, http.StatusNotFound, map[string]any{"error": "token not found"})
			return
		}
		if a.Audit != nil {
			a.Audit(r, "scim.token.revoke", map[string]any{"token_id": id})
		}
		writeJSONPlain(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSONPlain(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func writeJSONPlain(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
