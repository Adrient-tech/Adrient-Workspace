package tickets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// ErrInvalid marks bad input.
var ErrInvalid = errors.New("invalid input")

// Deps connects help-desk extras to other modules.
type Deps struct {
	// Mail sends an email; nil when mail is not configured.
	Mail func(ctx context.Context, to []string, subject, htmlBody, text string) error
	// BaseURL is the public origin used in survey links (no trailing slash).
	BaseURL string
	// Notify creates an in-app notification; nil when the notification centre is not wired.
	Notify func(ctx context.Context, org, user, title, body, url string) error
	// CreateTask files a task in the Tasks module for an automation; nil when not wired.
	CreateTask func(ctx context.Context, org, assignee, title, description, link string) error
	// CreateIssue files an issue in Projects (team "" = first team) and returns its id and key.
	CreateIssue func(ctx context.Context, org, user, teamID, title, description string) (id, key string, err error)
	// LogTime records worked minutes in the Time tracking module; nil when not wired.
	LogTime func(ctx context.Context, org, user string, minutes int, date, note string, billable bool) error
}

// ---- SLA ----

// SLAPolicies returns the policies of a project.
func (r *Repository) SLAPolicies(ctx context.Context, org, projectID string) ([]SLAPolicy, error) {
	rows, err := r.pool.Query(ctx, `SELECT s.priority, s.first_response_min, s.resolve_min FROM adrient.ticket_sla_policies s
		JOIN adrient.ticket_projects p ON p.id=s.project_id WHERE s.project_id=$1 AND p.org_id=$2
		ORDER BY CASE s.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END`, projectID, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SLAPolicy{}
	for rows.Next() {
		var p SLAPolicy
		if err := rows.Scan(&p.Priority, &p.FirstResponseMin, &p.ResolveMin); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) projectInOrg(ctx context.Context, org, projectID string) error {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_projects WHERE id=$1 AND org_id=$2`, projectID, org).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveSLAPolicies replaces the policies of a project.
func (r *Repository) SaveSLAPolicies(ctx context.Context, org, projectID string, ps []SLAPolicy) error {
	if err := ValidateSLA(ps); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.ticket_sla_policies WHERE project_id=$1`, projectID); err != nil {
		return err
	}
	for _, p := range ps {
		if p.FirstResponseMin == 0 && p.ResolveMin == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.ticket_sla_policies (project_id, priority, first_response_min, resolve_min) VALUES ($1,$2,$3,$4)`,
			projectID, p.Priority, p.FirstResponseMin, p.ResolveMin); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ---- canned replies ----

// Canned is a saved reply.
type Canned struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

// ListCanned returns replies for a project (and those shared by all projects).
func (r *Repository) ListCanned(ctx context.Context, org, projectID string) ([]Canned, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, COALESCE(project_id::text,''), title, body FROM adrient.ticket_canned
		WHERE org_id=$1 AND (project_id IS NULL OR project_id::text=$2) ORDER BY title`, org, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Canned{}
	for rows.Next() {
		var c Canned
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Title, &c.Body); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateCanned stores a reply. An empty project id shares it with all projects.
func (r *Repository) CreateCanned(ctx context.Context, org, user, projectID, title, body string) (Canned, error) {
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if title == "" || utf8.RuneCountInString(title) > 100 || body == "" || utf8.RuneCountInString(body) > 5000 {
		return Canned{}, fmt.Errorf("%w: a title (up to 100) and a reply (up to 5000 characters) are required", ErrInvalid)
	}
	if projectID != "" {
		if err := r.projectInOrg(ctx, org, projectID); err != nil {
			return Canned{}, err
		}
	}
	var c Canned
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_canned (org_id, project_id, title, body, created_by)
		VALUES ($1, NULLIF($2,'')::uuid, $3, $4, NULLIF($5,'')::uuid) RETURNING id::text, COALESCE(project_id::text,''), title, body`,
		org, projectID, title, body, user).Scan(&c.ID, &c.ProjectID, &c.Title, &c.Body)
	return c, err
}

// DeleteCanned removes a reply.
func (r *Repository) DeleteCanned(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_canned WHERE id=$1 AND org_id=$2`, id, org)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- rules ----

// ListRules returns the rules of a project in order.
func (r *Repository) ListRules(ctx context.Context, org, projectID string) ([]Rule, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, enabled, match_keyword, match_priority, match_source,
		COALESCE(set_assignee::text,''), set_priority, set_status FROM adrient.ticket_rules WHERE org_id=$1 AND project_id=$2 ORDER BY position, created_at`, org, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var x Rule
		if err := rows.Scan(&x.ID, &x.Name, &x.Enabled, &x.MatchKeyword, &x.MatchPriority, &x.MatchSource, &x.SetAssignee, &x.SetPriority, &x.SetStatus); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SaveRule creates (empty id) or updates a rule.
func (r *Repository) SaveRule(ctx context.Context, org, projectID string, x Rule) (Rule, error) {
	if err := x.Validate(); err != nil {
		return Rule{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return Rule{}, err
	}
	if x.SetAssignee != "" {
		var n int
		if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.users WHERE id=$1 AND org_id=$2`, x.SetAssignee, org).Scan(&n); err != nil || n == 0 {
			return Rule{}, fmt.Errorf("%w: the assignee is not in your organisation", ErrInvalid)
		}
	}
	if x.ID == "" {
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_rules (org_id, project_id, name, enabled, match_keyword, match_priority, match_source, set_assignee, set_priority, set_status, position)
			VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,$10,
			 COALESCE((SELECT MAX(position)+1 FROM adrient.ticket_rules WHERE project_id=$2),0)) RETURNING id::text`,
			org, projectID, x.Name, x.Enabled, x.MatchKeyword, x.MatchPriority, x.MatchSource, x.SetAssignee, x.SetPriority, x.SetStatus).Scan(&x.ID)
		return x, err
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.ticket_rules SET name=$4, enabled=$5, match_keyword=$6, match_priority=$7, match_source=$8,
		set_assignee=NULLIF($9,'')::uuid, set_priority=$10, set_status=$11 WHERE id=$1 AND org_id=$2 AND project_id=$3`,
		x.ID, org, projectID, x.Name, x.Enabled, x.MatchKeyword, x.MatchPriority, x.MatchSource, x.SetAssignee, x.SetPriority, x.SetStatus)
	if err != nil {
		return Rule{}, err
	}
	if tag.RowsAffected() == 0 {
		return Rule{}, ErrNotFound
	}
	return x, nil
}

