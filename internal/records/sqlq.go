package records

import (
	"fmt"
	"strings"
	"time"
)

// sqlPlan is a query pushed down to Postgres. It is used when every filter and
// sort key is a stored scalar field; anything else is evaluated in memory.
type sqlPlan struct {
	where  string
	args   []any
	nWhere int // how many of args the WHERE clause (and search) use; the rest belong to ORDER BY
	order  string
}

type sqlBuilder struct {
	s       *schema
	now     time.Time
	args    []any
	natural bool
}

func (b *sqlBuilder) arg(v any) string {
	b.args = append(b.args, v)
	return fmt.Sprintf("$%d", len(b.args)+2) // $1 org, $2 table
}

func escLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (b *sqlBuilder) jx(f Field) string { return fmt.Sprintf("(data->>'%s')", f.ID) } // f.ID is a uuid from our own schema

// cond compiles one condition; ok=false means it needs the in-memory path.
func (b *sqlBuilder) cond(n *Node) (string, bool) {
	f, found := b.s.field(n.Field)
	if !found {
		return "", false
	}
	has := fmt.Sprintf("(data ? '%s')", f.ID)
	switch n.Op {
	case "isEmpty":
		if IsComputed(f.Type) || f.Type == TLink {
			return "", false
		}
		return "NOT " + has, true
	case "isNotEmpty":
		if IsComputed(f.Type) || f.Type == TLink {
			return "", false
		}
		return has, true
	}
	if IsComputed(f.Type) || f.Type == TLink || f.Type == TAttachment {
		return "", false
	}
	if !n.active() {
		return "TRUE", true
	}
	text := func(v any) string { return toStr(v) }
	switch f.Type {
	case TText, TLongText, TPhone, TEmail, TURL, TBarcode:
		x := b.jx(f)
		switch n.Op {
		case "contains":
			return x + " ILIKE " + b.arg("%"+escLike(text(n.Value))+"%"), true
		case "doesNotContain":
			return "COALESCE(" + x + ",'') NOT ILIKE " + b.arg("%"+escLike(text(n.Value))+"%"), true
		case "is":
			return "lower(" + x + ") = lower(" + b.arg(text(n.Value)) + ")", true
		case "isNot":
			return "lower(COALESCE(" + x + ",'')) <> lower(" + b.arg(text(n.Value)) + ")", true
		}
	case TNumber, TCurrency, TPercent, TDuration, TRating:
		x := b.jx(f) + "::numeric"
		v := toNumSafe(n.Value)
		switch n.Op {
		case "=", "is":
			return x + " = " + b.arg(v), true
		case "!=", "isNot":
			return x + " IS DISTINCT FROM " + b.arg(v), true
		case "<", ">", "<=", ">=":
			return x + " " + n.Op + " " + b.arg(v), true
		}
	case TCheckbox:
		if n.Op == "is" || n.Op == "isNot" {
			want, _ := n.Value.(bool)
			if n.Op == "isNot" {
				want = !want
			}
			if want {
				return fmt.Sprintf("COALESCE(data->'%s' = 'true'::jsonb, false)", f.ID), true
			}
			return fmt.Sprintf("NOT COALESCE(data->'%s' = 'true'::jsonb, false)", f.ID), true
		}
	case TSelect:
		ids := b.choiceIDs(f, n.Value)
		x := b.jx(f)
		switch n.Op {
		case "is", "isAnyOf":
			return x + " = ANY(" + b.arg(ids) + "::text[])", true
		case "isNot", "isNoneOf":
			return "COALESCE(" + x + " <> ALL(" + b.arg(ids) + "::text[]), true)", true
		}
	case TMulti:
		ids := b.choiceIDs(f, n.Value)
		j := fmt.Sprintf("(data->'%s')", f.ID)
		switch n.Op {
		case "hasAnyOf":
			return "COALESCE(" + j + " ?| " + b.arg(ids) + "::text[], false)", true
		case "hasAllOf":
			return "COALESCE(" + j + " ?& " + b.arg(ids) + "::text[], false)", true
		case "hasNoneOf":
			return "NOT COALESCE(" + j + " ?| " + b.arg(ids) + "::text[], false)", true
		}
	case TDate:
		day := fmt.Sprintf("substr(%s,1,10)", b.jx(f))
		d := func(t time.Time) string { return b.arg(t.Format("2006-01-02")) }
		switch n.Op {
		case "is", "isBefore", "isAfter", "isOnOrBefore", "isOnOrAfter":
			t, ok := resolveDate(n.Value, b.now)
			if !ok {
				return "", false
			}
			op := map[string]string{"is": "=", "isBefore": "<", "isAfter": ">", "isOnOrBefore": "<=", "isOnOrAfter": ">="}[n.Op]
			return day + " " + op + " " + d(t), true
		case "isWithin":
			from, to, ok := withinRange(n.Value, b.now)
			if !ok {
				return "", false
			}
			return "(" + day + " >= " + d(from) + " AND " + day + " < " + d(to) + ")", true
		}
	}
	return "", false
}

