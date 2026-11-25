// Package audit is the cross-cutting activity trail for adrient-workspace. It
// records mutating actions across every built-in service — gRPC RPCs (via a
// unary interceptor) and raw upload/download/stream HTTP routes (via a handler
// middleware) — into adrient.audit_events, and exposes an admin-gated viewer.
//
// The package is intentionally dependency-light: pure database/SQL (pgx) +
// net/http + the auth context helpers. It does NOT import the generated protos
// (gen/), so it builds standalone, exactly like internal/adminusers.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Event is one row of adrient.audit_events. Fields left zero are stored as their
// SQL defaults (empty string / '{}' / NULL actor).
type Event struct {
	ID           string         // set by the DB; populated on read
	OrgID        string         // required: events with an empty org are dropped
	ActorID      string         // adrient user id; "" → NULL (unauthenticated)
	ActorEmail   string         // denormalized caller email
	Service      string         // derived slug, e.g. "video"
	Action       string         // derived verb, e.g. "create"
	ResourceType string         // e.g. "video", "file"
	ResourceID   string         // affected resource id, when known
	Method       string         // full gRPC method or "HTTPVERB path"
	Status       string         // "ok" | "error"
	Detail       map[string]any // free-form context (gRPC code, http status…)
	IP           string         // client IP (X-Forwarded-For / RemoteAddr)
	UserAgent    string         // client user agent
	CreatedAt    time.Time      // set by the DB; populated on read
}

// Filter narrows a List query. Zero-value fields are ignored.
type Filter struct {
	Service    string    // exact service slug
	ActorEmail string    // exact actor email (case-sensitive match on stored value)
	Action     string    // exact action verb
	Denied     bool      // only permission_denied events
	Limit      int       // max rows (clamped to [1,500], default 100)
	Before     time.Time // keyset: only events strictly older than this

	ActorID    string    // exact actor user id
	Status     string    // "ok" | "error"
	ResourceID string    // exact resource id
	From       time.Time // only events at or after this time
	To         time.Time // only events at or before this time (jump to date)
	CursorTime time.Time // stable keyset: events strictly after (CursorTime, CursorID) in newest-first order
	CursorID   string
	Asc        bool // oldest first (used by live tail, which asks for newer rows)
}

// Repository reads and writes audit events.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository over the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Insert appends one audit event. An empty OrgID is rejected (audit rows are
// org-scoped). An empty ActorID is stored as NULL.
func (r *Repository) Insert(ctx context.Context, e Event) error {
	if r == nil || r.pool == nil {
		return nil
	}
	if e.OrgID == "" {
		return fmt.Errorf("audit.Insert: empty org id")
	}
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	b, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("audit.Insert marshal detail: %w", err)
	}
	var actor *string
	if e.ActorID != "" {
		actor = &e.ActorID
	}
	const q = `INSERT INTO adrient.audit_events
		(org_id, actor_id, actor_email, service, action, resource_type,
		 resource_id, method, status, detail, ip, user_agent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`
	_, err = r.pool.Exec(ctx, q,
		e.OrgID, actor, e.ActorEmail, e.Service, e.Action, e.ResourceType,
		e.ResourceID, e.Method, e.Status, b, e.IP, e.UserAgent)
	if err != nil {
		return fmt.Errorf("audit.Insert: %w", err)
	}
	return nil
}

