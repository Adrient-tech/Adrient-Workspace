package calendar

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HoursWindow is a daily working window ("HH:MM").
type HoursWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// WorkingHours maps weekday ("0" = Sunday ... "6") to windows, in Zone.
type WorkingHours struct {
	Timezone string                   `json:"timezone"`
	Days     map[string][]HoursWindow `json:"days"`
}

// DefaultWorkingHours is Monday to Friday, 09:00 to 17:00 UTC.
func DefaultWorkingHours() WorkingHours {
	d := map[string][]HoursWindow{}
	for _, k := range []string{"1", "2", "3", "4", "5"} {
		d[k] = []HoursWindow{{Start: "09:00", End: "17:00"}}
	}
	return WorkingHours{Timezone: "UTC", Days: d}
}

func hmMinutes(s string) (int, error) {
	p := strings.Split(strings.TrimSpace(s), ":")
	if len(p) != 2 {
		return 0, fmt.Errorf("bad time %q", s)
	}
	h, e1 := strconv.Atoi(p[0])
	m, e2 := strconv.Atoi(p[1])
	if e1 != nil || e2 != nil || h < 0 || h > 24 || m < 0 || m > 59 || (h == 24 && m != 0) {
		return 0, fmt.Errorf("bad time %q", s)
	}
	return h*60 + m, nil
}

// Validate checks the zone and that windows are ordered and do not overlap.
func (w WorkingHours) Validate() error {
	if _, err := time.LoadLocation(w.Timezone); err != nil {
		return fmt.Errorf("unknown time zone")
	}
	for day, ws := range w.Days {
		d, err := strconv.Atoi(day)
		if err != nil || d < 0 || d > 6 {
			return fmt.Errorf("invalid weekday %q", day)
		}
		type iv struct{ s, e int }
		var ivs []iv
		for _, x := range ws {
			s, err := hmMinutes(x.Start)
			if err != nil {
				return err
			}
			e, err := hmMinutes(x.End)
			if err != nil {
				return err
			}
			if e <= s {
				return fmt.Errorf("a window must end after it starts")
			}
			ivs = append(ivs, iv{s, e})
		}
		sort.Slice(ivs, func(i, j int) bool { return ivs[i].s < ivs[j].s })
		for i := 1; i < len(ivs); i++ {
			if ivs[i].s < ivs[i-1].e {
				return fmt.Errorf("windows overlap")
			}
		}
	}
	return nil
}

// Contains reports whether [s, e) lies fully inside one working window.
func (w WorkingHours) Contains(s, e time.Time) bool {
	loc, err := time.LoadLocation(w.Timezone)
	if err != nil {
		return false
	}
	ls, le := s.In(loc), e.In(loc)
	if ls.YearDay() != le.YearDay() || ls.Year() != le.Year() {
		return false
	}
	for _, x := range w.Days[strconv.Itoa(int(ls.Weekday()))] {
		a, e1 := hmMinutes(x.Start)
		b, e2 := hmMinutes(x.End)
		if e1 != nil || e2 != nil {
			continue
		}
		start := time.Date(ls.Year(), ls.Month(), ls.Day(), 0, a, 0, 0, loc)
		end := time.Date(ls.Year(), ls.Month(), ls.Day(), 0, b, 0, 0, loc)
		if !ls.Before(start) && !le.After(end) {
			return true
		}
	}
	return false
}

// Person is one participant for find-a-time.
type Person struct {
	Email string
	Busy  []BusySpan
	Hours WorkingHours
}

// Suggestion is a start time at which everyone is free.
type Suggestion struct {
	Start, End time.Time
	// Conflicts lists people who are busy or outside working hours (only
	// filled when the suggestion is a partial match).
	Conflicts []string
}

// FindTimes returns up to limit start times in [from, to) of the given
// length, on a step grid, where every person is free and inside their working
// hours. If fewer than limit full matches exist, it adds the best partial ones
// (fewest conflicts) so the user still has options. Results are time ordered.
func FindTimes(people []Person, from, to time.Time, dur, step time.Duration, limit int) []Suggestion {
	if dur <= 0 || step <= 0 || limit <= 0 || !from.Before(to) || len(people) == 0 {
		return nil
	}
	from = from.Truncate(step)
	var full, partial []Suggestion
	for s := from; !s.Add(dur).After(to); s = s.Add(step) {
		e := s.Add(dur)
		var conflicts []string
		for _, p := range people {
			bad := !p.Hours.Contains(s, e)
			if !bad {
				for _, b := range p.Busy {
					if b.Start.Before(e) && b.End.After(s) {
						bad = true
						break
					}
				}
			}
			if bad {
				conflicts = append(conflicts, p.Email)
			}
		}
		if len(conflicts) == 0 {
			full = append(full, Suggestion{Start: s, End: e})
			if len(full) >= limit {
				break
			}
		} else if len(conflicts) < len(people) {
			partial = append(partial, Suggestion{Start: s, End: e, Conflicts: conflicts})
		}
	}
	out := full
	if len(out) < limit {
		sort.SliceStable(partial, func(i, j int) bool { return len(partial[i].Conflicts) < len(partial[j].Conflicts) })
		for _, p := range partial {
			if len(out) >= limit {
				break
			}
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}
