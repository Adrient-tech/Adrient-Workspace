// Package notifyhook is how other apps (HR, CRM, Accounting, Tickets...) put
// events into people's notification feed. Go modules call Hook.Emit; outside
// systems POST to /api/v1/notify/emit with a source token created in the admin
// console. The notification center groups everything by app.
package notifyhook

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/notifications"
)

// Event is one notification to deliver. Recipients are user ids, e-mail
// addresses of people in the org, or a group id; at least one is required.
type Event struct {
	OrgID      string
	App        string // launcher app id or free label, e.g. "hr", "crm"
	Type       string // e.g. "leave_request"; defaults to App
	Title      string
	Body       string
	URL        string // in-app link such as /hr/leave/123
	ActorID    string
	UserIDs    []string
	Emails     []string
	GroupID    string
	Recipients int // filled by Emit
}

// Limits.
const (
	MaxRecipients = 500
	MaxTitle      = 200
	MaxBody       = 2000
	MaxURL        = 2048
	SourcePerMin  = 600
)

var (
	ErrBad = errors.New("invalid notification")
)

// Hook delivers events.
type Hook struct {
	Pool *pgxpool.Pool
	Repo *notifications.Repository

	mu   sync.Mutex
	hits map[string][]time.Time
}

func cleanURL(u string) (string, bool) {
	u = strings.TrimSpace(u)
	if u == "" {
		return "", true
	}
	if len(u) > MaxURL || strings.ContainsAny(u, "\r\n") {
		return "", false
	}
	// Only in-app paths or http(s) links.
	if strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") {
		return u, true
	}
	if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		return u, true
	}
	return "", false
}

// Emit validates the event, resolves recipients inside the org and stores one
// notification per person (the live push to open tabs happens in the repository).
func (h *Hook) Emit(ctx context.Context, ev Event) (int, error) {
	ev.Title = strings.TrimSpace(ev.Title)
	if ev.OrgID == "" || ev.Title == "" || len(ev.Title) > MaxTitle || len(ev.Body) > MaxBody {
		return 0, fmt.Errorf("%w: a title (up to %d characters) is required", ErrBad, MaxTitle)
	}
	link, ok := cleanURL(ev.URL)
	if !ok {
		return 0, fmt.Errorf("%w: link must be an in-app path or an http(s) URL", ErrBad)
	}
	app := strings.ToLower(strings.TrimSpace(ev.App))
	if app == "" || len(app) > 40 {
		return 0, fmt.Errorf("%w: app is required", ErrBad)
	}
	typ := ev.Type
	if typ == "" {
		typ = app
	}
	ids := map[string]bool{}
	for _, id := range ev.UserIDs {
		ids[id] = true
	}
	if len(ev.Emails) > 0 {
		rows, err := h.Pool.Query(ctx, `SELECT id::text FROM adrient.users WHERE org_id::text=$1 AND lower(email)=ANY($2) AND status='active' AND deleted_at IS NULL`, ev.OrgID, lowerAll(ev.Emails))
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids[id] = true
		}
		rows.Close()
	}
	if ev.GroupID != "" {
		rows, err := h.Pool.Query(ctx, `SELECT gm.user_id::text FROM adrient.group_members gm JOIN adrient.groups g ON g.id=gm.group_id WHERE g.org_id::text=$1 AND g.id::text=$2`, ev.OrgID, ev.GroupID)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids[id] = true
		}
		rows.Close()
	}
	if len(ids) == 0 {
		return 0, fmt.Errorf("%w: no recipients", ErrBad)
	}
	if len(ids) > MaxRecipients {
		return 0, fmt.Errorf("%w: at most %d recipients", ErrBad, MaxRecipients)
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	// Keep only active people of this org: a source can never reach another org.
	rows, err := h.Pool.Query(ctx, `SELECT id::text FROM adrient.users WHERE org_id::text=$1 AND id::text = ANY($2) AND status='active' AND deleted_at IS NULL`, ev.OrgID, list)
	if err != nil {
		return 0, err
	}
	var valid []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		valid = append(valid, id)
	}
	rows.Close()
	n := 0
	for _, id := range valid {
		nn, err := h.Repo.Create(ctx, notifications.CreateParams{OrgID: ev.OrgID, UserID: id, Type: typ, ActorUserID: ev.ActorID, Title: ev.Title, Body: ev.Body, TargetURL: link})
		if err != nil {
			continue
		}
		_, _ = h.Pool.Exec(ctx, `UPDATE adrient.notifications SET app=$2 WHERE id::text=$1`, nn.ID, app)
		n++
	}
	return n, nil
}

func lowerAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, e := range in {
		out = append(out, strings.ToLower(strings.TrimSpace(e)))
	}
	return out
}

// ---- sources (tokens) --------------------------------------------------------------

// Source is a registered emitter.
type Source struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	App        string     `json:"app"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

func hashTok(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// CreateSource registers an emitter and returns its token once.
func (h *Hook) CreateSource(ctx context.Context, orgID, actor, name, app string) (string, Source, error) {
	name, app = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(app))
	if name == "" || len(name) > 80 || app == "" || len(app) > 40 {
		return "", Source{}, fmt.Errorf("%w: a name and an app are required", ErrBad)
	}
	raw := make([]byte, 24)
	_, _ = rand.Read(raw)
	plain := "ntf_" + hex.EncodeToString(raw)
	var s Source
	err := h.Pool.QueryRow(ctx, `INSERT INTO adrient.notify_sources (org_id, name, app, prefix, token_hash, created_by) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid)
RETURNING id::text, name, app, prefix, created_at, last_used_at, revoked_at`, orgID, name, app, plain[:10], hashTok(plain), actor).
		Scan(&s.ID, &s.Name, &s.App, &s.Prefix, &s.CreatedAt, &s.LastUsedAt, &s.RevokedAt)
	return plain, s, err
}

// ListSources lists the org's sources.
func (h *Hook) ListSources(ctx context.Context, orgID string) ([]Source, error) {
	rows, err := h.Pool.Query(ctx, `SELECT id::text, name, app, prefix, created_at, last_used_at, revoked_at FROM adrient.notify_sources WHERE org_id::text=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.ID, &s.Name, &s.App, &s.Prefix, &s.CreatedAt, &s.LastUsedAt, &s.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RevokeSource revokes a source token.
func (h *Hook) RevokeSource(ctx context.Context, orgID, id string) bool {
	tag, err := h.Pool.Exec(ctx, `UPDATE adrient.notify_sources SET revoked_at=now() WHERE org_id::text=$1 AND id::text=$2 AND revoked_at IS NULL`, orgID, id)
	return err == nil && tag.RowsAffected() > 0
}

func (h *Hook) allow(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hits == nil {
		h.hits = map[string][]time.Time{}
	}
	now := time.Now()
	l := h.hits[id][:0]
	for _, t := range h.hits[id] {
		if t.After(now.Add(-time.Minute)) {
			l = append(l, t)
		}
	}
	if len(l) >= SourcePerMin {
		h.hits[id] = l
		return false
	}
	h.hits[id] = append(l, now)
	return true
}

// ---- HTTP ------------------------------------------------------------------------------

// Paths.
const (
	EmitPath    = "/api/v1/notify/emit"
	CenterPath  = "/api/v1/notify/center"
	SourcesPath = "/api/v1/admin/notify-sources"
)

