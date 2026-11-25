package talent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// TemplateTask is one task of a checklist template.
type TemplateTask struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Role        string `json:"role"` // hr | it | manager | employee
	DueOffset   int    `json:"due_offset_days"`
	Link        string `json:"link"`
}

// TemplateInput creates or updates a checklist template.
type TemplateInput struct {
	Kind  string         `json:"kind"`
	Name  string         `json:"name"`
	Tasks []TemplateTask `json:"tasks"`
}

var checklistRoles = []string{"hr", "it", "manager", "employee"}

// Validate checks the template.
func (t *TemplateInput) Validate() error {
	t.Name = clean(t.Name)
	if t.Name == "" {
		return invalid("name is required")
	}
	if !oneOf(t.Kind, "onboarding", "offboarding") {
		return invalid("kind must be onboarding or offboarding")
	}
	if len(t.Tasks) == 0 || len(t.Tasks) > 100 {
		return invalid("a template needs between 1 and 100 tasks")
	}
	for i := range t.Tasks {
		t.Tasks[i].Title = clean(t.Tasks[i].Title)
		if t.Tasks[i].Title == "" {
			return invalid("every task needs a title")
		}
		if !oneOf(t.Tasks[i].Role, checklistRoles...) {
			return invalid("task %q: role must be hr, it, manager or employee", t.Tasks[i].Title)
		}
		if t.Tasks[i].DueOffset < -365 || t.Tasks[i].DueOffset > 365 {
			return invalid("task %q: due offset must be within a year", t.Tasks[i].Title)
		}
	}
	return nil
}

// DueDate computes a task due date from the checklist anchor date.
func DueDate(anchor time.Time, offsetDays int) time.Time { return anchor.AddDate(0, 0, offsetDays) }

// ListTemplates lists checklist templates (HR only).
func (r *Repository) ListTemplates(ctx context.Context, a Actor) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	return r.rows(ctx, `SELECT id::text AS id, kind, name, tasks, created_at FROM adrient.talent_checklist_templates WHERE org_id=$1 ORDER BY kind, name`, a.OrgID)
}

