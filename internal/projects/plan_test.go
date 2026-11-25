package projects

import "testing"

func TestWouldCycle(t *testing.T) {
	deps := map[string][]string{"b": {"a"}, "c": {"b"}, "d": {}}
	cases := []struct {
		issue string
		new   []string
		want  bool
	}{
		{"a", []string{"c"}, true},  // a -> c -> b -> a
		{"a", []string{"b"}, true},  // direct loop
		{"a", []string{"a"}, true},  // self
		{"d", []string{"c"}, false}, // fine
		{"c", []string{"a"}, false}, // shortcut, no loop
		{"x", []string{"y", "z"}, false},
	}
	for _, c := range cases {
		if got := WouldCycle(deps, c.issue, c.new); got != c.want {
			t.Errorf("%s needs %v: got %v want %v", c.issue, c.new, got, c.want)
		}
	}
}

func TestDateRange(t *testing.T) {
	a, b, err := DateRange("2030-01-05", "2030-01-10")
	if err != nil || a == nil || b == nil {
		t.Fatal(err)
	}
	if _, _, err := DateRange("", ""); err != nil {
		t.Fatal("both empty is fine")
	}
	if a, _, err := DateRange("2030-01-05", ""); err != nil || a == nil {
		t.Fatal("start only is fine")
	}
	for _, bad := range [][2]string{{"2030-02-01", "2030-01-01"}, {"05/01/2030", ""}, {"", "1999-01-01"}, {"2030-13-01", ""}} {
		if _, _, err := DateRange(bad[0], bad[1]); err == nil {
			t.Errorf("%v should fail", bad)
		}
	}
}

func TestTemplatesAreConsistent(t *testing.T) {
	ts := Templates()
	if len(ts) < 3 {
		t.Fatal("expected built-in templates")
	}
	for _, tpl := range ts {
		if err := tpl.Validate(); err != nil {
			t.Errorf("%s: %v", tpl.ID, err)
		}
		// The template's own dependencies must not loop.
		deps := map[string][]string{}
		for i, is := range tpl.Issues {
			id := string(rune('a' + i))
			var d []string
			for _, n := range is.Needs {
				d = append(d, string(rune('a'+n)))
			}
			if WouldCycle(deps, id, d) {
				t.Errorf("%s: issue %d loops", tpl.ID, i)
			}
			deps[id] = d
		}
	}
	if _, ok := FindTemplate("nope"); ok {
		t.Fatal("unknown template")
	}
	if _, ok := FindTemplate("software-release"); !ok {
		t.Fatal("known template")
	}
}

func TestTemplatesHaveNoScheduleConflicts(t *testing.T) {
	for _, tpl := range Templates() {
		for i, is := range tpl.Issues {
			for _, n := range is.Needs {
				pre := tpl.Issues[n]
				end := pre.StartDay + pre.Days - 1 // inclusive last day
				if end >= is.StartDay {
					t.Errorf("%s: issue %d starts day %d but prerequisite %d ends day %d", tpl.ID, i, is.StartDay, n, end)
				}
			}
		}
	}
}
