// Package globalsearch adds the sources the built-in unified search does not
// cover (Chat messages, CRM, Tasks, People) behind one registry. Any module can
// Register a Source and its results show up in the launcher search.
package globalsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Path is the REST endpoint.
const Path = "/api/v1/search/more"

// Result is one hit.
type Result struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

// Query is what a source receives. Pattern is a lower-cased ILIKE pattern with
// wildcards already added and special characters escaped.
type Query struct {
	OrgID, UserID string
	Text          string
	Pattern       string
	Limit         int
}

// Source searches one app. App is the launcher app id used for access checks
// ("" = always searched).
type Source struct {
	ID     string
	Label  string
	App    string
	Search func(ctx context.Context, q Query) ([]Result, error)
}

// Registry holds the sources.
type Registry struct {
	mu      sync.RWMutex
	sources []Source
}

// Register adds (or replaces) a source.
func (r *Registry) Register(s Source) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.sources {
		if r.sources[i].ID == s.ID {
			r.sources[i] = s
			return
		}
	}
	r.sources = append(r.sources, s)
}

// Group is the answer for one source.
type Group struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Results []Result `json:"results"`
}

// Escape makes text safe inside an ILIKE pattern.
func Escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(s))
}

// Search runs every source the caller may open, in parallel.
func (r *Registry) Search(ctx context.Context, orgID, userID, text string, limit int) []Group {
	text = strings.TrimSpace(text)
	if len(text) < 2 {
		return []Group{}
	}
	if limit <= 0 || limit > 20 {
		limit = 6
	}
	q := Query{OrgID: orgID, UserID: userID, Text: text, Pattern: "%" + Escape(text) + "%", Limit: limit}
	r.mu.RLock()
	srcs := append([]Source(nil), r.sources...)
	r.mu.RUnlock()
	out := make([]Group, len(srcs))
	var wg sync.WaitGroup
	for i, s := range srcs {
		if s.App != "" && !authz.CanOpenApp(ctx, s.App) {
			continue
		}
		wg.Add(1)
		go func(i int, s Source) {
			defer wg.Done()
			res, err := s.Search(ctx, q)
			if err == nil && len(res) > 0 {
				out[i] = Group{ID: s.ID, Label: s.Label, Results: res}
			}
		}(i, s)
	}
	wg.Wait()
	groups := []Group{}
	for _, g := range out {
		if len(g.Results) > 0 {
			groups = append(groups, g)
		}
	}
	sort.SliceStable(groups, func(a, b int) bool { return false })
	return groups
}

// Handler serves GET /api/v1/search/more?q=.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		u, ok := auth.UserFromContext(req.Context())
		o, ok2 := auth.OrgFromContext(req.Context())
		if !ok || !ok2 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"groups": r.Search(req.Context(), o.ID, u.ID, req.URL.Query().Get("q"), 6)})
	})
}

// RegisterBuiltins adds Chat, CRM, Tasks and People.
func (r *Registry) RegisterBuiltins(pool *pgxpool.Pool) {
	scan := func(ctx context.Context, sql string, args ...any) ([]Result, error) {
		rows, err := pool.Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []Result{}
		for rows.Next() {
			var x Result
			if err := rows.Scan(&x.ID, &x.Title, &x.Snippet, &x.URL); err != nil {
				return nil, err
			}
			out = append(out, x)
		}
		return out, rows.Err()
	}
	r.Register(Source{ID: "chat", Label: "Chat", App: "chat", Search: func(ctx context.Context, q Query) ([]Result, error) {
		return scan(ctx, `SELECT m.id::text, left(m.body, 80), m.sender_name || ' · ' || to_char(m.sent_at, 'DD Mon YYYY'), '/chat?channel=' || m.channel_id::text
FROM adrient.chat_messages m JOIN adrient.chat_channels c ON c.id=m.channel_id
WHERE m.org_id::text=$1 AND c.member_ids ? $2::text AND lower(m.body) LIKE $3 ESCAPE '\' ORDER BY m.sent_at DESC LIMIT $4`, q.OrgID, q.UserID, q.Pattern, q.Limit)
	}})
	r.Register(Source{ID: "crm", Label: "CRM", App: "crm", Search: func(ctx context.Context, q Query) ([]Result, error) {
		return scan(ctx, `SELECT * FROM (
SELECT d.id::text, d.title, 'Deal · ' || d.stage, '/crm/deals' FROM adrient.crm_deals d WHERE d.org_id::text=$1 AND (lower(d.title) LIKE $2 ESCAPE '\' OR lower(d.notes) LIKE $2 ESCAPE '\')
UNION ALL
SELECT c.id::text, c.name, 'Company', '/crm/companies' FROM adrient.crm_companies c WHERE c.org_id::text=$1 AND lower(c.name) LIKE $2 ESCAPE '\'
UNION ALL
SELECT p.id::text, ct.display_name, 'Person · ' || p.status, '/crm/people' FROM adrient.crm_people p JOIN adrient.contacts ct ON ct.id=p.contact_id
WHERE p.org_id::text=$1 AND lower(ct.display_name) LIKE $2 ESCAPE '\') x LIMIT $3`, q.OrgID, q.Pattern, q.Limit)
	}})
	r.Register(Source{ID: "tasks", Label: "Tasks", App: "tasks", Search: func(ctx context.Context, q Query) ([]Result, error) {
		return scan(ctx, `SELECT id::text, title, left(notes, 80), '/tasks' FROM adrient.tasks WHERE owner_user_id::text=$1 AND lower(title) LIKE $2 ESCAPE '\' ORDER BY created_at DESC LIMIT $3`, q.UserID, q.Pattern, q.Limit)
	}})
	r.Register(Source{ID: "people", Label: "People", Search: func(ctx context.Context, q Query) ([]Result, error) {
		return scan(ctx, `SELECT id::text, display_name, email || CASE WHEN job_title<>'' THEN ' · ' || job_title ELSE '' END, '/orgchart?person=' || id::text FROM adrient.users
WHERE org_id::text=$1 AND user_type='member' AND status='active' AND deleted_at IS NULL AND (lower(display_name) LIKE $2 ESCAPE '\' OR lower(email) LIKE $2 ESCAPE '\') ORDER BY display_name LIMIT $3`, q.OrgID, q.Pattern, q.Limit)
	}})
}
