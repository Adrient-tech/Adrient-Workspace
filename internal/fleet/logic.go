// Package fleet is the Adrient Fleet module: vehicles, meter readings, fuel,
// service history, service reminders and renewals (with Calendar dates),
// inspections that raise issues, and costs that post to Accounting. logic.go
// is pure.
package fleet

import (
	"regexp"
	"strings"
	"time"
)

// ---- VIN ----

var vinRe = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)

var vinValues = map[rune]int{
	'A': 1, 'B': 2, 'C': 3, 'D': 4, 'E': 5, 'F': 6, 'G': 7, 'H': 8, 'J': 1, 'K': 2, 'L': 3, 'M': 4, 'N': 5, 'P': 7, 'R': 9,
	'S': 2, 'T': 3, 'U': 4, 'V': 5, 'W': 6, 'X': 7, 'Y': 8, 'Z': 9,
}

// CleanVIN upper-cases and strips spaces and dashes.
func CleanVIN(s string) string {
	return strings.NewReplacer(" ", "", "-", "").Replace(strings.ToUpper(strings.TrimSpace(s)))
}

// ValidVIN checks the shape (17 characters, no I, O or Q). checkOK is the
// North American check-digit test (position 9); other markets do not use it,
// so a mismatch is a warning, not an error.
func ValidVIN(vin string) (shapeOK, checkOK bool) {
	if !vinRe.MatchString(vin) {
		return false, false
	}
	weights := []int{8, 7, 6, 5, 4, 3, 2, 10, 0, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i, c := range vin {
		v, ok := vinValues[c]
		if !ok {
			v = int(c - '0')
		}
		sum += v * weights[i]
	}
	want := byte('0' + sum%11)
	if sum%11 == 10 {
		want = 'X'
	}
	return true, vin[8] == want
}

// ModelYear reads the model year from position 10 of a VIN (1980-2039); 0 when unknown.
func ModelYear(vin string) int {
	if len(vin) != 17 {
		return 0
	}
	const codes = "ABCDEFGHJKLMNPRSTVWXY123456789"
	i := strings.IndexByte(codes, vin[9])
	if i < 0 {
		return 0
	}
	// Position 7 a letter means 2010-2039, a digit means 1980-2009.
	if vin[6] >= 'A' && vin[6] <= 'Z' {
		return 2010 + i
	}
	return 1980 + i
}

// ---- dates and intervals ----

// Interval repeats by time.
type Interval struct {
	Every int    `json:"every"`
	Unit  string `json:"unit"` // days | weeks | months | years
}

func parse(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

// AddInterval adds n intervals to a date (months clamp to the month end).
func AddInterval(s string, iv Interval, n int) string {
	t, ok := parse(s)
	if !ok || iv.Every <= 0 {
		return ""
	}
	k := iv.Every * n
	switch iv.Unit {
	case "days":
		t = t.AddDate(0, 0, k)
	case "weeks":
		t = t.AddDate(0, 0, 7*k)
	case "years":
		t = clampMonths(t, 12*k)
	default:
		t = clampMonths(t, k)
	}
	return t.Format("2006-01-02")
}

func clampMonths(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	return time.Date(first.Year(), first.Month(), min(d, last), 0, 0, 0, 0, time.UTC)
}

// DiffDays is the number of days from a to b (b - a).
func DiffDays(a, b string) int {
	x, ok1 := parse(a)
	y, ok2 := parse(b)
	if !ok1 || !ok2 {
		return 0
	}
	return int(y.Sub(x).Hours() / 24)
}

// ---- reminders ----

// HistoryItem is one completion of a reminder.
type HistoryItem struct {
	Date   string  `json:"date"`
	Meter  float64 `json:"meter"`
	OnTime bool    `json:"on_time"`
}

// Reminder repeats by time, by meter, or both (whichever comes first).
type Reminder struct {
	ID            string        `json:"id"`
	VehicleID     string        `json:"vehicle_id"`
	Title         string        `json:"title"`
	Interval      Interval      `json:"interval"`
	MeterInterval float64       `json:"meter_interval"`
	StartDate     string        `json:"start_date"`
	StartMeter    float64       `json:"start_meter"`
	LastDate      string        `json:"last_date"`
	LastMeter     float64       `json:"last_meter"`
	DueSoonDays   int           `json:"due_soon_days"`
	DueSoonMeter  float64       `json:"due_soon_meter"`
	SnoozedUntil  string        `json:"snoozed_until"`
	History       []HistoryItem `json:"history"`
	EventID       string        `json:"event_id"`
	// Derived, never stored.
	Status    string   `json:"status,omitempty"`
	NextDate  string   `json:"next_date,omitempty"`
	NextMeter *float64 `json:"next_meter,omitempty"`
	DaysLeft  *int     `json:"days_left,omitempty"`
	MeterLeft *float64 `json:"meter_left,omitempty"`
}

// State is a reminder's status against today and the vehicle's meter.
type State struct {
	Status    string // upcoming | due_soon | overdue | snoozed
	NextDate  string
	NextMeter *float64
	DaysLeft  *int
	MeterLeft *float64
}

// NextDue computes the next due date and meter from the last completion (or the start).
func (r Reminder) NextDue() (string, *float64) {
	from := r.LastDate
	if from == "" {
		from = r.StartDate
	}
	next := ""
	if r.Interval.Every > 0 && from != "" {
		next = AddInterval(from, r.Interval, 1)
	}
	var nm *float64
	if r.MeterInterval > 0 {
		base := r.LastMeter
		if r.LastDate == "" {
			base = r.StartMeter
		}
		v := base + r.MeterInterval
		nm = &v
	}
	return next, nm
}

// StateAt evaluates a reminder. soonDays and soonMeter are the defaults when
// the reminder has no thresholds of its own.
func (r Reminder) StateAt(meter float64, today string, soonDays int, soonMeter float64) State {
	st := State{Status: "upcoming"}
	st.NextDate, st.NextMeter = r.NextDue()
	if st.NextDate != "" {
		d := DiffDays(today, st.NextDate)
		st.DaysLeft = &d
	}
	if st.NextMeter != nil {
		m := *st.NextMeter - meter
		st.MeterLeft = &m
	}
	sd, sm := soonDays, soonMeter
	if r.DueSoonDays > 0 {
		sd = r.DueSoonDays
	}
	if r.DueSoonMeter > 0 {
		sm = r.DueSoonMeter
	}
	switch {
	case (st.DaysLeft != nil && *st.DaysLeft < 0) || (st.MeterLeft != nil && *st.MeterLeft <= 0):
		st.Status = "overdue"
	case (st.DaysLeft != nil && *st.DaysLeft <= sd) || (st.MeterLeft != nil && *st.MeterLeft <= sm):
		st.Status = "due_soon"
	}
	if r.SnoozedUntil != "" && r.SnoozedUntil >= today && st.Status != "upcoming" {
		st.Status = "snoozed"
	}
	return st
}

// Complete records a completion on date at meter: the next due date and meter
// are recomputed from it, and the history notes whether it was on time.
func (r Reminder) Complete(date string, meter float64, today string, soonDays int, soonMeter float64) Reminder {
	before := r.StateAt(meter, date, soonDays, soonMeter)
	r.History = append(r.History, HistoryItem{Date: date, Meter: meter, OnTime: before.Status != "overdue"})
	if len(r.History) > 50 {
		r.History = r.History[len(r.History)-50:]
	}
	r.LastDate, r.LastMeter, r.SnoozedUntil = date, meter, ""
	return r
}

// Compliance is the share of completions that were on time (-1 when none).
func (r Reminder) Compliance() float64 {
	if len(r.History) == 0 {
		return -1
	}
	n := 0
	for _, h := range r.History {
		if h.OnTime {
			n++
		}
	}
	return float64(n) / float64(len(r.History))
}

// ---- renewals ----

// Renewal is a dated obligation (registration, insurance, emission test, licence).
type Renewal struct {
	ID           string         `json:"id"`
	VehicleID    string         `json:"vehicle_id"`
	Kind         string         `json:"kind"`
	Title        string         `json:"title"`
	DueDate      string         `json:"due_date"`
	Repeat       Interval       `json:"repeat"`
	Cost         float64        `json:"cost"`
	DueSoonDays  int            `json:"due_soon_days"`
	SnoozedUntil string         `json:"snoozed_until"`
	Done         bool           `json:"done"`
	History      []RenewalEntry `json:"history"`
	EventID      string         `json:"event_id"`
	Status       string         `json:"status,omitempty"`
	DaysLeft     *int           `json:"days_left,omitempty"`
}

// RenewalEntry is one completed renewal.
type RenewalEntry struct {
	Date    string  `json:"date"`
	DueDate string  `json:"due_date"`
	Cost    float64 `json:"cost"`
}

// StateAt gives the renewal's status.
func (r Renewal) StateAt(today string, soon int) (string, *int) {
	if r.Done || r.DueDate == "" {
		return "upcoming", nil
	}
	d := DiffDays(today, r.DueDate)
	if r.DueSoonDays > 0 {
		soon = r.DueSoonDays
	}
	st := "upcoming"
	switch {
	case d < 0:
		st = "overdue"
	case d <= soon:
		st = "due_soon"
	}
	if r.SnoozedUntil != "" && r.SnoozedUntil >= today && st != "upcoming" {
		st = "snoozed"
	}
	return st, &d
}

// Complete records the renewal and, when it repeats, rolls the due date forward
// from the old due date (so paying late does not shift the cycle).
func (r Renewal) Complete(date string, cost float64, nextDue string) Renewal {
	r.History = append(r.History, RenewalEntry{Date: date, DueDate: r.DueDate, Cost: cost})
	if len(r.History) > 50 {
		r.History = r.History[len(r.History)-50:]
	}
	rolled := nextDue
	if rolled == "" && r.Repeat.Every > 0 && r.DueDate != "" {
		rolled = AddInterval(r.DueDate, r.Repeat, 1)
	}
	if rolled != "" {
		r.DueDate, r.Done = rolled, false
	} else {
		r.Done = true
	}
	r.SnoozedUntil = ""
	return r
}

// ---- costs ----

// CostPerDistance is spend divided by distance driven (0 when there is no distance).
func CostPerDistance(spend, distance float64) float64 {
	if distance <= 0 {
		return 0
	}
	return float64(int64(spend/distance*1000+0.5)) / 1000
}

// FuelEconomy is distance per unit of fuel between two full-tank fill-ups
// (0 when it cannot be computed).
func FuelEconomy(prevMeter, meter, litres float64) float64 {
	if litres <= 0 || meter <= prevMeter {
		return 0
	}
	return float64(int64((meter-prevMeter)/litres*100+0.5)) / 100
}
