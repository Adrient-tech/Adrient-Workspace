package records

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5"
)

// Record is what the API returns for one record.
type Record struct {
	ID        string         `json:"id"`
	Seq       int64          `json:"seq"`
	Fields    map[string]any `json:"fields"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	CreatedBy string         `json:"created_by,omitempty"`
	UpdatedBy string         `json:"updated_by,omitempty"`
}

// RecordInput carries field values keyed by field id or name. Link fields take a list of record ids.
type RecordInput struct {
	ID     string         `json:"id,omitempty"`
	Fields map[string]any `json:"fields"`
}

// apiField is a field as clients see it (formulas with names instead of ids).
func apiField(f Field, s *schema) Field {
	if f.Type != TFormula && f.Type != TButton {
		return f
	}
	opts := make(map[string]any, len(f.Options))
	for k, v := range f.Options {
		opts[k] = v
	}
	for _, k := range []string{"formula", "url"} {
		if src, ok := opts[k].(string); ok {
			opts[k] = displayFormula(src, s)
		}
	}
	f.Options = opts
	return f
}

func (r *Repository) loadUsers(ctx context.Context, s *schema, org string, recs []*rec) error {
	set := map[string]bool{}
	var ids []string
	for _, x := range recs {
		for _, id := range []string{x.CreatedBy, x.UpdatedBy} {
			if id != "" && !set[id] && s.users[id] == "" {
				set[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, COALESCE(NULLIF(display_name,''),email) FROM adrient.users WHERE org_id=$1 AND id = ANY($2::uuid[])`, org, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, n string
		if err := rows.Scan(&id, &n); err != nil {
			return err
		}
		s.users[id] = n
	}
	return rows.Err()
}

// sanitize makes a value safe for JSON (NaN and Inf become formula errors).
func sanitize(v any) any {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return FormulaError{"#NUM!"}
		}
	case []any:
		for i, e := range x {
			x[i] = sanitize(e)
		}
	}
	return v
}

// render builds API records with every wanted field computed. want nil = all fields.
func (r *Repository) render(ctx context.Context, e *engine, tableID string, recs []*rec, want []string) ([]Record, error) {
	if want == nil {
		for _, f := range e.sch.byTbl[tableID] {
			want = append(want, f.ID)
		}
	}
	if err := r.loadUsers(ctx, e.sch, e.org, recs); err != nil {
		return nil, err
	}
	vals, err := e.values(ctx, tableID, recs, want)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(recs))
	for _, x := range recs {
		fv := map[string]any{}
		for _, id := range want {
			if v, ok := vals[x.ID][id]; ok && v != nil {
				fv[id] = sanitize(v)
			}
		}
		out = append(out, Record{ID: x.ID, Seq: x.Seq, Fields: fv, CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt,
			CreatedBy: e.sch.users[x.CreatedBy], UpdatedBy: e.sch.users[x.UpdatedBy]})
	}
	return out, nil
}

func (r *Repository) newEngine(ctx context.Context, a knowkit.Actor, baseID string) (*engine, error) {
	s, err := loadSchema(ctx, r.pool, a.Org, baseID)
	if err != nil {
		return nil, err
	}
	return &engine{pool: r.pool, org: a.Org, sch: s, now: r.now()}, nil
}

const recCols = `id::text, seq, data, COALESCE(created_by::text,''), COALESCE(updated_by::text,''), created_at, updated_at`

func scanRec(rows pgx.Rows) (*rec, error) {
	x := &rec{}
	if err := rows.Scan(&x.ID, &x.Seq, &x.Data, &x.CreatedBy, &x.UpdatedBy, &x.CreatedAt, &x.UpdatedAt); err != nil {
		return nil, err
	}
	if x.Data == nil {
		x.Data = map[string]any{}
	}
	return x, nil
}

