package externalpolicy

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Paths.
const (
	PathPolicy   = "/api/v1/admin/external/policy"
	PathPartners = "/api/v1/admin/external/partners"
	PathPreview  = "/api/v1/shares/preview"
	PathTrusts   = "/api/v1/admin/external/trusts"
)

// Handler serves the external collaboration admin API and the share preview.
type Handler struct {
	Store *Store
	Gate  *Gate
	Audit func(r *http.Request, action string, detail map[string]any)
}

// Match reports whether path belongs to this handler.
func Match(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == PathPolicy || p == PathPartners || strings.HasPrefix(p, PathPartners+"/") || p == PathPreview ||
		p == PathTrusts || strings.HasPrefix(p, PathTrusts+"/")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func jerr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func (h *Handler) audit(r *http.Request, action string, d map[string]any) {
	if h.Audit != nil {
		h.Audit(r, action, d)
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	u, ok2 := auth.UserFromContext(r.Context())
	if !ok || !ok2 {
		jerr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	path := strings.TrimRight(r.URL.Path, "/")
	if path == PathPreview {
		if r.Method != http.MethodGet {
			jerr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		email := strings.TrimSpace(r.URL.Query().Get("email"))
		if email == "" {
			jerr(w, http.StatusBadRequest, "email required")
			return
		}
		writeJSON(w, http.StatusOK, h.Gate.Preview(r.Context(), email))
		return
	}
	if !authz.IsAdmin(r.Context()) {
		authz.WriteForbidden(w, authz.PermOrgAdmin)
		return
	}
	switch {
	case path == PathPolicy && r.Method == http.MethodGet:
		p, err := h.Store.Get(r.Context(), o.ID)
		if err != nil {
			jerr(w, http.StatusInternalServerError, "read failed")
			return
		}
		writeJSON(w, http.StatusOK, p)
	case path == PathPolicy && r.Method == http.MethodPut:
		var p Policy
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&p) != nil {
			jerr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := h.Store.Put(r.Context(), o.ID, p); err != nil {
			if err == ErrInvalid {
				jerr(w, http.StatusBadRequest, "invalid policy value")
				return
			}
			jerr(w, http.StatusInternalServerError, "save failed")
			return
		}
		h.audit(r, "policy.update", map[string]any{"domain_mode": p.DomainMode, "guest_invites": p.GuestInvites, "guest_visibility": p.GuestVisibility})
		writeJSON(w, http.StatusOK, p)
	case path == PathPartners && r.Method == http.MethodGet:
		l, err := h.Store.Partners(r.Context(), o.ID)
		if err != nil {
			jerr(w, http.StatusInternalServerError, "read failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"partners": l})
	case path == PathPartners && r.Method == http.MethodPost:
		var in Partner
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in) != nil {
			jerr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := h.Store.PutPartner(r.Context(), o.ID, u.ID, in); err != nil {
			if err == ErrInvalid {
				jerr(w, http.StatusBadRequest, "domain must look like partner.com and effect must be allow or block")
				return
			}
			jerr(w, http.StatusInternalServerError, "save failed")
			return
		}
		h.audit(r, "partner."+in.Effect, map[string]any{"domain": in.Domain})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case strings.HasPrefix(path, PathPartners+"/") && r.Method == http.MethodDelete:
		d := strings.TrimPrefix(path, PathPartners+"/")
		found, err := h.Store.DeletePartner(r.Context(), o.ID, d)
		if err != nil || !found {
			jerr(w, http.StatusNotFound, "partner not found")
			return
		}
		h.audit(r, "partner.remove", map[string]any{"domain": d})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case path == PathTrusts && r.Method == http.MethodGet:
		l, err := h.Store.ListTrusts(r.Context(), o.ID)
		if err != nil {
			jerr(w, http.StatusInternalServerError, "read failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"trusts": l, "actions": TrustActions})
	case path == PathTrusts && r.Method == http.MethodPost:
		var in struct {
			Partner  string          `json:"partner"`
			Outbound map[string]bool `json:"outbound"`
			Inbound  map[string]bool `json:"inbound"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in) != nil || strings.TrimSpace(in.Partner) == "" {
			jerr(w, http.StatusBadRequest, "partner (organization slug or verified domain) is required")
			return
		}
		t, err := h.Store.ProposeTrust(r.Context(), o.ID, in.Partner, in.Outbound, in.Inbound)
		switch err {
		case nil:
		case ErrTrustNotFound:
			jerr(w, http.StatusNotFound, "No organization with that slug or verified domain was found.")
			return
		case ErrTrustSelf:
			jerr(w, http.StatusBadRequest, err.Error())
			return
		default:
			jerr(w, http.StatusInternalServerError, "save failed")
			return
		}
		h.audit(r, "trust.propose", map[string]any{"partner_org_id": t.PartnerOrgID})
		writeJSON(w, http.StatusCreated, t)
	case strings.HasPrefix(path, PathTrusts+"/") && r.Method == http.MethodPost && strings.HasSuffix(path, "/accept"):
		pid := strings.TrimSuffix(strings.TrimPrefix(path, PathTrusts+"/"), "/accept")
		var in struct {
			Outbound map[string]bool `json:"outbound"`
			Inbound  map[string]bool `json:"inbound"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in)
		if err := h.Store.AcceptTrust(r.Context(), o.ID, pid, in.Outbound, in.Inbound); err != nil {
			if err == ErrNoTrust {
				jerr(w, http.StatusNotFound, "no pending trust from that organization")
				return
			}
			jerr(w, http.StatusInternalServerError, "save failed")
			return
		}
		h.audit(r, "trust.accept", map[string]any{"partner_org_id": pid})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case strings.HasPrefix(path, PathTrusts+"/") && r.Method == http.MethodDelete:
		pid := strings.TrimPrefix(path, PathTrusts+"/")
		if err := h.Store.RevokeTrust(r.Context(), o.ID, pid); err != nil {
			jerr(w, http.StatusNotFound, "trust not found")
			return
		}
		h.audit(r, "trust.revoke", map[string]any{"partner_org_id": pid})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		jerr(w, http.StatusNotFound, "not found")
	}
}