// SaveTemplate creates (id=="") or updates a template.
func (r *Repository) SaveTemplate(ctx context.Context, a Actor, id string, in TemplateInput) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tb, _ := json.Marshal(in.Tasks)
	var rid string
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_checklist_templates (org_id, kind, name, tasks) VALUES ($1,$2,$3,$4) RETURNING id::text`,
			a.OrgID, in.Kind, in.Name, tb).Scan(&rid)
	} else {
		err = r.pool.QueryRow(ctx, `UPDATE adrient.talent_checklist_templates SET kind=$3, name=$4, tasks=$5 WHERE org_id=$1 AND id=$2 RETURNING id::text`,
			a.OrgID, id, in.Kind, in.Name, tb).Scan(&rid)
	}
	if err != nil {
		return nil, mapNoRows(err)
	}
	return r.row(ctx, `SELECT id::text AS id, kind, name, tasks FROM adrient.talent_checklist_templates WHERE id=$1`, rid)
}

// DeleteTemplate removes a template (running checklists keep their tasks).
func (r *Repository) DeleteTemplate(ctx context.Context, a Actor, id string) error {
	if !a.IsHR {
		return ErrForbidden
	}
	ct, err := r.pool.Exec(ctx, `DELETE FROM adrient.talent_checklist_templates WHERE org_id=$1 AND id=$2`, a.OrgID, id)
	if err == nil && ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// StartInput starts a checklist for a person.
type StartInput struct {
	TemplateID    string `json:"template_id"`
	SubjectUserID string `json:"subject_user_id"`
	SubjectName   string `json:"subject_name"`
	SubjectEmail  string `json:"subject_email"`
	ManagerID     string `json:"manager_id"`
	HRID          string `json:"hr_id"`
	ITID          string `json:"it_id"`
	AnchorDate    string `json:"anchor_date"` // start date (onboarding) or last day (offboarding)
}

// StartChecklist instantiates a template. Tasks go to the Tasks app of each assignee when it is wired.
func (r *Repository) StartChecklist(ctx context.Context, a Actor, in StartInput) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	var kind, tname string
	var traw []byte
	if err := r.pool.QueryRow(ctx, `SELECT kind, name, tasks FROM adrient.talent_checklist_templates WHERE org_id=$1 AND id=$2`, a.OrgID, in.TemplateID).Scan(&kind, &tname, &traw); err != nil {
		return nil, mapNoRows(err)
	}
	var tasks []TemplateTask
	_ = json.Unmarshal(traw, &tasks)
	anchor, err := time.Parse("2006-01-02", in.AnchorDate)
	if err != nil {
		return nil, invalid("date must be YYYY-MM-DD")
	}
	name, email, mgr := clean(in.SubjectName), clean(in.SubjectEmail), in.ManagerID
	if in.SubjectUserID != "" {
		u, err := r.userBrief(ctx, a.OrgID, in.SubjectUserID)
		if err != nil {
			return nil, invalid("the person is not a member of this organisation")
		}
		name, email = u.Name, u.Email
		if mgr == "" {
			mgr = u.ManagerID
		}
	}
	if name == "" {
		return nil, invalid("choose a person or enter a name")
	}
	hr, it := in.HRID, in.ITID
	if hr == "" {
		hr = a.UserID
	}
	if it == "" {
		it = hr
	}
	var cid string
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.talent_checklists (org_id, template_id, kind, subject_user_id, subject_name, subject_email, manager_id, hr_id, it_id, anchor_date, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, a.OrgID, in.TemplateID, kind, nullUUID(in.SubjectUserID), name, email,
		nullUUID(mgr), nullUUID(hr), nullUUID(it), anchor, a.UserID).Scan(&cid); err != nil {
		return nil, err
	}
	type made struct {
		id, assignee, title, notes string
		due                        time.Time
	}
	var created []made
	for i, t := range tasks {
		assignee := ""
		switch t.Role {
		case "hr":
			assignee = hr
		case "it":
			assignee = it
		case "manager":
			assignee = mgr
		case "employee":
			assignee = in.SubjectUserID
		}
		link := t.Link
		if link == "" && kind == "offboarding" && t.Role == "it" {
			link = "/admin/users?q=" + url.QueryEscape(email)
		}
		due := DueDate(anchor, t.DueOffset)
		var tid string
		if err := tx.QueryRow(ctx, `INSERT INTO adrient.talent_checklist_tasks (org_id, checklist_id, title, description, assignee_role, assignee_id, due_date, link, position)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`, a.OrgID, cid, t.Title, t.Description, t.Role, nullUUID(assignee), due, link, i).Scan(&tid); err != nil {
			return nil, err
		}
		if assignee != "" {
			created = append(created, made{tid, assignee, t.Title + " (" + name + ")", t.Description, due})
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if r.Deps.Tasks != nil {
		for _, m := range created {
			d := m.due
			if id, err := r.Deps.Tasks(ctx, a.OrgID, m.assignee, m.title, m.notes, &d); err == nil {
				_, _ = r.pool.Exec(ctx, `UPDATE adrient.talent_checklist_tasks SET tasks_item_id=$2 WHERE id=$1`, m.id, id)
			}
		}
	}
	r.audit(ctx, a, "checklist.start", "talent_checklist", cid)
	return r.GetChecklist(ctx, a, cid)
}

// checklistVisible: HR, the manager, hr/it owners, the subject, or any assignee of a task.
const checklistVisible = `(c.org_id=$1 AND ($3::boolean OR c.manager_id=$2::uuid OR c.hr_id=$2::uuid OR c.it_id=$2::uuid OR c.subject_user_id=$2::uuid
	OR EXISTS(SELECT 1 FROM adrient.talent_checklist_tasks t WHERE t.checklist_id=c.id AND t.assignee_id=$2::uuid)))`

// ListChecklists lists the checklists the actor may see, with progress.
func (r *Repository) ListChecklists(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT c.id::text AS id, c.kind, c.subject_name, c.subject_email, COALESCE(c.subject_user_id::text,'') AS subject_user_id,
		c.anchor_date, c.status, c.created_at,
		(SELECT count(*) FROM adrient.talent_checklist_tasks t WHERE t.checklist_id=c.id) AS total,
		(SELECT count(*) FROM adrient.talent_checklist_tasks t WHERE t.checklist_id=c.id AND t.done) AS done,
		(SELECT count(*) FROM adrient.talent_checklist_tasks t WHERE t.checklist_id=c.id AND NOT t.done AND t.due_date < current_date) AS overdue
		FROM adrient.talent_checklists c WHERE `+checklistVisible+` ORDER BY c.created_at DESC`, a.OrgID, a.UserID, a.IsHR)
}

