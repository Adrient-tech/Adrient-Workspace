package tickets

import (
	"sort"
	"strings"
	"time"
)

// Routing strategies of a group.
const (
	StrategyManual     = "manual"
	StrategyRoundRobin = "round_robin"
	StrategyLoad       = "load"
)

// ValidStrategy reports whether s is a known routing strategy.
func ValidStrategy(s string) bool {
	return s == StrategyManual || s == StrategyRoundRobin || s == StrategyLoad
}

// Member is a group member as seen by routing.
type Member struct {
	UserID   string     `json:"user_id"`
	Name     string     `json:"name,omitempty"`
	Skills   []string   `json:"skills"`
	Capacity int        `json:"capacity"`
	OOOUntil *time.Time `json:"ooo_until,omitempty"`
	Position int        `json:"position"`
	Open     int        `json:"open"` // tickets currently assigned and not done (filled by the caller)
}

func (m Member) available(now time.Time) bool {
	return m.OOOUntil == nil || !m.OOOUntil.After(now)
}

func (m Member) hasSkills(need []string) bool {
	have := map[string]bool{}
	for _, s := range m.Skills {
		have[strings.ToLower(strings.TrimSpace(s))] = true
	}
	for _, n := range need {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" && !have[n] {
			return false
		}
	}
	return true
}

// PickAssignee chooses who should get a new ticket. cursor is the group's round-robin
// position; the returned next cursor must be stored by the caller. An empty userID
// means nobody qualifies (everyone away, full, or missing a required skill) and the
// ticket stays unassigned in the group queue.
func PickAssignee(strategy string, members []Member, needSkills []string, cursor int, now time.Time) (userID string, next int) {
	if strategy == StrategyManual || len(members) == 0 {
		return "", cursor
	}
	ms := append([]Member(nil), members...)
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].Position != ms[j].Position {
			return ms[i].Position < ms[j].Position
		}
		return ms[i].UserID < ms[j].UserID
	})
	ok := func(m Member) bool {
		return m.available(now) && m.hasSkills(needSkills) && (m.Capacity <= 0 || m.Open < m.Capacity)
	}
	switch strategy {
	case StrategyRoundRobin:
		n := len(ms)
		if cursor < 0 {
			cursor = 0
		}
		for i := 0; i < n; i++ {
			idx := (cursor + i) % n
			if ok(ms[idx]) {
				return ms[idx].UserID, (idx + 1) % n
			}
		}
	case StrategyLoad:
		best := -1
		for i, m := range ms {
			if !ok(m) {
				continue
			}
			// lowest utilisation first; ties go to the fewest open tickets, then list order
			if best < 0 || util(m) < util(ms[best]) || (util(m) == util(ms[best]) && m.Open < ms[best].Open) {
				best = i
			}
		}
		if best >= 0 {
			return ms[best].UserID, cursor
		}
	}
	return "", cursor
}

func util(m Member) float64 {
	if m.Capacity <= 0 {
		return float64(m.Open)
	}
	return float64(m.Open) / float64(m.Capacity)
}

// ---- priority matrix ----------------------------------------------------

// PriorityFromMatrix derives a priority from urgency and impact (low|medium|high),
// the usual 3x3 service-management matrix.
func PriorityFromMatrix(urgency, impact string) string {
	score := func(s string) int {
		switch strings.ToLower(s) {
		case "high":
			return 3
		case "low":
			return 1
		}
		return 2
	}
	switch sum := score(urgency) + score(impact); {
	case sum >= 6:
		return "urgent"
	case sum == 5:
		return "high"
	case sum >= 3:
		return "normal"
	default:
		return "low"
	}
}

// ---- status workflow ----------------------------------------------------

// StatusCategories are the groups a custom status falls into.
var StatusCategories = []string{"open", "pending", "hold", "solved", "closed"}

// StatusDef is one configurable status.
type StatusDef struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Category  string   `json:"category"`
	PausesSLA bool     `json:"pauses_sla"`
	Position  int      `json:"position"`
	Requires  []string `json:"requires"`
}

// DefaultStatusDefs is the built-in workflow used when a project defines none.
func DefaultStatusDefs() []StatusDef {
	return []StatusDef{
		{Key: "open", Label: "Open", Category: "open", Position: 0},
		{Key: "in_progress", Label: "In progress", Category: "open", Position: 1},
		{Key: "pending", Label: "Pending (waiting on requester)", Category: "pending", PausesSLA: true, Position: 2},
		{Key: "on_hold", Label: "On hold", Category: "hold", PausesSLA: true, Position: 3},
		{Key: "resolved", Label: "Resolved", Category: "solved", Position: 4},
		{Key: "closed", Label: "Closed", Category: "closed", Position: 5},
	}
}

// ValidateStatusDefs checks a configured workflow: unique keys, known categories, and at
// least one open and one solved/closed status.
func ValidateStatusDefs(defs []StatusDef) error {
	if len(defs) == 0 || len(defs) > 24 {
		return errStatus("between 1 and 24 statuses are allowed")
	}
	seen := map[string]bool{}
	hasOpen, hasDone := false, false
	for i := range defs {
		d := &defs[i]
		d.Key = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(d.Key, " ", "_")))
		d.Label = strings.TrimSpace(d.Label)
		if d.Key == "" || len(d.Key) > 32 {
			return errStatus("status key is required (max 32 characters)")
		}
		if d.Label == "" {
			d.Label = d.Key
		}
		if seen[d.Key] {
			return errStatus("duplicate status " + d.Key)
		}
		seen[d.Key] = true
		known := false
		for _, c := range StatusCategories {
			known = known || c == d.Category
		}
		if !known {
			return errStatus("unknown category for status " + d.Key)
		}
		hasOpen = hasOpen || d.Category == "open"
		hasDone = hasDone || d.Category == "solved" || d.Category == "closed"
		d.Position = i
	}
	if !hasOpen || !hasDone {
		return errStatus("need at least one open status and one solved or closed status")
	}
	return nil
}

type errStatus string

func (e errStatus) Error() string { return string(e) }

// CategoryOf returns a status' category, treating unknown keys by name.
func CategoryOf(defs []StatusDef, key string) string {
	for _, d := range defs {
		if d.Key == key {
			return d.Category
		}
	}
	switch key {
	case "resolved":
		return "solved"
	case "closed":
		return "closed"
	case "pending":
		return "pending"
	case "on_hold":
		return "hold"
	}
	return "open"
}

// PausesSLA reports whether a status stops the SLA clock.
func PausesSLA(defs []StatusDef, key string) bool {
	for _, d := range defs {
		if d.Key == key {
			return d.PausesSLA
		}
	}
	return key == "pending" || key == "on_hold"
}

// MissingRequired lists the fields a ticket still needs before it can enter a status.
// fields maps field name -> whether it is filled (assignee, category, resolution_note, ...).
func MissingRequired(defs []StatusDef, key string, filled map[string]bool) []string {
	for _, d := range defs {
		if d.Key != key {
			continue
		}
		var out []string
		for _, f := range d.Requires {
			if !filled[f] {
				out = append(out, f)
			}
		}
		return out
	}
	return nil
}

// ShouldReopen decides the automatic reopen: a requester reply on a solved ticket
// (not closed) moves it back to open.
func ShouldReopen(defs []StatusDef, status string, fromRequester bool) bool {
	return fromRequester && CategoryOf(defs, status) == "solved"
}
