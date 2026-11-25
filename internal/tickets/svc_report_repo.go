package tickets

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReportOpts filters a report.
type ReportOpts struct {
	Days     int    `json:"days"`
	From     string `json:"from"` // YYYY-MM-DD, overrides Days when both set
	To       string `json:"to"`
	Group    string `json:"group"`
	Type     string `json:"type"`
	Priority string `json:"priority"`
}

// Bucket is a labelled count.
type Bucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// Workload is one agent's open and resolved counts.
type Workload struct {
	Agent    string `json:"agent"`
	Open     int    `json:"open"`
	Resolved int    `json:"resolved"`
}

// ReportV2 is the full desk report.
type ReportV2 struct {
	Report
	From         string     `json:"from"`
	To           string     `json:"to"`
	ByType       []Bucket   `json:"by_type"`
	ByCategory   []Bucket   `json:"by_category"`
	ByGroup      []Bucket   `json:"by_group"`
	Backlog      []Bucket   `json:"backlog"`
	Workload     []Workload `json:"workload"`
	SLAByType    []Bucket   `json:"sla_breached_by_priority"`
	TotalMinutes int        `json:"time_logged_min"`
}

func (o ReportOpts) window(now time.Time) (time.Time, time.Time, error) {
	to := now.Add(24 * time.Hour).UTC().Truncate(24 * time.Hour)
	if o.To != "" {
		d, err := time.Parse("2006-01-02", o.To)
		if err != nil {
			return to, to, fmt.Errorf("%w: to must be YYYY-MM-DD", ErrInvalid)
		}
		to = d.Add(24 * time.Hour)
	}
	from := to.AddDate(0, 0, -30)
	if o.Days >= 1 && o.Days <= 730 {
		from = now.AddDate(0, 0, -o.Days).UTC().Truncate(24 * time.Hour)
	}
	if o.From != "" {
		d, err := time.Parse("2006-01-02", o.From)
		if err != nil {
			return from, to, fmt.Errorf("%w: from must be YYYY-MM-DD", ErrInvalid)
		}
		from = d
	}
	if !to.After(from) || to.Sub(from) > 731*24*time.Hour {
		return from, to, fmt.Errorf("%w: choose a period between 1 and 730 days", ErrInvalid)
	}
	return from, to, nil
}

type reportTicket struct {
	row          ReportRow
	ttype, cat   string
	group        string
	status       string
	assigneeName string
	done         bool
}

