package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// MountPrefix is the URL prefix of the Teams REST API (and the live feed at
// MountPrefix+"live").
const MountPrefix = "/api/v1/chat/x/"

// TeamsHandler serves the Teams-class chat REST API under MountPrefix.
type TeamsHandler struct {
	store      *Store
	hub        *Hub
	callNotify CallNotifier
	gate       MemberGate
}

// WithGate installs the external-chat member gate. Returns h for chaining.
func (h *TeamsHandler) WithGate(g MemberGate) *TeamsHandler { h.gate = g; return h }

// gateMembers writes a 403 and returns false when the gate refuses the members.
func (h *TeamsHandler) gateMembers(w http.ResponseWriter, r *http.Request, ids []string) bool {
	if h.gate == nil {
		return true
	}
	if err := h.gate.CheckChatMembers(r.Context(), ids); err != nil {
		if d, ok := authz.AsDenied(err); ok {
			authz.WriteControlDenied(w, d)
		} else {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		}
		return false
	}
	return true
}

// NewTeamsHandler builds the handler.
func NewTeamsHandler(store *Store, hub *Hub) *TeamsHandler {
	return &TeamsHandler{store: store, hub: hub}
}

type apiCtx struct {
	uid, uname, orgID string
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	msg := "internal error"
	switch {
	case errors.Is(err, ErrNotFound):
		code, msg = http.StatusNotFound, "not found"
	case errors.Is(err, ErrForbidden):
		code, msg = http.StatusForbidden, "not a member of this channel"
	case errors.Is(err, ErrConflict):
		code, msg = http.StatusConflict, "name already in use"
	case errors.Is(err, ErrInvalid):
		code, msg = http.StatusBadRequest, strings.TrimPrefix(err.Error(), "invalid: ")
	default:
		log.Printf("chat teams: %v", err)
	}
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%w: bad json", ErrInvalid)
	}
	return nil
}

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%w: bad time %q", ErrInvalid, s)
}

func (h *TeamsHandler) caller(r *http.Request) (apiCtx, bool) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		return apiCtx{}, false
	}
	o, ok := auth.OrgFromContext(r.Context())
	if !ok {
		return apiCtx{}, false
	}
	name := u.DisplayName
	if name == "" {
		name = u.Email
	}
	return apiCtx{uid: u.ID, uname: name, orgID: o.ID}, true
}

// memberChannel loads a channel and requires membership.
func (h *TeamsHandler) memberChannel(ctx context.Context, c apiCtx, id string) (TChannel, error) {
	ch, err := h.store.GetChannel(ctx, c.orgID, c.uid, id)
	if err != nil {
		return ch, err
	}
	if !ch.IsMember {
		return ch, ErrForbidden
	}
	return ch, nil
}

// visibleChannel allows members and, for public channels, non-members (read-only preview).
func (h *TeamsHandler) visibleChannel(ctx context.Context, c apiCtx, id string) (TChannel, error) {
	ch, err := h.store.GetChannel(ctx, c.orgID, c.uid, id)
	if err != nil {
		return ch, err
	}
	if !ch.IsMember && ch.Kind != "public" {
		return ch, ErrForbidden
	}
	return ch, nil
}

func (h *TeamsHandler) emit(ids []string, env feedEnvelope) {
	if h.hub != nil {
		h.hub.NotifyUsers(ids, env)
	}
}

// Broadcast pushes a freshly posted message to every member's live feed.
func (h *TeamsHandler) broadcastPost(res PostResult) {
	m := res.Message
	m.Saved = false
	h.emit(res.Channel.MemberIDs, feedEnvelope{"type": "message", "channel_id": res.Channel.ID, "message": m, "notify": res.Notify})
}

