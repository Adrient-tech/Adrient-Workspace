package chat

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

// Calls: ringing colleagues into a Meet room over the per-user live feed.
//
// The live feed (Hub.ServeUser) is opened by every signed-in tab, not only
// the Chat app, so a ring reaches the user wherever they are in the
// workspace. The Meet room itself is created by the caller beforehand
// (POST /api/v1/meet/codes); this endpoint only signals.

var meetCodeRe = regexp.MustCompile(`^[a-z]{3}-[a-z]{4}-[a-z]{3}$`)

// CallNotifier records a missed/incoming call in the notification center.
// Optional; nil disables it.
type CallNotifier func(ctx context.Context, orgID, userID, actorID, title, body, url string)

// SetCallNotifier installs the notification hook used for incoming calls.
func (h *TeamsHandler) SetCallNotifier(n CallNotifier) { h.callNotify = n }

type ringReq struct {
	UserIDs []string `json:"user_ids"`
	Code    string   `json:"code"`
	Title   string   `json:"title"`
	Video   bool     `json:"video"`
}

type respondReq struct {
	ToUserID string `json:"to_user_id"`
	Code     string `json:"code"`
	Answer   string `json:"answer"`
}

func (h *TeamsHandler) serveCall(w http.ResponseWriter, r *http.Request, c apiCtx, action string) {
	ctx := r.Context()
	switch action {
	case "ring":
		var req ringReq
		if err := decode(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		ids, err := h.callTargets(ctx, c, req.UserIDs, req.Code)
		if err != nil {
			writeErr(w, err)
			return
		}
		title := strings.TrimSpace(req.Title)
		if len(title) > 120 {
			title = title[:120]
		}
		if title == "" {
			title = "Call"
		}
		if h.hub != nil {
			h.hub.NotifyUsers(ids, feedEnvelope{
				"type": "call_ring", "code": req.Code, "title": title, "video": req.Video,
				"from_id": c.uid, "from_name": c.uname,
			})
		}
		if h.callNotify != nil {
			kind := "Voice call"
			if req.Video {
				kind = "Video call"
			}
			for _, id := range ids {
				h.callNotify(ctx, c.orgID, id, c.uid, fmt.Sprintf("%s from %s", kind, c.uname), title, "/meet/"+req.Code)
			}
		}
		if err := h.store.LogRing(ctx, c.orgID, c.uid, req.Code, title, req.Video, ids); err != nil {
			slog.Warn("chat: log call ring", "err", err)
		}
		writeJSON(w, 200, map[string]any{"rang": ids})
	case "invite":
		// Notifies invitees of a scheduled meeting (notification center + live toast).
		var req struct {
			UserIDs []string `json:"user_ids"`
			Code    string   `json:"code"`
			Title   string   `json:"title"`
			When    string   `json:"when"`
		}
		if err := decode(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		ids, err := h.callTargets(ctx, c, req.UserIDs, req.Code)
		if err != nil {
			writeErr(w, err)
			return
		}
		title := strings.TrimSpace(req.Title)
		if len(title) > 120 {
			title = title[:120]
		}
		when := strings.TrimSpace(req.When)
		if len(when) > 80 {
			when = when[:80]
		}
		if h.callNotify != nil {
			for _, id := range ids {
				h.callNotify(ctx, c.orgID, id, c.uid, fmt.Sprintf("Meeting invite from %s", c.uname), strings.TrimSpace(title+" · "+when), "/meet/"+req.Code)
			}
		}
		writeJSON(w, 200, map[string]any{"invited": ids})
	case "respond":
		var req respondReq
		if err := decode(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		switch req.Answer {
		case "accepted", "declined", "cancelled":
		default:
			writeErr(w, fmt.Errorf("%w: answer must be accepted, declined or cancelled", ErrInvalid))
			return
		}
		ids, err := h.callTargets(ctx, c, []string{req.ToUserID}, req.Code)
		if err != nil {
			writeErr(w, err)
			return
		}
		if h.hub != nil {
			h.hub.NotifyUsers(ids, feedEnvelope{
				"type": "call_" + req.Answer, "code": req.Code, "from_id": c.uid, "from_name": c.uname,
			})
		}
		if err := h.store.LogRespond(ctx, c.orgID, c.uid, req.ToUserID, req.Code, req.Answer); err != nil {
			slog.Warn("chat: log call respond", "err", err)
		}
		writeJSON(w, 200, map[string]string{"ok": "1"})
	case "end":
		// A participant left the room; the call's end settles on the last one out.
		var req struct {
			Code string `json:"code"`
		}
		if err := decode(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		if !meetCodeRe.MatchString(req.Code) {
			writeErr(w, fmt.Errorf("%w: invalid meeting code", ErrInvalid))
			return
		}
		if err := h.store.LogEnd(ctx, c.orgID, c.uid, req.Code); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "1"})
	case "history":
		list, err := h.store.CallHistory(ctx, c.orgID, c.uid, 60)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"calls": list})
	default:
		writeErr(w, ErrNotFound)
	}
}

// callTargets validates the code and keeps only users of the caller's org
// (never the caller), capped to keep a ring from becoming a broadcast.
func (h *TeamsHandler) callTargets(ctx context.Context, c apiCtx, userIDs []string, code string) ([]string, error) {
	if !meetCodeRe.MatchString(code) {
		return nil, fmt.Errorf("%w: invalid meeting code", ErrInvalid)
	}
	if len(userIDs) == 0 {
		return nil, fmt.Errorf("%w: no one to call", ErrInvalid)
	}
	if len(userIDs) > 50 {
		return nil, fmt.Errorf("%w: at most 50 people per call", ErrInvalid)
	}
	people, err := h.store.People(ctx, c.orgID)
	if err != nil {
		return nil, err
	}
	inOrg := make(map[string]bool, len(people))
	for _, p := range people {
		inOrg[p.ID] = true
	}
	out := make([]string, 0, len(userIDs))
	seen := map[string]bool{}
	for _, id := range userIDs {
		if id == "" || id == c.uid || seen[id] || !inOrg[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no valid people to call", ErrInvalid)
	}
	return out, nil
}

// PostAs posts body into channelID as userID and pushes it to open clients.
// Used by server-side integrations (for example a finished Meet recording).
func (h *TeamsHandler) PostAs(ctx context.Context, orgID, channelID, userID, userName, body string) error {
	res, err := h.store.Post(ctx, orgID, userID, userName, channelID, "", body, nil, "")
	if err != nil {
		return err
	}
	h.broadcastPost(res)
	return nil
}
