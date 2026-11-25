// (function set, argument rules and error behaviour); the evaluator itself is new Go code.

package records

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// FormulaError is the value of a formula that failed; it is shown as #ERROR!.
type FormulaError struct{ Msg string }

func (e FormulaError) Error() string { return e.Msg }

// Fn is a spreadsheet function over evaluated arguments.
type fnImpl func(c *fctx, args []any) any

// fctx is what a formula sees: field values by id plus record metadata.
type fctx struct {
	get     func(fieldID string) any // value of a field in this record
	now     time.Time
	created time.Time
	updated time.Time
	recID   string
}

type node interface{}
type (
	numNode float64
	strNode string
	refNode string
	unNode  struct {
		op string
		x  node
	}
	binNode struct {
		op   string
		a, b node
	}
	callNode struct {
		name string
		args []node
	}
)

// Formula is a parsed formula.
type Formula struct {
	root node
	refs []string
}

// Refs lists the field ids (or names, before resolution) the formula reads.
func (f *Formula) Refs() []string { return f.refs }

type tok struct {
	kind string // num str ref id op ( ) ,
	text string
}

func lex(src string) ([]tok, error) {
	var out []tok
	r := []rune(src)
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9' || (c == '.' && i+1 < len(r) && r[i+1] >= '0' && r[i+1] <= '9'):
			j := i
			for j < len(r) && (r[j] >= '0' && r[j] <= '9' || r[j] == '.') {
				j++
			}
			if j < len(r) && (r[j] == 'e' || r[j] == 'E') {
				k := j + 1
				if k < len(r) && (r[k] == '+' || r[k] == '-') {
					k++
				}
				if k < len(r) && r[k] >= '0' && r[k] <= '9' {
					for k < len(r) && r[k] >= '0' && r[k] <= '9' {
						k++
					}
					j = k
				}
			}
			out = append(out, tok{"num", string(r[i:j])})
			i = j
		case c == '"' || c == '\'':
			j := i + 1
			var b strings.Builder
			for j < len(r) && r[j] != c {
				if r[j] == '\\' && j+1 < len(r) {
					j++
					switch r[j] {
					case 'n':
						b.WriteRune('\n')
					case 't':
						b.WriteRune('\t')
					default:
						b.WriteRune(r[j])
					}
				} else {
					b.WriteRune(r[j])
				}
				j++
			}
			if j >= len(r) {
				return nil, fmt.Errorf("unterminated string")
			}
			out = append(out, tok{"str", b.String()})
			i = j + 1
		case c == '{':
			j := i + 1
			for j < len(r) && r[j] != '}' {
				j++
			}
			if j >= len(r) {
				return nil, fmt.Errorf("unterminated field reference")
			}
			name := strings.TrimSpace(string(r[i+1 : j]))
			if name == "" {
				return nil, fmt.Errorf("empty field reference")
			}
			out = append(out, tok{"ref", name})
			i = j + 1
		case c == '(' || c == ')' || c == ',':
			out = append(out, tok{string(c), string(c)})
			i++
		case c == '&' && i+1 < len(r) && r[i+1] == '&':
			out = append(out, tok{"op", "&&"})
			i += 2
		case strings.ContainsRune("+-*/%^&", c):
			out = append(out, tok{"op", string(c)})
			i++
		case c == '=' || c == '!' || c == '<' || c == '>' || c == '|':
			j := i + 1
			if j < len(r) && (r[j] == '=' || (c == '<' && r[j] == '>') || (c == '|' && r[j] == '|')) {
				j++
			}
			op := string(r[i:j])
			if op == "|" {
				return nil, fmt.Errorf("unexpected %q", op)
			}
			if op == "<>" {
				op = "!="
			}
			out = append(out, tok{"op", op})
			i = j
		case c == '&':
			out = append(out, tok{"op", "&"})
			i++
		case isIdentStart(c):
			j := i
			for j < len(r) && (isIdentStart(r[j]) || r[j] >= '0' && r[j] <= '9') {
				j++
			}
			out = append(out, tok{"id", string(r[i:j])})
			i = j
		default:
			return nil, fmt.Errorf("unexpected character %q", string(c))
		}
	}
	return out, nil
}

func isIdentStart(c rune) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

type parser struct {
	t    []tok
	i    int
	refs map[string]bool
}

func (p *parser) peek() tok {
	if p.i < len(p.t) {
		return p.t[p.i]
	}
	return tok{}
}
func (p *parser) next() tok { t := p.peek(); p.i++; return t }

