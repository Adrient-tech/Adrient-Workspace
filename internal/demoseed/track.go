// Package demoseed fills an organisation with realistic, clearly fictional
// demo data for screenshots and trials, and removes exactly what it added.
//
// Every row the seeder (and the repositories it calls) creates is found by
// comparing row ids per table before and after the run, and recorded in
// adrient.demo_seed_rows. "Remove" deletes those rows again. Run it while the
// organisation is otherwise idle: rows other users add during the run would be
// recorded as demo rows too.
package demoseed

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const manifestDDL = `
CREATE TABLE IF NOT EXISTS adrient.demo_seed_runs (
    org_id     UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    summary    JSONB NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS adrient.demo_seed_rows (
    org_id UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    tbl    TEXT NOT NULL,
    row_id TEXT NOT NULL,
    seq    BIGSERIAL,
    PRIMARY KEY (org_id, tbl, row_id)
);
ALTER TABLE adrient.demo_seed_runs ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;
ALTER TABLE adrient.demo_seed_rows ADD COLUMN IF NOT EXISTS ver INT NOT NULL DEFAULT 1;`

// skipTable names tables that are never tracked: manifests, sessions, audit
// trails (append-only) and caches.
func skipTable(t string) bool {
	if strings.HasPrefix(t, "demo_seed_") {
		return true
	}
	for _, s := range []string{"audit", "session", "schema_migrations", "rate", "cache", "notification", "push_"} {
		if strings.Contains(t, s) {
			return true
		}
	}
	return false
}

// keyExpr is how a tracked table's rows are identified: its uuid id column, or
// org_id for one-row-per-org settings tables (primary key = org_id).
var keyExpr = map[string]string{}

// trackedTables lists tables with an org_id column and either a uuid id column
// or org_id as their whole primary key.
func trackedTables(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT c.table_name,
		EXISTS(SELECT 1 FROM information_schema.columns i WHERE i.table_schema=c.table_schema AND i.table_name=c.table_name AND i.column_name='id' AND i.data_type='uuid') AS has_id,
		(SELECT count(*) FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage k ON k.constraint_name=tc.constraint_name AND k.table_schema=tc.table_schema
		   WHERE tc.table_schema=c.table_schema AND tc.table_name=c.table_name AND tc.constraint_type='PRIMARY KEY') AS pk_cols,
		EXISTS(SELECT 1 FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage k ON k.constraint_name=tc.constraint_name AND k.table_schema=tc.table_schema
		   WHERE tc.table_schema=c.table_schema AND tc.table_name=c.table_name AND tc.constraint_type='PRIMARY KEY' AND k.column_name='org_id') AS pk_org
		FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema=c.table_schema AND t.table_name=c.table_name AND t.table_type='BASE TABLE'
		WHERE c.table_schema='adrient' AND c.column_name='org_id' ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		var hasID, pkOrg bool
		var pkCols int
		if err := rows.Scan(&t, &hasID, &pkCols, &pkOrg); err != nil {
			return nil, err
		}
		if skipTable(t) {
			continue
		}
		switch {
		case hasID:
			keyExpr[t] = "id::text"
		case pkCols == 1 && pkOrg:
			keyExpr[t] = "org_id::text"
		default:
			continue
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type snapshot map[string]map[string]bool

func takeSnapshot(ctx context.Context, pool *pgxpool.Pool, org string, tables []string) (snapshot, error) {
	s := snapshot{}
	for _, t := range tables {
		rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT %s FROM adrient.%s WHERE org_id=$1`, keyExpr[t], pgx.Identifier{t}.Sanitize()), org)
		if err != nil {
			return nil, fmt.Errorf("snapshot %s: %w", t, err)
		}
		m := map[string]bool{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			m[id] = true
		}
		rows.Close()
		s[t] = m
	}
	return s, nil
}

// recordNew stores the ids that exist now but not in before. It returns the
// number of rows recorded per table.
func recordNew(ctx context.Context, pool *pgxpool.Pool, org string, ver int, before snapshot, tables []string) (map[string]int, error) {
	out := map[string]int{}
	for _, t := range tables {
		rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT %s FROM adrient.%s WHERE org_id=$1`, keyExpr[t], pgx.Identifier{t}.Sanitize()), org)
		if err != nil {
			return nil, err
		}
		var fresh []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			if !before[t][id] {
				fresh = append(fresh, id)
			}
		}
		rows.Close()
		if len(fresh) == 0 {
			continue
		}
		sort.Strings(fresh)
		if _, err := pool.Exec(ctx, `INSERT INTO adrient.demo_seed_rows (org_id, tbl, row_id, ver) SELECT $1, $2, unnest($3::text[]), $4 ON CONFLICT DO NOTHING`, org, t, fresh, ver); err != nil {
			return nil, err
		}
		out[t] = len(fresh)
	}
	return out, nil
}

// removeTracked deletes every recorded row. Foreign keys decide the order, so
// it makes repeated passes until nothing more can be deleted.
func removeTracked(ctx context.Context, pool *pgxpool.Pool, org string, minVer int) (deleted int, left map[string]int, err error) {
	rows, err := pool.Query(ctx, `SELECT tbl, array_agg(row_id) FROM adrient.demo_seed_rows WHERE org_id=$1 AND ver>=$2 GROUP BY tbl`, org, minVer)
	if err != nil {
		return 0, nil, err
	}
	pending := map[string][]string{}
	for rows.Next() {
		var t string
		var ids []string
		if err := rows.Scan(&t, &ids); err != nil {
			rows.Close()
			return 0, nil, err
		}
		pending[t] = ids
	}
	rows.Close()
	for pass := 0; pass < 12 && len(pending) > 0; pass++ {
		progress := false
		tables := make([]string, 0, len(pending))
		for t := range pending {
			tables = append(tables, t)
		}
		sort.Strings(tables)
		for _, t := range tables {
			ids := pending[t]
			// Delete in small batches inside a savepoint so one blocked row does not stop the rest.
			var remaining []string
			for i := 0; i < len(ids); i += 200 {
				j := i + 200
				if j > len(ids) {
					j = len(ids)
				}
				batch := ids[i:j]
				ct, derr := pool.Exec(ctx, fmt.Sprintf(`DELETE FROM adrient.%s WHERE org_id=$1 AND %s = ANY($2)`, pgx.Identifier{t}.Sanitize(), keyOf(t)), org, batch)
				if derr != nil {
					// Row by row: keep only the ones a foreign key still holds.
					for _, id := range batch {
						ct2, e2 := pool.Exec(ctx, fmt.Sprintf(`DELETE FROM adrient.%s WHERE org_id=$1 AND %s=$2`, pgx.Identifier{t}.Sanitize(), keyOf(t)), org, id)
						if e2 != nil {
							remaining = append(remaining, id)
							continue
						}
						deleted += int(ct2.RowsAffected())
						progress = true
					}
					continue
				}
				deleted += int(ct.RowsAffected())
				progress = true
			}
			if len(remaining) == 0 {
				delete(pending, t)
			} else {
				pending[t] = remaining
			}
		}
		if !progress {
			break
		}
	}
	left = map[string]int{}
	for t, ids := range pending {
		left[t] = len(ids)
	}
	return deleted, left, nil
}

// keyOf returns the key expression of a table; Remove runs in a fresh process
// so it is rebuilt from the table list when empty.
func keyOf(t string) string {
	if k, ok := keyExpr[t]; ok {
		return k
	}
	return "id::text"
}
