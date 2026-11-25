package tickets

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Automation kinds and events.
const (
	KindTrigger  = "trigger"  // runs when a ticket is created or updated
	KindTime     = "time"     // runs on a schedule against tickets that match the conditions
	KindDispatch = "dispatch" // runs only on creation, to route tickets
)

// Condition is one test on a ticket.
type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// Action is one change or side effect.
type Action struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Automation is a rule: when it applies, which conditions must hold, what to do.
type Automation struct {
	ID        string      `json:"id"`
	ProjectID string      `json:"project_id,omitempty"`
	Name      string      `json:"name"`
	Kind      string      `json:"kind"`
	Event     string      `json:"event"`      // create | update | any
	MatchMode string      `json:"match_mode"` // all | any
	Conds     []Condition `json:"conditions"`
	Actions   []Action    `json:"actions"`
	Position  int         `json:"position"`
	Enabled   bool        `json:"enabled"`
	RunCount  int         `json:"run_count"`
	LastRunAt *time.Time  `json:"last_run_at,omitempty"`
}

// AutoTicket is the ticket state a rule sees.
type AutoTicket struct {
	Title, Body, Status, Priority, Type, Category, Subcategory string
	Group, Assignee, RequesterEmail, Source, Urgency, Impact   string
	Tags                                                       []string
	Major                                                      bool
	NeedSkills                                                 []string
	CreatedAt, UpdatedAt                                       time.Time
	Fields                                                     map[string]string // custom fields
}

// ConditionFields lists the fields a condition can test, with the kind of value each takes.
var ConditionFields = map[string]string{
	"status": "text", "priority": "text", "type": "text", "category": "text", "subcategory": "text",
	"group": "text", "assignee": "text", "requester_email": "text", "requester_domain": "text",
	"source": "text", "urgency": "text", "impact": "text", "title": "text", "body": "text",
	"tag": "text", "major": "bool", "hours_since_created": "number", "hours_since_update": "number",
}

var condOps = map[string]bool{"is": true, "is_not": true, "contains": true, "not_contains": true,
	"starts_with": true, "in": true, "empty": true, "not_empty": true, "gt": true, "lt": true}

// ActionTypes lists what an action can do. "set_*" change a field; the rest are side effects.
var ActionTypes = map[string]bool{
	"set_status": true, "set_priority": true, "set_type": true, "set_group": true, "set_assignee": true,
	"set_category": true, "require_skill": true, "add_tag": true, "remove_tag": true, "mark_major": true,
	"notify": true, "webhook": true, "create_task": true, "add_note": true,
}

// Validate checks an automation before it is saved.
func (a *Automation) Validate(statuses []string) error {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" || len(a.Name) > 120 {
		return errStatus("name is required (max 120 characters)")
	}
	switch a.Kind {
	case KindTrigger, KindTime, KindDispatch:
	case "":
		a.Kind = KindTrigger
	default:
		return errStatus("unknown automation kind")
	}
	switch a.Event {
	case "create", "update", "any":
	case "":
		a.Event = "create"
	default:
		return errStatus("unknown event")
	}
	if a.Kind == KindDispatch {
		a.Event = "create"
	}
	if a.Kind == KindTime {
		a.Event = "any"
	}
	if a.MatchMode != "any" {
		a.MatchMode = "all"
	}
	if len(a.Conds) > 20 || len(a.Actions) > 20 {
		return errStatus("too many conditions or actions")
	}
	if len(a.Actions) == 0 {
		return errStatus("add at least one action")
	}
	if a.Kind == KindTime && len(a.Conds) == 0 {
		return errStatus("a time-based rule needs at least one condition, otherwise it would touch every ticket")
	}
	for i := range a.Conds {
		c := &a.Conds[i]
		kind, ok := ConditionFields[c.Field]
		if !ok && !strings.HasPrefix(c.Field, "custom.") {
			return errStatus("unknown condition field " + c.Field)
		}
		if !condOps[c.Op] {
			return errStatus("unknown operator " + c.Op)
		}
		if (c.Op == "gt" || c.Op == "lt") && kind != "number" {
			return errStatus("greater/less only apply to number fields")
		}
		if kind == "number" {
			if _, err := strconv.ParseFloat(strings.TrimSpace(c.Value), 64); err != nil {
				return errStatus("condition " + c.Field + " needs a number")
			}
		}
		c.Value = strings.TrimSpace(c.Value)
	}
	for i := range a.Actions {
		x := &a.Actions[i]
		if !ActionTypes[x.Type] {
			return errStatus("unknown action " + x.Type)
		}
		x.Value = strings.TrimSpace(x.Value)
		if x.Value == "" && x.Type != "mark_major" && x.Type != "set_assignee" {
			return errStatus("action " + x.Type + " needs a value")
		}
		if x.Type == "set_status" && len(statuses) > 0 && !containsStr(statuses, x.Value) {
			return errStatus("status " + x.Value + " is not in this project's workflow")
		}
		if x.Type == "set_priority" && !validPriority(x.Value) {
			return errStatus("priority must be low, normal, high or urgent")
		}
		if x.Type == "webhook" {
			u, err := url.Parse(x.Value)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				return errStatus("webhook needs an http(s) URL")
			}
		}
	}
	return nil
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// CondResult is the outcome of one condition, for the rule tester.
type CondResult struct {
	Field  string `json:"field"`
	Op     string `json:"op"`
	Value  string `json:"value"`
	Actual string `json:"actual"`
	Pass   bool   `json:"pass"`
}

