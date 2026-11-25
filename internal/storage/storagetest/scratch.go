// Package storagetest creates throwaway, fully migrated Postgres databases for
// integration tests. It never touches an existing database: each call creates
// a fresh one named adrient_test_<n> on the server in ADRIENT_TEST_ADMIN_DSN
// and drops it when the test ends.
package storagetest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ScratchDB returns a pool on a new migrated database, or skips the test when
// ADRIENT_TEST_ADMIN_DSN is unset (e.g. postgres://adrient@127.0.0.1:5533/postgres).
func ScratchDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	admin := os.Getenv("ADRIENT_TEST_ADMIN_DSN")
	if admin == "" {
		t.Skip("ADRIENT_TEST_ADMIN_DSN not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	ap, err := pgxpool.New(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("adrient_test_%d", time.Now().UnixNano())
	if _, err := ap.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		ap.Close()
		t.Fatalf("create scratch database: %v", err)
	}
	u, err := url.Parse(admin)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = ap.Exec(context.Background(), "DROP "+"DATABASE IF EXISTS "+name+" WITH (FORCE)")
		ap.Close()
	})
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatalf("migrate scratch database: %v", err)
	}
	return pool
}