// storedScalar reports whether a field can be ordered by SQL straight from the JSON data.
func storedScalar(f Field) (cast string, ok bool) {
	switch f.Type {
	case TText, TLongText, TPhone, TEmail, TURL, TBarcode, TDate:
		return "text", true
	case TNumber, TCurrency, TPercent, TDuration, TRating:
		return "numeric", true
	}
	return "", false
}

// QueryResult is one page of records.
type QueryResult struct {
	Records []Record `json:"records"`
	Total   int      `json:"total"`
	Offset  int      `json:"offset"`
}

// ListRecords returns the records of a table for a view query.
func (r *Repository) ListRecords(ctx context.Context, a knowkit.Actor, tableID string, q Query) (*QueryResult, error) {
	if q.Limit > 2000 {
		q.Limit = 2000
	}
	return r.list(ctx, a, tableID, q, 2000)
}

// list is ListRecords with a caller-chosen page limit (exports read whole tables).
func (r *Repository) list(ctx context.Context, a knowkit.Actor, tableID string, q Query, maxLimit int) (*QueryResult, error) {
	b, err := r.tableBase(ctx, a, tableID, RoleView)
	if err != nil {
		return nil, err
	}
	e, err := r.newEngine(ctx, a, b.ID)
	if err != nil {
		return nil, err
	}
	if err := q.validate(e.sch, tableID); err != nil {
		return nil, err
	}
	if q.Limit <= 0 || q.Limit > maxLimit {
		q.Limit = 200
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	filter := e.sch.compile(q.Filter, e.now)
	search := strings.ToLower(strings.TrimSpace(q.Search))
	var total int
	// Fast path: filter, search and sort on stored scalar fields run in Postgres.
	if plan, ok := e.sch.plan(q, tableID, search, e.now, r.naturalCollation(ctx)); ok && !r.forceMemory && (search == "" || !hasUnstoredFields(e.sch, tableID)) {
		where := plan.where
		base := append([]any{a.Org, tableID}, plan.args...)
		if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.records_records WHERE org_id=$1 AND table_id=$2 AND `+where, base[:2+plan.nWhere]...).Scan(&total); err != nil {
			return nil, err
		}
		n := len(base)
		rows, err := r.pool.Query(ctx, fmt.Sprintf(`SELECT `+recCols+` FROM adrient.records_records WHERE org_id=$1 AND table_id=$2 AND %s ORDER BY %s LIMIT $%d OFFSET $%d`, where, plan.order, n+1, n+2),
			append(base, q.Limit, q.Offset)...)
		if err != nil {
			return nil, err
		}
		var page []*rec
		for rows.Next() {
			x, err := scanRec(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			page = append(page, x)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		out, err := r.render(ctx, e, tableID, page, nil)
		if err != nil {
			return nil, err
		}
		return &QueryResult{Records: out, Total: total, Offset: q.Offset}, nil
	}
	// General path: load the table, compute what the query reads, filter and sort in memory.
	rows, err := r.pool.Query(ctx, `SELECT `+recCols+` FROM adrient.records_records WHERE org_id=$1 AND table_id=$2 ORDER BY seq`, a.Org, tableID)
	if err != nil {
		return nil, err
	}
	var all []*rec
	for rows.Next() {
		x, err := scanRec(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, x)
	}
	rows.Close()
	need := q.fieldsNeeded()
	if search != "" {
		for _, f := range e.sch.byTbl[tableID] {
			need = append(need, f.ID)
		}
	}
	if err := r.loadUsers(ctx, e.sch, a.Org, all); err != nil {
		return nil, err
	}
	vals, err := e.values(ctx, tableID, all, need)
	if err != nil {
		return nil, err
	}
	nat := make(map[string]map[string]any, len(all))
	for _, x := range all {
		m := map[string]any{}
		for _, id := range need {
			f, _ := e.sch.field(id)
			m[id] = nativeOrComputed(f, vals[x.ID][id])
		}
		nat[x.ID] = m
	}
	kept := all[:0:0]
	for _, x := range all {
		if filter != nil && !filter(nat[x.ID], x.Data) {
			continue
		}
		if search != "" {
			hit := false
			for _, f := range e.sch.byTbl[tableID] {
				v := vals[x.ID][f.ID]
				var txt string
				if f.Type == TLink {
					l, _ := v.([]any)
					var names []string
					for _, it := range l {
						if m, ok := it.(map[string]any); ok {
							names = append(names, toStr(m["name"]))
						}
					}
					txt = strings.Join(names, " ")
				} else if IsComputed(f.Type) {
					txt = toStr(v)
				} else {
					txt = Display(f, v, nil)
				}
				if strings.Contains(strings.ToLower(txt), search) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		kept = append(kept, x)
	}
	keys := make([]SortKey, 0, len(q.GroupBy)+len(q.Sort))
	for _, g := range q.GroupBy {
		keys = append(keys, SortKey{Field: g})
	}
	keys = append(keys, q.Sort...)
	if len(keys) > 0 {
		sk := make(map[string][]sortKey, len(kept))
		for _, x := range kept {
			ks := make([]sortKey, len(keys))
			for i, k := range keys {
				ks[i] = keyOf(nat[x.ID][k.Field])
			}
			sk[x.ID] = ks
		}
		sort.SliceStable(kept, func(i, j int) bool {
			a, b := sk[kept[i].ID], sk[kept[j].ID]
			for n, k := range keys {
				c := cmpKeys(a[n], b[n])
				if k.Desc && a[n].kind != 0 && b[n].kind != 0 {
					c = -c
				}
				if c != 0 {
					return c < 0
				}
			}
			return false
		})
	}
	total = len(kept)
	lo, hi := q.Offset, q.Offset+q.Limit
	if lo > total {
		lo = total
	}
	if hi > total {
		hi = total
	}
	out, err := r.render(ctx, e, tableID, kept[lo:hi], nil)
	if err != nil {
		return nil, err
	}
	return &QueryResult{Records: out, Total: total, Offset: q.Offset}, nil
}

func dirSQL(desc bool) string {
	if desc {
		return "DESC"
	}
	return "ASC"
}

// GetRecord returns one record with all fields.
func (r *Repository) GetRecord(ctx context.Context, a knowkit.Actor, id string) (*Record, string, error) {
	tableID, err := r.recordTable(ctx, a, id)
	if err != nil {
		return nil, "", err
	}
	b, err := r.tableBase(ctx, a, tableID, RoleView)
	if err != nil {
		return nil, "", err
	}
	e, err := r.newEngine(ctx, a, b.ID)
	if err != nil {
		return nil, "", err
	}
	got, err := e.loadRecs(ctx, tableID, []string{id})
	if err != nil || got[id] == nil {
		return nil, "", knowkit.ErrNotFound
	}
	out, err := r.render(ctx, e, tableID, []*rec{got[id]}, nil)
	if err != nil {
		return nil, "", err
	}
	return &out[0], tableID, nil
}

func (r *Repository) recordTable(ctx context.Context, a knowkit.Actor, id string) (string, error) {
	if !knowkit.IsUUID(id) {
		return "", knowkit.ErrNotFound
	}
	var t string
	if err := r.pool.QueryRow(ctx, `SELECT table_id::text FROM adrient.records_records WHERE org_id=$1 AND id=$2`, a.Org, id).Scan(&t); err != nil {
		return "", knowkit.ErrNotFound
	}
	return t, nil
}

// prepared is a validated input: stored values plus link lists.
type prepared struct {
	set   map[string]any
	clear []string
	links map[string][]string
}

func (r *Repository) prepare(ctx context.Context, a knowkit.Actor, e *engine, tableID string, in map[string]any, creating bool) (*prepared, error) {
	p := &prepared{set: map[string]any{}, links: map[string][]string{}}
	for key, raw := range in {
		var f Field
		var ok bool
		if knowkit.IsUUID(key) {
			f, ok = e.sch.field(key)
			ok = ok && f.TableID == tableID
		} else {
			f, ok = e.sch.fieldByName(tableID, key)
		}
		if !ok {
			return nil, knowkit.Invalid("unknown field %q", key)
		}
		if IsComputed(f.Type) {
			return nil, knowkit.Invalid("%q is calculated and cannot be edited", f.Name)
		}
		if f.Type == TLink {
			var ids []string
			switch x := raw.(type) {
			case nil:
			case []any:
				for _, it := range x {
					switch y := it.(type) {
					case string:
						ids = append(ids, y)
					case map[string]any:
						s, _ := y["id"].(string)
						ids = append(ids, s)
					}
				}
			default:
				return nil, knowkit.Invalid("%q: expected a list of record ids", f.Name)
			}
			seen := map[string]bool{}
			uniq := ids[:0:0]
			for _, id := range ids {
				if !knowkit.IsUUID(id) {
					return nil, knowkit.Invalid("%q: %q is not a record id", f.Name, id)
				}
				if !seen[id] {
					seen[id] = true
					uniq = append(uniq, id)
				}
			}
			if !f.optBool("allow_multiple") && len(uniq) > 1 {
				return nil, knowkit.Invalid("%q takes a single record", f.Name)
			}
			_, _, tid := e.sch.linkInfo(f)
			if len(uniq) > 0 {
				var n int
				if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.records_records WHERE org_id=$1 AND table_id=$2 AND id = ANY($3::uuid[])`, a.Org, tid, uniq).Scan(&n); err != nil {
					return nil, err
				}
				if n != len(uniq) {
					return nil, knowkit.Invalid("%q: a linked record does not exist in the linked table", f.Name)
				}
			}
			p.links[f.ID] = uniq
			continue
		}
		v, err := Coerce(f, raw)
		if err != nil {
			return nil, err
		}
		if f.Type == TAttachment && v != nil && r.Deps.FileMeta != nil {
			l, _ := v.([]any)
			for _, it := range l {
				m := it.(map[string]any)
				name, mime, size, err := r.Deps.FileMeta(ctx, a.Org, a.User, m["file_id"].(string))
				if err != nil {
					return nil, knowkit.Invalid("%q: a Drive file cannot be attached (%v)", f.Name, err)
				}
				m["name"], m["mime"], m["size"] = name, mime, float64(size)
			}
		}
		if v == nil {
			p.clear = append(p.clear, f.ID)
		} else {
			p.set[f.ID] = v
		}
	}
	return p, nil
}

