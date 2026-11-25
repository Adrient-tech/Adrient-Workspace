package authz

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
)

// Control value types.
const (
	CtlBool   = "bool"
	CtlEnum   = "enum"
	CtlNumber = "number"
)

// Rule effect modes. A restrict_only rule can only narrow a control: it wins
// over any broader or more specific "set" rule that would loosen it.
const (
	ModeSet          = "set"
	ModeRestrictOnly = "restrict_only"
)

// SubjectUserType targets a user type (member, external, guest, service).
const SubjectUserType = "user_type"

// ControlDef describes one per-app control. Enum values are ordered from the
// most restrictive to the most permissive; for bool, false is the restrictive
// side; for number, the smaller value is.
type ControlDef struct {
	Key     string   `json:"key"`
	App     string   `json:"app"`
	Label   string   `json:"label"`
	Help    string   `json:"help"`
	Type    string   `json:"type"`
	Enum    []string `json:"enum,omitempty"`
	Default any      `json:"default"`
	// Enforced says where the server applies the control (shown in the admin).
	Enforced string `json:"enforced_at"`
	// TypeDefaults overrides Default for a user type (guest, external).
	TypeDefaults map[string]any `json:"type_defaults,omitempty"`
	Min, Max     float64        `json:"-"`
}

// Controls is the catalog. Unknown keys are rejected everywhere.
var Controls = []ControlDef{
	{Key: "mail.external_forwarding", Enforced: "Mail: creating a filter or rule that forwards outside the organization", App: "mail", Label: "Forward mail to external addresses", Help: "Filters and rules may forward messages outside the organization.", Type: CtlBool, Default: true, TypeDefaults: map[string]any{TypeGuest: false, TypeExternal: false}},
	{Key: "mail.send_external", Enforced: "Mail: sending a message to an outside address", App: "mail", Label: "Send mail to external recipients", Help: "Allow sending to addresses outside the organization's verified domains.", Type: CtlBool, Default: true},
	{Key: "mail.attachment_max_mb", Enforced: "Mail: sending a message with attachments", App: "mail", Label: "Maximum attachment size (MB)", Help: "Largest attachment a user may send.", Type: CtlNumber, Default: 25.0, Min: 1, Max: 100},
	{Key: "drive.external_sharing", Enforced: "Drive and Docs: sharing with a person outside the organization, creating a share link", App: "drive", Label: "External sharing", Help: "off: nothing leaves the org. domains: only allowed partner domains. anyone: any outside person.", Type: CtlEnum, Enum: []string{"off", "domains", "anyone"}, Default: "domains"},
	{Key: "drive.guest_download", Enforced: "Drive: downloading a file as a guest", App: "drive", Label: "Guests can download files", Help: "Guests can view shared files; this decides whether they may also download them.", Type: CtlBool, Default: true, TypeDefaults: map[string]any{TypeGuest: false}},
	{Key: "drive.guest_reshare", Enforced: "Drive: sharing as a guest", App: "drive", Label: "Guests can re-share", Help: "Allow guests to share items with other people.", Type: CtlBool, Default: true, TypeDefaults: map[string]any{TypeGuest: false}},
	{Key: "drive.link_sharing_default", Enforced: "Drive and Docs: sets who can open a new share link, checked every time the link is opened", App: "drive", Label: "Default link visibility", Help: "Who can open a new share link by default.", Type: CtlEnum, Enum: []string{"org_only", "named", "anyone"}, Default: "org_only"},
	{Key: "chat.external_chats", Enforced: "Chat: starting a conversation or adding a member who is outside the organization", App: "chat", Label: "Chats with external people", Help: "off: no DMs or channels with outside people. allowlist: only allowed partner domains. on: anyone.", Type: CtlEnum, Enum: []string{"off", "allowlist", "on"}, Default: "allowlist"},
	{Key: "chat.file_upload_external", Enforced: "Chat: posting a message with attachments in a conversation that includes outside people", App: "chat", Label: "Upload files in external chats", Help: "Allow file uploads in chats that include external people.", Type: CtlBool, Default: true},
	{Key: "meet.recording", Enforced: "Meet: starting a recording", App: "meet", Label: "Recording", Help: "off: nobody. host: only the host. all: any participant.", Type: CtlEnum, Enum: []string{"off", "host", "all"}, Default: "host"},
	{Key: "meet.guests_allowed", Enforced: "Meet: a guest asking to join", App: "meet", Label: "Guests may join meetings", Help: "Allow guest accounts to join meetings.", Type: CtlBool, Default: true},
	{Key: "meet.allow_anonymous", Enforced: "Meet: joining through a meeting link without an account (always goes to the lobby)", App: "meet", Label: "Anonymous participants", Help: "Allow people without an account to join through a link.", Type: CtlBool, Default: false},
	{Key: "meet.lobby_for_external", Enforced: "Meet: an outside participant joining", App: "meet", Label: "Lobby for external participants", Help: "External participants wait in the lobby until admitted.", Type: CtlBool, Default: true},
	// AI assistant (internal/ai): who may use it; the org-wide switches live in Admin > AI assistant.
	{Key: "ai.assistant", Enforced: "AI assistant: every chat and Ask Adrient request", App: "ai", Label: "Use the AI assistant", Help: "Allow using the AI assistant in Sheets, Docs, Slides and Ask Adrient.", Type: CtlBool, Default: true, TypeDefaults: map[string]any{TypeGuest: false, TypeExternal: false}},
	// Stories (the Podcasts app): who may write stories, generate audio and download it.
	{Key: "stories.publish", Enforced: "Stories: creating a story, adding an episode, publishing", App: "podcasts", Label: "Write and publish stories", Help: "Allow writing and publishing stories. People who cannot publish can still listen.", Type: CtlBool, Default: true, TypeDefaults: map[string]any{TypeGuest: false, TypeExternal: false}},
	{Key: "stories.generate_audio", Enforced: "Stories: generating audio from the story text on the server", App: "podcasts", Label: "Generate story audio", Help: "Allow turning story text into audio with the server voices. Daily limits are set in Admin > Voices.", Type: CtlBool, Default: true, TypeDefaults: map[string]any{TypeGuest: false, TypeExternal: false}},
	{Key: "stories.download", Enforced: "Stories: downloading story audio (only for stories whose author allows it)", App: "podcasts", Label: "Download story audio", Help: "Allow downloading the audio of stories that allow downloads.", Type: CtlBool, Default: true, TypeDefaults: map[string]any{TypeGuest: false, TypeExternal: false}},
	{Key: "app.export_data", Enforced: "Drive downloads and share-link downloads", App: "*", Label: "Export and bulk download", Help: "Allow exports and bulk downloads.", Type: CtlBool, Default: true},
}

