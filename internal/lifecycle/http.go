package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Permissions (Owner/Admin hold both through "*").
const (
	PermUserWrite   = "identity:user:write"
	PermInviteWrite = "identity:invite:write"
)

// Paths.
const (
	PathInvitations = "/api/v1/admin/invitations"
	PathUsers       = "/api/v1/admin/identity/users"
	PathRunExpiry   = "/api/v1/admin/identity/run-expiry"
	PathLookup      = "/api/v1/invitations/lookup"
	PathAccept      = "/api/v1/invitations/accept"
)

// InviteSender delivers an invitation e-mail (adminusers.InviteSender shape).
type InviteSender func(ctx context.Context, to, name, inviteURL, orgName string) error

// Handler serves the lifecycle admin APIs and the invitation accept flow.
type Handler struct {
	Store  *Store
	Policy *Policy
	Audit  AuditFn
	Send   InviteSender // optional; nil means link-copy only
	Notify func(ctx context.Context, to, subject, body string)
	// InviteCheck is the external collaboration policy: it refuses invitations to
	// (and acceptances by) blocked partner domains. nil = allow. The error text
	// is shown to the admin.
	InviteCheck func(ctx context.Context, orgID, email, userType string) error
	// GuestInviteOpen lets people without the invite permission create guest
	// invitations when the policy level (members, everyone, sponsors) allows it.
	// Such callers can only invite guests, sponsored by themselves. nil = off.
	GuestInviteOpen func(ctx context.Context) bool

	mu   sync.Mutex
	hits map[string][]time.Time
}

// Match reports whether path belongs to this handler.
func Match(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == PathInvitations || strings.HasPrefix(p, PathInvitations+"/") ||
		p == PathUsers || strings.HasPrefix(p, PathUsers+"/") || p == PathRunExpiry || p == PathLookup || p == PathAccept
}

// IsPublic reports whether path may be served without a session.
func IsPublic(path string) bool { return strings.TrimRight(path, "/") == PathLookup }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func jerr(w http.ResponseWriter, code int, msg string) { writeJSON(w, code, map[string]any{"error": msg}) }

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	return r.RemoteAddr
}

func (h *Handler) audit(r *http.Request, action string, detail map[string]any) {
	if h.Audit != nil {
		if detail == nil {
			detail = map[string]any{}
		}
		detail["ip"] = clientIP(r)
		h.Audit(r.Context(), "lifecycle", action, detail)
	}
}