func sortedBuckets(m map[string]int, limit int) []Bucket {
	out := make([]Bucket, 0, len(m))
	for k, v := range m {
		if k == "" {
			k = "(none)"
		}
		out = append(out, Bucket{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ReportV2 builds the filtered report for a project.
func (r *Repository) ReportV2(ctx context.Context, org, projectID string, o ReportOpts, now time.Time) (ReportV2, error) {
	from, to, err := o.window(now)
	if err != nil {
		return ReportV2{}, err
	}
	rs, err := r.newSLAResolver(ctx, org, projectID)
	if err != nil {
		return ReportV2{}, err
	}
	args := []any{projectID, org, from, to}
	where := `t.project_id=$1 AND t.org_id=$2 AND t.merged_into IS NULL AND (t.created_at >= $3 AND t.created_at < $4 OR t.resolved_at >= $3 AND t.resolved_at < $4 OR ` + notDoneSQL("t") + `)`
	if o.Group != "" {
		args = append(args, o.Group)
		where += fmt.Sprintf(" AND t.group_id::text=$%d", len(args))
	}
	if o.Type != "" {
		args = append(args, o.Type)
		where += fmt.Sprintf(" AND t.ticket_type=$%d", len(args))
	}
	if o.Priority != "" {
		args = append(args, o.Priority)
		where += fmt.Sprintf(" AND t.priority=$%d", len(args))
	}
	rows, err := r.pool.Query(ctx, `SELECT t.priority, t.ticket_type, t.category, COALESCE(g.name,''), t.status, t.requester_email, COALESCE(t.group_id::text,''),
		t.created_at, t.resolved_at, COALESCE(u.display_name,''), s.rating,
		COALESCE(t.first_response_at, (SELECT min(c.created_at) FROM adrient.ticket_comments c WHERE c.ticket_id=t.id AND NOT c.is_internal
		   AND c.author_user_id IS NOT NULL AND c.author_user_id IS DISTINCT FROM t.requester_user_id)), t.sla_paused_at, t.sla_paused_min,
		`+notDoneSQL("t")+`
		FROM adrient.tickets t LEFT JOIN adrient.users u ON u.id=t.assignee_user_id LEFT JOIN adrient.ticket_csat s ON s.ticket_id=t.id
		LEFT JOIN adrient.ticket_groups g ON g.id=t.group_id
		WHERE `+where+` LIMIT 100000`, args...)
	if err != nil {
		return ReportV2{}, err
	}
	defer rows.Close()
	var list []ReportRow
	var all []reportTicket
	for rows.Next() {
		var x ReportRow
		var rt reportTicket
		var in slaInputs
		var notDone bool
		if err := rows.Scan(&x.Priority, &rt.ttype, &rt.cat, &rt.group, &rt.status, &in.Email, &in.GroupID, &x.Created, &x.Resolved, &x.Assignee, &x.Rating,
			&x.FirstResponse, &in.PausedAt, &in.PausedMin, &notDone); err != nil {
			return ReportV2{}, err
		}
		in.Priority, in.Type, in.Created, in.Resolved, in.FirstResponse = x.Priority, rt.ttype, x.Created, x.Resolved, x.FirstResponse
		x.SLA = rs.eval(ctx, in, now)
		rt.row, rt.assigneeName, rt.done = x, x.Assignee, !notDone
		all = append(all, rt)
		list = append(list, x)
	}
	if err := rows.Err(); err != nil {
		return ReportV2{}, err
	}
	out := ReportV2{Report: Summarize(list, from, to, now), From: from.Format("2006-01-02"), To: to.Add(-time.Second).Format("2006-01-02")}
	byType, byCat, byGroup := map[string]int{}, map[string]int{}, map[string]int{}
	breach := map[string]int{}
	work := map[string]*Workload{}
	age := []int{0, 0, 0, 0, 0}
	for _, t := range all {
		inPeriod := !t.row.Created.Before(from) && t.row.Created.Before(to)
		if inPeriod {
			byType[t.ttype]++
			byCat[t.cat]++
			byGroup[t.group]++
		}
		name := t.assigneeName
		if name == "" {
			name = "(unassigned)"
		}
		w := work[name]
		if w == nil {
			w = &Workload{Agent: name}
			work[name] = w
		}
		if !t.done {
			w.Open++
			d := now.Sub(t.row.Created)
			switch {
			case d < 24*time.Hour:
				age[0]++
			case d < 72*time.Hour:
				age[1]++
			case d < 7*24*time.Hour:
				age[2]++
			case d < 30*24*time.Hour:
				age[3]++
			default:
				age[4]++
			}
		} else if t.row.Resolved != nil && !t.row.Resolved.Before(from) && t.row.Resolved.Before(to) {
			w.Resolved++
		}
		if inPeriod && (t.row.SLA.Resolution.State == SLABreached || t.row.SLA.FirstResponse.State == SLABreached) {
			breach[t.row.Priority]++
		}
	}
	out.ByType, out.ByCategory, out.ByGroup = sortedBuckets(byType, 0), sortedBuckets(byCat, 8), sortedBuckets(byGroup, 0)
	out.SLAByType = sortedBuckets(breach, 0)
	out.Backlog = []Bucket{{"< 1 day", age[0]}, {"1-3 days", age[1]}, {"3-7 days", age[2]}, {"7-30 days", age[3]}, {"> 30 days", age[4]}}
	for _, w := range work {
		out.Workload = append(out.Workload, *w)
	}
	sort.Slice(out.Workload, func(i, j int) bool {
		return out.Workload[i].Open+out.Workload[i].Resolved > out.Workload[j].Open+out.Workload[j].Resolved
	})
	if len(out.Workload) > 30 {
		out.Workload = out.Workload[:30]
	}
	_ = r.pool.QueryRow(ctx, `SELECT COALESCE(sum(e.minutes),0) FROM adrient.ticket_time_entries e JOIN adrient.tickets t ON t.id=e.ticket_id
		WHERE t.project_id=$1 AND e.org_id=$2 AND e.work_date >= $3 AND e.work_date < $4`, projectID, org, from, to).Scan(&out.TotalMinutes)
	if out.ByType == nil {
		out.ByType = []Bucket{}
	}
	if out.Workload == nil {
		out.Workload = []Workload{}
	}
	return out, nil
}

// csvSafe neutralises spreadsheet formulas in exported cells.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// ExportCSV writes the tickets of a period as CSV (up to 50,000 rows).
func (r *Repository) ExportCSV(ctx context.Context, org, projectID string, o ReportOpts, now time.Time) ([]byte, error) {
	from, to, err := o.window(now)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT p.key||'-'||t.number, t.title, t.status, t.priority, t.ticket_type, t.category, COALESCE(g.name,''),
		COALESCE(u.display_name,''), t.requester_name, t.requester_email, t.source, array_to_string(t.tags,' '), t.created_at, t.resolved_at, t.first_response_at
		FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id=t.project_id LEFT JOIN adrient.users u ON u.id=t.assignee_user_id
		LEFT JOIN adrient.ticket_groups g ON g.id=t.group_id
		WHERE t.org_id=$1 AND t.project_id=$2 AND t.created_at >= $3 AND t.created_at < $4
		  AND ($5='' OR t.group_id::text=$5) AND ($6='' OR t.ticket_type=$6) AND ($7='' OR t.priority=$7)
		ORDER BY t.created_at DESC LIMIT 50000`, org, projectID, from, to, o.Group, o.Type, o.Priority)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Ticket", "Subject", "Status", "Priority", "Type", "Category", "Group", "Assignee", "Requester", "Requester email", "Source", "Tags", "Created", "Resolved", "First response", "Resolution minutes"})
	ts := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	for rows.Next() {
		var ref, title, status, pri, typ, cat, grp, asg, rn, re, src, tags string
		var created time.Time
		var resolved, first *time.Time
		if err := rows.Scan(&ref, &title, &status, &pri, &typ, &cat, &grp, &asg, &rn, &re, &src, &tags, &created, &resolved, &first); err != nil {
			return nil, err
		}
		res := ""
		if resolved != nil {
			res = strconv.Itoa(int(resolved.Sub(created) / time.Minute))
		}
		_ = w.Write([]string{ref, csvSafe(title), status, pri, typ, csvSafe(cat), csvSafe(grp), csvSafe(asg), csvSafe(rn), csvSafe(re), src, csvSafe(tags),
			created.UTC().Format(time.RFC3339), ts(resolved), ts(first), res})
	}
	w.Flush()
	return buf.Bytes(), rows.Err()
}

// SavedReport is a named set of report filters.
type SavedReport struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Params ReportOpts `json:"params"`
}

// ListSavedReports returns a project's saved reports.
func (r *Repository) ListSavedReports(ctx context.Context, org, projectID string) ([]SavedReport, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, params FROM adrient.ticket_saved_reports WHERE org_id=$1 AND project_id=$2 ORDER BY lower(name)`, org, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SavedReport{}
	for rows.Next() {
		var s SavedReport
		var raw []byte
		if err := rows.Scan(&s.ID, &s.Name, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &s.Params)
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveReport stores a report definition.
func (r *Repository) SaveReport(ctx context.Context, org, user, projectID string, s SavedReport) (SavedReport, error) {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" || len(s.Name) > 80 {
		return s, fmt.Errorf("%w: report name is required", ErrInvalid)
	}
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return s, err
	}
	raw, _ := json.Marshal(s.Params)
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_saved_reports (org_id,project_id,name,params,created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id::text`,
		org, projectID, s.Name, raw, nullUUID(user)).Scan(&s.ID)
	return s, err
}

// DeleteSavedReport removes a saved report.
func (r *Repository) DeleteSavedReport(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_saved_reports WHERE org_id=$1 AND id=$2`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ---- knowledge base extras --------------------------------------------------------

// SuggestArticles finds published articles that match free text (used while a requester types a
// ticket and by agents drafting a reply).
func (r *Repository) SuggestArticles(ctx context.Context, org, projectID, q string, publishedOnly bool) ([]Article, error) {
	words := []string{}
	for _, w := range strings.Fields(strings.ToLower(q)) {
		w = strings.Trim(w, ".,;:!?()[]\"'")
		if len(w) >= 3 {
			words = append(words, w)
		}
		if len(words) >= 8 {
			break
		}
	}
	if len(words) == 0 {
		return []Article{}, nil
	}
	score := "0"
	args := []any{org, projectID, publishedOnly}
	for _, w := range words {
		args = append(args, "%"+likeEscape(w)+"%")
		n := len(args)
		score += fmt.Sprintf(" + (CASE WHEN a.title ILIKE $%d THEN 3 ELSE 0 END) + (CASE WHEN a.body ILIKE $%d THEN 1 ELSE 0 END)", n, n)
	}
	rows, err := r.pool.Query(ctx, `SELECT a.id::text, a.slug, a.title, left(a.body, 400), a.published, a.updated_at FROM adrient.kb_articles a
		WHERE a.org_id=$1 AND a.project_id=$2 AND (NOT $3 OR a.published) AND (`+score+`) > 0 ORDER BY (`+score+`) DESC, a.updated_at DESC LIMIT 5`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Article{}
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Body, &a.Published, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ArticleStats is the knowledge base health summary.
type ArticleStats struct {
	Articles  int `json:"articles"`
	Published int `json:"published"`
	Views     int `json:"views"`
	Helpful   int `json:"helpful_yes"`
	NotHelp   int `json:"helpful_no"`
	Deflected int `json:"deflected"`
}

// KBStats sums article counters of a project.
func (r *Repository) KBStats(ctx context.Context, org, projectID string) (ArticleStats, error) {
	var s ArticleStats
	err := r.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE published), COALESCE(sum(views),0), COALESCE(sum(helpful_yes),0), COALESCE(sum(helpful_no),0), COALESCE(sum(deflected),0)
		FROM adrient.kb_articles WHERE org_id=$1 AND project_id=$2`, org, projectID).Scan(&s.Articles, &s.Published, &s.Views, &s.Helpful, &s.NotHelp, &s.Deflected)
	return s, err
}

// ArticleFeedback records a public reader's vote. Only published articles can be voted on.
func (r *Repository) ArticleFeedback(ctx context.Context, token, slug string, helpful, deflected bool) error {
	p, err := r.projectByToken(ctx, token)
	if err != nil {
		return err
	}
	col := "helpful_no"
	if helpful {
		col = "helpful_yes"
	}
	q := `UPDATE adrient.kb_articles SET ` + col + `=` + col + `+1`
	if deflected {
		q += `, deflected=deflected+1`
	}
	tag, err := r.pool.Exec(ctx, q+` WHERE project_id=$1 AND slug=$2 AND published`, p.ID, slug)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ArticleView counts a view of a published article.
func (r *Repository) ArticleView(ctx context.Context, token, slug string) {
	if p, err := r.projectByToken(ctx, token); err == nil {
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.kb_articles SET views=views+1 WHERE project_id=$1 AND slug=$2 AND published`, p.ID, slug)
	}
}

// ---- request forms --------------------------------------------------------------

// FormField is one question on a request form.
type FormField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"` // text | longtext | select | date | checkbox
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

// RequestForm is a portal form for one kind of request.
type RequestForm struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	TicketType  string      `json:"ticket_type"`
	GroupID     string      `json:"group_id"`
	Category    string      `json:"category"`
	Fields      []FormField `json:"fields"`
	Enabled     bool        `json:"enabled"`
	Position    int         `json:"position"`
	Skills      []string    `json:"skills"`
}

var formKeyOK = func(k string) bool {
	if k == "" || len(k) > 32 {
		return false
	}
	for _, c := range k {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

// Validate checks a form definition.
func (f *RequestForm) Validate() error {
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" || len(f.Name) > 80 {
		return errors.New("form name is required (max 80 characters)")
	}
	f.TicketType = NormalizeType(f.TicketType)
	if len(f.Fields) > 30 {
		return errors.New("at most 30 fields")
	}
	seen := map[string]bool{}
	for i := range f.Fields {
		fl := &f.Fields[i]
		fl.Key = strings.ToLower(strings.TrimSpace(fl.Key))
		fl.Label = strings.TrimSpace(fl.Label)
		if !formKeyOK(fl.Key) || seen[fl.Key] {
			return fmt.Errorf("field %d needs a unique key of letters, digits and underscores", i+1)
		}
		seen[fl.Key] = true
		if fl.Label == "" {
			return fmt.Errorf("field %s needs a label", fl.Key)
		}
		switch fl.Type {
		case "text", "longtext", "date", "checkbox":
		case "select":
			if len(fl.Options) < 2 || len(fl.Options) > 50 {
				return fmt.Errorf("select field %s needs between 2 and 50 options", fl.Key)
			}
		default:
			return fmt.Errorf("unknown field type %q", fl.Type)
		}
	}
	return nil
}

// ValidateAnswers checks submitted values against the form and renders the ticket body.
func (f RequestForm) ValidateAnswers(values map[string]string) (body string, custom map[string]string, err error) {
	custom = map[string]string{}
	var b strings.Builder
	for _, fl := range f.Fields {
		v := strings.TrimSpace(values[fl.Key])
		if len(v) > 5000 {
			return "", nil, fmt.Errorf("%s is too long", fl.Label)
		}
		if fl.Required && (v == "" || (fl.Type == "checkbox" && v != "true")) {
			return "", nil, fmt.Errorf("%s is required", fl.Label)
		}
		if v == "" {
			continue
		}
		switch fl.Type {
		case "select":
			ok := false
			for _, o := range fl.Options {
				ok = ok || o == v
			}
			if !ok {
				return "", nil, fmt.Errorf("%s: choose one of the listed options", fl.Label)
			}
		case "date":
			if _, e := time.Parse("2006-01-02", v); e != nil {
				return "", nil, fmt.Errorf("%s must be a date (YYYY-MM-DD)", fl.Label)
			}
		case "checkbox":
			if v != "true" && v != "false" {
				return "", nil, fmt.Errorf("%s must be yes or no", fl.Label)
			}
		}
		custom[fl.Key] = v
		fmt.Fprintf(&b, "%s: %s\n", fl.Label, v)
	}
	return strings.TrimSpace(b.String()), custom, nil
}

// ListForms returns a project's forms (enabledOnly for the portal).
func (r *Repository) ListForms(ctx context.Context, org, projectID string, enabledOnly bool) ([]RequestForm, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, description, ticket_type, COALESCE(group_id::text,''), category, fields, enabled, position, skills
		FROM adrient.ticket_forms WHERE org_id=$1 AND project_id=$2 AND (NOT $3 OR enabled) ORDER BY position, name`, org, projectID, enabledOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RequestForm{}
	for rows.Next() {
		var f RequestForm
		var raw []byte
		if err := rows.Scan(&f.ID, &f.Name, &f.Description, &f.TicketType, &f.GroupID, &f.Category, &raw, &f.Enabled, &f.Position, &f.Skills); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &f.Fields)
		if f.Skills == nil {
			f.Skills = []string{}
		}
		if f.Fields == nil {
			f.Fields = []FormField{}
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SaveForm creates or updates a form.
func (r *Repository) SaveForm(ctx context.Context, org, projectID string, f RequestForm) (RequestForm, error) {
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return f, err
	}
	if err := f.Validate(); err != nil {
		return f, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	raw, _ := json.Marshal(f.Fields)
	var err error
	if f.ID == "" {
		var n int
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_forms WHERE project_id=$1`, projectID).Scan(&n)
		if n >= 30 {
			return f, fmt.Errorf("%w: at most 30 forms per project", ErrInvalid)
		}
		f.Position = n
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_forms (org_id,project_id,name,description,ticket_type,group_id,category,fields,enabled,position,skills)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, org, projectID, f.Name, strings.TrimSpace(f.Description), f.TicketType, nullUUID(f.GroupID), strings.TrimSpace(f.Category), raw, f.Enabled, f.Position, NormalizeTags(f.Skills)).Scan(&f.ID)
	} else {
		var id string
		err = r.pool.QueryRow(ctx, `UPDATE adrient.ticket_forms SET name=$4,description=$5,ticket_type=$6,group_id=$7,category=$8,fields=$9,enabled=$10,position=$11,skills=$12
			WHERE id=$1 AND org_id=$2 AND project_id=$3 RETURNING id::text`, f.ID, org, projectID, f.Name, strings.TrimSpace(f.Description), f.TicketType, nullUUID(f.GroupID), strings.TrimSpace(f.Category), raw, f.Enabled, f.Position, NormalizeTags(f.Skills)).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return f, ErrNotFound
		}
	}
	return f, err
}

// DeleteForm removes a form.
func (r *Repository) DeleteForm(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_forms WHERE org_id=$1 AND id=$2`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// SubmitForm files a ticket from a portal form.
func (r *Repository) SubmitForm(ctx context.Context, token, formID, name, email, title string, values map[string]string) (Ticket, string, error) {
	p, err := r.projectByToken(ctx, token)
	if err != nil {
		return Ticket{}, "", err
	}
	forms, err := r.ListForms(ctx, p.OrgID, p.ID, true)
	if err != nil {
		return Ticket{}, "", err
	}
	var form *RequestForm
	for i := range forms {
		if forms[i].ID == formID {
			form = &forms[i]
		}
	}
	if form == nil {
		return Ticket{}, "", ErrNotFound
	}
	body, custom, verr := form.ValidateAnswers(values)
	if verr != nil {
		return Ticket{}, "", fmt.Errorf("%w: %v", ErrInvalid, verr)
	}
	if strings.TrimSpace(title) == "" {
		title = form.Name
	}
	ctx = WithCreateOpts(ctx, CreateOpts{GroupID: form.GroupID, Type: form.TicketType, Category: form.Category, FormID: form.ID, Custom: custom, NeedSkills: form.Skills})
	t, err := r.CreateTicket(ctx, p.OrgID, p.ID, title, body, "normal", nil, name, email, "portal")
	if err != nil {
		return Ticket{}, "", err
	}
	return t, fmt.Sprintf("%s-%d", p.Key, t.Number), nil
}
