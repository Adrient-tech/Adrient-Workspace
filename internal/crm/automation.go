package crm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Automation triggers.
const (
	TrigStageChanged  = "stage_changed"
	TrigDealIdle      = "deal_idle"
	TrigFormSubmitted = "form_submitted"
	TrigEmailReceived = "email_received"
	TrigLeadCreated   = "lead_created"
)

var triggers = map[string]bool{TrigStageChanged: true, TrigDealIdle: true, TrigFormSubmitted: true, TrigEmailReceived: true, TrigLeadCreated: true}

// Hooks are the optional integrations automation uses. Any may be nil; the
// matching action then records "skipped" instead of failing.
type Hooks struct {
	// Notify posts a Chat message to a user.
	Notify func(ctx context.Context, org, userID, text string) error
	// SendMail sends a system mail (automation has no signed-in sender).
	SendMail func(ctx context.Context, org string, to []string, subject, body string) error
	// SendAsUser sends through the Mail app as the signed-in caller and returns the thread id.
	SendAsUser func(ctx context.Context, to []string, subject, body string) (string, error)
	// OnDealWon runs once a deal enters the Won stage (estimate, project hand-off). Errors are the hook's to log.
	OnDealWon func(ctx context.Context, org, dealID string)
}

// Event is something that happened in the CRM that rules can react to. Key
// uniquely identifies the occurrence: a rule runs at most once per key.
type Event struct {
	Org       string
	Trigger   string
	DealID    string
	PersonID  string
	FormID    string
	Stage     string
	FromStage string
	FromAddr  string
	Source    string
	Key       string
}

// Action is one step of a rule.
type Action struct {
	Type       string   `json:"type"` // create_followup | assign_owner | send_template | notify_chat
	Title      string   `json:"title,omitempty"`
	DueInDays  int      `json:"due_in_days,omitempty"`
	Users      []string `json:"users,omitempty"`
	TeamID     string   `json:"team_id,omitempty"`
	TemplateID string   `json:"template_id,omitempty"`
	To         string   `json:"to,omitempty"` // notify_chat: "owner" (default) or a user id
	Text       string   `json:"text,omitempty"`
}

// Rule is a workflow rule.
type Rule struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Enabled    bool           `json:"enabled"`
	Trigger    string         `json:"trigger"`
	Conditions map[string]any `json:"conditions"`
	Actions    []Action       `json:"actions"`
}

func condStr(c map[string]any, k string) string {
	s, _ := c[k].(string)
	return s
}

func condInt(c map[string]any, k string) int {
	switch t := c[k].(type) {
	case float64:
		return int(t)
	case int:
		return t
	}
	return 0
}

// RuleMatches reports whether an enabled rule applies to an event (the event
// trigger must equal the rule trigger and every set condition must hold).
func RuleMatches(r Rule, ev Event) bool {
	if !r.Enabled || r.Trigger != ev.Trigger {
		return false
	}
	c := r.Conditions
	if v := condStr(c, "stage"); v != "" && v != ev.Stage {
		return false
	}
	if v := condStr(c, "from_stage"); v != "" && v != ev.FromStage {
		return false
	}
	if v := condStr(c, "form_id"); v != "" && v != ev.FormID {
		return false
	}
	if v := condStr(c, "source"); v != "" && !strings.EqualFold(v, ev.Source) {
		return false
	}
	if v := strings.ToLower(condStr(c, "from_domain")); v != "" {
		at := strings.LastIndex(ev.FromAddr, "@")
		if at < 0 || strings.ToLower(ev.FromAddr[at+1:]) != strings.TrimPrefix(v, "@") {
			return false
		}
	}
	return true
}

