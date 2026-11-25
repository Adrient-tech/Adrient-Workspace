// Package shifts is the Adrient Shifts module: a weekly rota for the people in
// HR, with availability, coverage rules, swap requests, attendance and an
// hours export to HR payroll. logic.go and plan.go are the pure scheduling
// logic; they never touch the database.
package shifts

import (
	"fmt"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

// Shift is one rota entry. EmployeeID is empty for an open shift.
type Shift struct {
	ID           string `json:"id"`
	Date         string `json:"date"`
	Start        string `json:"start"`
	End          string `json:"end"`
	BreakMinutes int    `json:"break_minutes"`
	Role         string `json:"role"`
	Location     string `json:"location"`
	Notes        string `json:"notes"`
	EmployeeID   string `json:"employee_id"`
	Published    bool   `json:"published"`
}

// DayWindow is one weekday of a weekly availability pattern.
type DayWindow struct {
	Off   bool   `json:"off"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// TimeOff is an inclusive date range an employee cannot work.
type TimeOff struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Note   string `json:"note"`
	Source string `json:"source,omitempty"` // "" (set here) or "leave" (approved in HR)
}

// Availability is a weekly pattern (index 0 = Sunday) plus time off.
type Availability struct {
	Days       []DayWindow `json:"days"`
	Exceptions []TimeOff   `json:"exceptions"`
}

// Rules are the optional working-time limits; zero means off.
type Rules struct {
	MinRestHours   float64 `json:"min_rest_hours"`
	MaxDailyHours  float64 `json:"max_daily_hours"`
	MaxConsecutive int     `json:"max_consecutive_days"`
}

// Staff is the scheduling profile of one HR employee.
type Staff struct {
	EmployeeID   string       `json:"employee_id"`
	Name         string       `json:"name"`
	EmpCode      string       `json:"emp_code"`
	Department   string       `json:"department"`
	Role         string       `json:"role"`
	Color        string       `json:"color"`
	HourlyRate   *float64     `json:"hourly_rate"`
	TargetHours  *float64     `json:"target_hours"`
	MaxHours     *float64     `json:"max_hours"`
	Availability Availability `json:"availability"`
	Active       bool         `json:"active"`
}

// Coverage is a minimum headcount for a role on some weekdays.
type Coverage struct {
	ID    string `json:"id"`
	Role  string `json:"role"`
	Days  []int  `json:"days"`
	Count int    `json:"count"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// Attendance is the clock record of one shift.
type Attendance struct {
	ShiftID  string `json:"shift_id"`
	Status   string `json:"status"` // present | late | absent
	ClockIn  string `json:"clock_in"`
	ClockOut string `json:"clock_out"`
	Note     string `json:"note"`
}

func toMinutes(hhmm string) (int, bool) {
	if !opshttp.ValidTime(hhmm) {
		return 0, false
	}
	return int(hhmm[0]-'0')*600 + int(hhmm[1]-'0')*60 + int(hhmm[3]-'0')*10 + int(hhmm[4]-'0'), true
}

// DurationMinutes is the wall-clock length of start..end, wrapping past
// midnight. ok is false when either time is invalid or both are equal.
func DurationMinutes(start, end string) (int, bool) {
	s, ok1 := toMinutes(start)
	e, ok2 := toMinutes(end)
	if !ok1 || !ok2 || s == e {
		return 0, false
	}
	if e > s {
		return e - s, true
	}
	return e + 1440 - s, true
}

func isOvernight(start, end string) bool {
	s, ok1 := toMinutes(start)
	e, ok2 := toMinutes(end)
	return ok1 && ok2 && e < s
}

func dayNumber(iso string) int {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return 0
	}
	return int(t.Unix() / 86400)
}

func weekday(iso string) int {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return 0
	}
	return int(t.Weekday())
}

// StartOfWeek returns the Monday on or before iso.
func StartOfWeek(iso string) string {
	d := weekday(iso)
	diff := 1 - d
	if d == 0 {
		diff = -6
	}
	return opshttp.AddDays(iso, diff)
}

func absoluteRange(s Shift) (int, int, bool) {
	st, ok := toMinutes(s.Start)
	dur, ok2 := DurationMinutes(s.Start, s.End)
	if !ok || !ok2 || !opshttp.ValidDate(s.Date) {
		return 0, 0, false
	}
	base := dayNumber(s.Date)*1440 + st
	return base, base + dur, true
}

// NetMinutes is the paid length of a shift: duration minus the unpaid break.
func NetMinutes(s Shift) int {
	d, _ := DurationMinutes(s.Start, s.End)
	n := d - max(0, s.BreakMinutes)
	if n < 0 {
		return 0
	}
	return n
}