// ServeHTTP routes the Teams API.
func (h *TeamsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, MountPrefix), "/")
	seg := strings.Split(rest, "/")
	ctx := r.Context()
	m := r.Method
	n := len(seg)

	fail := func(err error) { writeErr(w, err) }

	switch seg[0] {
	case "calls":
		// POST calls/ring    {user_ids, code, title, video}  ring colleagues
		// POST calls/respond {to_user_id, code, answer}       accepted|declined|cancelled
		// GET  calls/history                                  the caller's call log
		// POST calls/end     {code}                           a participant left the room
		if n != 2 || (m != http.MethodPost && !(m == http.MethodGet && seg[1] == "history")) {
			break
		}
		h.serveCall(w, r, c, seg[1])
		return
	case "bootstrap":
		if m != http.MethodGet {
			break
		}
		chs, err := h.store.ListChannels(ctx, c.orgID, c.uid)
		if err != nil {
			fail(err)
			return
		}
		secs, err := h.store.ListSections(ctx, c.uid)
		if err != nil {
			fail(err)
			return
		}
		d, sch, sv := h.store.Counts(ctx, c.uid)
		online, away := []string{}, []string{}
		if h.hub != nil {
			online, away = h.hub.PresenceOf(c.orgID)
		}
		writeJSON(w, 200, map[string]any{
			"channels": chs, "sections": secs, "me": c.uid, "me_name": c.uname,
			"activity_unread": h.store.ActivityUnread(ctx, c.orgID, c.uid),
			"drafts":          d, "scheduled": sch, "saved": sv, "online": online, "away": away,
		})
		return
	case "people":
		if m == http.MethodGet {
			ps, err := h.store.People(ctx, c.orgID)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"people": ps})
			return
		}
	case "profile":
		if m == http.MethodPut {
			var in struct{ Title, StatusText, StatusEmoji string }
			var raw struct {
				Title       string `json:"title"`
				StatusText  string `json:"status_text"`
				StatusEmoji string `json:"status_emoji"`
			}
			if err := decode(r, &raw); err != nil {
				fail(err)
				return
			}
			in.Title, in.StatusText, in.StatusEmoji = raw.Title, raw.StatusText, raw.StatusEmoji
			if len(in.Title) > 80 || len(in.StatusText) > 100 || len(in.StatusEmoji) > 16 {
				fail(ErrInvalid)
				return
			}
			if err := h.store.SetProfile(ctx, c.uid, in.Title, in.StatusText, in.StatusEmoji); err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
	case "channels":
		h.channels(w, r, c, seg)
		return
	case "sections":
		h.sections(w, r, c, seg)
		return
	case "messages":
		h.messages(w, r, c, seg)
		return
	case "saved":
		if m == http.MethodGet {
			ms, err := h.store.ListSaved(ctx, c.orgID, c.uid)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"messages": ms})
			return
		}
	case "sent":
		if m == http.MethodGet {
			ms, err := h.store.ListSent(ctx, c.orgID, c.uid)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"messages": ms})
			return
		}
	case "activity":
		if m == http.MethodGet {
			items, err := h.store.Activity(ctx, c.orgID, c.uid)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"items": items})
			return
		}
		if m == http.MethodPost && n == 2 && seg[1] == "seen" {
			if err := h.store.MarkActivitySeen(ctx, c.uid); err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
	case "search":
		if m == http.MethodGet {
			q := r.URL.Query()
			var from, to *time.Time
			if v := q.Get("from"); v != "" {
				t, err := parseTime(v)
				if err != nil {
					fail(err)
					return
				}
				from = &t
			}
			if v := q.Get("to"); v != "" {
				t, err := parseTime(v)
				if err != nil {
					fail(err)
					return
				}
				t = t.Add(24 * time.Hour)
				to = &t
			}
			ms, err := h.store.Search(ctx, c.orgID, c.uid, q.Get("q"), q.Get("channel_id"), q.Get("sender_id"), from, to)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"messages": ms})
			return
		}
	case "drafts":
		switch {
		case m == http.MethodGet && n == 1:
			ds, err := h.store.ListDrafts(ctx, c.uid)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"drafts": ds})
			return
		case m == http.MethodPut && n == 1:
			var in struct {
				ChannelID string `json:"channel_id"`
				ParentID  string `json:"parent_id"`
				Body      string `json:"body"`
			}
			if err := decode(r, &in); err != nil {
				fail(err)
				return
			}
			if _, err := h.memberChannel(ctx, c, in.ChannelID); err != nil {
				fail(err)
				return
			}
			if err := h.store.SaveDraft(ctx, c.uid, in.ChannelID, in.ParentID, in.Body); err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		case m == http.MethodDelete && n == 2:
			if err := h.store.DeleteDraft(ctx, c.uid, seg[1]); err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
	case "scheduled":
		switch {
		case m == http.MethodGet && n == 1:
			xs, err := h.store.ListScheduled(ctx, c.uid)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"scheduled": xs})
			return
		case m == http.MethodPost && n == 1:
			var in struct {
				ChannelID string `json:"channel_id"`
				ParentID  string `json:"parent_id"`
				Body      string `json:"body"`
				SendAt    string `json:"send_at"`
			}
			if err := decode(r, &in); err != nil {
				fail(err)
				return
			}
			if _, err := h.memberChannel(ctx, c, in.ChannelID); err != nil {
				fail(err)
				return
			}
			at, err := parseTime(in.SendAt)
			if err != nil {
				fail(err)
				return
			}
			x, err := h.store.Schedule(ctx, c.orgID, c.uid, in.ChannelID, in.ParentID, in.Body, at)
			if err != nil {
				fail(err)
				return
			}
			_ = h.store.SaveDraft(ctx, c.uid, in.ChannelID, in.ParentID, "")
			writeJSON(w, 200, x)
			return
		case m == http.MethodDelete && n == 2:
			if err := h.store.CancelScheduled(ctx, c.uid, seg[1]); err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
	case "reminders":
		switch {
		case m == http.MethodGet && n == 1:
			rs, err := h.store.ListReminders(ctx, c.orgID, c.uid)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"reminders": rs})
			return
		case m == http.MethodDelete && n == 2:
			if err := h.store.DeleteReminder(ctx, c.uid, seg[1]); err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
	case "bookmarks":
		if m == http.MethodDelete && n == 3 {
			// DELETE /bookmarks/{channel_id}/{id}
			if _, err := h.memberChannel(ctx, c, seg[1]); err != nil {
				fail(err)
				return
			}
			if err := h.store.DeleteBookmark(ctx, seg[1], seg[2]); err != nil {
				fail(err)
				return
			}
			h.emit([]string{c.uid}, feedEnvelope{"type": "pins", "channel_id": seg[1]})
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such route"})
}