// Precedence, low to high: || , && , comparison, & , + - , * / % , unary, ^
func (p *parser) parseExpr(level int) (node, error) {
	levels := [][]string{{"||"}, {"&&"}, {"=", "==", "!=", "<", ">", "<=", ">="}, {"&"}, {"+", "-"}, {"*", "/", "%"}}
	if level >= len(levels) {
		return p.parseUnary()
	}
	left, err := p.parseExpr(level + 1)
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != "op" || !contains(levels[level], t.text) {
			return left, nil
		}
		p.i++
		right, err := p.parseExpr(level + 1)
		if err != nil {
			return nil, err
		}
		op := t.text
		if op == "==" {
			op = "="
		}
		left = binNode{op, left, right}
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func (p *parser) parseUnary() (node, error) {
	t := p.peek()
	if t.kind == "op" && (t.text == "-" || t.text == "+" || t.text == "!") {
		p.i++
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return unNode{t.text, x}, nil
	}
	return p.parsePow()
}

func (p *parser) parsePow() (node, error) {
	base, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind == "op" && t.text == "^" {
		p.i++
		exp, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return binNode{"^", base, exp}, nil
	}
	return base, nil
}

func (p *parser) parsePrimary() (node, error) {
	t := p.next()
	switch t.kind {
	case "num":
		f, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", t.text)
		}
		return numNode(f), nil
	case "str":
		return strNode(t.text), nil
	case "ref":
		p.refs[t.text] = true
		return refNode(t.text), nil
	case "(":
		e, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if p.next().kind != ")" {
			return nil, fmt.Errorf("missing )")
		}
		return e, nil
	case "id":
		name := strings.ToUpper(t.text)
		if p.peek().kind != "(" {
			switch name {
			case "TRUE":
				return callNode{"TRUE", nil}, nil
			case "FALSE":
				return callNode{"FALSE", nil}, nil
			}
			return nil, fmt.Errorf("unknown name %q (field names go in {braces})", t.text)
		}
		p.i++
		var args []node
		if p.peek().kind == ")" {
			p.i++
		} else {
			for {
				a, err := p.parseExpr(0)
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				n := p.next()
				if n.kind == ")" {
					break
				}
				if n.kind != "," {
					return nil, fmt.Errorf("expected , or ) in %s()", name)
				}
			}
		}
		if _, ok := funcs[name]; !ok && !lazyFuncs[name] && name != "TRUE" && name != "FALSE" {
			return nil, fmt.Errorf("unknown function %s", name)
		}
		return callNode{name, args}, nil
	}
	if t.kind == "" {
		return nil, fmt.Errorf("formula ends too early")
	}
	return nil, fmt.Errorf("unexpected %q", t.text)
}

