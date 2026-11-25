package tasks

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Rule is a parsed repeat rule. Stored form (Extras.Recurrence) is either one
// of the simple names (daily, weekdays, weekly, monthly, yearly) or
//
//	every:<interval>:<daily|weekly|monthly|yearly>[:<weekday,weekday>][:done]
//
// where weekdays are 0 (Sunday) to 6 and "done" rolls from the completion day
// instead of the due date.
type Rule struct {
	Freq     string
	Interval int
	Weekdays []int
	FromDone bool
}

// ParseRule parses a stored repeat rule. ok is false for "" and bad input.
func ParseRule(s string) (Rule, bool) {
	switch s {
	case "":
		return Rule{}, false
	case "daily", "weekly", "monthly", "yearly":
		return Rule{Freq: s, Interval: 1}, true
	case "weekdays":
		return Rule{Freq: "weekly", Interval: 1, Weekdays: []int{1, 2, 3, 4, 5}}, true
	}
	parts := strings.Split(s, ":")
	if len(parts) < 3 || parts[0] != "every" {
		return Rule{}, false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 1 || n > 999 {
		return Rule{}, false
	}
	r := Rule{Interval: n}
	switch parts[2] {
	case "daily", "weekly", "monthly", "yearly":
		r.Freq = parts[2]
	default:
		return Rule{}, false
	}
	for _, p := range parts[3:] {
		if p == "done" {
			r.FromDone = true
			continue
		}
		if r.Freq != "weekly" || r.Weekdays != nil {
			return Rule{}, false
		}
		seen := map[int]bool{}
		for _, d := range strings.Split(p, ",") {
			v, err := strconv.Atoi(d)
			if err != nil || v < 0 || v > 6 {
				return Rule{}, false
			}
			if !seen[v] {
				seen[v] = true
				r.Weekdays = append(r.Weekdays, v)
			}
		}
		sort.Ints(r.Weekdays)
	}
	return r, true
}

func mondayOf(t time.Time) time.Time {
	return t.AddDate(0, 0, -((int(t.Weekday()) + 6) % 7))
}

func hasDay(days []int, d time.Weekday) bool {
	for _, x := range days {
		if x == int(d) {
			return true
		}
	}
	return false
}

// nextAfter is the single occurrence strictly after base.
func (r Rule) nextAfter(base time.Time) time.Time {
	switch r.Freq {
	case "daily":
		return base.AddDate(0, 0, r.Interval)
	case "monthly":
		return addMonthsClamped(base, r.Interval)
	case "yearly":
		return addMonthsClamped(base, 12*r.Interval)
	}
	if len(r.Weekdays) == 0 {
		return base.AddDate(0, 0, 7*r.Interval)
	}
	anchor := mondayOf(base)
	for i := 1; i <= 7*(r.Interval+1); i++ {
		d := base.AddDate(0, 0, i)
		if !hasDay(r.Weekdays, d.Weekday()) {
			continue
		}
		weeks := int(mondayOf(d).Sub(anchor).Round(time.Hour) / (7 * 24 * time.Hour))
		if weeks%r.Interval == 0 {
			return d
		}
	}
	return base.AddDate(0, 0, 7*r.Interval)
}

// Next returns the next due time after completing an occurrence due at due.
// With FromDone the roll starts at now. A result in the past (an overdue
// task) is advanced until it is not, so missed occurrences are skipped.
func (r Rule) Next(due, now time.Time) time.Time {
	base := due
	if r.FromDone {
		base = time.Date(now.Year(), now.Month(), now.Day(), due.Hour(), due.Minute(), due.Second(), 0, due.Location())
	}
	next := r.nextAfter(base)
	for i := 0; next.Before(now) && i < 5000; i++ {
		n := r.nextAfter(next)
		if !n.After(next) {
			break
		}
		next = n
	}
	return next
}
