package orgadmin_test

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/orgadmin"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

// setupDB drops and recreates the adrient schema, runs migrations, and returns the
// pool plus the default org id. Skips when ADRIENT_TEST_DSN is unset.
func setupDB(t *testing.T) (*pgxpool.Pool, string) {
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
	var orgID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&orgID); err != nil {
		t.Fatalf("default org: %v", err)
	}
	return pool, orgID
}

// seedUser inserts a adrient user in orgID with the given subject/email and returns
// its id.
func seedUser(t *testing.T, pool *pgxpool.Pool, orgID, subject, email string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		 VALUES ($1, 'https://issuer.test', $2, $3, $3)
		 RETURNING id::text`,
		orgID, subject, email,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

func TestGrantListRevokeCount(t *testing.T) {
	pool, orgID := setupDB(t)
	ctx := context.Background()
	repo := orgadmin.NewRepository(pool)

	alice := seedUser(t, pool, orgID, "z-alice", "alice@test")
	bob := seedUser(t, pool, orgID, "z-bob", "bob@test")

	// Nobody is an admin yet.
	if got, err := repo.CountAdmins(ctx, orgID); err != nil || got != 0 {
		t.Fatalf("CountAdmins start = %d, %v; want 0", got, err)
	}
	if ok, err := repo.IsAdmin(ctx, orgID, alice); err != nil || ok {
		t.Fatalf("IsAdmin(alice) = %v, %v; want false", ok, err)
	}

	// Grant alice (granted_by nil), then bob granted_by alice.
	if err := repo.GrantAdmin(ctx, orgID, alice, ""); err != nil {
		t.Fatalf("GrantAdmin(alice): %v", err)
	}
	if err := repo.GrantAdmin(ctx, orgID, bob, alice); err != nil {
		t.Fatalf("GrantAdmin(bob): %v", err)
	}

	if ok, err := repo.IsAdmin(ctx, orgID, alice); err != nil || !ok {
		t.Fatalf("IsAdmin(alice) = %v, %v; want true", ok, err)
	}
	if got, err := repo.CountAdmins(ctx, orgID); err != nil || got != 2 {
		t.Fatalf("CountAdmins = %d, %v; want 2", got, err)
	}

	admins, err := repo.ListAdmins(ctx, orgID)
	if err != nil {
		t.Fatalf("ListAdmins: %v", err)
	}
	if len(admins) != 2 {
		t.Fatalf("ListAdmins = %v; want 2 ids", admins)
	}
	// Ordered by granted_at: alice first.
	if admins[0] != alice {
		t.Fatalf("ListAdmins[0] = %s; want alice %s", admins[0], alice)
	}

	// Re-granting is idempotent (no error, no duplicate).
	if err := repo.GrantAdmin(ctx, orgID, alice, ""); err != nil {
		t.Fatalf("re-GrantAdmin(alice): %v", err)
	}
	if got, _ := repo.CountAdmins(ctx, orgID); got != 2 {
		t.Fatalf("CountAdmins after re-grant = %d; want 2", got)
	}

	// Revoke bob → 1 admin.
	if err := repo.RevokeAdmin(ctx, orgID, bob); err != nil {
		t.Fatalf("RevokeAdmin(bob): %v", err)
	}
	if got, _ := repo.CountAdmins(ctx, orgID); got != 1 {
		t.Fatalf("CountAdmins after revoke = %d; want 1", got)
	}
	if ok, _ := repo.IsAdmin(ctx, orgID, bob); ok {
		t.Fatalf("IsAdmin(bob) after revoke = true; want false")
	}

	// Revoking a non-admin is a no-op (no error).
	if err := repo.RevokeAdmin(ctx, orgID, bob); err != nil {
		t.Fatalf("RevokeAdmin(non-admin): %v", err)
	}
}

func TestEnsureFirstAdmin(t *testing.T) {
	pool, orgID := setupDB(t)
	ctx := context.Background()
	repo := orgadmin.NewRepository(pool)

	alice := seedUser(t, pool, orgID, "z-alice", "alice@test")
	bob := seedUser(t, pool, orgID, "z-bob", "bob@test")

	// First call grants (org had no admins).
	granted, err := repo.EnsureFirstAdmin(ctx, orgID, alice)
	if err != nil || !granted {
		t.Fatalf("EnsureFirstAdmin(alice) = %v, %v; want true", granted, err)
	}
	// Second call for a different user is a no-op (org already has an admin).
	granted, err = repo.EnsureFirstAdmin(ctx, orgID, bob)
	if err != nil || granted {
		t.Fatalf("EnsureFirstAdmin(bob) = %v, %v; want false", granted, err)
	}
	if got, _ := repo.CountAdmins(ctx, orgID); got != 1 {
		t.Fatalf("CountAdmins = %d; want 1 (only alice)", got)
	}
	if ok, _ := repo.IsAdmin(ctx, orgID, bob); ok {
		t.Fatalf("bob should not be admin")
	}
}

func TestAdminUserIDsForZitadel(t *testing.T) {
	pool, orgID := setupDB(t)
	ctx := context.Background()
	repo := orgadmin.NewRepository(pool)
	const issuer = "https://issuer.test"

	alice := seedUser(t, pool, orgID, "z-alice", "alice@test")
	_ = seedUser(t, pool, orgID, "z-bob", "bob@test")

	if err := repo.GrantAdmin(ctx, orgID, alice, ""); err != nil {
		t.Fatalf("GrantAdmin: %v", err)
	}

	// Query for both Zitadel ids plus an unknown one.
	got, err := repo.AdminUserIDsForZitadel(ctx, orgID, issuer, []string{"z-alice", "z-bob", "z-ghost"})
	if err != nil {
		t.Fatalf("AdminUserIDsForZitadel: %v", err)
	}
	if !got["z-alice"] {
		t.Fatalf("expected z-alice admin")
	}
	if got["z-bob"] {
		t.Fatalf("z-bob is not an admin")
	}
	if got["z-ghost"] {
		t.Fatalf("z-ghost has no adrient user")
	}
}

func hasOwner(t *testing.T, pool *pgxpool.Pool, orgID, userID string) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id = ra.role_id
		  WHERE ra.org_id=$1 AND ra.subject_id=$2 AND r.key='owner' AND r.org_id IS NULL`, orgID, userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// The first admin of a new org also becomes Owner; later admins do not.
func TestEnsureFirstAdmin_GrantsOwner(t *testing.T) {
	pool, _ := setupDB(t)
	ctx := context.Background()
	var orgID string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name, is_personal) VALUES ('fresh-org','Fresh',false) RETURNING id::text`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	alice := seedUser(t, pool, orgID, "o-alice", "alice@test")
	bob := seedUser(t, pool, orgID, "o-bob", "bob@test")
	repo := orgadmin.NewRepository(pool)
	if ok, err := repo.EnsureFirstAdmin(ctx, orgID, alice); err != nil || !ok {
		t.Fatalf("EnsureFirstAdmin = %v, %v", ok, err)
	}
	if !hasOwner(t, pool, orgID, alice) {
		t.Error("first admin should be Owner")
	}
	if ok, _ := repo.EnsureFirstAdmin(ctx, orgID, bob); ok {
		t.Fatal("second EnsureFirstAdmin should be a no-op")
	}
	if hasOwner(t, pool, orgID, bob) {
		t.Error("second user must not be Owner")
	}
}
