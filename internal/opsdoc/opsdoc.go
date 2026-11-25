// Package opsdoc is the small org-scoped record store shared by the wave-6 ops
// modules (Time tracking, Field service, Fleet). Each module owns one table
// (id, org_id, kind, ref, day, data JSONB); the modules keep their typed
// structs and validation in Go and store them as JSON, so every query is
// parameterised and org-scoped in one place.
package opsdoc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

var tableRe = regexp.MustCompile(`^[a-z][a-z_]{2,30}$`)

// Doc is one stored record.
type Doc struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Ref       string          `json:"ref"`
	Day       string          `json:"day"`
	Data      json.RawMessage `json:"data"`
	CreatedBy string          `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Filter narrows List. Empty fields do not filter.
type Filter struct {
	Ref   string
	From  string // inclusive YYYY-MM-DD on Day
	To    string
	Limit int
}

// Store reads and writes one module's table.
type Store struct {
	pool  *pgxpool.Pool
	table string
}

// New returns a store for table (already created by a migration).
func New(pool *pgxpool.Pool, table string) *Store {
	if !tableRe.MatchString(table) {
		panic("opsdoc: bad table name " + table)
	}
	return &Store{pool: pool, table: table}
}

const cols = `id::text, kind, ref, coalesce(to_char(day,'YYYY-MM-DD'),''), data, coalesce(created_by::text,''), created_at, updated_at`

func scan(row pgx.Row) (Doc, error) {
	var d Doc
	err := row.Scan(&d.ID, &d.Kind, &d.Ref, &d.Day, &d.Data, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// List returns the records of a kind, newest day first.
func (s *Store) List(ctx context.Context, org, kind string, f Filter) ([]Doc, error) {
	lim := f.Limit
	if lim <= 0 || lim > 5000 {
		lim = 5000
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`SELECT %s FROM adrient.%s WHERE org_id=$1 AND kind=$2
		AND ($3='' OR ref=$3) AND ($4='' OR day >= $4::date) AND ($5='' OR day <= $5::date)
		ORDER BY day DESC NULLS LAST, created_at DESC LIMIT $6`, cols, s.table), org, kind, f.Ref, f.From, f.To, lim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Doc{}
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Get returns one record of the kind.
func (s *Store) Get(ctx context.Context, org, kind, id string) (Doc, error) {
	if !opshttp.IsUUID(id) {
		return Doc{}, opshttp.ErrNotFound
	}
	d, err := scan(s.pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM adrient.%s WHERE org_id=$1 AND kind=$2 AND id=$3::uuid`, cols, s.table), org, kind, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, opshttp.ErrNotFound
	}
	return d, err
}

// Put inserts (id == "") or replaces the record.
func (s *Store) Put(ctx context.Context, org, user, kind, id, ref, day string, data any) (Doc, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return Doc{}, err
	}
	if day != "" && !opshttp.ValidDate(day) {
		return Doc{}, opshttp.Invalid("bad date %q", day)
	}
	if id == "" {
		return scan(s.pool.QueryRow(ctx, fmt.Sprintf(`INSERT INTO adrient.%s (org_id, kind, ref, day, data, created_by)
			VALUES ($1,$2,$3,NULLIF($4,'')::date,$5::jsonb,NULLIF($6,'')::uuid) RETURNING %s`, s.table, cols), org, kind, ref, day, raw, user))
	}
	if !opshttp.IsUUID(id) {
		return Doc{}, opshttp.ErrNotFound
	}
	d, err := scan(s.pool.QueryRow(ctx, fmt.Sprintf(`UPDATE adrient.%s SET ref=$4, day=NULLIF($5,'')::date, data=$6::jsonb, updated_at=now()
		WHERE org_id=$1 AND kind=$2 AND id=$3::uuid RETURNING %s`, s.table, cols), org, kind, id, ref, day, raw))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, opshttp.ErrNotFound
	}
	return d, err
}

// Delete removes one record.
func (s *Store) Delete(ctx context.Context, org, kind, id string) error {
	if !opshttp.IsUUID(id) {
		return opshttp.ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, fmt.Sprintf(`DELETE FROM adrient.%s WHERE org_id=$1 AND kind=$2 AND id=$3::uuid`, s.table), org, kind, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return opshttp.ErrNotFound
	}
	return nil
}

// Decode unmarshals a record's data into v and sets the id through set.
func Decode[T any](d Doc, set func(*T, Doc)) (T, error) {
	var v T
	if err := json.Unmarshal(d.Data, &v); err != nil {
		return v, err
	}
	if set != nil {
		set(&v, d)
	}
	return v, nil
}

// DecodeAll decodes every record of a list.
func DecodeAll[T any](docs []Doc, set func(*T, Doc)) []T {
	out := make([]T, 0, len(docs))
	for _, d := range docs {
		if v, err := Decode(d, set); err == nil {
			out = append(out, v)
		}
	}
	return out
}