func (h *TeamsHandler) sections(w http.ResponseWriter, r *http.Request, c apiCtx, seg []string) {
	ctx := r.Context()
	switch {
	case r.Method == http.MethodPost && len(seg) == 1:
		var in struct {
			Name string `json:"name"`
		}
		if err := decode(r, &in); err != nil {
			writeErr(w, err)
			return
		}
		s, err := h.store.CreateSection(ctx, c.orgID, c.uid, in.Name)
		if err != nil {
			writeErr(w, err)
			return
		}
		h.emit([]string{c.uid}, feedEnvelope{"type": "sections"})
		writeJSON(w, 200, s)
		return
	case r.Method == http.MethodPatch && len(seg) == 2:
		var in struct {
			Name      *string `json:"name"`
			Collapsed *bool   `json:"collapsed"`
		}
		if err := decode(r, &in); err != nil {
			writeErr(w, err)
			return
		}
		if err := h.store.UpdateSection(ctx, c.uid, seg[1], in.Name, in.Collapsed); err != nil {
			writeErr(w, err)
			return
		}
		h.emit([]string{c.uid}, feedEnvelope{"type": "sections"})
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	case r.Method == http.MethodDelete && len(seg) == 2:
		if err := h.store.DeleteSection(ctx, c.uid, seg[1]); err != nil {
			writeErr(w, err)
			return
		}
		h.emit([]string{c.uid}, feedEnvelope{"type": "sections"})
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such route"})
}

func (h *TeamsHandler) channels(w http.ResponseWriter, r *http.Request, c apiCtx, seg []string) {
	ctx := r.Context()
	n := len(seg)
	m := r.Method
	fail := func(err error) { writeErr(w, err) }
	ok := func() { writeJSON(w, 200, map[string]bool{"ok": true}) }

	switch {
	case n == 1 && m == http.MethodPost:
		var in struct {
			Kind        string   `json:"kind"`
			Name        string   `json:"name"`
			Topic       string   `json:"topic"`
			Description string   `json:"description"`
			MemberIDs   []string `json:"member_ids"`
		}
		if err := decode(r, &in); err != nil {
			fail(err)
			return
		}
		if !h.gateMembers(w, r, in.MemberIDs) {
			return
		}
		ch, err := h.store.CreateTChannel(ctx, c.orgID, c.uid, in.Kind, in.Name, in.Topic, in.Description, in.MemberIDs)
		if err != nil {
			fail(err)
			return
		}
		h.emit(ch.MemberIDs, feedEnvelope{"type": "channel", "channel_id": ch.ID})
		writeJSON(w, 200, ch)
		return
	case n == 2 && seg[1] == "browse" && m == http.MethodGet:
		chs, err := h.store.BrowseChannels(ctx, c.orgID, c.uid)
		if err != nil {
			fail(err)
			return
		}
		writeJSON(w, 200, map[string]any{"channels": chs})
		return
	}
	if n < 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such route"})
		return
	}
	id := seg[1]
	if h.tabsRoute(w, r, c, seg) {
		return
	}
	if n == 2 {
		switch m {
		case http.MethodGet:
			ch, err := h.visibleChannel(ctx, c, id)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, ch)
			return
		case http.MethodPatch:
			ch, err := h.memberChannel(ctx, c, id)
			if err != nil {
				fail(err)
				return
			}
			var in struct {
				Name        *string `json:"name"`
				Topic       *string `json:"topic"`
				Description *string `json:"description"`
				Archived    *bool   `json:"archived"`
				Announce    *bool   `json:"announce"`
			}
			if err := decode(r, &in); err != nil {
				fail(err)
				return
			}
			if in.Announce != nil && ch.CreatedBy != "" && ch.CreatedBy != c.uid {
				fail(ErrForbidden)
				return
			}
			if (in.Topic != nil && len(*in.Topic) > 250) || (in.Description != nil && len(*in.Description) > 2000) {
				fail(ErrInvalid)
				return
			}
			if in.Archived != nil && ch.CreatedBy != "" && ch.CreatedBy != c.uid {
				fail(ErrForbidden)
				return
			}
			if err := h.store.UpdateChannel(ctx, c.orgID, id, in.Name, in.Topic, in.Description, in.Archived, in.Announce); err != nil {
				fail(err)
				return
			}
			h.emit(ch.MemberIDs, feedEnvelope{"type": "channel", "channel_id": id})
			up, _ := h.store.GetChannel(ctx, c.orgID, c.uid, id)
			writeJSON(w, 200, up)
			return
		}
	}
	if n == 3 {
		act := seg[2]
		switch {
		case act == "join" && m == http.MethodPost:
			ch, err := h.store.GetChannel(ctx, c.orgID, c.uid, id)
			if err != nil {
				fail(err)
				return
			}
			if ch.Kind != "public" || ch.Archived {
				fail(ErrForbidden)
				return
			}
			if err := h.store.AddMembers(ctx, c.orgID, id, []string{c.uid}); err != nil {
				fail(err)
				return
			}
			h.emit(append(ch.MemberIDs, c.uid), feedEnvelope{"type": "channel", "channel_id": id})
			up, _ := h.store.GetChannel(ctx, c.orgID, c.uid, id)
			writeJSON(w, 200, up)
			return
		case act == "leave" && m == http.MethodPost:
			ch, err := h.memberChannel(ctx, c, id)
			if err != nil {
				fail(err)
				return
			}
			if ch.Kind == "dm" {
				fail(fmt.Errorf("%w: cannot leave a DM", ErrInvalid))
				return
			}
			if err := h.store.RemoveMember(ctx, c.orgID, id, c.uid); err != nil {
				fail(err)
				return
			}
			h.emit(ch.MemberIDs, feedEnvelope{"type": "channel", "channel_id": id})
			ok()
			return
		case act == "members" && m == http.MethodPost:
			ch, err := h.memberChannel(ctx, c, id)
			if err != nil {
				fail(err)
				return
			}
			if ch.Kind == "dm" {
				fail(fmt.Errorf("%w: cannot add people to a DM", ErrInvalid))
				return
			}
			var in struct {
				UserIDs []string `json:"user_ids"`
			}
			if err := decode(r, &in); err != nil {
				fail(err)
				return
			}
			if !h.gateMembers(w, r, append(append([]string{}, ch.MemberIDs...), in.UserIDs...)) {
				return
			}
			if err := h.store.AddMembers(ctx, c.orgID, id, in.UserIDs); err != nil {
				fail(err)
				return
			}
			h.emit(append(append([]string{}, ch.MemberIDs...), in.UserIDs...), feedEnvelope{"type": "channel", "channel_id": id})
			up, _ := h.store.GetChannel(ctx, c.orgID, c.uid, id)
			writeJSON(w, 200, up)
			return
		case act == "typing" && m == http.MethodPost:
			ch, err := h.memberChannel(ctx, c, id)
			if err != nil {
				fail(err)
				return
			}
			h.emit(typingTargets(ch.MemberIDs, c.uid), feedEnvelope{"type": "typing", "channel_id": id, "user_id": c.uid})
			ok()
			return
		case act == "read" && m == http.MethodPost:
			if _, err := h.memberChannel(ctx, c, id); err != nil {
				fail(err)
				return
			}
			if err := h.store.repo.UpdateReadCursor(ctx, id, c.uid); err != nil {
				fail(err)
				return
			}
			h.emit([]string{c.uid}, feedEnvelope{"type": "read", "channel_id": id})
			ok()
			return
		case act == "prefs" && m == http.MethodPatch:
			if _, err := h.memberChannel(ctx, c, id); err != nil {
				fail(err)
				return
			}
			var in struct {
				Starred   *bool   `json:"starred"`
				Muted     *bool   `json:"muted"`
				SectionID *string `json:"section_id"`
			}
			if err := decode(r, &in); err != nil {
				fail(err)
				return
			}
			if err := h.store.SetPrefs(ctx, c.uid, id, in.Starred, in.Muted, in.SectionID); err != nil {
				fail(err)
				return
			}
			h.emit([]string{c.uid}, feedEnvelope{"type": "channel", "channel_id": id})
			ok()
			return
		case act == "messages" && m == http.MethodGet:
			if _, err := h.visibleChannel(ctx, c, id); err != nil {
				fail(err)
				return
			}
			q := r.URL.Query()
			limit, _ := strconv.Atoi(q.Get("limit"))
			var from *time.Time
			if v := q.Get("from"); v != "" {
				t, err := parseTime(v)
				if err != nil {
					fail(err)
					return
				}
				from = &t
			}
			ms, more, err := h.store.ListChannelMessages(ctx, c.orgID, c.uid, id, q.Get("before"), q.Get("around"), from, limit)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"messages": ms, "has_more": more})
			return
		case act == "messages" && m == http.MethodPost:
			h.post(w, r, c, id)
			return
		case act == "pins" && m == http.MethodGet:
			if _, err := h.visibleChannel(ctx, c, id); err != nil {
				fail(err)
				return
			}
			ms, err := h.store.ListPins(ctx, c.orgID, c.uid, id)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"messages": ms})
			return
		case act == "bookmarks" && m == http.MethodGet:
			if _, err := h.visibleChannel(ctx, c, id); err != nil {
				fail(err)
				return
			}
			bs, err := h.store.ListBookmarks(ctx, id)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, 200, map[string]any{"bookmarks": bs})
			return
		case act == "bookmarks" && m == http.MethodPost:
			ch, err := h.memberChannel(ctx, c, id)
			if err != nil {
				fail(err)
				return
			}
			var in struct {
				Title string `json:"title"`
				URL   string `json:"url"`
			}
			if err := decode(r, &in); err != nil {
				fail(err)
				return
			}
			b, err := h.store.AddBookmark(ctx, c.uid, id, in.Title, in.URL)
			if err != nil {
				fail(err)
				return
			}
			h.emit(ch.MemberIDs, feedEnvelope{"type": "pins", "channel_id": id})
			writeJSON(w, 200, b)
			return
		case act == "huddle" && m == http.MethodPost:
			h.huddle(w, r, c, id)
			return
		}
	}
	if n == 4 && seg[2] == "members" && m == http.MethodDelete {
		ch, err := h.memberChannel(ctx, c, id)
		if err != nil {
			fail(err)
			return
		}
		if ch.Kind == "dm" {
			fail(fmt.Errorf("%w: cannot remove people from a DM", ErrInvalid))
			return
		}
		if err := h.store.RemoveMember(ctx, c.orgID, id, seg[3]); err != nil {
			fail(err)
			return
		}
		h.emit(append(append([]string{}, ch.MemberIDs...), seg[3]), feedEnvelope{"type": "channel", "channel_id": id})
		ok()
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such route"})
}

