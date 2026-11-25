package chat

import (
	"context"
	"net/http"
	"strings"
)

// Tab is an app or link pinned to a channel (Teams-style channel tab).
type Tab struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// ChannelFile is an attachment shared in a channel, with the posting message.
type ChannelFile struct {
	TAttachment
	MessageID string `json:"message_id"`
	SentAt    string `json:"sent_at"`
}

var tabKinds = map[string]bool{
	"doc": true, "sheet": true, "slides": true, "whiteboard": true,
	"tasks": true, "project": true, "link": true,
}

const maxTabsPerChannel = 20

// ListTabs returns a channel's tabs in display order.
func (s *Store) ListTabs(ctx context.Context, chID string) ([]Tab, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, kind, title, url FROM adrient.chat_tabs
	  WHERE channel_id=$1::uuid ORDER BY position, created_at`, chID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tab{}
	for rows.Next() {
		var t Tab
		if err := rows.Scan(&t.ID, &t.Kind, &t.Title, &t.URL); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AddTab pins a tab. URLs must be in-app paths ("/sheets/<id>") or http(s) links.
func (s *Store) AddTab(ctx context.Context, uid, chID, kind, title, url string) (Tab, error) {
	kind, title, url = strings.TrimSpace(kind), strings.TrimSpace(title), strings.TrimSpace(url)
	if kind == "" {
		kind = "link"
	}
	validURL := strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") ||
		(strings.HasPrefix(url, "/") && !strings.HasPrefix(url, "//"))
	if !tabKinds[kind] || !validURL || len(url) > 2000 || len(title) > 120 {
		return Tab{}, ErrInvalid
	}
	if title == "" {
		title = url
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.chat_tabs WHERE channel_id=$1::uuid`, chID).Scan(&n); err != nil {
		return Tab{}, err
	}
	if n >= maxTabsPerChannel {
		return Tab{}, ErrInvalid
	}
	var t Tab
	err := s.pool.QueryRow(ctx, `INSERT INTO adrient.chat_tabs (channel_id,kind,title,url,position,created_by)
	  VALUES ($1::uuid,$2,$3,$4,$5,$6::uuid) RETURNING id::text,kind,title,url`,
		chID, kind, title, url, n, uid).Scan(&t.ID, &t.Kind, &t.Title, &t.URL)
	return t, err
}

// RenameTab changes a tab title.
func (s *Store) RenameTab(ctx context.Context, chID, id, title string) error {
	title = strings.TrimSpace(title)
	if !IsUUID(id) || title == "" || len(title) > 120 {
		return ErrInvalid
	}
	tag, err := s.pool.Exec(ctx, `UPDATE adrient.chat_tabs SET title=$3 WHERE id=$1::uuid AND channel_id=$2::uuid`, id, chID, title)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteTab removes a tab.
func (s *Store) DeleteTab(ctx context.Context, chID, id string) error {
	if !IsUUID(id) {
		return ErrNotFound
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM adrient.chat_tabs WHERE id=$1::uuid AND channel_id=$2::uuid`, id, chID)
	return err
}

// ListChannelFiles returns every attachment posted in a channel, newest first.
func (s *Store) ListChannelFiles(ctx context.Context, orgID, chID string) ([]ChannelFile, error) {
	rows, err := s.pool.Query(ctx, `SELECT a.id::text, a.name, a.mime_type, a.size_bytes, m.id::text, m.sent_at::text
	  FROM adrient.chat_attachments a JOIN adrient.chat_messages m ON m.id=a.message_id
	  WHERE m.channel_id=$1::uuid AND m.org_id=$2::uuid ORDER BY m.sent_at DESC LIMIT 500`, chID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChannelFile{}
	for rows.Next() {
		var f ChannelFile
		if err := rows.Scan(&f.ID, &f.Name, &f.MimeType, &f.Size, &f.MessageID, &f.SentAt); err != nil {
			return nil, err
		}
		f.URL = "/api/v1/chat/attachments/" + f.ID + "/content"
		out = append(out, f)
	}
	return out, rows.Err()
}

// tabsRoute serves /channels/{id}/tabs[/{tabId}] and /channels/{id}/files.
// It reports whether it handled the request.
func (h *TeamsHandler) tabsRoute(w http.ResponseWriter, r *http.Request, c apiCtx, seg []string) bool {
	n := len(seg)
	if n < 3 || (seg[2] != "tabs" && seg[2] != "files") {
		return false
	}
	ctx := r.Context()
	id := seg[1]
	m := r.Method
	fail := func(err error) { writeErr(w, err) }
	switch {
	case seg[2] == "files" && n == 3 && m == http.MethodGet:
		if _, err := h.visibleChannel(ctx, c, id); err != nil {
			fail(err)
			return true
		}
		fs, err := h.store.ListChannelFiles(ctx, c.orgID, id)
		if err != nil {
			fail(err)
			return true
		}
		writeJSON(w, 200, map[string]any{"files": fs})
	case seg[2] == "tabs" && n == 3 && m == http.MethodGet:
		if _, err := h.visibleChannel(ctx, c, id); err != nil {
			fail(err)
			return true
		}
		ts, err := h.store.ListTabs(ctx, id)
		if err != nil {
			fail(err)
			return true
		}
		writeJSON(w, 200, map[string]any{"tabs": ts})
	case seg[2] == "tabs" && n == 3 && m == http.MethodPost:
		ch, err := h.memberChannel(ctx, c, id)
		if err != nil {
			fail(err)
			return true
		}
		var in struct {
			Kind  string `json:"kind"`
			Title string `json:"title"`
			URL   string `json:"url"`
		}
		if err := decode(r, &in); err != nil {
			fail(err)
			return true
		}
		t, err := h.store.AddTab(ctx, c.uid, id, in.Kind, in.Title, in.URL)
		if err != nil {
			fail(err)
			return true
		}
		h.emit(ch.MemberIDs, feedEnvelope{"type": "channel", "channel_id": id})
		writeJSON(w, 200, t)
	case seg[2] == "tabs" && n == 4 && (m == http.MethodPatch || m == http.MethodDelete):
		ch, err := h.memberChannel(ctx, c, id)
		if err != nil {
			fail(err)
			return true
		}
		if m == http.MethodDelete {
			err = h.store.DeleteTab(ctx, id, seg[3])
		} else {
			var in struct {
				Title string `json:"title"`
			}
			if err = decode(r, &in); err == nil {
				err = h.store.RenameTab(ctx, id, seg[3], in.Title)
			}
		}
		if err != nil {
			fail(err)
			return true
		}
		h.emit(ch.MemberIDs, feedEnvelope{"type": "channel", "channel_id": id})
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		return false
	}
	return true
}
