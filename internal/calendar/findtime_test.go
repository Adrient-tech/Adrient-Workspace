package calendar

import (
	"testing"
	"time"
)

func TestWorkingHoursContainsAndValidate(t *testing.T) {
	w := DefaultWorkingHours()
	w.Timezone = "Asia/Kolkata"
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
	mon := time.Date(2030, 1, 7, 4, 0, 0, 0, time.UTC) // 09:30 IST Monday
	if !w.Contains(mon, mon.Add(time.Hour)) {
		t.Fatal("09:30 IST should be inside")
	}
	if w.Contains(mon.Add(-2*time.Hour), mon) { // 07:30 IST
		t.Fatal("07:30 IST must be outside")
	}
	sat := time.Date(2030, 1, 12, 5, 0, 0, 0, time.UTC)
	if w.Contains(sat, sat.Add(time.Hour)) {
		t.Fatal("Saturday has no hours")
	}
	bad := WorkingHours{Timezone: "Nope/Zone"}
	if bad.Validate() == nil {
		t.Fatal("bad zone must fail")
	}
	bad = WorkingHours{Timezone: "UTC", Days: map[string][]HoursWindow{"1": {{"09:00", "11:00"}, {"10:00", "12:00"}}}}
	if bad.Validate() == nil {
		t.Fatal("overlap must fail")
	}
}

func TestFindTimes(t *testing.T) {
	utc := DefaultWorkingHours()
	day := time.Date(2030, 1, 7, 0, 0, 0, 0, time.UTC) // Monday
	h := func(n float64) time.Time { return day.Add(time.Duration(n * float64(time.Hour))) }
	a := Person{Email: "a", Hours: utc, Busy: []BusySpan{{h(9), h(11)}}}
	b := Person{Email: "b", Hours: utc, Busy: []BusySpan{{h(13), h(14)}}}
	got := FindTimes([]Person{a, b}, h(0), h(24), time.Hour, time.Hour, 3)
	if len(got) != 3 || !got[0].Start.Equal(h(11)) || !got[1].Start.Equal(h(12)) || !got[2].Start.Equal(h(14)) {
		t.Fatalf("got %+v", got)
	}
	for _, s := range got {
		if len(s.Conflicts) != 0 {
			t.Fatal("full matches must have no conflicts")
		}
	}
	// Only one full match exists; the rest are partial with named conflicts.
	a.Busy = []BusySpan{{h(9), h(16)}}
	got = FindTimes([]Person{a, b}, h(0), h(24), time.Hour, time.Hour, 3)
	if len(got) != 3 || len(got[0].Conflicts) == 0 {
		t.Fatalf("partials: %+v", got)
	}
	if got := FindTimes(nil, h(0), h(24), time.Hour, time.Hour, 3); got != nil {
		t.Fatal("no people, no suggestions")
	}
}
