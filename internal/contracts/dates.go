// Package contracts tracks agreements: renewals and notice deadlines,
// recurring obligations, documents (Drive) and parties (Contacts), with
// reminders pushed to Calendar and Tasks.
package contracts

import "time"

const dayLayout = "2006-01-02"

// Date is a calendar day as YYYY-MM-DD ("" is no date).
type Date = string

func parse(d Date) (time.Time, bool) {
	t, err := time.Parse(dayLayout, d)
	return t, err == nil
}

// IsValidDate reports whether d is a real calendar day in YYYY-MM-DD form.
func IsValidDate(d Date) bool { _, ok := parse(d); return ok }

// AddDays adds n days to d.
func AddDays(d Date, n int) Date {
	t, ok := parse(d)
	if !ok {
		return ""
	}
	return t.AddDate(0, 0, n).Format(dayLayout)
}

// AddMonths adds n months, clamping the day to the target month (Jan 31 + 1 = Feb 28 or 29).
func AddMonths(d Date, n int) Date {
	t, ok := parse(d)
	if !ok {
		return ""
	}
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := t.Day()
	if day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC).Format(dayLayout)
}

// DaysBetween is the whole days from a to b (positive when b is later).
func DaysBetween(a, b Date) int {
	x, ok1 := parse(a)
	y, ok2 := parse(b)
	if !ok1 || !ok2 {
		return 0
	}
	return int(y.Sub(x).Hours() / 24)
}

// Today is today's date in loc.
func Today(now time.Time, loc *time.Location) Date {
	if loc == nil {
		loc = time.UTC
	}
	return now.In(loc).Format(dayLayout)
}

// Decision kinds recorded about a renewal.
const (
	DecNoticeGiven = "notice-given"
	DecRenewing    = "renewing"
	DecNotRenewing = "not-renewing"
	DecSnoozed     = "snoozed"
)

// Decision is what the user decided about the renewal at ForTermEnd.
type Decision struct {
	Kind       string `json:"kind"`
	ForTermEnd Date   `json:"for_term_end,omitempty"`
	Until      Date   `json:"until,omitempty"`
	Note       string `json:"note,omitempty"`
	At         string `json:"at,omitempty"`
}

func validDecisionKind(k string) bool {
	return k == DecNoticeGiven || k == DecRenewing || k == DecNotRenewing || k == DecSnoozed
}

func endingDecision(k string) bool { return k == DecNoticeGiven || k == DecNotRenewing }

// Contract holds the fields the date logic needs.
type Contract struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	StartDate  Date      `json:"start_date"`
	EndDate    Date      `json:"end_date"`
	AutoRenew  bool      `json:"auto_renew"`
	TermMonths int       `json:"term_months"`
	NoticeDays int       `json:"notice_days"`
	Decision   *Decision `json:"decision,omitempty"`
}

func (c Contract) term() int {
	if c.TermMonths < 1 {
		return 12
	}
	return c.TermMonths
}

// decisionFor is the contract's decision when it applies to termEnd (a stale one gives nil).
func (c Contract) decisionFor(termEnd Date) *Decision {
	d := c.Decision
	if d == nil || !validDecisionKind(d.Kind) {
		return nil
	}
	if termEnd != "" && d.ForTermEnd != "" && d.ForTermEnd != termEnd {
		return nil
	}
	return d
}

func decisionHandles(d *Decision, today Date) bool {
	if d == nil {
		return false
	}
	if d.Kind == DecSnoozed {
		return IsValidDate(d.Until) && DaysBetween(today, d.Until) >= 0
	}
	return validDecisionKind(d.Kind)
}

// IsTermEnd reports whether date is the end date or (auto-renewing) a renewal boundary after it.
func (c Contract) IsTermEnd(date Date) bool {
	if !IsValidDate(c.EndDate) || !IsValidDate(date) {
		return false
	}
	if date == c.EndDate {
		return true
	}
	if !c.AutoRenew || date < c.EndDate {
		return false
	}
	for i := 1; i <= 1200; i++ {
		b := AddMonths(c.EndDate, c.term()*i)
		if b == date {
			return true
		}
		if b > date {
			return false
		}
	}
	return false
}

