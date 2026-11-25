// Package kittest gives the knowledge modules' tests a private database
// fixture. It never drops anything: each test creates its own org and users
// in the database named by ADRIENT_KNOW_TEST_DSN and the test is skipped when
// that variable is not set.
package kittest

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Fixture is one org with an admin user and a second ordinary user.
type Fixture struct {
	Pool  *pgxpool.Pool
	Org   string
	Admin knowkit.Actor
	User  knowkit.Actor
	Other knowkit.Actor // a user of a different org
}

// Setup connects, migrates and creates the fixture.
func Setup(t *testing.T) *Fixture {
	t.Helper()
	dsn := os.Getenv("ADRIENT_KNOW_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_KNOW_TEST_DSN not set; skipping integration test")
	}
	if cfg, err := pgxpool.ParseConfig(dsn); err != nil || cfg.ConnConfig.Database == "adrient" || cfg.ConnConfig.Database == "" {
		t.Fatalf("refusing to run against the shared dev database; create a scratch database and point ADRIENT_KNOW_TEST_DSN at it")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	mkOrg := func(tag string) string {
		var id string
		slug := fmt.Sprintf("kt-%s-%d", tag, time.Now().UnixNano())
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ($1,$1) RETURNING id::text`, slug).Scan(&id); err != nil {
			t.Fatalf("org: %v", err)
		}
		t.Cleanup(func() { cleanupOrg(pool, id) })
		return id
	}
	mkUser := func(org, name string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'test',$2,$3,$2) RETURNING id::text`,
			org, name+fmt.Sprint(time.Now().UnixNano()), name+"@example.test").Scan(&id); err != nil {
			t.Fatalf("user: %v", err)
		}
		return id
	}
	org := mkOrg("a")
	other := mkOrg("b")
	f := &Fixture{Pool: pool, Org: org}
	f.Admin = knowkit.Actor{Org: org, User: mkUser(org, "admin"), Admin: true}
	f.User = knowkit.Actor{Org: org, User: mkUser(org, "user")}
	f.Other = knowkit.Actor{Org: other, User: mkUser(other, "outsider")}
	return f
}

// cleanupOrg removes the fixture org's rows; tables with org_id cascade from the org.
func cleanupOrg(pool *pgxpool.Pool, org string) {
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM adrient.users WHERE org_id=$1`, org)
	_, _ = pool.Exec(ctx, `DELETE FROM adrient.orgs WHERE id=$1`, org)
}
