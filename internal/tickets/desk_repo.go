package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type actorKey struct{}

// Actor is who performed a change; it is stamped on the event trail.
type Actor struct{ ID, Name string }

// WithActor stores the acting user on the context so UpdateTicket can record events.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

func actorOf(ctx context.Context) Actor {
	a, _ := ctx.Value(actorKey{}).(Actor)
	return a
}

// Event is one line of a ticket's history.
type Event struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	ActorName string          `json:"actor_name"`
	Detail    json.RawMessage `json:"detail"`
	CreatedAt time.Time       `json:"created_at"`
}

// RecordEvent appends to the trail. Failures are ignored: history must never block a change.
func (r *Repository) RecordEvent(ctx context.Context, org, ticketID, kind string, detail map[string]any) {
	a := actorOf(ctx)
	b, _ := json.Marshal(detail)
	_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.ticket_events (org_id, ticket_id, actor_id, actor_name, kind, detail)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb)`, org, ticketID, nullUUID(a.ID), a.Name, kind, string(b))
}

// recordChanges logs what UpdateTicket changed.
func (r *Repository) recordChanges(ctx context.Context, before, after Ticket) {
	if before.Status != after.Status {
		r.RecordEvent(ctx, after.OrgID, after.ID, "status", map[string]any{"from": before.Status, "to": after.Status})
	}
	if before.Priority != after.Priority {
		r.RecordEvent(ctx, after.OrgID, after.ID, "priority", map[string]any{"from": before.Priority, "to": after.Priority})
	}
	if deref(before.AssigneeUserID) != deref(after.AssigneeUserID) {
		r.RecordEvent(ctx, after.OrgID, after.ID, "assignee", map[string]any{"from": deref(before.AssigneeUserID), "to": deref(after.AssigneeUserID)})
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Events returns the trail of a ticket, oldest first.
func (r *Repository) Events(ctx context.Context, org, ticketID string) ([]Event, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, kind, actor_name, detail, created_at FROM adrient.ticket_events
		WHERE org_id=$1 AND ticket_id=$2 ORDER BY created_at, id LIMIT 500`, org, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var d []byte
		if err := rows.Scan(&e.ID, &e.Kind, &e.ActorName, &d, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Detail = d
		out = append(out, e)
	}
	return out, rows.Err()
}

const deskCols = `t.id::text, t.project_id::text, p.key, t.number, t.title, t.body, t.status, t.priority, t.ticket_type, t.tags,
	t.requester_name, t.requester_email, COALESCE(t.assignee_user_id::text,''), t.source, t.created_at, t.updated_at, t.resolved_at,
	COALESCE(t.merged_into::text,''), COALESCE(t.problem_id::text,''),
	COALESCE(t.group_id::text,''), t.urgency, t.impact, t.category, t.subcategory, COALESCE(t.parent_id::text,''), t.major, t.snoozed_until, t.first_response_at, COALESCE(t.form_id::text,''), t.custom, t.sentiment, t.ai_summary, t.need_skills`

func scanDesk(row interface{ Scan(...any) error }) (DeskTicket, error) {
	var t DeskTicket
	var key string
	var custom []byte
	err := row.Scan(&t.ID, &t.ProjectID, &key, &t.Number, &t.Title, &t.Body, &t.Status, &t.Priority, &t.Type, &t.Tags,
		&t.RequesterName, &t.RequesterEmail, &t.Assignee, &t.Source, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &t.MergedInto, &t.ProblemID,
		&t.GroupID, &t.Urgency, &t.Impact, &t.Category, &t.Subcategory, &t.ParentID, &t.Major, &t.SnoozedUntil, &t.FirstResponse, &t.FormID, &custom, &t.Sentiment, &t.AISummary, &t.NeedSkills)
	if t.NeedSkills == nil {
		t.NeedSkills = []string{}
	}
	t.Custom = map[string]string{}
	if len(custom) > 0 {
		_ = json.Unmarshal(custom, &t.Custom)
	}
	t.Ref = fmt.Sprintf("%s-%d", key, t.Number)
	if t.Tags == nil {
		t.Tags = []string{}
	}
	return t, err
}

