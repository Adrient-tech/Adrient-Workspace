package tickets

import (
	"fmt"
	"strings"
	"time"
)

// Calendar is a set of weekly working windows in one time zone, minus holidays.
// A nil *Calendar means round the clock (wall-clock minutes).
type Calendar struct {
	TZ       string                 `json:"tz"`
	Hours    map[string][][2]string `json:"hours"` // "mon".."sun" -> [["09:00","18:00"], ...]
	Holidays map[string]bool        `json:"-"`     // "2026-01-26" -> true
	loc      *time.Location
}

var dayKeys = [...]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// maxCalendarDays bounds every walk over days so a bad calendar cannot loop forever.
const maxCalendarDays = 800

// Prepare resolves the time zone; it must be called before use.
func (c *Calendar) Prepare() error {
	if c == nil {
		return nil
	}
	loc, err := time.LoadLocation(c.TZ)
	if err != nil {
		return fmt.Errorf("unknown time zone %q", c.TZ)
	}
	c.loc = loc
	for k, ws := range c.Hours {
		ok := false
		for _, d := range dayKeys {
			ok = ok || d == k
		}
		if !ok {
			return fmt.Errorf("unknown weekday %q", k)
		}
		for _, w := range ws {
			a, b := parseHM(w[0]), parseHM(w[1])
			if a < 0 || b < 0 || a >= b {
				return fmt.Errorf("invalid window %s-%s on %s", w[0], w[1], k)
			}
		}
	}
	return nil
}

func parseHM(s string) int {
	var h, m int
	if n, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); n != 2 || err != nil || h < 0 || h > 24 || m < 0 || m > 59 || (h == 24 && m != 0) {
		return -1
	}
	return h*60 + m
}

// windows returns the working intervals of the local day starting at midnight `day`.
func (c *Calendar) windows(day time.Time) [][2]time.Time {
	if c.Holidays[day.Format("2006-01-02")] {
		return nil
	}
	var out [][2]time.Time
	for _, w := range c.Hours[dayKeys[day.Weekday()]] {
		a, b := parseHM(w[0]), parseHM(w[1])
		y, mo, d := day.Date()
		out = append(out, [2]time.Time{
			time.Date(y, mo, d, 0, a, 0, 0, c.loc),
			time.Date(y, mo, d, 0, b, 0, 0, c.loc),
		})
	}
	return out
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// BusinessMinutes counts working minutes between a and b (0 if b <= a).
func (c *Calendar) BusinessMinutes(a, b time.Time) int {
	if !b.After(a) {
		return 0
	}
	if c == nil {
		return int(b.Sub(a) / time.Minute)
	}
	a, b = a.In(c.loc), b.In(c.loc)
	total := time.Duration(0)
	day := startOfDay(a)
	for i := 0; i < maxCalendarDays && day.Before(b); i++ {
		for _, w := range c.windows(day) {
			s, e := w[0], w[1]
			if s.Before(a) {
				s = a
			}
			if e.After(b) {
				e = b
			}
			if e.After(s) {
				total += e.Sub(s)
			}
		}
		day = startOfDay(day.AddDate(0, 0, 1).Add(2 * time.Hour)) // survive DST shifts
	}
	return int(total / time.Minute)
}

// AddBusinessMinutes returns the instant `mins` working minutes after start.
func (c *Calendar) AddBusinessMinutes(start time.Time, mins int) time.Time {
	if mins <= 0 {
		return start
	}
	if c == nil {
		return start.Add(time.Duration(mins) * time.Minute)
	}
	remain := time.Duration(mins) * time.Minute
	cur := start.In(c.loc)
	day := startOfDay(cur)
	for i := 0; i < maxCalendarDays; i++ {
		for _, w := range c.windows(day) {
			s, e := w[0], w[1]
			if e.Before(cur) || e.Equal(cur) {
				continue
			}
			if s.Before(cur) {
				s = cur
			}
			if avail := e.Sub(s); avail >= remain {
				return s.Add(remain)
			} else {
				remain -= avail
			}
		}
		day = startOfDay(day.AddDate(0, 0, 1).Add(2 * time.Hour))
	}
	return start.Add(time.Duration(mins) * time.Minute) // calendar has no hours: fall back to wall clock
}

// SLATimer describes one running SLA target against a calendar.
type SLATimer struct {
	Created   time.Time
	TargetMin int
	Calendar  *Calendar
	PausedMin int        // working minutes already spent paused
	PausedAt  *time.Time // set while the clock is paused
	DoneAt    *time.Time
}

// SLAState evaluates a timer. While paused the clock does not advance; elapsed time
// excludes everything before the pause that was spent paused.
func SLAState(t SLATimer, now time.Time) (clock SLAClock, usedMin, leftMin int) {
	if t.TargetMin <= 0 {
		return SLAClock{}, 0, 0
	}
	end := now
	if t.DoneAt != nil {
		end = *t.DoneAt
	} else if t.PausedAt != nil {
		end = *t.PausedAt
	}
	used := t.Calendar.BusinessMinutes(t.Created, end) - t.PausedMin
	if used < 0 {
		used = 0
	}
	left := t.TargetMin - used
	due := t.Calendar.AddBusinessMinutes(t.Created, t.TargetMin+t.PausedMin)
	c := SLAClock{Due: &due}
	switch {
	case t.DoneAt != nil && left >= 0:
		c.State = SLAMet
	case t.DoneAt != nil, left < 0:
		c.State = SLABreached
	case left*5 < t.TargetMin:
		c.State = SLAAtRisk
	default:
		c.State = SLAOK
	}
	return c, used, left
}

// PauseMinutesOnResume returns the working minutes to add to PausedMin when a ticket
// leaves a pausing status.
func PauseMinutesOnResume(cal *Calendar, pausedAt, now time.Time) int {
	return cal.BusinessMinutes(pausedAt, now)
}
