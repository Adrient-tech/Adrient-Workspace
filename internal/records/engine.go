// and fields.js (lookup, rollup, count and formula semantics, rollup function list).

package records

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Table is one table of a base.
type Table struct {
	ID          string  `json:"id"`
	BaseID      string  `json:"base_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Position    int     `json:"position"`
	Fields      []Field `json:"fields,omitempty"`
	Views       []View  `json:"views,omitempty"`
	RecordCount int64   `json:"record_count"`
}

// View is a saved way to look at a table.
type View struct {
	ID       string         `json:"id"`
	TableID  string         `json:"table_id"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Config   map[string]any `json:"config"`
	Position int            `json:"position"`
}

// rec is a stored record.
type rec struct {
	ID        string
	Seq       int64
	Data      map[string]any
	CreatedBy string
	UpdatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// schema is the field layout of a whole base, loaded once per request.
type schema struct {
	tables map[string]*Table
	fields map[string]Field // by id
	byTbl  map[string][]Field
	// users maps user id to display name (for created_by / modified_by).
	users map[string]string
}

func (s *schema) field(id string) (Field, bool) { f, ok := s.fields[id]; return f, ok }

func (s *schema) primary(tableID string) (Field, bool) {
	l := s.byTbl[tableID]
	if len(l) == 0 {
		return Field{}, false
	}
	return l[0], true
}

func (s *schema) fieldByName(tableID, name string) (Field, bool) {
	for _, f := range s.byTbl[tableID] {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return Field{}, false
}

// linkInfo says where the links of a link field are stored.
// A reverse field reads the links of its partner backwards.
func (s *schema) linkInfo(f Field) (storeID string, reverse bool, target string) {
	target = f.optStr("table_id")
	if inv := f.optStr("inverse_field_id"); inv != "" && f.optBool("reverse") {
		return inv, true, target
	}
	return f.ID, false, target
}

func loadSchema(ctx context.Context, pool *pgxpool.Pool, org, baseID string) (*schema, error) {
	s := &schema{tables: map[string]*Table{}, fields: map[string]Field{}, byTbl: map[string][]Field{}, users: map[string]string{}}
	rows, err := pool.Query(ctx, `SELECT id::text, name, description, position FROM adrient.records_tables WHERE org_id=$1 AND base_id=$2 ORDER BY position, created_at`, org, baseID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		t := &Table{BaseID: baseID}
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Position); err != nil {
			rows.Close()
			return nil, err
		}
		s.tables[t.ID] = t
	}
	rows.Close()
	frows, err := pool.Query(ctx, `SELECT id::text, table_id::text, name, type, description, options, position FROM adrient.records_fields WHERE org_id=$1 AND base_id=$2 ORDER BY position, created_at`, org, baseID)
	if err != nil {
		return nil, err
	}
	defer frows.Close()
	for frows.Next() {
		var f Field
		if err := frows.Scan(&f.ID, &f.TableID, &f.Name, &f.Type, &f.Description, &f.Options, &f.Position); err != nil {
			return nil, err
		}
		if f.Options == nil {
			f.Options = map[string]any{}
		}
		s.fields[f.ID] = f
		s.byTbl[f.TableID] = append(s.byTbl[f.TableID], f)
	}
	for id, l := range s.byTbl {
		if t := s.tables[id]; t != nil {
			t.Fields = l
		}
	}
	return s, frows.Err()
}

// ---- native values ---------------------------------------------------------

// native converts a stored value to the form formulas, filters and sorts use.
func native(f Field, v any) any {
	if v == nil {
		return nil
	}
	switch f.Type {
	case TSelect:
		id, _ := v.(string)
		for _, c := range f.Choices() {
			if c.ID == id {
				return c.Name
			}
		}
		return id
	case TMulti:
		l, _ := v.([]any)
		out := make([]any, 0, len(l))
		for _, e := range l {
			id, _ := e.(string)
			n := id
			for _, c := range f.Choices() {
				if c.ID == id {
					n = c.Name
				}
			}
			out = append(out, n)
		}
		return out
	case TDate:
		if s, ok := v.(string); ok {
			if t, ok := parseDate(s); ok {
				return t
			}
		}
		return nil
	case TAttachment:
		l, _ := v.([]any)
		out := make([]any, 0, len(l))
		for _, e := range l {
			if m, ok := e.(map[string]any); ok {
				n, _ := m["name"].(string)
				out = append(out, n)
			}
		}
		return out
	}
	return v
}

// ---- compute ---------------------------------------------------------------

type engine struct {
	pool  *pgxpool.Pool
	org   string
	sch   *schema
	now   time.Time
	depth int
}

type target struct {
	id   string
	name string
}

// load fetches records of one table by id, keeping only existing ones.
func (e *engine) loadRecs(ctx context.Context, tableID string, ids []string) (map[string]*rec, error) {
	out := map[string]*rec{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := e.pool.Query(ctx, `SELECT id::text, seq, data, COALESCE(created_by::text,''), COALESCE(updated_by::text,''), created_at, updated_at
		FROM adrient.records_records WHERE org_id=$1 AND table_id=$2 AND id = ANY($3::uuid[])`, e.org, tableID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		r := &rec{}
		if err := rows.Scan(&r.ID, &r.Seq, &r.Data, &r.CreatedBy, &r.UpdatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if r.Data == nil {
			r.Data = map[string]any{}
		}
		out[r.ID] = r
	}
	return out, rows.Err()
}

// linkIDs returns, per source record, the ordered ids linked through field f.
func (e *engine) linkIDs(ctx context.Context, f Field, recIDs []string) (map[string][]string, error) {
	store, reverse, _ := e.sch.linkInfo(f)
	out := map[string][]string{}
	if len(recIDs) == 0 {
		return out, nil
	}
	q := `SELECT src::text, dst::text FROM adrient.records_links WHERE org_id=$1 AND field_id=$2 AND src = ANY($3::uuid[]) ORDER BY pos, dst`
	if reverse {
		q = `SELECT dst::text, src::text FROM adrient.records_links WHERE org_id=$1 AND field_id=$2 AND dst = ANY($3::uuid[]) ORDER BY pos, src`
	}
	rows, err := e.pool.Query(ctx, q, e.org, store, recIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out[a] = append(out[a], b)
	}
	return out, rows.Err()
}

// values computes the requested fields for recs of one table. The result maps
// record id to field id to value. Link values are []map{id,name}; every other
// value is stored form for stored fields and native form for computed ones.
func (e *engine) values(ctx context.Context, tableID string, recs []*rec, want []string) (map[string]map[string]any, error) {
	if e.depth > 5 {
		return nil, knowkit.Invalid("links are nested too deeply")
	}
	out := make(map[string]map[string]any, len(recs))
	ids := make([]string, len(recs))
	for i, r := range recs {
		ids[i] = r.ID
		out[r.ID] = map[string]any{}
	}
	// Expand the wanted set with everything formulas read.
	need := map[string]bool{}
	var add func(id string)
	add = func(id string) {
		if need[id] {
			return
		}
		f, ok := e.sch.field(id)
		if !ok || f.TableID != tableID {
			return
		}
		need[id] = true
		switch f.Type {
		case TFormula, TButton:
			src := f.optStr("formula")
			if f.Type == TButton {
				src = f.optStr("url")
			}
			if src == "" {
				return
			}
			if fm, err := ParseFormula(src); err == nil {
				for _, ref := range fm.Refs() {
					add(ref)
				}
			}
		}
	}
	for _, id := range want {
		add(id)
	}
	// Link-based fields first (link, lookup, rollup, count).
	linkCache := map[string]map[string][]string{} // link field id -> rec -> ids
	linkOf := func(lf Field) (map[string][]string, error) {
		if m, ok := linkCache[lf.ID]; ok {
			return m, nil
		}
		m, err := e.linkIDs(ctx, lf, ids)
		if err != nil {
			return nil, err
		}
		linkCache[lf.ID] = m
		return m, nil
	}
	targetRecs := map[string]map[string]*rec{} // target table -> id -> rec
	fetchTargets := func(tid string, m map[string][]string) error {
		set := map[string]bool{}
		var list []string
		for _, l := range m {
			for _, id := range l {
				if !set[id] && targetRecs[tid][id] == nil {
					set[id] = true
					list = append(list, id)
				}
			}
		}
		got, err := e.loadRecs(ctx, tid, list)
		if err != nil {
			return err
		}
		if targetRecs[tid] == nil {
			targetRecs[tid] = map[string]*rec{}
		}
		for k, v := range got {
			targetRecs[tid][k] = v
		}
		return nil
	}
	sub := &engine{pool: e.pool, org: e.org, sch: e.sch, now: e.now, depth: e.depth + 1}
	// subVals computes field fid of the given target records.
	subVals := func(tid, fid string, ids []string) (map[string]any, error) {
		var rs []*rec
		for _, id := range ids {
			if r := targetRecs[tid][id]; r != nil {
				rs = append(rs, r)
			}
		}
		v, err := sub.values(ctx, tid, rs, []string{fid})
		if err != nil {
			return nil, err
		}
		res := map[string]any{}
		tf, _ := e.sch.field(fid)
		for id, m := range v {
			res[id] = nativeOrComputed(tf, m[fid])
		}
		return res, nil
	}
	for id := range need {
		f, _ := e.sch.field(id)
		switch f.Type {
		case TLink:
			m, err := linkOf(f)
			if err != nil {
				return nil, err
			}
			_, _, tid := e.sch.linkInfo(f)
			if err := fetchTargets(tid, m); err != nil {
				return nil, err
			}
			prim, ok := e.sch.primary(tid)
			names := map[string]any{}
			if ok {
				var all []string
				for k := range targetRecs[tid] {
					all = append(all, k)
				}
				pv, err := subVals(tid, prim.ID, all)
				if err != nil {
					return nil, err
				}
				names = pv
			}
			for _, r := range recs {
				l := make([]any, 0, len(m[r.ID]))
				for _, did := range m[r.ID] {
					if targetRecs[tid][did] == nil {
						continue
					}
					l = append(l, map[string]any{"id": did, "name": toStr(names[did])})
				}
				out[r.ID][f.ID] = l
			}
		case TLookup, TRollup, TCount:
			lf, ok := e.sch.field(f.optStr("link_field_id"))
			if !ok || lf.TableID != tableID || lf.Type != TLink {
				for _, r := range recs {
					out[r.ID][f.ID] = FormulaError{"the link field is missing"}
				}
				continue
			}
			m, err := linkOf(lf)
			if err != nil {
				return nil, err
			}
			_, _, tid := e.sch.linkInfo(lf)
			if f.Type == TCount {
				if err := fetchTargets(tid, m); err != nil {
					return nil, err
				}
				for _, r := range recs {
					n := 0
					for _, did := range m[r.ID] {
						if targetRecs[tid][did] != nil {
							n++
						}
					}
					out[r.ID][f.ID] = float64(n)
				}
				continue
			}
			tf, ok := e.sch.field(f.optStr("target_field_id"))
			if !ok || tf.TableID != tid {
				for _, r := range recs {
					out[r.ID][f.ID] = FormulaError{"the looked-up field is missing"}
				}
				continue
			}
			if err := fetchTargets(tid, m); err != nil {
				return nil, err
			}
			var all []string
			for k := range targetRecs[tid] {
				all = append(all, k)
			}
			tv, err := subVals(tid, tf.ID, all)
			if err != nil {
				return nil, err
			}
			for _, r := range recs {
				var vals []any
				for _, did := range m[r.ID] {
					if targetRecs[tid][did] != nil {
						vals = append(vals, tv[did])
					}
				}
				if f.Type == TLookup {
					if vals == nil {
						vals = []any{}
					}
					out[r.ID][f.ID] = vals
				} else {
					out[r.ID][f.ID] = rollup(f.optStr("fn"), vals)
				}
			}
		case TAutonumber:
			for _, r := range recs {
				out[r.ID][f.ID] = float64(r.Seq)
			}
		case TCreatedTime:
			for _, r := range recs {
				out[r.ID][f.ID] = r.CreatedAt.UTC()
			}
		case TModifiedAt:
			for _, r := range recs {
				out[r.ID][f.ID] = r.UpdatedAt.UTC()
			}
		case TCreatedBy:
			for _, r := range recs {
				out[r.ID][f.ID] = e.sch.users[r.CreatedBy]
			}
		case TModifiedBy:
			for _, r := range recs {
				out[r.ID][f.ID] = e.sch.users[r.UpdatedBy]
			}
		}
	}
	// Stored fields.
	for id := range need {
		f, _ := e.sch.field(id)
		if IsComputed(f.Type) || f.Type == TLink {
			continue
		}
		for _, r := range recs {
			out[r.ID][f.ID] = r.Data[f.ID]
		}
	}
	// Formulas and buttons, with memo and cycle detection per record.
	type fkey struct{ rec, field string }
	done := map[fkey]bool{}
	busy := map[fkey]bool{}
	byRec := map[string]*rec{}
	for _, r := range recs {
		byRec[r.ID] = r
	}
	var evalField func(r *rec, f Field) any
	getNative := func(r *rec, fid string) any {
		f, ok := e.sch.field(fid)
		if !ok || f.TableID != tableID {
			return nil
		}
		if f.Type == TFormula || f.Type == TButton {
			return evalField(r, f)
		}
		return nativeOrComputed(f, out[r.ID][fid])
	}
	evalField = func(r *rec, f Field) any {
		k := fkey{r.ID, f.ID}
		if done[k] {
			return out[r.ID][f.ID]
		}
		if busy[k] {
			return FormulaError{"circular reference"}
		}
		busy[k] = true
		src := f.optStr("formula")
		if f.Type == TButton {
			src = f.optStr("url")
		}
		var v any
		if src == "" {
			v = nil
		} else if fm, err := ParseFormula(src); err != nil {
			v = FormulaError{err.Error()}
		} else {
			v = fm.Eval(&fctx{now: e.now, created: r.CreatedAt.UTC(), updated: r.UpdatedAt.UTC(), recID: r.ID,
				get: func(fid string) any { return getNative(r, fid) }})
		}
		busy[k] = false
		done[k] = true
		out[r.ID][f.ID] = v
		return v
	}
	for id := range need {
		f, _ := e.sch.field(id)
		if f.Type == TFormula || f.Type == TButton {
			for _, r := range recs {
				evalField(r, f)
			}
		}
	}
	return out, nil
}

// nativeOrComputed converts a value produced by values(): computed fields are
// already native; stored fields are converted with native().
func nativeOrComputed(f Field, v any) any {
	if IsComputed(f.Type) {
		if l, ok := v.([]any); ok {
			return l
		}
		return v
	}
	if f.Type == TLink {
		l, _ := v.([]any)
		out := make([]any, 0, len(l))
		for _, e := range l {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m["name"])
			}
		}
		return out
	}
	return native(f, v)
}

// rollup applies a rollup function to the looked-up values.
func rollup(fn string, vals []any) any {
	flat := flatten(vals)
	switch fn {
	case "count_all":
		return float64(len(flat))
	case "count":
		n := 0
		for _, v := range flat {
			if !isEmptyVal(v) {
				n++
			}
		}
		return float64(n)
	case "count_unique":
		seen := map[string]bool{}
		for _, v := range flat {
			if !isEmptyVal(v) {
				seen[toStr(v)] = true
			}
		}
		return float64(len(seen))
	case "concat":
		parts := []string{}
		for _, v := range flat {
			if !isEmptyVal(v) {
				parts = append(parts, toStr(v))
			}
		}
		return strings.Join(parts, ", ")
	case "unique":
		seen := map[string]bool{}
		out := []any{}
		for _, v := range flat {
			if !isEmptyVal(v) && !seen[toStr(v)] {
				seen[toStr(v)] = true
				out = append(out, v)
			}
		}
		return out
	case "and":
		for _, v := range flat {
			if !toBool(v) {
				return false
			}
		}
		return len(flat) > 0
	case "or":
		for _, v := range flat {
			if toBool(v) {
				return true
			}
		}
		return false
	case "earliest", "latest":
		var best time.Time
		found := false
		for _, v := range flat {
			t, ok := toTime(v)
			if !ok {
				continue
			}
			if !found || (fn == "earliest" && t.Before(best)) || (fn == "latest" && t.After(best)) {
				best, found = t, true
			}
		}
		if !found {
			return nil
		}
		return best
	}
	nums := numbers(flat)
	if len(nums) == 0 {
		if fn == "sum" {
			return 0.0
		}
		return nil
	}
	switch fn {
	case "sum", "avg":
		s := 0.0
		for _, n := range nums {
			s += n
		}
		if fn == "avg" {
			return s / float64(len(nums))
		}
		return s
	case "min":
		sort.Float64s(nums)
		return nums[0]
	case "max":
		sort.Float64s(nums)
		return nums[len(nums)-1]
	}
	return FormulaError{fmt.Sprintf("unknown rollup %q", fn)}
}
