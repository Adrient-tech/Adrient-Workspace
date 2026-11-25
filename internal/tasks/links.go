package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func newToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// FeedToken returns the user's private calendar feed token, creating or rotating it.
func (r *Repository) FeedToken(ctx context.Context, org, user string, rotate bool) (string, error) {
	if !rotate {
		var t string
		err := r.pool.QueryRow(ctx, `SELECT token FROM adrient.task_feed_tokens WHERE user_id=$1`, user).Scan(&t)
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	t, err := newToken()
	if err != nil {
		return "", err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.task_feed_tokens (user_id, org_id, token) VALUES ($1,$2,$3)
		ON CONFLICT (user_id) DO UPDATE SET token=$3, created_at=now()`, user, org, t)
	return t, err
}

// FeedOwner resolves a feed token to its owner.
func (r *Repository) FeedOwner(ctx context.Context, token string) (org, user string, err error) {
	if len(token) < 32 {
		return "", "", ErrNotFound
	}
	err = r.pool.QueryRow(ctx, `SELECT org_id::text, user_id::text FROM adrient.task_feed_tokens WHERE token=$1`, token).Scan(&org, &user)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return
}

// PublicLink returns the list's public token, or "" when it is not shared. Owner only.
func (r *Repository) PublicLink(ctx context.Context, org, user, listID string) (string, error) {
	if err := r.requireOwner(ctx, org, user, listID); err != nil {
		return "", err
	}
	var t string
	err := r.pool.QueryRow(ctx, `SELECT token FROM adrient.task_public_links WHERE list_id=$1 AND org_id=$2`, listID, org).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return t, err
}

// CreatePublicLink publishes the list read-only (idempotent; rotate=true replaces the token).
func (r *Repository) CreatePublicLink(ctx context.Context, org, user, listID string, rotate bool) (string, error) {
	if err := r.requireOwner(ctx, org, user, listID); err != nil {
		return "", err
	}
	if !rotate {
		if t, err := r.PublicLink(ctx, org, user, listID); err != nil || t != "" {
			return t, err
		}
	}
	t, err := newToken()
	if err != nil {
		return "", err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.task_public_links (list_id, org_id, token, created_by) VALUES ($1,$2,$3,$4)
		ON CONFLICT (list_id) DO UPDATE SET token=$3, created_at=now()`, listID, org, t, user)
	return t, err
}

// RevokePublicLink stops sharing the list.
func (r *Repository) RevokePublicLink(ctx context.Context, org, user, listID string) error {
	if err := r.requireOwner(ctx, org, user, listID); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.task_public_links WHERE list_id=$1 AND org_id=$2`, listID, org)
	return err
}

func (r *Repository) requireOwner(ctx context.Context, org, user, listID string) error {
	role, err := r.Role(ctx, org, user, listID)
	if err != nil {
		return err
	}
	if role != RoleOwner {
		return ErrForbidden
	}
	return nil
}

// PublicTask is one row of a publicly shared list. It carries no ids of people.
type PublicTask struct {
	Title     string     `json:"title"`
	Notes     string     `json:"notes"`
	DueAt     *time.Time `json:"due_at"`
	Completed bool       `json:"completed"`
	Sub       bool       `json:"subtask"`
}

// PublicList is what an anonymous viewer of a shared list sees.
type PublicList struct {
	Name  string       `json:"name"`
	Tasks []PublicTask `json:"tasks"`
}

// PublicByToken loads a shared list by its token.
func (r *Repository) PublicByToken(ctx context.Context, token string) (PublicList, error) {
	var out PublicList
	if len(token) < 32 {
		return out, ErrNotFound
	}
	var listID, org string
	err := r.pool.QueryRow(ctx, `SELECT l.id::text, l.org_id::text, l.name FROM adrient.task_public_links p
		JOIN adrient.task_lists l ON l.id=p.list_id WHERE p.token=$1`, token).Scan(&listID, &org, &out.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	rows, err := r.pool.Query(ctx, `SELECT title, notes, due_at, completed, parent_task_id IS NOT NULL FROM adrient.tasks
		WHERE list_id=$1 AND org_id=$2 ORDER BY completed, position, created_at LIMIT 1000`, listID, org)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.Tasks = []PublicTask{}
	for rows.Next() {
		var t PublicTask
		if err := rows.Scan(&t.Title, &t.Notes, &t.DueAt, &t.Completed, &t.Sub); err != nil {
			return out, err
		}
		out.Tasks = append(out.Tasks, t)
	}
	return out, rows.Err()
}

// PublicHandler serves the session-less endpoints:
//
//	GET /api/v1/public/tasks/feed/{token}.ics   subscribe feed (calendar apps)
//	GET /api/v1/public/tasks/list/{token}       read-only HTML page of a shared list
//	GET /api/v1/public/tasks/list/{token}.json  the same as JSON
type PublicHandler struct{ repo *Repository }

// NewPublicHandler builds the handler.
func NewPublicHandler(repo *Repository) *PublicHandler { return &PublicHandler{repo: repo} }

const publicMount = "/api/v1/public/tasks/"

// Match reports whether path belongs to this handler.
func (h *PublicHandler) Match(path string) bool { return strings.HasPrefix(path, publicMount) }

var publicPage = template.Must(template.New("p").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex">
<title>{{.Name}} - Adrient Tasks</title>
<style>body{font:16px system-ui,sans-serif;max-width:640px;margin:32px auto;padding:0 16px;color:#1b2a27}
h1{color:#126C5A;font-size:24px}li{padding:8px 0;border-bottom:1px solid #e3ebe8;list-style:none}
.done{text-decoration:line-through;opacity:.55}.sub{margin-left:24px}.m{font-size:13px;opacity:.7}</style></head>
<body><h1>{{.Name}}</h1><p class="m">Shared read-only from Adrient Workspace.</p><ul style="padding:0">
{{range .Tasks}}<li class="{{if .Completed}}done{{end}} {{if .Sub}}sub{{end}}">{{if .Completed}}&#9745;{{else}}&#9744;{{end}} {{.Title}}
{{if .DueAt}}<span class="m"> due {{.DueAt.Format "2 Jan 2006"}}</span>{{end}}{{if .Notes}}<div class="m">{{.Notes}}</div>{{end}}</li>
{{else}}<li class="m">No tasks.</li>{{end}}</ul></body></html>`))

func (h *PublicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, publicMount)
	ctx := r.Context()
	switch {
	case strings.HasPrefix(rest, "feed/"):
		org, user, err := h.repo.FeedOwner(ctx, strings.TrimSuffix(strings.TrimPrefix(rest, "feed/"), ".ics"))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		v, err := h.repo.OpenDated(ctx, org, user)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		host := r.Host
		if i := strings.Index(host, ":"); i > 0 {
			host = host[:i]
		}
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("Cache-Control", "private, max-age=300")
		_, _ = io.WriteString(w, BuildICS(v, host))
	case strings.HasPrefix(rest, "list/"):
		tok := strings.TrimPrefix(rest, "list/")
		asJSON := strings.HasSuffix(tok, ".json")
		tok = strings.TrimSuffix(tok, ".json")
		pl, err := h.repo.PublicByToken(ctx, tok)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Robots-Tag", "noindex")
		if asJSON {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, jsonString(pl))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		_ = publicPage.Execute(w, pl)
	default:
		http.NotFound(w, r)
	}
}

func (h *PublicHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