func (h *TeamsHandler) post(w http.ResponseWriter, r *http.Request, c apiCtx, chID string) {
	var in struct {
		Body          string   `json:"body"`
		ParentID      string   `json:"parent_id"`
		AttachmentIDs []string `json:"attachment_ids"`
		ClientNonce   string   `json:"client_nonce"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if len(in.AttachmentIDs) > 0 && h.gate != nil {
		ch, cerr := h.memberChannel(r.Context(), c, chID)
		if cerr != nil {
			writeErr(w, cerr)
			return
		}
		if gerr := checkUpload(r.Context(), h.gate, ch.MemberIDs, len(in.AttachmentIDs)); gerr != nil {
			if d, ok := authz.AsDenied(gerr); ok {
				authz.WriteControlDenied(w, d)
			} else {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": gerr.Error()})
			}
			return
		}
	}
	res, err := h.store.Post(r.Context(), c.orgID, c.uid, c.uname, chID, in.ParentID, in.Body, in.AttachmentIDs, in.ClientNonce)
	if err != nil {
		writeErr(w, err)
		return
	}
	h.broadcastPost(res)
	writeJSON(w, 200, res.Message)
}

// huddle returns (creating if needed) the channel's Meet code. The client
// creates the Meet room via /meet/codes and passes the code in; the first
// caller wins. A "Huddle started" message is posted if none was in the last 10 min.
func (h *TeamsHandler) huddle(w http.ResponseWriter, r *http.Request, c apiCtx, chID string) {
	ctx := r.Context()
	ch, err := h.memberChannel(ctx, c, chID)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	code := ch.HuddleCode
	if code == "" {
		if len(in.Code) != 12 {
			writeErr(w, fmt.Errorf("%w: meeting code required", ErrInvalid))
			return
		}
		code, err = h.store.SetHuddle(ctx, c.orgID, chID, in.Code)
		if err != nil {
			writeErr(w, err)
			return
		}
	}
	var recent bool
	_ = h.store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.chat_messages WHERE channel_id=$1::uuid AND body LIKE '%Huddle started%' AND sent_at > now() - interval '10 minutes')`, chID).Scan(&recent)
	if !recent && !ch.Archived {
		body := fmt.Sprintf("📞 **Huddle started** by %s. [Join the huddle](/meet/%s?audio=1)", c.uname, code)
		if res, err := h.store.Post(ctx, c.orgID, c.uid, c.uname, chID, "", body, nil, ""); err == nil {
			h.broadcastPost(res)
		}
	}
	h.emit(ch.MemberIDs, feedEnvelope{"type": "channel", "channel_id": chID})
	writeJSON(w, 200, map[string]string{"code": code, "url": "/meet/" + code + "?audio=1"})
}

