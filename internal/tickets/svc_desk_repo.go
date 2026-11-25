package tickets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- creation options and hooks ------------------------------------------------

// CreateOpts carries desk fields into CreateTicket through the context.
type CreateOpts struct {
	GroupID, Type, Category, Subcategory, Urgency, Impact, FormID string
	Custom                                                        map[string]string
	NeedSkills                                                    []string
}

type createOptsKey struct{}

// WithCreateOpts attaches desk fields to the context of a CreateTicket call.
func WithCreateOpts(ctx context.Context, o CreateOpts) context.Context {
	return context.WithValue(ctx, createOptsKey{}, o)
}

// afterCreate sets desk fields, derives priority from urgency and impact, routes the
// ticket and runs the dispatch and trigger automations.
func (r *Repository) afterCreate(ctx context.Context, t *Ticket) {
	o, _ := ctx.Value(createOptsKey{}).(CreateOpts)
	if len(o.NeedSkills) > 0 || o.Type != "" || o.GroupID != "" || o.Category != "" || o.FormID != "" || o.Urgency != "" || o.Impact != "" || len(o.Custom) > 0 {
		custom, _ := json.Marshal(o.Custom)
		if o.Custom == nil {
			custom = []byte("{}")
		}
		urg, imp := normLevel(o.Urgency), normLevel(o.Impact)
		pri := t.Priority
		if o.Urgency != "" || o.Impact != "" {
			pri = PriorityFromMatrix(urg, imp)
		}
		var grp any
		if o.GroupID != "" {
			var n int
			_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_groups WHERE id=$1 AND org_id=$2 AND project_id=$3`, o.GroupID, t.OrgID, t.ProjectID).Scan(&n)
			if n > 0 {
				grp = o.GroupID
			}
		}
		_, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET ticket_type=COALESCE(NULLIF($2,''),ticket_type), group_id=$3, category=$4, subcategory=$5,
			urgency=$6, impact=$7, priority=$8, custom=$9, form_id=$10 WHERE id=$1`,
			t.ID, NormalizeType(o.Type), grp, strings.TrimSpace(o.Category), strings.TrimSpace(o.Subcategory), urg, imp, pri, custom, nullUUID(o.FormID))
		if err == nil {
			t.Priority = pri
		}
		if len(o.NeedSkills) > 0 {
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.tickets SET need_skills=$2 WHERE id=$1`, t.ID, NormalizeTags(o.NeedSkills))
		}
		if grp != nil {
			r.routeTicket(ctx, t.OrgID, t.ID, o.GroupID, o.NeedSkills)
		}
	}
	if t.Source == "public" || t.Source == "portal" || t.Source == "email" {
		_ = r.PortalToken(ctx, t.ID)
	}
	r.runAutomations(ctx, t.OrgID, t.ID, "create")
	var assignee *string
	var status, priority string
	if err := r.pool.QueryRow(ctx, `SELECT assignee_user_id::text, status, priority FROM adrient.tickets WHERE id=$1`, t.ID).Scan(&assignee, &status, &priority); err == nil {
		t.AssigneeUserID, t.Status, t.Priority = assignee, status, priority
	}
}

func normLevel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return "low"
	case "high":
		return "high"
	}
	return "medium"
}

var mentionRe = regexp.MustCompile(`\(user:([0-9a-fA-F-]{36})\)`)

// afterComment records the first response, reopens solved tickets on a requester
// reply and notifies mentioned teammates and the assignee.
func (r *Repository) afterComment(ctx context.Context, org string, ticketID string, authorUserID *string, c Comment) {
	t, err := r.GetDesk(ctx, org, ticketID)
	if err != nil {
		return
	}
	author := ""
	if authorUserID != nil {
		author = *authorUserID
	}
	var requester string
	_ = r.pool.QueryRow(ctx, `SELECT COALESCE(requester_user_id::text,'') FROM adrient.tickets WHERE id=$1`, ticketID).Scan(&requester)
	fromRequester := !c.IsInternal && (author == "" || author == requester)
	if !c.IsInternal && !fromRequester {
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.tickets SET first_response_at=now() WHERE id=$1 AND first_response_at IS NULL`, ticketID)
		r.mailRequesterReply(ctx, t, c.AuthorName, c.Body)
	}
	if fromRequester {
		defs, _ := r.StatusDefs(ctx, org, t.ProjectID)
		cat := CategoryOf(defs, t.Status)
		if ShouldReopen(defs, t.Status, true) || cat == "pending" {
			open := "open"
			for _, d := range defs {
				if d.Category == "open" {
					open = d.Key
					break
				}
			}
			if _, err := r.UpdateTicket(WithActor(ctx, Actor{Name: "Requester reply"}), org, ticketID, &open, nil, nil, nil, nil, false); err == nil {
				r.RecordEvent(WithActor(ctx, Actor{Name: "System"}), org, ticketID, "reopen", map[string]any{"from": t.Status, "to": open})
			}
		}
		r.notify(ctx, org, t.Assignee, "Requester replied on "+t.Ref, t.Title, ticketID)
	}
	for _, m := range mentionRe.FindAllStringSubmatch(c.Body, 10) {
		if ok, _ := r.memberOf(ctx, org, m[1]); ok {
			r.notify(ctx, org, m[1], "You were mentioned on "+t.Ref, t.Title, ticketID)
		}
	}
	_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.ticket_drafts WHERE ticket_id=$1 AND user_id=$2`, ticketID, nullUUIDv(author))
}

func nullUUIDv(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ---- presence ---------------------------------------------------------------------

// Presence is one teammate looking at, or replying to, a ticket.
type Presence struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Mode   string `json:"mode"`
}

// Heartbeat records that a user has the ticket open and returns the others active in the last 30 seconds.
func (r *Repository) Heartbeat(ctx context.Context, org, ticketID, user, name, mode string) ([]Presence, error) {
	if mode != "replying" {
		mode = "viewing"
	}
	if _, err := r.GetDesk(ctx, org, ticketID); err != nil {
		return nil, err
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.ticket_presence (ticket_id,user_id,org_id,name,mode,seen_at) VALUES ($1,$2,$3,$4,$5,now())
		ON CONFLICT (ticket_id,user_id) DO UPDATE SET mode=$5, name=$4, seen_at=now()`, ticketID, user, org, name, mode); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT user_id::text, name, mode FROM adrient.ticket_presence WHERE ticket_id=$1 AND org_id=$2 AND user_id<>$3 AND seen_at > now() - interval '30 seconds' ORDER BY seen_at`, ticketID, org, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Presence{}
	for rows.Next() {
		var p Presence
		if err := rows.Scan(&p.UserID, &p.Name, &p.Mode); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- drafts -----------------------------------------------------------------------

// Draft is an unsent reply.
type Draft struct {
	Body      string    `json:"body"`
	Internal  bool      `json:"internal"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetDraft returns the caller's draft for a ticket (empty when none).
