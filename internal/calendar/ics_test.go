package calendar

import (
	"strings"
	"testing"
	"time"
)

const sampleICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\nUID:a1@x\r\nSUMMARY:Team sync\\, weekly\r\nDESCRIPTION:Line1\\nLine2 and a long\r\n  folded part\r\n" +
	"LOCATION:Room 4\r\nDTSTART;TZID=Asia/Kolkata:20300107T093000\r\nDTEND;TZID=Asia/Kolkata:20300107T100000\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20300301T000000Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a2@x\r\nSUMMARY:Holiday\r\nDTSTART;VALUE=DATE:20300110\r\nDTEND;VALUE=DATE:20300112\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a3@x\r\nSUMMARY:Dur\r\nDTSTART:20300108T100000Z\r\nDURATION:PT1H30M\r\nRRULE:FREQ=MONTHLY;BYDAY=1MO\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:bad\r\nSUMMARY:No start\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func TestParseICS(t *testing.T) {
	ev, skipped := ParseICS(sampleICS)
	if len(ev) != 3 || skipped != 1 {
		t.Fatalf("events=%d skipped=%d", len(ev), skipped)
	}
	a := ev[0]
	if a.Title != "Team sync, weekly" || a.Location != "Room 4" || !strings.Contains(a.Description, "Line1\nLine2 and a long folded part") {
		t.Fatalf("text fields: %+v", a)
	}
	if !a.Start.Equal(time.Date(2030, 1, 7, 4, 0, 0, 0, time.UTC)) || !a.End.Equal(time.Date(2030, 1, 7, 4, 30, 0, 0, time.UTC)) {
		t.Fatalf("tz conversion: %v %v", a.Start, a.End)
	}
	if a.Recurrence != "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=2030-03-01T00:00:00Z" {
		t.Fatalf("rrule: %q", a.Recurrence)
	}
	if _, ok := parseRecurrence(a.Recurrence); !ok {
		t.Fatal("converted rule must be parseable by the expander")
	}
	if !ev[1].AllDay || !ev[1].End.Equal(time.Date(2030, 1, 12, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("all day: %+v", ev[1])
	}
	if !ev[2].End.Equal(ev[2].Start.Add(90*time.Minute)) || ev[2].Recurrence != "" {
		t.Fatalf("duration/unsupported rule: %+v", ev[2])
	}
}

func TestBuildICSRoundTripAndPrivacy(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	evs := []Event{
		{ID: "1", Title: "Plan; budget", Description: "x", StartAt: time.Date(2030, 1, 7, 9, 0, 0, 0, time.UTC), EndAt: time.Date(2030, 1, 7, 10, 0, 0, 0, time.UTC),
			Recurrence: "FREQ=WEEKLY;BYDAY=MO;COUNT=3", Reminders: []int32{10}},
		{ID: "2", Title: "Secret", Description: "hush", Location: "Vault", Visibility: VisibilityPrivate,
			StartAt: time.Date(2030, 1, 8, 9, 0, 0, 0, time.UTC), EndAt: time.Date(2030, 1, 8, 10, 0, 0, 0, time.UTC)},
		{ID: "3", Title: "Task", ItemType: ItemTypeTask, StartAt: now, EndAt: now},
	}
	out := BuildCalendarICS("Me", evs, now)
	if strings.Contains(out, "Secret") || strings.Contains(out, "Vault") || strings.Contains(out, "Task") {
		t.Fatalf("leaked private or task data:\n%s", out)
	}
	if !strings.Contains(out, "SUMMARY:Plan\\; budget") || !strings.Contains(out, "TRIGGER:-PT10M") || !strings.Contains(out, "RRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=3") {
		t.Fatalf("missing fields:\n%s", out)
	}
	back, _ := ParseICS(out)
	if len(back) != 2 || back[0].Title != "Plan; budget" || back[0].Recurrence != "FREQ=WEEKLY;BYDAY=MO;COUNT=3" || back[1].Title != "Busy" {
		t.Fatalf("round trip: %+v", back)
	}
	for _, l := range strings.Split(out, "\r\n") {
		if len(l) > 75 {
			t.Fatalf("unfolded line of %d bytes", len(l))
		}
	}
}