// ControlByKey returns the definition for key.
func ControlByKey(key string) (ControlDef, bool) {
	for _, c := range Controls {
		if c.Key == key {
			return c, true
		}
	}
	return ControlDef{}, false
}

// DefaultFor returns the default value for a user type.
func (d ControlDef) DefaultFor(userType string) any {
	if v, ok := d.TypeDefaults[userType]; ok {
		return v
	}
	return d.Default
}

// Normalize validates v against the definition and returns the canonical value
// (bool, string or float64).
func (d ControlDef) Normalize(v any) (any, error) {
	switch d.Type {
	case CtlBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%s expects true or false", d.Key)
		}
		return b, nil
	case CtlEnum:
		s, ok := v.(string)
		if !ok || indexOf(d.Enum, s) < 0 {
			return nil, fmt.Errorf("%s must be one of %s", d.Key, strings.Join(d.Enum, ", "))
		}
		return s, nil
	case CtlNumber:
		f, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("%s expects a number", d.Key)
		}
		if (d.Min != 0 || d.Max != 0) && (f < d.Min || f > d.Max) {
			return nil, fmt.Errorf("%s must be between %v and %v", d.Key, d.Min, d.Max)
		}
		return f, nil
	}
	return nil, fmt.Errorf("unknown control type")
}

func indexOf(l []string, v string) int {
	for i, x := range l {
		if x == v {
			return i
		}
	}
	return -1
}

// restrictiveness: lower is more restrictive.
func (d ControlDef) rank(v any) float64 {
	switch d.Type {
	case CtlBool:
		if b, _ := v.(bool); b {
			return 1
		}
		return 0
	case CtlEnum:
		s, _ := v.(string)
		return float64(indexOf(d.Enum, s))
	}
	f, _ := v.(float64)
	return f
}