func (r *Repository) setLinks(ctx context.Context, tx pgx.Tx, a knowkit.Actor, e *engine, recID string, links map[string][]string) error {
	for fid, ids := range links {
		f, _ := e.sch.field(fid)
		store, reverse, _ := e.sch.linkInfo(f)
		if reverse {
			if _, err := tx.Exec(ctx, `DELETE FROM adrient.records_links WHERE field_id=$1 AND dst=$2`, store, recID); err != nil {
				return err
			}
			// The forward side takes one record only: moving a record here takes it from its old owner.
			if len(ids) > 0 && !forwardMulti(e, store) {
				if _, err := tx.Exec(ctx, `DELETE FROM adrient.records_links WHERE field_id=$1 AND src = ANY($2::uuid[])`, store, ids); err != nil {
					return err
				}
			}
		} else if _, err := tx.Exec(ctx, `DELETE FROM adrient.records_links WHERE field_id=$1 AND src=$2`, store, recID); err != nil {
			return err
		}
		for i, id := range ids {
			src, dst := recID, id
			if reverse {
				src, dst = id, recID
			}
			// A single-link forward field: the target keeps only one source when the partner is single too (not enforced).
			if _, err := tx.Exec(ctx, `INSERT INTO adrient.records_links (org_id, field_id, src, dst, pos) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, a.Org, store, src, dst, i); err != nil {
				return err
			}
		}
	}
	return nil
}

// CreateRecords inserts up to MaxBatch records.
func (r *Repository) CreateRecords(ctx context.Context, a knowkit.Actor, tableID string, ins []RecordInput) ([]Record, error) {
	b, err := r.tableBase(ctx, a, tableID, RoleEdit)
	if err != nil {
		return nil, err
	}
	if len(ins) == 0 {
		return []Record{}, nil
	}
	if len(ins) > MaxBatch {
		return nil, knowkit.Invalid("at most %d records per request", MaxBatch)
	}
	e, err := r.newEngine(ctx, a, b.ID)
	if err != nil {
		return nil, err
	}
	var have int64
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.records_records WHERE table_id=$1`, tableID).Scan(&have)
	if have+int64(len(ins)) > MaxRecordsPerTable {
		return nil, knowkit.Invalid("a table can hold at most %d records", MaxRecordsPerTable)
	}
	preps := make([]*prepared, len(ins))
	for i, in := range ins {
		p, err := r.prepare(ctx, a, e, tableID, in.Fields, true)
		if err != nil {
			return nil, err
		}
		preps[i] = p
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var next int64
	if err := tx.QueryRow(ctx, `UPDATE adrient.records_tables SET next_seq = next_seq + $2 WHERE id=$1 RETURNING next_seq - $2`, tableID, len(ins)).Scan(&next); err != nil {
		return nil, err
	}
	ids := make([]string, len(ins))
	for i, p := range preps {
		raw, _ := json.Marshal(p.set)
		ids[i] = NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.records_records (id, org_id, table_id, seq, data, created_by, updated_by) VALUES ($1,$2,$3,$4,$5,$6,$6)`,
			ids[i], a.Org, tableID, next+int64(i), raw, a.User); err != nil {
			return nil, err
		}
	}
	for i, p := range preps {
		if err := r.setLinks(ctx, tx, a, e, ids[i], p.links); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	got, err := e.loadRecs(ctx, tableID, ids)
	if err != nil {
		return nil, err
	}
	rs := make([]*rec, 0, len(ids))
	for _, id := range ids {
		if x := got[id]; x != nil {
			rs = append(rs, x)
		}
	}
	return r.render(ctx, e, tableID, rs, nil)
}

// UpdateRecords applies patches. A field set to null is cleared. Every changed field is written to the history.
func (r *Repository) UpdateRecords(ctx context.Context, a knowkit.Actor, tableID string, patches []RecordInput) ([]Record, error) {
	b, err := r.tableBase(ctx, a, tableID, RoleEdit)
	if err != nil {
		return nil, err
	}
	if len(patches) > MaxBatch {
		return nil, knowkit.Invalid("at most %d records per request", MaxBatch)
	}
	e, err := r.newEngine(ctx, a, b.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(patches))
	for i, p := range patches {
		if !knowkit.IsUUID(p.ID) {
			return nil, knowkit.Invalid("each update needs a record id")
		}
		ids[i] = p.ID
	}
	existing, err := e.loadRecs(ctx, tableID, ids)
	if err != nil {
		return nil, err
	}
	preps := make([]*prepared, len(patches))
	for i, p := range patches {
		if existing[p.ID] == nil {
			return nil, knowkit.ErrNotFound
		}
		pp, err := r.prepare(ctx, a, e, tableID, p.Fields, false)
		if err != nil {
			return nil, err
		}
		preps[i] = pp
	}
	// Old link values for the history.
	oldLinks := map[string]map[string][]string{}
	for i := range patches {
		for fid := range preps[i].links {
			f, _ := e.sch.field(fid)
			m, err := e.linkIDs(ctx, f, []string{patches[i].ID})
			if err != nil {
				return nil, err
			}
			if oldLinks[patches[i].ID] == nil {
				oldLinks[patches[i].ID] = map[string][]string{}
			}
			oldLinks[patches[i].ID][fid] = m[patches[i].ID]
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for i, p := range patches {
		pp := preps[i]
		old := existing[p.ID]
		var changes []map[string]any
		for fid, v := range pp.set {
			f, _ := e.sch.field(fid)
			if jsonEq(old.Data[fid], v) {
				delete(pp.set, fid)
				continue
			}
			changes = append(changes, map[string]any{"field_id": fid, "field": f.Name, "old": Display(f, old.Data[fid], nil), "new": Display(f, v, nil)})
		}
		var clear []string
		for _, fid := range pp.clear {
			if old.Data[fid] == nil {
				continue
			}
			f, _ := e.sch.field(fid)
			changes = append(changes, map[string]any{"field_id": fid, "field": f.Name, "old": Display(f, old.Data[fid], nil), "new": ""})
			clear = append(clear, fid)
		}
		for fid, ids := range pp.links {
			f, _ := e.sch.field(fid)
			before := oldLinks[p.ID][fid]
			if strings.Join(before, ",") == strings.Join(ids, ",") {
				delete(pp.links, fid)
				continue
			}
			changes = append(changes, map[string]any{"field_id": fid, "field": f.Name, "old": fmt.Sprintf("%d linked", len(before)), "new": fmt.Sprintf("%d linked", len(ids))})
		}
		if len(changes) == 0 {
			continue
		}
		patchRaw, _ := json.Marshal(pp.set)
		if _, err := tx.Exec(ctx, `UPDATE adrient.records_records SET data = (data || $3::jsonb) - $4::text[], updated_by=$5, updated_at=now() WHERE org_id=$1 AND id=$2`,
			a.Org, p.ID, patchRaw, nonNil(clear), a.User); err != nil {
			return nil, err
		}
		if err := r.setLinks(ctx, tx, a, e, p.ID, pp.links); err != nil {
			return nil, err
		}
		// Reverse-side edits touch the other record too.
		hist, _ := json.Marshal(changes)
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.records_history (org_id, record_id, user_id, changes) VALUES ($1,$2,$3,$4)`, a.Org, p.ID, a.User, hist); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	got, err := e.loadRecs(ctx, tableID, ids)
	if err != nil {
		return nil, err
	}
	rs := make([]*rec, 0, len(ids))
	for _, id := range ids {
		if x := got[id]; x != nil {
			rs = append(rs, x)
		}
	}
	return r.render(ctx, e, tableID, rs, nil)
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func jsonEq(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// DeleteRecords removes records (their links go with them).
func (r *Repository) DeleteRecords(ctx context.Context, a knowkit.Actor, tableID string, ids []string) (int64, error) {
	if _, err := r.tableBase(ctx, a, tableID, RoleEdit); err != nil {
		return 0, err
	}
	for _, id := range ids {
		if !knowkit.IsUUID(id) {
			return 0, knowkit.Invalid("%q is not a record id", id)
		}
	}
	if len(ids) > 5000 {
		return 0, knowkit.Invalid("at most 5000 records per request")
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.records_records WHERE org_id=$1 AND table_id=$2 AND id = ANY($3::uuid[])`, a.Org, tableID, ids)
	if err != nil {
		return 0, err
	}
	r.audit(ctx, "record.delete", "records_table", tableID)
	return tag.RowsAffected(), nil
}

// ---- comments and history ---------------------------------------------------

// Comment is a note on a record.
type Comment struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// ListComments returns the comments of a record.
func (r *Repository) ListComments(ctx context.Context, a knowkit.Actor, recID string) ([]Comment, error) {
	tid, err := r.recordTable(ctx, a, recID)
	if err != nil {
		return nil, err
	}
	if _, err := r.tableBase(ctx, a, tid, RoleView); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT c.id::text, c.user_id::text, COALESCE(NULLIF(u.display_name,''),u.email,''), c.body, c.created_at
		FROM adrient.records_comments c LEFT JOIN adrient.users u ON u.id=c.user_id WHERE c.org_id=$1 AND c.record_id=$2 ORDER BY c.created_at`, a.Org, recID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.UserID, &c.Author, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddComment adds a comment; anyone who can edit may comment.
func (r *Repository) AddComment(ctx context.Context, a knowkit.Actor, recID, body string) (*Comment, error) {
	tid, err := r.recordTable(ctx, a, recID)
	if err != nil {
		return nil, err
	}
	if _, err := r.tableBase(ctx, a, tid, RoleEdit); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 5000 {
		return nil, knowkit.Invalid("a comment needs 1 to 5000 characters")
	}
	c := &Comment{UserID: a.User, Body: body}
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.records_comments (org_id, record_id, user_id, body) VALUES ($1,$2,$3,$4) RETURNING id::text, created_at`, a.Org, recID, a.User, body).Scan(&c.ID, &c.CreatedAt)
	return c, err
}

// HistoryItem is one edit.
type HistoryItem struct {
	At      time.Time        `json:"at"`
	User    string           `json:"user"`
	Changes []map[string]any `json:"changes"`
}

// History lists the recent edits of a record.
func (r *Repository) History(ctx context.Context, a knowkit.Actor, recID string) ([]HistoryItem, error) {
	tid, err := r.recordTable(ctx, a, recID)
	if err != nil {
		return nil, err
	}
	if _, err := r.tableBase(ctx, a, tid, RoleView); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT h.at, COALESCE(NULLIF(u.display_name,''),u.email,''), h.changes FROM adrient.records_history h LEFT JOIN adrient.users u ON u.id=h.user_id
		WHERE h.org_id=$1 AND h.record_id=$2 ORDER BY h.at DESC LIMIT 100`, a.Org, recID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryItem{}
	for rows.Next() {
		var h HistoryItem
		if err := rows.Scan(&h.At, &h.User, &h.Changes); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func forwardMulti(e *engine, fieldID string) bool {
	f, ok := e.sch.field(fieldID)
	return !ok || f.optBool("allow_multiple")
}

// hasUnstoredFields reports whether the table has links, attachments or calculated fields,
// which a SQL text search cannot see.
func hasUnstoredFields(s *schema, tableID string) bool {
	for _, f := range s.byTbl[tableID] {
		if IsComputed(f.Type) || f.Type == TLink {
			return true
		}
	}
	return false
}

// search matches the text of every stored field.
func (b *sqlBuilder) search(tableID, q string) string {
	like := b.arg("%" + escLike(q) + "%")
	var parts []string
	for _, f := range b.s.byTbl[tableID] {
		switch f.Type {
		case TSelect:
			var ids []string
			for _, c := range f.Choices() {
				if strings.Contains(strings.ToLower(c.Name), q) {
					ids = append(ids, c.ID)
				}
			}
			if len(ids) > 0 {
				parts = append(parts, b.jx(f)+" = ANY("+b.arg(ids)+"::text[])")
			}
		case TMulti:
			var ids []string
			for _, c := range f.Choices() {
				if strings.Contains(strings.ToLower(c.Name), q) {
					ids = append(ids, c.ID)
				}
			}
			if len(ids) > 0 {
				parts = append(parts, fmt.Sprintf("COALESCE(data->'%s' ?| %s::text[], false)", f.ID, b.arg(ids)))
			}
		case TAttachment:
			parts = append(parts, fmt.Sprintf("(data->'%s')::text ILIKE %s", f.ID, like))
		case TCheckbox:
		default:
			parts = append(parts, b.jx(f)+" ILIKE "+like)
		}
	}
	if len(parts) == 0 {
		return "FALSE"
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// naturalCollation reports whether the ICU natural-order collation exists.
func (r *Repository) naturalCollation(ctx context.Context) bool {
	r.natOnce.Do(func() {
		var n int
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM pg_collation WHERE collname='records_natural'`).Scan(&n)
		r.natural = n > 0
	})
	return r.natural
}
