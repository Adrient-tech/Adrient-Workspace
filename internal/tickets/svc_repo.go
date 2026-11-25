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

// ---- groups -----------------------------------------------------------------

// Group is a team that tickets are routed to.
type Group struct {
	ID          string   `json:"id"`
	ProjectID   string   `json:"project_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Strategy    string   `json:"strategy"`
	CalendarID  string   `json:"calendar_id"`
	EmailAlias  string   `json:"email_alias"`
	Members     []Member `json:"members"`
	OpenCount   int      `json:"open_count"`
}

// ListGroups returns a project's groups with members and open counts.
func (r *Repository) ListGroups(ctx context.Context, org, projectID string) ([]Group, error) {
	rows, err := r.pool.Query(ctx, `SELECT g.id::text, g.project_id::text, g.name, g.description, g.strategy,
		COALESCE(g.calendar_id::text,''), g.email_alias,
		(SELECT count(*) FROM adrient.tickets t WHERE t.group_id=g.id AND `+notDoneSQL("t")+`)
		FROM adrient.ticket_groups g WHERE g.org_id=$1 AND g.project_id=$2 ORDER BY lower(g.name)`, org, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.ProjectID, &g.Name, &g.Description, &g.Strategy, &g.CalendarID, &g.EmailAlias, &g.OpenCount); err != nil {
			return nil, err
		}
		g.Members = []Member{}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		ms, err := r.groupMembers(ctx, org, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Members = ms
	}
	return out, nil
}

func (r *Repository) groupMembers(ctx context.Context, org, groupID string) ([]Member, error) {
	rows, err := r.pool.Query(ctx, `SELECT m.user_id::text, COALESCE(u.display_name,''), m.skills, m.capacity, m.ooo_until, m.position,
		(SELECT count(*) FROM adrient.tickets t WHERE t.org_id=m.org_id AND t.assignee_user_id=m.user_id AND `+notDoneSQL("t")+`)
		FROM adrient.ticket_group_members m LEFT JOIN adrient.users u ON u.id=m.user_id
		WHERE m.group_id=$1 AND m.org_id=$2 ORDER BY m.position, u.display_name`, groupID, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Name, &m.Skills, &m.Capacity, &m.OOOUntil, &m.Position, &m.Open); err != nil {
			return nil, err
		}
		if m.Skills == nil {
			m.Skills = []string{}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// notDoneSQL is the SQL test for "ticket is still being worked on", honouring custom statuses.
func notDoneSQL(a string) string {
	return fmt.Sprintf(`(%[1]s.status NOT IN ('resolved','closed') AND NOT EXISTS (SELECT 1 FROM adrient.ticket_status_defs sd
		WHERE sd.project_id=%[1]s.project_id AND sd.key=%[1]s.status AND sd.category IN ('solved','closed')))`, a)
}

// SaveGroup creates or updates a group and replaces its members.
func (r *Repository) SaveGroup(ctx context.Context, org, projectID string, g Group) (Group, error) {
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" || len(g.Name) > 80 {
		return g, fmt.Errorf("%w: group name is required (max 80 characters)", ErrInvalid)
	}
	if g.Strategy == "" {
		g.Strategy = StrategyManual
	}
	g.EmailAlias = strings.ToLower(strings.TrimSpace(g.EmailAlias))
	if g.EmailAlias != "" {
		if _, addr, ok := ParseAddress(g.EmailAlias); !ok || addr != g.EmailAlias {
			return g, fmt.Errorf("%w: the team email address is not valid", ErrInvalid)
		}
	}
	if !ValidStrategy(g.Strategy) {
		return g, fmt.Errorf("%w: unknown routing strategy", ErrInvalid)
	}
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return g, err
	}
	if len(g.Members) > 200 {
		return g, fmt.Errorf("%w: too many members", ErrInvalid)
	}
	cal := nullUUID(g.CalendarID)
	if cal != nil {
		var n int
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_calendars WHERE id=$1 AND org_id=$2`, *cal, org).Scan(&n)
		if n == 0 {
			return g, fmt.Errorf("%w: calendar not found", ErrInvalid)
		}
	}
	for _, m := range g.Members {
		if _, err := r.memberOf(ctx, org, m.UserID); err != nil {
			return g, fmt.Errorf("%w: %s is not a member of this organisation", ErrInvalid, m.UserID)
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return g, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if g.ID == "" {
		err = tx.QueryRow(ctx, `INSERT INTO adrient.ticket_groups (org_id, project_id, name, description, strategy, calendar_id, email_alias)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, org, projectID, g.Name, strings.TrimSpace(g.Description), g.Strategy, cal, strings.TrimSpace(g.EmailAlias)).Scan(&g.ID)
	} else {
		var id string
		err = tx.QueryRow(ctx, `UPDATE adrient.ticket_groups SET name=$4, description=$5, strategy=$6, calendar_id=$7, email_alias=$8
			WHERE id=$1 AND org_id=$2 AND project_id=$3 RETURNING id::text`, g.ID, org, projectID, g.Name, strings.TrimSpace(g.Description), g.Strategy, cal, strings.TrimSpace(g.EmailAlias)).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return g, ErrNotFound
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "ticket_groups_alias_idx") {
			return g, fmt.Errorf("%w: another team already uses this email address", ErrInvalid)
		}
		if strings.Contains(err.Error(), "ticket_groups_name_idx") {
			return g, fmt.Errorf("%w: a group with this name already exists", ErrInvalid)
		}
		return g, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.ticket_group_members WHERE group_id=$1`, g.ID); err != nil {
		return g, err
	}
	for i, m := range g.Members {
		cap := m.Capacity
		if cap < 0 || cap > 500 {
			cap = 20
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.ticket_group_members (group_id, org_id, user_id, skills, capacity, ooo_until, position)
			VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (group_id,user_id) DO NOTHING`, g.ID, org, m.UserID, NormalizeTags(m.Skills), cap, m.OOOUntil, i); err != nil {
			return g, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return g, err
	}
	g.ProjectID = projectID
	g.Members, _ = r.groupMembers(ctx, org, g.ID)
	return g, nil
}

// DeleteGroup removes a group; its tickets become group-less.
func (r *Repository) DeleteGroup(ctx context.Context, org, id string) error {
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.tickets SET group_id=NULL WHERE org_id=$1 AND group_id=$2`, org, id)
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_groups WHERE org_id=$1 AND id=$2`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// SetOOO sets or clears a member's out-of-office end for every group they belong to in the org.
func (r *Repository) SetOOO(ctx context.Context, org, user string, until *time.Time) error {
	if until != nil && (!until.After(time.Now()) || until.After(time.Now().AddDate(1, 0, 0))) {
		return fmt.Errorf("%w: choose an end date within the next year", ErrInvalid)
	}
	_, err := r.pool.Exec(ctx, `UPDATE adrient.ticket_group_members SET ooo_until=$3 WHERE org_id=$1 AND user_id=$2`, org, user, until)
	return err
}

// routeTicket assigns a ticket to a member of its group by the group's strategy.
// It returns the chosen user id ("" when nobody qualifies or routing is manual).
func (r *Repository) routeTicket(ctx context.Context, org, ticketID, groupID string, needSkills []string) string {
	var strategy string
	var cursor int
	if err := r.pool.QueryRow(ctx, `SELECT strategy, rr_cursor FROM adrient.ticket_groups WHERE id=$1 AND org_id=$2`, groupID, org).Scan(&strategy, &cursor); err != nil {
		return ""
	}
	ms, err := r.groupMembers(ctx, org, groupID)
	if err != nil {
		return ""
	}
	if len(needSkills) == 0 {
		_ = r.pool.QueryRow(ctx, `SELECT need_skills FROM adrient.tickets WHERE id=$1 AND org_id=$2`, ticketID, org).Scan(&needSkills)
	}
	user, next := PickAssignee(strategy, ms, needSkills, cursor, time.Now())
	if user == "" {
		if strategy != StrategyManual {
			why := "nobody in the team is available"
			if len(needSkills) > 0 {
				why = "nobody available has the skills: " + strings.Join(needSkills, ", ")
			}
			r.RecordEvent(ctx, org, ticketID, "routing", map[string]any{"unassigned": why})
		}
		return ""
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET assignee_user_id=$3, updated_at=now() WHERE id=$1 AND org_id=$2 AND assignee_user_id IS NULL`, ticketID, org, user); err != nil {
		return ""
	}
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.ticket_groups SET rr_cursor=$2 WHERE id=$1`, groupID, next)
	r.RecordEvent(ctx, org, ticketID, "assignee", map[string]any{"to": user, "via": "routing:" + strategy})
	r.notify(ctx, org, user, "Ticket assigned to you", "A ticket was routed to you.", ticketID)
	return user
}

// ---- calendars --------------------------------------------------------------

// CalendarRow is a stored business-hours calendar.
type CalendarRow struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	TZ        string                 `json:"tz"`
	Hours     map[string][][2]string `json:"hours"`
	IsDefault bool                   `json:"is_default"`
	Holidays  []Holiday              `json:"holidays"`
}

// Holiday is a closed day.
type Holiday struct {
	ID   string `json:"id,omitempty"`
	Day  string `json:"day"`
	Name string `json:"name"`
}

// ListCalendars returns the org's calendars with holidays.
func (r *Repository) ListCalendars(ctx context.Context, org string) ([]CalendarRow, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, tz, hours, is_default FROM adrient.ticket_calendars WHERE org_id=$1 ORDER BY is_default DESC, lower(name)`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CalendarRow{}
	for rows.Next() {
		var c CalendarRow
		var raw []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.TZ, &raw, &c.IsDefault); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &c.Hours)
		c.Holidays = []Holiday{}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		hr, err := r.pool.Query(ctx, `SELECT id::text, to_char(day,'YYYY-MM-DD'), name FROM adrient.ticket_holidays WHERE calendar_id=$1 ORDER BY day`, out[i].ID)
		if err != nil {
			return nil, err
		}
		for hr.Next() {
			var h Holiday
			if err := hr.Scan(&h.ID, &h.Day, &h.Name); err != nil {
				hr.Close()
				return nil, err
			}
			out[i].Holidays = append(out[i].Holidays, h)
		}
		hr.Close()
	}
	return out, nil
}