// List returns an org's events newest-first, applying the filter. Limit is
// clamped to [1,500] (default 100). The Before field drives keyset pagination.
func (r *Repository) List(ctx context.Context, orgID string, f Filter) ([]Event, error) {
	if r == nil || r.pool == nil {
		return nil, nil
	}
	if orgID == "" {
		return nil, fmt.Errorf("audit.List: empty org id")
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	// Build a parameterized WHERE incrementally so optional filters don't bloat
	// the plan and so we stay injection-safe.
	where, args := f.where(orgID)
	args = append(args, limit)
	dir := "DESC"
	if f.Asc {
		dir = "ASC"
	}
	q := fmt.Sprintf(`SELECT id, org_id, actor_id, actor_email, service, action,
			resource_type, resource_id, method, status, detail, ip, user_agent, created_at
		FROM adrient.audit_events
		WHERE %s
		ORDER BY created_at %s, id %s
		LIMIT $%d`, where, dir, dir, len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("audit.List: %w", err)
	}
	defer rows.Close()

	out := make([]Event, 0, limit)
	for rows.Next() {
		var (
			e       Event
			actor   *string
			detailB []byte
		)
		if err := rows.Scan(&e.ID, &e.OrgID, &actor, &e.ActorEmail, &e.Service,
			&e.Action, &e.ResourceType, &e.ResourceID, &e.Method, &e.Status,
			&detailB, &e.IP, &e.UserAgent, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("audit.List scan: %w", err)
		}
		if actor != nil {
			e.ActorID = *actor
		}
		if len(detailB) > 0 {
			_ = json.Unmarshal(detailB, &e.Detail)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// where builds the parameterised WHERE clause shared by List, Count and Each.
func (f Filter) where(orgID string) (string, []any) {
	args := []any{orgID}
	where := "org_id = $1"
	add := func(cond string, val any) {
		args = append(args, val)
		where += fmt.Sprintf(" AND %s $%d", cond, len(args))
	}
	if f.Service != "" {
		add("service =", f.Service)
	}
	if f.ActorEmail != "" {
		add("actor_email =", f.ActorEmail)
	}
	if f.ActorID != "" {
		add("actor_id =", f.ActorID)
	}
	if f.Action != "" {
		add("action =", f.Action)
	}
	if f.Denied {
		add("action =", ActionDenied)
	}
	if f.Status != "" {
		add("status =", f.Status)
	}
	if f.ResourceID != "" {
		add("resource_id =", f.ResourceID)
	}
	if !f.Before.IsZero() {
		add("created_at <", f.Before)
	}
	if !f.From.IsZero() {
		add("created_at >=", f.From)
	}
	if !f.To.IsZero() {
		add("created_at <=", f.To)
	}
	if !f.CursorTime.IsZero() && f.CursorID != "" {
		args = append(args, f.CursorTime, f.CursorID)
		op := "<"
		if f.Asc {
			op = ">"
		}
		where += fmt.Sprintf(" AND (created_at, id) %s ($%d, $%d::uuid)", op, len(args)-1, len(args))
	}
	return where, args
}

// CountCapped counts matching events, stopping at limit+1 rows so a huge log
// never costs a full scan. capped is true when the real count is above limit.
func (r *Repository) CountCapped(ctx context.Context, orgID string, f Filter, limit int) (n int64, capped bool, err error) {
	if r == nil || r.pool == nil {
		return 0, false, nil
	}
	where, args := f.where(orgID)
	q := fmt.Sprintf(`SELECT count(*) FROM (SELECT 1 FROM adrient.audit_events WHERE %s LIMIT %d) c`, where, limit+1)
	if err = r.pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		return 0, false, fmt.Errorf("audit.CountCapped: %w", err)
	}
	if n > int64(limit) {
		return int64(limit), true, nil
	}
	return n, false, nil
}

// Each streams every matching event newest-first to fn, in keyset batches, so
// an export of millions of rows never holds more than one batch in memory.
// maxRows bounds the export (<=0: 1,000,000).
func (r *Repository) Each(ctx context.Context, orgID string, f Filter, maxRows int, fn func(Event) error) error {
	if maxRows <= 0 {
		maxRows = 1000000
	}
	f.Limit = 500
	sent := 0
	for sent < maxRows {
		batch, err := r.List(ctx, orgID, f)
		if err != nil {
			return err
		}
		for _, e := range batch {
			if sent >= maxRows {
				return nil
			}
			if err := fn(e); err != nil {
				return err
			}
			sent++
		}
		if len(batch) < 500 {
			return nil
		}
		last := batch[len(batch)-1]
		f.CursorTime, f.CursorID = last.CreatedAt, last.ID
	}
	return nil
}
