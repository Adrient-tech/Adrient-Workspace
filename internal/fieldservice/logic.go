// Package fieldservice is the Adrient Field service module: requests, quotes,
// jobs with recurring visits, scheduling into Calendar and invoicing through
// Invoicing, for service businesses. Customers are Contacts. logic.go is pure.
package fieldservice

import (
	"math"
	"time"
)

// Schedule is when a job's visits happen.
type Schedule struct {
	Start     string `json:"start"`      // first visit date
	Repeat    string `json:"repeat"`     // none | daily | weekly | monthly | yearly
	Interval  int    `json:"interval"`   // every N units, default 1
	Weekdays  []int  `json:"weekdays"`   // 0 = Sunday, for weekly
	MonthlyBy string `json:"monthly_by"` // "date" (default) or "weekday" (the 2nd Tuesday)
	EndType   string `json:"end_type"`   // count | until
	EndCount  int    `json:"end_count"`
	EndDate   string `json:"end_date"`
	StartTime string `json:"start_time"` // HH:MM, empty = anytime
	EndTime   string `json:"end_time"`
}

// MaxVisits caps how many visits one schedule can create.
const MaxVisits = 400

func parse(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

func fmtDate(t time.Time) string { return t.Format("2006-01-02") }

// addMonths adds n months and clamps the day to the target month's length
// (Jan 31 + 1 month = Feb 28/29).
func addMonths(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, time.UTC)
}

// VisitDates expands a schedule into visit dates (at most MaxVisits).
func VisitDates(s Schedule) []string {
	start, ok := parse(s.Start)
	if !ok {
		return nil
	}
	if s.Repeat == "" || s.Repeat == "none" {
		return []string{s.Start}
	}
	interval := max(1, s.Interval)
	limit := MaxVisits
	until := ""
	if s.EndType == "until" {
		until = s.EndDate
	} else {
		c := s.EndCount
		if c <= 0 {
			c = 12
		}
		limit = min(MaxVisits, c)
	}
	var out []string
	push := func(t time.Time) bool {
		d := fmtDate(t)
		if until != "" && d > until {
			return false
		}
		if d < s.Start {
			return true
		}
		out = append(out, d)
		return len(out) < limit
	}
	switch s.Repeat {
	case "daily":
		for t := start; push(t); t = t.AddDate(0, 0, interval) {
		}
	case "weekly":
		days := append([]int(nil), s.Weekdays...)
		if len(days) == 0 {
			days = []int{int(start.Weekday())}
		}
		for i := 0; i < len(days); i++ {
			for j := i + 1; j < len(days); j++ {
				if days[j] < days[i] {
					days[i], days[j] = days[j], days[i]
				}
			}
		}
		week := start.AddDate(0, 0, -int(start.Weekday()))
		for guard := 0; guard < 5000; guard++ {
			for _, wd := range days {
				if wd < 0 || wd > 6 {
					continue
				}
				if !push(week.AddDate(0, 0, wd)) {
					return out
				}
			}
			week = week.AddDate(0, 0, 7*interval)
		}
	case "monthly":
		nth := (start.Day() - 1) / 7
		wd := int(start.Weekday())
		for i := 0; i < 5000; i++ {
			var t time.Time
			if s.MonthlyBy == "weekday" {
				first := addMonths(time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC), i*interval)
				offset := (wd - int(first.Weekday()) + 7) % 7
				day := 1 + offset + nth*7
				last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
				if day > last {
					day -= 7
				}
				t = time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC)
			} else {
				t = addMonths(start, i*interval)
			}
			if !push(t) {
				return out
			}
		}
	case "yearly":
		for i := 0; i < MaxVisits; i++ {
			if !push(addMonths(start, 12*i*interval)) {
				return out
			}
		}
	default:
		return []string{s.Start}
	}
	return out
}

