package tickets

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ---- SLA ------------------------------------------------------------------

// SLAPolicy sets the targets for one priority.
type SLAPolicy struct {
	Priority         string `json:"priority"`
	FirstResponseMin int    `json:"first_response_min"`
	ResolveMin       int    `json:"resolve_min"`
}

// SLA clock states.
const (
	SLAOff      = ""         // no target configured
	SLAMet      = "met"      // done in time
	SLAOK       = "ok"       // running, plenty of time
	SLAAtRisk   = "at_risk"  // running, less than 20% of the target left
	SLABreached = "breached" // late (running or finished late)
)

// SLAClock is the state of one timer.
type SLAClock struct {
	State   string     `json:"state"`
	Due     *time.Time `json:"due,omitempty"`
	Paused  bool       `json:"paused,omitempty"`
	LeftMin *int       `json:"left_min,omitempty"`
}

// SLAInfo is the SLA view of one ticket.
type SLAInfo struct {
	FirstResponse SLAClock `json:"first_response"`
	Resolution    SLAClock `json:"resolution"`
}

func clock(created time.Time, targetMin int, doneAt *time.Time, now time.Time) SLAClock {
	if targetMin <= 0 {
		return SLAClock{}
	}
	d := created.Add(time.Duration(targetMin) * time.Minute)
	c := SLAClock{Due: &d}
	switch {
	case doneAt != nil && !doneAt.After(d):
		c.State = SLAMet
	case doneAt != nil:
		c.State = SLABreached
	case now.After(d):
		c.State = SLABreached
	case d.Sub(now) < time.Duration(targetMin)*time.Minute/5:
		c.State = SLAAtRisk
	default:
		c.State = SLAOK
	}
	return c
}

// ComputeSLA evaluates both timers. A ticket that is resolved or closed stops
// the resolution timer at resolvedAt. The first-response timer stops at the
// first public reply from an agent.
func ComputeSLA(p *SLAPolicy, created time.Time, firstResponse, resolved *time.Time, now time.Time) SLAInfo {
	if p == nil {
		return SLAInfo{}
	}
	return SLAInfo{
		FirstResponse: clock(created, p.FirstResponseMin, firstResponse, now),
		Resolution:    clock(created, p.ResolveMin, resolved, now),
	}
}

// ValidateSLA checks a policy list.
func ValidateSLA(ps []SLAPolicy) error {
	seen := map[string]bool{}
	for _, p := range ps {
		if !validPriority(p.Priority) {
			return fmt.Errorf("unknown priority %q", p.Priority)
		}
		if seen[p.Priority] {
			return fmt.Errorf("priority %q is listed twice", p.Priority)
		}
		seen[p.Priority] = true
		if p.FirstResponseMin < 0 || p.FirstResponseMin > 60*24*30 || p.ResolveMin < 0 || p.ResolveMin > 60*24*365 {
			return fmt.Errorf("targets are out of range")
		}
		if p.ResolveMin > 0 && p.FirstResponseMin > p.ResolveMin {
			return fmt.Errorf("first response cannot be later than resolution for %s", p.Priority)
		}
	}
	return nil
}

func validPriority(p string) bool {
	switch p {
	case "low", "normal", "high", "urgent":
		return true
	}
	return false
}

// ---- assignment rules ------------------------------------------------------

// Rule routes or labels new tickets. Empty match fields match everything;
// when several are set, all must match.
type Rule struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	MatchKeyword  string `json:"match_keyword"`
	MatchPriority string `json:"match_priority"`
	MatchSource   string `json:"match_source"`
	SetAssignee   string `json:"set_assignee"`
	SetPriority   string `json:"set_priority"`
	SetStatus     string `json:"set_status"`
}

// Matches reports whether the rule applies to a ticket.
func (r Rule) Matches(title, body, priority, source string) bool {
	if !r.Enabled {
		return false
	}
	if r.MatchPriority != "" && r.MatchPriority != priority {
		return false
	}
	if r.MatchSource != "" && r.MatchSource != source {
		return false
	}
	if kw := strings.ToLower(strings.TrimSpace(r.MatchKeyword)); kw != "" {
		hay := strings.ToLower(title + "\n" + body)
		hit := false
		for _, k := range strings.Split(kw, ",") {
			if k = strings.TrimSpace(k); k != "" && strings.Contains(hay, k) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return r.MatchKeyword != "" || r.MatchPriority != "" || r.MatchSource != ""
}

// Validate checks a rule is meaningful and safe to store.
func (r *Rule) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" || len(r.Name) > 100 {
		return fmt.Errorf("a rule needs a name (up to 100 characters)")
	}
	if r.MatchKeyword == "" && r.MatchPriority == "" && r.MatchSource == "" {
		return fmt.Errorf("add at least one condition, otherwise the rule would match every ticket")
	}
	if len(r.MatchKeyword) > 300 {
		return fmt.Errorf("keywords are too long")
	}
	if r.MatchPriority != "" && !validPriority(r.MatchPriority) {
		return fmt.Errorf("unknown priority condition")
	}
	if r.SetPriority != "" && !validPriority(r.SetPriority) {
		return fmt.Errorf("unknown priority action")
	}
	if r.MatchSource != "" && r.MatchSource != "public" && r.MatchSource != "team" {
		return fmt.Errorf("source must be public or team")
	}
	if r.SetAssignee == "" && r.SetPriority == "" && r.SetStatus == "" {
		return fmt.Errorf("add at least one action")
	}
	return nil
}