// ListDesk returns up to 3000 of a project's tickets with desk fields, newest first.
func (r *Repository) ListDesk(ctx context.Context, org, projectID string) ([]DeskTicket, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+deskCols+` FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id=t.project_id
		WHERE t.org_id=$1 AND t.project_id=$2 ORDER BY t.created_at DESC LIMIT 3000`, org, projectID)
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
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetDesk returns one ticket with desk fields.
func (r *Repository) GetDesk(ctx context.Context, org, id string) (DeskTicket, error) {
	t, err := scanDesk(r.pool.QueryRow(ctx, `SELECT `+deskCols+` FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id=t.project_id
		WHERE t.org_id=$1 AND t.id=$2`, org, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// Meta is a patch of tags, type and the linked problem. Nil fields are left alone.
type Meta struct {
	Tags      *[]string `json:"tags"`
	Type      *string   `json:"type"`
	ProblemID *string   `json:"problem_id"` // "" clears the link
}

// SetMeta changes tags, type or the problem link of a ticket and records the change.
func (r *Repository) SetMeta(ctx context.Context, org, id string, m Meta) (DeskTicket, error) {
	cur, err := r.GetDesk(ctx, org, id)
	if err != nil {
		return cur, err
	}
	if m.Tags != nil {
		tags := NormalizeTags(*m.Tags)
		if strings.Join(tags, ",") != strings.Join(cur.Tags, ",") {
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET tags=$3, updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, tags); err != nil {
				return cur, err
			}
			r.RecordEvent(ctx, org, id, "tags", map[string]any{"from": cur.Tags, "to": tags})
		}
	}
	if m.Type != nil {
		ty := NormalizeType(*m.Type)
		if ty != cur.Type {
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET ticket_type=$3, updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, ty); err != nil {
				return cur, err
			}
			r.RecordEvent(ctx, org, id, "type", map[string]any{"from": cur.Type, "to": ty})
		}
	}
	if m.ProblemID != nil {
		pid := strings.TrimSpace(*m.ProblemID)
		if pid == id {
			return cur, fmt.Errorf("%w: a ticket cannot be its own problem", ErrInvalid)
		}
		if pid != "" {
			p, err := r.GetDesk(ctx, org, pid)
			if err != nil {
				return cur, err
			}
			if p.Type != "problem" {
				return cur, fmt.Errorf("%w: link to a ticket of type problem", ErrInvalid)
			}
		}
		if pid != cur.ProblemID {
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET problem_id=$3, updated_at=now() WHERE org_id=$1 AND id=$2`, org, id, nullUUID(pid)); err != nil {
				return cur, err
			}
			r.RecordEvent(ctx, org, id, "problem", map[string]any{"from": cur.ProblemID, "to": pid})
		}
	}
	return r.GetDesk(ctx, org, id)
}

// Incidents returns the tickets linked to a problem ticket.
func (r *Repository) Incidents(ctx context.Context, org, problemID string) ([]DeskTicket, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+deskCols+` FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id=t.project_id
		WHERE t.org_id=$1 AND t.problem_id=$2 ORDER BY t.created_at DESC LIMIT 200`, org, problemID)
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
		out = append(out, t)
	}
	return out, rows.Err()
}

// BulkChange is applied to several tickets at once.
type BulkChange struct {
	IDs        []string `json:"ids"`
	Status     string   `json:"status"`
	Priority   string   `json:"priority"`
	Type       string   `json:"type"`
	Assignee   string   `json:"assignee"` // user id, or "none" to clear
	AddTags    []string `json:"add_tags"`
	RemoveTags []string `json:"remove_tags"`
}

// Bulk applies a change to up to 200 tickets and returns how many changed.
func (r *Repository) Bulk(ctx context.Context, org string, c BulkChange) (int, error) {
	if len(c.IDs) == 0 || len(c.IDs) > 200 {
		return 0, fmt.Errorf("%w: choose between 1 and 200 tickets", ErrInvalid)
	}
	if c.Status == "" && c.Priority == "" && c.Type == "" && c.Assignee == "" && len(c.AddTags) == 0 && len(c.RemoveTags) == 0 {
		return 0, fmt.Errorf("%w: nothing to change", ErrInvalid)
	}
	n := 0
	me := actorOf(ctx).ID
	for _, id := range c.IDs {
		t, err := r.GetDesk(ctx, org, id)
		if err != nil {
			continue // not in this org: skipped, never touched
		}
		if err := r.applyActions(ctx, org, t, MacroActions{Status: c.Status, Priority: c.Priority, Type: c.Type, Assignee: c.Assignee, AddTags: c.AddTags, RemoveTags: c.RemoveTags}, &me); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// applyActions performs macro-style actions on one ticket. me is the acting user for "me".
func (r *Repository) applyActions(ctx context.Context, org string, t DeskTicket, a MacroActions, me *string) error {
	var status, priority, assignee *string
	clear := false
	if a.Status != "" {
		p, err := r.GetProject(ctx, org, t.ProjectID)
		if err != nil {
			return err
		}
		ok := false
		for _, s := range p.Statuses {
			ok = ok || s == a.Status
		}
		if !ok {
			return fmt.Errorf("%w: status %q does not exist in this project", ErrInvalid, a.Status)
		}
		status = &a.Status
	}
	if a.Priority != "" {
		priority = &a.Priority
	}
	switch a.Assignee {
	case "":
	case "none":
		clear = true
	case "me":
		if me != nil && *me != "" {
			assignee = me
		}
	default:
		if _, err := r.memberOf(ctx, org, a.Assignee); err != nil {
			return fmt.Errorf("%w: assignee is not a member of this organisation", ErrInvalid)
		}
		assignee = &a.Assignee
	}
	if status != nil || priority != nil || assignee != nil || clear {
		if _, err := r.UpdateTicket(ctx, org, t.ID, status, priority, nil, nil, assignee, clear); err != nil {
			return err
		}
	}
	meta := Meta{}
	if len(a.AddTags) > 0 || len(a.RemoveTags) > 0 {
		tags := AddRemoveTags(t.Tags, a.AddTags, a.RemoveTags)
		meta.Tags = &tags
	}
	if a.Type != "" {
		ty := a.Type
		meta.Type = &ty
	}
	if meta.Tags != nil || meta.Type != nil {
		if _, err := r.SetMeta(ctx, org, t.ID, meta); err != nil {
			return err
		}
	}
	return nil
}

// memberOf checks a user id belongs to the org.
func (r *Repository) memberOf(ctx context.Context, org, user string) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.users WHERE org_id=$1 AND id::text=$2`, org, user).Scan(&n)
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, ErrNotFound
	}
	return true, nil
}

// Merge folds the source ticket into the target: public and internal comments are
// copied across (tagged with where they came from), tags are joined, and the source
// is closed and hidden from the views. Both stay readable. Merging twice, into
// itself, into a merged ticket or across projects is refused.
func (r *Repository) Merge(ctx context.Context, org, sourceID, targetID string) (DeskTicket, error) {
	if sourceID == targetID {
		return DeskTicket{}, fmt.Errorf("%w: a ticket cannot be merged into itself", ErrInvalid)
	}
	src, err := r.GetDesk(ctx, org, sourceID)
	if err != nil {
		return src, err
	}
	dst, err := r.GetDesk(ctx, org, targetID)
	if err != nil {
		return dst, err
	}
	if src.MergedInto != "" {
		return src, fmt.Errorf("%w: %s was already merged", ErrInvalid, src.Ref)
	}
	if dst.MergedInto != "" {
		return dst, fmt.Errorf("%w: %s was itself merged into another ticket", ErrInvalid, dst.Ref)
	}
	if src.ProjectID != dst.ProjectID {
		return dst, fmt.Errorf("%w: tickets can only be merged inside one project", ErrInvalid)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return dst, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.ticket_comments (ticket_id, author_user_id, author_name, body, is_internal, created_at)
		SELECT $2, author_user_id, author_name, '[from '||$3||'] '||body, is_internal, created_at
		FROM adrient.ticket_comments WHERE ticket_id=$1 ORDER BY created_at`, sourceID, targetID, src.Ref); err != nil {
		return dst, err
	}
	a := actorOf(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.ticket_comments (ticket_id, author_user_id, author_name, body, is_internal)
		VALUES ($1,$2,$3,$4,true)`, targetID, nullUUID(a.ID), a.Name, fmt.Sprintf("Merged %s (%s) into this ticket.", src.Ref, src.Title)); err != nil {
		return dst, err
	}
	closed := "closed"
	if _, err := tx.Exec(ctx, `UPDATE adrient.tickets SET merged_into=$3, status=$4, resolved_at=COALESCE(resolved_at, now()), updated_at=now()
		WHERE org_id=$1 AND id=$2`, org, sourceID, targetID, closed); err != nil {
		return dst, err
	}
	joined := AddRemoveTags(dst.Tags, src.Tags, nil)
	if _, err := tx.Exec(ctx, `UPDATE adrient.tickets SET tags=$3, updated_at=now() WHERE org_id=$1 AND id=$2`, org, targetID, joined); err != nil {
		return dst, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.tickets SET problem_id=$3 WHERE org_id=$1 AND problem_id=$2`, org, sourceID, targetID); err != nil {
		return dst, err
	}
	if err := tx.Commit(ctx); err != nil {
		return dst, err
	}
	r.RecordEvent(ctx, org, sourceID, "merged_into", map[string]any{"ticket": dst.Ref, "id": targetID})
	r.RecordEvent(ctx, org, targetID, "merged", map[string]any{"ticket": src.Ref, "id": sourceID})
	return r.GetDesk(ctx, org, targetID)
}

// ---- macros ----

func (r *Repository) ListMacros(ctx context.Context, org, projectID string) ([]Macro, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, COALESCE(project_id::text,''), name, actions FROM adrient.ticket_macros
		WHERE org_id=$1 AND (project_id IS NULL OR project_id::text=$2 OR $2='') ORDER BY lower(name)`, org, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Macro{}
	for rows.Next() {
		var m Macro
		var raw []byte
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Name, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &m.Actions)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) SaveMacro(ctx context.Context, org, user string, m Macro) (Macro, error) {
	statuses := DefaultStatuses
	if m.ProjectID != "" {
		p, err := r.GetProject(ctx, org, m.ProjectID)
		if err != nil {
			return m, err
		}
		statuses = p.Statuses
	}
	if err := m.Validate(statuses); err != nil {
		return m, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	raw, _ := json.Marshal(m.Actions)
	if m.ID == "" {
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_macros (org_id, project_id, name, actions, created_by)
			VALUES ($1,$2,$3,$4::jsonb,$5) RETURNING id::text`, org, nullUUID(m.ProjectID), m.Name, string(raw), nullUUID(user)).Scan(&m.ID)
		return m, err
	}
	ct, err := r.pool.Exec(ctx, `UPDATE adrient.ticket_macros SET name=$3, actions=$4::jsonb, project_id=$5 WHERE org_id=$1 AND id=$2`,
		org, m.ID, m.Name, string(raw), nullUUID(m.ProjectID))
	if err == nil && ct.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return m, err
}

func (r *Repository) DeleteMacro(ctx context.Context, org, id string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_macros WHERE org_id=$1 AND id=$2`, org, id)
	if err == nil && ct.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// ApplyMacro runs a macro on a ticket: field changes first, then the reply (placeholders filled).
func (r *Repository) ApplyMacro(ctx context.Context, org, macroID, ticketID string) (DeskTicket, error) {
	var raw []byte
	var name, mproj string
	err := r.pool.QueryRow(ctx, `SELECT name, actions, COALESCE(project_id::text,'') FROM adrient.ticket_macros WHERE org_id=$1 AND id=$2`, org, macroID).Scan(&name, &raw, &mproj)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeskTicket{}, ErrNotFound
	}
	if err != nil {
		return DeskTicket{}, err
	}
	t, err := r.GetDesk(ctx, org, ticketID)
	if err != nil {
		return t, err
	}
	if mproj != "" && mproj != t.ProjectID {
		return t, fmt.Errorf("%w: this macro belongs to another project", ErrInvalid)
	}
	var a MacroActions
	_ = json.Unmarshal(raw, &a)
	me := actorOf(ctx).ID
	if err := r.applyActions(ctx, org, t, a, &me); err != nil {
		return t, err
	}
	if a.Reply != "" {
		body := FillPlaceholders(a.Reply, t, actorOf(ctx).Name)
		var uid *string
		if me != "" {
			uid = &me
		}
		if _, err := r.AddComment(ctx, org, ticketID, uid, actorOf(ctx).Name, body, a.Internal); err != nil {
			return t, err
		}
	}
	r.RecordEvent(ctx, org, ticketID, "macro", map[string]any{"name": name})
	return r.GetDesk(ctx, org, ticketID)
}

// ---- followers ----

// ToggleFollow adds or removes the user as a follower and reports the new state.
func (r *Repository) ToggleFollow(ctx context.Context, org, ticketID, user string) (bool, error) {
	if _, err := r.GetDesk(ctx, org, ticketID); err != nil {
		return false, err
	}
	ct, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_followers WHERE ticket_id=$1 AND user_id=$2 AND org_id=$3`, ticketID, user, org)
	if err != nil {
		return false, err
	}
	if ct.RowsAffected() > 0 {
		return false, nil
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.ticket_followers (ticket_id, user_id, org_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, ticketID, user, org)
	return true, err
}

// Followers lists follower user ids.
func (r *Repository) Followers(ctx context.Context, org, ticketID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id::text FROM adrient.ticket_followers WHERE ticket_id=$1 AND org_id=$2 ORDER BY created_at`, ticketID, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ActorID returns the id of the acting user stored in the context ("" when none).
func ActorID(ctx context.Context) string { return actorOf(ctx).ID }