// Validate checks a rule definition.
func (r *Rule) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return invalid("rule name is required")
	}
	if !triggers[r.Trigger] {
		return invalid("unknown trigger %q", r.Trigger)
	}
	if r.Conditions == nil {
		r.Conditions = map[string]any{}
	}
	if r.Trigger == TrigDealIdle && condInt(r.Conditions, "idle_days") <= 0 {
		return invalid("an idle rule needs idle_days above 0")
	}
	if len(r.Actions) == 0 {
		return invalid("a rule needs at least one action")
	}
	for _, a := range r.Actions {
		switch a.Type {
		case "create_followup":
			if strings.TrimSpace(a.Title) == "" || a.DueInDays < 0 {
				return invalid("follow-up action needs a title and a non-negative due_in_days")
			}
		case "assign_owner":
			if len(a.Users) == 0 && a.TeamID == "" {
				return invalid("assign action needs users or a team")
			}
		case "send_template":
			if a.TemplateID == "" {
				return invalid("send_template action needs a template")
			}
		case "notify_chat":
			if strings.TrimSpace(a.Text) == "" {
				return invalid("notify action needs text")
			}
		default:
			return invalid("unknown action %q", a.Type)
		}
	}
	return nil
}

// RenderTemplate replaces {{key}} placeholders from vars (unknown keys become empty).
func RenderTemplate(s string, vars map[string]string) string {
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "}}")
		if j < 0 {
			return s
		}
		key := strings.TrimSpace(s[i+2 : i+j])
		s = s[:i] + vars[key] + s[i+j+2:]
	}
}

// PickRoundRobin returns the user at counter idx.
func PickRoundRobin(users []string, idx int64) string {
	if len(users) == 0 {
		return ""
	}
	return users[int(idx%int64(len(users)))]
}

// ---- rule CRUD ----

func (r *Repository) ListRules(ctx context.Context, org string) ([]Rule, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, enabled, trigger, conditions, actions FROM adrient.crm_rules WHERE org_id=$1 ORDER BY created_at`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var x Rule
		var c, a []byte
		if err := rows.Scan(&x.ID, &x.Name, &x.Enabled, &x.Trigger, &c, &a); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(c, &x.Conditions)
		_ = json.Unmarshal(a, &x.Actions)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) SaveRule(ctx context.Context, org, id string, x Rule) (Rule, error) {
	if err := x.Validate(); err != nil {
		return x, err
	}
	c, _ := json.Marshal(x.Conditions)
	a, _ := json.Marshal(x.Actions)
	if id == "" {
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_rules (org_id,name,enabled,trigger,conditions,actions) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
			org, x.Name, x.Enabled, x.Trigger, c, a).Scan(&x.ID)
		return x, err
	}
	ct, err := r.pool.Exec(ctx, `UPDATE adrient.crm_rules SET name=$3,enabled=$4,trigger=$5,conditions=$6,actions=$7 WHERE org_id=$1 AND id=$2`, org, id, x.Name, x.Enabled, x.Trigger, c, a)
	if err != nil {
		return x, err
	}
	if ct.RowsAffected() == 0 {
		return x, ErrNotFound
	}
	x.ID = id
	return x, nil
}

func (r *Repository) DeleteRule(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_rules WHERE org_id=$1 AND id=$2`, org, id)
}

// RuleRun is one recorded execution.
type RuleRun struct {
	RuleID   string          `json:"rule_id"`
	RuleName string          `json:"rule_name"`
	EventKey string          `json:"event_key"`
	Result   json.RawMessage `json:"result"`
	RanAt    time.Time       `json:"ran_at"`
}

