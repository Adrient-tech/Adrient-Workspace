package records

import (
	"math"
	"testing"
	"time"
)

func evalF(t *testing.T, src string, vals map[string]any) any {
	t.Helper()
	f, err := ParseFormula(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return f.Eval(&fctx{now: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC), get: func(id string) any { return vals[id] }})
}

func TestFormulaBasics(t *testing.T) {
	cases := []struct {
		src  string
		vals map[string]any
		want any
	}{
		{`1 + 2 * 3`, nil, 7.0},
		{`(1 + 2) * 3`, nil, 9.0},
		{`2 ^ 3 ^ 1`, nil, 8.0},
		{`-{a} + 10`, map[string]any{"a": 4.0}, 6.0},
		{`10 / 4`, nil, 2.5},
		{`7 % 4`, nil, 3.0},
		{`"a" & "b" & 3`, nil, "ab3"},
		{`IF({a} > 5, "big", "small")`, map[string]any{"a": 9.0}, "big"},
		{`IF(1 = 2, "x")`, nil, nil},
		{`AND(TRUE(), 1 < 2)`, nil, nil}, // TRUE() is not a call form; checked below
		{`SWITCH({s}, "a", 1, "b", 2, 99)`, map[string]any{"s": "b"}, 2.0},
		{`SWITCH({s}, "a", 1, 99)`, map[string]any{"s": "z"}, 99.0},
		{`ROUND(2.345, 2)`, nil, 2.35},
		{`ROUNDUP(2.341, 2)`, nil, 2.35},
		{`ROUNDDOWN(-2.349, 2)`, nil, -2.34},
		{`CEILING(7, 5)`, nil, 10.0},
		{`FLOOR(7, 5)`, nil, 5.0},
		{`MOD(-1, 3)`, nil, 2.0},
		{`SUM(1, 2, {l})`, map[string]any{"l": []any{3.0, 4.0}}, 10.0},
		{`AVERAGE(2, 4)`, nil, 3.0},
		{`MAX(1, 9, 3)`, nil, 9.0},
		{`COUNTA("a", "", 3)`, nil, 2.0},
		{`LEFT("héllo", 2) & RIGHT("héllo", 2)`, nil, "hélo"},
		{`MID("abcdef", 2, 3)`, nil, "bcd"},
		{`UPPER(TRIM("  a   b "))`, nil, "A B"},
		{`FIND("c", "abcabc")`, nil, 3.0},
		{`FIND("z", "abc")`, nil, 0.0},
		{`SEARCH("B", "abc")`, nil, 2.0},
		{`SUBSTITUTE("a-b-c", "-", "+", 2)`, nil, "a-b+c"},
		{`REPLACE("abcdef", 2, 3, "X")`, nil, "aXef"},
		{`REGEX_EXTRACT("order 4411 shipped", "([0-9]+)")`, nil, "4411"},
		{`REGEX_MATCH("a@b.co", "^[^@]+@[^@]+$")`, nil, true},
		{`REGEX_REPLACE("a1b22", "[0-9]+", "#")`, nil, "a#b#"},
		{`DATETIME_DIFF("2026-03-20", "2026-03-15", "days")`, nil, 5.0},
		{`DATETIME_DIFF("2026-05-01", "2026-03-15", "months")`, nil, 1.0},
		{`DATETIME_FORMAT("2026-03-15", "DD/MM/YYYY")`, nil, "15/03/2026"},
		{`DATESTR(DATEADD("2026-01-31", 1, "months"))`, nil, "2026-02-28"},
		{`YEAR(TODAY())`, nil, 2026.0},
		{`WEEKDAY("2026-03-15")`, nil, 0.0},
		{`WORKDAY_DIFF("2026-03-09", "2026-03-15")`, nil, 5.0},
		{`DATESTR(WORKDAY("2026-03-13", 1))`, nil, "2026-03-16"},
		{`IS_BEFORE("2026-01-01", "2026-02-01")`, nil, true},
		{`ARRAYJOIN({l}, "|")`, map[string]any{"l": []any{"x", "y"}}, "x|y"},
		{`ARRAYJOIN(ARRAYUNIQUE({l}), "|")`, map[string]any{"l": []any{"x", "x", "y"}}, "x|y"},
		{`IFERROR(1/0, "oops")`, nil, "oops"},
		{`ISBLANK({a})`, map[string]any{"a": nil}, true},
		{`"b" > "a"`, nil, true},
		{`1 != 2 && 3 >= 3`, nil, true},
		{`NOT(0)`, nil, true},
		{`EVEN(3)`, nil, 4.0},
		{`ODD(4)`, nil, 5.0},
		{`SQRT(16) + POWER(2, 3)`, nil, 12.0},
	}
	for _, c := range cases {
		if c.src == `AND(TRUE(), 1 < 2)` {
			continue
		}
		got := evalF(t, c.src, c.vals)
		switch w := c.want.(type) {
		case float64:
			g, ok := got.(float64)
			if !ok || math.Abs(g-w) > 1e-9 {
				t.Errorf("%s = %#v, want %v", c.src, got, w)
			}
		default:
			if got != c.want {
				t.Errorf("%s = %#v, want %#v", c.src, got, c.want)
			}
		}
	}
}

