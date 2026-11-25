package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Extras serves per-user saved searches and signatures over plain HTTP JSON
// (no proto change needed): /api/v1/mail/saved-searches and /api/v1/mail/signatures.
type Extras struct{ pool *pgxpool.Pool }

// NewExtras constructs the handler.
func NewExtras(pool *pgxpool.Pool) *Extras { return &Extras{pool: pool} }

// SavedSearch is a named search query (operators allowed).
type SavedSearch struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Query string `json:"query"`
}

// Signature is a reusable text signature; at most one is the default.
type Signature struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Body      string `json:"body"`
	IsDefault bool   `json:"is_default"`
}

const (
	maxNameLen  = 80
	maxQueryLen = 300
	maxSigLen   = 4000
	maxPerUser  = 50
)

// Match reports whether the path belongs to this handler.
func (e *Extras) Match(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == "/api/v1/mail/saved-searches" || p == "/api/v1/mail/signatures" ||
		strings.HasPrefix(p, "/api/v1/mail/saved-searches/") || strings.HasPrefix(p, "/api/v1/mail/signatures/")
}

func writeExtrasJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ServeHTTP routes requests; the caller must already have authenticated the session.
func (e *Extras) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	o, ook := auth.OrgFromContext(r.Context())
	if !ok || !ook {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	p := strings.TrimRight(r.URL.Path, "/")
	switch {
	case strings.HasPrefix(p, "/api/v1/mail/saved-searches"):
		id := strings.TrimPrefix(strings.TrimPrefix(p, "/api/v1/mail/saved-searches"), "/")
		e.searches(w, r, o.ID, u.ID, id)
	default:
		id := strings.TrimPrefix(strings.TrimPrefix(p, "/api/v1/mail/signatures"), "/")
		e.signatures(w, r, o.ID, u.ID, id)
	}
}

func (e *Extras) searches(w http.ResponseWriter, r *http.Request, orgID, userID, id string) {
	ctx := r.Context()
	switch {
	case r.Method == http.MethodGet && id == "":
		list, err := e.ListSearches(ctx, userID)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		writeExtrasJSON(w, 200, map[string]any{"searches": list})
	case r.Method == http.MethodPost && id == "":
		var in SavedSearch
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in) != nil {
			http.Error(w, "bad json", 400)
			return
		}
		out, err := e.SaveSearch(ctx, orgID, userID, in.Name, in.Query)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeExtrasJSON(w, 200, out)
	case r.Method == http.MethodDelete && id != "":
		if _, err := e.pool.Exec(ctx, `DELETE FROM adrient.mail_saved_searches WHERE id=$1 AND user_id=$2`, id, userID); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		writeExtrasJSON(w, 200, map[string]any{})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

// ListSearches returns the user's saved searches by name.
func (e *Extras) ListSearches(ctx context.Context, userID string) ([]SavedSearch, error) {
	rows, err := e.pool.Query(ctx, `SELECT id::text, name, query FROM adrient.mail_saved_searches WHERE user_id=$1 ORDER BY lower(name)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SavedSearch{}
	for rows.Next() {
		var s SavedSearch
		if err := rows.Scan(&s.ID, &s.Name, &s.Query); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ValidateSearch trims and validates a saved-search name and query.
func ValidateSearch(name, query string) (string, string, error) {
	name, query = strings.TrimSpace(name), strings.TrimSpace(query)
	if name == "" || query == "" {
		return "", "", errValidation("name and query are required")
	}
	if len(name) > maxNameLen || len(query) > maxQueryLen {
		return "", "", errValidation("name or query too long")
	}
	return name, query, nil
}

type errValidation string

func (e errValidation) Error() string { return string(e) }

// SaveSearch upserts a saved search by name.
func (e *Extras) SaveSearch(ctx context.Context, orgID, userID, name, query string) (SavedSearch, error) {
	name, query, err := ValidateSearch(name, query)
	if err != nil {
		return SavedSearch{}, err
	}
	var n int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.mail_saved_searches WHERE user_id=$1 AND name<>$2`, userID, name).Scan(&n)
	if n >= maxPerUser {
		return SavedSearch{}, errValidation("too many saved searches")
	}
	var s SavedSearch
	err = e.pool.QueryRow(ctx, `INSERT INTO adrient.mail_saved_searches (org_id,user_id,name,query) VALUES ($1,$2,$3,$4)
		ON CONFLICT (user_id,name) DO UPDATE SET query=EXCLUDED.query
		RETURNING id::text, name, query`, orgID, userID, name, query).Scan(&s.ID, &s.Name, &s.Query)
	return s, err
}

func (e *Extras) signatures(w http.ResponseWriter, r *http.Request, orgID, userID, id string) {
	ctx := r.Context()
	switch {
	case r.Method == http.MethodGet && id == "":
		list, err := e.ListSignatures(ctx, userID)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		writeExtrasJSON(w, 200, map[string]any{"signatures": list})
	case r.Method == http.MethodPost && id == "":
		var in Signature
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil {
			http.Error(w, "bad json", 400)
			return
		}
		out, err := e.SaveSignature(ctx, orgID, userID, in)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeExtrasJSON(w, 200, out)
	case r.Method == http.MethodDelete && id != "":
		if _, err := e.pool.Exec(ctx, `DELETE FROM adrient.mail_signatures WHERE id=$1 AND user_id=$2`, id, userID); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		writeExtrasJSON(w, 200, map[string]any{})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

// ListSignatures returns the user's signatures, default first.
func (e *Extras) ListSignatures(ctx context.Context, userID string) ([]Signature, error) {
	rows, err := e.pool.Query(ctx, `SELECT id::text, name, body, is_default FROM adrient.mail_signatures WHERE user_id=$1 ORDER BY is_default DESC, lower(name)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Signature{}
	for rows.Next() {
		var s Signature
		if err := rows.Scan(&s.ID, &s.Name, &s.Body, &s.IsDefault); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveSignature upserts a signature by name; making it default clears the others.
func (e *Extras) SaveSignature(ctx context.Context, orgID, userID string, in Signature) (Signature, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > maxNameLen || strings.TrimSpace(in.Body) == "" || len(in.Body) > maxSigLen {
		return Signature{}, errValidation("name and body are required (body max 4000 chars)")
	}
	tx, err := e.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Signature{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if in.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE adrient.mail_signatures SET is_default=false WHERE user_id=$1`, userID); err != nil {
			return Signature{}, err
		}
	}
	var s Signature
	err = tx.QueryRow(ctx, `INSERT INTO adrient.mail_signatures (org_id,user_id,name,body,is_default) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (user_id,name) DO UPDATE SET body=EXCLUDED.body, is_default=EXCLUDED.is_default
		RETURNING id::text, name, body, is_default`, orgID, userID, in.Name, in.Body, in.IsDefault).Scan(&s.ID, &s.Name, &s.Body, &s.IsDefault)
	if err != nil {
		return Signature{}, err
	}
	return s, tx.Commit(ctx)
}
