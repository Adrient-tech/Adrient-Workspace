package tickets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- hooks to other modules -------------------------------------------------

// notify delivers an in-app notification through Deps.Notify when it is wired.
func (r *Repository) notify(ctx context.Context, org, user, title, body, ticketID string) {
	if r.Deps.Notify == nil || user == "" {
		return
	}
	if actor := actorOf(ctx).ID; actor == user {
		return // do not notify people about their own actions
	}
	if err := r.Deps.Notify(ctx, org, user, title, body, "/tickets?ticket="+ticketID); err != nil {
		slog.Warn("tickets: notify failed", "err", err)
	}
}

// ---- rules CRUD ---------------------------------------------------------------

// ListAutomations returns a project's automations in order.
func (r *Repository) ListAutomations(ctx context.Context, org, projectID string) ([]Automation, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, project_id::text, name, kind, event, match_mode, conditions, actions, position, enabled, run_count, last_run_at
		FROM adrient.ticket_automations WHERE org_id=$1 AND project_id=$2 ORDER BY position, created_at`, org, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Automation{}
	for rows.Next() {
		var a Automation
		var c, ac []byte
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Kind, &a.Event, &a.MatchMode, &c, &ac, &a.Position, &a.Enabled, &a.RunCount, &a.LastRunAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(c, &a.Conds)
		_ = json.Unmarshal(ac, &a.Actions)
		if a.Conds == nil {
			a.Conds = []Condition{}
		}
		if a.Actions == nil {
			a.Actions = []Action{}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveAutomation validates and stores a rule.
func (r *Repository) SaveAutomation(ctx context.Context, org, user, projectID string, a Automation) (Automation, error) {
	p, err := r.GetProject(ctx, org, projectID)
	if err != nil {
		return a, err
	}
	if err := a.Validate(p.Statuses); err != nil {
		return a, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if a.Conds == nil {
		a.Conds = []Condition{}
	}
	cj, _ := json.Marshal(a.Conds)
	aj, _ := json.Marshal(a.Actions)
	if a.ID == "" {
		var n int
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_automations WHERE project_id=$1`, projectID).Scan(&n)
		if n >= 100 {
			return a, fmt.Errorf("%w: at most 100 automations per project", ErrInvalid)
		}
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_automations (org_id,project_id,name,kind,event,match_mode,conditions,actions,position,enabled,created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`,
			org, projectID, a.Name, a.Kind, a.Event, a.MatchMode, cj, aj, n, a.Enabled, nullUUID(user)).Scan(&a.ID)
		a.Position = n
	} else {
		var id string
		err = r.pool.QueryRow(ctx, `UPDATE adrient.ticket_automations SET name=$4,kind=$5,event=$6,match_mode=$7,conditions=$8,actions=$9,position=$10,enabled=$11
			WHERE id=$1 AND org_id=$2 AND project_id=$3 RETURNING id::text`,
			a.ID, org, projectID, a.Name, a.Kind, a.Event, a.MatchMode, cj, aj, a.Position, a.Enabled).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return a, ErrNotFound
		}
	}
	a.ProjectID = projectID
	return a, err
}

// DeleteAutomation removes a rule.
func (r *Repository) DeleteAutomation(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_automations WHERE org_id=$1 AND id=$2`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ---- running ------------------------------------------------------------------

func autoTicketOf(t DeskTicket) AutoTicket {
	return AutoTicket{Title: t.Title, Body: t.Body, Status: t.Status, Priority: t.Priority, Type: t.Type, Category: t.Category,
		Subcategory: t.Subcategory, Group: t.GroupID, Assignee: t.Assignee, RequesterEmail: t.RequesterEmail, Source: t.Source,
		Urgency: t.Urgency, Impact: t.Impact, Tags: t.Tags, NeedSkills: t.NeedSkills, Major: t.Major, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, Fields: t.Custom}
}

// AutomationTest is the result of a dry run.
type AutomationTest struct {
	Matched     bool         `json:"matched"`
	Conditions  []CondResult `json:"conditions"`
	Changes     []FieldDiff  `json:"changes"`
	Effects     []Effect     `json:"effects"`
	Description string       `json:"description"`
}

// FieldDiff is one field a rule would change.
type FieldDiff struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

func diffAuto(a, b AutoTicket) []FieldDiff {
	var out []FieldDiff
	add := func(f, x, y string) {
		if x != y {
			out = append(out, FieldDiff{f, x, y})
		}
	}
	add("status", a.Status, b.Status)
	add("priority", a.Priority, b.Priority)
	add("type", a.Type, b.Type)
	add("category", a.Category, b.Category)
	add("group", a.Group, b.Group)
	add("assignee", a.Assignee, b.Assignee)
	add("tags", strings.Join(a.Tags, ","), strings.Join(b.Tags, ","))
	add("major", fmt.Sprint(a.Major), fmt.Sprint(b.Major))
	add("skills", strings.Join(a.NeedSkills, ","), strings.Join(b.NeedSkills, ","))
	return out
}

// TestAutomation runs a rule against a real ticket without changing anything.
func (r *Repository) TestAutomation(ctx context.Context, org, ticketID string, a Automation, event string) (AutomationTest, error) {
	t, err := r.GetDesk(ctx, org, ticketID)
	if err != nil {
		return AutomationTest{}, err
	}
	p, err := r.GetProject(ctx, org, t.ProjectID)
	if err != nil {
		return AutomationTest{}, err
	}
	a.Enabled = true
	if err := a.Validate(p.Statuses); err != nil {
		return AutomationTest{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if event == "" {
		event = "create"
		if a.Kind == KindTime {
			event = "any"
		}
	}
	cur := autoTicketOf(t)
	ok, trace := a.Evaluate(cur, event, time.Now())
	out := AutomationTest{Matched: ok, Conditions: trace, Changes: []FieldDiff{}, Effects: []Effect{}, Description: DescribeAutomation(a)}
	if ok {
		next, fx := a.Apply(cur)
		out.Changes = diffAuto(cur, next)
		if out.Changes == nil {
			out.Changes = []FieldDiff{}
		}
		if fx != nil {
			out.Effects = fx
		}
	}
	return out, nil
}

// runAutomations applies the project's trigger and dispatch rules to one ticket.
func (r *Repository) runAutomations(ctx context.Context, org, ticketID, event string) {
	t, err := r.GetDesk(ctx, org, ticketID)
	if err != nil {
		return
	}
	rules, err := r.ListAutomations(ctx, org, t.ProjectID)
	if err != nil || len(rules) == 0 {
		return
	}
	var use []Automation
	for _, a := range rules {
		if a.Enabled && a.Kind != KindTime {
			use = append(use, a)
		}
	}
	if len(use) == 0 {
		return
	}
	r.applyRuleSet(ctx, org, t, use, event)
}

func (r *Repository) applyRuleSet(ctx context.Context, org string, t DeskTicket, rules []Automation, event string) bool {
	now := time.Now()
	cur := autoTicketOf(t)
	next, fired, fx := RunAll(rules, cur, event, now)
	if len(fired) == 0 {
		return false
	}
	changes := diffAuto(cur, next)
	if len(changes) > 0 {
		r.writeAutoState(ctx, org, t, next)
	}
	for _, name := range fired {
		for _, ru := range rules {
			if ru.Name == name {
				_, _ = r.pool.Exec(ctx, `UPDATE adrient.ticket_automations SET run_count=run_count+1, last_run_at=now() WHERE id=$1`, ru.ID)
			}
		}
	}
	r.RecordEvent(WithActor(ctx, Actor{Name: "Automation"}), org, t.ID, "automation", map[string]any{"rules": fired, "changes": changes})
	if next.Group != cur.Group && next.Group != "" && next.Assignee == "" {
		r.routeTicket(ctx, org, t.ID, next.Group, next.NeedSkills)
	}
	for _, e := range fx {
		r.runEffect(ctx, org, t, next, e)
	}
	return true
}

// writeAutoState stores the fields a rule changed, keeping SLA pause state and
// resolved_at correct for status changes.
func (r *Repository) writeAutoState(ctx context.Context, org string, t DeskTicket, n AutoTicket) {
	before, _, _ := r.GetTicket(ctx, org, t.ID)
	_, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET status=$3, priority=$4, ticket_type=$5, category=$6, group_id=$7, assignee_user_id=$8, tags=$9, major=$10, need_skills=$11, updated_at=now()
		WHERE id=$1 AND org_id=$2`, t.ID, org, n.Status, normalizePriority(n.Priority), NormalizeType(n.Type), n.Category, nullUUID(n.Group), nullUUID(n.Assignee), NormalizeTags(n.Tags), n.Major, NormalizeTags(n.NeedSkills))
	if err != nil {
		slog.Warn("tickets: automation write failed", "err", err)
		return
	}
	after, _, err := r.GetTicket(ctx, org, t.ID)
	if err == nil && before.ID != "" {
		r.onStatusChange(ctx, before, after)
		r.slaTransition(ctx, before, after)
		if n.Assignee != t.Assignee {
			r.notify(ctx, org, n.Assignee, "Ticket assigned to you", t.Title, t.ID)
		}
	}
}

func (r *Repository) runEffect(ctx context.Context, org string, t DeskTicket, now AutoTicket, e Effect) {
	switch e.Type {
	case "notify":
		var to []string
		switch e.Value {
		case "assignee":
			to = append(to, now.Assignee)
		case "group":
			if now.Group != "" {
				ms, _ := r.groupMembers(ctx, org, now.Group)
				for _, m := range ms {
					to = append(to, m.UserID)
				}
			}
		default:
			to = append(to, e.Value)
		}
		for _, u := range to {
			if u != "" {
				r.notify(WithActor(ctx, Actor{}), org, u, "Ticket update: "+t.Ref, t.Title, t.ID)
			}
		}
	case "add_note":
		_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.ticket_comments (ticket_id, author_name, body, is_internal) VALUES ($1,'Automation',$2,true)`, t.ID, FillPlaceholders(e.Value, t, "Automation"))
	case "webhook":
		if err := postWebhook(ctx, e.Value, map[string]any{"event": "ticket.automation", "ticket": t.Ref, "title": t.Title,
			"status": now.Status, "priority": now.Priority, "type": now.Type, "assignee": now.Assignee}); err != nil {
			slog.Warn("tickets: webhook failed", "err", err)
			r.RecordEvent(WithActor(ctx, Actor{Name: "Automation"}), org, t.ID, "automation", map[string]any{"webhook_error": err.Error()})
		}
	case "create_task":
		if r.Deps.CreateTask == nil {
			r.RecordEvent(WithActor(ctx, Actor{Name: "Automation"}), org, t.ID, "automation", map[string]any{"skipped": "create_task: tasks module not connected"})
			return
		}
		assignee := now.Assignee
		if assignee == "" && now.Group != "" {
			if ms, _ := r.groupMembers(ctx, org, now.Group); len(ms) > 0 {
				assignee = ms[0].UserID
			}
		}
		if assignee == "" {
			_ = r.pool.QueryRow(ctx, `SELECT COALESCE(created_by::text,'') FROM adrient.ticket_projects WHERE id=$1`, t.ProjectID).Scan(&assignee)
		}
		if err := r.Deps.CreateTask(ctx, org, assignee, e.Value+" ("+t.Ref+")", t.Title, "/tickets?ticket="+t.ID); err != nil {
			slog.Warn("tickets: create task failed", "err", err)
			r.RecordEvent(WithActor(ctx, Actor{Name: "Automation"}), org, t.ID, "automation", map[string]any{"skipped": "create_task: " + err.Error()})
			return
		} else {
			r.RecordEvent(WithActor(ctx, Actor{Name: "Automation"}), org, t.ID, "automation", map[string]any{"task": e.Value})
		}
	}
}

// postWebhook sends a JSON POST to a public http(s) address. Private, loopback and
// link-local targets are refused, redirects are not followed.
func postWebhook(ctx context.Context, raw string, payload map[string]any) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return errors.New("webhook URL must be http(s)")
	}
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	client := &http.Client{
		Timeout:       6 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if blockedIP(ip.IP) {
					return nil, errors.New("webhook target is not a public address")
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, raw, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook answered %d", resp.StatusCode)
	}
	return nil
}

