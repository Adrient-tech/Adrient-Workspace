package offboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Base path of the lifecycle admin API.
const Base = "/api/v1/admin/lifecycle"

// Handler serves the lifecycle admin API.
type Handler struct{ Svc *Service }

// Match reports whether path belongs to this handler.
func Match(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == Base || strings.HasPrefix(p, Base+"/")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	msg := "request failed"
	var he *HoldError
	switch {
	case errors.As(err, &he):
		code, msg = http.StatusConflict, he.Error()
	case errors.Is(err, ErrNotFound):
		code, msg = http.StatusNotFound, err.Error()
	case errors.Is(err, ErrBadRequest), errors.Is(err, ErrNoSeats):
		code, msg = http.StatusBadRequest, err.Error()
	case err != nil && strings.Contains(err.Error(), "no seat"):
		code, msg = http.StatusConflict, err.Error()
	}
	writeJSON(w, code, map[string]any{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	u, ok2 := auth.UserFromContext(r.Context())
	if !ok || !ok2 {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	write := r.Method != http.MethodGet
	perm := "admin:lifecycle:read"
	if write {
		perm = "admin:lifecycle:write"
	}
	if !authz.IsAdmin(r.Context()) && !authz.Can(r.Context(), perm) {
		authz.WriteForbidden(w, perm)
		return
	}
	ctx := r.Context()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(strings.TrimRight(r.URL.Path, "/"), Base), "/"), "/")
	switch {
	case seg[0] == "options" && r.Method == http.MethodGet:
		opts := []map[string]any{}
		labels := map[string][]string{}
		for _, t := range transferable {
			labels[t.Group] = append(labels[t.Group], t.Label)
		}
		for _, g := range Groups {
			opts = append(opts, map[string]any{"id": g, "includes": labels[g]})
		}
		writeJSON(w, http.StatusOK, map[string]any{"transfer_groups": opts, "restore_window_days": h.Svc.RestoreWindow(ctx, o.ID),
			"alert_events": AlertEvents})
	case seg[0] == "users" && len(seg) == 1 && r.Method == http.MethodGet:
		h.list(w, r, o.ID)
	case seg[0] == "users" && len(seg) == 3 && r.Method == http.MethodPost:
		h.action(w, r, o.ID, u.ID, seg[1], seg[2])
	case seg[0] == "holds" && len(seg) == 1 && r.Method == http.MethodGet:
		l, err := h.Svc.ListHolds(ctx, o.ID)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"holds": l})
	case seg[0] == "holds" && len(seg) == 1 && r.Method == http.MethodPost:
		var in Hold
		if decode(w, r, &in) != nil {
			fail(w, ErrBadRequest)
			return
		}
		hold, err := h.Svc.CreateHold(ctx, o.ID, u.ID, in)
		if err != nil {
			fail(w, err)
			return
		}
		h.Svc.alert(ctx, o.ID, "hold.create", "Legal hold created: "+hold.Name, "A legal hold was created by "+u.Email+".")
		writeJSON(w, http.StatusCreated, hold)
	case seg[0] == "holds" && len(seg) == 2 && r.Method == http.MethodDelete:
		if err := h.Svc.ReleaseHold(ctx, o.ID, u.ID, seg[1]); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case seg[0] == "retention" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, h.Svc.GetRetention(ctx, o.ID))
	case seg[0] == "retention" && r.Method == http.MethodPut:
		var in Retention
		if decode(w, r, &in) != nil {
			fail(w, ErrBadRequest)
			return
		}
		if err := h.Svc.PutRetention(ctx, o.ID, u.ID, in); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, in)
	case seg[0] == "alerts" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, h.Svc.GetAlerts(ctx, o.ID))
	case seg[0] == "alerts" && r.Method == http.MethodPut:
		var in AlertSettings
		if decode(w, r, &in) != nil {
			fail(w, ErrBadRequest)
			return
		}
		if err := h.Svc.PutAlerts(ctx, o.ID, in); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, in)
	case seg[0] == "history" && r.Method == http.MethodGet:
		l, err := h.Svc.History(ctx, o.ID, 100)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"history": l})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, orgID string) {
	state := r.URL.Query().Get("state")
	cond := "u.deleted_at IS NOT NULL"
	switch state {
	case "archived":
		cond = "u.archived_at IS NOT NULL AND u.deleted_at IS NULL"
	case "suspended":
		cond = "u.status='blocked' AND u.archived_at IS NULL AND u.deleted_at IS NULL"
	case "deleted", "":
	default:
		fail(w, ErrBadRequest)
		return
	}
	rows, err := h.Svc.Pool.Query(r.Context(), `SELECT u.id::text, u.email, u.display_name, u.status, u.blocked_reason, u.deleted_at, u.purge_after, u.archived_at,
EXISTS(SELECT 1 FROM adrient.legal_holds h WHERE h.org_id=u.org_id AND h.released_at IS NULL AND (h.org_wide OR u.id = ANY(h.user_ids)))
FROM adrient.users u WHERE u.org_id::text=$1 AND `+cond+` ORDER BY u.updated_at DESC LIMIT 200`, orgID)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type row struct {
		ID, Email, Name, Status, Reason string
		DeletedAt, PurgeAfter, Archived *time.Time
		OnHold                          bool
	}
	out := []map[string]any{}
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.Email, &x.Name, &x.Status, &x.Reason, &x.DeletedAt, &x.PurgeAfter, &x.Archived, &x.OnHold); err != nil {
			fail(w, err)
			return
		}
		out = append(out, map[string]any{"id": x.ID, "email": x.Email, "name": x.Name, "status": x.Status, "reason": x.Reason,
			"deleted_at": x.DeletedAt, "purge_after": x.PurgeAfter, "archived_at": x.Archived, "on_hold": x.OnHold})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (h *Handler) action(w http.ResponseWriter, r *http.Request, orgID, actor, userID, act string) {
	ctx := r.Context()
	var in struct {
		Reason      string   `json:"reason"`
		Note        string   `json:"note"`
		TransferTo  string   `json:"transfer_to"`
		Transfer    []string `json:"transfer"`
		ForwardMail bool     `json:"forward_mail"`
		LeaveChats  bool     `json:"leave_chats"`
	}
	_ = decode(w, r, &in)
	var err error
	switch act {
	case "suspend":
		err = h.Svc.Suspend(ctx, orgID, userID, actor, strings.TrimSpace(in.Reason))
	case "unsuspend":
		err = h.Svc.Unsuspend(ctx, orgID, userID, actor)
	case "archive":
		err = h.Svc.Archive(ctx, orgID, userID, actor, in.Note)
	case "restore":
		err = h.Svc.Restore(ctx, orgID, userID, actor)
	case "delete":
		res, derr := h.Svc.Delete(ctx, DeleteInput{OrgID: orgID, UserID: userID, Actor: actor, TransferTo: in.TransferTo,
			Transfer: in.Transfer, ForwardMail: in.ForwardMail, LeaveChats: in.LeaveChats, Note: in.Note})
		if derr != nil {
			fail(w, derr)
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown action"})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": act})
}