func (r *Repository) ListRuleRuns(ctx context.Context, org string, limit int) ([]RuleRun, error) {
	rows, err := r.pool.Query(ctx, `SELECT rr.rule_id::text, ru.name, rr.event_key, rr.result, rr.ran_at FROM adrient.crm_rule_runs rr
		JOIN adrient.crm_rules ru ON ru.id=rr.rule_id WHERE rr.org_id=$1 ORDER BY rr.ran_at DESC LIMIT $2`, org, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RuleRun{}
	for rows.Next() {
		var x RuleRun
		if err := rows.Scan(&x.RuleID, &x.RuleName, &x.EventKey, &x.Result, &x.RanAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ---- engine ----

// fire runs every enabled matching rule once for the event. Failures are
// logged and never abort the caller's operation.
func (r *Repository) fire(ctx context.Context, ev Event) {
	if ev.Key == "" {
		return
	}
	rules, err := r.ListRules(ctx, ev.Org)
	if err != nil {
		slog.Warn("crm rules", "err", err)
		return
	}
	for _, rule := range rules {
		if RuleMatches(rule, ev) {
			r.runRule(ctx, rule, ev)
		}
	}
}

// Fire is the exported entry for other packages (forms, mail glue).
func (r *Repository) Fire(ctx context.Context, ev Event) { r.fire(ctx, ev) }

// runRule claims (rule, event key) and executes the actions. A second call for
// the same key is a no-op, which makes retries and sweeps idempotent.
func (r *Repository) runRule(ctx context.Context, rule Rule, ev Event) bool {
	ct, err := r.pool.Exec(ctx, `INSERT INTO adrient.crm_rule_runs (org_id,rule_id,event_key) VALUES ($1,$2,$3) ON CONFLICT (rule_id,event_key) DO NOTHING`,
		ev.Org, rule.ID, ev.Key)
	if err != nil || ct.RowsAffected() == 0 {
		return false
	}
	res := []string{}
	for _, a := range rule.Actions {
		msg, err := r.execAction(ctx, rule, a, ev)
		if err != nil {
			msg = a.Type + ": error: " + err.Error()
			slog.Warn("crm rule action", "rule", rule.Name, "action", a.Type, "err", err)
		}
		res = append(res, msg)
	}
	b, _ := json.Marshal(res)
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.crm_rule_runs SET result=$3 WHERE rule_id=$1 AND event_key=$2`, rule.ID, ev.Key, b)
	return true
}

// eventContext resolves the records an event refers to.
type eventContext struct {
	DealID, DealTitle, Stage, Value, OwnerID string
	PersonID, ContactID, PersonName, Email   string
	CompanyName, OwnerName                   string
}

func (r *Repository) loadContext(ctx context.Context, ev Event) eventContext {
	var c eventContext
	c.PersonID = ev.PersonID
	if ev.DealID != "" {
		var contact, owner, company *string
		_ = r.pool.QueryRow(ctx, `SELECT d.id::text, d.title, d.stage, d.value::text, d.contact_id::text, d.owner_id::text,
			(SELECT name FROM adrient.crm_companies WHERE id=d.company_id) FROM adrient.crm_deals d WHERE d.org_id=$1 AND d.id=$2`, ev.Org, ev.DealID).
			Scan(&c.DealID, &c.DealTitle, &c.Stage, &c.Value, &contact, &owner, &company)
		if contact != nil {
			c.ContactID = *contact
		}
		if owner != nil {
			c.OwnerID = *owner
		}
		if company != nil {
			c.CompanyName = *company
		}
	}
	if c.ContactID == "" && ev.PersonID != "" {
		_ = r.pool.QueryRow(ctx, `SELECT contact_id::text FROM adrient.crm_people WHERE org_id=$1 AND id=$2`, ev.Org, ev.PersonID).Scan(&c.ContactID)
	}
	if ev.PersonID != "" && c.OwnerID == "" {
		var o *string
		_ = r.pool.QueryRow(ctx, `SELECT owner_id::text FROM adrient.crm_people WHERE org_id=$1 AND id=$2`, ev.Org, ev.PersonID).Scan(&o)
		if o != nil {
			c.OwnerID = *o
		}
	}
	if c.ContactID != "" {
		_ = r.pool.QueryRow(ctx, `SELECT display_name, coalesce(emails->>0,'') FROM adrient.contacts WHERE org_id=$1 AND id=$2`, ev.Org, c.ContactID).Scan(&c.PersonName, &c.Email)
		if ev.PersonID == "" {
			_ = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.crm_people WHERE org_id=$1 AND contact_id=$2`, ev.Org, c.ContactID).Scan(&c.PersonID)
		}
	}
	if c.OwnerID != "" {
		_ = r.pool.QueryRow(ctx, `SELECT display_name FROM adrient.users WHERE id=$1`, c.OwnerID).Scan(&c.OwnerName)
	}
	return c
}

func (c eventContext) vars() map[string]string {
	return map[string]string{"deal.title": c.DealTitle, "deal.stage": c.Stage, "deal.value": c.Value, "person.name": c.PersonName,
		"person.email": c.Email, "company.name": c.CompanyName, "owner.name": c.OwnerName}
}

func (r *Repository) execAction(ctx context.Context, rule Rule, a Action, ev Event) (string, error) {
	ec := r.loadContext(ctx, ev)
	vars := ec.vars()
	switch a.Type {
	case "create_followup":
		due := time.Now().AddDate(0, 0, a.DueInDays).Format("2006-01-02")
		var owner any
		if ec.OwnerID != "" {
			owner = ec.OwnerID
		}
		_, err := r.pool.Exec(ctx, `INSERT INTO adrient.crm_followups (org_id,title,due_date,deal_id,contact_id,user_id) VALUES ($1,$2,$3,$4,$5,$6)`,
			ev.Org, RenderTemplate(a.Title, vars), due, nz(ec.DealID), nz(ec.ContactID), owner)
		return "create_followup: due " + due, err
	case "assign_owner":
		users := a.Users
		if a.TeamID != "" {
			rows, err := r.pool.Query(ctx, `SELECT m.user_id::text FROM adrient.crm_team_members m JOIN adrient.crm_teams t ON t.id=m.team_id
				WHERE t.org_id=$1 AND t.id=$2 ORDER BY m.user_id`, ev.Org, a.TeamID)
			if err != nil {
				return "", err
			}
			for rows.Next() {
				var u string
				_ = rows.Scan(&u)
				users = append(users, u)
			}
			rows.Close()
		}
		idx, err := r.nextCounter(ctx, ev.Org, "rule:"+rule.ID)
		if err != nil {
			return "", err
		}
		pick := PickRoundRobin(users, idx)
		if pick == "" {
			return "assign_owner: no users", nil
		}
		if ec.DealID != "" {
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.crm_deals SET owner_id=$3 WHERE org_id=$1 AND id=$2`, ev.Org, ec.DealID, pick); err != nil {
				return "", err
			}
		}
		if ec.PersonID != "" {
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.crm_people SET owner_id=$3 WHERE org_id=$1 AND id=$2`, ev.Org, ec.PersonID, pick); err != nil {
				return "", err
			}
		}
		return "assign_owner: " + pick, nil
	case "send_template":
		if r.hooks == nil || r.hooks.SendMail == nil {
			return "send_template: skipped (mail is not configured)", nil
		}
		if ec.Email == "" {
			return "send_template: skipped (no email address)", nil
		}
		var subj, body string
		if err := r.pool.QueryRow(ctx, `SELECT subject, body FROM adrient.crm_email_templates WHERE org_id=$1 AND id=$2`, ev.Org, a.TemplateID).Scan(&subj, &body); err != nil {
			return "", fmt.Errorf("template not found")
		}
		if err := r.hooks.SendMail(ctx, ev.Org, []string{ec.Email}, RenderTemplate(subj, vars), RenderTemplate(body, vars)); err != nil {
			return "", err
		}
		_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.crm_activities (org_id,kind,summary,direction,deal_id,contact_id) VALUES ($1,'email',$2,'out',$3,$4)`,
			ev.Org, "Automated mail: "+RenderTemplate(subj, vars), nz(ec.DealID), nz(ec.ContactID))
		return "send_template: sent to " + ec.Email, nil
	case "notify_chat":
		to := a.To
		if to == "" || to == "owner" {
			to = ec.OwnerID
		}
		if to == "" {
			return "notify_chat: skipped (no owner)", nil
		}
		if r.hooks == nil || r.hooks.Notify == nil {
			return "notify_chat: skipped (chat not available)", nil
		}
		if err := r.hooks.Notify(ctx, ev.Org, to, RenderTemplate(a.Text, vars)); err != nil {
			return "", err
		}
		return "notify_chat: sent", nil
	}
	return "", fmt.Errorf("unknown action %q", a.Type)
}

// nextCounter atomically returns 0,1,2,... for a key.
func (r *Repository) nextCounter(ctx context.Context, org, key string) (int64, error) {
	var idx int64
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_rr_state (org_id,key,idx) VALUES ($1,$2,0)
		ON CONFLICT (org_id,key) DO UPDATE SET idx = adrient.crm_rr_state.idx + 1 RETURNING idx`, org, key).Scan(&idx)
	return idx, err
}

