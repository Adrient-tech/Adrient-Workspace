package projects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// ErrInvalid marks bad input.
var ErrInvalid = errors.New("invalid input")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// IssueExtras are the plan details of one issue.
type IssueExtras struct {
	StartDate     string   `json:"start_date"`
	DueDate       string   `json:"due_date"`
	MilestoneID   string   `json:"milestone_id"`
	EstimateHours *float64 `json:"estimate_hours"`
	LoggedHours   float64  `json:"logged_hours"`
	DependsOn     []string `json:"depends_on"`
	Archived      bool     `json:"archived"`
}

// ListExtras returns the plan details of every issue that has some.
func (r *Repository) ListExtras(ctx context.Context, org string) (map[string]IssueExtras, error) {
	out := map[string]IssueExtras{}
	rows, err := r.pool.Query(ctx, `SELECT issue_id::text, COALESCE(to_char(start_date,'YYYY-MM-DD'),''), COALESCE(to_char(due_date,'YYYY-MM-DD'),''),
		COALESCE(milestone_id::text,''), estimate_hours::float8, logged_hours::float8, archived FROM adrient.project_issue_extras WHERE org_id=$1`, org)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var e IssueExtras
		if err := rows.Scan(&id, &e.StartDate, &e.DueDate, &e.MilestoneID, &e.EstimateHours, &e.LoggedHours, &e.Archived); err != nil {
			rows.Close()
			return nil, err
		}
		e.DependsOn = []string{}
		out[id] = e
	}
	rows.Close()
	drows, err := r.pool.Query(ctx, `SELECT issue_id::text, depends_on_id::text FROM adrient.project_issue_deps WHERE org_id=$1`, org)
	if err != nil {
		return nil, err
	}
	defer drows.Close()
	for drows.Next() {
		var a, b string
		if err := drows.Scan(&a, &b); err != nil {
			return nil, err
		}
		e, ok := out[a]
		if !ok {
			e = IssueExtras{DependsOn: []string{}}
		}
		e.DependsOn = append(e.DependsOn, b)
		out[a] = e
	}
	return out, drows.Err()
}

func (r *Repository) allDeps(ctx context.Context, org string) (map[string][]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT issue_id::text, depends_on_id::text FROM adrient.project_issue_deps WHERE org_id=$1`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string][]string{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		m[a] = append(m[a], b)
	}
	return m, rows.Err()
}

// SaveIssueExtras stores dates, estimate, milestone and prerequisites.
func (r *Repository) SaveIssueExtras(ctx context.Context, org, issueID string, in IssueExtras) error {
	if err := r.issueInOrg(ctx, org, issueID); err != nil {
		return err
	}
	start, due, err := DateRange(in.StartDate, in.DueDate)
	if err != nil {
		return invalid("%v", err)
	}
	if in.EstimateHours != nil && (*in.EstimateHours < 0 || *in.EstimateHours > 10000) {
		return invalid("the estimate must be between 0 and 10000 hours")
	}
	if len(in.DependsOn) > MaxDeps {
		return invalid("at most %d prerequisites", MaxDeps)
	}
	seen := map[string]bool{}
	var deps []string
	for _, d := range in.DependsOn {
		if d == issueID {
			return invalid("an issue cannot depend on itself")
		}
		if !seen[d] {
			seen[d] = true
			deps = append(deps, d)
		}
	}
	for _, d := range deps {
		if err := r.issueInOrg(ctx, org, d); err != nil {
			return invalid("a prerequisite does not exist")
		}
	}
	all, err := r.allDeps(ctx, org)
	if err != nil {
		return err
	}
	if WouldCycle(all, issueID, deps) {
		return invalid("those prerequisites would make a loop")
	}
	if in.MilestoneID != "" {
		var n int
		if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.project_milestones WHERE id=$1 AND org_id=$2`, in.MilestoneID, org).Scan(&n); err != nil || n == 0 {
			return invalid("unknown milestone")
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.project_issue_extras (issue_id, org_id, start_date, due_date, milestone_id, estimate_hours)
		VALUES ($1,$2,$3,$4,NULLIF($5,'')::uuid,$6) ON CONFLICT (issue_id) DO UPDATE SET start_date=$3, due_date=$4,
		milestone_id=NULLIF($5,'')::uuid, estimate_hours=$6, updated_at=now()`, issueID, org, start, due, in.MilestoneID, in.EstimateHours); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.project_issue_deps WHERE issue_id=$1`, issueID); err != nil {
		return err
	}
	for _, d := range deps {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.project_issue_deps (issue_id, depends_on_id, org_id) VALUES ($1,$2,$3)`, issueID, d, org); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// LogHours adds time spent to an issue.
func (r *Repository) LogHours(ctx context.Context, org, issueID string, hours float64) (float64, error) {
	if hours <= 0 || hours > 24 {
		return 0, invalid("log between 0 and 24 hours at a time")
	}
	if err := r.issueInOrg(ctx, org, issueID); err != nil {
		return 0, err
	}
	var total float64
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.project_issue_extras (issue_id, org_id, logged_hours) VALUES ($1,$2,$3)
		ON CONFLICT (issue_id) DO UPDATE SET logged_hours = adrient.project_issue_extras.logged_hours + $3, updated_at=now()
		RETURNING logged_hours::float8`, issueID, org, hours).Scan(&total)
	return total, err
}

