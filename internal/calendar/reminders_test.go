package calendar

import (
	"testing"
	"time"
)

func TestDueReminders(t *testing.T) {
	now := time.Date(2030, 1, 7, 10, 0, 0, 0, time.UTC)
	mk := func(id string, start time.Time, rec string, rem ...int32) Event {
		return Event{ID: id, StartAt: start, EndAt: start.Add(time.Hour), Recurrence: rec, Reminders: rem}
	}
	evs := []Event{
		mk("due", now.Add(10*time.Minute), "", 10),                           // fires exactly now
		mk("early", now.Add(30*time.Minute), "", 10),                         // not yet
		mk("stale", now.Add(5*time.Minute), "", 60),                          // fire time 55 min ago: too old
		mk("started", now.Add(-time.Minute), "", 5),                          // already started
		mk("two", now.Add(5*time.Minute), "", 5, 0),                          // 5-min reminder due; 0-min not yet
		mk("none", now.Add(10*time.Minute), ""),                              // no reminders
		mk("daily", now.Add(-24*time.Hour+10*time.Minute), "FREQ=DAILY", 10), // next occurrence in 10 min
	}
	got := dueReminders(evs, now, 15*time.Minute)
	ids := map[string]int{}
	for _, d := range got {
		ids[d.Event.ID]++
	}
	want := map[string]int{"due": 1, "two": 1, "daily": 1}
	if len(ids) != len(want) {
		t.Fatalf("got %v", ids)
	}
	for k, v := range want {
		if ids[k] != v {
			t.Fatalf("got %v want %v", ids, want)
		}
	}
	for _, d := range got {
		if d.Event.ID == "daily" && !d.Occurrence.Equal(now.Add(10*time.Minute)) {
			t.Fatalf("daily occurrence = %v", d.Occurrence)
		}
	}
}
