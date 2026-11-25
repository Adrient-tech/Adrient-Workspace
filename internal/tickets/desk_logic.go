package tickets

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Ticket types : an incident can point at a problem ticket.
var TicketTypes = []string{"question", "incident", "problem", "task", "request", "change"}

// NormalizeType returns a known ticket type, "question" for anything else.
func NormalizeType(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, t := range TicketTypes {
		if t == s {
			return s
		}
	}
	return "question"
}

// NormalizeTags lower-cases, trims, replaces spaces with "-", drops empties and
// duplicates and caps the list (20 tags, 40 characters each).
func NormalizeTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range in {
		t := strings.ToLower(strings.TrimSpace(raw))
		t = strings.Join(strings.Fields(t), "-")
		t = strings.Trim(t, "#,")
		if t == "" || seen[t] {
			continue
		}
		if len([]rune(t)) > 40 {
			t = string([]rune(t)[:40])
		}
		seen[t] = true
		out = append(out, t)
		if len(out) == 20 {
			break
		}
	}
	return out
}

// AddRemoveTags applies additions and removals to a tag list.
func AddRemoveTags(cur, add, remove []string) []string {
	rm := map[string]bool{}
	for _, t := range NormalizeTags(remove) {
		rm[t] = true
	}
	out := []string{}
	for _, t := range NormalizeTags(append(append([]string{}, cur...), add...)) {
		if !rm[t] {
			out = append(out, t)
		}
	}
	return out
}

// IsDone reports whether a status ends the ticket's life (resolved or closed).
func IsDone(status string) bool { return status == "resolved" || status == "closed" }