// ParseFormula parses src. Field references are the {names} inside it.
func ParseFormula(src string) (*Formula, error) {
	if strings.TrimSpace(src) == "" {
		return nil, fmt.Errorf("the formula is empty")
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{t: toks, refs: map[string]bool{}}
	n, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	if p.i < len(p.t) {
		return nil, fmt.Errorf("unexpected %q", p.t[p.i].text)
	}
	f := &Formula{root: n}
	for r := range p.refs {
		f.refs = append(f.refs, r)
	}
	sort.Strings(f.refs)
	return f, nil
}

// Eval runs the formula. A failure returns a FormulaError value.
func (f *Formula) Eval(c *fctx) (out any) {
	defer func() {
		if r := recover(); r != nil {
			if fe, ok := r.(FormulaError); ok {
				out = fe
				return
			}
			out = FormulaError{fmt.Sprint(r)}
		}
	}()
	return eval(c, f.root)
}

func eval(c *fctx, n node) any {
	switch x := n.(type) {
	case numNode:
		return float64(x)
	case strNode:
		return string(x)
	case refNode:
		return c.get(string(x))
	case unNode:
		v := eval(c, x.x)
		if err, ok := v.(FormulaError); ok {
			return err
		}
		switch x.op {
		case "-":
			return -toNum(v)
		case "!":
			return !toBool(v)
		}
		return toNum(v)
	case binNode:
		return evalBin(c, x)
	case callNode:
		switch x.name {
		case "TRUE":
			return true
		case "FALSE":
			return false
		}
		// Lazy functions.
		switch x.name {
		case "IF":
			if len(x.args) < 2 || len(x.args) > 3 {
				panic(FormulaError{"IF takes 2 or 3 arguments"})
			}
			cond := eval(c, x.args[0])
			if e, ok := cond.(FormulaError); ok {
				return e
			}
			if toBool(cond) {
				return eval(c, x.args[1])
			}
			if len(x.args) == 3 {
				return eval(c, x.args[2])
			}
			return nil
		case "IFERROR":
			if len(x.args) != 2 {
				panic(FormulaError{"IFERROR takes 2 arguments"})
			}
			v := eval(c, x.args[0])
			if _, ok := v.(FormulaError); ok {
				return eval(c, x.args[1])
			}
			return v
		case "SWITCH":
			if len(x.args) < 3 {
				panic(FormulaError{"SWITCH needs a value, a case and a result"})
			}
			v := eval(c, x.args[0])
			rest := x.args[1:]
			for len(rest) >= 2 {
				if looseEq(v, eval(c, rest[0])) {
					return eval(c, rest[1])
				}
				rest = rest[2:]
			}
			if len(rest) == 1 {
				return eval(c, rest[0])
			}
			return nil
		}
		args := make([]any, len(x.args))
		for i, a := range x.args {
			args[i] = eval(c, a)
			if e, ok := args[i].(FormulaError); ok && x.name != "ISERROR" && x.name != "ISBLANK" {
				return e
			}
		}
		return funcs[x.name](c, args)
	}
	return nil
}

func evalBin(c *fctx, x binNode) any {
	if x.op == "&&" || x.op == "||" {
		a := eval(c, x.a)
		if e, ok := a.(FormulaError); ok {
			return e
		}
		if x.op == "&&" && !toBool(a) {
			return false
		}
		if x.op == "||" && toBool(a) {
			return true
		}
		b := eval(c, x.b)
		if e, ok := b.(FormulaError); ok {
			return e
		}
		return toBool(b)
	}
	a, b := eval(c, x.a), eval(c, x.b)
	if e, ok := a.(FormulaError); ok {
		return e
	}
	if e, ok := b.(FormulaError); ok {
		return e
	}
	switch x.op {
	case "+":
		return toNum(a) + toNum(b)
	case "-":
		return toNum(a) - toNum(b)
	case "*":
		return toNum(a) * toNum(b)
	case "/":
		d := toNum(b)
		if d == 0 {
			return FormulaError{"#DIV/0!"}
		}
		return toNum(a) / d
	case "%":
		d := toNum(b)
		if d == 0 {
			return FormulaError{"#DIV/0!"}
		}
		return math.Mod(toNum(a), d)
	case "^":
		return math.Pow(toNum(a), toNum(b))
	case "&":
		return toStr(a) + toStr(b)
	case "=":
		return looseEq(a, b)
	case "!=":
		return !looseEq(a, b)
	case "<", ">", "<=", ">=":
		cmp := compareVals(a, b)
		switch x.op {
		case "<":
			return cmp < 0
		case ">":
			return cmp > 0
		case "<=":
			return cmp <= 0
		}
		return cmp >= 0
	}
	return nil
}

func looseEq(a, b any) bool {
	if a == nil || b == nil {
		return (a == nil || a == "") && (b == nil || b == "")
	}
	if fa, ok := a.(float64); ok {
		if fb, ok := b.(float64); ok {
			return math.Abs(fa-fb) < 1e-9
		}
	}
	if ba, ok := a.(bool); ok {
		if bb, ok := b.(bool); ok {
			return ba == bb
		}
	}
	return toStr(a) == toStr(b)
}

func compareVals(a, b any) int {
	if ta, ok := a.(time.Time); ok {
		if tb, ok := b.(time.Time); ok {
			switch {
			case ta.Before(tb):
				return -1
			case ta.After(tb):
				return 1
			}
			return 0
		}
	}
	_, an := a.(float64)
	_, bn := b.(float64)
	if an || bn {
		x, y := toNum(a), toNum(b)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	}
	return strings.Compare(toStr(a), toStr(b))
}

func toNum(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		s := strings.TrimSpace(strings.ReplaceAll(x, ",", ""))
		if s == "" {
			return 0
		}
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			panic(FormulaError{"#VALUE! cannot use " + strconv.Quote(x) + " as a number"})
		}
		if strings.HasSuffix(s, "%") {
			f /= 100
		}
		return f
	case time.Time:
		return float64(x.Unix()) * 1000
	case []any:
		if len(x) == 0 {
			return 0
		}
		return toNum(x[0])
	}
	return 0
}

func toBool(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x != "" && !strings.EqualFold(x, "false")
	case []any:
		return len(x) > 0
	}
	return true
}

func fmtNum(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func toStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return fmtNum(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case time.Time:
		if x.Hour() == 0 && x.Minute() == 0 && x.Second() == 0 {
			return x.Format("2006-01-02")
		}
		return x.Format(time.RFC3339)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = toStr(e)
		}
		return strings.Join(parts, ", ")
	case FormulaError:
		return "#ERROR!"
	}
	return fmt.Sprint(v)
}

func toTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case string:
		return parseDate(x)
	case float64:
		return time.UnixMilli(int64(x)).UTC(), true
	}
	return time.Time{}, false
}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, l := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02", "2006/01/02", "02/01/2006", "1/2/2006"} {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func flatten(args []any) []any {
	var out []any
	for _, a := range args {
		if l, ok := a.([]any); ok {
			out = append(out, flatten(l)...)
		} else {
			out = append(out, a)
		}
	}
	return out
}

func numbers(args []any) []float64 {
	var out []float64
	for _, a := range flatten(args) {
		switch x := a.(type) {
		case nil:
		case string:
			if strings.TrimSpace(x) == "" {
				continue
			}
			if f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(x), ",", ""), 64); err == nil {
				out = append(out, f)
			}
		case bool:
		default:
			out = append(out, toNum(a))
		}
	}
	return out
}