// DeleteRule removes a rule.
func (r *Repository) DeleteRule(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_rules WHERE id=$1 AND org_id=$2`, id, org)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// applyRules runs the first matching rule on a new ticket. It never fails the
// ticket: any error is logged and the ticket stays as filed.
func (r *Repository) applyRules(ctx context.Context, t Ticket) Ticket {
	rules, err := r.ListRules(ctx, t.OrgID, t.ProjectID)
	if err != nil || len(rules) == 0 {
		return t
	}
	m := FirstMatch(rules, t.Title, t.Body, t.Priority, t.Source)
	if m == nil {
		return t
	}
	status, pri := t.Status, t.Priority
	if m.SetStatus != "" {
		status = m.SetStatus
	}
	if m.SetPriority != "" {
		pri = m.SetPriority
	}
	var assignee *string
	if m.SetAssignee != "" {
		assignee = &m.SetAssignee
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET status=$2, priority=$3, assignee_user_id=COALESCE($4::uuid, assignee_user_id) WHERE id=$1`,
		t.ID, status, pri, assignee); err != nil {
		slog.Warn("tickets: rule apply failed", "rule", m.Name, "err", err)
		return t
	}
	t.Status, t.Priority = status, pri
	if assignee != nil {
		t.AssigneeUserID = assignee
	}
	return t
}

