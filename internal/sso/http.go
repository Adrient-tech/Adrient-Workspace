package sso

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/zitadelmgmt"
)

// Paths.
const (
	AdminBase    = "/api/v1/admin/sso"
	PathDiscover = "/api/v1/auth/sso/discover"
	PathStart    = "/api/v1/auth/sso/start"
)

// Starter builds the redirect to the identity provider for one Zitadel provider
// (implemented by auth.Service).
type Starter interface {
	StartSSO(w http.ResponseWriter, r *http.Request, idpID, loginHint string) error
}

// Handler serves the admin API and the public discover/start endpoints.
type Handler struct {
	Svc     *Service
	Starter Starter
}

// MatchAdmin reports whether path is an admin path.
func MatchAdmin(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == AdminBase || strings.HasPrefix(p, AdminBase+"/")
}

// MatchPublic reports whether path is a public SSO path.
func MatchPublic(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == PathDiscover || p == PathStart
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
		code, msg = http.StatusNotFound, "connection not found"
	}
	var ze *zitadelmgmt.Error
	if errors.As(err, &ze) {
		code, msg = http.StatusBadGateway, err.Error()
	}
	writeJSON(w, code, map[string]any{"error": msg})
}

// ServePublic answers GET /api/v1/auth/sso/discover and /start (no session).
func (h *Handler) ServePublic(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	c, ok := h.Svc.Discover(r.Context(), email)
	switch strings.TrimRight(r.URL.Path, "/") {
	case PathDiscover:
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{"sso": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sso": true, "name": c.Name, "kind": c.Kind, "enforced": c.Enforce,
			"start_url": PathStart + "?email=" + urlQuery(email)})
	case PathStart:
		if !ok || h.Starter == nil {
			http.Error(w, "Single sign-on is not set up for that e-mail domain.", http.StatusNotFound)
			return
		}
		if err := h.Starter.StartSSO(w, r, c.IdPID, email); err != nil {
			http.Error(w, "Could not start single sign-on.", http.StatusBadGateway)
		}
	}
}

func urlQuery(s string) string {
	r := strings.NewReplacer("%", "%25", "&", "%26", "+", "%2B", " ", "%20", "#", "%23", "?", "%3F", "=", "%3D")
	return r.Replace(s)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	u, ok2 := auth.UserFromContext(r.Context())
	if !ok || !ok2 {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	perm := "admin:identity:read"
	if r.Method != http.MethodGet {
		perm = "admin:identity:write"
	}
	if !authz.IsAdmin(r.Context()) && !authz.Can(r.Context(), perm) {
		authz.WriteForbidden(w, perm)
		return
	}
	ctx := r.Context()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(strings.TrimRight(r.URL.Path, "/"), AdminBase), "/"), "/")
	switch {
	case seg[0] == "" && r.Method == http.MethodGet:
		l, err := h.Svc.List(ctx, o.ID)
		if err != nil {
			fail(w, err)
			return
		}
		var verified []string
		rows, qerr := h.Svc.Pool.Query(ctx, `SELECT domain FROM adrient.org_domains WHERE org_id::text=$1 AND status='verified' ORDER BY domain`, o.ID)
		if qerr == nil {
			for rows.Next() {
				var d string
				if rows.Scan(&d) == nil {
					verified = append(verified, d)
				}
			}
			rows.Close()
		}
		callback := ""
		if origin := r.Header.Get("Origin"); origin != "" {
			callback = origin
		}
		writeJSON(w, http.StatusOK, map[string]any{"connections": l, "verified_domains": verified,
			"zitadel_connected": h.Svc.ZM != nil, "callback_hint": callback})
	case seg[0] == "" && r.Method == http.MethodPost:
		var in CreateInput
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&in) != nil {
			fail(w, ErrBad)
			return
		}
		c, err := h.Svc.Create(ctx, o.ID, u.ID, in)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, c)
	case len(seg) == 1 && r.Method == http.MethodPatch:
		var in UpdateInput
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil {
			fail(w, ErrBad)
			return
		}
		c, err := h.Svc.Update(ctx, o.ID, u.ID, seg[0], in)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	case len(seg) == 1 && r.Method == http.MethodDelete:
		if err := h.Svc.Delete(ctx, o.ID, u.ID, seg[0]); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}
