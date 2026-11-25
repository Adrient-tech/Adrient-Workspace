package adminops

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BackupFormat is bumped when the archive layout changes incompatibly.
const BackupFormat = 1

// Manifest describes a backup archive. It is the first entry in the tar.
type Manifest struct {
	Format         int              `json:"format"`
	Version        string           `json:"version"`
	CreatedAt      time.Time        `json:"created_at"`
	MaxMigration   int              `json:"max_migration"`
	Migrations     []int            `json:"migrations"` // every applied version
	Tables         map[string]int64 `json:"tables"`     // row counts at snapshot time
	ObjectCount    int              `json:"object_count"`
	ObjectBytes    int64            `json:"object_bytes"`
	ConfigIncluded bool             `json:"config_included"`
}

// ObjectEntry is one record of objects.json inside the archive.
type ObjectEntry struct {
	Key    string `json:"key"`
	File   string `json:"file"` // path inside the archive
	Mime   string `json:"mime"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// BackupOptions control what a backup contains.
type BackupOptions struct {
	Version     string
	ConfigFile  string // optional env file to include (contains secrets)
	SkipObjects bool
}

// listTables returns base tables in the adrient schema, sorted.
func listTables(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT table_name FROM information_schema.tables
		WHERE table_schema='adrient' AND table_type='BASE TABLE' ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// Backup writes a consistent snapshot (one REPEATABLE READ transaction) of the
// adrient schema, every object in the store, and optionally the config file,
// as a gzip'd tar. Layout:
//
//	manifest.json
//	objects.json
//	db/<table>.csv        (COPY ... FORMAT csv)
//	objects/<n>           (blob bytes, indexed by objects.json)
//	config/<basename>
//
// Postgres data is staged in a temp dir first so the manifest (row counts) can
// lead the archive and tar sizes are known.
func Backup(ctx context.Context, pool *pgxpool.Pool, store ObjectStore, w io.Writer, opt BackupOptions) (*Manifest, error) {
	tmp, err := os.MkdirTemp("", "adrient-backup-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tables, err := listTables(ctx, tx)
	if err != nil {
		return nil, err
	}
	var maxMig int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version),0) FROM adrient.schema_migrations`).Scan(&maxMig); err != nil {
		return nil, fmt.Errorf("read schema version (is this an Adrient database?): %w", err)
	}
	vrows, err := tx.Query(ctx, `SELECT version FROM adrient.schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	var versions []int
	for vrows.Next() {
		var v int
		if err := vrows.Scan(&v); err != nil {
			vrows.Close()
			return nil, err
		}
		versions = append(versions, v)
	}
	vrows.Close()
	m := &Manifest{Migrations: versions, Format: BackupFormat, Version: opt.Version, CreatedAt: time.Now().UTC(), MaxMigration: maxMig, Tables: map[string]int64{}}

	for _, t := range tables {
		f, err := os.Create(path.Join(tmp, t+".csv"))
		if err != nil {
			return nil, err
		}
		tag, err := tx.Conn().PgConn().CopyTo(ctx, f, "COPY adrient."+quoteIdent(t)+" TO STDOUT WITH (FORMAT csv)")
		cerr := f.Close()
		if err != nil {
			return nil, fmt.Errorf("copy out %s: %w", t, err)
		}
		if cerr != nil {
			return nil, cerr
		}
		m.Tables[t] = tag.RowsAffected()
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	var objs []ObjectEntry
	if !opt.SkipObjects && store != nil {
		var keys []string
		sizes := map[string]int64{}
		if err := store.List(ctx, "", func(k string, sz int64) error { keys = append(keys, k); sizes[k] = sz; return nil }); err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		sort.Strings(keys)
		for i, k := range keys {
			rc, mime, _, err := store.Get(ctx, k)
			if err != nil {
				return nil, err
			}
			fn := path.Join(tmp, fmt.Sprintf("obj-%d", i))
			f, err := os.Create(fn)
			if err != nil {
				rc.Close()
				return nil, err
			}
			h := sha256.New()
			n, err := io.Copy(io.MultiWriter(f, h), rc)
			rc.Close()
			f.Close()
			if err != nil {
				return nil, fmt.Errorf("read object %s: %w", k, err)
			}
			objs = append(objs, ObjectEntry{Key: k, File: fmt.Sprintf("objects/%d", i), Mime: mime, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))})
			m.ObjectBytes += n
		}
		m.ObjectCount = len(objs)
	}

	var cfgData []byte
	if opt.ConfigFile != "" {
		cfgData, err = os.ReadFile(opt.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
		m.ConfigIncluded = true
	}

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	putBytes := func(name string, b []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(b)), ModTime: m.CreatedAt}); err != nil {
			return err
		}
		_, err := tw.Write(b)
		return err
	}
	putFile := func(name, src string) error {
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer f.Close()
		st, _ := f.Stat()
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: st.Size(), ModTime: m.CreatedAt}); err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		return err
	}
	mb, _ := json.MarshalIndent(m, "", "  ")
	ob, _ := json.Marshal(objs)
	if err := putBytes("manifest.json", mb); err != nil {
		return nil, err
	}
	if err := putBytes("objects.json", ob); err != nil {
		return nil, err
	}
	for _, t := range tables {
		if err := putFile("db/"+t+".csv", path.Join(tmp, t+".csv")); err != nil {
			return nil, err
		}
	}
	for i := range objs {
		if err := putFile(objs[i].File, path.Join(tmp, fmt.Sprintf("obj-%d", i))); err != nil {
			return nil, err
		}
	}
	if cfgData != nil {
		if err := putBytes("config/"+path.Base(opt.ConfigFile), cfgData); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return m, nil
}

// Archive is a backup unpacked to a temp directory.
type Archive struct {
	Dir      string
	Manifest Manifest
	Objects  []ObjectEntry
	Config   string // path of the unpacked config file, "" if none
}

// Close removes the unpacked files.
func (a *Archive) Close() { _ = os.RemoveAll(a.Dir) }

// OpenArchive unpacks a backup, rejecting path traversal and unknown formats.
func OpenArchive(r io.Reader) (*Archive, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("not a gzip archive: %w", err)
	}
	dir, err := os.MkdirTemp("", "adrient-restore-*")
	if err != nil {
		return nil, err
	}
	a := &Archive{Dir: dir}
	ok := false
	defer func() {
		if !ok {
			a.Close()
		}
	}()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		clean := path.Clean(h.Name)
		if strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") {
			return nil, fmt.Errorf("unsafe path in archive: %q", h.Name)
		}
		dst := path.Join(dir, clean)
		if err := os.MkdirAll(path.Dir(dst), 0o700); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return nil, err
		}
		f.Close()
		if strings.HasPrefix(clean, "config/") {
			a.Config = dst
		}
	}
	mb, err := os.ReadFile(path.Join(dir, "manifest.json"))
	if err != nil {
		return nil, errors.New("archive has no manifest.json (not an Adrient backup)")
	}
	if err := json.Unmarshal(mb, &a.Manifest); err != nil {
		return nil, fmt.Errorf("bad manifest: %w", err)
	}
	if a.Manifest.Format != BackupFormat {
		return nil, fmt.Errorf("backup format %d is not supported by this build (supports %d)", a.Manifest.Format, BackupFormat)
	}
	if ob, err := os.ReadFile(path.Join(dir, "objects.json")); err == nil {
		if err := json.Unmarshal(ob, &a.Objects); err != nil {
			return nil, fmt.Errorf("bad objects.json: %w", err)
		}
	}
	ok = true
	return a, nil
}