func (t AutoTicket) fieldValue(f string, now time.Time) string {
	switch f {
	case "status":
		return t.Status
	case "priority":
		return t.Priority
	case "type":
		return t.Type
	case "category":
		return t.Category
	case "subcategory":
		return t.Subcategory
	case "group":
		return t.Group
	case "assignee":
		return t.Assignee
	case "requester_email":
		return strings.ToLower(t.RequesterEmail)
	case "requester_domain":
		if i := strings.LastIndexByte(t.RequesterEmail, '@'); i >= 0 {
			return strings.ToLower(t.RequesterEmail[i+1:])
		}
		return ""
	case "source":
		return t.Source
	case "urgency":
		return t.Urgency
	case "impact":
		return t.Impact
	case "title":
		return t.Title
	case "body":
		return t.Body
	case "tag":
		return strings.Join(t.Tags, ",")
	case "major":
		return strconv.FormatBool(t.Major)
	case "hours_since_created":
		return strconv.Itoa(int(now.Sub(t.CreatedAt).Hours()))
	case "hours_since_update":
		return strconv.Itoa(int(now.Sub(t.UpdatedAt).Hours()))
	}
	if strings.HasPrefix(f, "custom.") {
		return t.Fields[strings.TrimPrefix(f, "custom.")]
	}
	return ""
}

func (c Condition) test(t AutoTicket, now time.Time) CondResult {
	actual := t.fieldValue(c.Field, now)
	want := c.Value
	r := CondResult{Field: c.Field, Op: c.Op, Value: want, Actual: actual}
	la, lw := strings.ToLower(actual), strings.ToLower(want)
	if c.Field == "tag" { // tag tests match against the list, not the joined text
		has := func(v string) bool {
			for _, tg := range t.Tags {
				if strings.EqualFold(tg, v) {
					return true
				}
			}
			return false
		}
		switch c.Op {
		case "is", "contains":
			r.Pass = has(want)
		case "is_not", "not_contains":
			r.Pass = !has(want)
		case "empty":
			r.Pass = len(t.Tags) == 0
		case "not_empty":
			r.Pass = len(t.Tags) > 0
		case "in":
			for _, v := range strings.Split(want, ",") {
				r.Pass = r.Pass || has(strings.TrimSpace(v))
			}
		}
		return r
	}
	switch c.Op {
	case "is":
		r.Pass = la == lw
	case "is_not":
		r.Pass = la != lw
	case "contains":
		r.Pass = strings.Contains(la, lw)
	case "not_contains":
		r.Pass = !strings.Contains(la, lw)
	case "starts_with":
		r.Pass = strings.HasPrefix(la, lw)
	case "in":
		for _, v := range strings.Split(lw, ",") {
			r.Pass = r.Pass || strings.TrimSpace(v) == la
		}
	case "empty":
		r.Pass = strings.TrimSpace(actual) == ""
	case "not_empty":
		r.Pass = strings.TrimSpace(actual) != ""
	case "gt", "lt":
		a, e1 := strconv.ParseFloat(actual, 64)
		b, e2 := strconv.ParseFloat(want, 64)
		if e1 == nil && e2 == nil {
			r.Pass = (c.Op == "gt" && a > b) || (c.Op == "lt" && a < b)
		}
	}
	return r
}