// ValidateShift lists the problems of a shift; empty means valid.
func ValidateShift(s Shift) []string {
	var errs []string
	if !opshttp.ValidDate(s.Date) {
		errs = append(errs, "A valid date is required.")
	}
	if !opshttp.ValidTime(s.Start) {
		errs = append(errs, "A valid start time is required.")
	}
	if !opshttp.ValidTime(s.End) {
		errs = append(errs, "A valid end time is required.")
	}
	if len(errs) == 0 {
		if _, ok := DurationMinutes(s.Start, s.End); !ok {
			errs = append(errs, "End time must differ from start time.")
		}
	}
	if strings.TrimSpace(s.Role) == "" {
		errs = append(errs, "A role or label is required.")
	}
	if s.BreakMinutes < 0 {
		errs = append(errs, "The unpaid break must be zero or more minutes.")
	} else if len(errs) == 0 {
		if d, _ := DurationMinutes(s.Start, s.End); s.BreakMinutes >= d {
			errs = append(errs, "The unpaid break must be shorter than the shift itself.")
		}
	}
	return errs
}

// FindConflicts returns the shifts of the same employee that overlap s.
// Open shifts never conflict.
func FindConflicts(s Shift, all []Shift, employeeID string) []Shift {
	if employeeID == "" {
		return nil
	}
	a0, a1, ok := absoluteRange(s)
	if !ok {
		return nil
	}
	before, after := opshttp.AddDays(s.Date, -1), opshttp.AddDays(s.Date, 1)
	var out []Shift
	for _, o := range all {
		if o.ID == s.ID || o.EmployeeID != employeeID {
			continue
		}
		if o.Date != s.Date && o.Date != before && o.Date != after {
			continue
		}
		b0, b1, ok := absoluteRange(o)
		if ok && b0 < a1 && a0 < b1 {
			out = append(out, o)
		}
	}
	return out
}

// DefaultAvailability is 09:00-17:00 on every day.
func DefaultAvailability() Availability {
	d := make([]DayWindow, 7)
	for i := range d {
		d[i] = DayWindow{Start: "09:00", End: "17:00"}
	}
	return Availability{Days: d, Exceptions: []TimeOff{}}
}

// TimeOffOn returns the time-off entry covering iso, if any.
func TimeOffOn(a Availability, iso string) *TimeOff {
	for i := range a.Exceptions {
		if a.Exceptions[i].From <= iso && iso <= a.Exceptions[i].To {
			return &a.Exceptions[i]
		}
	}
	return nil
}

// Availability statuses.
const (
	AvailAvailable   = "available"
	AvailPartial     = "partial"
	AvailUnavailable = "unavailable"
)

func segmentCoverage(day *DayWindow, s, e int) string {
	if e <= s || day == nil {
		return "full"
	}
	if day.Off {
		return "none"
	}
	ws, ok1 := toMinutes(day.Start)
	we, ok2 := toMinutes(day.End)
	if !ok1 || !ok2 {
		return "full"
	}
	if we == 1439 {
		we = 1440
	}
	if s >= ws && e <= we {
		return "full"
	}
	if !(s < we && ws < e) {
		return "none"
	}
	return "partial"
}

// AvailabilityStatus tells how a shift fits an employee's weekly pattern and
// time off. Overnight shifts are checked in two segments.
func AvailabilityStatus(s Shift, a Availability) string {
	if len(a.Days) != 7 && len(a.Exceptions) == 0 {
		return AvailAvailable
	}
	st, ok1 := toMinutes(s.Start)
	en, ok2 := toMinutes(s.End)
	if !ok1 || !ok2 {
		return AvailAvailable
	}
	dow := weekday(s.Date)
	type seg struct {
		date      string
		dow, a, b int
	}
	segs := []seg{{s.Date, dow, st, en}}
	if en <= st {
		segs = []seg{{s.Date, dow, st, 1440}, {opshttp.AddDays(s.Date, 1), (dow + 1) % 7, 0, en}}
	}
	var res []string
	for _, g := range segs {
		if g.b <= g.a {
			continue
		}
		if TimeOffOn(a, g.date) != nil {
			res = append(res, "none")
			continue
		}
		var w *DayWindow
		if len(a.Days) == 7 {
			w = &a.Days[g.dow]
		}
		res = append(res, segmentCoverage(w, g.a, g.b))
	}
	all := func(v string) bool {
		for _, r := range res {
			if r != v {
				return false
			}
		}
		return true
	}
	switch {
	case all("full"):
		return AvailAvailable
	case all("none"):
		return AvailUnavailable
	}
	return AvailPartial
}

// WeeklyMinutes is the paid minutes of an employee in the week starting weekStart.
func WeeklyMinutes(shifts []Shift, employeeID, weekStart string) int {
	end := opshttp.AddDays(weekStart, 6)
	n := 0
	for _, s := range shifts {
		if s.EmployeeID == employeeID && s.Date >= weekStart && s.Date <= end {
			n += NetMinutes(s)
		}
	}
	return n
}