// GetChecklist returns one visible checklist with tasks, assets and exit interview.
func (r *Repository) GetChecklist(ctx context.Context, a Actor, id string) (json.RawMessage, error) {
	base, err := r.row(ctx, `SELECT c.id::text AS id, c.kind, c.subject_name, c.subject_email, COALESCE(c.subject_user_id::text,'') AS subject_user_id,
		COALESCE(c.manager_id::text,'') AS manager_id, c.anchor_date, c.status FROM adrient.talent_checklists c WHERE c.id=$4 AND `+checklistVisible,
		a.OrgID, a.UserID, a.IsHR, id)
	if err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	_ = json.Unmarshal(base, &out)
	out["tasks"], err = r.rows(ctx, `SELECT t.id::text AS id, t.title, t.description, t.assignee_role, COALESCE(t.assignee_id::text,'') AS assignee_id,
		COALESCE(NULLIF(u.display_name,''),u.email,'') AS assignee_name, t.due_date, t.link, t.done, t.done_at,
		(NOT t.done AND t.due_date < current_date) AS overdue
		FROM adrient.talent_checklist_tasks t LEFT JOIN adrient.users u ON u.id=t.assignee_id WHERE t.checklist_id=$1 ORDER BY t.position`, id)
	if err != nil {
		return nil, err
	}
	out["assets"], err = r.rows(ctx, `SELECT id::text AS id, name, serial, status FROM adrient.talent_assets WHERE checklist_id=$1 ORDER BY name`, id)
	if err != nil {
		return nil, err
	}
	out["exit_interview"] = json.RawMessage(`null`)
	// Exit feedback is HR-only: managers and assignees never receive it.
	ex, err := r.row(ctx, `SELECT reason, would_return, answers, notes, at FROM adrient.talent_exit_interviews WHERE checklist_id=$1 AND $2::boolean`, id, a.IsHR)
	if err == nil {
		out["exit_interview"] = ex
	} else {
		out["exit_interview"] = json.RawMessage(`null`)
	}
	b, _ := json.Marshal(out)
	return b, nil
}

// canWorkTask: HR, the assignee, or the checklist manager may tick a task.
func (r *Repository) taskChecklist(ctx context.Context, a Actor, taskID string) (cid string, err error) {
	var assignee, mgr string
	err = r.pool.QueryRow(ctx, `SELECT t.checklist_id::text, COALESCE(t.assignee_id::text,''), COALESCE(c.manager_id::text,'')
		FROM adrient.talent_checklist_tasks t JOIN adrient.talent_checklists c ON c.id=t.checklist_id WHERE t.org_id=$1 AND t.id=$2`, a.OrgID, taskID).Scan(&cid, &assignee, &mgr)
	if err != nil {
		return "", mapNoRows(err)
	}
	if !a.IsHR && a.UserID != assignee && a.UserID != mgr {
		return "", ErrNotFound
	}
	return cid, nil
}

// SetTaskDone ticks or unticks a checklist task. The matching Tasks item follows.
func (r *Repository) SetTaskDone(ctx context.Context, a Actor, taskID string, done bool) (json.RawMessage, error) {
	cid, err := r.taskChecklist(ctx, a, taskID)
	if err != nil {
		return nil, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.talent_checklist_tasks SET done=$2, done_at=CASE WHEN $2 THEN now() END, done_by=CASE WHEN $2 THEN $3::uuid END WHERE id=$1`,
		taskID, done, a.UserID); err != nil {
		return nil, err
	}
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.tasks SET completed=$2, completed_at=CASE WHEN $2 THEN now() END
		WHERE id=(SELECT tasks_item_id FROM adrient.talent_checklist_tasks WHERE id=$1)`, taskID, done)
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.talent_checklists c SET status=CASE WHEN NOT EXISTS(SELECT 1 FROM adrient.talent_checklist_tasks t WHERE t.checklist_id=c.id AND NOT t.done) THEN 'done' ELSE 'active' END
		WHERE c.id=$1 AND c.status<>'cancelled'`, cid)
	return r.GetChecklist(ctx, a, cid)
}

// AssetInput adds an asset to hand over or take back.
type AssetInput struct {
	Name   string `json:"name"`
	Serial string `json:"serial"`
}

func (r *Repository) hrChecklist(ctx context.Context, a Actor, cid string) error {
	if !a.IsHR {
		var ok bool
		_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.talent_checklists WHERE org_id=$1 AND id=$2 AND (it_id=$3::uuid OR hr_id=$3::uuid))`, a.OrgID, cid, a.UserID).Scan(&ok)
		if !ok {
			return ErrNotFound
		}
	}
	return nil
}