// SaveCalendar validates and stores a calendar and replaces its holidays.
func (r *Repository) SaveCalendar(ctx context.Context, org string, c CalendarRow) (CalendarRow, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len(c.Name) > 80 {
		return c, fmt.Errorf("%w: calendar name is required", ErrInvalid)
	}
	cal := &Calendar{TZ: c.TZ, Hours: c.Hours}
	if err := cal.Prepare(); err != nil {
		return c, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if len(c.Holidays) > 400 {
		return c, fmt.Errorf("%w: too many holidays", ErrInvalid)
	}
	for _, h := range c.Holidays {
		if _, err := time.Parse("2006-01-02", h.Day); err != nil {
			return c, fmt.Errorf("%w: holiday date %q must be YYYY-MM-DD", ErrInvalid, h.Day)
		}
	}
	raw, _ := json.Marshal(c.Hours)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if c.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE adrient.ticket_calendars SET is_default=false WHERE org_id=$1`, org); err != nil {
			return c, err
		}
	}
	if c.ID == "" {
		err = tx.QueryRow(ctx, `INSERT INTO adrient.ticket_calendars (org_id,name,tz,hours,is_default) VALUES ($1,$2,$3,$4,$5) RETURNING id::text`,
			org, c.Name, c.TZ, raw, c.IsDefault).Scan(&c.ID)
	} else {
		var id string
		err = tx.QueryRow(ctx, `UPDATE adrient.ticket_calendars SET name=$3,tz=$4,hours=$5,is_default=$6 WHERE id=$1 AND org_id=$2 RETURNING id::text`,
			c.ID, org, c.Name, c.TZ, raw, c.IsDefault).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return c, ErrNotFound
		}
	}
	if err != nil {
		return c, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.ticket_holidays WHERE calendar_id=$1`, c.ID); err != nil {
		return c, err
	}
	for _, h := range c.Holidays {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.ticket_holidays (org_id,calendar_id,day,name) VALUES ($1,$2,$3,$4) ON CONFLICT (calendar_id,day) DO NOTHING`,
			org, c.ID, h.Day, strings.TrimSpace(h.Name)); err != nil {
			return c, err
		}
	}
	return c, tx.Commit(ctx)
}

// DeleteCalendar removes a calendar; groups and SLA rules that used it fall back to the default.
func (r *Repository) DeleteCalendar(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_calendars WHERE org_id=$1 AND id=$2`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// LoadCalendar returns a prepared calendar. id "" picks the org default; nil means 24x7.
func (r *Repository) LoadCalendar(ctx context.Context, org, id string) *Calendar {
	var cid, tz string
	var raw []byte
	var err error
	if id != "" {
		err = r.pool.QueryRow(ctx, `SELECT id::text, tz, hours FROM adrient.ticket_calendars WHERE org_id=$1 AND id=$2`, org, id).Scan(&cid, &tz, &raw)
	}
	if id == "" || err != nil {
		err = r.pool.QueryRow(ctx, `SELECT id::text, tz, hours FROM adrient.ticket_calendars WHERE org_id=$1 AND is_default LIMIT 1`, org).Scan(&cid, &tz, &raw)
		if err != nil {
			return nil
		}
	}
	c := &Calendar{TZ: tz, Holidays: map[string]bool{}}
	_ = json.Unmarshal(raw, &c.Hours)
	if c.Prepare() != nil {
		return nil
	}
	rows, err := r.pool.Query(ctx, `SELECT to_char(day,'YYYY-MM-DD') FROM adrient.ticket_holidays WHERE calendar_id=$1`, cid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var d string
			if rows.Scan(&d) == nil {
				c.Holidays[d] = true
			}
		}
	}
	return c
}

// ---- statuses ---------------------------------------------------------------

// StatusDefs returns the project's workflow: stored definitions, or defaults derived from
// the project's status list.
func (r *Repository) StatusDefs(ctx context.Context, org, projectID string) ([]StatusDef, error) {
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT key,label,category,pauses_sla,position,requires FROM adrient.ticket_status_defs WHERE project_id=$1 ORDER BY position`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StatusDef{}
	for rows.Next() {
		var d StatusDef
		if err := rows.Scan(&d.Key, &d.Label, &d.Category, &d.PausesSLA, &d.Position, &d.Requires); err != nil {
			return nil, err
		}
		if d.Requires == nil {
			d.Requires = []string{}
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil || len(out) > 0 {
		return out, err
	}
	p, err := r.GetProject(ctx, org, projectID)
	if err != nil {
		return nil, err
	}
	for i, s := range p.Statuses {
		cat := CategoryOf(nil, s)
		out = append(out, StatusDef{Key: s, Label: strings.ToUpper(s[:1]) + strings.ReplaceAll(s[1:], "_", " "), Category: cat, PausesSLA: PausesSLA(nil, s), Position: i, Requires: []string{}})
	}
	return out, nil
}

// SaveStatusDefs replaces the workflow of a project and keeps the project's status list in step.
// A status that still has tickets cannot be removed.
func (r *Repository) SaveStatusDefs(ctx context.Context, org, projectID string, defs []StatusDef) ([]StatusDef, error) {
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return nil, err
	}
	if err := ValidateStatusDefs(defs); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	keys := make([]string, len(defs))
	for i, d := range defs {
		keys[i] = d.Key
		for _, f := range d.Requires {
			switch f {
			case "assignee", "category", "group", "type":
			default:
				return nil, fmt.Errorf("%w: cannot require unknown field %q", ErrInvalid, f)
			}
		}
	}
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.tickets WHERE project_id=$1 AND NOT (status = ANY($2))`, projectID, keys).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("%w: %d ticket(s) use a status you removed; move them first", ErrInvalid, n)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.ticket_status_defs WHERE project_id=$1`, projectID); err != nil {
		return nil, err
	}
	for _, d := range defs {
		req := d.Requires
		if req == nil {
			req = []string{}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.ticket_status_defs (project_id,key,label,category,pauses_sla,position,requires) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			projectID, d.Key, d.Label, d.Category, d.PausesSLA, d.Position, req); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.ticket_projects SET statuses=$3 WHERE id=$1 AND org_id=$2`, projectID, org, keys); err != nil {
		return nil, err
	}
	return defs, tx.Commit(ctx)
}

