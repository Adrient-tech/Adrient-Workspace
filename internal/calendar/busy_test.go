package calendar

import (
	"testing"
	"time"
)

func TestBusySpans(t *testing.T) {
	day := time.Date(2030, 1, 7, 0, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return day.Add(time.Duration(n) * time.Hour) }
	mine := func(e Event) bool { return e.OwnerID == "me" || (len(e.Attendees) > 0 && e.Attendees[0] == "me@x") }
	list := []Event{
		{ID: "a", OwnerID: "me", StartAt: h(9), EndAt: h(10), Status: StatusBusy},
		{ID: "b", OwnerID: "me", StartAt: h(11), EndAt: h(12), Status: StatusFree},                                 // free: ignored
		{ID: "c", OwnerID: "me", StartAt: h(13), EndAt: h(14), ItemType: ItemTypeTask, Status: StatusBusy},         // task: ignored
		{ID: "d", OwnerID: "other", StartAt: h(15), EndAt: h(16), Status: StatusBusy},                              // not mine
		{ID: "e", OwnerID: "other", Attendees: []string{"me@x"}, StartAt: h(17), EndAt: h(18), Status: StatusBusy}, // invited
		{ID: "f", OwnerID: "other", Attendees: []string{"me@x"}, StartAt: h(19), EndAt: h(20), Status: StatusBusy}, // declined
		{ID: "g", OwnerID: "me", StartAt: h(21), EndAt: h(22), Status: StatusBusy, Recurrence: "FREQ=DAILY"},
	}
	got := busySpans(list, day, day.Add(24*time.Hour), mine, map[string]bool{"f": true})
	want := map[time.Time]bool{h(9): true, h(17): true, h(21): true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, s := range got {
		if !want[s.Start] {
			t.Fatalf("unexpected span %v", s)
		}
	}
}