// onStageEntered seeds the stage playbook checklist (idempotent) and fires stage_changed.
func (r *Repository) onStageEntered(ctx context.Context, org, dealID, stage, from string) {
	if err := r.seedChecklist(ctx, org, dealID, stage); err != nil {
		slog.Warn("crm checklist", "err", err)
	}
	if stage == StageWon && from != StageWon && r.hooks != nil && r.hooks.OnDealWon != nil {
		r.hooks.OnDealWon(ctx, org, dealID)
	}
	if from == "" {
		return
	}
	var hid string
	if err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.crm_stage_history WHERE deal_id=$1 ORDER BY entered_at DESC, id LIMIT 1`, dealID).Scan(&hid); err != nil {
		return
	}
	r.fire(ctx, Event{Org: org, Trigger: TrigStageChanged, DealID: dealID, Stage: stage, FromStage: from, Key: "stage:" + hid})
}

// SweepIdle fires deal_idle rules for open deals untouched for the rule's
// idle_days. The event key carries the last-touch time, so a deal fires once
// per idle period and again only after new activity and a new idle stretch.
func (r *Repository) SweepIdle(ctx context.Context) (int, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, org_id::text, name, enabled, trigger, conditions, actions FROM adrient.crm_rules WHERE enabled AND trigger=$1`, TrigDealIdle)
	if err != nil {
		return 0, err
	}
	var rules []struct {
		org string
		r   Rule
	}
	for rows.Next() {
		var x Rule
		var org string
		var c, a []byte
		if err := rows.Scan(&x.ID, &org, &x.Name, &x.Enabled, &x.Trigger, &c, &a); err != nil {
			rows.Close()
			return 0, err
		}
		_ = json.Unmarshal(c, &x.Conditions)
		_ = json.Unmarshal(a, &x.Actions)
		rules = append(rules, struct {
			org string
			r   Rule
		}{org, x})
	}
	rows.Close()
	fired := 0
	for _, ru := range rules {
		days := condInt(ru.r.Conditions, "idle_days")
		if days <= 0 {
			continue
		}
		drows, err := r.pool.Query(ctx, `SELECT id::text, stage, extract(epoch from touch)::bigint FROM (
			SELECT d.id, d.stage, greatest(d.updated_at, d.stage_entered_at,
			  coalesce((SELECT max(a.occurred_at) FROM adrient.crm_activities a WHERE a.deal_id=d.id), d.created_at)) AS touch
			FROM adrient.crm_deals d WHERE d.org_id=$1 AND d.stage NOT IN ('won','lost')) x
			WHERE touch < now() - make_interval(days => $2)`, ru.org, days)
		if err != nil {
			return fired, err
		}
		type hit struct {
			id, stage string
			touch     int64
		}
		var hits []hit
		for drows.Next() {
			var h hit
			_ = drows.Scan(&h.id, &h.stage, &h.touch)
			hits = append(hits, h)
		}
		drows.Close()
		for _, h := range hits {
			ev := Event{Org: ru.org, Trigger: TrigDealIdle, DealID: h.id, Stage: h.stage, Key: fmt.Sprintf("idle:%s:%d", h.id, h.touch)}
			if RuleMatches(ru.r, ev) && r.runRule(ctx, ru.r, ev) {
				fired++
			}
		}
	}
	return fired, nil
}