// Evaluate tests a rule against a ticket for the given event ("create" or "update"; use
// "any" for a scheduled sweep). It returns whether the rule applies and every
// condition's result.
func (a Automation) Evaluate(t AutoTicket, event string, now time.Time) (bool, []CondResult) {
	if !a.Enabled {
		return false, nil
	}
	switch a.Kind {
	case KindDispatch:
		if event != "create" {
			return false, nil
		}
	case KindTime:
		if event != "any" {
			return false, nil
		}
	default:
		if a.Event != "any" && a.Event != event {
			return false, nil
		}
		if event == "any" {
			return false, nil
		}
	}
	res := make([]CondResult, 0, len(a.Conds))
	pass := a.MatchMode != "any" // all: start true; any: start false
	if len(a.Conds) == 0 {
		return true, res
	}
	for _, c := range a.Conds {
		r := c.test(t, now)
		res = append(res, r)
		if a.MatchMode == "any" {
			pass = pass || r.Pass
		} else {
			pass = pass && r.Pass
		}
	}
	return pass, res
}

// Effect is one planned side effect that is not a plain field change.
type Effect struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Apply runs a rule's actions on a copy of the ticket and returns the new state plus the
// side effects (notify, webhook, create_task, add_note) to perform.
func (a Automation) Apply(t AutoTicket) (AutoTicket, []Effect) {
	out := t
	out.Tags = append([]string(nil), t.Tags...)
	out.NeedSkills = append([]string(nil), t.NeedSkills...)
	var fx []Effect
	for _, x := range a.Actions {
		switch x.Type {
		case "set_status":
			out.Status = x.Value
		case "set_priority":
			out.Priority = x.Value
		case "set_type":
			out.Type = NormalizeType(x.Value)
		case "set_group":
			out.Group = x.Value
		case "set_assignee":
			out.Assignee = x.Value
		case "set_category":
			out.Category = x.Value
		case "mark_major":
			out.Major = true
		case "require_skill":
			out.NeedSkills = NormalizeTags(append(append([]string{}, out.NeedSkills...), strings.Split(x.Value, ",")...))
		case "add_tag":
			out.Tags = AddRemoveTags(out.Tags, []string{x.Value}, nil)
		case "remove_tag":
			out.Tags = AddRemoveTags(out.Tags, nil, []string{x.Value})
		default:
			fx = append(fx, Effect{Type: x.Type, Value: x.Value})
		}
	}
	return out, fx
}

// RunAll evaluates rules in position order, feeding each rule the state left by the
// previous one. A rule fires at most once per call. It returns the final state, the
// names of the rules that fired and all side effects.
func RunAll(rules []Automation, t AutoTicket, event string, now time.Time) (AutoTicket, []string, []Effect) {
	cur := t
	var fired []string
	var fx []Effect
	for _, a := range rules {
		ok, _ := a.Evaluate(cur, event, now)
		if !ok {
			continue
		}
		next, e := a.Apply(cur)
		cur = next
		fired = append(fired, a.Name)
		fx = append(fx, e...)
		if len(fired) >= 25 {
			break
		}
	}
	return cur, fired, fx
}

// DescribeAutomation renders a rule as a sentence for the builder preview.
func DescribeAutomation(a Automation) string {
	var cs []string
	for _, c := range a.Conds {
		switch c.Op {
		case "empty", "not_empty":
			cs = append(cs, fmt.Sprintf("%s %s", c.Field, strings.ReplaceAll(c.Op, "_", " ")))
		default:
			cs = append(cs, fmt.Sprintf("%s %s %q", c.Field, strings.ReplaceAll(c.Op, "_", " "), c.Value))
		}
	}
	var as []string
	for _, x := range a.Actions {
		as = append(as, strings.ReplaceAll(x.Type, "_", " ")+" "+x.Value)
	}
	join := " and "
	if a.MatchMode == "any" {
		join = " or "
	}
	when := "When a ticket is " + map[string]string{"create": "created", "update": "updated", "any": "created or updated"}[a.Event]
	if a.Kind == KindTime {
		when = "Every few minutes, for tickets"
	}
	if len(cs) == 0 {
		return when + ": " + strings.Join(as, ", ")
	}
	return when + " where " + strings.Join(cs, join) + ": " + strings.Join(as, ", ")
}