func (r *Repository) GetDraft(ctx context.Context, org, ticketID, user string) (Draft, error) {
	var d Draft
	err := r.pool.QueryRow(ctx, `SELECT body, internal, updated_at FROM adrient.ticket_drafts WHERE ticket_id=$1 AND user_id=$2 AND org_id=$3`, ticketID, user, org).Scan(&d.Body, &d.Internal, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Draft{}, nil
	}
	return d, err
}

// SaveDraft stores (or, for an empty body, deletes) the caller's draft.
func (r *Repository) SaveDraft(ctx context.Context, org, ticketID, user string, d Draft) error {
	if len(d.Body) > 50000 {
		return fmt.Errorf("%w: draft is too long", ErrInvalid)
	}
	if _, err := r.GetDesk(ctx, org, ticketID); err != nil {
		return err
	}
	if strings.TrimSpace(d.Body) == "" {
		_, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_drafts WHERE ticket_id=$1 AND user_id=$2`, ticketID, user)
		return err
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.ticket_drafts (ticket_id,user_id,org_id,body,internal,updated_at) VALUES ($1,$2,$3,$4,$5,now())
		ON CONFLICT (ticket_id,user_id) DO UPDATE SET body=$4, internal=$5, updated_at=now()`, ticketID, user, org, d.Body, d.Internal)
	return err
}