// AddAsset records an asset on a checklist (HR, or the HR/IT owner of it).
func (r *Repository) AddAsset(ctx context.Context, a Actor, cid string, in AssetInput) (json.RawMessage, error) {
	if err := r.hrChecklist(ctx, a, cid); err != nil {
		return nil, err
	}
	if clean(in.Name) == "" {
		return nil, invalid("asset name is required")
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_assets (org_id, checklist_id, name, serial) VALUES ($1,$2,$3,$4)`, a.OrgID, cid, clean(in.Name), clean(in.Serial)); err != nil {
		return nil, mapNoRows(err)
	}
	return r.GetChecklist(ctx, a, cid)
}

// SetAssetStatus marks an asset assigned, returned or lost.
func (r *Repository) SetAssetStatus(ctx context.Context, a Actor, assetID, status string) (json.RawMessage, error) {
	if !oneOf(status, "assigned", "returned", "lost") {
		return nil, invalid("status must be assigned, returned or lost")
	}
	var cid string
	if err := r.pool.QueryRow(ctx, `SELECT checklist_id::text FROM adrient.talent_assets WHERE org_id=$1 AND id=$2`, a.OrgID, assetID).Scan(&cid); err != nil {
		return nil, mapNoRows(err)
	}
	if err := r.hrChecklist(ctx, a, cid); err != nil {
		return nil, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.talent_assets SET status=$2, updated_at=now() WHERE id=$1`, assetID, status); err != nil {
		return nil, err
	}
	return r.GetChecklist(ctx, a, cid)
}

// ExitInput is an exit interview record.
type ExitInput struct {
	Reason      string            `json:"reason"`
	WouldReturn *bool             `json:"would_return"`
	Answers     map[string]string `json:"answers"`
	Notes       string            `json:"notes"`
}

// SaveExitInterview records the exit interview of an offboarding checklist. HR only
// (exit feedback is sensitive; the manager does not see it).
func (r *Repository) SaveExitInterview(ctx context.Context, a Actor, cid string, in ExitInput) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	var kind string
	if err := r.pool.QueryRow(ctx, `SELECT kind FROM adrient.talent_checklists WHERE org_id=$1 AND id=$2`, a.OrgID, cid).Scan(&kind); err != nil {
		return nil, mapNoRows(err)
	}
	if kind != "offboarding" {
		return nil, invalid("exit interviews belong to offboarding checklists")
	}
	if in.Answers == nil {
		in.Answers = map[string]string{}
	}
	ab, _ := json.Marshal(in.Answers)
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_exit_interviews (checklist_id, org_id, reason, would_return, answers, notes, by_user)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (checklist_id) DO UPDATE SET reason=EXCLUDED.reason, would_return=EXCLUDED.would_return,
		answers=EXCLUDED.answers, notes=EXCLUDED.notes, by_user=EXCLUDED.by_user, at=now()`,
		cid, a.OrgID, clean(in.Reason), in.WouldReturn, ab, in.Notes, a.UserID); err != nil {
		return nil, err
	}
	return r.GetChecklist(ctx, a, cid)
}

// MyChecklistTasks lists open checklist tasks assigned to the actor.
func (r *Repository) MyChecklistTasks(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT t.id::text AS id, t.checklist_id::text AS checklist_id, t.title, t.due_date, t.link, c.subject_name, c.kind,
		(t.due_date < current_date) AS overdue
		FROM adrient.talent_checklist_tasks t JOIN adrient.talent_checklists c ON c.id=t.checklist_id
		WHERE t.org_id=$1 AND t.assignee_id=$2 AND NOT t.done AND c.status='active' ORDER BY t.due_date`, a.OrgID, a.UserID)
}