func (h *TeamsHandler) messages(w http.ResponseWriter, r *http.Request, c apiCtx, seg []string) {
	ctx := r.Context()
	n := len(seg)
	m := r.Method
	fail := func(err error) { writeErr(w, err) }
	if n < 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such route"})
		return
	}
	id := seg[1]
	msg, err := h.store.GetMessage(ctx, c.orgID, c.uid, id)
	if err != nil {
		fail(err)
		return
	}
	var ch TChannel
	if m == http.MethodGet && n == 2 {
		ch, err = h.visibleChannel(ctx, c, msg.ChannelID)
	} else {
		ch, err = h.memberChannel(ctx, c, msg.ChannelID)
	}
	if err != nil {
		fail(err)
		return
	}
	switch {
	case n == 2 && m == http.MethodGet:
		writeJSON(w, 200, msg)
		return
	case n == 2 && m == http.MethodPatch:
		var in struct {
			Body string `json:"body"`
		}
		if err := decode(r, &in); err != nil {
			fail(err)
			return
		}
		up, err := h.store.EditMessage(ctx, c.orgID, c.uid, id, in.Body)
		if err != nil {
			fail(err)
			return
		}
		h.emit(ch.MemberIDs, feedEnvelope{"type": "edited", "channel_id": ch.ID, "message": up})
		writeJSON(w, 200, up)
		return
	case n == 2 && m == http.MethodDelete:
		if err := h.store.repo.DeleteMessage(ctx, c.orgID, ch.ID, id, c.uid); err != nil {
			fail(err)
			return
		}
		h.emit(ch.MemberIDs, feedEnvelope{"type": "deleted", "channel_id": ch.ID, "id": id, "parent_id": msg.ParentID})
		if h.hub != nil {
			h.hub.BroadcastDeleted(ch.ID, id)
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	case n == 3 && seg[2] == "thread" && m == http.MethodGet:
		root := msg
		if msg.ParentID != "" {
			root, err = h.store.GetMessage(ctx, c.orgID, c.uid, msg.ParentID)
			if err != nil {
				fail(err)
				return
			}
		}
		replies, err := h.store.ListThread(ctx, c.orgID, c.uid, root.ID)
		if err != nil {
			fail(err)
			return
		}
		writeJSON(w, 200, map[string]any{"parent": root, "replies": replies})
		return
	case n == 3 && seg[2] == "react" && m == http.MethodPost:
		var in struct {
			Emoji string `json:"emoji"`
		}
		if err := decode(r, &in); err != nil {
			fail(err)
			return
		}
		if in.Emoji == "" || len(in.Emoji) > 64 {
			fail(ErrInvalid)
			return
		}
		rx, err := h.store.repo.ToggleReaction(ctx, c.orgID, id, c.uid, in.Emoji)
		if err != nil {
			fail(err)
			return
		}
		lists, _ := h.store.ReactorLists(ctx, id)
		order := []map[string]any{}
		for _, x := range rx {
			order = append(order, map[string]any{"emoji": x.Emoji, "user_ids": lists[x.Emoji]})
		}
		h.emit(ch.MemberIDs, feedEnvelope{"type": "reaction", "channel_id": ch.ID, "message_id": id, "reactions": order})
		out := make([]TReaction, 0, len(rx))
		for _, x := range rx {
			out = append(out, TReaction{Emoji: x.Emoji, Count: int(x.Count), Me: x.Me})
		}
		writeJSON(w, 200, map[string]any{"reaction_details": out})
		return
	case n == 3 && seg[2] == "pin" && m == http.MethodPost:
		pinned, err := h.store.TogglePin(ctx, c.uid, ch.ID, id)
		if err != nil {
			fail(err)
			return
		}
		h.emit(ch.MemberIDs, feedEnvelope{"type": "pins", "channel_id": ch.ID, "message_id": id, "pinned": pinned})
		writeJSON(w, 200, map[string]bool{"pinned": pinned})
		return
	case n == 3 && seg[2] == "save" && m == http.MethodPost:
		saved, err := h.store.ToggleSaved(ctx, c.uid, id)
		if err != nil {
			fail(err)
			return
		}
		h.emit([]string{c.uid}, feedEnvelope{"type": "saved", "message_id": id, "saved": saved})
		writeJSON(w, 200, map[string]bool{"saved": saved})
		return
	case n == 3 && seg[2] == "remind" && m == http.MethodPost:
		var in struct {
			At   string `json:"at"`
			Note string `json:"note"`
		}
		if err := decode(r, &in); err != nil {
			fail(err)
			return
		}
		at, err := parseTime(in.At)
		if err != nil {
			fail(err)
			return
		}
		rem, err := h.store.AddReminder(ctx, c.orgID, c.uid, id, in.Note, at)
		if err != nil {
			fail(err)
			return
		}
		writeJSON(w, 200, rem)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such route"})
}

// ServeLive upgrades the per-user live feed.
func (h *TeamsHandler) ServeLive(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(r)
	if !ok || h.hub == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.hub.ServeUser(w, r, c.uid, c.orgID)
}

// IsLivePath reports whether path is the live feed endpoint.
func IsLivePath(path string) bool { return path == MountPrefix+"live" }

// IsTeamsPath reports whether path belongs to the Teams REST API.
func IsTeamsPath(path string) bool {
	return strings.HasPrefix(path, MountPrefix) && path != MountPrefix+"live"
}

// RunScheduler delivers scheduled messages and reminders until ctx ends.
func (h *TeamsHandler) RunScheduler(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.Tick(ctx)
		}
	}
}

