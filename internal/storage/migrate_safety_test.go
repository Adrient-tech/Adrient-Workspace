package storage

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestMigrationFilesHaveNoDuplicateVersions guards the numbering rule. Gaps are
// legal (several ranges were reserved and never used) and only logged.
func TestMigrationFilesHaveNoDuplicateVersions(t *testing.T) {
	dups, gaps, err := LintMigrationNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(dups) > 0 {
		t.Fatalf("duplicate migration versions: %v", dups)
	}
	t.Logf("%d migrations embedded, %d unused version numbers (gaps are allowed)", EmbeddedMigrationCount(), len(gaps))
}

var (
	reCreate = regexp.MustCompile(`(?im)^\s*create\s+(unique\s+)?(table|index|schema|extension|sequence)\b.*$`)
	reAlter  = regexp.MustCompile(`(?im)^\s*alter\s+table\s+.*add\s+column\b.*$`)
	reDrop   = regexp.MustCompile(`(?im)^\s*(drop\s+(table|schema|database|column)|truncate)\b`)
)

// TestMigrationsAreIdempotentByText is a cheap static lint: CREATE/ADD COLUMN
// need IF NOT EXISTS, and no migration may drop or truncate data. The runtime
// re-apply test below is the real proof.
func TestMigrationsAreIdempotentByText(t *testing.T) {
	migs, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migs {
		for _, loc := range reCreate.FindAllStringSubmatch(m.sql, -1) {
			if !strings.Contains(strings.ToLower(loc[0]), "if not exists") {
				t.Errorf("%04d_%s: %q lacks IF NOT EXISTS", m.version, m.name, strings.TrimSpace(loc[0]))
			}
		}
		for _, loc := range reAlter.FindAllStringSubmatch(m.sql, -1) {
			if !strings.Contains(strings.ToLower(loc[0]), "if not exists") {
				t.Errorf("%04d_%s: %q lacks IF NOT EXISTS", m.version, m.name, strings.TrimSpace(loc[0]))
			}
		}
		for _, loc := range reDrop.FindAllString(m.sql, -1) {
			t.Errorf("%04d_%s: destructive statement %q", m.version, m.name, strings.TrimSpace(loc))
		}
	}
}

// scratchPool creates an empty throwaway database on the server named by
// ADRIENT_TEST_ADMIN_DSN (any role allowed to CREATE DATABASE) and returns a
// pool on it. It never touches the shared "adrient" database.
func scratchPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	admin := os.Getenv("ADRIENT_TEST_ADMIN_DSN")
	if admin == "" {
		t.Skip("ADRIENT_TEST_ADMIN_DSN not set; skipping fresh-database migration test")
	}
	ctx := context.Background()
	ap, err := pgxpool.New(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("adrient_migtest_%d", time.Now().UnixNano())
	if _, err := ap.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		ap.Close()
		t.Fatalf("create scratch db: %v", err)
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
	return pool
}

func countTables(t *testing.T, pool *pgxpool.Pool) (tables, columns, indexes int) {
	t.Helper()
	ctx := context.Background()
	must := func(q string, dst *int) {
		if err := pool.QueryRow(ctx, q).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	must(`SELECT count(*) FROM information_schema.tables WHERE table_schema='adrient'`, &tables)
	must(`SELECT count(*) FROM information_schema.columns WHERE table_schema='adrient'`, &columns)
	must(`SELECT count(*) FROM pg_indexes WHERE schemaname='adrient'`, &indexes)
	return
}

// TestMigrationsApplyToEmptyDatabaseThenReapply applies every migration to an
// empty database, runs the runner again (must be a no-op), then forcibly
// re-executes the SQL of EVERY migration file a second time, which is what an
// operator re-running a half-finished upgrade would cause. Schema shape must
// not change and nothing may error.
func TestMigrationsApplyToEmptyDatabaseThenReapply(t *testing.T) {
	pool := scratchPool(t)
	ctx := context.Background()

	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatalf("first run: %v", err)
	}
	st, err := Preflight(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Pending()) != 0 {
		t.Fatalf("pending after run: %v", st.Pending())
	}
	t1, c1, i1 := countTables(t, pool)
	if t1 < 50 {
		t.Fatalf("suspiciously few tables after migrating: %d", t1)
	}

	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}

	migs, _ := loadMigrations()
	for _, m := range migs {
		if _, err := pool.Exec(ctx, m.sql); err != nil {
			t.Errorf("re-applying %04d_%s failed (not idempotent): %v", m.version, m.name, err)
		}
	}
	t2, c2, i2 := countTables(t, pool)
	if t1 != t2 || c1 != c2 || i1 != i2 {
		t.Errorf("schema changed on re-apply: tables %d->%d columns %d->%d indexes %d->%d", t1, t2, c1, c2, i1, i2)
	}
	t.Logf("%d migrations; %d tables, %d columns, %d indexes; re-apply clean", len(migs), t1, c1, i1)
}

func TestPreflightRefusesDatabaseAheadOfBinary(t *testing.T) {
	pool := scratchPool(t)
	ctx := context.Background()
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.schema_migrations (version) VALUES (9999)`); err != nil {
		t.Fatal(err)
	}
	if _, err := Preflight(ctx, pool); err == nil {
		t.Fatal("expected preflight to refuse a database newer than the binary")
	}
}
