// Package booking implements appointment schedules: public booking pages that
// offer the owner's free time as bookable slots (Calendly-style).
package booking

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Window is a daily availability window in the page time zone ("HH:MM").
type Window struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Availability maps weekday ("0" = Sunday ... "6" = Saturday) to windows.
type Availability map[string][]Window

// Span is a half-open time range.
type Span struct{ Start, End time.Time }

// Rules are the scheduling rules of one page.
type Rules struct {
	Duration     time.Duration
	BufferBefore time.Duration
	BufferAfter  time.Duration
	MinNotice    time.Duration
	MaxDaysAhead int
	Step         time.Duration
	MaxPerDay    int
	Location     *time.Location
	Avail        Availability
}

// parseHM parses "HH:MM" into minutes since midnight.
func parseHM(s string) (int, error) {
	p := strings.Split(strings.TrimSpace(s), ":")
	if len(p) != 2 {
		return 0, fmt.Errorf("bad time %q", s)
	}
	h, err1 := strconv.Atoi(p[0])
	m, err2 := strconv.Atoi(p[1])
	if err1 != nil || err2 != nil || h < 0 || h > 24 || m < 0 || m > 59 || (h == 24 && m != 0) {
		return 0, fmt.Errorf("bad time %q", s)
	}
	return h*60 + m, nil
}

// ValidateAvailability checks windows are well formed, ordered and non overlapping.
func ValidateAvailability(a Availability) error {
	for day, ws := range a {
		d, err := strconv.Atoi(day)
		if err != nil || d < 0 || d > 6 {
			return fmt.Errorf("invalid weekday %q", day)
		}
		type iv struct{ s, e int }
		var ivs []iv
		for _, w := range ws {
			s, err := parseHM(w.Start)
			if err != nil {
				return err
			}
			e, err := parseHM(w.End)
			if err != nil {
				return err
			}
			if e <= s {
				return errors.New("a window must end after it starts")
			}
			ivs = append(ivs, iv{s, e})
		}
		sort.Slice(ivs, func(i, j int) bool { return ivs[i].s < ivs[j].s })
		for i := 1; i < len(ivs); i++ {
			if ivs[i].s < ivs[i-1].e {
				return errors.New("availability windows overlap")
			}
		}
	}
	return nil
}

// ComputeSlots returns the start times a guest may book in [from, to).
// busy are the owner's busy spans; booked counts confirmed appointments per
// local day ("2006-01-02") for the MaxPerDay limit. Results are sorted, UTC.
func ComputeSlots(r Rules, busy []Span, booked map[string]int, from, to, now time.Time) []time.Time {
	if r.Duration <= 0 || r.Location == nil {
		return nil
	}
	step := r.Step
	if step <= 0 {
		step = r.Duration
	}
	earliest := now.Add(r.MinNotice)
	latest := now.Add(time.Duration(r.MaxDaysAhead) * 24 * time.Hour)
	if r.MaxDaysAhead <= 0 {
		latest = now.Add(60 * 24 * time.Hour)
	}
	if to.After(latest) {
		to = latest
	}
	if from.Before(earliest) {
		from = earliest
	}
	if !from.Before(to) {
		return nil
	}
	var out []time.Time
	// Walk local calendar days (one extra on each side covers zone offsets).
	day := time.Date(from.In(r.Location).Year(), from.In(r.Location).Month(), from.In(r.Location).Day(), 0, 0, 0, 0, r.Location)
	day = day.AddDate(0, 0, -1)
	endDay := to.In(r.Location).AddDate(0, 0, 1)
	for ; day.Before(endDay); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		count := booked[key]
		for _, w := range r.Avail[strconv.Itoa(int(day.Weekday()))] {
			ws, err1 := parseHM(w.Start)
			we, err2 := parseHM(w.End)
			if err1 != nil || err2 != nil || we <= ws {
				continue
			}
			// time.Date normalises minutes beyond 59 and respects DST.
			wStart := time.Date(day.Year(), day.Month(), day.Day(), 0, ws, 0, 0, r.Location)
			wEnd := time.Date(day.Year(), day.Month(), day.Day(), 0, we, 0, 0, r.Location)
			for s := wStart; !s.Add(r.Duration).After(wEnd); s = s.Add(step) {
				if s.Before(from) || !s.Before(to) {
					continue
				}
				if r.MaxPerDay > 0 && count >= r.MaxPerDay {
					break
				}
				e := s.Add(r.Duration)
				if conflicts(busy, s.Add(-r.BufferBefore), e.Add(r.BufferAfter)) {
					continue
				}
				out = append(out, s.UTC())
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	// De-duplicate (overlapping day walks cannot produce duplicates, but be safe).
	uniq := out[:0]
	for i, t := range out {
		if i == 0 || !t.Equal(out[i-1]) {
			uniq = append(uniq, t)
		}
	}
	return uniq
}

func conflicts(busy []Span, s, e time.Time) bool {
	for _, b := range busy {
		if b.Start.Before(e) && b.End.After(s) {
			return true
		}
	}
	return false
}

// IsBookable reports whether start is exactly one of the offered slots.
func IsBookable(r Rules, busy []Span, booked map[string]int, start, now time.Time) bool {
	for _, s := range ComputeSlots(r, busy, booked, start.Add(-time.Minute), start.Add(time.Minute), now) {
		if s.Equal(start) {
			return true
		}
	}
	return false
}