// ---- customers ----

// Customers groups the org's tickets by requester email.
func (r *Repository) Customers(ctx context.Context, org, projectID, q string) ([]Customer, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	rows, err := r.pool.Query(ctx, `SELECT lower(requester_email), max(requester_name), count(*),
		count(*) FILTER (WHERE resolved_at IS NULL AND status NOT IN ('resolved','closed')), max(created_at)
		FROM adrient.tickets WHERE org_id=$1 AND requester_email <> '' AND ($2='' OR project_id::text=$2)
		  AND ($3='' OR lower(requester_email) LIKE '%'||$3||'%' OR lower(requester_name) LIKE '%'||$3||'%')
		GROUP BY lower(requester_email) ORDER BY max(created_at) DESC LIMIT 300`, org, projectID, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Customer{}
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.Email, &c.Name, &c.Total, &c.Open, &c.LastAt); err != nil {
			return nil, err
		}
		c.Organisation = OrgOf(c.Email)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- CSAT ----

func newCSATToken() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return "cs_" + hex.EncodeToString(b)
}

// EnsureCSAT returns the survey token of a ticket, creating it if needed.
func (r *Repository) EnsureCSAT(ctx context.Context, org, ticketID string) (string, error) {
	var tok string
	err := r.pool.QueryRow(ctx, `SELECT token FROM adrient.ticket_csat WHERE ticket_id=$1 AND org_id=$2`, ticketID, org).Scan(&tok)
	if err == nil {
		return tok, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	tok = newCSATToken()
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.ticket_csat (ticket_id, org_id, token) VALUES ($1,$2,$3) ON CONFLICT (ticket_id) DO NOTHING`, ticketID, org, tok)
	if err != nil {
		return "", err
	}
	err = r.pool.QueryRow(ctx, `SELECT token FROM adrient.ticket_csat WHERE ticket_id=$1`, ticketID).Scan(&tok)
	return tok, err
}

// CSATInfo is what the survey page and the agent see.
type CSATInfo struct {
	Token    string `json:"token,omitempty"`
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Rating   *int   `json:"rating"`
	Comment  string `json:"comment"`
	Answered bool   `json:"answered"`
}

// CSATFor returns the survey state of a ticket.
func (r *Repository) CSATFor(ctx context.Context, org, ticketID string) (CSATInfo, error) {
	var c CSATInfo
	err := r.pool.QueryRow(ctx, `SELECT s.token, p.key || '-' || t.number::text, t.title, s.rating, s.comment, s.answered_at IS NOT NULL
		FROM adrient.ticket_csat s JOIN adrient.tickets t ON t.id=s.ticket_id JOIN adrient.ticket_projects p ON p.id=t.project_id
		WHERE s.ticket_id=$1 AND s.org_id=$2`, ticketID, org).Scan(&c.Token, &c.Ref, &c.Title, &c.Rating, &c.Comment, &c.Answered)
	if errors.Is(err, pgx.ErrNoRows) {
		return CSATInfo{}, ErrNotFound
	}
	return c, err
}

// CSATByToken loads the survey for a requester.
func (r *Repository) CSATByToken(ctx context.Context, token string) (CSATInfo, error) {
	if len(token) < 20 {
		return CSATInfo{}, ErrNotFound
	}
	var c CSATInfo
	err := r.pool.QueryRow(ctx, `SELECT p.key || '-' || t.number::text, t.title, s.rating, s.comment, s.answered_at IS NOT NULL
		FROM adrient.ticket_csat s JOIN adrient.tickets t ON t.id=s.ticket_id JOIN adrient.ticket_projects p ON p.id=t.project_id
		WHERE s.token=$1`, token).Scan(&c.Ref, &c.Title, &c.Rating, &c.Comment, &c.Answered)
	if errors.Is(err, pgx.ErrNoRows) {
		return CSATInfo{}, ErrNotFound
	}
	return c, err
}

// AnswerCSAT records the rating once.
func (r *Repository) AnswerCSAT(ctx context.Context, token string, rating int, comment string) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("%w: choose a rating from 1 to 5", ErrInvalid)
	}
	if utf8.RuneCountInString(comment) > 2000 {
		return fmt.Errorf("%w: comment is too long", ErrInvalid)
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.ticket_csat SET rating=$2, comment=$3, answered_at=now() WHERE token=$1 AND answered_at IS NULL`, token, rating, strings.TrimSpace(comment))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: this survey was already answered", ErrInvalid)
	}
	return nil
}

