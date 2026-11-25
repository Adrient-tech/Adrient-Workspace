package timetracking

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opsdoc"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
	"github.com/Adrient-tech/Adrient-Workspace/internal/payrollhours"
)

// Caller is the signed-in person. Admin means org admin or HR admin.
type Caller struct {
	OrgID      string
	UserID     string
	Admin      bool
	HRAdmin    bool
	EmployeeID string
}

// Repository is the Postgres store for Time tracking.
type Repository struct {
	pool     *pgxpool.Pool
	docs     *opsdoc.Store
	hr       *hr.Repository
	inv      opshttp.InvoiceWriter
	contacts opshttp.ContactDirectory
	now      func() time.Time
}

// NewRepository builds the store. inv and contacts may be nil (the matching features then report "not available").
func NewRepository(pool *pgxpool.Pool, hrRepo *hr.Repository, inv opshttp.InvoiceWriter, contacts opshttp.ContactDirectory) *Repository {
	return &Repository{pool: pool, docs: opsdoc.New(pool, "tt_docs"), hr: hrRepo, inv: inv, contacts: contacts, now: time.Now}
}

// CallerFor resolves roles.
func (r *Repository) CallerFor(ctx context.Context, org, user string, orgAdmin bool) (Caller, error) {
	c := Caller{OrgID: org, UserID: user, Admin: orgAdmin}
	if r.hr != nil {
		a, err := r.hr.ActorFor(ctx, org, user, orgAdmin)
		if err != nil {
			return c, err
		}
		c.EmployeeID, c.HRAdmin = a.EmployeeID, a.IsHR
		c.Admin = c.Admin || a.IsHR
	}
	return c, nil
}

func (c Caller) needAdmin() error {
	if !c.Admin {
		return opshttp.Forbidden("only admins can manage clients, projects, billing and payroll export")
	}
	return nil
}

func setID[T any](f func(*T, string)) func(*T, opsdoc.Doc) {
	return func(v *T, d opsdoc.Doc) { f(v, d.ID) }
}

// ---- settings ----

func defaultSettings() Settings {
	return Settings{Currency: "INR", TermsDays: 15, WarnLongDayHrs: 16, PayrollRates: map[string]float64{}}
}

// GetSettings returns the organisation defaults.
func (r *Repository) GetSettings(ctx context.Context, org string) (Settings, string, error) {
	s := defaultSettings()
	l, err := r.docs.List(ctx, org, "settings", opsdoc.Filter{Limit: 1})
	if err != nil || len(l) == 0 {
		return s, "", err
	}
	v, err := opsdoc.Decode[Settings](l[0], nil)
	if err == nil {
		s = v
		if s.PayrollRates == nil {
			s.PayrollRates = map[string]float64{}
		}
	}
	return s, l[0].ID, err
}

// SaveSettings validates and stores the defaults (admins only).
func (r *Repository) SaveSettings(ctx context.Context, c Caller, s Settings) (Settings, error) {
	if err := c.needAdmin(); err != nil {
		return s, err
	}
	allowed := map[int]bool{0: true, 5: true, 6: true, 10: true, 15: true, 30: true, 60: true}
	if s.DefaultRate < 0 || s.DefaultRate > 1e6 || s.PayrollRate < 0 || s.PayrollRate > 1e6 || s.TaxRate < 0 || s.TaxRate > 100 || s.TermsDays < 0 || s.TermsDays > 365 {
		return s, opshttp.Invalid("rates, tax or payment terms are out of range")
	}
	if !allowed[s.Rounding] || s.MinMinutes < 0 || s.MinMinutes > 480 {
		return s, opshttp.Invalid("rounding must be 0, 5, 6, 10, 15, 30 or 60 minutes; the minimum at most 480")
	}
	s.Currency = strings.ToUpper(strings.TrimSpace(s.Currency))
	if len(s.Currency) != 3 {
		s.Currency = "INR"
	}
	if s.WarnLongDayHrs <= 0 || s.WarnLongDayHrs > 24 {
		s.WarnLongDayHrs = 16
	}
	for k, v := range s.PayrollRates {
		if !opshttp.IsUUID(k) || v < 0 || v > 1e6 {
			return s, opshttp.Invalid("payroll rates must be per HR employee with a sensible amount")
		}
	}
	_, id, err := r.GetSettings(ctx, c.OrgID)
	if err != nil {
		return s, err
	}
	_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "settings", id, "", "", s)
	return s, err
}

