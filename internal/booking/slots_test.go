package booking

import (
	"testing"
	"time"
)

func rules(t *testing.T, tz string) Rules {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatal(err)
	}
	return Rules{
		Duration: 30 * time.Minute, Step: 30 * time.Minute, MaxDaysAhead: 30, Location: loc,
		Avail: Availability{"1": {{Start: "09:00", End: "11:00"}}, "2": {{Start: "09:00", End: "10:00"}, {Start: "14:00", End: "15:00"}}},
	}
}

func TestComputeSlotsBasic(t *testing.T) {
	r := rules(t, "UTC")
	now := time.Date(2030, 1, 6, 0, 0, 0, 0, time.UTC) // Sunday
	from := time.Date(2030, 1, 7, 0, 0, 0, 0, time.UTC)
	got := ComputeSlots(r, nil, nil, from, from.Add(24*time.Hour), now)
	if len(got) != 4 || !got[0].Equal(time.Date(2030, 1, 7, 9, 0, 0, 0, time.UTC)) || !got[3].Equal(time.Date(2030, 1, 7, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("slots = %v", got)
	}
	// Tuesday has two windows -> 2 + 2 slots.
	tue := from.Add(24 * time.Hour)
	if n := len(ComputeSlots(r, nil, nil, tue, tue.Add(24*time.Hour), now)); n != 4 {
		t.Fatalf("tuesday slots = %d", n)
	}
	// Wednesday has no windows.
	if n := len(ComputeSlots(r, nil, nil, tue.Add(24*time.Hour), tue.Add(48*time.Hour), now)); n != 0 {
		t.Fatalf("wednesday slots = %d", n)
	}
}

func TestComputeSlotsBusyAndBuffers(t *testing.T) {
	r := rules(t, "UTC")
	r.BufferBefore = 15 * time.Minute
	r.BufferAfter = 15 * time.Minute
	now := time.Date(2030, 1, 6, 0, 0, 0, 0, time.UTC)
	from := time.Date(2030, 1, 7, 0, 0, 0, 0, time.UTC)
	busy := []Span{{time.Date(2030, 1, 7, 9, 30, 0, 0, time.UTC), time.Date(2030, 1, 7, 10, 0, 0, 0, time.UTC)}}
	got := ComputeSlots(r, busy, nil, from, from.Add(24*time.Hour), now)
	// 09:00 (+15 after -> 09:45 overlaps 09:30), 09:30 busy, 10:00 (-15 before -> 09:45 overlaps) blocked, 10:30 ok.
	if len(got) != 1 || !got[0].Equal(time.Date(2030, 1, 7, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("slots = %v", got)
	}
}

func TestComputeSlotsNoticeHorizonAndMaxPerDay(t *testing.T) {
	r := rules(t, "UTC")
	r.MinNotice = 2 * time.Hour
	now := time.Date(2030, 1, 7, 8, 30, 0, 0, time.UTC) // Monday 08:30
	from := time.Date(2030, 1, 7, 0, 0, 0, 0, time.UTC)
	got := ComputeSlots(r, nil, nil, from, from.Add(24*time.Hour), now)
	if len(got) != 0 { // earliest 10:30, window ends 11:00 -> only 10:30
		if len(got) != 1 || !got[0].Equal(time.Date(2030, 1, 7, 10, 30, 0, 0, time.UTC)) {
			t.Fatalf("slots = %v", got)
		}
	}
	r.MinNotice = 0
	r.MaxPerDay = 2
	if n := len(ComputeSlots(r, nil, map[string]int{"2030-01-07": 2}, from, from.Add(24*time.Hour), now.Add(-24*time.Hour))); n != 0 {
		t.Fatalf("max per day not applied: %d", n)
	}
	r.MaxDaysAhead = 1
	far := time.Date(2030, 1, 14, 0, 0, 0, 0, time.UTC)
	if n := len(ComputeSlots(r, nil, nil, far, far.Add(24*time.Hour), now)); n != 0 {
		t.Fatalf("horizon not applied: %d", n)
	}
}

func TestComputeSlotsTimeZoneAndDST(t *testing.T) {
	r := rules(t, "America/New_York")
	now := time.Date(2030, 3, 1, 0, 0, 0, 0, time.UTC)
	// Monday 2030-03-11 is the day after US DST starts (EDT, UTC-4): 09:00 local = 13:00 UTC.
	from := time.Date(2030, 3, 11, 0, 0, 0, 0, time.UTC)
	got := ComputeSlots(r, nil, nil, from, from.Add(24*time.Hour), now)
	if len(got) == 0 || !got[0].Equal(time.Date(2030, 3, 11, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("EDT slots = %v", got)
	}
	// Monday 2030-03-04 is before DST (EST, UTC-5): 09:00 local = 14:00 UTC.
	from = time.Date(2030, 3, 4, 0, 0, 0, 0, time.UTC)
	got = ComputeSlots(r, nil, nil, from, from.Add(24*time.Hour), now)
	if len(got) == 0 || !got[0].Equal(time.Date(2030, 3, 4, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("EST slots = %v", got)
	}
}

func TestIsBookable(t *testing.T) {
	r := rules(t, "UTC")
	now := time.Date(2030, 1, 6, 0, 0, 0, 0, time.UTC)
	if !IsBookable(r, nil, nil, time.Date(2030, 1, 7, 9, 30, 0, 0, time.UTC), now) {
		t.Fatal("09:30 should be bookable")
	}
	if IsBookable(r, nil, nil, time.Date(2030, 1, 7, 9, 10, 0, 0, time.UTC), now) {
		t.Fatal("off-grid time must not be bookable")
	}
	if IsBookable(r, nil, nil, time.Date(2030, 1, 7, 11, 0, 0, 0, time.UTC), now) {
		t.Fatal("outside window must not be bookable")
	}
}

func TestValidateAvailability(t *testing.T) {
	if err := ValidateAvailability(Availability{"1": {{"09:00", "12:00"}, {"13:00", "17:00"}}}); err != nil {
		t.Fatal(err)
	}
	bad := []Availability{
		{"7": {{"09:00", "10:00"}}},
		{"1": {{"10:00", "09:00"}}},
		{"1": {{"09:00", "11:00"}, {"10:00", "12:00"}}},
		{"1": {{"9am", "10:00"}}},
	}
	for i, a := range bad {
		if ValidateAvailability(a) == nil {
			t.Fatalf("case %d should fail", i)
		}
	}
}