// onStatusChange maintains resolved_at and sends the satisfaction survey the
// first time a ticket is resolved.
func (r *Repository) onStatusChange(ctx context.Context, before, after Ticket) {
	done := func(s string) bool { return s == "resolved" || s == "closed" }
	switch {
	case done(after.Status) && !done(before.Status):
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.tickets SET resolved_at=now() WHERE id=$1`, after.ID)
		tok, err := r.EnsureCSAT(ctx, after.OrgID, after.ID)
		if err != nil || r.Deps.Mail == nil || r.Deps.BaseURL == "" || after.RequesterEmail == "" {
			return
		}
		ref := fmt.Sprintf("%s-%d", after.ProjectKey, after.Number)
		link := r.Deps.BaseURL + "/csat/" + tok
		text := fmt.Sprintf("Your request %s (%s) was marked %s.\n\nHow did we do? %s\n", ref, after.Title, after.Status, link)
		htm := fmt.Sprintf("<p>Your request <b>%s</b> (%s) was marked %s.</p><p><a href=\"%s\">How did we do? Rate the support you received.</a></p>",
			html.EscapeString(ref), html.EscapeString(after.Title), html.EscapeString(after.Status), html.EscapeString(link))
		if err := r.Deps.Mail(ctx, []string{after.RequesterEmail}, "How did we do? "+ref, htm, text); err != nil {
			slog.Warn("tickets: survey mail failed", "err", err)
		}
	case !done(after.Status) && done(before.Status):
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.tickets SET resolved_at=NULL WHERE id=$1`, after.ID)
	}
}

// ---- knowledge base ----

// Article is a help-centre article.
type Article struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Published bool      `json:"published"`
	UpdatedAt time.Time `json:"updated_at"`
	Category  string    `json:"category"`
	Views     int       `json:"views"`
	Helpful   int       `json:"helpful_yes"`
	NotHelpf  int       `json:"helpful_no"`
}