// ---- clients and projects ----

// ListClients returns clients; admins also get the unbilled amount per client.
func (r *Repository) ListClients(ctx context.Context, c Caller, includeArchived bool) ([]Client, error) {
	l, err := r.docs.List(ctx, c.OrgID, "client", opsdoc.Filter{})
	if err != nil {
		return nil, err
	}
	out := []Client{}
	for _, cl := range opsdoc.DecodeAll(l, setID(func(v *Client, id string) { v.ID = id })) {
		if cl.Archived && !includeArchived {
			continue
		}
		if !c.Admin {
			cl.Rate, cl.TaxRate, cl.TermsDays, cl.Rounding, cl.MinMinutes = nil, nil, nil, nil, nil
		}
		out = append(out, cl)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	if c.Admin {
		un, _ := r.unbilledByClient(ctx, c)
		for i := range out {
			out[i].Unbilled, out[i].UnbilledSec = un[out[i].ID].amount, un[out[i].ID].sec
		}
	}
	return out, nil
}

// SaveClient creates (id == "") or updates a client (admins only).
func (r *Repository) SaveClient(ctx context.Context, c Caller, id string, cl Client) (Client, error) {
	if err := c.needAdmin(); err != nil {
		return cl, err
	}
	cl.Name = strings.TrimSpace(cl.Name)
	if cl.Name == "" || len(cl.Name) > 120 {
		return cl, opshttp.Invalid("a client needs a name of up to 120 characters")
	}
	if (cl.Rate != nil && (*cl.Rate < 0 || *cl.Rate > 1e6)) || (cl.TaxRate != nil && (*cl.TaxRate < 0 || *cl.TaxRate > 100)) ||
		(cl.Rounding != nil && (*cl.Rounding < 0 || *cl.Rounding > 60)) || (cl.MinMinutes != nil && (*cl.MinMinutes < 0 || *cl.MinMinutes > 480)) ||
		(cl.TermsDays != nil && (*cl.TermsDays < 0 || *cl.TermsDays > 365)) {
		return cl, opshttp.Invalid("billing values are out of range")
	}
	if cl.ContactID != "" && r.contacts != nil {
		ref, err := r.contacts.Get(ctx, c.OrgID, cl.ContactID)
		if err != nil {
			return cl, opshttp.Invalid("that contact does not exist")
		}
		if cl.Email == "" {
			cl.Email = ref.Email
		}
	}
	cl.Unbilled, cl.UnbilledSec = 0, 0
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "client", id, "", "", cl)
	cl.ID = d.ID
	return cl, err
}

