package orgs

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := testguard.DropSchema(ctx, t, pool); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return pool
}

func TestRepository_GetBySlug_FindsDefault(t *testing.T) {
	pool := setupDB(t)
	repo := NewRepository(pool)

	org, err := repo.GetBySlug(context.Background(), "default")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if org.Slug != "default" {
		t.Errorf("slug: got %q, want default", org.Slug)
	}
	if org.DisplayName != "Default" {
		t.Errorf("display_name: got %q, want Default", org.DisplayName)
	}
	if org.ID == "" {
		t.Errorf("id should be non-empty")
	}
}

func TestRepository_CreatePersonal(t *testing.T) {
	pool := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	o, err := repo.CreatePersonal(ctx, "Ada Lovelace")
	if err != nil {
		t.Fatalf("CreatePersonal: %v", err)
	}
	if o.ID == "" {
		t.Fatalf("personal org id empty")
	}
	if o.DisplayName != "Ada Lovelace" {
		t.Errorf("display_name = %q; want Ada Lovelace", o.DisplayName)
	}
	if len(o.Slug) < len("personal-") || o.Slug[:len("personal-")] != "personal-" {
		t.Errorf("slug = %q; want personal-<...>", o.Slug)
	}

	// Two personal orgs get distinct slugs/ids.
	o2, err := repo.CreatePersonal(ctx, "")
	if err != nil {
		t.Fatalf("CreatePersonal #2: %v", err)
	}
	if o2.Slug == o.Slug || o2.ID == o.ID {
		t.Errorf("second personal org collides: %q vs %q", o2.Slug, o.Slug)
	}
	if o2.DisplayName != "Personal workspace" {
		t.Errorf("empty name default = %q; want Personal workspace", o2.DisplayName)
	}

	// Round-trips by id.
	got, err := repo.GetByID(ctx, o.ID)
	if err != nil || got.Slug != o.Slug {
		t.Fatalf("GetByID = %+v, %v", got, err)
	}
}

func TestRepository_GetBySlug_NotFound(t *testing.T) {
	pool := setupDB(t)
	repo := NewRepository(pool)

	_, err := repo.GetBySlug(context.Background(), "missing")
	if err != ErrNotFound {
		t.Errorf("got err=%v, want ErrNotFound", err)
	}
}

// Create makes the creator both admin and Owner.
func TestRepository_Create_CreatorIsOwner(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	var def string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&def); err != nil {
		t.Fatal(err)
	}
	var uid string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		VALUES ($1,'https://issuer.test','creator','c@test','c@test') RETURNING id::text`, def).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	o, err := NewRepository(pool).Create(ctx, "acme", "Acme", uid)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id
		WHERE ra.org_id=$1 AND ra.subject_id=$2 AND r.key='owner'`, o.ID, uid).Scan(&n); err != nil || n != 1 {
		t.Fatalf("owner assignments = %d, %v; want 1", n, err)
	}
}
