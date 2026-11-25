package hr

import (
	"fmt"
	"time"
)

// Pure leave rules. No database access so they are easy to test.

// LeaveYearOf returns the label of the leave year containing d. A leave year
// that starts in April 2026 is labelled 2026 (it ends in March 2027).
func LeaveYearOf(d time.Time, startMonth int) int {
	if startMonth < 1 || startMonth > 12 {
		startMonth = 1
	}
	if int(d.Month()) >= startMonth {
		return d.Year()
	}
	return d.Year() - 1
}

// LeaveYearBounds returns the first and last day of leave year y.
func LeaveYearBounds(y, startMonth int) (time.Time, time.Time) {
	from := time.Date(y, time.Month(startMonth), 1, 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(1, 0, -1)
}

func monthKey(t time.Time) string { return fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month())) }

// firstAccrualMonth is the first month an employee accrues: the join month when
// they joined on or before the 15th, else the following month.
func firstAccrualMonth(join time.Time) time.Time {
	m := time.Date(join.Year(), join.Month(), 1, 0, 0, 0, 0, time.UTC)
	if join.Day() > 15 {
		m = m.AddDate(0, 1, 0)
	}
	return m
}

// AccrualPeriods lists the YYYY-MM months of leave year y an employee accrues
// for, up to and including the asOf month, from the join month, and stopping
// after the exit month.
func AccrualPeriods(y, startMonth int, join time.Time, exit *time.Time, asOf time.Time) []string {
	start, end := LeaveYearBounds(y, startMonth)
	first := firstAccrualMonth(join)
	if first.Before(start) {
		first = start
	}
	last := time.Date(asOf.Year(), asOf.Month(), 1, 0, 0, 0, 0, time.UTC)
	if e := end; last.After(time.Date(e.Year(), e.Month(), 1, 0, 0, 0, 0, time.UTC)) {
		last = time.Date(e.Year(), e.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	if exit != nil {
		em := time.Date(exit.Year(), exit.Month(), 1, 0, 0, 0, 0, time.UTC)
		if em.Before(last) {
			last = em
		}
	}
	out := []string{}
	for m := first; !m.After(last); m = m.AddDate(0, 1, 0) {
		out = append(out, monthKey(m))
	}
	return out
}

// MonthlyAccrual is the days credited per month for an annual quota.
func MonthlyAccrual(quota float64) float64 { return round2(quota / 12) }

// AnnualGrant is the upfront grant for leave year y: the full quota, prorated by
// the months remaining when the employee joins mid-year.
func AnnualGrant(quota float64, y, startMonth int, join time.Time) float64 {
	start, end := LeaveYearBounds(y, startMonth)
	first := firstAccrualMonth(join)
	if !first.After(start) {
		return round2(quota)
	}
	if first.After(end) {
		return 0
	}
	months := 0
	for m := first; !m.After(end); m = m.AddDate(0, 1, 0) {
		months++
	}
	return round2(quota * float64(months) / 12)
}

// CarryForward splits the closing balance into the part carried into the next
// year (up to cap) and the part that lapses. Negative balances carry nothing.
func CarryForward(balance, carryCap float64, enabled bool) (carry, lapse float64) {
	if balance <= 0 {
		return 0, 0
	}
	if !enabled || carryCap <= 0 {
		return 0, round2(balance)
	}
	if balance <= carryCap {
		return round2(balance), 0
	}
	return round2(carryCap), round2(balance - carryCap)
}

// CapAccrual limits an accrual so the running balance never passes max.
func CapAccrual(current, add float64, max *float64) float64 {
	if max == nil {
		return add
	}
	room := *max - current
	if room <= 0 {
		return 0
	}
	if add > room {
		return round2(room)
	}
	return add
}

// CountLeaveDays counts chargeable days between from and to inclusive,
// skipping weekly offs and holidays. A half day is only valid for one date.
func CountLeaveDays(from, to time.Time, half string, weeklyOff map[int]bool, holidays map[string]bool) (float64, error) {
	if to.Before(from) {
		return 0, invalid("the end date is before the start date")
	}
	if to.Sub(from) > 366*24*time.Hour {
		return 0, invalid("a leave request cannot span more than a year")
	}
	var n float64
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if weeklyOff[int(d.Weekday())] || holidays[fmtDate(d)] {
			continue
		}
		n++
	}
	if half != "" {
		if !from.Equal(to) {
			return 0, invalid("a half day applies to a single date")
		}
		if n == 0 {
			return 0, invalid("that date is a weekly off or holiday")
		}
		return 0.5, nil
	}
	if n == 0 {
		return 0, invalid("the selected dates are all weekly offs or holidays")
	}
	return n, nil
}

// leavesOverlap reports whether two requests collide. Two half-day requests on
// the same date for opposite halves do not.
func leavesOverlap(aFrom, aTo time.Time, aHalf string, bFrom, bTo time.Time, bHalf string) bool {
	if aTo.Before(bFrom) || bTo.Before(aFrom) {
		return false
	}
	if aHalf != "" && bHalf != "" && aFrom.Equal(aTo) && bFrom.Equal(bTo) && aFrom.Equal(bFrom) && aHalf != bHalf {
		return false
	}
	return true
}
