// Package opstest is the database fixture for the integration tests of the
// wave-6 ops modules. It only ever runs against a database whose DSN contains
// "test" (it recreates the adrient schema), and skips when ADRIENT_TEST_DSN is unset.
package opstest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

// DB connects, recreates the schema, runs the migrations and returns the pool
// with the default org and one admin user.
func DB(t *testing.T) (pool *pgxpool.Pool, org, user string) {
	t.Helper()
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set; skipping integration test")
	}
	if !strings.Contains(dsn, "test") {
		t.Fatalf("refusing to recreate the schema of a database that is not a test database: %q", dsn)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS adrient CASCADE"); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	user = User(t, pool, org, "admin")
	return pool, org, user
}

// User inserts a user and returns its id.
func User(t *testing.T, pool *pgxpool.Pool, org, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		VALUES ($1,'test',$2,$3,$4) RETURNING id::text`, org, "sub-"+name, name+"@adrient.localtest.me", name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Employee inserts an active HR employee (optionally linked to a user) and returns its id.
func Employee(t *testing.T, pool *pgxpool.Pool, org, userID, first string, managerID string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `INSERT INTO adrient.hr_employees (org_id, user_id, emp_code, first_name, last_name, work_email, join_date, manager_id)
		VALUES ($1, NULLIF($2,'')::uuid, $3, $4, 'Test', $5, '2024-01-01', NULLIF($6,'')::uuid) RETURNING id::text`,
		org, userID, "T-"+strings.ToUpper(first), first, strings.ToLower(first)+"@x.test", managerID).Scan(&id)
	if err != nil {
		t.Fatal(fmt.Sprint("employee: ", err))
	}
	return id
}
