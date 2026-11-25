// (operators per value kind, date modes and "within" ranges, typed sort keys); written afresh in Go.

package records

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// MarshalJSON makes formula errors travel as {"error": "..."}.
func (e FormulaError) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"error": e.Msg})
}

// Node is a filter condition or a group of conditions.
type Node struct {
	Logic string `json:"logic,omitempty"` // "and" (default) or "or", for groups
	Conds []Node `json:"conds,omitempty"`
	Field string `json:"field,omitempty"`
	Op    string `json:"op,omitempty"`
	Value any    `json:"value,omitempty"`
}

// SortKey orders by one field.
type SortKey struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc,omitempty"`
}

// Query is what a view asks of a table.
type Query struct {
	Filter  *Node     `json:"filter,omitempty"`
	Sort    []SortKey `json:"sort,omitempty"`
	GroupBy []string  `json:"group_by,omitempty"`
	Search  string    `json:"search,omitempty"`
	Offset  int       `json:"offset,omitempty"`
	Limit   int       `json:"limit,omitempty"`
}

var noValueOps = map[string]bool{"isEmpty": true, "isNotEmpty": true}

// FilterOps lists the operators the server understands.
var FilterOps = []string{"contains", "doesNotContain", "is", "isNot", "isEmpty", "isNotEmpty", "=", "!=", "<", ">", "<=", ">=",
	"isAnyOf", "isNoneOf", "hasAnyOf", "hasAllOf", "isExactly", "hasNoneOf", "isBefore", "isAfter", "isOnOrBefore", "isOnOrAfter",
	"isWithin", "filenameContains", "hasFileType"}

