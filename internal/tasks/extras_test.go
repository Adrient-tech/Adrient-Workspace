package tasks

import (
	"testing"
	"time"
)

func TestNextDue(t *testing.T) {
	d := func(y int, m time.Month, day, h int) time.Time { return time.Date(y, m, day, h, 30, 0, 0, time.UTC) }
	cases := []struct {
		rule string
		from time.Time
		want time.Time
	}{
		{"daily", d(2030, 1, 31, 9), d(2030, 2, 1, 9)},
		{"weekly", d(2030, 1, 31, 9), d(2030, 2, 7, 9)},
		{"weekdays", d(2030, 1, 11, 9), d(2030, 1, 14, 9)}, // Friday -> Monday
		{"weekdays", d(2030, 1, 9, 9), d(2030, 1, 10, 9)},
		{"monthly", d(2030, 1, 31, 9), d(2030, 2, 28, 9)}, // clamped
		{"monthly", d(2028, 1, 31, 9), d(2028, 2, 29, 9)}, // leap year
		{"monthly", d(2030, 12, 15, 9), d(2031, 1, 15, 9)},
		{"yearly", d(2028, 2, 29, 9), d(2029, 2, 28, 9)},
	}
	for _, c := range cases {
		got, ok := NextDue(c.from, c.rule)
		if !ok || !got.Equal(c.want) {
			t.Errorf("%s from %v: got %v want %v", c.rule, c.from, got, c.want)
		}
	}
	if _, ok := NextDue(d(2030, 1, 1, 9), ""); ok {
		t.Error("no rule must not repeat")
	}
}

func TestExtrasNormalize(t *testing.T) {
	five := 5
	ok := Extras{Priority: 2, Recurrence: "weekly", RemindMinutes: &five, LinkURL: "https://x.test/a"}
	if err := ok.Normalize(); err != nil || ok.BoardStatus != "todo" {
		t.Fatalf("%v %+v", err, ok)
	}
	neg := -1
	for _, bad := range []Extras{
		{Priority: 4}, {Priority: -1}, {Recurrence: "hourly"}, {BoardStatus: "later"},
		{RemindMinutes: &neg}, {LinkURL: "javascript:alert(1)"},
	} {
		if bad.Normalize() == nil {
			t.Errorf("%+v should fail", bad)
		}
	}
	rel := Extras{LinkURL: "/mail?id=1"}
	if rel.Normalize() != nil {
		t.Error("app-relative links are allowed")
	}
}

func TestRemindersDue(t *testing.T) {
	now := time.Date(2030, 1, 7, 10, 0, 0, 0, time.UTC)
	rows := []dueTask{
		{ID: "due", Due: now.Add(30 * time.Minute), Minutes: 30},    // fires now
		{ID: "soon", Due: now.Add(45 * time.Minute), Minutes: 30},   // fires in 15 min
		{ID: "old", Due: now.Add(-50 * time.Minute), Minutes: 30},   // fired 80 min ago
		{ID: "atdue", Due: now.Add(-5 * time.Minute), Minutes: 0},   // due 5 min ago, remind at due
		{ID: "recent", Due: now.Add(10 * time.Minute), Minutes: 20}, // fired 10 min ago
	}
	got := map[string]bool{}
	for _, d := range remindersDue(rows, now, 30*time.Minute) {
		got[d.ID] = true
	}
	want := map[string]bool{"due": true, "atdue": true, "recent": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("missing %s in %v", k, got)
		}
	}
}