func formatHours(min int) string {
	h, m := min/60, min%60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// Violation is one broken working-time rule.
type Violation struct {
	Rule    string `json:"rule"`
	Short   string `json:"short"`
	Message string `json:"message"`
}

// RuleViolations checks the optional limits for giving s to employeeID.
func RuleViolations(s Shift, employeeID string, shifts []Shift, r Rules, name string) []Violation {
	if employeeID == "" || (r.MinRestHours <= 0 && r.MaxDailyHours <= 0 && r.MaxConsecutive <= 0) {
		return nil
	}
	a0, a1, ok := absoluteRange(s)
	if !ok {
		return nil
	}
	var others []Shift
	for _, o := range shifts {
		if o.EmployeeID == employeeID && o.ID != s.ID {
			others = append(others, o)
		}
	}
	var out []Violation
	if r.MinRestHours > 0 {
		lo, hi := opshttp.AddDays(s.Date, -2), opshttp.AddDays(s.Date, 2)
		worst, worstGap := Shift{}, -1
		for _, o := range others {
			if o.Date < lo || o.Date > hi {
				continue
			}
			b0, b1, ok := absoluteRange(o)
			if !ok || (b0 < a1 && a0 < b1) {
				continue
			}
			gap := a0 - b1
			if b0 >= a1 {
				gap = b0 - a1
			}
			if float64(gap) < r.MinRestHours*60 && (worstGap < 0 || gap < worstGap) {
				worst, worstGap = o, gap
			}
		}
		if worstGap >= 0 {
			out = append(out, Violation{Rule: "rest", Short: "Short rest", Message: fmt.Sprintf("%s would get %s rest around their %s shift, under the %gh minimum.", name, formatHours(worstGap), worst.Date, r.MinRestHours)})
		}
	}
	if r.MaxDailyHours > 0 {
		total := NetMinutes(s)
		for _, o := range others {
			if o.Date == s.Date {
				total += NetMinutes(o)
			}
		}
		if float64(total) > r.MaxDailyHours*60 {
			out = append(out, Violation{Rule: "daily", Short: "Long day", Message: fmt.Sprintf("%s would work %s on %s, over the %gh daily limit.", name, formatHours(total), s.Date, r.MaxDailyHours)})
		}
	}
	if r.MaxConsecutive > 0 {
		worked := map[string]bool{s.Date: true}
		for _, o := range others {
			worked[o.Date] = true
		}
		run := 1
		for d, i := opshttp.AddDays(s.Date, -1), 0; worked[d] && i < 400; d, i = opshttp.AddDays(d, -1), i+1 {
			run++
		}
		for d, i := opshttp.AddDays(s.Date, 1), 0; worked[d] && i < 400; d, i = opshttp.AddDays(d, 1), i+1 {
			run++
		}
		if run > r.MaxConsecutive {
			out = append(out, Violation{Rule: "consecutive", Short: fmt.Sprintf("%d days in a row", run), Message: fmt.Sprintf("%s would be working %d days in a row, over the %d-day limit.", name, run, r.MaxConsecutive)})
		}
	}
	return out
}

// CheckAssign returns a blocking message when the person already has an
// overlapping shift (or is inactive), otherwise every soft warning.
func CheckAssign(s Shift, st Staff, shifts []Shift, rules Rules) (blocked string, warnings []string) {
	if len(FindConflicts(s, shifts, st.EmployeeID)) > 0 {
		return st.Name + " already has an overlapping shift then.", nil
	}
	if !st.Active {
		return st.Name + " is not active.", nil
	}
	return "", assignWarnings(s, st, shifts, rules)
}

func assignWarnings(s Shift, st Staff, shifts []Shift, rules Rules) []string {
	var warnings []string
	if t := TimeOffOn(st.Availability, s.Date); t != nil {
		w := st.Name + " has time off then"
		if t.Note != "" {
			w += " (" + t.Note + ")"
		}
		warnings = append(warnings, w+".")
	} else {
		switch AvailabilityStatus(s, st.Availability) {
		case AvailUnavailable:
			warnings = append(warnings, st.Name+" is marked unavailable at this time.")
		case AvailPartial:
			warnings = append(warnings, "This only partially fits "+st.Name+"'s availability.")
		}
	}
	var others []Shift
	for _, o := range shifts {
		if o.ID != s.ID {
			others = append(others, o)
		}
	}
	if st.MaxHours != nil {
		proj := WeeklyMinutes(others, st.EmployeeID, StartOfWeek(s.Date)) + NetMinutes(s)
		if float64(proj) > *st.MaxHours*60 {
			warnings = append(warnings, fmt.Sprintf("This puts %s at %s that week, over their %gh max.", st.Name, formatHours(proj), *st.MaxHours))
		}
	}
	for _, v := range RuleViolations(s, st.EmployeeID, others, rules, st.Name) {
		warnings = append(warnings, v.Message)
	}
	return warnings
}