// Match reports whether path belongs to this package.
func Match(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == EmitPath || p == CenterPath || p == SourcesPath || strings.HasPrefix(p, SourcesPath+"/")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ServeEmit handles POST /api/v1/notify/emit with a source bearer token (no session).
func (h *Hook) ServeEmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	hdr := r.Header.Get("Authorization")
	plain := ""
	if len(hdr) > 7 && strings.EqualFold(hdr[:7], "bearer ") {
		plain = strings.TrimSpace(hdr[7:])
	}
	var orgID, app, sid string
	err := h.Pool.QueryRow(r.Context(), `SELECT org_id::text, app, id::text FROM adrient.notify_sources WHERE token_hash=$1 AND revoked_at IS NULL`, hashTok(plain)).Scan(&orgID, &app, &sid)
	if err != nil || !strings.HasPrefix(plain, "ntf_") {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "a valid source token is required"})
		return
	}
	if !h.allow(sid) {
		w.Header().Set("Retry-After", "30")
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate limit exceeded"})
		return
	}
	var in struct {
		Type    string   `json:"type"`
		Title   string   `json:"title"`
		Body    string   `json:"body"`
		Link    string   `json:"link"`
		UserIDs []string `json:"user_ids"`
		Emails  []string `json:"emails"`
		GroupID string   `json:"group_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(&in) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	n, err := h.Emit(r.Context(), Event{OrgID: orgID, App: app, Type: in.Type, Title: in.Title, Body: in.Body, URL: in.Link, UserIDs: in.UserIDs, Emails: in.Emails, GroupID: in.GroupID})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	_, _ = h.Pool.Exec(r.Context(), `UPDATE adrient.notify_sources SET last_used_at=now() WHERE id::text=$1`, sid)
	writeJSON(w, http.StatusOK, map[string]any{"delivered": n})
}

// ServeSession handles the session-authenticated routes (center and admin sources).
func (h *Hook) ServeSession(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	u, ok2 := auth.UserFromContext(r.Context())
	if !ok || !ok2 {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	ctx := r.Context()
	p := strings.TrimRight(r.URL.Path, "/")
	if p == CenterPath && r.Method == http.MethodGet {
		app := r.URL.Query().Get("app")
		unread := r.URL.Query().Get("unread") == "1"
		rows, err := h.Pool.Query(ctx, `SELECT * FROM (SELECT id::text, type, COALESCE(NULLIF(app,''), CASE
  WHEN type LIKE 'meet%' OR type LIKE 'call%' THEN 'meet' WHEN type LIKE 'chat%' OR type IN ('mention','message','dm') THEN 'chat'
  WHEN type LIKE 'drive%' OR type LIKE 'share%' OR type='grant' THEN 'drive' WHEN type LIKE 'calendar%' OR type LIKE 'event%' THEN 'calendar'
  WHEN type LIKE 'task%' THEN 'tasks' WHEN type LIKE 'mail%' THEN 'mail' ELSE type END) AS app,
  title, body, target_url, read, created_at FROM adrient.notifications WHERE org_id::text=$1 AND user_id::text=$2) t
  WHERE ($3='' OR t.app=$3) AND (NOT $4 OR NOT t.read) ORDER BY t.created_at DESC LIMIT 200`, o.ID, u.ID, app, unread)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "read failed"})
			return
		}
		defer rows.Close()
		type item struct {
			ID, Type, App, Title, Body, URL string
			Read                            bool
			At                              time.Time
		}
		items := []map[string]any{}
		counts := map[string]int{}
		for rows.Next() {
			var x item
			if err := rows.Scan(&x.ID, &x.Type, &x.App, &x.Title, &x.Body, &x.URL, &x.Read, &x.At); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "read failed"})
				return
			}
			if !x.Read {
				counts[x.App]++
			}
			items = append(items, map[string]any{"id": x.ID, "type": x.Type, "app": x.App, "title": x.Title, "body": x.Body, "url": x.URL, "read": x.Read, "created_at": x.At})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "unread_by_app": counts})
		return
	}
	// Admin: source tokens. Super admins only.
	if !authz.IsAdmin(ctx) {
		authz.WriteForbidden(w, authz.PermOrgAdmin)
		return
	}
	switch {
	case p == SourcesPath && r.Method == http.MethodGet:
		l, err := h.ListSources(ctx, o.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "read failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sources": l, "endpoint": EmitPath})
	case p == SourcesPath && r.Method == http.MethodPost:
		var in struct{ Name, App string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		plain, s, err := h.CreateSource(ctx, o.ID, u.ID, in.Name, in.App)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"token": plain, "source": s, "note": "Copy the token now. It is not shown again."})
	case strings.HasPrefix(p, SourcesPath+"/") && r.Method == http.MethodDelete:
		if !h.RevokeSource(ctx, o.ID, strings.TrimPrefix(p, SourcesPath+"/")) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "source not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}