// DeskTicket is a ticket with the fields the desk views and search need.
type DeskTicket struct {
	ID             string            `json:"id"`
	ProjectID      string            `json:"project_id"`
	Ref            string            `json:"ref"`
	Number         int64             `json:"number"`
	Title          string            `json:"title"`
	Body           string            `json:"body,omitempty"`
	Status         string            `json:"status"`
	Priority       string            `json:"priority"`
	Type           string            `json:"type"`
	Tags           []string          `json:"tags"`
	RequesterName  string            `json:"requester_name"`
	RequesterEmail string            `json:"requester_email"`
	Assignee       string            `json:"assignee_user_id,omitempty"`
	Source         string            `json:"source"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	ResolvedAt     *time.Time        `json:"resolved_at,omitempty"`
	MergedInto     string            `json:"merged_into,omitempty"`
	ProblemID      string            `json:"problem_id,omitempty"`
	GroupID        string            `json:"group_id,omitempty"`
	Urgency        string            `json:"urgency"`
	Impact         string            `json:"impact"`
	Category       string            `json:"category"`
	Subcategory    string            `json:"subcategory"`
	ParentID       string            `json:"parent_id,omitempty"`
	Major          bool              `json:"major"`
	SnoozedUntil   *time.Time        `json:"snoozed_until,omitempty"`
	FirstResponse  *time.Time        `json:"first_response_at,omitempty"`
	FormID         string            `json:"form_id,omitempty"`
	Custom         map[string]string `json:"custom"`
	Sentiment      string            `json:"sentiment"`
	AISummary      string            `json:"ai_summary"`
	NeedSkills     []string          `json:"need_skills"`
}

// ---- search -------------------------------------------------------------

// Filter is one key:value term of a search query.
type Filter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Query is a parsed search string: filters plus free-text words, or a ticket number.
type Query struct {
	Filters []Filter `json:"filters"`
	Words   []string `json:"words"`
	Number  int64    `json:"number,omitempty"`
}

// SearchKeys are the filter keys the search understands.
var SearchKeys = map[string]bool{"status": true, "assignee": true, "priority": true, "tag": true, "requester": true, "type": true, "via": true}

var numberRe = regexp.MustCompile(`^#(\d{1,9})$`)

// ParseSearch splits "status:open tag:billing refund #12" into filters, words and a number.
// Unknown "key:" prefixes stay ordinary words so a pasted URL or time still searches.
func ParseSearch(q string) Query {
	out := Query{Filters: []Filter{}, Words: []string{}}
	for _, tok := range splitQuery(q) {
		if m := numberRe.FindStringSubmatch(tok); m != nil {
			n, _ := strconv.ParseInt(m[1], 10, 64)
			out.Number = n
			continue
		}
		if i := strings.IndexByte(tok, ':'); i > 0 && i < len(tok)-1 {
			k := strings.ToLower(tok[:i])
			if SearchKeys[k] {
				out.Filters = append(out.Filters, Filter{Key: k, Value: strings.Trim(tok[i+1:], `"`)})
				continue
			}
		}
		out.Words = append(out.Words, strings.ToLower(strings.Trim(tok, `"`)))
	}
	return out
}

// splitQuery splits on spaces but keeps "quoted phrases" and key:"quoted value" together.
func splitQuery(q string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	for _, r := range q {
		switch {
		case r == '"':
			inQ = !inQ
			cur.WriteRune(r)
		case (r == ' ' || r == '\t') && !inQ:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// Matches reports whether a ticket satisfies the query. me is the viewing agent's user id.
func (q Query) Matches(t DeskTicket, me string) bool {
	if q.Number > 0 && t.Number != q.Number {
		return false
	}
	for _, f := range q.Filters {
		v := strings.ToLower(f.Value)
		switch f.Key {
		case "status":
			switch v {
			case "unsolved", "active":
				if IsDone(t.Status) {
					return false
				}
			case "solved":
				if !IsDone(t.Status) {
					return false
				}
			default:
				if strings.ToLower(t.Status) != v {
					return false
				}
			}
		case "assignee":
			switch v {
			case "me":
				if me == "" || t.Assignee != me {
					return false
				}
			case "none", "unassigned":
				if t.Assignee != "" {
					return false
				}
			default:
				if t.Assignee == "" || !strings.HasPrefix(strings.ToLower(t.Assignee), v) {
					return false
				}
			}
		case "priority":
			if t.Priority != v {
				return false
			}
		case "type":
			if t.Type != v {
				return false
			}
		case "via":
			if strings.ToLower(t.Source) != v {
				return false
			}
		case "tag":
			ok := false
			for _, tg := range t.Tags {
				if tg == strings.Join(strings.Fields(v), "-") {
					ok = true
				}
			}
			if !ok {
				return false
			}
		case "requester":
			if !strings.Contains(strings.ToLower(t.RequesterName), v) && !strings.Contains(strings.ToLower(t.RequesterEmail), v) {
				return false
			}
		}
	}
	if len(q.Words) > 0 {
		hay := strings.ToLower(t.Ref + " " + t.Title + " " + t.Body + " " + t.RequesterName + " " + t.RequesterEmail + " " + strings.Join(t.Tags, " "))
		for _, w := range q.Words {
			if !strings.Contains(hay, w) {
				return false
			}
		}
	}
	return true
}

// ---- built-in views -----------------------------------------------------

// View is a named ticket list with a live count.
type View struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

type viewDef struct {
	id, title string
	match     func(t DeskTicket, me string, now time.Time) bool
}

var viewDefs = []viewDef{
	{"mine", "Assigned to me", func(t DeskTicket, me string, _ time.Time) bool {
		return me != "" && t.Assignee == me && !IsDone(t.Status)
	}},
	{"unassigned", "Unassigned", func(t DeskTicket, _ string, _ time.Time) bool { return t.Assignee == "" && !IsDone(t.Status) }},
	{"urgent", "Urgent and high", func(t DeskTicket, _ string, _ time.Time) bool {
		return (t.Priority == "urgent" || t.Priority == "high") && !IsDone(t.Status)
	}},
	{"all_open", "All unsolved", func(t DeskTicket, _ string, _ time.Time) bool { return !IsDone(t.Status) }},
	{"stale", "No update for 3 days", func(t DeskTicket, _ string, now time.Time) bool {
		return !IsDone(t.Status) && now.Sub(t.UpdatedAt) > 72*time.Hour
	}},
	{"incidents", "Open incidents", func(t DeskTicket, _ string, _ time.Time) bool {
		return !IsDone(t.Status) && (t.Type == "incident" || t.Type == "problem")
	}},
	{"solved_recent", "Solved in the last 7 days", func(t DeskTicket, _ string, now time.Time) bool {
		return IsDone(t.Status) && t.ResolvedAt != nil && now.Sub(*t.ResolvedAt) <= 7*24*time.Hour
	}},
}

// ViewIDs lists the built-in view ids in display order.
func ViewIDs() []string {
	out := make([]string, len(viewDefs))
	for i, v := range viewDefs {
		out[i] = v.id
	}
	return out
}

// ViewCounts counts the tickets of each built-in view. Merged tickets are not counted.
func ViewCounts(ts []DeskTicket, me string, now time.Time) []View {
	out := make([]View, len(viewDefs))
	for i, v := range viewDefs {
		out[i] = View{ID: v.id, Title: v.title}
		for _, t := range ts {
			if t.MergedInto == "" && v.match(t, me, now) {
				out[i].Count++
			}
		}
	}
	return out
}

// InView filters tickets to one built-in view (most recently updated first). ok is false for an unknown id.
func InView(ts []DeskTicket, id, me string, now time.Time) (out []DeskTicket, ok bool) {
	for _, v := range viewDefs {
		if v.id != id {
			continue
		}
		out = []DeskTicket{}
		for _, t := range ts {
			if t.MergedInto == "" && v.match(t, me, now) {
				out = append(out, t)
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
		return out, true
	}
	return nil, false
}

// ---- macros -------------------------------------------------------------

// MacroActions is what applying a macro does to a ticket. Empty fields do nothing.
type MacroActions struct {
	Status     string   `json:"status,omitempty"`
	Priority   string   `json:"priority,omitempty"`
	Type       string   `json:"type,omitempty"`
	Assignee   string   `json:"assignee,omitempty"` // "me", "none" or a user id
	AddTags    []string `json:"add_tags,omitempty"`
	RemoveTags []string `json:"remove_tags,omitempty"`
	Reply      string   `json:"reply,omitempty"`
	Internal   bool     `json:"internal,omitempty"` // post the reply as an internal note
}

// Macro is a saved set of actions.
type Macro struct {
	ID        string       `json:"id"`
	ProjectID string       `json:"project_id,omitempty"`
	Name      string       `json:"name"`
	Actions   MacroActions `json:"actions"`
}

// Validate normalises and checks a macro against the project's status list.
func (m *Macro) Validate(statuses []string) error {
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" || len([]rune(m.Name)) > 80 {
		return fmt.Errorf("a macro needs a name of at most 80 characters")
	}
	a := &m.Actions
	if a.Status != "" && len(statuses) > 0 {
		ok := false
		for _, s := range statuses {
			ok = ok || s == a.Status
		}
		if !ok {
			return fmt.Errorf("unknown status %q", a.Status)
		}
	}
	if a.Priority != "" {
		a.Priority = normalizePriority(a.Priority)
	}
	if a.Type != "" {
		a.Type = NormalizeType(a.Type)
	}
	a.AddTags, a.RemoveTags = NormalizeTags(a.AddTags), NormalizeTags(a.RemoveTags)
	a.Reply = strings.TrimSpace(a.Reply)
	if len(a.Reply) > 8000 {
		return fmt.Errorf("the reply is too long")
	}
	if a.Status == "" && a.Priority == "" && a.Type == "" && a.Assignee == "" && len(a.AddTags) == 0 && len(a.RemoveTags) == 0 && a.Reply == "" {
		return fmt.Errorf("a macro needs at least one action")
	}
	return nil
}

var placeholderRe = regexp.MustCompile(`\{\{\s*([a-z_.]+)\s*\}\}`)

// FillPlaceholders replaces {{ticket.ref}}, {{ticket.title}}, {{requester.name}},
// {{requester.first_name}} and {{agent.name}}. Unknown names are left as written.
func FillPlaceholders(text string, t DeskTicket, agentName string) string {
	first := ""
	if f := strings.Fields(t.RequesterName); len(f) > 0 {
		first = f[0]
	}
	vals := map[string]string{
		"ticket.ref": t.Ref, "ticket.title": t.Title, "ticket.status": t.Status, "ticket.priority": t.Priority,
		"requester.name": t.RequesterName, "requester.first_name": first, "requester.email": t.RequesterEmail,
		"agent.name": agentName,
	}
	return placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		k := placeholderRe.FindStringSubmatch(m)[1]
		if v, ok := vals[k]; ok {
			return v
		}
		return m
	})
}
