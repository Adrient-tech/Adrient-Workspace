// (scheduleRows) and src/invoicing/src/lib/aging.js (agingParts, overdueAmount); rewritten in paise-exact Go.

package invoicing

import (
	"math"
	"sort"
	"time"
)

// Instalment is one row of a payment schedule with what has been applied to it. Nothing here is
// stored: payments fill the rows in date order, so adding, editing or deleting a payment always
// leaves the schedule telling the truth.
type Instalment struct {
	Label   string  `json:"label"`
	DueDate string  `json:"due_date"`
	Amount  float64 `json:"amount"`
	Paid    float64 `json:"paid"`
	Open    float64 `json:"open"`
}

func cents(v float64) int64     { return int64(math.Round(v * 100)) }
func fromCents(c int64) float64 { return float64(c) / 100 }

// Instalments allocates `applied` (money received plus credits and write-offs) over the schedule
// in due-date order. The rows always add up to total: each is clamped to what is left of the
// total and the last row takes the remainder, so a schedule asking for less than the total still
// bills all of it and one asking for more never bills past it. A row without a date goes after
// the dated ones, keeping its place among the others.
func Instalments(total, applied float64, sched []Schedule) []Instalment {
	if len(sched) == 0 {
		return nil
	}
	type item struct {
		s Schedule
		i int
	}
	items := make([]item, len(sched))
	for i, s := range sched {
		items[i] = item{s, i}
	}
	key := func(s Schedule) string {
		if s.DueDate == "" {
			return "\uffff"
		}
		return s.DueDate
	}
	sort.SliceStable(items, func(a, b int) bool {
		ka, kb := key(items[a].s), key(items[b].s)
		if ka != kb {
			return ka < kb
		}
		return items[a].i < items[b].i
	})
	tot := cents(total)
	used := int64(0)
	left := max(cents(applied), 0)
	out := make([]Instalment, len(items))
	for i, it := range items {
		var amt int64
		if i == len(items)-1 {
			amt = tot - used
		} else {
			amt = max(cents(it.s.Amount), 0)
			if rest := tot - used; amt > rest {
				amt = max(rest, 0)
			}
			used += amt
		}
		covered := min(max(amt, 0), left)
		left -= covered
		out[i] = Instalment{Label: it.s.Label, DueDate: it.s.DueDate, Amount: fromCents(amt), Paid: fromCents(covered), Open: fromCents(amt - covered)}
	}
	return out
}

// effectiveDue is the date an invoice is late against: the earliest open instalment, else the last
// instalment, else the invoice's own due date.
func effectiveDue(dueDate string, rows []Instalment) string {
	if len(rows) > 0 {
		for _, r := range rows {
			if r.Open > 0.004 {
				if r.DueDate != "" {
					return r.DueDate
				}
				break
			}
		}
		if last := rows[len(rows)-1].DueDate; last != "" {
			return last
		}
	}
	return dueDate
}

// orDue is the date a document is late against: its effective due date, which is the plain due
// date when there is no schedule. Documents built by hand (tests) may not have been derived.
func orDue(d Document) string {
	if d.EffectiveDue != "" {
		return d.EffectiveDue
	}
	return d.DueDate
}

// AgePart is part of an open balance that ages from its own due date.
type AgePart struct {
	Days   int // days past due as of now; <= 0 means not yet due
	Amount float64
}

func daysLate(due string, now time.Time) int {
	t := parseDate(due)
	if t == nil || !t.Before(now) {
		return 0
	}
	return int(now.Sub(*t).Hours() / 24)
}

// AgingParts spreads an open balance over its parts. An invoice without a schedule is one part.
// On a plan each open instalment ages by its own date, and whatever the open instalments do not
// cover (a refund pushing the balance past them) ages with the invoice as a whole.
func AgingParts(d Document, now time.Time) []AgePart {
	if d.Balance <= 0.004 {
		return nil
	}
	whole := daysLate(orDue(d), now)
	var open []Instalment
	for _, r := range d.Instalments {
		if r.Open > 0.004 {
			open = append(open, r)
		}
	}
	if len(open) == 0 {
		return []AgePart{{Days: whole, Amount: d.Balance}}
	}
	var parts []AgePart
	var sum int64
	for _, r := range open {
		due := r.DueDate
		if due == "" {
			due = d.IssueDate
		}
		parts = append(parts, AgePart{Days: daysLate(due, now), Amount: r.Open})
		sum += cents(r.Open)
	}
	if rest := cents(d.Balance) - sum; rest != 0 {
		parts = append(parts, AgePart{Days: whole, Amount: fromCents(rest)})
	}
	return parts
}

// OverdueAmount is how much of the balance is past due: the whole balance once the due date has
// passed, or on a plan only the open instalments whose own dates have.
func OverdueAmount(d Document, now time.Time) float64 {
	var total int64
	for _, p := range AgingParts(d, now) {
		if p.Days > 0 {
			total += cents(p.Amount)
		}
	}
	return fromCents(total)
}

// ageBucket maps days past due to 0 current, 1 = 1-30, 2 = 31-60, 3 = 61-90, 4 = 91+.
func ageBucket(days int) int {
	switch {
	case days > 90:
		return 4
	case days > 60:
		return 3
	case days > 30:
		return 2
	case days > 0:
		return 1
	}
	return 0
}