// ---- milestones ----

// Milestone is a dated goal inside a project.
type Milestone struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	DueDate   string `json:"due_date"`
	Done      bool   `json:"done"`
}

func (r *Repository) projectInOrg(ctx context.Context, org, id string) error {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.project_projects WHERE id=$1 AND org_id=$2 AND trashed_at IS NULL`, id, org).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListMilestones returns the milestones of one project, or all when projectID is empty.
func (r *Repository) ListMilestones(ctx context.Context, org, projectID string) ([]Milestone, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, project_id::text, name, COALESCE(to_char(due_date,'YYYY-MM-DD'),''), done FROM adrient.project_milestones
		WHERE org_id=$1 AND ($2='' OR project_id::text=$2) ORDER BY due_date NULLS LAST, created_at`, org, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Milestone{}
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Name, &m.DueDate, &m.Done); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SaveMilestone creates (empty id) or updates a milestone.
func (r *Repository) SaveMilestone(ctx context.Context, org, projectID string, m Milestone) (Milestone, error) {
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" || utf8.RuneCountInString(m.Name) > 120 {
		return m, invalid("a name of 1 to 120 characters is required")
	}
	_, due, err := DateRange("", m.DueDate)
	if err != nil {
		return m, invalid("%v", err)
	}
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return m, err
	}
	if m.ID == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.project_milestones (org_id, project_id, name, due_date, done) VALUES ($1,$2,$3,$4,$5) RETURNING id::text`,
			org, projectID, m.Name, due, m.Done).Scan(&m.ID)
		m.ProjectID = projectID
		return m, err
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.project_milestones SET name=$4, due_date=$5, done=$6 WHERE id=$1 AND org_id=$2 AND project_id=$3`, m.ID, org, projectID, m.Name, due, m.Done)
	if err != nil {
		return m, err
	}
	if tag.RowsAffected() == 0 {
		return m, ErrNotFound
	}
	m.ProjectID = projectID
	return m, nil
}

// DeleteMilestone removes a milestone; issues in it keep their other details.
func (r *Repository) DeleteMilestone(ctx context.Context, org, id string) error {
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.project_issue_extras SET milestone_id=NULL WHERE milestone_id=$1 AND org_id=$2`, id, org)
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.project_milestones WHERE id=$1 AND org_id=$2`, id, org)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- brief ----

// GetBrief returns the project brief text.
func (r *Repository) GetBrief(ctx context.Context, org, projectID string) (string, error) {
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return "", err
	}
	var b string
	err := r.pool.QueryRow(ctx, `SELECT body FROM adrient.project_briefs WHERE project_id=$1`, projectID).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return b, err
}