// ---- time entries -----------------------------------------------------------------

// TimeEntry is worked time on a ticket.
type TimeEntry struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	Minutes  int    `json:"minutes"`
	Note     string `json:"note"`
	Billable bool   `json:"billable"`
	Date     string `json:"date"`
}

// AddTime logs time on a ticket and forwards it to Time tracking when that module is connected.
func (r *Repository) AddTime(ctx context.Context, org, user, userName, ticketID string, e TimeEntry) (TimeEntry, error) {
	t, err := r.GetDesk(ctx, org, ticketID)
	if err != nil {
		return e, err
	}
	if e.Minutes < 1 || e.Minutes > 1440 {
		return e, fmt.Errorf("%w: minutes must be between 1 and 1440", ErrInvalid)
	}
	if e.Date == "" {
		e.Date = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", e.Date); err != nil {
		return e, fmt.Errorf("%w: date must be YYYY-MM-DD", ErrInvalid)
	}
	e.Note = strings.TrimSpace(e.Note)
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_time_entries (org_id,ticket_id,user_id,user_name,minutes,note,billable,work_date) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id::text`, org, ticketID, user, userName, e.Minutes, e.Note, e.Billable, e.Date).Scan(&e.ID)
	if err != nil {
		return e, err
	}
	e.UserID, e.UserName = user, userName
	r.RecordEvent(ctx, org, ticketID, "time", map[string]any{"minutes": e.Minutes, "billable": e.Billable})
	if r.Deps.LogTime != nil {
		if err := r.Deps.LogTime(ctx, org, user, e.Minutes, e.Date, t.Ref+" "+t.Title+": "+e.Note, e.Billable); err != nil {
			r.RecordEvent(ctx, org, ticketID, "time", map[string]any{"time_tracking": "not linked: " + err.Error()})
		}
	}
	return e, nil
}

// ListTime returns a ticket's time entries and the total.
func (r *Repository) ListTime(ctx context.Context, org, ticketID string) ([]TimeEntry, int, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, user_id::text, user_name, minutes, note, billable, to_char(work_date,'YYYY-MM-DD') FROM adrient.ticket_time_entries
		WHERE org_id=$1 AND ticket_id=$2 ORDER BY work_date DESC, created_at DESC LIMIT 200`, org, ticketID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []TimeEntry{}
	total := 0
	for rows.Next() {
		var e TimeEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.UserName, &e.Minutes, &e.Note, &e.Billable, &e.Date); err != nil {
			return nil, 0, err
		}
		total += e.Minutes
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// DeleteTime removes an entry; only its author (or an admin) may.
func (r *Repository) DeleteTime(ctx context.Context, org, user string, admin bool, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_time_entries WHERE org_id=$1 AND id=$2 AND ($4 OR user_id::text=$3)`, org, id, user, admin)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ---- snooze, parent, major incident -----------------------------------------------

// Snooze hides a ticket from open views until the given time; nil wakes it now.
func (r *Repository) Snooze(ctx context.Context, org, id string, until *time.Time) error {
	if until != nil && (!until.After(time.Now()) || until.After(time.Now().AddDate(0, 3, 0))) {
		return fmt.Errorf("%w: snooze until a time within the next 3 months", ErrInvalid)
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET snoozed_until=$3 WHERE id=$1 AND org_id=$2`, id, org, until)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	r.RecordEvent(ctx, org, id, "snooze", map[string]any{"until": until})
	return nil
}

