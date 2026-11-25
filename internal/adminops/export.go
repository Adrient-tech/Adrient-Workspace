package adminops

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExportManifest describes an org export.
type ExportManifest struct {
	Format      int                 `json:"format"`
	Version     string              `json:"version"`
	ExportedAt  time.Time           `json:"exported_at"`
	OrgID       string              `json:"org_id"`
	OrgSlug     string              `json:"org_slug"`
	Tables      map[string]int64    `json:"tables"`
	Files       int                 `json:"files"`
	FileBytes   int64               `json:"file_bytes"`
	NotExported []string            `json:"not_exported"` // tables that cannot be attributed to one org, or are secrets
	Redacted    map[string][]string `json:"redacted_columns,omitempty"`
}

// Tables never exported: live credentials/sessions, not user data.
var exportSkipTables = map[string]bool{
	"schema_migrations": true, "sessions": true, "api_tokens": true,
}

var sensitiveCol = regexp.MustCompile(`(?i)(password|passwd|secret|token_hash|private_key|api_key|apikey|credential)`)

type fkInfo struct{ col, parent, parentCol string }

// scopeConds computes, for every table, a SQL boolean expression that selects
// the rows belonging to org $1: either "org_id = $1" or an IN-subquery through
// a single-column foreign key to a table that is itself org-scoped.
func scopeConds(ctx context.Context, pool *pgxpool.Pool) (map[string]string, []string, error) {
	tables, err := listTables(ctx, pool)
	if err != nil {
		return nil, nil, err
	}
	hasOrg := map[string]bool{}
	rows, err := pool.Query(ctx, `SELECT table_name FROM information_schema.columns WHERE table_schema='adrient' AND column_name='org_id'`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		hasOrg[n] = true
	}
	rows.Close()

	fks := map[string][]fkInfo{}
	rows, err = pool.Query(ctx, `
		SELECT c.conrelid::regclass::text, a.attname, c.confrelid::regclass::text, fa.attname
		FROM pg_constraint c
		JOIN pg_attribute a  ON a.attrelid=c.conrelid  AND a.attnum=c.conkey[1]
		JOIN pg_attribute fa ON fa.attrelid=c.confrelid AND fa.attnum=c.confkey[1]
		WHERE c.contype='f' AND array_length(c.conkey,1)=1
		  AND c.connamespace='adrient'::regnamespace
		ORDER BY 1,2`)
	if err != nil {
		return nil, nil, err
	}
	strip := func(s string) string { return strings.Trim(strings.TrimPrefix(s, "adrient."), `"`) }
	for rows.Next() {
		var t, col, p, pc string
		_ = rows.Scan(&t, &col, &p, &pc)
		fks[strip(t)] = append(fks[strip(t)], fkInfo{col, strip(p), pc})
	}
	rows.Close()

	memo := map[string]string{}
	var cond func(t string, depth int) string
	cond = func(t string, depth int) string {
		if c, ok := memo[t]; ok {
			return c
		}
		memo[t] = "" // cycle guard
		var c string
		switch {
		case t == "orgs":
			c = "id = $1"
		case hasOrg[t]:
			c = "org_id = $1"
		case depth < 5:
			var parts []string
			for _, fk := range fks[t] {
				if fk.parent == t || exportSkipTables[fk.parent] {
					continue
				}
				if pc := cond(fk.parent, depth+1); pc != "" {
					parts = append(parts, fmt.Sprintf("%s IN (SELECT %s FROM adrient.%s WHERE %s)",
						quoteIdent(fk.col), quoteIdent(fk.parentCol), quoteIdent(fk.parent), pc))
				}
			}
			c = strings.Join(parts, " OR ")
			if len(parts) > 1 {
				c = "(" + c + ")"
			}
		}
		memo[t] = c
		return c
	}
	var unscoped []string
	out := map[string]string{}
	for _, t := range tables {
		if exportSkipTables[t] {
			continue
		}
		if c := cond(t, 0); c != "" {
			out[t] = c
		} else {
			unscoped = append(unscoped, t)
		}
	}
	return out, unscoped, nil
}