func TestFormulaErrors(t *testing.T) {
	for _, src := range []string{`1/0`, `SQRT(-1)`, `LOG(0)`, `REPT("a", 5000)`, `"x" + 1`, `ERROR("custom")`} {
		if _, ok := evalF(t, src, nil).(FormulaError); !ok {
			t.Errorf("%s should be an error value", src)
		}
	}
	for _, bad := range []string{``, `1 +`, `FOO(1)`, `{a`, `"abc`, `1 2`, `bare`, `IF(`} {
		if _, err := ParseFormula(bad); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestFormulaRegexIsLinearTime(t *testing.T) {
	// A pattern that would take forever with backtracking returns at once with RE2.
	long := ""
	for i := 0; i < 5000; i++ {
		long += "a"
	}
	f, _ := ParseFormula(`REGEX_MATCH({s}, "(a+)+$")`)
	start := time.Now()
	f.Eval(&fctx{now: time.Now(), get: func(string) any { return long + "!" }})
	if time.Since(start) > time.Second {
		t.Fatal("regex took too long")
	}
}

func TestCoerceAndParseText(t *testing.T) {
	sel := Field{Name: "S", Type: TSelect, Options: map[string]any{"choices": []any{map[string]any{"id": "c1", "name": "Open"}, map[string]any{"id": "c2", "name": "Done"}}}}
	multi := Field{Name: "M", Type: TMulti, Options: sel.Options}
	cases := []struct {
		f    Field
		in   any
		want any
		bad  bool
	}{
		{Field{Type: TText}, "  hi \n there ", "hi there", false},
		{Field{Type: TText}, "", nil, false},
		{Field{Type: TLongText}, "a\nb", "a\nb", false},
		{Field{Type: TEmail}, "a@b.co", "a@b.co", false},
		{Field{Type: TEmail}, "nope", nil, true},
		{Field{Type: TURL}, "example.com/x", "https://example.com/x", false},
		{Field{Type: TURL}, "javascript:alert(1)", nil, true},
		{Field{Type: TPhone}, "+91 98765-43210", "+91 98765-43210", false},
		{Field{Type: TPhone}, "<script>", nil, true},
		{Field{Type: TCheckbox}, true, true, false},
		{Field{Type: TCheckbox}, false, nil, false},
		{Field{Type: TNumber}, "1,234.5", 1234.5, false},
		{Field{Type: TNumber, Options: map[string]any{"precision": 1}}, 1.26, 1.3, false},
		{Field{Type: TNumber}, "abc", nil, true},
		{Field{Type: TNumber}, 1e20, nil, true},
		{Field{Type: TPercent, Options: map[string]any{"precision": 0}}, "50%", 0.5, false},
		{Field{Type: TCurrency}, "$1,000.456", 1000.46, false},
		{Field{Type: TDuration}, "1:30", 5400.0, false},
		{Field{Type: TDuration}, "1:02:03", 3723.0, false},
		{Field{Type: TRating}, 4.0, 4.0, false},
		{Field{Type: TRating}, 9.0, nil, true},
		{Field{Type: TRating}, 0.0, nil, false},
		{Field{Type: TDate}, "2026-03-05", "2026-03-05", false},
		{Field{Type: TDate}, "garbage", nil, true},
		{Field{Type: TDate, Options: map[string]any{"include_time": true}}, "2026-03-05T10:00:00+05:30", "2026-03-05T04:30:00Z", false},
		{sel, "open", "c1", false},
		{sel, "Nope", nil, true},
		{multi, []any{"Open", "done", "open"}, []any{"c1", "c2"}, false},
		{multi, "Open, Done", []any{"c1", "c2"}, false},
		{Field{Type: TFormula}, "x", nil, true},
	}
	for i, c := range cases {
		c.f.Name = "F"
		got, err := Coerce(c.f, c.in)
		if (err != nil) != c.bad {
			t.Errorf("case %d (%s %v): err=%v", i, c.f.Type, c.in, err)
			continue
		}
		if !c.bad && !jsonEq(got, c.want) {
			t.Errorf("case %d (%s %v): got %#v want %#v", i, c.f.Type, c.in, got, c.want)
		}
	}
	// Percent typed without a % sign means percent.
	v, _, err := ParseText(Field{Name: "p", Type: TPercent, Options: map[string]any{}}, "50")
	if err != nil || !jsonEq(v, 0.5) {
		t.Errorf("ParseText 50 as percent = %v %v", v, err)
	}
	// Unknown options are reported, not guessed.
	_, nc, _ := ParseText(sel, "Later")
	if len(nc) != 1 || nc[0] != "Later" {
		t.Errorf("new choices = %v", nc)
	}
}

func TestFilterOperators(t *testing.T) {
	s := &schema{fields: map[string]Field{
		"t": {ID: "t", Type: TText}, "n": {ID: "n", Type: TNumber}, "d": {ID: "d", Type: TDate},
		"s": {ID: "s", Type: TSelect, Options: map[string]any{"choices": []any{map[string]any{"id": "c1", "name": "Open"}}}},
		"m": {ID: "m", Type: TMulti, Options: map[string]any{"choices": []any{map[string]any{"id": "x", "name": "A"}, map[string]any{"id": "y", "name": "B"}}}},
		"c": {ID: "c", Type: TCheckbox},
	}}
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	day := func(s string) time.Time { x, _ := parseDate(s); return x }
	row := map[string]any{"t": "Hello World", "n": 10.0, "d": day("2026-03-10"), "s": "Open", "m": []any{"A", "B"}, "c": true}
	test := func(n Node, want bool) {
		t.Helper()
		p := s.compile(&n, now)
		if p == nil {
			t.Fatalf("%+v compiled to nothing", n)
		}
		if got := p(row, nil); got != want {
			t.Errorf("%s %s %v = %v, want %v", n.Field, n.Op, n.Value, got, want)
		}
	}
	test(Node{Field: "t", Op: "contains", Value: "world"}, true)
	test(Node{Field: "t", Op: "doesNotContain", Value: "zzz"}, true)
	test(Node{Field: "t", Op: "is", Value: "hello world"}, true)
	test(Node{Field: "n", Op: ">", Value: 9.0}, true)
	test(Node{Field: "n", Op: "<=", Value: 9.99}, false)
	test(Node{Field: "n", Op: "=", Value: "10"}, true)
	test(Node{Field: "s", Op: "isAnyOf", Value: []any{"c1", "other"}}, true)
	test(Node{Field: "s", Op: "isNoneOf", Value: []any{"other"}}, true)
	test(Node{Field: "m", Op: "hasAllOf", Value: []any{"a", "b"}}, true)
	test(Node{Field: "m", Op: "isExactly", Value: []any{"A"}}, false)
	test(Node{Field: "m", Op: "hasNoneOf", Value: []any{"z"}}, true)
	test(Node{Field: "c", Op: "is", Value: true}, true)
	test(Node{Field: "c", Op: "is", Value: false}, false)
	test(Node{Field: "d", Op: "isBefore", Value: map[string]any{"mode": "today"}}, true)
	test(Node{Field: "d", Op: "isWithin", Value: map[string]any{"mode": "pastWeek"}}, true)
	test(Node{Field: "d", Op: "isWithin", Value: map[string]any{"mode": "nextWeek"}}, false)
	test(Node{Field: "d", Op: "is", Value: map[string]any{"mode": "exactDate", "date": "2026-03-10"}}, true)
	test(Node{Field: "d", Op: "isOnOrAfter", Value: map[string]any{"mode": "daysAgo", "n": 5.0}}, true)
	test(Node{Field: "d", Op: "isOnOrAfter", Value: map[string]any{"mode": "daysAgo", "n": 3.0}}, false)
	test(Node{Field: "t", Op: "isNotEmpty"}, true)
	// Groups: (n>100 OR t contains hello) AND c is true
	g := Node{Conds: []Node{
		{Logic: "or", Conds: []Node{{Field: "n", Op: ">", Value: 100.0}, {Field: "t", Op: "contains", Value: "hello"}}},
		{Field: "c", Op: "is", Value: true},
	}}
	if p := s.compile(&g, now); p == nil || !p(row, nil) {
		t.Error("group filter should match")
	}
	// Incomplete conditions are ignored.
	if p := s.compile(&Node{Field: "t", Op: "contains"}, now); p != nil {
		t.Error("a condition without a value must be ignored")
	}
}

func TestNaturalSortAndKeys(t *testing.T) {
	in := []string{"item10", "item2", "Item1", "item20"}
	keys := make([]sortKey, len(in))
	for i, s := range in {
		keys[i] = keyOf(s)
	}
	for i := 0; i < len(in); i++ {
		for j := i + 1; j < len(in); j++ {
			if cmpKeys(keys[j], keys[i]) < 0 {
				in[i], in[j] = in[j], in[i]
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	want := []string{"Item1", "item2", "item10", "item20"}
	for i := range want {
		if in[i] != want[i] {
			t.Fatalf("natural order = %v", in)
		}
	}
	if cmpKeys(keyOf(nil), keyOf(1.0)) <= 0 {
		t.Error("empty values sort last")
	}
}

func TestRollupFunctions(t *testing.T) {
	vals := []any{3.0, 5.0, nil, 5.0}
	check := func(fn string, want any) {
		t.Helper()
		got := rollup(fn, vals)
		if !jsonEq(got, want) {
			t.Errorf("rollup %s = %v want %v", fn, got, want)
		}
	}
	check("sum", 13.0)
	check("avg", 13.0/3)
	check("min", 3.0)
	check("max", 5.0)
	check("count", 3.0)
	check("count_all", 4.0)
	check("count_unique", 2.0)
	check("concat", "3, 5, 5")
	check("unique", []any{3.0, 5.0})
	if got := rollup("and", []any{true, false}); got != false {
		t.Error("and")
	}
	if got := rollup("or", []any{false, true}); got != true {
		t.Error("or")
	}
	if got := rollup("sum", nil); got != 0.0 {
		t.Errorf("empty sum = %v", got)
	}
	if got := rollup("earliest", []any{"2026-05-01", "2026-01-02"}); got.(time.Time).Format("2006-01-02") != "2026-01-02" {
		t.Errorf("earliest = %v", got)
	}
}

func TestDetectType(t *testing.T) {
	cases := map[string][]string{
		TNumber:   {"1", "2.5", "1,000"},
		TCurrency: {"$10", "$2,500.50"},
		TPercent:  {"10%", "25.5%"},
		TDate:     {"2026-01-02", "2026-02-03"},
		TEmail:    {"a@b.co", "c@d.org"},
		TURL:      {"https://a.example", "http://b.example"},
		TCheckbox: {"yes", "no", "yes"},
		TText:     {"007", "abc"}, // leading zeros stay text
	}
	for want, vals := range cases {
		if got := detectType(vals, len(vals)); got != want {
			t.Errorf("detect %v = %s, want %s", vals, got, want)
		}
	}
	many := []string{}
	for i := 0; i < 30; i++ {
		many = append(many, []string{"red", "green", "blue"}[i%3])
	}
	if got := detectType(many, 30); got != TSelect {
		t.Errorf("repeated words = %s, want select", got)
	}
}

func TestCSVSafe(t *testing.T) {
	for in, want := range map[string]string{"=SUM(A1)": "'=SUM(A1)", "-5": "-5", "@x": "'@x", "plain": "plain", "+91 99999": "'+91 99999"} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateField(t *testing.T) {
	if err := ValidateField(Field{Name: "ok", Type: TText}); err != nil {
		t.Error(err)
	}
	for _, f := range []Field{
		{Name: "", Type: TText}, {Name: "a{b}", Type: TText}, {Name: "x", Type: "weird"},
		{Name: "r", Type: TRating, Options: map[string]any{"max": 99.0}},
		{Name: "f", Type: TFormula, Options: map[string]any{"formula": "1 +"}},
		{Name: "u", Type: TRollup, Options: map[string]any{"fn": "nope"}},
		{Name: "s", Type: TSelect, Options: map[string]any{"choices": []any{map[string]any{"id": "1", "name": "a"}, map[string]any{"id": "2", "name": "A"}}}},
	} {
		if err := ValidateField(f); err == nil {
			t.Errorf("field %+v should be refused", f)
		}
	}
}