func baseURL(r *http.Request) string {
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
	return proto + "://" + host
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimRight(r.URL.Path, "/")
	if path == PathLookup {
		h.lookup(w, r)
		return
	}
	u, hasUser := auth.UserFromContext(r.Context())
	o, hasOrg := auth.OrgFromContext(r.Context())
	if !hasUser || !hasOrg {
		jerr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if path == PathAccept {
		h.accept(w, r, u.ID, u.Email, o.ID)
		return
	}
	perm := PermUserWrite
	if strings.HasPrefix(path, PathInvitations) {
		perm = PermInviteWrite
	}
	limited := false // caller may only invite guests, sponsored by themself
	if !authz.Can(r.Context(), perm) {
		if path == PathInvitations && r.Method == http.MethodPost && h.GuestInviteOpen != nil && h.GuestInviteOpen(r.Context()) {
			limited = true
		} else {
			authz.WriteForbidden(w, perm)
			return
		}
	}
	switch {
	case path == PathInvitations && r.Method == http.MethodGet:
		list, err := h.Store.ListInvitations(r.Context(), o.ID)
		if err != nil {
			jerr(w, http.StatusInternalServerError, "list failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"invitations": list})
	case path == PathInvitations && r.Method == http.MethodPost:
		var in InviteInput
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in) != nil {
			jerr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if limited {
			if in.UserType != "" && in.UserType != "guest" {
				jerr(w, http.StatusForbidden, "You may invite guests only.")
				return
			}
			in.UserType, in.SponsorID = "guest", u.ID
		}
		if h.InviteCheck != nil {
			ut := in.UserType
			if ut == "" {
				ut = "guest"
			}
			if cerr := h.InviteCheck(r.Context(), o.ID, in.Email, ut); cerr != nil {
				h.audit(r, "invite.denied", map[string]any{"email": in.Email, "reason": cerr.Error(), "status": "error"})
				jerr(w, http.StatusForbidden, cerr.Error())
				return
			}
		}
		inv, secret, err := h.Store.CreateInvitation(r.Context(), o.ID, u.ID, in)
		if err != nil {
			h.fail(w, r, "invite.denied", in.Email, err)
			return
		}
		h.respondInvite(w, r, http.StatusCreated, inv, secret, o.DisplayName, "user.invite")
	case strings.HasPrefix(path, PathInvitations+"/"):
		id, action, _ := strings.Cut(strings.TrimPrefix(path, PathInvitations+"/"), "/")
		switch {
		case action == "resend" && r.Method == http.MethodPost:
			inv, secret, err := h.Store.ResendInvitation(r.Context(), o.ID, id)
			if err != nil {
				h.fail(w, r, "invite.resend.denied", id, err)
				return
			}
			h.respondInvite(w, r, http.StatusOK, inv, secret, o.DisplayName, "user.invite.resend")
		case action == "" && r.Method == http.MethodDelete:
			if err := h.Store.RevokeInvitation(r.Context(), o.ID, id); err != nil {
				h.fail(w, r, "invite.revoke.denied", id, err)
				return
			}
			h.audit(r, "user.invite.revoke", map[string]any{"resource_type": "invitation", "resource_id": id})
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			jerr(w, http.StatusNotFound, "not found")
		}
	case path == PathUsers && r.Method == http.MethodGet:
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		rows, err := h.Store.ListUsers(r.Context(), o.ID, UserFilter{Type: q.Get("type"), Status: q.Get("status"), Q: q.Get("q"), Limit: limit + 1, Offset: (page - 1) * limit})
		next := ""
		if err == nil && len(rows) > limit {
			rows, next = rows[:limit], strconv.Itoa(page+1)
		}
		if err != nil {
			jerr(w, http.StatusInternalServerError, "list failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": rows, "items": rows, "next_cursor": next})
	case path == PathRunExpiry && r.Method == http.MethodPost:
		n, err := h.Policy.RunExpiry(r.Context(), h.Notify)
		if err != nil {
			jerr(w, http.StatusInternalServerError, "expiry run failed")
			return
		}
		h.audit(r, "expiry.run", map[string]any{"blocked": n})
		writeJSON(w, http.StatusOK, map[string]any{"blocked": n})
	case strings.HasPrefix(path, PathUsers+"/"):
		id, action, _ := strings.Cut(strings.TrimPrefix(path, PathUsers+"/"), "/")
		h.userOp(w, r, o.ID, u.ID, id, action)
	default:
		jerr(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) respondInvite(w http.ResponseWriter, r *http.Request, code int, inv Invitation, secret, orgName, event string) {
	link := baseURL(r) + "/accept-invite?code=" + url.QueryEscape(secret)
	sent := false
	if h.Send != nil {
		sent = h.Send(r.Context(), inv.Email, "", link, orgName) == nil
	}
	h.audit(r, event, map[string]any{"resource_type": "invitation", "resource_id": inv.ID, "email": inv.Email,
		"user_type": inv.UserType, "sponsor": inv.SponsorEmail, "email_sent": sent})
	writeJSON(w, code, map[string]any{"invitation": inv, "accept_url": link, "email_sent": sent})
}

func (h *Handler) userOp(w http.ResponseWriter, r *http.Request, orgID, actorID, id, action string) {
	switch {
	case action == "block" && r.Method == http.MethodPost:
		if id == actorID {
			jerr(w, http.StatusConflict, "you cannot block yourself")
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in)
		if in.Reason == "" {
			in.Reason = "blocked by admin"
		}
		email, err := h.Store.Block(r.Context(), orgID, id, in.Reason)
		if err != nil {
			h.fail(w, r, "user.block.denied", id, err)
			return
		}
		h.audit(r, "user.block", map[string]any{"resource_type": "user", "resource_id": id, "email": email, "reason": in.Reason})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case action == "unblock" && r.Method == http.MethodPost:
		email, err := h.Store.Unblock(r.Context(), orgID, id)
		if err != nil {
			h.fail(w, r, "user.unblock.denied", id, err)
			return
		}
		h.audit(r, "user.unblock", map[string]any{"resource_type": "user", "resource_id": id, "email": email})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case action == "" && r.Method == http.MethodPatch:
		var p UserPatch
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&p) != nil {
			jerr(w, http.StatusBadRequest, "invalid body")
			return
		}
		before, after, err := h.Store.UpdateUser(r.Context(), orgID, id, p)
		if err != nil {
			h.fail(w, r, "user.update.denied", id, err)
			return
		}
		h.audit(r, "user.type_change", map[string]any{"resource_type": "user", "resource_id": id, "before": before, "after": after,
			"expires_at": p.ExpiresAt, "clear_expiry": p.ClearExpiry})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user_type": after})
	default:
		jerr(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, event, what string, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrInvalid):
		code = http.StatusBadRequest
	case errors.Is(err, ErrExists):
		code = http.StatusConflict
	case errors.Is(err, ErrNotFound):
		code = http.StatusNotFound
	}
	if code == http.StatusInternalServerError {
		jerr(w, code, "internal error")
		return
	}
	h.audit(r, event, map[string]any{"target": what, "reason": err.Error(), "status": "error"})
	jerr(w, code, err.Error())
}

// lookup returns public details of a valid invitation (no auth). Rate-limited.
func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jerr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ip, now := clientIP(r), time.Now()
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
	over := len(keep) >= 20
	if !over {
		keep = append(keep, now)
	}
	h.hits[ip] = keep
	h.mu.Unlock()
	if over {
		jerr(w, http.StatusTooManyRequests, "slow down")
		return
	}
	inv, org, err := h.Store.LookupPublic(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		jerr(w, http.StatusNotFound, ErrInviteGone.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"org": org, "email": inv.Email, "user_type": inv.UserType,
		"apps": inv.AppIDs, "expires_at": inv.ExpiresAt})
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request, userID, email, orgID string) {
	if r.Method != http.MethodPost {
		jerr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || in.Code == "" {
		jerr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if h.InviteCheck != nil {
		if cerr := h.InviteCheck(r.Context(), orgID, email, ""); cerr != nil {
			h.audit(r, "user.accept.denied", map[string]any{"reason": cerr.Error(), "status": "error"})
			jerr(w, http.StatusForbidden, cerr.Error())
			return
		}
	}
	inv, err := h.Store.Accept(r.Context(), in.Code, userID, email, orgID)
	if err != nil {
		code := http.StatusGone
		switch {
		case errors.Is(err, ErrEmailMismatch), errors.Is(err, ErrOtherOrg):
			code = http.StatusForbidden
		case !errors.Is(err, ErrInviteGone):
			code = http.StatusInternalServerError
		}
		if code != http.StatusInternalServerError {
			if h.Audit != nil {
				h.Audit(r.Context(), "lifecycle", "user.accept.denied", map[string]any{"reason": err.Error(), "status": "error", "ip": clientIP(r)})
			}
			jerr(w, code, err.Error())
			return
		}
		jerr(w, code, "internal error")
		return
	}
	h.audit(r, "user.accept", map[string]any{"resource_type": "invitation", "resource_id": inv.ID, "user_type": inv.UserType})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user_type": inv.UserType})
}