func need(name string, args []any, min, max int) {
	if len(args) < min || (max >= 0 && len(args) > max) {
		if min == max {
			panic(FormulaError{fmt.Sprintf("%s takes %d argument(s)", name, min)})
		}
		panic(FormulaError{fmt.Sprintf("%s: wrong number of arguments", name)})
	}
}

func roundTo(f float64, digits int, mode string) float64 {
	p := math.Pow(10, float64(digits))
	x := f * p
	switch mode {
	case "up":
		if x >= 0 {
			x = math.Ceil(x - 1e-9)
		} else {
			x = math.Floor(x + 1e-9)
		}
	case "down":
		x = math.Trunc(x)
	default:
		x = math.Round(x)
	}
	return x / p
}

func isEmptyVal(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	}
	return false
}

func unitDiff(a, b time.Time, unit string) float64 {
	d := a.Sub(b)
	switch strings.ToLower(unit) {
	case "milliseconds", "ms":
		return float64(d.Milliseconds())
	case "seconds", "s":
		return math.Trunc(d.Seconds())
	case "minutes", "m":
		return math.Trunc(d.Minutes())
	case "hours", "h":
		return math.Trunc(d.Hours())
	case "weeks", "w":
		return math.Trunc(d.Hours() / 24 / 7)
	case "months", "M":
		m := (a.Year()-b.Year())*12 + int(a.Month()) - int(b.Month())
		if m > 0 && a.Day() < b.Day() {
			m--
		} else if m < 0 && a.Day() > b.Day() {
			m++
		}
		return float64(m)
	case "quarters":
		return math.Trunc(unitDiff(a, b, "months") / 3)
	case "years", "y":
		return math.Trunc(unitDiff(a, b, "months") / 12)
	}
	return math.Trunc(d.Hours() / 24)
}

func addUnit(t time.Time, n int, unit string) time.Time {
	switch strings.ToLower(unit) {
	case "milliseconds", "ms":
		return t.Add(time.Duration(n) * time.Millisecond)
	case "seconds", "s":
		return t.Add(time.Duration(n) * time.Second)
	case "minutes", "m":
		return t.Add(time.Duration(n) * time.Minute)
	case "hours", "h":
		return t.Add(time.Duration(n) * time.Hour)
	case "weeks", "w":
		return t.AddDate(0, 0, 7*n)
	case "months", "M":
		return addMonths(t, n)
	case "quarters":
		return addMonths(t, 3*n)
	case "years", "y":
		return addMonths(t, 12*n)
	}
	return t.AddDate(0, 0, n)
}

// addMonths keeps the day clamped to the target month (Jan 31 + 1 month = Feb 28/29).
func addMonths(t time.Time, n int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
	d := t.Day()
	if d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

func isWeekend(t time.Time) bool { return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday }

func holidaySet(v any) map[string]bool {
	m := map[string]bool{}
	s := toStr(v)
	for _, p := range strings.Split(s, ",") {
		if t, ok := parseDate(strings.TrimSpace(p)); ok {
			m[t.Format("2006-01-02")] = true
		}
	}
	return m
}

// workdayDiff counts working days from a to b inclusive of both ends, like Airtable's WORKDAY_DIFF.
func workdayDiff(a, b time.Time, hol map[string]bool) float64 {
	a = time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	b = time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	sign := 1.0
	if b.Before(a) {
		a, b = b, a
		sign = -1
	}
	n := 0
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		if !isWeekend(d) && !hol[d.Format("2006-01-02")] {
			n++
		}
	}
	return sign * float64(n)
}

func formatTime(t time.Time, layout string) string {
	r := strings.NewReplacer("YYYY", "2006", "YY", "06", "MMMM", "January", "MMM", "Jan", "MM", "01", "DD", "02", "dddd", "Monday", "ddd", "Mon",
		"HH", "15", "hh", "03", "mm", "04", "ss", "05", "A", "PM", "a", "pm", "Z", "Z07:00", "M", "1", "D", "2")
	if layout == "" {
		layout = "YYYY-MM-DD"
	}
	return t.Format(r.Replace(layout))
}

var funcs map[string]fnImpl

// lazyFuncs evaluate only the arguments they need; eval handles them.
var lazyFuncs = map[string]bool{"IF": true, "IFERROR": true, "SWITCH": true}

