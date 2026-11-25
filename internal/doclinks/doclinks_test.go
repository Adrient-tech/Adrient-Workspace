package doclinks

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLinkFirstWriterWins(t *testing.T) {
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
	var org, user string
	if err := pool.QueryRow(ctx, `SELECT o.id::text, u.id::text FROM adrient.orgs o JOIN adrient.users u ON u.org_id=o.id LIMIT 1`).Scan(&org, &user); err != nil {
		t.Skip("no seeded org/user: ", err)
	}
	var file, d1, d2 string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, name, mime_type, storage_key, size_bytes) VALUES ($1,$2,'t.xlsx','x','k-doclinks-test',0) RETURNING id::text`, org, user).Scan(&file); err != nil {
		t.Skip("drive_files insert: ", err)
	}
	defer pool.Exec(ctx, `DELETE FROM adrient.drive_files WHERE id=$1`, file)
	for _, p := range []*string{&d1, &d2} {
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.sheets_documents (org_id, owner_id) VALUES ($1,$2) RETURNING id::text`, org, user).Scan(p); err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(ctx, `DELETE FROM adrient.sheets_documents WHERE id=$1`, *p)
	}
	defer pool.Exec(ctx, `DELETE FROM adrient.drive_doc_links WHERE drive_file_id=$1`, file)
	s := NewStore(pool, nil)
	w1, ok, err := s.Link(ctx, org, user, file, "sheets", d1)
	if err != nil || !ok || w1 != d1 {
		t.Fatalf("first: %v %v %v", w1, ok, err)
	}
	w2, _, err := s.Link(ctx, org, user, file, "sheets", d2)
	if err != nil || w2 != d1 {
		t.Fatalf("second must lose: %v %v", w2, err)
	}
	got, ok, _ := s.Resolve(ctx, org, user, file, "sheets")
	if !ok || got != d1 {
		t.Fatalf("resolve: %v %v", got, ok)
	}
	if _, ok, _ := s.Resolve(ctx, "00000000-0000-0000-0000-000000000001", user, file, "sheets"); ok {
		t.Fatal("other org must not resolve")
	}
}