func toNumSafe(v any) float64 {
	defer func() { _ = recover() }()
	return toNum(v)
}

// choiceIDs maps option names or ids to ids; unknown names match nothing.
func (b *sqlBuilder) choiceIDs(f Field, v any) []string {
	var names []string
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			names = append(names, toStr(e))
		}
	default:
		names = []string{toStr(v)}
	}
	ids := []string{}
	for _, n := range names {
		if c, ok := f.choiceByName(n); ok {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

func (b *sqlBuilder) node(n *Node) (string, bool) {
	if n == nil {
		return "TRUE", true
	}
	if n.Field != "" {
		return b.cond(n)
	}
	var parts []string
	for i := range n.Conds {
		c := &n.Conds[i]
		if c.Field != "" && !c.active() {
			continue
		}
		s, ok := b.node(c)
		if !ok {
			return "", false
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "TRUE", true
	}
	join := " AND "
	if strings.EqualFold(n.Logic, "or") {
		join = " OR "
	}
	return "(" + strings.Join(parts, join) + ")", true
}

// order builds ORDER BY for group and sort keys on stored fields.
func (b *sqlBuilder) orderBy(keys []SortKey) (string, bool) {
	var parts []string
	for _, k := range keys {
		f, ok := b.s.field(k.Field)
		if !ok {
			return "", false
		}
		dir := dirSQL(k.Desc)
		switch f.Type {
		case TAutonumber:
			parts = append(parts, "seq "+dir)
		case TCreatedTime:
			parts = append(parts, "created_at "+dir)
		case TModifiedAt:
			parts = append(parts, "updated_at "+dir)
		case TNumber, TCurrency, TPercent, TDuration, TRating:
			parts = append(parts, b.jx(f)+"::numeric "+dir+" NULLS LAST")
		case TDate:
			parts = append(parts, b.jx(f)+" "+dir+" NULLS LAST")
		case TCheckbox:
			parts = append(parts, fmt.Sprintf("COALESCE(data->'%s' = 'true'::jsonb, false) %s", f.ID, dir))
		case TText, TLongText, TPhone, TEmail, TURL, TBarcode:
			col := ""
			if b.natural {
				col = ` COLLATE adrient.records_natural`
			}
			parts = append(parts, "lower("+b.jx(f)+")"+col+" "+dir+" NULLS LAST")
		case TSelect:
			// Order by the option's name.
			cs := f.Choices()
			if len(cs) == 0 {
				continue
			}
			var sb strings.Builder
			sb.WriteString("(CASE " + b.jx(f))
			for _, c := range cs {
				sb.WriteString(" WHEN " + b.arg(c.ID) + " THEN " + b.arg(strings.ToLower(c.Name)))
			}
			sb.WriteString(" END)")
			col := ""
			if b.natural {
				col = ` COLLATE adrient.records_natural`
			}
			parts = append(parts, sb.String()+col+" "+dir+" NULLS LAST")
		default:
			return "", false
		}
	}
	parts = append(parts, "seq")
	return strings.Join(parts, ", "), true
}

// plan tries to push the query down; ok=false means filter and sort in memory.
func (s *schema) plan(q Query, tableID, search string, now time.Time, natural bool) (*sqlPlan, bool) {
	b := &sqlBuilder{s: s, now: now, natural: natural}
	where, ok := b.node(q.Filter)
	if !ok {
		return nil, false
	}
	if search != "" {
		where += " AND " + b.search(tableID, search)
	}
	nWhere := len(b.args)
	keys := append(append([]SortKey{}, groupKeys(q.GroupBy)...), q.Sort...)
	order, ok := b.orderBy(keys)
	if !ok {
		return nil, false
	}
	return &sqlPlan{where: where, args: b.args, nWhere: nWhere, order: order}, true
}

func groupKeys(g []string) []SortKey {
	out := make([]SortKey, len(g))
	for i, f := range g {
		out[i] = SortKey{Field: f}
	}
	return out
}