// RemindOverdue sends one reminder mail per overdue checklist task that has not been reminded today.
// It returns the number of reminders sent. Called by the HTTP endpoint HR uses ("Send reminders").
func (r *Repository) RemindOverdue(ctx context.Context, a Actor) (int, error) {
	if !a.IsHR {
		return 0, ErrForbidden
	}
	if r.Deps.Mail == nil {
		return 0, ErrUnavailable
	}
	rows, err := r.pool.Query(ctx, `SELECT t.id::text, u.email, t.title, c.subject_name, t.due_date::text
		FROM adrient.talent_checklist_tasks t JOIN adrient.talent_checklists c ON c.id=t.checklist_id JOIN adrient.users u ON u.id=t.assignee_id
		WHERE t.org_id=$1 AND NOT t.done AND c.status='active' AND t.due_date < current_date AND (t.reminded_at IS NULL OR t.reminded_at < now() - interval '20 hours')`, a.OrgID)
	if err != nil {
		return 0, err
	}
	type rem struct{ id, to, title, who, due string }
	var list []rem
	for rows.Next() {
		var x rem
		if err := rows.Scan(&x.id, &x.to, &x.title, &x.who, &x.due); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, x)
	}
	rows.Close()
	n := 0
	for _, x := range list {
		msg := "Reminder: \"" + x.title + "\" for " + x.who + " was due on " + x.due + "."
		if err := r.Deps.Mail(ctx, []string{x.to}, "Overdue task: "+x.title, "<p>"+htmlEscape(msg)+"</p>", msg); err == nil {
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.talent_checklist_tasks SET reminded_at=now() WHERE id=$1`, x.id)
			n++
		}
	}
	return n, nil
}

func (h *HTTPHandler) onboarding(rt route) bool {
	w, r, a, ctx := rt.w, rt.r, rt.a, rt.r.Context()
	id := rt.id()
	switch rt.seg[0] {
	case "templates":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.ListTemplates(ctx, a)
			replyRaw(w, "templates", raw, err)
		case rt.is(1, http.MethodPost), rt.is(2, http.MethodPut):
			var in TemplateInput
			if decode(w, r, &in) {
				raw, err := h.repo.SaveTemplate(ctx, a, id, in)
				replyRawBody(w, raw, err)
			}
		case rt.is(2, http.MethodDelete):
			replyOK(w, h.repo.DeleteTemplate(ctx, a, id))
		default:
			return false
		}
	case "checklists":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.ListChecklists(ctx, a)
			replyRaw(w, "checklists", raw, err)
		case rt.is(1, http.MethodPost):
			var in StartInput
			if decode(w, r, &in) {
				raw, err := h.repo.StartChecklist(ctx, a, in)
				replyRawBody(w, raw, err)
			}
		case rt.is(2, http.MethodGet):
			raw, err := h.repo.GetChecklist(ctx, a, id)
			replyRawBody(w, raw, err)
		case rt.sub("assets", http.MethodPost):
			var in AssetInput
			if decode(w, r, &in) {
				raw, err := h.repo.AddAsset(ctx, a, id, in)
				replyRawBody(w, raw, err)
			}
		case rt.sub("exit-interview", http.MethodPost):
			var in ExitInput
			if decode(w, r, &in) {
				raw, err := h.repo.SaveExitInterview(ctx, a, id, in)
				replyRawBody(w, raw, err)
			}
		case rt.is(1, http.MethodGet):
		default:
			return false
		}
	case "checklist-tasks":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.MyChecklistTasks(ctx, a)
			replyRaw(w, "tasks", raw, err)
		case rt.sub("done", http.MethodPost):
			var b struct {
				Done bool `json:"done"`
			}
			if decode(w, r, &b) {
				raw, err := h.repo.SetTaskDone(ctx, a, id, b.Done)
				replyRawBody(w, raw, err)
			}
		case rt.is(1, http.MethodPost):
			n, err := h.repo.RemindOverdue(ctx, a)
			reply(w, map[string]int{"sent": n}, err)
		default:
			return false
		}
	case "assets":
		if rt.sub("status", http.MethodPost) {
			var b struct {
				Status string `json:"status"`
			}
			if decode(w, r, &b) {
				raw, err := h.repo.SetAssetStatus(ctx, a, id, b.Status)
				replyRawBody(w, raw, err)
			}
		} else {
			return false
		}
	default:
		return false
	}
	return true
}
