// Package scim is a SCIM 2.0 (RFC 7643/7644) server so Microsoft Entra ID,
// Okta and Google Workspace can provision users and groups into an
// organization. Requests authenticate with an org-scoped bearer token created
// in the admin console; every query is scoped to that token's organization.
package scim

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Schemas.
const (
	SchemaUser       = "urn:ietf:params:scim:schemas:core:2.0:User"
	SchemaGroup      = "urn:ietf:params:scim:schemas:core:2.0:Group"
	SchemaEnterprise = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	SchemaList       = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	SchemaError      = "urn:ietf:params:scim:api:messages:2.0:Error"
	SchemaPatch      = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
)

// PreprovisionedIssuer marks users who exist before they first sign in. The
// first verified sign-in with the same e-mail adopts the row (see
// users.ClaimPreprovisioned).
const PreprovisionedIssuer = "adrient:preprovisioned"

// Token is a SCIM bearer token row.
type Token struct {
	ID         string     `json:"id"`
	OrgID      string     `json:"-"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	RatePerMin int        `json:"rate_per_min"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// Store reads and writes SCIM state.
type Store struct {
	Pool *pgxpool.Pool

	mu   sync.Mutex
	hits map[string][]time.Time
	now  func() time.Time
}

func (s *Store) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

const tokenPrefix = "scim_"

// CreateToken issues a token; the plaintext is returned once.
func (s *Store) CreateToken(ctx context.Context, orgID, userID, name string, rate int, expires *time.Time) (string, Token, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return "", Token{}, errors.New("name is required (up to 80 characters)")
	}
	if rate == 0 {
		rate = 600
	}
	if rate < 10 || rate > 100000 {
		return "", Token{}, errors.New("rate limit must be between 10 and 100000 requests per minute")
	}
	raw := make([]byte, 28)
	if _, err := rand.Read(raw); err != nil {
		return "", Token{}, err
	}
	plain := tokenPrefix + hex.EncodeToString(raw)
	var t Token
	err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.scim_tokens (org_id, name, prefix, token_hash, rate_per_min, created_by, expires_at)
VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7)
RETURNING id::text, name, prefix, rate_per_min, created_at, last_used_at, expires_at, revoked_at`,
		orgID, name, plain[:11], hashToken(plain), rate, userID, expires).
		Scan(&t.ID, &t.Name, &t.Prefix, &t.RatePerMin, &t.CreatedAt, &t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt)
	return plain, t, err
}

// ListTokens lists an org's tokens (never the secret).
func (s *Store) ListTokens(ctx context.Context, orgID string) ([]Token, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, name, prefix, rate_per_min, created_at, last_used_at, expires_at, revoked_at
FROM adrient.scim_tokens WHERE org_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.RatePerMin, &t.CreatedAt, &t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken revokes a token of orgID.
func (s *Store) RevokeToken(ctx context.Context, orgID, id string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.scim_tokens SET revoked_at=now() WHERE id::text=$1 AND org_id=$2 AND revoked_at IS NULL`, id, orgID)
	return tag.RowsAffected() > 0, err
}

// ErrBadToken is returned for unknown, revoked or expired tokens.
var ErrBadToken = errors.New("invalid token")

// Authenticate resolves a bearer token to its org.
func (s *Store) Authenticate(ctx context.Context, plain string) (Token, error) {
	if !strings.HasPrefix(plain, tokenPrefix) {
		return Token{}, ErrBadToken
	}
	var t Token
	err := s.Pool.QueryRow(ctx, `SELECT id::text, org_id::text, name, prefix, rate_per_min, created_at, last_used_at, expires_at, revoked_at
FROM adrient.scim_tokens WHERE token_hash=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())`, hashToken(plain)).
		Scan(&t.ID, &t.OrgID, &t.Name, &t.Prefix, &t.RatePerMin, &t.CreatedAt, &t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrBadToken
	}
	if err != nil {
		return Token{}, err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.scim_tokens SET last_used_at=now() WHERE id::text=$1`, t.ID)
	return t, nil
}

// Allow applies the token's per-minute rate limit (sliding window).
// It returns whether the request may proceed and how long to wait otherwise.
func (s *Store) Allow(t Token) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hits == nil {
		s.hits = map[string][]time.Time{}
	}
	now := s.clock()
	cut := now.Add(-time.Minute)
	l := s.hits[t.ID][:0]
	for _, x := range s.hits[t.ID] {
		if x.After(cut) {
			l = append(l, x)
		}
	}
	if len(l) >= t.RatePerMin {
		s.hits[t.ID] = l
		wait := l[0].Add(time.Minute).Sub(now)
		if wait < time.Second {
			wait = time.Second
		}
		return false, wait
	}
	s.hits[t.ID] = append(l, now)
	return true, 0
}

// Event is a request-log row.
type Event struct {
	ID        int64     `json:"id"`
	TokenID   string    `json:"token_id"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// LogEvent records a request (best effort) and trims old rows now and then.
func (s *Store) LogEvent(ctx context.Context, orgID, tokenID, method, path string, status int, detail string) {
	if len(detail) > 300 {
		detail = detail[:300]
	}
	_, _ = s.Pool.Exec(ctx, `INSERT INTO adrient.scim_events (org_id, token_id, method, path, status, detail) VALUES ($1,NULLIF($2,'')::uuid,$3,$4,$5,$6)`,
		orgID, tokenID, method, path, status, detail)
	if time.Now().UnixNano()%97 == 0 {
		_, _ = s.Pool.Exec(ctx, `DELETE FROM adrient.scim_events WHERE org_id=$1 AND id < (SELECT COALESCE(max(id),0) - 2000 FROM adrient.scim_events WHERE org_id=$1)`, orgID)
	}
}

// Events returns the newest requests of an org.
func (s *Store) Events(ctx context.Context, orgID string, limit int) ([]Event, error) {
	return s.EventsBefore(ctx, orgID, limit, 0)
}

// EventsBefore pages the request log: rows with id below beforeID (0 = newest).
func (s *Store) EventsBefore(ctx context.Context, orgID string, limit int, beforeID int64) ([]Event, error) {
	if limit <= 0 || limit > 501 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx, `SELECT id, COALESCE(token_id::text,''), method, path, status, detail, created_at
FROM adrient.scim_events WHERE org_id=$1 AND ($3 = 0 OR id < $3) ORDER BY id DESC LIMIT $2`, orgID, limit, beforeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.TokenID, &e.Method, &e.Path, &e.Status, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func fmtErr(format string, a ...any) error { return fmt.Errorf(format, a...) }