// fieldsNeeded lists the field ids a query reads (filter, sort, group).
func (q Query) fieldsNeeded() []string {
	set := map[string]bool{}
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Field != "" {
			set[n.Field] = true
		}
		for i := range n.Conds {
			walk(&n.Conds[i])
		}
	}
	walk(q.Filter)
	for _, s := range q.Sort {
		set[s.Field] = true
	}
	for _, g := range q.GroupBy {
		set[g] = true
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// validate checks field ids and operators.
func (q Query) validate(s *schema, tableID string) error {
	var walk func(n *Node, depth int) error
	walk = func(n *Node, depth int) error {
		if depth > 4 {
			return knowkit.Invalid("filters are nested too deeply")
		}
		if n.Field != "" {
			f, ok := s.field(n.Field)
			if !ok || f.TableID != tableID {
				return knowkit.Invalid("filter refers to an unknown field")
			}
			okOp := false
			for _, o := range FilterOps {
				okOp = okOp || o == n.Op
			}
			if !okOp {
				return knowkit.Invalid("unknown filter operator %q", n.Op)
			}
		}
		for i := range n.Conds {
			if err := walk(&n.Conds[i], depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if q.Filter != nil {
		if err := walk(q.Filter, 0); err != nil {
			return err
		}
	}
	for _, sk := range q.Sort {
		if f, ok := s.field(sk.Field); !ok || f.TableID != tableID {
			return knowkit.Invalid("sort refers to an unknown field")
		}
	}
	if len(q.GroupBy) > 3 {
		return knowkit.Invalid("group by at most 3 fields")
	}
	for _, g := range q.GroupBy {
		if f, ok := s.field(g); !ok || f.TableID != tableID {
			return knowkit.Invalid("group refers to an unknown field")
		}
	}
	return nil
}

// active reports whether a condition has what it needs; incomplete ones are ignored.
func (n *Node) active() bool {
	if n.Field == "" || n.Op == "" {
		return false
	}
	if noValueOps[n.Op] {
		return true
	}
	if b, ok := n.Value.(bool); ok && n.Op == "is" {
		_ = b
		return true
	}
	switch v := n.Value.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		mode, _ := v["mode"].(string)
		switch mode {
		case "exactDate":
			d, _ := v["date"].(string)
			return d != ""
		case "daysAgo", "daysFromNow", "pastDays", "nextDays":
			_, ok := v["n"].(float64)
			return ok
		}
		return mode != ""
	}
	return true
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func dayFrom(t time.Time, days, months int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	d := t.Day()
	if d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d+days, 0, 0, 0, 0, time.UTC)
}

func numOf(m map[string]any) int {
	n, _ := m["n"].(float64)
	return int(n)
}

func resolveDate(v any, now time.Time) (time.Time, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		if s, isStr := v.(string); isStr {
			if t, ok := parseDate(s); ok {
				return dayStart(t), true
			}
		}
		return time.Time{}, false
	}
	switch m["mode"] {
	case "today":
		return dayFrom(now, 0, 0), true
	case "tomorrow":
		return dayFrom(now, 1, 0), true
	case "yesterday":
		return dayFrom(now, -1, 0), true
	case "oneWeekAgo":
		return dayFrom(now, -7, 0), true
	case "oneWeekFromNow":
		return dayFrom(now, 7, 0), true
	case "oneMonthAgo":
		return dayFrom(now, 0, -1), true
	case "oneMonthFromNow":
		return dayFrom(now, 0, 1), true
	case "daysAgo":
		return dayFrom(now, -numOf(m), 0), true
	case "daysFromNow":
		return dayFrom(now, numOf(m), 0), true
	case "exactDate":
		s, _ := m["date"].(string)
		if t, ok := parseDate(s); ok {
			return dayStart(t), true
		}
	}
	return time.Time{}, false
}

func withinRange(v any, now time.Time) (from, to time.Time, ok bool) {
	m, isMap := v.(map[string]any)
	if !isMap {
		return
	}
	today, tomorrow := dayFrom(now, 0, 0), dayFrom(now, 1, 0)
	switch m["mode"] {
	case "pastWeek":
		return dayFrom(now, -7, 0), tomorrow, true
	case "pastMonth":
		return dayFrom(now, 0, -1), tomorrow, true
	case "pastYear":
		return dayFrom(now, 0, -12), tomorrow, true
	case "nextWeek":
		return today, dayFrom(now, 8, 0), true
	case "nextMonth":
		return today, dayFrom(now, 1, 1), true
	case "nextYear":
		return today, dayFrom(now, 1, 12), true
	case "pastDays":
		return dayFrom(now, -numOf(m), 0), tomorrow, true
	case "nextDays":
		return today, dayFrom(now, numOf(m)+1, 0), true
	}
	return
}

func fileKind(name, mime string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case mime == "application/pdf" || strings.HasSuffix(n, ".pdf"):
		return "pdf"
	case hasSuffixAny(n, ".csv", ".xlsx", ".xls", ".ods", ".tsv"):
		return "spreadsheet"
	case strings.HasPrefix(mime, "text/") || hasSuffixAny(n, ".txt", ".md", ".rtf", ".doc", ".docx"):
		return "text"
	}
	return "other"
}

func hasSuffixAny(s string, suf ...string) bool {
	for _, x := range suf {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	return false
}

// predicate builds a test over a record's native values (field id -> value).
// Text comparisons ignore case. The raw map is used for attachments.
type pred func(vals map[string]any, raw map[string]any) bool

func (s *schema) compile(n *Node, now time.Time) pred {
	if n == nil {
		return nil
	}
	if n.Field == "" { // group
		var subs []pred
		for i := range n.Conds {
			if p := s.compile(&n.Conds[i], now); p != nil {
				subs = append(subs, p)
			}
		}
		if len(subs) == 0 {
			return nil
		}
		or := strings.EqualFold(n.Logic, "or")
		return func(v, raw map[string]any) bool {
			for _, p := range subs {
				if p(v, raw) == or {
					return or
				}
			}
			return !or
		}
	}
	if !n.active() {
		return nil
	}
	f, ok := s.field(n.Field)
	if !ok {
		return nil
	}
	fid, op, q := f.ID, n.Op, n.Value
	text := func(x any) string { return strings.ToLower(toStr(x)) }
	switch op {
	case "isEmpty":
		return func(v, _ map[string]any) bool { return isEmptyVal(v[fid]) || v[fid] == false }
	case "isNotEmpty":
		return func(v, _ map[string]any) bool { return !isEmptyVal(v[fid]) && v[fid] != false }
	}
	nameOf := func(x any) string { // select option id or name to a lowercase name
		str := toStr(x)
		for _, c := range f.Choices() {
			if c.ID == str {
				return strings.ToLower(c.Name)
			}
		}
		return strings.ToLower(str)
	}
	list := func(x any) []string {
		var out []string
		switch t := x.(type) {
		case []any:
			for _, e := range t {
				out = append(out, nameOf(e))
			}
		case nil:
		default:
			out = append(out, nameOf(t))
		}
		return out
	}
	switch op {
	case "contains", "doesNotContain":
		needle := text(q)
		neg := op == "doesNotContain"
		return func(v, _ map[string]any) bool { return strings.Contains(text(v[fid]), needle) != neg }
	case "is", "isNot":
		neg := op == "isNot"
		if b, ok := q.(bool); ok {
			return func(v, _ map[string]any) bool { cur, _ := v[fid].(bool); return (cur == b) != neg }
		}
		if f.Type == TDate {
			d, ok := resolveDate(q, now)
			if !ok {
				return nil
			}
			return func(v, _ map[string]any) bool {
				t, ok := toTime(v[fid])
				return ok && dayStart(t).Equal(d) != neg
			}
		}
		if f.Type == TNumber || f.Type == TCurrency || f.Type == TPercent || f.Type == TDuration || f.Type == TRating {
			target := toNum(q)
			return func(v, _ map[string]any) bool { return (v[fid] != nil && math.Abs(toNum(v[fid])-target) < 1e-9) != neg }
		}
		want := nameOf(q)
		return func(v, _ map[string]any) bool {
			if f.Type == TSelect {
				return (nameOf(v[fid]) == want) != neg
			}
			return (text(v[fid]) == want) != neg
		}
	case "=", "!=", "<", ">", "<=", ">=":
		target := toNum(q)
		return func(v, _ map[string]any) bool {
			x, ok := v[fid].(float64)
			if !ok {
				return op == "!="
			}
			switch op {
			case "=":
				return math.Abs(x-target) < 1e-9
			case "!=":
				return math.Abs(x-target) >= 1e-9
			case "<":
				return x < target-1e-9
			case ">":
				return x > target+1e-9
			case "<=":
				return x <= target+1e-9
			}
			return x >= target-1e-9
		}
	case "isAnyOf", "isNoneOf":
		set := map[string]bool{}
		for _, x := range list(q) {
			set[x] = true
		}
		neg := op == "isNoneOf"
		return func(v, _ map[string]any) bool { return set[nameOf(v[fid])] != neg }
	case "hasAnyOf", "hasAllOf", "isExactly", "hasNoneOf":
		want := list(q)
		return func(v, _ map[string]any) bool {
			have := map[string]bool{}
			for _, x := range list(v[fid]) {
				have[x] = true
			}
			switch op {
			case "hasAnyOf":
				for _, w := range want {
					if have[w] {
						return true
					}
				}
				return false
			case "hasNoneOf":
				for _, w := range want {
					if have[w] {
						return false
					}
				}
				return true
			case "hasAllOf":
				for _, w := range want {
					if !have[w] {
						return false
					}
				}
				return true
			}
			if len(have) != len(want) {
				return false
			}
			for _, w := range want {
				if !have[w] {
					return false
				}
			}
			return true
		}
	case "isBefore", "isAfter", "isOnOrBefore", "isOnOrAfter":
		d, ok := resolveDate(q, now)
		if !ok {
			return nil
		}
		return func(v, _ map[string]any) bool {
			t, ok := toTime(v[fid])
			if !ok {
				return false
			}
			t = dayStart(t)
			switch op {
			case "isBefore":
				return t.Before(d)
			case "isAfter":
				return t.After(d)
			case "isOnOrBefore":
				return !t.After(d)
			}
			return !t.Before(d)
		}
	case "isWithin":
		from, to, ok := withinRange(q, now)
		if !ok {
			return nil
		}
		return func(v, _ map[string]any) bool {
			t, ok := toTime(v[fid])
			if !ok {
				return false
			}
			t = dayStart(t)
			return !t.Before(from) && t.Before(to)
		}
	case "filenameContains":
		needle := text(q)
		return func(_, raw map[string]any) bool {
			l, _ := raw[fid].([]any)
			for _, e := range l {
				if m, ok := e.(map[string]any); ok && strings.Contains(strings.ToLower(toStr(m["name"])), needle) {
					return true
				}
			}
			return false
		}
	case "hasFileType":
		want := text(q)
		return func(_, raw map[string]any) bool {
			l, _ := raw[fid].([]any)
			for _, e := range l {
				if m, ok := e.(map[string]any); ok && fileKind(toStr(m["name"]), toStr(m["mime"])) == want {
					return true
				}
			}
			return false
		}
	}
	return nil
}

// sortKeyOf is a comparable form of a native value.
type sortKey struct {
	kind int // 0 empty, 1 number, 2 text, 3 time
	n    float64
	s    string
}

func keyOf(v any) sortKey {
	switch x := v.(type) {
	case nil:
		return sortKey{}
	case float64:
		return sortKey{kind: 1, n: x}
	case bool:
		if x {
			return sortKey{kind: 1, n: 1}
		}
		return sortKey{kind: 1, n: 0}
	case time.Time:
		return sortKey{kind: 3, n: float64(x.UnixMilli())}
	case []any:
		if len(x) == 0 {
			return sortKey{}
		}
		return sortKey{kind: 2, s: strings.ToLower(toStr(x))}
	case string:
		if x == "" {
			return sortKey{}
		}
		return sortKey{kind: 2, s: strings.ToLower(x)}
	case FormulaError:
		return sortKey{}
	}
	return sortKey{kind: 2, s: strings.ToLower(toStr(v))}
}

// naturalLess compares text with embedded numbers in numeric order ("a2" < "a10").
func naturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if ca >= '0' && ca <= '9' && cb >= '0' && cb <= '9' {
			si := i
			for i < len(a) && a[i] >= '0' && a[i] <= '9' {
				i++
			}
			sj := j
			for j < len(b) && b[j] >= '0' && b[j] <= '9' {
				j++
			}
			na, nb := strings.TrimLeft(a[si:i], "0"), strings.TrimLeft(b[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return len(a)-i < len(b)-j
}

func cmpKeys(a, b sortKey) int {
	if a.kind != b.kind {
		if a.kind == 0 {
			return 1 // empty last
		}
		if b.kind == 0 {
			return -1
		}
		return a.kind - b.kind
	}
	switch a.kind {
	case 1, 3:
		switch {
		case a.n < b.n:
			return -1
		case a.n > b.n:
			return 1
		}
	case 2:
		switch {
		case naturalLess(a.s, b.s):
			return -1
		case naturalLess(b.s, a.s):
			return 1
		}
	}
	return 0
}