// SaveBrief stores the project brief.
func (r *Repository) SaveBrief(ctx context.Context, org, user, projectID, body string) error {
	if utf8.RuneCountInString(body) > 50000 {
		return invalid("the brief is too long (50000 characters at most)")
	}
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.project_briefs (project_id, org_id, body, updated_by) VALUES ($1,$2,$3,NULLIF($4,'')::uuid)
		ON CONFLICT (project_id) DO UPDATE SET body=$3, updated_by=NULLIF($4,'')::uuid, updated_at=now()`, projectID, org, body, user)
	return err
}

// ---- saved views ----

// SavedView stores a named set of filters.
type SavedView struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Mode    string          `json:"mode"`
	Filters json.RawMessage `json:"filters"`
	Shared  bool            `json:"shared"`
	Mine    bool            `json:"mine"`
}

var viewModes = map[string]bool{"list": true, "board": true, "my": true, "timeline": true}

// ListViews returns the user's views and the ones shared with the org.
func (r *Repository) ListViews(ctx context.Context, org, user string) ([]SavedView, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, mode, filters, shared, owner_id::text = $2 FROM adrient.project_saved_views
		WHERE org_id=$1 AND (owner_id::text=$2 OR shared) ORDER BY name`, org, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SavedView{}
	for rows.Next() {
		var v SavedView
		if err := rows.Scan(&v.ID, &v.Name, &v.Mode, &v.Filters, &v.Shared, &v.Mine); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CreateView saves a view.
func (r *Repository) CreateView(ctx context.Context, org, user string, v SavedView) (SavedView, error) {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || utf8.RuneCountInString(v.Name) > 80 {
		return v, invalid("a name of 1 to 80 characters is required")
	}
	if !viewModes[v.Mode] {
		return v, invalid("unknown view type")
	}
	if len(v.Filters) == 0 {
		v.Filters = json.RawMessage(`{}`)
	}
	if len(v.Filters) > 4096 || !json.Valid(v.Filters) {
		return v, invalid("the filters are not valid")
	}
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.project_saved_views (org_id, owner_id, name, mode, filters, shared) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		org, user, v.Name, v.Mode, []byte(v.Filters), v.Shared).Scan(&v.ID)
	v.Mine = true
	return v, err
}

// DeleteView removes one of the user's views.
func (r *Repository) DeleteView(ctx context.Context, org, user, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.project_saved_views WHERE id=$1 AND org_id=$2 AND owner_id=$3`, id, org, user)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- links to CRM and HR ----

// Link ties a project to a CRM deal or an HR employee.
type Link struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	RefID string `json:"ref_id"`
	Label string `json:"label"`
}

// ListLinks returns the links of a project with readable labels.
func (r *Repository) ListLinks(ctx context.Context, org, projectID string) ([]Link, error) {
	rows, err := r.pool.Query(ctx, `SELECT l.id::text, l.kind, l.ref_id::text,
		COALESCE((SELECT d.title || ' (' || d.stage || ')' FROM adrient.crm_deals d WHERE l.kind='crm_deal' AND d.id=l.ref_id AND d.org_id=l.org_id),
		         (SELECT e.first_name || ' ' || e.last_name FROM adrient.hr_employees e WHERE l.kind='hr_employee' AND e.id=l.ref_id AND e.org_id=l.org_id), '(removed)')
		FROM adrient.project_links l WHERE l.org_id=$1 AND l.project_id=$2 ORDER BY l.created_at`, org, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Link{}
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ID, &l.Kind, &l.RefID, &l.Label); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Pickable is a CRM deal or HR employee that can be linked.
type Pickable struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Pick lists candidates for a link kind.
func (r *Repository) Pick(ctx context.Context, org, kind, q string) ([]Pickable, error) {
	q = "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	var rows pgx.Rows
	var err error
	switch kind {
	case "crm_deal":
		rows, err = r.pool.Query(ctx, `SELECT id::text, title || ' (' || stage || ')' FROM adrient.crm_deals WHERE org_id=$1 AND lower(title) LIKE $2 ORDER BY updated_at DESC LIMIT 30`, org, q)
	case "hr_employee":
		rows, err = r.pool.Query(ctx, `SELECT id::text, first_name || ' ' || last_name FROM adrient.hr_employees WHERE org_id=$1 AND lower(first_name || ' ' || last_name) LIKE $2 ORDER BY first_name LIMIT 30`, org, q)
	default:
		return nil, invalid("kind must be crm_deal or hr_employee")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Pickable{}
	for rows.Next() {
		var p Pickable
		if err := rows.Scan(&p.ID, &p.Label); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddLink links a project to a deal or employee of the same org.
func (r *Repository) AddLink(ctx context.Context, org, projectID, kind, ref string) error {
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return err
	}
	var n int
	switch kind {
	case "crm_deal":
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.crm_deals WHERE id=$1 AND org_id=$2`, ref, org).Scan(&n)
	case "hr_employee":
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.hr_employees WHERE id=$1 AND org_id=$2`, ref, org).Scan(&n)
	default:
		return invalid("kind must be crm_deal or hr_employee")
	}
	if n == 0 {
		return invalid("that item was not found")
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.project_links (org_id, project_id, kind, ref_id) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, org, projectID, kind, ref)
	return err
}

// DeleteLink removes a link.
func (r *Repository) DeleteLink(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.project_links WHERE id=$1 AND org_id=$2`, id, org)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- templates ----