// Item is one quote or job line.
type Item struct {
	Name      string  `json:"name"`
	Desc      string  `json:"description"`
	Qty       float64 `json:"qty"`
	UnitPrice float64 `json:"unit_price"`
	UnitCost  float64 `json:"unit_cost"`
	Taxable   bool    `json:"taxable"`
	Optional  bool    `json:"optional"`
	Selected  bool    `json:"selected"`
}

// Discount is a percentage or fixed amount.
type Discount struct {
	Kind  string  `json:"kind"` // pct | amount
	Value float64 `json:"value"`
}

// Totals are the money figures of a set of items.
type Totals struct {
	Subtotal float64 `json:"subtotal"`
	Discount float64 `json:"discount"`
	Tax      float64 `json:"tax"`
	Total    float64 `json:"total"`
	Cost     float64 `json:"cost"`
	Profit   float64 `json:"profit"`
}

func r2(f float64) float64 { return math.Round(f*100) / 100 }

// Included reports whether an item counts (optional add-ons only when selected).
func (i Item) Included() bool { return !i.Optional || i.Selected }

// Compute totals items: subtotal, a percentage or fixed discount spread over
// the taxable part, tax on what is left, total, estimated cost and profit.
func Compute(items []Item, d *Discount, taxRate float64) Totals {
	var sub, taxable, cost float64
	for _, it := range items {
		if !it.Included() {
			continue
		}
		line := r2(it.Qty * it.UnitPrice)
		sub += line
		if it.Taxable {
			taxable += line
		}
		cost += r2(it.Qty * it.UnitCost)
	}
	disc := 0.0
	if d != nil && d.Value > 0 {
		if d.Kind == "pct" {
			disc = r2(sub * math.Min(100, d.Value) / 100)
		} else {
			disc = math.Round(d.Value*100) / 100
		}
		disc = math.Max(0, math.Min(sub, disc))
	}
	taxAfter := 0.0
	if sub > 0 {
		taxAfter = taxable - r2(disc*taxable/sub)
	}
	tax := 0.0
	if taxRate > 0 {
		tax = r2(taxAfter * taxRate / 100)
	}
	return Totals{Subtotal: r2(sub), Discount: disc, Tax: tax, Total: r2(sub - disc + tax), Cost: r2(cost), Profit: r2(sub - disc - cost)}
}

// Lifecycle states.
const (
	ReqNew        = "new"
	ReqAssessment = "assessment"
	ReqQuoted     = "quoted"
	ReqConverted  = "converted"
	ReqClosed     = "closed"
)

var reqNext = map[string][]string{
	ReqNew:        {ReqAssessment, ReqQuoted, ReqConverted, ReqClosed},
	ReqAssessment: {ReqQuoted, ReqConverted, ReqClosed},
	ReqQuoted:     {ReqConverted, ReqClosed, ReqAssessment},
	ReqConverted:  {},
	ReqClosed:     {ReqNew},
}

// CanMoveRequest reports whether a request may go from one status to another.
func CanMoveRequest(from, to string) bool {
	if from == to {
		return true
	}
	for _, n := range reqNext[from] {
		if n == to {
			return true
		}
	}
	return false
}

// JobStatus derives a job's display status from its visits: "late" when an open
// visit is before today, "requires_invoicing" when all visits are done but it is
// not invoiced, "completed" once invoiced, "cancelled" or "active" otherwise.
func JobStatus(stored string, visits []VisitState, invoiced bool, today string) string {
	if stored == "cancelled" {
		return "cancelled"
	}
	open, late := 0, false
	for _, v := range visits {
		if v.Status == "scheduled" {
			open++
			if v.Date != "" && v.Date < today {
				late = true
			}
		}
	}
	switch {
	case open == 0 && len(visits) > 0 && invoiced:
		return "completed"
	case open == 0 && len(visits) > 0:
		return "requires_invoicing"
	case late:
		return "late"
	}
	return "active"
}

// VisitState is the part of a visit JobStatus needs.
type VisitState struct {
	Date   string
	Status string
}
