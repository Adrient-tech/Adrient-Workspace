package tasks

import (
	"strings"
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseRule(t *testing.T) {
	for _, bad := range []string{"every", "every:0:daily", "every:2:hourly", "every:1:daily:1,2", "every:1:weekly:9", "every:x:daily", "weekdayz"} {
		if _, ok := ParseRule(bad); ok {
			t.Errorf("%q should be invalid", bad)
		}
	}
	r, ok := ParseRule("every:2:weekly:3,1:done")
	if !ok || r.Interval != 2 || !r.FromDone || len(r.Weekdays) != 2 || r.Weekdays[0] != 1 {
		t.Fatalf("got %+v ok=%v", r, ok)
	}
}

func TestRuleNext(t *testing.T) {
	now := d("2026-11-19 08:00") // Thursday
	cases := []struct{ rule, due, want string }{
		{"every:3:daily", "2026-11-19 09:00", "2026-11-22 09:00"},
		{"every:1:weekly:1,3", "2026-11-18 09:00", "2026-11-23 09:00"}, // Wed -> Mon
		{"every:2:weekly:1", "2026-11-23 09:00", "2026-12-07 09:00"},   // every other Monday
		{"every:1:monthly", "2026-12-31 09:00", "2027-01-31 09:00"},
		{"every:1:weekly:1,2,3,4,5", "2026-11-20 09:00", "2026-11-23 09:00"}, // Fri -> Mon
		{"every:1:daily", "2026-11-01 09:00", "2026-11-19 09:00"},            // overdue skips missed days
		{"every:1:daily:done", "2026-11-01 09:00", "2026-11-20 09:00"},       // rolls from completion day
	}
	for _, c := range cases {
		r, ok := ParseRule(c.rule)
		if !ok {
			t.Fatalf("parse %s", c.rule)
		}
		if got := r.Next(d(c.due), now); !got.Equal(d(c.want)) {
			t.Errorf("%s from %s: got %s want %s", c.rule, c.due, got.Format("2006-01-02 15:04"), c.want)
		}
	}
}

func TestNormalizeTags(t *testing.T) {
	got, err := NormalizeTags([]string{"#Home", "home", " Work ", ""})
	if err != nil || len(got) != 2 || got[0] != "home" || got[1] != "work" {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := NormalizeTags([]string{"a", "b", "c", "d", "e", "f"}); err == nil {
		t.Error("6 tags should fail")
	}
	if _, err := NormalizeTags([]string{"bad tag"}); err == nil {
		t.Error("space should fail")
	}
}

func TestBuildICS(t *testing.T) {
	min := 30
	s := BuildICS([]FeedTask{{ID: "1", Title: "Pay, rent", List: "Home", Due: d("2026-11-20 09:00"), UpdatedAt: d("2026-11-19 08:00"),
		Recurrence: "every:2:weekly:1", RemindMinutes: &min}}, "x.test")
	for _, w := range []string{"BEGIN:VCALENDAR", "UID:task-1@x.test", "SUMMARY:Pay\\, rent", "RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO", "TRIGGER:-PT30M", "DTSTART:20261120T090000Z"} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q in\n%s", w, s)
		}
	}
}