// ExportOrg writes everything belonging to one org as a portable tar.gz:
// data/<table>.ndjson (one JSON object per row), files/<drive path> (the Drive
// file contents with their folder structure), manifest.json and README.txt.
// Secrets (sessions, API tokens, password/secret-like columns) are left out.
func ExportOrg(ctx context.Context, pool *pgxpool.Pool, store ObjectStore, orgRef string, w io.Writer, version string) (*ExportManifest, error) {
	var orgID, slug string
	if err := pool.QueryRow(ctx, `SELECT id::text, slug FROM adrient.orgs WHERE slug=$1 OR id::text=$1`, orgRef).Scan(&orgID, &slug); err != nil {
		return nil, fmt.Errorf("org %q not found: %w", orgRef, err)
	}
	conds, unscoped, err := scopeConds(ctx, pool)
	if err != nil {
		return nil, err
	}
	m := &ExportManifest{Format: 1, Version: version, ExportedAt: time.Now().UTC(), OrgID: orgID, OrgSlug: slug,
		Tables: map[string]int64{}, Redacted: map[string][]string{}}
	m.NotExported = append(m.NotExported, unscoped...)
	for t := range exportSkipTables {
		m.NotExported = append(m.NotExported, t)
	}
	sort.Strings(m.NotExported)

	names := make([]string, 0, len(conds))
	for t := range conds {
		names = append(names, t)
	}
	sort.Strings(names)

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	put := func(name string, b []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(b)), ModTime: m.ExportedAt}); err != nil {
			return err
		}
		_, err := tw.Write(b)
		return err
	}

	// Tables are buffered one at a time (tar needs the size up front).
	var buffered []struct {
		name string
		data []byte
	}
	for _, t := range names {
		var cols []string
		rows, err := pool.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema='adrient' AND table_name=$1`, t)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c string
			_ = rows.Scan(&c)
			if sensitiveCol.MatchString(c) {
				m.Redacted[t] = append(m.Redacted[t], c)
			} else {
				continue
			}
			cols = append(cols, c)
		}
		rows.Close()
		expr := "to_jsonb(t)"
		for _, c := range cols {
			expr += " - '" + strings.ReplaceAll(c, "'", "''") + "'"
		}
		q := fmt.Sprintf("SELECT %s FROM adrient.%s t WHERE %s", expr, quoteIdent(t), conds[t])
		r2, err := pool.Query(ctx, q, orgID)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", t, err)
		}
		var buf bytes.Buffer
		var n int64
		for r2.Next() {
			var raw []byte
			if err := r2.Scan(&raw); err != nil {
				r2.Close()
				return nil, err
			}
			buf.Write(raw)
			buf.WriteByte('\n')
			n++
		}
		r2.Close()
		if err := r2.Err(); err != nil {
			return nil, fmt.Errorf("export %s: %w", t, err)
		}
		if n > 0 {
			m.Tables[t] = n
			buffered = append(buffered, struct {
				name string
				data []byte
			}{"data/" + t + ".ndjson", buf.Bytes()})
		}
	}

	// Drive file contents, laid out by folder path.
	type fileRow struct{ key, p, mime string }
	var files []fileRow
	if store != nil {
		rows, err := pool.Query(ctx, `
			WITH RECURSIVE tree AS (
				SELECT id, parent_id, name, storage_key, mime_type, name::text AS p
				FROM adrient.drive_files WHERE org_id=$1 AND parent_id IS NULL AND trashed_at IS NULL
				UNION ALL
				SELECT f.id, f.parent_id, f.name, f.storage_key, f.mime_type, tree.p || '/' || f.name
				FROM adrient.drive_files f JOIN tree ON f.parent_id = tree.id WHERE f.trashed_at IS NULL)
			SELECT storage_key, p, mime_type FROM tree WHERE storage_key IS NOT NULL ORDER BY p`, orgID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var f fileRow
			_ = rows.Scan(&f.key, &f.p, &f.mime)
			files = append(files, f)
		}
		rows.Close()
	}
	used := map[string]bool{}
	type staged struct {
		name string
		key  string
	}
	var plan []staged
	for _, f := range files {
		name := "files/" + safeRel(f.p)
		for i := 2; used[name]; i++ {
			name = fmt.Sprintf("files/%s (%d)", safeRel(f.p), i)
		}
		used[name] = true
		plan = append(plan, staged{name, f.key})
	}
	// Count first so the manifest is accurate and leads the archive.
	type blob struct {
		name string
		data []byte
	}
	var blobs []blob
	for _, p := range plan {
		rc, _, _, err := store.Get(ctx, p.key)
		if err != nil {
			m.NotExported = append(m.NotExported, "file missing in object store: "+p.name)
			continue
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		blobs = append(blobs, blob{p.name, b})
		m.Files++
		m.FileBytes += int64(len(b))
	}

	mb, _ := json.MarshalIndent(m, "", "  ")
	if err := put("manifest.json", mb); err != nil {
		return nil, err
	}
	if err := put("README.txt", []byte(exportReadme)); err != nil {
		return nil, err
	}
	for _, b := range buffered {
		if err := put(b.name, b.data); err != nil {
			return nil, err
		}
	}
	for _, b := range blobs {
		if err := put(b.name, b.data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return m, gz.Close()
}

// safeRel makes a Drive path safe to use inside an archive.
func safeRel(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		s = strings.ReplaceAll(s, "\x00", "")
		if s == "" || s == "." || s == ".." {
			s = "_"
		}
		parts[i] = s
	}
	return path.Join(parts...)
}

const exportReadme = `Adrient Workspace organisation export
=====================================
manifest.json   what is in this archive, row counts, what was left out
data/*.ndjson   one JSON object per row, one file per table (UUID ids, RFC 3339 times)
files/          Drive files with their folder structure (trashed items excluded)

Left out on purpose: login sessions, API tokens, and any column whose name looks
like a password/secret/credential. Tables that cannot be tied to a single
organisation are listed in manifest.json under "not_exported".
`