// NextTermEnd is the next end of term on or after today. Auto-renewing contracts roll forward by the renewal
// term (each boundary is computed from the original end date, so month-end dates never drift). A recorded
// ending decision caps the roll-forward at the term it was recorded for.
func (c Contract) NextTermEnd(today Date) Date {
	if !IsValidDate(c.EndDate) {
		return ""
	}
	last := Date("")
	if c.Decision != nil && endingDecision(c.Decision.Kind) && c.IsTermEnd(c.Decision.ForTermEnd) {
		last = c.Decision.ForTermEnd
	}
	if !c.AutoRenew {
		return c.EndDate
	}
	date := c.EndDate
	for i := 1; i <= 1200 && DaysBetween(today, date) < 0; i++ {
		date = AddMonths(c.EndDate, c.term()*i)
	}
	if last != "" && DaysBetween(last, date) > 0 {
		return last
	}
	return date
}

// AgreementEnd is when the agreement itself stops: "" for evergreen or rolling contracts unless an ending decision was recorded.
func (c Contract) AgreementEnd(today Date) Date {
	end := c.NextTermEnd(today)
	if end == "" || !c.AutoRenew {
		return end
	}
	if c.Decision != nil && endingDecision(c.Decision.Kind) && c.Decision.ForTermEnd == end {
		return end
	}
	return ""
}

// Term is one finished term.
type Term struct {
	Start Date `json:"start"`
	End   Date `json:"end"`
}

// CompletedTerms lists the terms already finished, oldest first.
func (c Contract) CompletedTerms(today Date) []Term {
	if !IsValidDate(c.EndDate) || DaysBetween(today, c.EndDate) >= 0 {
		return []Term{}
	}
	first := Date("")
	if IsValidDate(c.StartDate) {
		first = c.StartDate
	}
	if !c.AutoRenew {
		return []Term{{first, c.EndDate}}
	}
	out := []Term{}
	start, boundary := first, c.EndDate
	for i := 1; i <= 1200 && DaysBetween(today, boundary) < 0; i++ {
		out = append(out, Term{start, boundary})
		start = AddDays(boundary, 1)
		boundary = AddMonths(c.EndDate, c.term()*i)
		if c.Decision != nil && endingDecision(c.Decision.Kind) && c.Decision.ForTermEnd == out[len(out)-1].End {
			break
		}
	}
	return out
}

// Renewal phases.
const (
	PhaseNone      = "none"
	PhaseEvergreen = "evergreen"
	PhaseExpired   = "expired"
	PhaseNotice    = "notice"
	PhaseSoon      = "soon"
	PhaseOK        = "ok"
)

// Renewal describes a contract's clock.
type Renewal struct {
	Phase          string    `json:"phase"`
	Date           Date      `json:"date,omitempty"`
	DaysLeft       *int      `json:"days_left,omitempty"`
	NoticeDate     Date      `json:"notice_date,omitempty"`
	NoticeDaysLeft *int      `json:"notice_days_left,omitempty"`
	Decision       *Decision `json:"decision,omitempty"`
	Handled        bool      `json:"handled"`
	Attention      bool      `json:"attention"`
}

func ip(n int) *int { return &n }

// RenewalInfo computes the phase: evergreen, expired, notice (the window to cancel or renegotiate is open), soon, ok.
func (c Contract) RenewalInfo(today Date, soonDays int) Renewal {
	if c.Status == "terminated" || c.Status == "draft" {
		return Renewal{Phase: PhaseNone}
	}
	date := c.NextTermEnd(today)
	if date == "" {
		return Renewal{Phase: PhaseEvergreen}
	}
	dec := c.decisionFor(date)
	handled := decisionHandles(dec, today)
	left := DaysBetween(today, date)
	r := Renewal{Phase: PhaseOK, Date: date, DaysLeft: ip(left), Decision: dec, Handled: handled}
	if left < 0 {
		r.Phase = PhaseExpired
	} else {
		nd := c.NoticeDays
		if nd < 0 {
			nd = 0
		}
		if nd > 0 {
			r.NoticeDate = AddDays(date, -nd)
			r.NoticeDaysLeft = ip(DaysBetween(today, r.NoticeDate))
		}
		if nd > 0 && left <= nd {
			r.Phase = PhaseNotice
		} else if left <= soonDays {
			r.Phase = PhaseSoon
		}
	}
	r.Attention = c.Status == "active" && !handled && (r.Phase == PhaseNotice || r.Phase == PhaseExpired)
	return r
}