// SetParent links a ticket under a parent (one level deep).
func (r *Repository) SetParent(ctx context.Context, org, id, parentID string) error {
	if parentID == id {
		return fmt.Errorf("%w: a ticket cannot be its own parent", ErrInvalid)
	}
	if _, err := r.GetDesk(ctx, org, id); err != nil {
		return err
	}
	if parentID != "" {
		p, err := r.GetDesk(ctx, org, parentID)
		if err != nil {
			return err
		}
		if p.ParentID != "" {
			return fmt.Errorf("%w: the chosen parent is itself a child ticket", ErrInvalid)
		}
		var kids int
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.tickets WHERE parent_id=$1`, id).Scan(&kids)
		if kids > 0 {
			return fmt.Errorf("%w: this ticket has children, so it cannot become a child", ErrInvalid)
		}
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET parent_id=$3, updated_at=now() WHERE id=$1 AND org_id=$2`, id, org, nullUUID(parentID)); err != nil {
		return err
	}
	r.RecordEvent(ctx, org, id, "parent", map[string]any{"to": parentID})
	return nil
}

// Children lists a ticket's child tickets.
func (r *Repository) Children(ctx context.Context, org, id string) ([]DeskTicket, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+deskCols+` FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id=t.project_id
		WHERE t.org_id=$1 AND t.parent_id=$2 ORDER BY t.number LIMIT 100`, org, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeskTicket{}
	for rows.Next() {
		t, err := scanDesk(rows)
		if err != nil {
			return nil, err
		}
		t.Body = ""
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetMajor flags or clears a major incident and notifies the group.
func (r *Repository) SetMajor(ctx context.Context, org, id string, on bool) error {
	t, err := r.GetDesk(ctx, org, id)
	if err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET major=$3, updated_at=now() WHERE id=$1 AND org_id=$2`, id, org, on); err != nil {
		return err
	}
	r.RecordEvent(ctx, org, id, "major", map[string]any{"on": on})
	if on && t.GroupID != "" {
		ms, _ := r.groupMembers(ctx, org, t.GroupID)
		for _, m := range ms {
			r.notify(ctx, org, m.UserID, "Major incident: "+t.Ref, t.Title, id)
		}
	}
	return nil
}

// MajorIncidents lists open major incidents of the org for the banner.
func (r *Repository) MajorIncidents(ctx context.Context, org string) ([]DeskTicket, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+deskCols+` FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id=t.project_id
		WHERE t.org_id=$1 AND t.major AND `+notDoneSQL("t")+` ORDER BY t.created_at DESC LIMIT 5`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeskTicket{}
	for rows.Next() {
		t, err := scanDesk(rows)
		if err != nil {
			return nil, err
		}
		t.Body = ""
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetFields patches category, subcategory, urgency, impact, group and custom fields.
type FieldsPatch struct {
	Category    *string            `json:"category"`
	Subcategory *string            `json:"subcategory"`
	Urgency     *string            `json:"urgency"`
	Impact      *string            `json:"impact"`
	GroupID     *string            `json:"group_id"`
	Custom      *map[string]string `json:"custom"`
}

// SetFields changes the service-management fields of a ticket. Urgency and impact
// recompute the priority through the matrix.
func (r *Repository) SetFields(ctx context.Context, org, id string, f FieldsPatch) (DeskTicket, error) {
	cur, err := r.GetDesk(ctx, org, id)
	if err != nil {
		return cur, err
	}
	if f.Category != nil {
		cur.Category = strings.TrimSpace(*f.Category)
	}
	if f.Subcategory != nil {
		cur.Subcategory = strings.TrimSpace(*f.Subcategory)
	}
	if len(cur.Category) > 80 || len(cur.Subcategory) > 80 {
		return cur, fmt.Errorf("%w: category is too long", ErrInvalid)
	}
	oldPri := cur.Priority
	if f.Urgency != nil {
		cur.Urgency = normLevel(*f.Urgency)
	}
	if f.Impact != nil {
		cur.Impact = normLevel(*f.Impact)
	}
	if f.Urgency != nil || f.Impact != nil {
		cur.Priority = PriorityFromMatrix(cur.Urgency, cur.Impact)
	}
	if f.Custom != nil {
		if len(*f.Custom) > 40 {
			return cur, fmt.Errorf("%w: too many custom fields", ErrInvalid)
		}
		defs, err := r.ListFieldDefs(ctx, org, cur.ProjectID)
		if err != nil {
			return cur, err
		}
		clean, err := ValidateCustom(defs, cur.Type, *f.Custom)
		if err != nil {
			return cur, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		cur.Custom = clean
	}
	groupChanged := false
	if f.GroupID != nil {
		g := strings.TrimSpace(*f.GroupID)
		if g != "" {
			var n int
			_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_groups WHERE id=$1 AND org_id=$2 AND project_id=$3`, g, org, cur.ProjectID).Scan(&n)
			if n == 0 {
				return cur, fmt.Errorf("%w: group not found", ErrInvalid)
			}
		}
		groupChanged = g != cur.GroupID
		cur.GroupID = g
	}
	cj, _ := json.Marshal(cur.Custom)
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET category=$3, subcategory=$4, urgency=$5, impact=$6, priority=$7, group_id=$8, custom=$9, updated_at=now() WHERE id=$1 AND org_id=$2`,
		id, org, cur.Category, cur.Subcategory, cur.Urgency, cur.Impact, cur.Priority, nullUUID(cur.GroupID), cj); err != nil {
		return cur, err
	}
	if cur.Priority != oldPri {
		r.RecordEvent(ctx, org, id, "priority", map[string]any{"from": oldPri, "to": cur.Priority, "via": "matrix"})
	}
	r.RecordEvent(ctx, org, id, "fields", map[string]any{"category": cur.Category, "group": cur.GroupID})
	if groupChanged && cur.GroupID != "" && cur.Assignee == "" {
		r.routeTicket(ctx, org, id, cur.GroupID, nil)
	}
	return r.GetDesk(ctx, org, id)
}

// ---- approvals (change requests) -------------------------------------------------

// Approval is one approver's decision on a ticket.
type Approval struct {
	ID         string     `json:"id"`
	Step       int        `json:"step"`
	ApproverID string     `json:"approver_id"`
	Approver   string     `json:"approver"`
	Decision   string     `json:"decision"`
	Comment    string     `json:"comment"`
	DecidedAt  *time.Time `json:"decided_at,omitempty"`
}

// RequestApprovals asks approvers to sign off on a ticket. Steps run in order.
func (r *Repository) RequestApprovals(ctx context.Context, org, ticketID string, steps [][]string) error {
	t, err := r.GetDesk(ctx, org, ticketID)
	if err != nil {
		return err
	}
	if len(steps) == 0 || len(steps) > 5 {
		return fmt.Errorf("%w: use between 1 and 5 approval steps", ErrInvalid)
	}
	var existing int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_approvals WHERE ticket_id=$1 AND decision='pending'`, ticketID).Scan(&existing)
	if existing > 0 {
		return fmt.Errorf("%w: approvals are already pending on this ticket", ErrInvalid)
	}
	_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.ticket_approvals WHERE ticket_id=$1`, ticketID)
	for i, ids := range steps {
		if len(ids) == 0 || len(ids) > 10 {
			return fmt.Errorf("%w: each step needs 1 to 10 approvers", ErrInvalid)
		}
		for _, u := range ids {
			if ok, _ := r.memberOf(ctx, org, u); !ok {
				return fmt.Errorf("%w: approver is not a member of this organisation", ErrInvalid)
			}
			if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.ticket_approvals (org_id,ticket_id,step,approver_id) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, org, ticketID, i+1, u); err != nil {
				return err
			}
		}
	}
	r.RecordEvent(ctx, org, ticketID, "approval", map[string]any{"requested_steps": len(steps)})
	for _, u := range steps[0] {
		r.notify(ctx, org, u, "Approval needed: "+t.Ref, t.Title, ticketID)
	}
	return nil
}

// Approvals lists a ticket's approvals and the overall state (none, pending, approved, rejected).
func (r *Repository) Approvals(ctx context.Context, org, ticketID string) ([]Approval, string, error) {
	rows, err := r.pool.Query(ctx, `SELECT a.id::text, a.step, a.approver_id::text, COALESCE(u.display_name,''), a.decision, a.comment, a.decided_at
		FROM adrient.ticket_approvals a LEFT JOIN adrient.users u ON u.id=a.approver_id WHERE a.org_id=$1 AND a.ticket_id=$2 ORDER BY a.step, u.display_name`, org, ticketID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Approval{}
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.Step, &a.ApproverID, &a.Approver, &a.Decision, &a.Comment, &a.DecidedAt); err != nil {
			return nil, "", err
		}
		out = append(out, a)
	}
	return out, ApprovalState(out), rows.Err()
}

// ApprovalState folds approvals into one state: any rejection rejects, all approved approves.
func ApprovalState(list []Approval) string {
	if len(list) == 0 {
		return "none"
	}
	pending := false
	for _, a := range list {
		switch a.Decision {
		case "rejected":
			return "rejected"
		case "pending":
			pending = true
		}
	}
	if pending {
		return "pending"
	}
	return "approved"
}

// Decide records an approver's decision. Only approvers of the current (lowest pending) step may decide.
func (r *Repository) Decide(ctx context.Context, org, ticketID, user, decision, comment string) error {
	if decision != "approved" && decision != "rejected" {
		return fmt.Errorf("%w: decision must be approved or rejected", ErrInvalid)
	}
	list, _, err := r.Approvals(ctx, org, ticketID)
	if err != nil {
		return err
	}
	cur := 0
	for _, a := range list {
		if a.Decision == "pending" {
			cur = a.Step
			break
		}
	}
	if cur == 0 {
		return fmt.Errorf("%w: nothing is waiting for approval", ErrInvalid)
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.ticket_approvals SET decision=$5, comment=$6, decided_at=now()
		WHERE org_id=$1 AND ticket_id=$2 AND approver_id=$3 AND step=$4 AND decision='pending'`, org, ticketID, user, cur, decision, strings.TrimSpace(comment))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: you are not an approver for the current step", ErrInvalid)
	}
	r.RecordEvent(ctx, org, ticketID, "approval", map[string]any{"decision": decision, "step": cur})
	after, state, _ := r.Approvals(ctx, org, ticketID)
	t, _ := r.GetDesk(ctx, org, ticketID)
	switch state {
	case "rejected", "approved":
		r.notify(ctx, org, t.Assignee, "Change "+state+": "+t.Ref, t.Title, ticketID)
	case "pending":
		next := 0
		for _, a := range after {
			if a.Decision == "pending" {
				next = a.Step
				break
			}
		}
		if next > cur {
			for _, a := range after {
				if a.Step == next {
					r.notify(ctx, org, a.ApproverID, "Approval needed: "+t.Ref, t.Title, ticketID)
				}
			}
		}
	}
	return nil
}

