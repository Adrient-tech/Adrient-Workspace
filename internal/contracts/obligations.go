// (recurring obligations: one record with one live occurrence, completed occurrences kept as history).

package contracts

import "time"

// Repeat cadence in months.
var repeatMonthsOf = map[string]int{"none": 0, "monthly": 1, "quarterly": 3, "annually": 12}

// Repeat makes an obligation recur. Anchor is the first occurrence; occurrence n is Anchor plus n cadences,
// each computed from the anchor so a month-end schedule never drifts (Jan 31, Feb 28, Mar 31).
type Repeat struct {
	Every       string `json:"every"`
	Anchor      Date   `json:"anchor"`
	Count       *int   `json:"count,omitempty"`        // stop after this many occurrences
	Until       Date   `json:"until,omitempty"`        // stop after this date
	ContractEnd bool   `json:"contract_end,omitempty"` // stop when the agreement ends
}

// Done is one completed occurrence.
type Done struct {
	Due    Date   `json:"due"`
	DoneAt string `json:"done_at"`
}

// Obligation is a duty one side owes.
type Obligation struct {
	ID         string  `json:"id"`
	ContractID string  `json:"contract_id"`
	Title      string  `json:"title"`
	Owner      string  `json:"owner"` // us | them | mutual
	Due        Date    `json:"due,omitempty"`
	Done       bool    `json:"done"`
	Repeat     *Repeat `json:"repeat,omitempty"`
	History    []Done  `json:"history"`
	AssigneeID string  `json:"assignee_id,omitempty"`
	Notes      string  `json:"notes,omitempty"`
}

// Months is the cadence length.
func (r Repeat) Months() int { return repeatMonthsOf[r.Every] }

// OccurrenceDue is occurrence index (0-based).
func (r Repeat) OccurrenceDue(index int) Date { return AddMonths(r.Anchor, r.Months()*index) }

// WithinSeries reports whether occurrence index is still inside the series' end.
func (r Repeat) WithinSeries(index int, c *Contract, today Date) bool {
	if index < 0 || r.Months() == 0 {
		return false
	}
	if r.Count != nil && index >= *r.Count {
		return false
	}
	due := r.OccurrenceDue(index)
	if r.Until != "" && due > r.Until {
		return false
	}
	if r.ContractEnd && c != nil {
		if end := c.AgreementEnd(today); end != "" && due > end {
			return false
		}
	}
	return true
}

// RepeatAnchor is the anchor that puts due at occurrence number completed, keeping the existing anchor when it still fits.
func RepeatAnchor(due Date, every string, completed int, existing *Repeat) Date {
	m := repeatMonthsOf[every]
	if existing != nil && existing.Every == every && IsValidDate(existing.Anchor) && AddMonths(existing.Anchor, m*completed) == due {
		return existing.Anchor
	}
	if completed > 0 {
		return AddMonths(due, -m*completed)
	}
	return due
}

// Advance ticks off the live occurrence: record it and schedule the next. A non-repeating obligation just becomes done.
func (o Obligation) Advance(c *Contract, now time.Time, today Date) Obligation {
	h := append(append([]Done{}, o.History...), Done{Due: o.Due, DoneAt: now.UTC().Format(time.RFC3339)})
	if o.Repeat == nil {
		o.Done, o.History = true, h
		return o
	}
	next := len(h)
	if !o.Repeat.WithinSeries(next, c, today) {
		o.Done, o.History = true, h
		return o
	}
	o.Done, o.Due, o.History = false, o.Repeat.OccurrenceDue(next), h
	return o
}

// Rewind un-ticks the most recently completed occurrence.
func (o Obligation) Rewind() Obligation {
	if len(o.History) == 0 {
		o.Done = false
		return o
	}
	last := o.History[len(o.History)-1]
	o.History = append([]Done{}, o.History[:len(o.History)-1]...)
	o.Done = false
	if last.Due != "" {
		o.Due = last.Due
	}
	return o
}

// Reopened gives a finished series its next occurrence back when an edit made room for it.
func (o Obligation) Reopened(c *Contract, today Date) Obligation {
	if !o.Done || o.Repeat == nil {
		return o
	}
	n := len(o.History)
	if !o.Repeat.WithinSeries(n, c, today) {
		return o
	}
	o.Done, o.Due = false, o.Repeat.OccurrenceDue(n)
	return o
}

// Urgency of a due date.
const (
	UrgOverdue = "overdue"
	UrgSoon    = "soon" // within 7 days
	UrgLater   = "later"
	UrgNone    = "undated"
)

// Urgency classifies an obligation's due date.
func Urgency(due, today Date) string {
	if due == "" {
		return UrgNone
	}
	d := DaysBetween(today, due)
	switch {
	case d < 0:
		return UrgOverdue
	case d <= 7:
		return UrgSoon
	}
	return UrgLater
}
