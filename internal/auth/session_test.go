package auth

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"os"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
	"github.com/jackc/pgx/v5/pgxpool"
)

func sessionDB(t *testing.T) (*pgxpool.Pool, string) {
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
		t.Fatalf("migrate: %v", err)
	}
	var orgID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&orgID); err != nil {
		t.Fatalf("get default org: %v", err)
	}
	urepo := users.NewRepository(pool)
	u, err := urepo.UpsertByOIDC(ctx, users.UpsertInput{
		OrgID: orgID, OIDCIssuer: "x", OIDCSubject: "y",
		Email: "z@example.com", DisplayName: "Z",
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return pool, u.ID
}

func TestSessionStore_CreateAndLookup(t *testing.T) {
	pool, userID := sessionDB(t)
	store := NewSessionStore(pool)
	ctx := context.Background()

	tok, err := store.Create(ctx, userID, 1*time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(tok) != 64 { // 32 bytes hex-encoded
		t.Errorf("token length: got %d, want 64", len(tok))
	}

	sess, err := store.Lookup(ctx, tok)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if sess.UserID != userID {
		t.Errorf("user id: got %s, want %s", sess.UserID, userID)
	}
	if sess.ExpiresAt.Before(time.Now()) {
		t.Errorf("expires_at is in the past: %v", sess.ExpiresAt)
	}
}

func TestSessionStore_LookupExpired(t *testing.T) {
	pool, userID := sessionDB(t)
	store := NewSessionStore(pool)
	ctx := context.Background()

	tok, err := store.Create(ctx, userID, -1*time.Hour) // already expired
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := store.Lookup(ctx, tok); err != ErrSessionExpired {
		t.Errorf("got %v, want ErrSessionExpired", err)
	}
}

func TestSessionStore_Revoke(t *testing.T) {
	pool, userID := sessionDB(t)
	store := NewSessionStore(pool)
	ctx := context.Background()

	tok, err := store.Create(ctx, userID, 1*time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Revoke(ctx, tok); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := store.Lookup(ctx, tok); err != ErrSessionRevoked {
		t.Errorf("got %v, want ErrSessionRevoked", err)
	}
}

func TestSessionStore_LookupUnknown(t *testing.T) {
	pool, _ := sessionDB(t)
	store := NewSessionStore(pool)
	if _, err := store.Lookup(context.Background(), "nonsense"); err != ErrSessionNotFound {
		t.Errorf("got %v, want ErrSessionNotFound", err)
	}
}
