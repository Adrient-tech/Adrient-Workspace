package tickets

import (
	"context"
	"errors"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

func TestHTTPErrDuplicateKeyIs409(t *testing.T) {
	rec := httptest.NewRecorder()
	httpErr(rec, ErrDuplicateKey)
	if rec.Code != 409 {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if body := rec.Body.String(); body == "" || body == "internal error" {
		t.Fatalf("body = %q", body)
	}
}

func TestCreateProjectDuplicateKey(t *testing.T) {
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := testguard.DropSchema(ctx, t, pool); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var org string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	if _, err := repo.CreateProject(ctx, org, "", "ABC", "First", "", "team"); err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateProject(ctx, org, "", "abc", "Second", "", "team")
	if !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("want ErrDuplicateKey, got %v", err)
	}
}