// statusGuard rejects a status change that misses required fields.
func (r *Repository) statusGuard(ctx context.Context, org string, t DeskTicket, newStatus string) error {
	if newStatus == "" || newStatus == t.Status {
		return nil
	}
	defs, err := r.StatusDefs(ctx, org, t.ProjectID)
	if err != nil {
		return nil
	}
	filled := map[string]bool{"assignee": t.Assignee != "", "category": t.Category != "", "group": t.GroupID != "", "type": t.Type != ""}
	if miss := MissingRequired(defs, newStatus, filled); len(miss) > 0 {
		return fmt.Errorf("%w: set %s before moving to %s", ErrInvalid, strings.Join(miss, " and "), newStatus)
	}
	if fd, err := r.ListFieldDefs(ctx, org, t.ProjectID); err == nil {
		if miss := MissingCustom(fd, t.Type, newStatus, t.Custom); len(miss) > 0 {
			return fmt.Errorf("%w: fill in %s before moving to %s", ErrInvalid, strings.Join(miss, " and "), newStatus)
		}
	}
	return nil
}

// slaTransition pauses or resumes the SLA clock when a status change crosses the pause boundary.
func (r *Repository) slaTransition(ctx context.Context, before, after Ticket) {
	if before.Status == after.Status {
		return
	}
	defs, err := r.StatusDefs(ctx, after.OrgID, after.ProjectID)
	if err != nil {
		return
	}
	was, is := PausesSLA(defs, before.Status), PausesSLA(defs, after.Status)
	switch {
	case !was && is:
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.tickets SET sla_paused_at=now() WHERE id=$1 AND sla_paused_at IS NULL`, after.ID)
	case was && !is:
		var pausedAt *time.Time
		var groupID string
		_ = r.pool.QueryRow(ctx, `SELECT sla_paused_at, COALESCE(group_id::text,'') FROM adrient.tickets WHERE id=$1`, after.ID).Scan(&pausedAt, &groupID)
		if pausedAt == nil {
			return
		}
		mins := PauseMinutesOnResume(r.ticketCalendar(ctx, after.OrgID, after.ProjectID, groupID), *pausedAt, time.Now())
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.tickets SET sla_paused_at=NULL, sla_paused_min=sla_paused_min+$2 WHERE id=$1`, after.ID, mins)
	}
}

// ticketCalendar picks the calendar for a ticket: its group's, else the org default.
func (r *Repository) ticketCalendar(ctx context.Context, org, projectID, groupID string) *Calendar {
	if groupID != "" {
		var cid string
		_ = r.pool.QueryRow(ctx, `SELECT COALESCE(calendar_id::text,'') FROM adrient.ticket_groups WHERE id=$1 AND org_id=$2`, groupID, org).Scan(&cid)
		return r.LoadCalendar(ctx, org, cid)
	}
	return r.LoadCalendar(ctx, org, "")
}