// FirstMatch returns the first enabled rule (in order) that matches.
func FirstMatch(rules []Rule, title, body, priority, source string) *Rule {
	for i := range rules {
		if rules[i].Matches(title, body, priority, source) {
			return &rules[i]
		}
	}
	return nil
}

// ---- customers ------------------------------------------------------------

// Customer groups tickets by requester email.
type Customer struct {
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Organisation string    `json:"organisation"`
	Total        int       `json:"total"`
	Open         int       `json:"open"`
	LastAt       time.Time `json:"last_at"`
}

var freeMail = map[string]bool{"gmail.com": true, "yahoo.com": true, "outlook.com": true, "hotmail.com": true, "icloud.com": true, "proton.me": true, "protonmail.com": true, "live.com": true, "aol.com": true, "rediffmail.com": true}

// OrgOf names a customer's organisation from the email domain; personal mail
// providers have none.
func OrgOf(email string) string {
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return ""
	}
	d := strings.ToLower(email[at+1:])
	if d == "" || freeMail[d] {
		return ""
	}
	return d
}

// ---- reports --------------------------------------------------------------

// ReportRow is one ticket as the report needs it.
type ReportRow struct {
	Created       time.Time
	Resolved      *time.Time
	FirstResponse *time.Time
	Priority      string
	Assignee      string
	SLA           SLAInfo
	Rating        *int
}

// Report summarises a period.
type Report struct {
	Created         int            `json:"created"`
	Resolved        int            `json:"resolved"`
	OpenNow         int            `json:"open_now"`
	AvgFirstRespMin float64        `json:"avg_first_response_min"`
	AvgResolveMin   float64        `json:"avg_resolution_min"`
	SLAMetPct       float64        `json:"sla_met_pct"`
	SLAChecked      int            `json:"sla_checked"`
	CSATAvg         float64        `json:"csat_avg"`
	CSATCount       int            `json:"csat_count"`
	ByPriority      map[string]int `json:"by_priority"`
	ByAssignee      map[string]int `json:"by_assignee"`
	CreatedPerDay   []DayCount     `json:"created_per_day"`
	ResolvedPerDay  []DayCount     `json:"resolved_per_day"`
}

// DayCount is a count on a day.
type DayCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// Summarize builds the report for tickets created in [from, to).
func Summarize(rows []ReportRow, from, to, now time.Time) Report {
	rep := Report{ByPriority: map[string]int{}, ByAssignee: map[string]int{}}
	cdays, rdays := map[string]int{}, map[string]int{}
	var frSum, rsSum float64
	var frN, rsN, ratingSum int
	for _, r := range rows {
		if !r.Created.Before(from) && r.Created.Before(to) {
			rep.Created++
			rep.ByPriority[r.Priority]++
			a := r.Assignee
			if a == "" {
				a = "Unassigned"
			}
			rep.ByAssignee[a]++
			cdays[r.Created.UTC().Format("2006-01-02")]++
			if r.FirstResponse != nil {
				frSum += r.FirstResponse.Sub(r.Created).Minutes()
				frN++
			}
			for _, c := range []SLAClock{r.SLA.FirstResponse, r.SLA.Resolution} {
				if c.State == SLAMet || c.State == SLABreached {
					rep.SLAChecked++
					if c.State == SLAMet {
						rep.SLAMetPct++
					}
				}
			}
			if r.Rating != nil {
				ratingSum += *r.Rating
				rep.CSATCount++
			}
		}
		if r.Resolved != nil && !r.Resolved.Before(from) && r.Resolved.Before(to) {
			rep.Resolved++
			rdays[r.Resolved.UTC().Format("2006-01-02")]++
			rsSum += r.Resolved.Sub(r.Created).Minutes()
			rsN++
		}
		if r.Resolved == nil && r.Created.Before(now) {
			rep.OpenNow++
		}
	}
	if frN > 0 {
		rep.AvgFirstRespMin = frSum / float64(frN)
	}
	if rsN > 0 {
		rep.AvgResolveMin = rsSum / float64(rsN)
	}
	if rep.SLAChecked > 0 {
		rep.SLAMetPct = rep.SLAMetPct * 100 / float64(rep.SLAChecked)
	}
	if rep.CSATCount > 0 {
		rep.CSATAvg = float64(ratingSum) / float64(rep.CSATCount)
	}
	rep.CreatedPerDay = daySeries(cdays, from, to)
	rep.ResolvedPerDay = daySeries(rdays, from, to)
	return rep
}

func daySeries(m map[string]int, from, to time.Time) []DayCount {
	var out []DayCount
	for d := from.UTC().Truncate(24 * time.Hour); d.Before(to); d = d.AddDate(0, 0, 1) {
		k := d.Format("2006-01-02")
		out = append(out, DayCount{Day: k, Count: m[k]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out
}

// ---- knowledge base --------------------------------------------------------

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify makes a URL slug from a title.
func Slugify(s string) string {
	s = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		return "article"
	}
	return s
}
