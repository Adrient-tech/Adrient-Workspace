// Package testguard keeps integration tests away from the dev databases.
package testguard

import (
	"context"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Skipper is the part of *testing.T the guard needs.
type Skipper interface {
	Helper()
	Skipf(format string, args ...any)
}

// Protected database names: tests drop the whole schema, so they must never run here.
var Protected = map[string]bool{"adrient": true, "zitadel": true}

// DBName returns the database name of a connection URL or key=value DSN.
func DBName(dsn string) string {
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" {
		return strings.TrimPrefix(u.Path, "/")
	}
	for _, f := range strings.Fields(dsn) {
		if v, ok := strings.CutPrefix(f, "dbname="); ok {
			return v
		}
	}
	return ""
}

// DropSchema drops the adrient schema of a scratch database. It skips the test, and drops
// nothing, when the pool points at a protected dev database.
func DropSchema(ctx context.Context, t Skipper, pool *pgxpool.Pool) (pgconn.CommandTag, error) {
	t.Helper()
	name := DBName(pool.Config().ConnString())
	if cfg := pool.Config().ConnConfig; cfg != nil && cfg.Database != "" {
		name = cfg.Database
	}
	if Protected[name] {
		t.Skipf("refusing to run: database %q is a dev database and this test drops its schema. Point the test DSN at a scratch database (for example w6fin_test).", name)
		return pgconn.CommandTag{}, nil
	}
	return pool.Exec(ctx, "DROP SCHEMA IF EXISTS adrient CASCADE")
}