// RunBackground sweeps idle deals and auto-logs mail for opted-in users until ctx ends.
func (r *Repository) RunBackground(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := r.SweepIdle(ctx); err != nil {
				slog.Warn("crm idle sweep", "err", err)
			}
			r.AutoLogMail(ctx)
		}
	}
}

// ---- playbooks ----

// Playbook is the checklist applied when a deal enters a stage.
type Playbook struct {
	ID       string   `json:"id"`
	Stage    string   `json:"stage"`
	Name     string   `json:"name"`
	Items    []string `json:"items"`
	Required bool     `json:"required"`
}

func (r *Repository) ListPlaybooks(ctx context.Context, org string) ([]Playbook, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, stage, name, items, required FROM adrient.crm_playbooks WHERE org_id=$1 ORDER BY stage`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Playbook{}
	for rows.Next() {
		var p Playbook
		var it []byte
		if err := rows.Scan(&p.ID, &p.Stage, &p.Name, &it, &p.Required); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(it, &p.Items)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) SavePlaybook(ctx context.Context, org string, p Playbook) (Playbook, error) {
	if err := r.validStage(ctx, org, p.Stage); err != nil {
		return p, err
	}
	items := []string{}
	for _, s := range p.Items {
		if s = strings.TrimSpace(s); s != "" {
			items = append(items, s)
		}
	}
	if len(items) == 0 || len(items) > 30 {
		return p, invalid("a playbook needs between 1 and 30 items")
	}
	p.Items = items
	b, _ := json.Marshal(items)
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_playbooks (org_id,stage,name,items,required) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (org_id,stage) DO UPDATE SET name=EXCLUDED.name, items=EXCLUDED.items, required=EXCLUDED.required RETURNING id::text`,
		org, p.Stage, strings.TrimSpace(p.Name), b, p.Required).Scan(&p.ID)
	return p, err
}