// FromTemplate creates a project with its milestones, issues, dependencies and
// brief from a built-in template, starting on `start` (YYYY-MM-DD).
func (r *Repository) FromTemplate(ctx context.Context, org, user, templateID, name, teamID, start string) (Project, error) {
	tpl, ok := FindTemplate(templateID)
	if !ok {
		return Project{}, invalid("unknown template")
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 120 {
		return Project{}, invalid("a project name is required")
	}
	startT, _, err := DateRange(start, "")
	if err != nil || startT == nil {
		return Project{}, invalid("a start date like 2030-01-31 is required")
	}
	if _, err := r.GetTeam(ctx, org, teamID); err != nil {
		return Project{}, invalid("choose a team for the issues")
	}
	day := func(n int) string { return startT.AddDate(0, 0, n).Format("2006-01-02") }
	last := 0
	for _, is := range tpl.Issues {
		if e := is.StartDay + is.Days; e > last {
			last = e
		}
	}
	p, err := r.CreateProject(ctx, org, ProjectFields{Name: name, Description: tpl.Description, State: "planned", LeadID: user, TargetDate: day(last)})
	if err != nil {
		return Project{}, err
	}
	// Roll back what was created if a later step fails, so a half-built project is not left behind.
	fail := func(e error) (Project, error) {
		_ = r.DeleteProject(ctx, org, p.ID)
		return Project{}, e
	}
	if err := r.SaveBrief(ctx, org, user, p.ID, tpl.Brief); err != nil {
		return fail(err)
	}
	ms := make([]string, len(tpl.Milestones))
	for i, m := range tpl.Milestones {
		saved, err := r.SaveMilestone(ctx, org, p.ID, Milestone{Name: m.Name, DueDate: day(m.Day)})
		if err != nil {
			return fail(err)
		}
		ms[i] = saved.ID
	}
	ids := make([]string, len(tpl.Issues))
	for i, is := range tpl.Issues {
		iss, err := r.CreateIssue(ctx, org, teamID, user, IssueFields{Title: is.Title, Description: is.Description, Status: "todo", Priority: is.Priority, ProjectID: p.ID})
		if err != nil {
			return fail(err)
		}
		ids[i] = iss.ID
		var deps []string
		for _, n := range is.Needs {
			deps = append(deps, ids[n])
		}
		hours := is.Hours
		mid := ""
		if is.Milestone >= 0 && is.Milestone < len(ms) {
			mid = ms[is.Milestone]
		}
		if err := r.SaveIssueExtras(ctx, org, iss.ID, IssueExtras{StartDate: day(is.StartDay), DueDate: day(is.StartDay + is.Days - 1), MilestoneID: mid, EstimateHours: &hours, DependsOn: deps}); err != nil {
			return fail(err)
		}
	}
	return p, nil
}

// today is split out so tests can fake it if needed.
var today = func() time.Time { return time.Now() }
