package orgdomains

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// PermDomainWrite is the permission needed for every admin domain endpoint.
// Owner and Admin hold it through "*".
const PermDomainWrite = "identity:domain:write"

// AuditFn records an identity event (audit.Recorder.Emit).
type AuditFn func(ctx context.Context, service, action string, detail map[string]any)

// Paths served by Handler.
const (
	PathDomains  = "/api/v1/admin/domains"
	PathSettings = "/api/v1/admin/org/settings"
	PathHint     = "/api/v1/auth/domain-hint"
)

// Handler serves the domain and org-settings admin APIs.
type Handler struct {
	Store *Store
	Audit AuditFn

	mu   sync.Mutex
	hits map[string][]time.Time
}

// Match reports whether path belongs to this handler.
func Match(path string) bool {
	path = strings.TrimRight(path, "/")
	return path == PathDomains || strings.HasPrefix(path, PathDomains+"/") || path == PathSettings || path == PathHint
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func jerr(w http.ResponseWriter, code int, msg string) { writeJSON(w, code, map[string]any{"error": msg}) }

func (h *Handler) audit(r *http.Request, action string, detail map[string]any) {
	if h.Audit != nil {
		if detail == nil {
			detail = map[string]any{}
		}
		detail["ip"] = clientIP(r)
		h.Audit(r.Context(), "domain", action, detail)
	}
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	return r.RemoteAddr
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimRight(r.URL.Path, "/")
	if path == PathHint {
		h.hint(w, r)
		return
	}
	o, ok := auth.OrgFromContext(r.Context())
	if _, hasUser := auth.UserFromContext(r.Context()); !ok || !hasUser {
		jerr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !authz.Can(r.Context(), PermDomainWrite) {
		authz.WriteForbidden(w, PermDomainWrite)
		return
	}
	switch {
	case path == PathDomains && r.Method == http.MethodGet:
		ds, err := h.Store.List(r.Context(), o.ID)
		if err != nil {
			jerr(w, http.StatusInternalServerError, "list failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"domains": ds})
	case path == PathDomains && r.Method == http.MethodPost:
		var in struct {
			Domain string `json:"domain"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
			jerr(w, http.StatusBadRequest, "invalid body")
			return
		}
		d, err := h.Store.Add(r.Context(), o.ID, in.Domain)
		if err != nil {
			h.fail(w, r, "add", in.Domain, err)
			return
		}
		h.audit(r, "add", map[string]any{"resource_type": "domain", "resource_id": d.ID, "domain": d.Domain, "method": "txt"})
		writeJSON(w, http.StatusCreated, d)
	case path == PathSettings && r.Method == http.MethodGet:
		st, err := h.Store.GetSettings(r.Context(), o.ID)
		if err != nil {
			jerr(w, http.StatusInternalServerError, "settings failed")
			return
		}
		writeJSON(w, http.StatusOK, st)
	case path == PathSettings && r.Method == http.MethodPut:
		var in Settings
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in) != nil {
			jerr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if cu, ok := auth.UserFromContext(r.Context()); ok && len(in.AllowedSigninDomains) > 0 {
			mine, found := EmailDomain(cu.Email), false
			for _, d := range in.AllowedSigninDomains {
				if strings.EqualFold(strings.TrimSpace(d), mine) {
					found = true
				}
			}
			if !found {
				jerr(w, http.StatusBadRequest, "the allow list must include your own e-mail domain ("+mine+") so you are not locked out")
				return
			}
		}
		before, _ := h.Store.GetSettings(r.Context(), o.ID)
		st, err := h.Store.PutSettings(r.Context(), o.ID, in)
		if err != nil {
			jerr(w, http.StatusBadRequest, err.Error())
			return
		}
		h.audit(r, "settings.change", map[string]any{"resource_type": "org_settings", "resource_id": o.ID, "before": before, "after": st})
		writeJSON(w, http.StatusOK, st)
	case strings.HasPrefix(path, PathDomains+"/"):
		rest := strings.TrimPrefix(path, PathDomains+"/")
		id, action, _ := strings.Cut(rest, "/")
		h.one(w, r, o.ID, id, action)
	default:
		jerr(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) one(w http.ResponseWriter, r *http.Request, orgID, id, action string) {
	switch {
	case action == "verify" && r.Method == http.MethodPost:
		d, ok, err := h.Store.Verify(r.Context(), orgID, id)
		if err != nil {
			h.fail(w, r, "verify", id, err)
			return
		}
		ev := "verify"
		if !ok {
			ev = "fail"
		}
		h.audit(r, ev, map[string]any{"resource_type": "domain", "resource_id": d.ID, "domain": d.Domain, "method": "txt",
			"resolver": "public", "status_after": d.Status})
		writeJSON(w, http.StatusOK, map[string]any{"verified": ok, "domain": d})
	case action == "" && r.Method == http.MethodPatch:
		var p Patch
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&p) != nil {
			jerr(w, http.StatusBadRequest, "invalid body")
			return
		}
		d, err := h.Store.Update(r.Context(), orgID, id, p)
		if err != nil {
			h.fail(w, r, "update", id, err)
			return
		}
		ev := "update"
		if p.Primary != nil && *p.Primary {
			ev = "set_primary"
		}
		h.audit(r, ev, map[string]any{"resource_type": "domain", "resource_id": d.ID, "domain": d.Domain})
		writeJSON(w, http.StatusOK, d)
	case action == "" && r.Method == http.MethodDelete:
		d, _ := h.Store.Get(r.Context(), orgID, id)
		if err := h.Store.Remove(r.Context(), orgID, id); err != nil {
			h.fail(w, r, "remove", id, err)
			return
		}
		h.audit(r, "remove", map[string]any{"resource_type": "domain", "resource_id": id, "domain": d.Domain})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		jerr(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, op, what string, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrInvalidDomain), errors.Is(err, ErrFreeMail):
		code = http.StatusBadRequest
	case errors.Is(err, ErrClaimed), errors.Is(err, ErrPrimary), errors.Is(err, ErrInUse), errors.Is(err, ErrNotVerified):
		code = http.StatusConflict
	case errors.Is(err, ErrNotFound):
		code = http.StatusNotFound
	}
	if code != http.StatusInternalServerError {
		h.audit(r, op+".denied", map[string]any{"domain": what, "reason": err.Error(), "status": "error"})
		jerr(w, code, err.Error())
		return
	}
	jerr(w, code, "internal error")
}

// hint answers whether an email's domain is claimed (login routing). It never
// returns org data and is rate-limited per client IP (30 per minute).
func (h *Handler) hint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jerr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ip := clientIP(r)
	now := time.Now()
	h.mu.Lock()
	if h.hits == nil {
		h.hits = map[string][]time.Time{}
	}
	keep := h.hits[ip][:0]
	for _, t := range h.hits[ip] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 30 {
		h.hits[ip] = keep
		h.mu.Unlock()
		jerr(w, http.StatusTooManyRequests, "slow down")
		return
	}
	h.hits[ip] = append(keep, now)
	h.mu.Unlock()
	dom := EmailDomain(r.URL.Query().Get("email"))
	writeJSON(w, http.StatusOK, map[string]any{"claimed": dom != "" && h.Store.DomainClaimed(r.Context(), dom)})
}