func (r *Repository) DeletePlaybook(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_playbooks WHERE org_id=$1 AND id=$2`, org, id)
}

// seedChecklist copies the stage playbook onto the deal; existing rows (and their done state) are kept.
func (r *Repository) seedChecklist(ctx context.Context, org, dealID, stage string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.crm_deal_checklist (deal_id,stage,position,text)
		SELECT $2, pb.stage, (i.ord-1)::int, i.val FROM adrient.crm_playbooks pb,
		  jsonb_array_elements_text(pb.items) WITH ORDINALITY AS i(val, ord)
		WHERE pb.org_id=$1 AND pb.stage=$3 ON CONFLICT DO NOTHING`, org, dealID, stage)
	return err
}

// ChecklistItem is one deal checklist row.
type ChecklistItem struct {
	Stage    string `json:"stage"`
	Position int    `json:"position"`
	Text     string `json:"text"`
	Done     bool   `json:"done"`
}

func (r *Repository) ListChecklist(ctx context.Context, org, dealID string) ([]ChecklistItem, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.stage, c.position, c.text, c.done FROM adrient.crm_deal_checklist c
		JOIN adrient.crm_deals d ON d.id=c.deal_id WHERE d.org_id=$1 AND d.id=$2 AND `+ViewerFrom(ctx).ownerSQL("d")+` ORDER BY c.stage, c.position`, org, dealID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChecklistItem{}
	for rows.Next() {
		var c ChecklistItem
		if err := rows.Scan(&c.Stage, &c.Position, &c.Text, &c.Done); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) SetChecklistDone(ctx context.Context, org, dealID, stage string, pos int, done bool) error {
	return r.del(ctx, `UPDATE adrient.crm_deal_checklist c SET done=$5, done_at=CASE WHEN $5 THEN now() END
		FROM adrient.crm_deals d WHERE d.id=c.deal_id AND d.org_id=$1 AND d.id=$2 AND c.stage=$3 AND c.position=$4 AND `+ViewerFrom(ctx).ownerSQL("d"),
		org, dealID, stage, pos, done)
}