// Condition limits when a rule or policy applies.
type Condition struct {
	IPCIDRs     []string `json:"ip_cidrs,omitempty"`
	UserTypes   []string `json:"user_types,omitempty"`
	Departments []string `json:"departments,omitempty"`
}

// Empty reports whether the condition has no constraints.
func (c *Condition) Empty() bool {
	return c == nil || (len(c.IPCIDRs) == 0 && len(c.UserTypes) == 0 && len(c.Departments) == 0)
}

// ParseCondition decodes the stored JSON; invalid or empty JSON gives nil.
func ParseCondition(raw []byte) *Condition {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var c Condition
	if json.Unmarshal(raw, &c) != nil || c.Empty() {
		return nil
	}
	return &c
}

// Validate checks that the CIDRs parse and user types are known.
func (c *Condition) Validate() error {
	if c == nil {
		return nil
	}
	for _, s := range c.IPCIDRs {
		if _, _, err := net.ParseCIDR(s); err != nil {
			if net.ParseIP(s) == nil {
				return fmt.Errorf("invalid IP range %q", s)
			}
		}
	}
	for _, t := range c.UserTypes {
		switch t {
		case TypeMember, TypeExternal, TypeGuest, TypeService:
		default:
			return fmt.Errorf("invalid user type %q", t)
		}
	}
	return nil
}

func ipInList(ip string, cidrs []string) bool {
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil {
		return false
	}
	for _, s := range cidrs {
		if _, n, err := net.ParseCIDR(s); err == nil {
			if n.Contains(p) {
				return true
			}
		} else if q := net.ParseIP(s); q != nil && q.Equal(p) {
			return true
		}
	}
	return false
}

// Met evaluates the condition for s and explains the first unmet part.
func (c *Condition) Met(s Subject) (bool, string) {
	if c.Empty() {
		return true, ""
	}
	if len(c.IPCIDRs) > 0 && !ipInList(s.IP, c.IPCIDRs) {
		if s.IP == "" {
			return false, "no client IP known"
		}
		return false, "IP " + s.IP + " is outside " + strings.Join(c.IPCIDRs, ", ")
	}
	if len(c.UserTypes) > 0 && indexOf(c.UserTypes, s.userType()) < 0 {
		return false, "user type " + s.userType() + " not in " + strings.Join(c.UserTypes, ", ")
	}
	if len(c.Departments) > 0 {
		hit := false
		for _, d := range c.Departments {
			hit = hit || contains(s.DeptIDs, d)
		}
		if !hit {
			return false, "not in the required department"
		}
	}
	return true, ""
}

func (s Subject) userType() string {
	if s.UserType == "" {
		return TypeMember
	}
	return s.UserType
}

// ControlRule is one row of adrient.app_control_rules.
type ControlRule struct {
	ID          string     `json:"id,omitempty"`
	AppID       string     `json:"app_id"`
	Control     string     `json:"control"`
	Value       any        `json:"value"`
	SubjectType string     `json:"subject_type"`
	SubjectID   string     `json:"subject_id"`
	Condition   *Condition `json:"condition,omitempty"`
	Priority    int        `json:"priority"`
	EffectMode  string     `json:"effect_mode"`
	Note        string     `json:"note,omitempty"`
}

// specificity: higher wins. user > group > department > user_type > role > everyone.
func specificity(subjectType string) int {
	switch subjectType {
	case SubjectUser:
		return 6
	case SubjectGroup:
		return 5
	case SubjectDepartment:
		return 4
	case SubjectUserType:
		return 3
	case SubjectRole:
		return 2
	case SubjectEveryone:
		return 1
	}
	return 0
}

func subjectMatches(subjectType, subjectID string, s Subject) bool {
	switch subjectType {
	case SubjectEveryone:
		return true
	case SubjectUser:
		return subjectID == s.UserID
	case SubjectGroup:
		return contains(s.GroupIDs, subjectID)
	case SubjectDepartment:
		return contains(s.DeptIDs, subjectID)
	case SubjectRole:
		return contains(s.RoleKeys, subjectID)
	case SubjectUserType:
		return subjectID == s.userType()
	}
	return false
}

// RuleEval is one rule considered during resolution.
type RuleEval struct {
	Rule   ControlRule `json:"rule"`
	Status string      `json:"status"` // winner | overridden | restricts | condition_not_met | not_applicable
	Why    string      `json:"why,omitempty"`
}