// ListArticles returns a project's articles (all for agents, published for the public).
func (r *Repository) ListArticles(ctx context.Context, org, projectID string, publishedOnly bool) ([]Article, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, slug, title, body, published, updated_at, category, views, helpful_yes, helpful_no FROM adrient.kb_articles
		WHERE org_id=$1 AND project_id=$2 AND (NOT $3 OR published) ORDER BY category, title`, org, projectID, publishedOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Article{}
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Body, &a.Published, &a.UpdatedAt, &a.Category, &a.Views, &a.Helpful, &a.NotHelpf); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveArticle creates (empty id) or updates an article.
func (r *Repository) SaveArticle(ctx context.Context, org, user, projectID string, a Article) (Article, error) {
	a.Title = strings.TrimSpace(a.Title)
	if a.Title == "" || utf8.RuneCountInString(a.Title) > 150 || utf8.RuneCountInString(a.Body) > 50000 {
		return Article{}, fmt.Errorf("%w: a title (up to 150) and a body (up to 50000 characters) are required", ErrInvalid)
	}
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return Article{}, err
	}
	if a.ID == "" {
		base := Slugify(a.Title)
		for i := 0; i < 20; i++ {
			slug := base
			if i > 0 {
				slug = fmt.Sprintf("%s-%d", base, i+1)
			}
			err := r.pool.QueryRow(ctx, `INSERT INTO adrient.kb_articles (org_id, project_id, slug, title, body, published, created_by, category)
				VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,$8) ON CONFLICT (project_id, slug) DO NOTHING RETURNING id::text, updated_at`,
				org, projectID, slug, a.Title, a.Body, a.Published, user, strings.TrimSpace(a.Category)).Scan(&a.ID, &a.UpdatedAt)
			if err == nil {
				a.Slug = slug
				return a, nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return Article{}, err
			}
		}
		return Article{}, fmt.Errorf("%w: could not make a unique link for this title", ErrInvalid)
	}
	err := r.pool.QueryRow(ctx, `UPDATE adrient.kb_articles SET title=$4, body=$5, published=$6, category=$7, updated_at=now()
		WHERE id=$1 AND org_id=$2 AND project_id=$3 RETURNING slug, updated_at`, a.ID, org, projectID, a.Title, a.Body, a.Published, strings.TrimSpace(a.Category)).Scan(&a.Slug, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Article{}, ErrNotFound
	}
	return a, err
}

// DeleteArticle removes an article.
func (r *Repository) DeleteArticle(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.kb_articles WHERE id=$1 AND org_id=$2`, id, org)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// PublicHelp returns the published articles of a project by its public token.
func (r *Repository) PublicHelp(ctx context.Context, token string) (name string, list []Article, err error) {
	p, err := r.projectByToken(ctx, token)
	if err != nil {
		return "", nil, err
	}
	list, err = r.ListArticles(ctx, p.OrgID, p.ID, true)
	return p.Name, list, err
}

// ---- report ----

// ReportFor builds the report for the last `days` days of a project.
func (r *Repository) ReportFor(ctx context.Context, org, projectID string, days int, now time.Time) (Report, error) {
	if days < 1 || days > 365 {
		days = 30
	}
	pols, err := r.SLAPolicies(ctx, org, projectID)
	if err != nil {
		return Report{}, err
	}
	byPri := map[string]*SLAPolicy{}
	for i := range pols {
		byPri[pols[i].Priority] = &pols[i]
	}
	from := now.AddDate(0, 0, -days).UTC().Truncate(24 * time.Hour)
	rows, err := r.pool.Query(ctx, `SELECT t.priority, t.created_at, t.resolved_at, COALESCE(u.display_name,''), s.rating,
		(SELECT min(c.created_at) FROM adrient.ticket_comments c WHERE c.ticket_id=t.id AND NOT c.is_internal
		   AND c.author_user_id IS NOT NULL AND c.author_user_id IS DISTINCT FROM t.requester_user_id)
		FROM adrient.tickets t LEFT JOIN adrient.users u ON u.id=t.assignee_user_id LEFT JOIN adrient.ticket_csat s ON s.ticket_id=t.id
		WHERE t.project_id=$1 AND t.org_id=$2 AND (t.created_at >= $3 OR t.resolved_at >= $3 OR t.resolved_at IS NULL)`, projectID, org, from)
	if err != nil {
		return Report{}, err
	}
	defer rows.Close()
	var list []ReportRow
	for rows.Next() {
		var x ReportRow
		if err := rows.Scan(&x.Priority, &x.Created, &x.Resolved, &x.Assignee, &x.Rating, &x.FirstResponse); err != nil {
			return Report{}, err
		}
		fr := x.FirstResponse
		if x.Resolved != nil && fr == nil {
			fr = x.Resolved
		}
		x.SLA = ComputeSLA(byPri[x.Priority], x.Created, fr, x.Resolved, now)
		list = append(list, x)
	}
	if err := rows.Err(); err != nil {
		return Report{}, err
	}
	return Summarize(list, from, now.Add(24*time.Hour).UTC().Truncate(24*time.Hour), now), nil
}
