package shifts

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

// Candidate is a suggested assignee for an open shift.
type Candidate struct {
	EmployeeID string   `json:"employee_id"`
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	Deficit    int      `json:"deficit_minutes"`
	RoleMatch  bool     `json:"role_match"`
	Projected  int      `json:"projected_minutes"`
	Warnings   []string `json:"warnings"`
	Reasons    []string `json:"reasons"`
}

// SuggestAssignees ranks who could take s. Hard blockers (inactive, time off,
// overlap, unavailable, over weekly max) are excluded; broken working-time
// rules demote rather than exclude.
func SuggestAssignees(s Shift, staff []Staff, shifts []Shift, rules Rules, limit int) []Candidate {
	week := StartOfWeek(s.Date)
	var others []Shift
	for _, o := range shifts {
		if o.ID != s.ID {
			others = append(others, o)
		}
	}
	net := NetMinutes(s)
	role := strings.ToLower(strings.TrimSpace(s.Role))
	var out []Candidate
	for _, e := range staff {
		if !e.Active || e.EmployeeID == s.EmployeeID {
			continue
		}
		if TimeOffOn(e.Availability, s.Date) != nil {
			continue
		}
		if isOvernight(s.Start, s.End) && TimeOffOn(e.Availability, opshttp.AddDays(s.Date, 1)) != nil {
			continue
		}
		status := AvailabilityStatus(s, e.Availability)
		if status == AvailUnavailable || len(FindConflicts(s, others, e.EmployeeID)) > 0 {
			continue
		}
		cur := WeeklyMinutes(others, e.EmployeeID, week)
		proj := cur + net
		if e.MaxHours != nil && float64(proj) > *e.MaxHours*60 {
			continue
		}
		deficit := 0
		if e.TargetHours != nil {
			deficit = max(0, int(*e.TargetHours*60)-cur)
		}
		match := role != "" && strings.ToLower(strings.TrimSpace(e.Role)) == role
		var warn []string
		for _, v := range RuleViolations(s, e.EmployeeID, others, rules, e.Name) {
			warn = append(warn, v.Short)
		}
		reasons := []string{"Available"}
		if status == AvailPartial {
			reasons[0] = "Partially available"
		}
		if deficit > 0 {
			reasons = append(reasons, formatHours(deficit)+" below target")
		}
		if match {
			reasons = append(reasons, "Role matches")
		}
		reasons = append(reasons, warn...)
		out = append(out, Candidate{EmployeeID: e.EmployeeID, Name: e.Name, Status: status, Deficit: deficit, RoleMatch: match, Projected: proj, Warnings: warn, Reasons: reasons})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (len(a.Warnings) > 0) != (len(b.Warnings) > 0) {
			return len(a.Warnings) == 0
		}
		if a.Status != b.Status {
			return a.Status == AvailAvailable
		}
		if a.Deficit != b.Deficit {
			return a.Deficit > b.Deficit
		}
		if a.RoleMatch != b.RoleMatch {
			return a.RoleMatch
		}
		return a.Name < b.Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// FillProposal is one open shift with the person proposed for it.
type FillProposal struct {
	Shift      Shift       `json:"shift"`
	Candidates []Candidate `json:"candidates"`
	Proposal   *Candidate  `json:"proposal"`
}

// PlanFill proposes an assignee for every open shift in dates, each proposal
// aware of the ones before it (earliest shift first).
func PlanFill(shifts []Shift, staff []Staff, rules Rules, dates []string) []FillProposal {
	days := map[string]bool{}
	for _, d := range dates {
		days[d] = true
	}
	var open []Shift
	for _, s := range shifts {
		if s.EmployeeID == "" && days[s.Date] {
			open = append(open, s)
		}
	}
	sort.SliceStable(open, func(i, j int) bool {
		a, b := open[i], open[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.ID < b.ID
	})
	working := append([]Shift(nil), shifts...)
	plan := make([]FillProposal, 0, len(open))
	for _, s := range open {
		c := SuggestAssignees(s, staff, working, rules, 4)
		p := FillProposal{Shift: s, Candidates: c}
		if len(c) > 0 {
			p.Proposal = &c[0]
			for i := range working {
				if working[i].ID == s.ID {
					working[i].EmployeeID = c[0].EmployeeID
				}
			}
		}
		plan = append(plan, p)
	}
	return plan
}

// Gap is an unmet coverage requirement on one date.
type Gap struct {
	Date      string   `json:"date"`
	Role      string   `json:"role"`
	Required  int      `json:"required"`
	Scheduled int      `json:"scheduled"`
	Missing   int      `json:"missing"`
	Coverage  Coverage `json:"requirement"`
}

// CoverageGaps compares the shifts (open ones count) against the requirements.
func CoverageGaps(shifts []Shift, reqs []Coverage, dates []string) []Gap {
	var gaps []Gap
	for _, date := range dates {
		dow := weekday(date)
		base := dayNumber(date) * 1440
		for _, rq := range reqs {
			has := false
			for _, d := range rq.Days {
				has = has || d == dow
			}
			if !has {
				continue
			}
			ws, we := 0, 1440
			if v, ok := toMinutes(rq.Start); ok {
				ws = v
			}
			if v, ok := toMinutes(rq.End); ok {
				we = v
			}
			if we == 1439 {
				we = 1440
			}
			if we <= ws {
				ws, we = 0, 1440
			}
			lo, hi := base+ws, base+we
			n := 0
			for _, s := range shifts {
				if !strings.EqualFold(strings.TrimSpace(s.Role), strings.TrimSpace(rq.Role)) {
					continue
				}
				if a, b, ok := absoluteRange(s); ok && a < hi && lo < b {
					n++
				}
			}
			if n < rq.Count {
				gaps = append(gaps, Gap{Date: date, Role: rq.Role, Required: rq.Count, Scheduled: n, Missing: rq.Count - n, Coverage: rq})
			}
		}
	}
	return gaps
}

// CostOptions configure the overtime split.
type CostOptions struct {
	ThresholdHours float64 `json:"overtime_threshold_hours"`
	Multiplier     float64 `json:"overtime_multiplier"`
}

// Bucket is one row of a cost breakdown.
type Bucket struct {
	Key     string  `json:"key"`
	Minutes int     `json:"minutes"`
	Cost    float64 `json:"cost"`
}

// Cost is the projected labour cost of a range.
type Cost struct {
	Cost            float64  `json:"cost"`
	BaseCost        float64  `json:"base_cost"`
	OvertimeCost    float64  `json:"overtime_cost"`
	Minutes         int      `json:"minutes"`
	OvertimeMinutes int      `json:"overtime_minutes"`
	RatedCount      int      `json:"rated_count"`
	EmployeeCount   int      `json:"employee_count"`
	ByDay           []Bucket `json:"by_day"`
	ByRole          []Bucket `json:"by_role"`
	ByEmployee      []Bucket `json:"by_employee"`
}

// LaborCost prices the assigned shifts on dates: net hours times the hourly
// rate, with paid minutes past a weekly threshold costing the multiplier. The
// split follows the order shifts are worked.
func LaborCost(shifts []Shift, staff []Staff, dates []string, o CostOptions) Cost {
	days, weeks := map[string]bool{}, map[string]bool{}
	for _, d := range dates {
		days[d] = true
		weeks[StartOfWeek(d)] = true
	}
	byID := map[string]Staff{}
	for _, s := range staff {
		byID[s.EmployeeID] = s
	}
	thr := o.ThresholdHours * 60
	otOn := o.ThresholdHours > 0 && o.Multiplier > 1
	var ordered []Shift
	for _, s := range shifts {
		if s.EmployeeID != "" && (days[s.Date] || (otOn && weeks[StartOfWeek(s.Date)])) {
			ordered = append(ordered, s)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.ID < b.ID
	})
	c := Cost{ByDay: []Bucket{}, ByRole: []Bucket{}, ByEmployee: []Bucket{}}
	involved, rated := map[string]bool{}, map[string]bool{}
	dayB, roleB, empB := map[string]*Bucket{}, map[string]*Bucket{}, map[string]*Bucket{}
	add := func(m map[string]*Bucket, k string, net int, money float64) {
		b := m[k]
		if b == nil {
			b = &Bucket{Key: k}
			m[k] = b
		}
		b.Minutes += net
		b.Cost += money
	}
	week := map[string]int{}
	for _, s := range ordered {
		net := NetMinutes(s)
		key := s.EmployeeID + "|" + StartOfWeek(s.Date)
		before := week[key]
		week[key] = before + net
		if !days[s.Date] {
			continue
		}
		involved[s.EmployeeID] = true
		c.Minutes += net
		money := 0.0
		if e, ok := byID[s.EmployeeID]; ok && e.HourlyRate != nil {
			rated[s.EmployeeID] = true
			ot := 0
			if otOn {
				ot = int(max(0, float64(before+net)-thr) - max(0, float64(before)-thr))
			}
			mult := 1.0
			if otOn {
				mult = o.Multiplier
			}
			base := float64(net-ot) / 60 * *e.HourlyRate
			otm := float64(ot) / 60 * *e.HourlyRate * mult
			money = base + otm
			c.BaseCost += base
			c.OvertimeCost += otm
			c.OvertimeMinutes += ot
			c.Cost += money
		}
		role := strings.TrimSpace(s.Role)
		if role == "" {
			role = "No role"
		}
		add(dayB, s.Date, net, money)
		add(roleB, role, net, money)
		add(empB, s.EmployeeID, net, money)
	}
	c.RatedCount, c.EmployeeCount = len(rated), len(involved)
	flat := func(m map[string]*Bucket, less func(a, b Bucket) bool) []Bucket {
		out := make([]Bucket, 0, len(m))
		for _, b := range m {
			b.Cost = opshttp.Round2(b.Cost)
			out = append(out, *b)
		}
		sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
		return out
	}
	byCost := func(a, b Bucket) bool {
		if a.Cost != b.Cost {
			return a.Cost > b.Cost
		}
		if a.Minutes != b.Minutes {
			return a.Minutes > b.Minutes
		}
		return a.Key < b.Key
	}
	c.ByDay = flat(dayB, func(a, b Bucket) bool { return a.Key < b.Key })
	c.ByRole = flat(roleB, byCost)
	c.ByEmployee = flat(empB, byCost)
	c.Cost, c.BaseCost, c.OvertimeCost = opshttp.Round2(c.Cost), opshttp.Round2(c.BaseCost), opshttp.Round2(c.OvertimeCost)
	return c
}

// LateMinutes is how late clockIn is against the shift start (overnight aware).
func LateMinutes(s Shift, clockIn string) int {
	st, ok1 := toMinutes(s.Start)
	en, ok2 := toMinutes(s.End)
	in, ok3 := toMinutes(clockIn)
	if !ok1 || !ok3 {
		return 0
	}
	if in >= st {
		return in - st
	}
	if ok2 && en < st && in <= en {
		return in + 1440 - st
	}
	return 0
}

// PaidMinutes is what payroll pays for a shift: nothing when absent, the
// clocked time minus the unpaid break when recorded, else the scheduled net.
func PaidMinutes(s Shift, a *Attendance) int {
	if a != nil && a.Status == "absent" {
		return 0
	}
	if a != nil {
		if w, ok := DurationMinutes(a.ClockIn, a.ClockOut); ok {
			return max(0, w-max(0, s.BreakMinutes))
		}
	}
	return NetMinutes(s)
}

// EvaluateSwap lists the non-blocking warnings for handing s to staff member to.
func EvaluateSwap(s Shift, to Staff, shifts []Shift, rules Rules, hasAttendance bool) []string {
	var w []string
	if len(FindConflicts(s, shifts, to.EmployeeID)) > 0 {
		w = append(w, "The receiving employee already has an overlapping shift.")
	}
	w = append(w, assignWarnings(s, to, shifts, rules)...)
	if hasAttendance {
		w = append(w, fmt.Sprintf("The attendance already recorded for this shift becomes %s's.", to.Name))
	}
	return w
}