func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast()
}

// SweepResult counts what a sweep did.
type SweepResult struct {
	Woken        int `json:"woken"`
	RuleHits     int `json:"rule_hits"`
	Evaluated    int `json:"evaluated"`
	MailTickets  int `json:"mail_tickets"`
	MailComments int `json:"mail_comments"`
	Scheduled    int `json:"scheduled_sent"`
}

// Sweep runs the scheduled work: wakes snoozed tickets and applies time-based
// automations (once per ticket and rule). Call it every few minutes.
func (r *Repository) Sweep(ctx context.Context, now time.Time) (SweepResult, error) {
	var res SweepResult
	rows, err := r.pool.Query(ctx, `UPDATE adrient.tickets SET snoozed_until=NULL, updated_at=now() WHERE snoozed_until IS NOT NULL AND snoozed_until <= $1
		RETURNING id::text, org_id::text, COALESCE(assignee_user_id::text,''), title`, now)
	if err != nil {
		return res, err
	}
	type woke struct{ id, org, assignee, title string }
	var wk []woke
	for rows.Next() {
		var w woke
		if rows.Scan(&w.id, &w.org, &w.assignee, &w.title) == nil {
			wk = append(wk, w)
		}
	}
	rows.Close()
	for _, w := range wk {
		res.Woken++
		r.RecordEvent(WithActor(ctx, Actor{Name: "System"}), w.org, w.id, "snooze", map[string]any{"until": nil})
		r.notify(WithActor(ctx, Actor{}), w.org, w.assignee, "Snoozed ticket is back", w.title, w.id)
	}
	if n, c, err := r.PollInbound(ctx); err == nil {
		res.MailTickets, res.MailComments = n, c
	}
	res.Scheduled = r.sendDueScheduled(ctx)
	ar, err := r.pool.Query(ctx, `SELECT org_id::text, project_id::text FROM adrient.ticket_automations WHERE kind='time' AND enabled GROUP BY org_id, project_id`)
	if err != nil {
		return res, err
	}
	type pj struct{ org, project string }
	var pjs []pj
	for ar.Next() {
		var p pj
		if ar.Scan(&p.org, &p.project) == nil {
			pjs = append(pjs, p)
		}
	}
	ar.Close()
	for _, p := range pjs {
		rules, err := r.ListAutomations(ctx, p.org, p.project)
		if err != nil {
			continue
		}
		for _, a := range rules {
			if !a.Enabled || a.Kind != KindTime {
				continue
			}
			tr, err := r.pool.Query(ctx, `SELECT `+deskCols+` FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id=t.project_id
				WHERE t.org_id=$1 AND t.project_id=$2 AND t.merged_into IS NULL
				  AND NOT EXISTS (SELECT 1 FROM adrient.ticket_automation_runs ar WHERE ar.automation_id=$3 AND ar.ticket_id=t.id)
				ORDER BY t.updated_at LIMIT 500`, p.org, p.project, a.ID)
			if err != nil {
				continue
			}
			var cands []DeskTicket
			for tr.Next() {
				if t, err := scanDesk(tr); err == nil {
					cands = append(cands, t)
				}
			}
			tr.Close()
			for _, t := range cands {
				res.Evaluated++
				if ok, _ := a.Evaluate(autoTicketOf(t), "any", now); !ok {
					continue
				}
				if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.ticket_automation_runs (automation_id, ticket_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, a.ID, t.ID); err != nil {
					continue
				}
				if r.applyRuleSet(ctx, p.org, t, []Automation{a}, "any") {
					res.RuleHits++
				}
			}
		}
	}
	return res, nil
}

// RunSweeper calls Sweep on an interval until ctx ends. Errors are logged, never fatal.
func (r *Repository) RunSweeper(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := r.Sweep(ctx, time.Now()); err != nil {
				slog.Warn("tickets: sweep failed", "err", err)
			}
		}
	}
}
