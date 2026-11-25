package storage

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

// RunMigrations applies any pending migrations from the embedded migrations/ tree
// to the database. Idempotent: already-applied migrations are skipped.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migs, err := loadMigrations()
	if err != nil {
		return err
	}

	// Serialise concurrent starters (several replicas, or adrient-admin migrate
	// up racing the server) with a session-level advisory lock on one connection.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("migration advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockKey)
	}()

	// Ensure the schema_migrations table exists before reading it. We do this by
	// always running migration 0001 — which is idempotent (CREATE TABLE IF NOT EXISTS).
	for _, m := range migs {
		if m.version != 1 {
			continue
		}
		if _, err := pool.Exec(ctx, m.sql); err != nil {
			return fmt.Errorf("apply migration %04d: %w", m.version, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO adrient.schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`,
			m.version); err != nil {
			return fmt.Errorf("record migration %04d: %w", m.version, err)
		}
		break
	}

	// Now read which versions have been applied and run the rest in order.
	rows, err := pool.Query(ctx, `SELECT version FROM adrient.schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return fmt.Errorf("scan schema_migrations row: %w", err)
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate schema_migrations: %w", err)
	}

	for _, m := range migs {
		if applied[m.version] {
			continue
		}
		// Atomic apply+record: if either step fails, neither persists.
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for migration %04d_%s: %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %04d_%s: %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO adrient.schema_migrations (version) VALUES ($1)`,
			m.version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %04d_%s: %w", m.version, m.name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %04d_%s: %w", m.version, m.name, err)
		}
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	migs := make([]migration, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		// Filename format: 0001_init.sql  → version 1, name "init"
		parts := strings.SplitN(strings.TrimSuffix(e.Name(), ".sql"), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed migration filename: %s", e.Name())
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("migration filename version not numeric: %s", e.Name())
		}
		data, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		migs = append(migs, migration{version: v, name: parts[1], sql: string(data)})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
	return migs, nil
}

// migrationLockKey is the pg advisory-lock key that serialises migration runs.
const migrationLockKey int64 = 0x41445249454e54 // "ADRIENT"

// MigrationInfo describes one embedded migration and whether it is applied.
type MigrationInfo struct {
	Version   int
	Name      string
	Applied   bool
	AppliedAt *time.Time
}

// MigrationStatus is the result of comparing embedded migrations with a database.
type MigrationStatus struct {
	Migrations []MigrationInfo
	// Unknown lists versions recorded in the database that this binary does not
	// ship. That means the database is NEWER than the binary (downgrade attempt).
	Unknown []int
}

// Pending returns the migrations that have not been applied yet.
func (s MigrationStatus) Pending() []MigrationInfo {
	var out []MigrationInfo
	for _, m := range s.Migrations {
		if !m.Applied {
			out = append(out, m)
		}
	}
	return out
}

// EmbeddedMigrationCount returns how many migrations this binary ships.
func EmbeddedMigrationCount() int {
	migs, err := loadMigrations()
	if err != nil {
		return 0
	}
	return len(migs)
}

// GetMigrationStatus reads schema_migrations without changing anything. A
// database that was never migrated reports every migration as pending.
func GetMigrationStatus(ctx context.Context, pool *pgxpool.Pool) (MigrationStatus, error) {
	migs, err := loadMigrations()
	if err != nil {
		return MigrationStatus{}, err
	}
	applied := map[int]time.Time{}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('adrient.schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return MigrationStatus{}, fmt.Errorf("check schema_migrations: %w", err)
	}
	if exists {
		rows, err := pool.Query(ctx, `SELECT version, applied_at FROM adrient.schema_migrations`)
		if err != nil {
			return MigrationStatus{}, fmt.Errorf("read schema_migrations: %w", err)
		}
		for rows.Next() {
			var v int
			var at time.Time
			if err := rows.Scan(&v, &at); err != nil {
				rows.Close()
				return MigrationStatus{}, err
			}
			applied[v] = at
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return MigrationStatus{}, err
		}
	}
	st := MigrationStatus{}
	known := map[int]bool{}
	for _, m := range migs {
		known[m.version] = true
		info := MigrationInfo{Version: m.version, Name: m.name}
		if at, ok := applied[m.version]; ok {
			a := at
			info.Applied, info.AppliedAt = true, &a
		}
		st.Migrations = append(st.Migrations, info)
	}
	for v := range applied {
		if !known[v] {
			st.Unknown = append(st.Unknown, v)
		}
	}
	sort.Ints(st.Unknown)
	return st, nil
}

// Preflight checks that migrating is safe: the database must not be ahead of
// this binary (migrations are forward-only, so an older binary against a newer
// schema is refused rather than guessed at). It changes nothing.
func Preflight(ctx context.Context, pool *pgxpool.Pool) (MigrationStatus, error) {
	st, err := GetMigrationStatus(ctx, pool)
	if err != nil {
		return st, err
	}
	if len(st.Unknown) > 0 {
		return st, fmt.Errorf("database has %d migration(s) this binary does not know (e.g. %04d): it was upgraded by a newer release; run the newer binary or restore a backup (downgrades are not supported)", len(st.Unknown), st.Unknown[0])
	}
	return st, nil
}

// LintMigrationNames reports problems in the embedded file set: duplicate
// versions, and gaps (gaps are legal, only listed for the operator). Applied
// files must never be renumbered, so this only reports.
func LintMigrationNames() (dups []int, gaps []int, err error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, nil, err
	}
	seen := map[int]int{}
	max := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		v, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil {
			return nil, nil, fmt.Errorf("non-numeric migration prefix: %s", e.Name())
		}
		seen[v]++
		if v > max {
			max = v
		}
	}
	for v := 1; v <= max; v++ {
		switch n := seen[v]; {
		case n > 1:
			dups = append(dups, v)
		case n == 0:
			gaps = append(gaps, v)
		}
	}
	return dups, gaps, nil
}
