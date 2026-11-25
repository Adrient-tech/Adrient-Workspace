package contacts

import (
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

func TestOccurrences(t *testing.T) {
	got := occurrences(d(1990, 5, 17), d(2030, 5, 1), d(2030, 6, 1))
	if len(got) != 1 || !got[0].Equal(d(2030, 5, 17)) {
		t.Fatalf("simple: %v", got)
	}
	// Window spanning New Year returns both years.
	got = occurrences(d(1990, 12, 31), d(2029, 12, 1), d(2030, 2, 1))
	if len(got) != 1 || !got[0].Equal(d(2029, 12, 31)) {
		t.Fatalf("year end: %v", got)
	}
	got = occurrences(d(1990, 1, 5), d(2029, 12, 1), d(2030, 2, 1))
	if len(got) != 1 || !got[0].Equal(d(2030, 1, 5)) {
		t.Fatalf("new year: %v", got)
	}
	// Leap day shows on 28 Feb in a normal year and 29 Feb in a leap year.
	got = occurrences(d(1992, 2, 29), d(2030, 2, 1), d(2030, 3, 1))
	if len(got) != 1 || !got[0].Equal(d(2030, 2, 28)) {
		t.Fatalf("leap in common year: %v", got)
	}
	got = occurrences(d(1992, 2, 29), d(2028, 2, 1), d(2028, 3, 1))
	if len(got) != 1 || !got[0].Equal(d(2028, 2, 29)) {
		t.Fatalf("leap in leap year: %v", got)
	}
	// Never before the birth year.
	if got = occurrences(d(2031, 5, 17), d(2030, 5, 1), d(2030, 6, 1)); len(got) != 0 {
		t.Fatalf("future birth: %v", got)
	}
}

func TestExtrasNormalize(t *testing.T) {
	e := Extras{Birthday: "1990-05-17", Anniversary: "", Website: "example.com", Address: "1 Main St"}
	if err := e.Normalize(); err != nil || e.Website != "https://example.com" {
		t.Fatalf("%v %+v", err, e)
	}
	for _, bad := range []Extras{{Birthday: "17/05/1990"}, {Anniversary: "1800-01-01"}, {Birthday: "2030-02-31"}} {
		if bad.Normalize() == nil {
			t.Fatalf("%+v should fail", bad)
		}
	}
	long := Extras{Address: string(make([]rune, 600))}
	for i := range long.Address {
		_ = i
	}
	long.Address = ""
	for i := 0; i < 600; i++ {
		long.Address += "x"
	}
	if long.Normalize() == nil {
		t.Fatal("long address must fail")
	}
}