// Tick processes everything due now (exported for tests).
func (h *TeamsHandler) Tick(ctx context.Context) {
	due, err := h.store.ClaimDueScheduled(ctx)
	if err != nil {
		log.Printf("chat scheduler: %v", err)
	}
	for _, d := range due {
		name := h.store.UserName(ctx, d.UserID)
		res, err := h.store.Post(ctx, d.OrgID, d.UserID, name, d.ChannelID, d.ParentID, d.Body, nil, "")
		if err != nil {
			_, _ = h.store.pool.Exec(ctx, `UPDATE adrient.chat_scheduled SET status='failed' WHERE id=$1::uuid`, d.ID)
			continue
		}
		h.broadcastPost(res)
		h.emit([]string{d.UserID}, feedEnvelope{"type": "scheduled"})
	}
	rems, err := h.store.ClaimDueReminders(ctx)
	if err != nil {
		log.Printf("chat reminders: %v", err)
	}
	for _, rm := range rems {
		env := feedEnvelope{"type": "reminder", "note": rm.Note, "message_id": rm.MessageID}
		if rm.MessageID != "" {
			if msg, err := h.store.GetMessage(ctx, rm.OrgID, rm.UserID, rm.MessageID); err == nil {
				env["message"] = msg
			}
		}
		h.emit([]string{rm.UserID}, env)
	}
}

// typingTargets are the members who should see that uid is typing: everyone
// in the channel except the typist.
func typingTargets(members []string, uid string) []string {
	out := make([]string, 0, len(members))
	for _, m := range members {
		if m != uid {
			out = append(out, m)
		}
	}
	return out
}
