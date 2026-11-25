package tickets

import (
	"net/http"
	"strings"
	"time"
)

// serveDesk handles the wave 6 desk routes under /api/v1/ts/. It reports false when the
// path is not one of its own so the caller can fall through to 404.
//
//	GET  projects/{id}/desk?view=&q=   tickets (search / built-in view) and view counts
//	GET  tickets/{id}/desk             ticket with events, linked incidents and followers
//	PUT  tickets/{id}/meta             {tags, type, problem_id}
//	POST tickets/bulk                  {ids, status, priority, type, assignee, add_tags, remove_tags}
//	POST tickets/{id}/merge            {into}
//	POST tickets/{id}/follow           toggle following
//	POST tickets/{id}/macro            {macro_id}
//	GET/POST macros, PUT/DELETE macros/{id}
func (h *ExtrasHandler) serveDesk(w http.ResponseWriter, r *http.Request, org, user string, seg []string) bool {
	get, post, put, del := r.Method == http.MethodGet, r.Method == http.MethodPost, r.Method == http.MethodPut, r.Method == http.MethodDelete
	name := ""
	if h.auth.UserName != nil {
		name = h.auth.UserName(r)
	}
	ctx := WithActor(r.Context(), Actor{ID: user, Name: name})
	switch {
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "desk" && get:
		all, err := h.repo.ListDesk(ctx, org, seg[1])
		if err != nil {
			failHTTP(w, err)
			return true
		}
		now := time.Now()
		list := all
		if v := r.URL.Query().Get("view"); v != "" {
			var ok bool
			if list, ok = InView(all, v, user, now); !ok {
				http.Error(w, "unknown view", http.StatusBadRequest)
				return true
			}
		}
		if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
			pq := ParseSearch(q)
			kept := []DeskTicket{}
			for _, t := range list {
				if pq.Matches(t, user) {
					kept = append(kept, t)
				}
			}
			list = kept
		}
		if len(list) > 500 {
			list = list[:500]
		}
		for i := range list {
			list[i].Body = ""
		}
		outJSON(w, map[string]any{"tickets": list, "views": ViewCounts(all, user, now), "types": TicketTypes})
	case len(seg) == 3 && seg[0] == "tickets" && seg[2] == "desk" && get:
		t, err := h.repo.GetDesk(ctx, org, seg[1])
		if err != nil {
			failHTTP(w, err)
			return true
		}
		ev, err := h.repo.Events(ctx, org, seg[1])
		if err != nil {
			failHTTP(w, err)
			return true
		}
		inc, err := h.repo.Incidents(ctx, org, seg[1])
		if err != nil {
			failHTTP(w, err)
			return true
		}
		fol, err := h.repo.Followers(ctx, org, seg[1])
		if err != nil {
			failHTTP(w, err)
			return true
		}
		following := false
		for _, f := range fol {
			following = following || f == user
		}
		outJSON(w, map[string]any{"ticket": t, "events": ev, "incidents": inc, "followers": fol, "following": following})
	case len(seg) == 3 && seg[0] == "tickets" && seg[2] == "meta" && put:
		var m Meta
		if !decode(w, r, &m) {
			return true
		}
		t, err := h.repo.SetMeta(ctx, org, seg[1], m)
		if err != nil {
			failHTTP(w, err)
			return true
		}
		outJSON(w, t)
	case len(seg) == 2 && seg[0] == "tickets" && seg[1] == "bulk" && post:
		var c BulkChange
		if !decode(w, r, &c) {
			return true
		}
		n, err := h.repo.Bulk(ctx, org, c)
		if err != nil {
			failHTTP(w, err)
			return true
		}
		outJSON(w, map[string]any{"changed": n})
	case len(seg) == 3 && seg[0] == "tickets" && seg[2] == "merge" && post:
		var in struct {
			Into string `json:"into"`
		}
		if !decode(w, r, &in) {
			return true
		}
		t, err := h.repo.Merge(ctx, org, seg[1], in.Into)
		if err != nil {
			failHTTP(w, err)
			return true
		}
		outJSON(w, t)
	case len(seg) == 3 && seg[0] == "tickets" && seg[2] == "follow" && post:
		on, err := h.repo.ToggleFollow(ctx, org, seg[1], user)
		if err != nil {
			failHTTP(w, err)
			return true
		}
		outJSON(w, map[string]any{"following": on})
	case len(seg) == 3 && seg[0] == "tickets" && seg[2] == "macro" && post:
		var in struct {
			MacroID string `json:"macro_id"`
		}
		if !decode(w, r, &in) {
			return true
		}
		t, err := h.repo.ApplyMacro(ctx, org, in.MacroID, seg[1])
		if err != nil {
			failHTTP(w, err)
			return true
		}
		outJSON(w, t)
	case len(seg) == 1 && seg[0] == "macros" && get:
		v, err := h.repo.ListMacros(ctx, org, r.URL.Query().Get("project"))
		if err != nil {
			failHTTP(w, err)
			return true
		}
		outJSON(w, map[string]any{"macros": v})
	case len(seg) == 1 && seg[0] == "macros" && post, len(seg) == 2 && seg[0] == "macros" && put:
		var m Macro
		if !decode(w, r, &m) {
			return true
		}
		if len(seg) == 2 {
			m.ID = seg[1]
		}
		v, err := h.repo.SaveMacro(ctx, org, user, m)
		if err != nil {
			failHTTP(w, err)
			return true
		}
		outJSON(w, v)
	case len(seg) == 2 && seg[0] == "macros" && del:
		if err := h.repo.DeleteMacro(ctx, org, seg[1]); err != nil {
			failHTTP(w, err)
			return true
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		return false
	}
	return true
}