func init() {
	funcs = map[string]fnImpl{
		"AND": func(c *fctx, a []any) any {
			need("AND", a, 1, -1)
			for _, v := range flatten(a) {
				if !toBool(v) {
					return false
				}
			}
			return true
		},
		"OR": func(c *fctx, a []any) any {
			need("OR", a, 1, -1)
			for _, v := range flatten(a) {
				if toBool(v) {
					return true
				}
			}
			return false
		},
		"XOR": func(c *fctx, a []any) any {
			n := 0
			for _, v := range flatten(a) {
				if toBool(v) {
					n++
				}
			}
			return n%2 == 1
		},
		"NOT":     func(c *fctx, a []any) any { need("NOT", a, 1, 1); return !toBool(a[0]) },
		"BLANK":   func(c *fctx, a []any) any { return nil },
		"ISBLANK": func(c *fctx, a []any) any { need("ISBLANK", a, 1, 1); return isEmptyVal(a[0]) },
		"ISERROR": func(c *fctx, a []any) any { need("ISERROR", a, 1, 1); _, ok := a[0].(FormulaError); return ok },
		"ERROR": func(c *fctx, a []any) any {
			msg := "#ERROR!"
			if len(a) > 0 {
				msg = toStr(a[0])
			}
			return FormulaError{msg}
		},
		"RECORD_ID": func(c *fctx, a []any) any { return c.recID },
		"SUM": func(c *fctx, a []any) any {
			s := 0.0
			for _, f := range numbers(a) {
				s += f
			}
			return s
		},
		"AVERAGE": func(c *fctx, a []any) any {
			n := numbers(a)
			if len(n) == 0 {
				return FormulaError{"#DIV/0!"}
			}
			s := 0.0
			for _, f := range n {
				s += f
			}
			return s / float64(len(n))
		},
		"MIN": func(c *fctx, a []any) any {
			n := numbers(a)
			if len(n) == 0 {
				return 0.0
			}
			m := n[0]
			for _, f := range n {
				m = math.Min(m, f)
			}
			return m
		},
		"MAX": func(c *fctx, a []any) any {
			n := numbers(a)
			if len(n) == 0 {
				return 0.0
			}
			m := n[0]
			for _, f := range n {
				m = math.Max(m, f)
			}
			return m
		},
		"COUNT": func(c *fctx, a []any) any { return float64(len(numbers(a))) },
		"COUNTA": func(c *fctx, a []any) any {
			n := 0
			for _, v := range flatten(a) {
				if !isEmptyVal(v) {
					n++
				}
			}
			return float64(n)
		},
		"COUNTALL": func(c *fctx, a []any) any { return float64(len(flatten(a))) },
		"ABS":      func(c *fctx, a []any) any { need("ABS", a, 1, 1); return math.Abs(toNum(a[0])) },
		"INT":      func(c *fctx, a []any) any { need("INT", a, 1, 1); return math.Floor(toNum(a[0])) },
		"SQRT": func(c *fctx, a []any) any {
			need("SQRT", a, 1, 1)
			if toNum(a[0]) < 0 {
				return FormulaError{"#NUM!"}
			}
			return math.Sqrt(toNum(a[0]))
		},
		"EXP": func(c *fctx, a []any) any { need("EXP", a, 1, 1); return math.Exp(toNum(a[0])) },
		"LOG": func(c *fctx, a []any) any {
			need("LOG", a, 1, 2)
			base := 10.0
			if len(a) == 2 {
				base = toNum(a[1])
			}
			if toNum(a[0]) <= 0 || base <= 0 || base == 1 {
				return FormulaError{"#NUM!"}
			}
			return math.Log(toNum(a[0])) / math.Log(base)
		},
		"POWER": func(c *fctx, a []any) any { need("POWER", a, 2, 2); return math.Pow(toNum(a[0]), toNum(a[1])) },
		"MOD": func(c *fctx, a []any) any {
			need("MOD", a, 2, 2)
			d := toNum(a[1])
			if d == 0 {
				return FormulaError{"#DIV/0!"}
			}
			r := math.Mod(toNum(a[0]), d)
			if r != 0 && (r < 0) != (d < 0) {
				r += d
			}
			return r
		},
		"ROUND": func(c *fctx, a []any) any {
			need("ROUND", a, 1, 2)
			d := 0
			if len(a) == 2 {
				d = int(toNum(a[1]))
			}
			return roundTo(toNum(a[0]), d, "")
		},
		"ROUNDUP": func(c *fctx, a []any) any {
			need("ROUNDUP", a, 1, 2)
			d := 0
			if len(a) == 2 {
				d = int(toNum(a[1]))
			}
			return roundTo(toNum(a[0]), d, "up")
		},
		"ROUNDDOWN": func(c *fctx, a []any) any {
			need("ROUNDDOWN", a, 1, 2)
			d := 0
			if len(a) == 2 {
				d = int(toNum(a[1]))
			}
			return roundTo(toNum(a[0]), d, "down")
		},
		"CEILING": func(c *fctx, a []any) any {
			need("CEILING", a, 1, 2)
			s := 1.0
			if len(a) == 2 {
				s = toNum(a[1])
			}
			if s == 0 {
				return 0.0
			}
			return math.Ceil(toNum(a[0])/s-1e-12) * s
		},
		"FLOOR": func(c *fctx, a []any) any {
			need("FLOOR", a, 1, 2)
			s := 1.0
			if len(a) == 2 {
				s = toNum(a[1])
			}
			if s == 0 {
				return 0.0
			}
			return math.Floor(toNum(a[0])/s+1e-12) * s
		},
		"EVEN": func(c *fctx, a []any) any {
			need("EVEN", a, 1, 1)
			x := toNum(a[0])
			r := 2 * math.Ceil(math.Abs(x)/2)
			if x < 0 {
				return -r
			}
			return r
		},
		"ODD": func(c *fctx, a []any) any {
			need("ODD", a, 1, 1)
			x := toNum(a[0])
			r := 2*math.Ceil((math.Abs(x)-1)/2) + 1
			if x < 0 {
				return -r
			}
			return r
		},
		"VALUE": func(c *fctx, a []any) any { need("VALUE", a, 1, 1); return toNum(a[0]) },
		"NUM":   func(c *fctx, a []any) any { need("NUM", a, 1, 1); return toNum(a[0]) },
		"CONCATENATE": func(c *fctx, a []any) any {
			var b strings.Builder
			for _, v := range a {
				b.WriteString(toStr(v))
			}
			return b.String()
		},
		"LEFT": func(c *fctx, a []any) any {
			need("LEFT", a, 2, 2)
			r := []rune(toStr(a[0]))
			n := clampInt(int(toNum(a[1])), 0, len(r))
			return string(r[:n])
		},
		"RIGHT": func(c *fctx, a []any) any {
			need("RIGHT", a, 2, 2)
			r := []rune(toStr(a[0]))
			n := clampInt(int(toNum(a[1])), 0, len(r))
			return string(r[len(r)-n:])
		},
		"MID": func(c *fctx, a []any) any {
			need("MID", a, 3, 3)
			r := []rune(toStr(a[0]))
			start := clampInt(int(toNum(a[1]))-1, 0, len(r))
			end := clampInt(start+int(toNum(a[2])), start, len(r))
			return string(r[start:end])
		},
		"LEN":   func(c *fctx, a []any) any { need("LEN", a, 1, 1); return float64(utf8.RuneCountInString(toStr(a[0]))) },
		"UPPER": func(c *fctx, a []any) any { need("UPPER", a, 1, 1); return strings.ToUpper(toStr(a[0])) },
		"LOWER": func(c *fctx, a []any) any { need("LOWER", a, 1, 1); return strings.ToLower(toStr(a[0])) },
		"TRIM": func(c *fctx, a []any) any {
			need("TRIM", a, 1, 1)
			return strings.Join(strings.Fields(toStr(a[0])), " ")
		},
		"REPT": func(c *fctx, a []any) any {
			need("REPT", a, 2, 2)
			n := int(toNum(a[1]))
			if n < 0 || n > 1000 {
				return FormulaError{"#NUM!"}
			}
			return strings.Repeat(toStr(a[0]), n)
		},
		"FIND": func(c *fctx, a []any) any {
			need("FIND", a, 2, 3)
			return findStr(a, false)
		},
		"SEARCH": func(c *fctx, a []any) any {
			need("SEARCH", a, 2, 3)
			return findStr(a, true)
		},
		"SUBSTITUTE": func(c *fctx, a []any) any {
			need("SUBSTITUTE", a, 3, 4)
			s, old, nw := toStr(a[0]), toStr(a[1]), toStr(a[2])
			if old == "" {
				return s
			}
			if len(a) == 4 {
				n := int(toNum(a[3]))
				idx, from := -1, 0
				for i := 0; i < n; i++ {
					j := strings.Index(s[from:], old)
					if j < 0 {
						return s
					}
					idx = from + j
					from = idx + len(old)
				}
				return s[:idx] + nw + s[idx+len(old):]
			}
			return strings.ReplaceAll(s, old, nw)
		},
		"REPLACE": func(c *fctx, a []any) any {
			need("REPLACE", a, 4, 4)
			r := []rune(toStr(a[0]))
			start := clampInt(int(toNum(a[1]))-1, 0, len(r))
			end := clampInt(start+int(toNum(a[2])), start, len(r))
			return string(r[:start]) + toStr(a[3]) + string(r[end:])
		},
		"T": func(c *fctx, a []any) any {
			need("T", a, 1, 1)
			if s, ok := a[0].(string); ok {
				return s
			}
			return ""
		},
		"ENCODE_URL_COMPONENT": func(c *fctx, a []any) any {
			need("ENCODE_URL_COMPONENT", a, 1, 1)
			return strings.ReplaceAll(urlQueryEscape(toStr(a[0])), "+", "%20")
		},
		"REGEX_MATCH": func(c *fctx, a []any) any {
			need("REGEX_MATCH", a, 2, 2)
			re := compileRe(toStr(a[1]))
			return re.MatchString(toStr(a[0]))
		},
		"REGEX_EXTRACT": func(c *fctx, a []any) any {
			need("REGEX_EXTRACT", a, 2, 2)
			re := compileRe(toStr(a[1]))
			m := re.FindStringSubmatch(toStr(a[0]))
			if m == nil {
				return nil
			}
			if len(m) > 1 {
				return m[1]
			}
			return m[0]
		},
		"REGEX_REPLACE": func(c *fctx, a []any) any {
			need("REGEX_REPLACE", a, 3, 3)
			re := compileRe(toStr(a[1]))
			return re.ReplaceAllString(toStr(a[0]), toStr(a[2]))
		},
		"TODAY": func(c *fctx, a []any) any {
			n := c.now.UTC()
			return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
		},
		"NOW": func(c *fctx, a []any) any { return c.now.UTC() },
		"YEAR": func(c *fctx, a []any) any {
			return dateField(a, "YEAR", func(t time.Time) float64 { return float64(t.Year()) })
		},
		"MONTH": func(c *fctx, a []any) any {
			return dateField(a, "MONTH", func(t time.Time) float64 { return float64(t.Month()) })
		},
		"DAY": func(c *fctx, a []any) any {
			return dateField(a, "DAY", func(t time.Time) float64 { return float64(t.Day()) })
		},
		"HOUR": func(c *fctx, a []any) any {
			return dateField(a, "HOUR", func(t time.Time) float64 { return float64(t.Hour()) })
		},
		"MINUTE": func(c *fctx, a []any) any {
			return dateField(a, "MINUTE", func(t time.Time) float64 { return float64(t.Minute()) })
		},
		"SECOND": func(c *fctx, a []any) any {
			return dateField(a, "SECOND", func(t time.Time) float64 { return float64(t.Second()) })
		},
		"WEEKNUM": func(c *fctx, a []any) any {
			return dateField(a, "WEEKNUM", func(t time.Time) float64 { _, w := t.ISOWeek(); return float64(w) })
		},
		"WEEKDAY": func(c *fctx, a []any) any {
			need("WEEKDAY", a, 1, 2)
			t, ok := toTime(a[0])
			if !ok {
				return nil
			}
			start := time.Sunday
			if len(a) == 2 {
				switch strings.ToLower(toStr(a[1])) {
				case "monday":
					start = time.Monday
				case "sunday", "":
				default:
					return FormulaError{"WEEKDAY: start day must be Sunday or Monday"}
				}
			}
			return float64((int(t.Weekday()) - int(start) + 7) % 7)
		},
		"DATEADD": func(c *fctx, a []any) any {
			need("DATEADD", a, 3, 3)
			t, ok := toTime(a[0])
			if !ok {
				return nil
			}
			return addUnit(t, int(toNum(a[1])), toStr(a[2]))
		},
		"DATETIME_DIFF": func(c *fctx, a []any) any {
			need("DATETIME_DIFF", a, 2, 3)
			x, ok1 := toTime(a[0])
			y, ok2 := toTime(a[1])
			if !ok1 || !ok2 {
				return nil
			}
			unit := "days"
			if len(a) == 3 {
				unit = toStr(a[2])
			}
			return unitDiff(x, y, unit)
		},
		"DATETIME_FORMAT": func(c *fctx, a []any) any {
			need("DATETIME_FORMAT", a, 1, 2)
			t, ok := toTime(a[0])
			if !ok {
				return nil
			}
			l := ""
			if len(a) == 2 {
				l = toStr(a[1])
			}
			return formatTime(t, l)
		},
		"DATETIME_PARSE": func(c *fctx, a []any) any {
			need("DATETIME_PARSE", a, 1, 2)
			t, ok := parseDate(toStr(a[0]))
			if !ok {
				return nil
			}
			return t
		},
		"DATESTR": func(c *fctx, a []any) any {
			need("DATESTR", a, 1, 1)
			t, ok := toTime(a[0])
			if !ok {
				return nil
			}
			return t.Format("2006-01-02")
		},
		"IS_BEFORE": func(c *fctx, a []any) any {
			return dateCmp(a, "IS_BEFORE", func(x, y time.Time) bool { return x.Before(y) })
		},
		"IS_AFTER": func(c *fctx, a []any) any {
			return dateCmp(a, "IS_AFTER", func(x, y time.Time) bool { return x.After(y) })
		},
		"IS_SAME": func(c *fctx, a []any) any {
			need("IS_SAME", a, 2, 3)
			x, ok1 := toTime(a[0])
			y, ok2 := toTime(a[1])
			if !ok1 || !ok2 {
				return false
			}
			unit := "milliseconds"
			if len(a) == 3 {
				unit = toStr(a[2])
			}
			switch strings.ToLower(unit) {
			case "days", "d":
				return x.Format("2006-01-02") == y.Format("2006-01-02")
			case "months", "M":
				return x.Format("2006-01") == y.Format("2006-01")
			case "years", "y":
				return x.Year() == y.Year()
			}
			return x.Equal(y)
		},
		"WORKDAY": func(c *fctx, a []any) any {
			need("WORKDAY", a, 2, 3)
			t, ok := toTime(a[0])
			if !ok {
				return nil
			}
			n := int(toNum(a[1]))
			hol := map[string]bool{}
			if len(a) == 3 {
				hol = holidaySet(a[2])
			}
			step := 1
			if n < 0 {
				step, n = -1, -n
			}
			for n > 0 {
				t = t.AddDate(0, 0, step)
				if !isWeekend(t) && !hol[t.Format("2006-01-02")] {
					n--
				}
			}
			return t
		},
		"WORKDAY_DIFF": func(c *fctx, a []any) any {
			need("WORKDAY_DIFF", a, 2, 3)
			x, ok1 := toTime(a[0])
			y, ok2 := toTime(a[1])
			if !ok1 || !ok2 {
				return nil
			}
			hol := map[string]bool{}
			if len(a) == 3 {
				hol = holidaySet(a[2])
			}
			return workdayDiff(x, y, hol)
		},
		"CREATED_TIME":       func(c *fctx, a []any) any { return c.created },
		"LAST_MODIFIED_TIME": func(c *fctx, a []any) any { return c.updated },
		"ARRAYJOIN": func(c *fctx, a []any) any {
			need("ARRAYJOIN", a, 1, 2)
			sep := ", "
			if len(a) == 2 {
				sep = toStr(a[1])
			}
			var parts []string
			for _, v := range flatten(a[:1]) {
				parts = append(parts, toStr(v))
			}
			return strings.Join(parts, sep)
		},
		"ARRAYUNIQUE": func(c *fctx, a []any) any {
			seen := map[string]bool{}
			out := []any{}
			for _, v := range flatten(a) {
				k := toStr(v)
				if !seen[k] {
					seen[k] = true
					out = append(out, v)
				}
			}
			return out
		},
		"ARRAYCOMPACT": func(c *fctx, a []any) any {
			out := []any{}
			for _, v := range flatten(a) {
				if !isEmptyVal(v) {
					out = append(out, v)
				}
			}
			return out
		},
		"ARRAYFLATTEN": func(c *fctx, a []any) any { return flatten(a) },
		"ARRAYSLICE": func(c *fctx, a []any) any {
			need("ARRAYSLICE", a, 2, 3)
			l, _ := a[0].([]any)
			s := int(toNum(a[1]))
			if s < 0 {
				s += len(l)
			}
			e := len(l)
			if len(a) == 3 {
				e = int(toNum(a[2]))
				if e < 0 {
					e += len(l)
				}
			}
			s, e = clampInt(s, 0, len(l)), clampInt(e, 0, len(l))
			if e < s {
				e = s
			}
			return append([]any{}, l[s:e]...)
		},
	}
	funcs["CONCAT"] = funcs["CONCATENATE"]
	funcs["IS_EVEN"] = func(c *fctx, a []any) any { need("IS_EVEN", a, 1, 1); return math.Mod(toNum(a[0]), 2) == 0 }
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func findStr(a []any, fold bool) any {
	needle, hay := toStr(a[0]), toStr(a[1])
	if fold {
		needle, hay = strings.ToLower(needle), strings.ToLower(hay)
	}
	hr := []rune(hay)
	start := 0
	if len(a) == 3 {
		start = clampInt(int(toNum(a[2]))-1, 0, len(hr))
	}
	idx := strings.Index(string(hr[start:]), needle)
	if idx < 0 {
		return 0.0
	}
	return float64(utf8.RuneCountInString(string(hr[start:])[:idx]) + start + 1)
}

func dateField(a []any, name string, f func(time.Time) float64) any {
	need(name, a, 1, 1)
	t, ok := toTime(a[0])
	if !ok {
		return nil
	}
	return f(t)
}

func dateCmp(a []any, name string, f func(x, y time.Time) bool) any {
	need(name, a, 2, 2)
	x, ok1 := toTime(a[0])
	y, ok2 := toTime(a[1])
	if !ok1 || !ok2 {
		return false
	}
	return f(x, y)
}

var (
	reMu    sync.Mutex
	reCache = map[string]*regexp.Regexp{}
)

func compileRe(p string) *regexp.Regexp {
	if len(p) > 500 {
		panic(FormulaError{"regular expression too long"})
	}
	reMu.Lock()
	defer reMu.Unlock()
	if re, ok := reCache[p]; ok {
		return re
	}
	re, err := regexp.Compile(p) // RE2: linear time, no catastrophic backtracking
	if err != nil {
		panic(FormulaError{"bad regular expression"})
	}
	if len(reCache) > 500 {
		reCache = map[string]*regexp.Regexp{}
	}
	reCache[p] = re
	return re
}

func urlQueryEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