// MyApprovals lists the pending approvals waiting on a user (current step only).
func (r *Repository) MyApprovals(ctx context.Context, org, user string) ([]DeskTicket, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+deskCols+` FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id=t.project_id
		JOIN adrient.ticket_approvals a ON a.ticket_id=t.id AND a.approver_id=$2 AND a.decision='pending'
		WHERE t.org_id=$1 AND a.step = (SELECT min(step) FROM adrient.ticket_approvals x WHERE x.ticket_id=t.id AND x.decision='pending')
		ORDER BY t.created_at DESC LIMIT 50`, org, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeskTicket{}
	for rows.Next() {
		t, err := scanDesk(rows)
		if err != nil {
			return nil, err
		}
		t.Body = ""
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---- paginated list ---------------------------------------------------------------

// ListOpts selects one page of a project's tickets.
type ListOpts struct {
	ProjectID string
	View      string
	Q         string
	Group     string
	Assignee  string
	Limit     int
	Cursor    string
	Me        string
}

// ListView names the SQL-backed views, in display order.
var ListViews = []struct{ ID, Title string }{
	{"mine", "Assigned to me"}, {"unassigned", "Unassigned"}, {"urgent", "Urgent and high"}, {"all_open", "All unsolved"},
	{"stale", "No update for 3 days"}, {"incidents", "Open incidents"}, {"major", "Major incidents"},
	{"snoozed", "Snoozed"}, {"solved_recent", "Solved in the last 7 days"}, {"all", "Everything"},
}

func viewSQL(view string) (string, bool) {
	nd := notDoneSQL("t")
	awake := "(t.snoozed_until IS NULL OR t.snoozed_until <= now())"
	switch view {
	case "mine":
		return nd + " AND " + awake + " AND t.assignee_user_id::text = $ME", true
	case "unassigned":
		return nd + " AND " + awake + " AND t.assignee_user_id IS NULL", true
	case "urgent":
		return nd + " AND " + awake + " AND t.priority IN ('urgent','high')", true
	case "all_open":
		return nd + " AND " + awake, true
	case "stale":
		return nd + " AND " + awake + " AND t.updated_at < now() - interval '72 hours'", true
	case "incidents":
		return nd + " AND " + awake + " AND t.ticket_type IN ('incident','problem')", true
	case "major":
		return nd + " AND t.major", true
	case "snoozed":
		return "t.snoozed_until > now()", true
	case "solved_recent":
		return "NOT " + nd + " AND t.resolved_at > now() - interval '7 days'", true
	case "all", "":
		return "TRUE", true
	}
	return "", false
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// buildWhere turns a search string into SQL conditions with numbered arguments.
func buildWhere(q string, args *[]any) []string {
	var conds []string
	arg := func(v any) string { *args = append(*args, v); return fmt.Sprintf("$%d", len(*args)) }
	pq := ParseSearch(q)
	if pq.Number > 0 {
		conds = append(conds, "t.number = "+arg(pq.Number))
	}
	for _, f := range pq.Filters {
		v := strings.ToLower(f.Value)
		switch f.Key {
		case "status":
			switch v {
			case "unsolved", "active":
				conds = append(conds, notDoneSQL("t"))
			case "solved":
				conds = append(conds, "NOT "+notDoneSQL("t"))
			default:
				conds = append(conds, "t.status = "+arg(v))
			}
		case "assignee":
			switch v {
			case "none", "unassigned":
				conds = append(conds, "t.assignee_user_id IS NULL")
			case "me":
				conds = append(conds, "t.assignee_user_id::text = $ME")
			default:
				conds = append(conds, "(t.assignee_user_id::text LIKE "+arg(likeEscape(v)+"%")+" OR EXISTS (SELECT 1 FROM adrient.users u WHERE u.id=t.assignee_user_id AND u.display_name ILIKE "+arg("%"+likeEscape(v)+"%")+"))")
			}
		case "priority":
			conds = append(conds, "t.priority = "+arg(v))
		case "type":
			conds = append(conds, "t.ticket_type = "+arg(v))
		case "via":
			conds = append(conds, "lower(t.source) = "+arg(v))
		case "tag":
			conds = append(conds, arg(strings.Join(strings.Fields(v), "-"))+" = ANY(t.tags)")
		case "requester":
			p := arg("%" + likeEscape(v) + "%")
			conds = append(conds, "(t.requester_name ILIKE "+p+" OR t.requester_email ILIKE "+p+")")
		}
	}
	for _, w := range pq.Words {
		p := arg("%" + likeEscape(w) + "%")
		conds = append(conds, "(t.title ILIKE "+p+" OR t.body ILIKE "+p+" OR t.requester_name ILIKE "+p+" OR t.requester_email ILIKE "+p+
			" OR t.category ILIKE "+p+" OR (p.key || '-' || t.number::text) ILIKE "+p+")")
	}
	return conds
}

// ListPage returns one page of tickets (newest activity first) and the cursor of the next page.
func (r *Repository) ListPage(ctx context.Context, org string, o ListOpts) ([]DeskTicket, string, error) {
	if o.Limit < 1 || o.Limit > 100 {
		o.Limit = 50
	}
	vsql, ok := viewSQL(o.View)
	if !ok {
		return nil, "", fmt.Errorf("%w: unknown view", ErrInvalid)
	}
	args := []any{org, o.ProjectID, o.Me}
	where := []string{"t.org_id=$1", "t.project_id=$2", "t.merged_into IS NULL", "($3::text IS NOT NULL OR TRUE)", vsql}
	where = append(where, buildWhere(o.Q, &args)...)
	if o.Group == "none" {
		where = append(where, "t.group_id IS NULL")
	} else if o.Group != "" {
		args = append(args, o.Group)
		where = append(where, fmt.Sprintf("t.group_id::text = $%d", len(args)))
	}
	if o.Assignee != "" {
		args = append(args, o.Assignee)
		where = append(where, fmt.Sprintf("t.assignee_user_id::text = $%d", len(args)))
	}
	if o.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(o.Cursor)
		parts := strings.SplitN(string(raw), "|", 2)
		if err != nil || len(parts) != 2 {
			return nil, "", fmt.Errorf("%w: bad cursor", ErrInvalid)
		}
		ts, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			return nil, "", fmt.Errorf("%w: bad cursor", ErrInvalid)
		}
		args = append(args, ts, parts[1])
		where = append(where, fmt.Sprintf("(t.updated_at, t.id) < ($%d, $%d::uuid)", len(args)-1, len(args)))
	}
	sqlText := strings.ReplaceAll(`SELECT `+deskCols+` FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id=t.project_id WHERE `+strings.Join(where, " AND ")+
		fmt.Sprintf(` ORDER BY t.updated_at DESC, t.id DESC LIMIT %d`, o.Limit+1), "$ME", "$3")
	rows, err := r.pool.Query(ctx, sqlText, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []DeskTicket{}
	for rows.Next() {
		t, err := scanDesk(rows)
		if err != nil {
			return nil, "", err
		}
		t.Body = ""
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > o.Limit {
		out = out[:o.Limit]
		last := out[len(out)-1]
		next = base64.RawURLEncoding.EncodeToString([]byte(last.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID))
	}
	return out, next, nil
}

// ViewCountsSQL counts every list view of a project in one query.
func (r *Repository) ViewCountsSQL(ctx context.Context, org, projectID, me string) ([]View, error) {
	var sel []string
	for _, v := range ListViews {
		if v.ID == "all" {
			continue
		}
		cond, _ := viewSQL(v.ID)
		sel = append(sel, fmt.Sprintf("count(*) FILTER (WHERE %s)", strings.ReplaceAll(cond, "$ME", "$3")))
	}
	row := r.pool.QueryRow(ctx, `SELECT `+strings.Join(sel, ", ")+` FROM adrient.tickets t WHERE t.org_id=$1 AND t.project_id=$2 AND t.merged_into IS NULL`, org, projectID, me)
	counts := make([]int, len(sel))
	ptrs := make([]any, len(sel))
	for i := range counts {
		ptrs[i] = &counts[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		return nil, err
	}
	out := []View{}
	i := 0
	for _, v := range ListViews {
		if v.ID == "all" {
			continue
		}
		out = append(out, View{ID: v.ID, Title: v.Title, Count: counts[i]})
		i++
	}
	return out, nil
}