// ControlResult is the effective value of one control for one subject.
type ControlResult struct {
	Key    string     `json:"key"`
	App    string     `json:"app"`
	Value  any        `json:"value"`
	Source string     `json:"source"` // default | rule
	Winner *RuleEval  `json:"winner,omitempty"`
	Rules  []RuleEval `json:"rules,omitempty"`
}

// ResolveControl computes the effective value of def for s from rules.
// Order: most specific matching subject, then lowest priority number, then the
// newest rule id for determinism. restrict_only rules always narrow the result.
func ResolveControl(def ControlDef, rules []ControlRule, s Subject) ControlResult {
	res := ControlResult{Key: def.Key, App: def.App, Value: def.DefaultFor(s.userType()), Source: "default"}
	var applic []int
	for i, r := range rules {
		if r.Control != def.Key {
			continue
		}
		if !subjectMatches(r.SubjectType, r.SubjectID, s) {
			continue
		}
		if ok, why := r.Condition.Met(s); !ok {
			res.Rules = append(res.Rules, RuleEval{Rule: r, Status: "condition_not_met", Why: why})
			continue
		}
		applic = append(applic, i)
	}
	sort.SliceStable(applic, func(a, b int) bool {
		ra, rb := rules[applic[a]], rules[applic[b]]
		if sa, sb := specificity(ra.SubjectType), specificity(rb.SubjectType); sa != sb {
			return sa > sb
		}
		if ra.Priority != rb.Priority {
			return ra.Priority < rb.Priority
		}
		return ra.ID < rb.ID
	})
	var setRules, restrict []int
	for _, i := range applic {
		if rules[i].EffectMode == ModeRestrictOnly {
			restrict = append(restrict, i)
		} else {
			setRules = append(setRules, i)
		}
	}
	winnerIdx := -1
	if len(setRules) > 0 {
		winnerIdx = setRules[0]
		res.Value, res.Source = rules[winnerIdx].Value, "rule"
	}
	// restrict_only rules narrow the value: the most restrictive of the current
	// value and every applicable restriction wins.
	restrictWin := -1
	for _, i := range restrict {
		if def.rank(rules[i].Value) < def.rank(res.Value) {
			res.Value, res.Source = rules[i].Value, "rule"
			restrictWin = i
		}
	}
	win := winnerIdx
	if restrictWin >= 0 {
		win = restrictWin
	}
	for _, i := range applic {
		ev := RuleEval{Rule: rules[i]}
		switch {
		case i == win:
			ev.Status = "winner"
			ev.Why = "most specific matching rule"
			if i == restrictWin {
				ev.Why = "restrict-only rule narrows the value"
			}
			w := ev
			res.Winner = &w
		case rules[i].EffectMode == ModeRestrictOnly:
			ev.Status, ev.Why = "restricts", "restrict-only rule did not narrow the value further"
		default:
			ev.Status, ev.Why = "overridden", "a more specific or higher priority rule applies"
		}
		res.Rules = append(res.Rules, ev)
	}
	return res
}

// BoolValue / StringValue / NumberValue read a resolved value.
func (r ControlResult) Bool() bool { b, _ := r.Value.(bool); return b }

// String returns the value of an enum control.
func (r ControlResult) String() string { s, _ := r.Value.(string); return s }

// Number returns the value of a number control.
func (r ControlResult) Number() float64 { f, _ := r.Value.(float64); return f }

// ControlDenied is returned by enforcement sites when a control or the
// external collaboration policy refuses an action. Reason is safe to show.
type ControlDenied struct {
	Control string
	Reason  string
}

func (e *ControlDenied) Error() string { return e.Reason }

// Denied builds a *ControlDenied.
func Denied(control, reason string) error { return &ControlDenied{Control: control, Reason: reason} }

// AsDenied extracts a *ControlDenied from err.
func AsDenied(err error) (*ControlDenied, bool) {
	for err != nil {
		if d, ok := err.(*ControlDenied); ok {
			return d, true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return nil, false
		}
		err = u.Unwrap()
	}
	return nil, false
}

// WriteControlDenied writes the 403 JSON body for a refused control.
func WriteControlDenied(w http.ResponseWriter, d *ControlDenied) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": "forbidden", "message": d.Reason, "control": d.Control})
}