// ListProjects returns projects (members see the active ones, admins can ask for archived too).
func (r *Repository) ListProjects(ctx context.Context, c Caller, includeArchived bool) ([]Project, error) {
	l, err := r.docs.List(ctx, c.OrgID, "project", opsdoc.Filter{})
	if err != nil {
		return nil, err
	}
	out := []Project{}
	for _, p := range opsdoc.DecodeAll(l, setID(func(v *Project, id string) { v.ID = id })) {
		if p.Archived && !(includeArchived && c.Admin) {
			continue
		}
		if !c.Admin {
			p.Rate = nil
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// SaveProject creates or updates a project (admins only).
func (r *Repository) SaveProject(ctx context.Context, c Caller, id string, p Project) (Project, error) {
	if err := c.needAdmin(); err != nil {
		return p, err
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 120 {
		return p, opshttp.Invalid("a project needs a name of up to 120 characters")
	}
	if (p.Rate != nil && (*p.Rate < 0 || *p.Rate > 1e6)) || p.BudgetHours < 0 || p.BudgetAmount < 0 {
		return p, opshttp.Invalid("rate and budgets must not be negative")
	}
	if p.ClientID != "" {
		if _, err := r.docs.Get(ctx, c.OrgID, "client", p.ClientID); err != nil {
			return p, opshttp.Invalid("that client does not exist")
		}
	}
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "project", id, "", "", p)
	p.ID = d.ID
	return p, err
}

func (r *Repository) resolver(ctx context.Context, org string) (Resolver, error) {
	s, _, err := r.GetSettings(ctx, org)
	if err != nil {
		return Resolver{}, err
	}
	res := Resolver{Settings: s, Projects: map[string]Project{}, Clients: map[string]Client{}}
	pl, err := r.docs.List(ctx, org, "project", opsdoc.Filter{})
	if err != nil {
		return res, err
	}
	for _, p := range opsdoc.DecodeAll(pl, setID(func(v *Project, id string) { v.ID = id })) {
		res.Projects[p.ID] = p
	}
	cl, err := r.docs.List(ctx, org, "client", opsdoc.Filter{})
	if err != nil {
		return res, err
	}
	for _, x := range opsdoc.DecodeAll(cl, setID(func(v *Client, id string) { v.ID = id })) {
		res.Clients[x.ID] = x
	}
	return res, nil
}

// ---- entries ----

func entryOf(d opsdoc.Doc) (Entry, error) {
	return opsdoc.Decode(d, func(e *Entry, d opsdoc.Doc) { e.ID, e.UserID, e.Date = d.ID, d.Ref, d.Day })
}

func (r *Repository) userNames(ctx context.Context, org string, ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, display_name FROM adrient.users WHERE org_id=$1 AND id = ANY($2::uuid[])`, org, ids)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, n string
		if rows.Scan(&id, &n) == nil {
			out[id] = n
		}
	}
	return out
}

// EntryFilter narrows ListEntries.
type EntryFilter struct {
	From, To  string
	UserID    string // "" = everyone (admins) or yourself; "me" = yourself
	ProjectID string
	ClientID  string
	Unbilled  bool
}

// ListEntries returns entries with billing numbers. Members only ever see
// their own entries and no amounts.
func (r *Repository) ListEntries(ctx context.Context, c Caller, f EntryFilter) ([]Entry, error) {
	if (f.From != "" && !opshttp.ValidDate(f.From)) || (f.To != "" && !opshttp.ValidDate(f.To)) {
		return nil, opshttp.Invalid("from and to must be dates")
	}
	ref := f.UserID
	if ref == "me" || !c.Admin {
		ref = c.UserID
	}
	l, err := r.docs.List(ctx, c.OrgID, "entry", opsdoc.Filter{Ref: ref, From: f.From, To: f.To})
	if err != nil {
		return nil, err
	}
	res, err := r.resolver(ctx, c.OrgID)
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	out := []Entry{}
	for _, d := range l {
		e, err := entryOf(d)
		if err != nil {
			continue
		}
		if f.ProjectID != "" && e.ProjectID != f.ProjectID {
			continue
		}
		if f.ClientID != "" && res.Projects[e.ProjectID].ClientID != f.ClientID {
			continue
		}
		if f.Unbilled && (e.InvoiceID != "" || !res.Billable(e)) {
			continue
		}
		e.Billed = res.BilledSeconds(e)
		if c.Admin {
			a := res.Amount(e)
			e.Amount = &a
		} else {
			e.Rate, e.LockedRate = nil, nil
		}
		ids[e.UserID] = true
		out = append(out, e)
	}
	idl := make([]string, 0, len(ids))
	for id := range ids {
		idl = append(idl, id)
	}
	nm := r.userNames(ctx, c.OrgID, idl)
	for i := range out {
		out[i].UserName = nm[out[i].UserID]
	}
	return out, nil
}

// EntryResult is a saved entry with non-blocking warnings.
type EntryResult struct {
	Entry    Entry    `json:"entry"`
	Warnings []string `json:"warnings"`
}

func (r *Repository) validateEntry(ctx context.Context, c Caller, e *Entry, res Resolver, creating bool) error {
	if !opshttp.ValidDate(e.Date) {
		return opshttp.Invalid("a valid date is required")
	}
	if e.Date > r.now().AddDate(0, 0, 1).Format("2006-01-02") {
		return opshttp.Invalid("time cannot be logged for future days")
	}
	if e.Start != "" || e.End != "" {
		if !opshttp.ValidTime(e.Start) || !opshttp.ValidTime(e.End) {
			return opshttp.Invalid("start and end must both be HH:MM")
		}
		if e.Seconds == 0 {
			s, _ := time.Parse("15:04", e.Start)
			t, _ := time.Parse("15:04", e.End)
			d := int(t.Sub(s).Seconds())
			if d <= 0 {
				d += 86400
			}
			e.Seconds = d
		}
	}
	if e.Seconds < 1 || e.Seconds > 86400 {
		return opshttp.Invalid("a block of time must be between 1 second and 24 hours")
	}
	p, ok := res.Projects[e.ProjectID]
	if !ok {
		return opshttp.Invalid("choose a project")
	}
	if p.Archived && creating {
		return opshttp.Invalid("that project is archived")
	}
	e.Task, e.Note = strings.TrimSpace(e.Task), strings.TrimSpace(e.Note)
	if len(e.Task) > 120 || len(e.Note) > 1000 {
		return opshttp.Invalid("task or note is too long")
	}
	if e.Rate != nil && (*e.Rate < 0 || *e.Rate > 1e6) {
		return opshttp.Invalid("rate is out of range")
	}
	return nil
}

func (r *Repository) entryWarnings(ctx context.Context, c Caller, e Entry, res Resolver) []string {
	var w []string
	if r.hr != nil {
		if emp := r.hr.EmployeeIDForUser(ctx, c.OrgID, e.UserID); emp != "" {
			if sp, err := r.hr.ApprovedLeave(ctx, c.OrgID, e.Date, e.Date); err == nil {
				for _, s := range sp {
					if s.EmployeeID == emp {
						w = append(w, fmt.Sprintf("%s is on approved leave (%s) on %s.", "This person", s.TypeName, e.Date))
						break
					}
				}
			}
		}
	}
	day, _ := r.docs.List(ctx, c.OrgID, "entry", opsdoc.Filter{Ref: e.UserID, From: e.Date, To: e.Date})
	total := 0
	for _, d := range day {
		if x, err := entryOf(d); err == nil && x.ID != e.ID {
			total += x.Seconds
		}
	}
	total += e.Seconds
	if limit := res.Settings.WarnLongDayHrs; limit > 0 && float64(total) > limit*3600 {
		w = append(w, fmt.Sprintf("%.1f hours are logged on %s, more than %.0f hours in a day.", float64(total)/3600, e.Date, limit))
	}
	return w
}

// SaveEntry creates (id == "") or edits an entry. Admins may log for others.
func (r *Repository) SaveEntry(ctx context.Context, c Caller, id string, e Entry) (EntryResult, error) {
	res, err := r.resolver(ctx, c.OrgID)
	if err != nil {
		return EntryResult{}, err
	}
	owner := c.UserID
	if id != "" {
		d, err := r.docs.Get(ctx, c.OrgID, "entry", id)
		if err != nil {
			return EntryResult{}, err
		}
		old, _ := entryOf(d)
		if old.UserID != c.UserID && !c.Admin {
			return EntryResult{}, opshttp.ErrNotFound
		}
		if old.InvoiceID != "" {
			return EntryResult{}, opshttp.Conflict("this time is already on an invoice and can no longer be changed")
		}
		owner = old.UserID
		e.InvoiceID, e.LockedRate, e.LockedRound, e.LockedMinimum = "", nil, nil, nil
	} else if e.UserID != "" && e.UserID != c.UserID {
		if !c.Admin {
			return EntryResult{}, opshttp.Forbidden("you can only log your own time")
		}
		owner = e.UserID
	}
	if !c.Admin {
		e.Rate = nil
	}
	e.UserID = owner
	if err := r.validateEntry(ctx, c, &e, res, id == ""); err != nil {
		return EntryResult{}, err
	}
	e.ID, e.Billed, e.Amount, e.UserName = "", 0, nil, ""
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "entry", id, owner, e.Date, e)
	if err != nil {
		return EntryResult{}, err
	}
	out, _ := entryOf(d)
	out.Billed = res.BilledSeconds(out)
	if c.Admin {
		a := res.Amount(out)
		out.Amount = &a
	}
	return EntryResult{Entry: out, Warnings: r.entryWarnings(ctx, c, out, res)}, nil
}

// DeleteEntry removes an entry that is not on an invoice.
func (r *Repository) DeleteEntry(ctx context.Context, c Caller, id string) error {
	d, err := r.docs.Get(ctx, c.OrgID, "entry", id)
	if err != nil {
		return err
	}
	e, _ := entryOf(d)
	if e.UserID != c.UserID && !c.Admin {
		return opshttp.ErrNotFound
	}
	if e.InvoiceID != "" {
		return opshttp.Conflict("this time is already on an invoice")
	}
	return r.docs.Delete(ctx, c.OrgID, "entry", id)
}

// SetCell sets the total of one project on one day for the caller (the weekly
// grid): it updates the first uninvoiced entry and removes duplicates.
func (r *Repository) SetCell(ctx context.Context, c Caller, projectID, date string, seconds int) (int, error) {
	if seconds < 0 || seconds > 86400 {
		return 0, opshttp.Invalid("a day cell must be between 0 and 24 hours")
	}
	l, err := r.docs.List(ctx, c.OrgID, "entry", opsdoc.Filter{Ref: c.UserID, From: date, To: date})
	if err != nil {
		return 0, err
	}
	var mine []Entry
	for _, d := range l {
		if e, err := entryOf(d); err == nil && e.ProjectID == projectID {
			if e.InvoiceID != "" {
				return 0, opshttp.Conflict("part of this cell is already invoiced")
			}
			mine = append(mine, e)
		}
	}
	if seconds == 0 {
		for _, e := range mine {
			if err := r.docs.Delete(ctx, c.OrgID, "entry", e.ID); err != nil {
				return 0, err
			}
		}
		return 0, nil
	}
	if len(mine) == 0 {
		_, err := r.SaveEntry(ctx, c, "", Entry{ProjectID: projectID, Date: date, Seconds: seconds, Billable: true})
		return seconds, err
	}
	first := mine[0]
	first.Seconds, first.Start, first.End = seconds, "", ""
	if _, err := r.SaveEntry(ctx, c, first.ID, first); err != nil {
		return 0, err
	}
	for _, e := range mine[1:] {
		_ = r.docs.Delete(ctx, c.OrgID, "entry", e.ID)
	}
	return seconds, nil
}

// ---- timer ----

// Timer is a running timer.
type Timer struct {
	ProjectID string `json:"project_id"`
	Task      string `json:"task"`
	Billable  bool   `json:"billable"`
	StartedAt string `json:"started_at"`
	Elapsed   int    `json:"elapsed_seconds"`
}

// GetTimer returns the caller's running timer (nil when none).
func (r *Repository) GetTimer(ctx context.Context, c Caller) (*Timer, error) {
	var t Timer
	var started time.Time
	err := r.pool.QueryRow(ctx, `SELECT project_id, task, billable, started_at FROM adrient.tt_timers WHERE org_id=$1 AND user_id=$2::uuid`, c.OrgID, c.UserID).
		Scan(&t.ProjectID, &t.Task, &t.Billable, &started)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.StartedAt = started.Format(time.RFC3339)
	t.Elapsed = max(0, int(r.now().Sub(started).Seconds()))
	return &t, nil
}

// StartTimer starts a timer; a running one is stopped and saved first.
func (r *Repository) StartTimer(ctx context.Context, c Caller, projectID, task string, billable bool) (*Timer, *EntryResult, error) {
	res, err := r.resolver(ctx, c.OrgID)
	if err != nil {
		return nil, nil, err
	}
	if p, ok := res.Projects[projectID]; !ok || p.Archived {
		return nil, nil, opshttp.Invalid("choose an active project to track time on")
	}
	if len(task) > 120 {
		return nil, nil, opshttp.Invalid("task is too long")
	}
	var saved *EntryResult
	if cur, _ := r.GetTimer(ctx, c); cur != nil {
		e, err := r.StopTimer(ctx, c)
		if err != nil {
			return nil, nil, err
		}
		saved = e
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.tt_timers (org_id, user_id, project_id, task, billable) VALUES ($1,$2::uuid,$3,$4,$5)
		ON CONFLICT (org_id, user_id) DO UPDATE SET project_id=EXCLUDED.project_id, task=EXCLUDED.task, billable=EXCLUDED.billable, started_at=now()`,
		c.OrgID, c.UserID, projectID, strings.TrimSpace(task), billable); err != nil {
		return nil, nil, err
	}
	t, err := r.GetTimer(ctx, c)
	return t, saved, err
}

// StopTimer stops the caller's timer and saves the elapsed time as an entry
// (rounded to whole seconds, at least one second, at most 24 hours).
func (r *Repository) StopTimer(ctx context.Context, c Caller) (*EntryResult, error) {
	var t Timer
	var started time.Time
	err := r.pool.QueryRow(ctx, `DELETE FROM adrient.tt_timers WHERE org_id=$1 AND user_id=$2::uuid RETURNING project_id, task, billable, started_at`, c.OrgID, c.UserID).
		Scan(&t.ProjectID, &t.Task, &t.Billable, &started)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, opshttp.Conflict("no timer is running")
	}
	if err != nil {
		return nil, err
	}
	sec := min(86400, max(1, int(r.now().Sub(started).Seconds())))
	e := Entry{ProjectID: t.ProjectID, Date: started.Format("2006-01-02"), Seconds: sec, Billable: t.Billable, Task: t.Task,
		Start: started.Format("15:04"), End: r.now().Format("15:04")}
	if started.Format("2006-01-02") != r.now().Format("2006-01-02") {
		e.Start, e.End = "", ""
	}
	out, err := r.SaveEntry(ctx, c, "", e)
	return &out, err
}

// DiscardTimer drops the running timer without saving.
func (r *Repository) DiscardTimer(ctx context.Context, c Caller) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.tt_timers WHERE org_id=$1 AND user_id=$2::uuid`, c.OrgID, c.UserID)
	return err
}

// ---- reports ----

// ReportRow is one group of a report.
type ReportRow struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Entries int      `json:"entries"`
	Seconds int      `json:"seconds"`
	Billed  int      `json:"billed_seconds"`
	Amount  *float64 `json:"amount,omitempty"`
}

// Report groups entries by client, project, person, task or day.
func (r *Repository) Report(ctx context.Context, c Caller, f EntryFilter, groupBy string) ([]ReportRow, error) {
	entries, err := r.ListEntries(ctx, c, f)
	if err != nil {
		return nil, err
	}
	res, err := r.resolver(ctx, c.OrgID)
	if err != nil {
		return nil, err
	}
	rows := map[string]*ReportRow{}
	for _, e := range entries {
		p := res.Projects[e.ProjectID]
		var key, label string
		switch groupBy {
		case "client":
			key = p.ClientID
			label = res.Clients[p.ClientID].Name
			if label == "" {
				label = "No client"
			}
		case "user":
			key, label = e.UserID, e.UserName
		case "task":
			key, label = strings.ToLower(e.Task), e.Task
			if label == "" {
				label = "(no task)"
			}
		case "day":
			key, label = e.Date, e.Date
		case "project", "":
			key, label = e.ProjectID, p.Name
		default:
			return nil, opshttp.Invalid("group by client, project, user, task or day")
		}
		row := rows[key]
		if row == nil {
			row = &ReportRow{Key: key, Label: label}
			if c.Admin {
				z := 0.0
				row.Amount = &z
			}
			rows[key] = row
		}
		row.Entries++
		row.Seconds += e.Seconds
		row.Billed += e.Billed
		if row.Amount != nil && e.Amount != nil {
			*row.Amount = round2(*row.Amount + *e.Amount)
		}
	}
	out := make([]ReportRow, 0, len(rows))
	for _, v := range rows {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if groupBy == "day" {
			return out[i].Key < out[j].Key
		}
		return out[i].Seconds > out[j].Seconds
	})
	return out, nil
}

type unbilled struct {
	sec    int
	amount float64
}

func (r *Repository) unbilledByClient(ctx context.Context, c Caller) (map[string]unbilled, error) {
	entries, err := r.ListEntries(ctx, c, EntryFilter{Unbilled: true})
	if err != nil {
		return nil, err
	}
	res, _ := r.resolver(ctx, c.OrgID)
	out := map[string]unbilled{}
	for _, e := range entries {
		cid := res.Projects[e.ProjectID].ClientID
		u := out[cid]
		u.sec += e.Billed
		if e.Amount != nil {
			u.amount = round2(u.amount + *e.Amount)
		}
		out[cid] = u
	}
	return out, nil
}

// ---- invoicing ----

// InvoiceResult is what was billed.
type InvoiceResult struct {
	Doc     opshttp.DocRef `json:"document"`
	Entries int            `json:"entries"`
	Hours   float64        `json:"hours"`
}

// CreateInvoice bills the client's uninvoiced billable time up to `to` (and
// from `from`, when given) as an invoice in Invoicing: one line per project and
// rate. The rate, rounding and minimum are frozen on each entry, which then
// can no longer be edited.
func (r *Repository) CreateInvoice(ctx context.Context, c Caller, clientID, from, to string, entryIDs []string) (InvoiceResult, error) {
	if err := c.needAdmin(); err != nil {
		return InvoiceResult{}, err
	}
	if r.inv == nil {
		return InvoiceResult{}, opshttp.Invalid("Invoicing is not available")
	}
	res, err := r.resolver(ctx, c.OrgID)
	if err != nil {
		return InvoiceResult{}, err
	}
	cl, ok := res.Clients[clientID]
	if !ok {
		return InvoiceResult{}, opshttp.ErrNotFound
	}
	if to == "" {
		to = r.now().Format("2006-01-02")
	}
	entries, err := r.ListEntries(ctx, c, EntryFilter{From: from, To: to, ClientID: clientID, Unbilled: true})
	if err != nil {
		return InvoiceResult{}, err
	}
	pick := map[string]bool{}
	for _, id := range entryIDs {
		pick[id] = true
	}
	type key struct {
		project string
		rate    float64
	}
	groups := map[key]int{}
	var order []key
	var chosen []Entry
	for _, e := range entries {
		if len(pick) > 0 && !pick[e.ID] {
			continue
		}
		if e.Billed <= 0 {
			continue
		}
		k := key{e.ProjectID, res.Rate(e)}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] += e.Billed
		chosen = append(chosen, e)
	}
	if len(chosen) == 0 {
		return InvoiceResult{}, opshttp.Invalid("there is no unbilled time for this client in that range")
	}
	tax := res.Settings.TaxRate
	if cl.TaxRate != nil {
		tax = *cl.TaxRate
	}
	terms := res.Settings.TermsDays
	if cl.TermsDays != nil {
		terms = *cl.TermsDays
	}
	cur := res.Settings.Currency
	if cl.Currency != "" {
		cur = cl.Currency
	}
	var lines []opshttp.InvLine
	total := 0
	for _, k := range order {
		sec := groups[k]
		total += sec
		lines = append(lines, opshttp.InvLine{Description: res.Projects[k.project].Name + " (time)", Qty: Hours(sec), Unit: "h", Price: k.rate, TaxRate: tax})
	}
	issue := r.now().Format("2006-01-02")
	due := opshttp.AddDays(issue, terms)
	doc, err := r.inv.Create(ctx, c.OrgID, c.UserID, opshttp.InvoiceDraft{Kind: "invoice", ContactID: cl.ContactID, ClientName: cl.Name, ClientEmail: cl.Email,
		Currency: cur, IssueDate: issue, DueDate: due, Lines: lines, Notes: fmt.Sprintf("Time worked up to %s", to), SourceRef: "timetracking"})
	if err != nil {
		return InvoiceResult{}, err
	}
	for _, e := range chosen {
		rate := res.Rate(e)
		rd, mn := res.rounding(e), res.minimum(e)
		old, err := r.docs.Get(ctx, c.OrgID, "entry", e.ID)
		if err != nil {
			continue
		}
		cur, _ := entryOf(old)
		cur.InvoiceID, cur.LockedRate, cur.LockedRound, cur.LockedMinimum = doc.ID, &rate, &rd, &mn
		cur.ID, cur.UserName, cur.Billed, cur.Amount = "", "", 0, nil
		if _, err := r.docs.Put(ctx, c.OrgID, c.UserID, "entry", e.ID, e.UserID, e.Date, cur); err != nil {
			return InvoiceResult{}, err
		}
	}
	return InvoiceResult{Doc: doc, Entries: len(chosen), Hours: Hours(total)}, nil
}

// InvoiceRow is an invoice created from tracked time, with its live status.
type InvoiceRow struct {
	opshttp.DocRef
	Client  string  `json:"client"`
	Entries int     `json:"entries"`
	Hours   float64 `json:"hours"`
}

// Invoices lists the invoices created from time, with payment status from Invoicing (admins only).
func (r *Repository) Invoices(ctx context.Context, c Caller) ([]InvoiceRow, error) {
	if err := c.needAdmin(); err != nil {
		return nil, err
	}
	l, err := r.docs.List(ctx, c.OrgID, "entry", opsdoc.Filter{})
	if err != nil {
		return nil, err
	}
	res, _ := r.resolver(ctx, c.OrgID)
	agg := map[string]*InvoiceRow{}
	var order []string
	for _, d := range l {
		e, err := entryOf(d)
		if err != nil || e.InvoiceID == "" {
			continue
		}
		row := agg[e.InvoiceID]
		if row == nil {
			row = &InvoiceRow{Client: res.Clients[res.Projects[e.ProjectID].ClientID].Name}
			row.ID = e.InvoiceID
			agg[e.InvoiceID] = row
			order = append(order, e.InvoiceID)
		}
		row.Entries++
		row.Hours = round2(row.Hours + float64(res.BilledSeconds(e))/3600)
	}
	out := []InvoiceRow{}
	for _, id := range order {
		row := agg[id]
		if r.inv != nil {
			if d, err := r.inv.Get(ctx, c.OrgID, id); err == nil {
				row.DocRef = d
			}
		}
		out = append(out, *row)
	}
	return out, nil
}

// ---- payroll ----

// PayrollLines turns a month of tracked time into pay lines: hours × the pay
// rate of the person's HR employee record. People without an HR record or a pay
// rate are returned in skipped.
func (r *Repository) PayrollLines(ctx context.Context, c Caller, period string) ([]payrollhours.Line, []string, error) {
	if !c.HRAdmin {
		return nil, nil, opshttp.Forbidden("only HR admins can export hours to payroll")
	}
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return nil, nil, opshttp.Invalid("period must be YYYY-MM")
	}
	from, to := t.Format("2006-01-02"), t.AddDate(0, 1, -1).Format("2006-01-02")
	entries, err := r.ListEntries(ctx, c, EntryFilter{From: from, To: to})
	if err != nil {
		return nil, nil, err
	}
	s, _, _ := r.GetSettings(ctx, c.OrgID)
	secs := map[string]int{}
	names := map[string]string{}
	var order []string
	for _, e := range entries {
		if _, ok := secs[e.UserID]; !ok {
			order = append(order, e.UserID)
		}
		secs[e.UserID] += e.Seconds
		names[e.UserID] = e.UserName
	}
	var lines []payrollhours.Line
	var skipped []string
	for _, u := range order {
		emp := r.hr.EmployeeIDForUser(ctx, c.OrgID, u)
		if emp == "" {
			skipped = append(skipped, names[u]+": no HR employee record")
			continue
		}
		rate := s.PayrollRate
		if v, ok := s.PayrollRates[emp]; ok {
			rate = v
		}
		if rate <= 0 {
			skipped = append(skipped, names[u]+": no payroll rate set")
			continue
		}
		h := Hours(secs[u])
		lines = append(lines, payrollhours.Line{EmployeeID: emp, Hours: h, Amount: round2(h * rate)})
	}
	return lines, skipped, nil
}

// ExportPayroll adds the month's tracked hours to the draft HR payroll run.
func (r *Repository) ExportPayroll(ctx context.Context, c Caller, period string) (payrollhours.Result, []string, error) {
	lines, skipped, err := r.PayrollLines(ctx, c, period)
	if err != nil {
		return payrollhours.Result{}, nil, err
	}
	if len(lines) == 0 {
		return payrollhours.Result{}, skipped, opshttp.Invalid("no tracked hours with a payroll rate in that month")
	}
	a, err := r.hr.ActorFor(ctx, c.OrgID, c.UserID, c.Admin)
	if err != nil {
		return payrollhours.Result{}, skipped, err
	}
	out, err := payrollhours.Export(ctx, r.hr, a, period, "time tracking", lines)
	if errors.Is(err, hr.ErrConflict) {
		return out, skipped, opshttp.Conflict("the payroll run for that month is not a draft any more")
	}
	return out, skipped, err
}

// Contacts searches Contacts for the client picker.
func (r *Repository) Contacts(ctx context.Context, c Caller, q string) ([]opshttp.ContactRef, error) {
	if err := c.needAdmin(); err != nil {
		return nil, err
	}
	if r.contacts == nil {
		return []opshttp.ContactRef{}, nil
	}
	return r.contacts.Search(ctx, c.OrgID, q, 20)
}
