package adminops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"sort"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RestoreOptions control a restore.
type RestoreOptions struct {
	// Replace acknowledges that ALL existing rows in the target adrient schema
	// are replaced by the backup. Required: restore never merges.
	Replace bool
}

// RestoreReport lists what was restored and what the verification found.
type RestoreReport struct {
	Tables   map[string]int64 // rows loaded per table
	Objects  int
	Mismatch []string // table: want N got M
}

// Restore loads an unpacked archive into the database behind pool: it applies
// pending migrations (so the schema exists and matches this binary), replaces
// every table's rows with the backup's, resets sequences, restores objects
// (checksum-verified) and finally compares row counts with the manifest.
//
// The database role needs permission to SET session_replication_role (a
// superuser, as in the shipped compose files), because rows are loaded in
// file order, not foreign-key order, with triggers/FKs deferred for the load.
func Restore(ctx context.Context, pool *pgxpool.Pool, store ObjectStore, a *Archive, opt RestoreOptions) (*RestoreReport, error) {
	if !opt.Replace {
		return nil, fmt.Errorf("restore replaces all data in the target database; pass --replace to confirm")
	}
	if embedded := storage.EmbeddedMigrationCount(); embedded == 0 {
		return nil, fmt.Errorf("no embedded migrations")
	}
	// The backup must not come from a newer schema than this binary knows.
	status, err := storage.GetMigrationStatus(ctx, pool)
	if err != nil {
		return nil, err
	}
	known := map[int]bool{}
	for _, m := range status.Migrations {
		known[m.Version] = true
	}
	var unknown []int
	for _, v := range append(a.Manifest.Migrations, a.Manifest.MaxMigration) {
		if v > 0 && !known[v] {
			unknown = append(unknown, v)
		}
	}
	if len(unknown) > 0 {
		sort.Ints(unknown)
		return nil, fmt.Errorf("backup contains schema versions this build does not know (%v); restore it with the release that made it (or newer)", unknown)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		return nil, fmt.Errorf("prepare schema: %w", err)
	}

	live, err := func() ([]string, error) { return listTables(ctx, pool) }()
	if err != nil {
		return nil, err
	}
	liveSet := map[string]bool{}
	for _, t := range live {
		liveSet[t] = true
	}

	rep := &RestoreReport{Tables: map[string]int64{}}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		return nil, fmt.Errorf("cannot defer constraints (needs a superuser database role): %w", err)
	}
	for _, t := range live {
		if _, err := tx.Exec(ctx, "TRUNCATE adrient."+quoteIdent(t)+" CASCADE"); err != nil {
			return nil, fmt.Errorf("clear %s: %w", t, err)
		}
	}
	tables := make([]string, 0, len(a.Manifest.Tables))
	for t := range a.Manifest.Tables {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	for _, t := range tables {
		if !liveSet[t] {
			return nil, fmt.Errorf("backup has table %s which does not exist after migrating; refusing a partial restore", t)
		}
		f, err := os.Open(path.Join(a.Dir, "db", t+".csv"))
		if err != nil {
			return nil, fmt.Errorf("archive is missing %s: %w", t, err)
		}
		tag, err := tx.Conn().PgConn().CopyFrom(ctx, f, "COPY adrient."+quoteIdent(t)+" FROM STDIN WITH (FORMAT csv)")
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", t, err)
		}
		rep.Tables[t] = tag.RowsAffected()
	}
	// Re-point serial/identity sequences past the loaded data.
	rows, err := tx.Query(ctx, `
		SELECT c.relname, a.attname, pg_get_serial_sequence('adrient.'||quote_ident(c.relname), a.attname)
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace AND n.nspname='adrient'
		JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped
		WHERE c.relkind='r' AND pg_get_serial_sequence('adrient.'||quote_ident(c.relname), a.attname) IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	type seq struct{ tbl, col, name string }
	var seqs []seq
	for rows.Next() {
		var s seq
		if err := rows.Scan(&s.tbl, &s.col, &s.name); err != nil {
			rows.Close()
			return nil, err
		}
		seqs = append(seqs, s)
	}
	rows.Close()
	for _, s := range seqs {
		q := fmt.Sprintf(`SELECT setval(%s::regclass, GREATEST(COALESCE((SELECT MAX(%s) FROM adrient.%s),0),1))`,
			"'"+s.name+"'", quoteIdent(s.col), quoteIdent(s.tbl))
		if _, err := tx.Exec(ctx, q); err != nil {
			return nil, fmt.Errorf("reset sequence %s: %w", s.name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	if store != nil {
		for _, o := range a.Objects {
			f, err := os.Open(path.Join(a.Dir, path.Clean(o.File)))
			if err != nil {
				return nil, err
			}
			h := sha256.New()
			if _, err := io.Copy(h, f); err != nil {
				f.Close()
				return nil, err
			}
			if hex.EncodeToString(h.Sum(nil)) != o.SHA256 {
				f.Close()
				return nil, fmt.Errorf("object %q is corrupt in the archive (checksum mismatch)", o.Key)
			}
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				f.Close()
				return nil, err
			}
			err = store.Put(ctx, o.Key, o.Mime, o.Size, f)
			f.Close()
			if err != nil {
				return nil, err
			}
			rep.Objects++
		}
	}

	for t, want := range a.Manifest.Tables {
		if !liveSet[t] {
			continue
		}
		var got int64
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM adrient."+quoteIdent(t)).Scan(&got); err != nil {
			return nil, err
		}
		if got != want {
			rep.Mismatch = append(rep.Mismatch, fmt.Sprintf("%s: backup has %d rows, database has %d", t, want, got))
		}
	}
	sort.Strings(rep.Mismatch)
	if len(rep.Mismatch) > 0 {
		return rep, fmt.Errorf("verification failed for %d table(s)", len(rep.Mismatch))
	}
	return rep, nil
}
