package accounting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Recurring is a schedule that creates a journal, bill or expense on each occurrence.
type Recurring struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"` // journal | bill | expense
	Name      string          `json:"name"`
	Frequency string          `json:"frequency"` // daily | weekly | monthly | quarterly | yearly
	Every     int             `json:"every"`
	StartDate string          `json:"start_date"`
	EndDate   string          `json:"end_date"`
	MaxRuns   int             `json:"max_runs"`
	Active    bool            `json:"active"`
	AutoPost  bool            `json:"auto_post"`
	Template  json.RawMessage `json:"template"`
	LastRun   string          `json:"last_run"`
	Runs      int             `json:"runs"`
	NextRun   string          `json:"next_run"`
	LastError string          `json:"last_error"`
}

// OccurrenceDate returns the i-th occurrence (0-based) of a schedule. Dates are
// always derived from the start date, so a month-end schedule does not drift
// (31 Jan, 28 Feb, 31 Mar).
func OccurrenceDate(start time.Time, freq string, every, i int) time.Time {
	if every < 1 {
		every = 1
	}
	n := i * every
	switch freq {
	case "daily":
		return start.AddDate(0, 0, n)
	case "weekly":
		return start.AddDate(0, 0, 7*n)
	case "quarterly":
		return addMonthsClamped(start, 3*n)
	case "yearly":
		return addMonthsClamped(start, 12*n)
	}
	return addMonthsClamped(start, n)
}

func addMonthsClamped(t time.Time, months int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).AddDate(0, months, 0)
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, time.UTC)
}

func validRecurring(in *Recurring) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("name is required")
	}
	switch in.Kind {
	case "journal", "bill", "expense":
	default:
		return invalid("kind must be journal, bill or expense")
	}
	switch in.Frequency {
	case "daily", "weekly", "monthly", "quarterly", "yearly":
	default:
		return invalid("frequency must be daily, weekly, monthly, quarterly or yearly")
	}
	if in.Every < 1 || in.Every > 365 {
		return invalid("repeat interval must be between 1 and 365")
	}
	var err error
	if in.StartDate, err = validDate(in.StartDate); err != nil {
		return err
	}
	if in.EndDate != "" {
		if in.EndDate, err = validDate(in.EndDate); err != nil {
			return err
		}
	}
	if in.MaxRuns < 0 {
		return invalid("max runs cannot be negative")
	}
	switch in.Kind {
	case "journal":
		var t JournalInput
		if err := json.Unmarshal(in.Template, &t); err != nil {
			return invalid("template: %v", err)
		}
		if _, err := checkBalanced(t.Lines); err != nil {
			return err
		}
	case "bill":
		var t BillInput
		if err := json.Unmarshal(in.Template, &t); err != nil {
			return invalid("template: %v", err)
		}
		if _, _, _, err := computeBill(t.Lines); err != nil {
			return err
		}
	case "expense":
		var t Expense
		if err := json.Unmarshal(in.Template, &t); err != nil {
			return invalid("template: %v", err)
		}
		if paise(t.Amount) <= 0 || !isUUID(t.CategoryID) {
			return invalid("an expense template needs a category and an amount")
		}
	}
	return nil
}

// SaveRecurring creates (id "") or updates a schedule.
func (r *Repository) SaveRecurring(ctx context.Context, orgID, userID, id string, in Recurring) (Recurring, error) {
	if in.Every == 0 {
		in.Every = 1
	}
	if len(in.Template) == 0 {
		in.Template = json.RawMessage("{}")
	}
	if err := validRecurring(&in); err != nil {
		return Recurring{}, err
	}
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_recurring (org_id, kind, name, frequency, every, start_date, end_date, max_runs, active, template, auto_post, created_by)
			VALUES ($1,$2,$3,$4,$5,$6::date,NULLIF($7,'')::date,$8,$9,$10::jsonb,$11,$12) RETURNING id::text`,
			orgID, in.Kind, in.Name, in.Frequency, in.Every, in.StartDate, in.EndDate, in.MaxRuns, in.Active, string(in.Template), in.AutoPost, userID).Scan(&id)
	} else {
		if !isUUID(id) {
			return Recurring{}, ErrNotFound
		}
		t, e := r.pool.Exec(ctx, `UPDATE adrient.acct_recurring SET kind=$3, name=$4, frequency=$5, every=$6, start_date=$7::date, end_date=NULLIF($8,'')::date, max_runs=$9,
			active=$10, template=$11::jsonb, auto_post=$12 WHERE org_id=$1 AND id=$2::uuid`,
			orgID, id, in.Kind, in.Name, in.Frequency, in.Every, in.StartDate, in.EndDate, in.MaxRuns, in.Active, string(in.Template), in.AutoPost)
		err = e
		if err == nil && t.RowsAffected() == 0 {
			return Recurring{}, ErrNotFound
		}
	}
	if err != nil {
		return Recurring{}, err
	}
	r.audit(ctx, orgID, userID, "save", "recurring", id, in.Name)
	return r.GetRecurring(ctx, orgID, id)
}

const recurringCols = `id::text, kind, name, frequency, every, to_char(start_date,'YYYY-MM-DD'), coalesce(to_char(end_date,'YYYY-MM-DD'),''), max_runs, active, auto_post, template,
	coalesce(to_char(last_run,'YYYY-MM-DD'),''),
	(SELECT count(*) FROM adrient.acct_recurring_runs x WHERE x.recurring_id=c.id AND x.status='done'),
	coalesce((SELECT error FROM adrient.acct_recurring_runs x WHERE x.recurring_id=c.id AND x.status='failed' ORDER BY run_date DESC LIMIT 1),'')`

func scanRecurring(row pgx.Row) (Recurring, error) {
	var x Recurring
	var tpl []byte
	err := row.Scan(&x.ID, &x.Kind, &x.Name, &x.Frequency, &x.Every, &x.StartDate, &x.EndDate, &x.MaxRuns, &x.Active, &x.AutoPost, &tpl, &x.LastRun, &x.Runs, &x.LastError)
	x.Template = tpl
	if err == nil {
		x.NextRun = nextOccurrence(x)
	}
	return x, err
}

// nextOccurrence is the first scheduled date after the completed runs (display only).
func nextOccurrence(x Recurring) string {
	start, err := time.Parse("2006-01-02", x.StartDate)
	if err != nil || !x.Active {
		return ""
	}
	if x.MaxRuns > 0 && x.Runs >= x.MaxRuns {
		return ""
	}
	d := OccurrenceDate(start, x.Frequency, x.Every, x.Runs).Format("2006-01-02")
	if x.EndDate != "" && d > x.EndDate {
		return ""
	}
	return d
}

func (r *Repository) GetRecurring(ctx context.Context, orgID, id string) (Recurring, error) {
	if !isUUID(id) {
		return Recurring{}, ErrNotFound
	}
	x, err := scanRecurring(r.pool.QueryRow(ctx, `SELECT `+recurringCols+` FROM adrient.acct_recurring c WHERE org_id=$1 AND id=$2::uuid`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, ErrNotFound
	}
	return x, err
}

func (r *Repository) ListRecurring(ctx context.Context, orgID string) ([]Recurring, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+recurringCols+` FROM adrient.acct_recurring c WHERE org_id=$1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recurring{}
	for rows.Next() {
		x, err := scanRecurring(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteRecurring(ctx context.Context, orgID, userID, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.acct_recurring WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	r.audit(ctx, orgID, userID, "delete", "recurring", id, "")
	return nil
}

// RecurringRunResult reports one pass of the runner.
type RecurringRunResult struct {
	Created int      `json:"created"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
}

// RunRecurring creates every occurrence of every active schedule that is due on or
// before asOf and has not been created yet. It is safe to run any number of times,
// concurrently or after a restart: (schedule, date) is a unique key claimed before
// anything is posted, and a failed occurrence is retried on the next run. At most
// 400 missed occurrences are created per schedule per pass.
func (r *Repository) RunRecurring(ctx context.Context, orgID, userID, asOf string) (RecurringRunResult, error) {
	res := RecurringRunResult{Errors: []string{}}
	if _, err := validDate(asOf); err != nil {
		return res, err
	}
	list, err := r.ListRecurring(ctx, orgID)
	if err != nil {
		return res, err
	}
	for _, x := range list {
		if !x.Active {
			continue
		}
		start, _ := time.Parse("2006-01-02", x.StartDate)
		made := 0
		for i := 0; i < 5000 && made < 400; i++ {
			ds := OccurrenceDate(start, x.Frequency, x.Every, i).Format("2006-01-02")
			if ds > asOf || (x.EndDate != "" && ds > x.EndDate) {
				break
			}
			if x.MaxRuns > 0 && i >= x.MaxRuns {
				break
			}
			var runID string
			err := r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_recurring_runs (recurring_id, org_id, run_date) VALUES ($1::uuid,$2,$3::date)
				ON CONFLICT (recurring_id, run_date) DO UPDATE SET status='running', error='' WHERE acct_recurring_runs.status='failed' RETURNING id::text`,
				x.ID, orgID, ds).Scan(&runID)
			if errors.Is(err, pgx.ErrNoRows) {
				continue // already done or being run elsewhere
			}
			if err != nil {
				return res, err
			}
			resultID, runErr := r.runOne(ctx, orgID, userID, x, ds)
			if runErr != nil {
				_, _ = r.pool.Exec(ctx, `UPDATE adrient.acct_recurring_runs SET status='failed', error=$2 WHERE id=$1::uuid`, runID, runErr.Error())
				res.Failed++
				if len(res.Errors) < 10 {
					res.Errors = append(res.Errors, fmt.Sprintf("%s %s: %v", x.Name, ds, runErr))
				}
				continue
			}
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.acct_recurring_runs SET status='done', result_id=$2 WHERE id=$1::uuid`, runID, resultID)
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.acct_recurring SET last_run=greatest(coalesce(last_run,'0001-01-01'::date),$2::date) WHERE id=$1::uuid`, x.ID, ds)
			res.Created++
			made++
		}
	}
	if res.Created+res.Failed > 0 {
		r.audit(ctx, orgID, userID, "run", "recurring", "", fmt.Sprintf("%d created, %d failed", res.Created, res.Failed))
	}
	return res, nil
}

func (r *Repository) runOne(ctx context.Context, orgID, userID string, x Recurring, date string) (string, error) {
	switch x.Kind {
	case "journal":
		var t JournalInput
		if err := json.Unmarshal(x.Template, &t); err != nil {
			return "", err
		}
		t.Date, t.DueDate = date, ""
		t.Source, t.SourceID = SourceRecurring, x.ID+":"+date
		t.SourceHash = hashOf(x.ID, date, x.Name)
		t.Memo = orDefault(t.Memo, x.Name)
		if err := r.applyCurrency(ctx, orgID, &t); err != nil {
			return "", err
		}
		j, _, err := r.PostJournal(ctx, orgID, userID, t)
		return j.ID, err
	case "bill":
		var t BillInput
		if err := json.Unmarshal(x.Template, &t); err != nil {
			return "", err
		}
		t.BillDate, t.DueDate, t.Rate = date, "", 0
		t.Draft = !x.AutoPost
		if t.Number != "" {
			t.Number = fmt.Sprintf("%s-%s", t.Number, strings.ReplaceAll(date, "-", ""))
		}
		b, err := r.SaveBill(ctx, orgID, userID, "", t)
		return b.ID, err
	case "expense":
		var t Expense
		if err := json.Unmarshal(x.Template, &t); err != nil {
			return "", err
		}
		t.ID, t.Date = "", date
		e, err := r.SaveExpense(ctx, orgID, userID, "", t)
		return e.ID, err
	}
	return "", invalid("unknown kind")
}

// RunRecurringAllOrgs is what the server runs on start and periodically.
func (r *Repository) RunRecurringAllOrgs(ctx context.Context) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT org_id::text FROM adrient.acct_recurring WHERE active`)
	if err != nil {
		slog.Warn("accounting recurring: list orgs", "err", err)
		return
	}
	var orgs []string
	for rows.Next() {
		var o string
		if rows.Scan(&o) == nil {
			orgs = append(orgs, o)
		}
	}
	rows.Close()
	asOf := today()
	// Monthly depreciation for every org with assets: months that have fully elapsed.
	if arows, err := r.pool.Query(ctx, `SELECT DISTINCT org_id::text FROM adrient.acct_assets WHERE status='active'`); err == nil {
		var aorgs []string
		for arows.Next() {
			var o string
			if arows.Scan(&o) == nil {
				aorgs = append(aorgs, o)
			}
		}
		arows.Close()
		prev := time.Now().In(time.FixedZone("IST", 5*3600+1800)).AddDate(0, -1, 0).Format("2006-01")
		for _, o := range aorgs {
			if res, err := r.RunDepreciation(ctx, o, "", prev); err != nil {
				slog.Warn("accounting depreciation run", "org", o, "err", err)
			} else if res.Posted > 0 {
				slog.Info("accounting depreciation run", "org", o, "posted", res.Posted)
			}
		}
	}
	for _, o := range orgs {
		res, err := r.RunRecurring(ctx, o, "", asOf)
		if err != nil {
			slog.Warn("accounting recurring run", "org", o, "err", err)
			continue
		}
		if res.Created+res.Failed > 0 {
			slog.Info("accounting recurring run", "org", o, "created", res.Created, "failed", res.Failed)
		}
	}
}

// StartRecurringRunner runs the schedules shortly after start and then every six
// hours (idempotent, so effectively once per due date). It stops with ctx.
func (r *Repository) StartRecurringRunner(ctx context.Context) {
	go func() {
		select {
		case <-time.After(20 * time.Second):
		case <-ctx.Done():
			return
		}
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for {
			r.RunRecurringAllOrgs(ctx)
			select {
			case <-t.C:
			case <-ctx.Done():
				return
			}
		}
	}()
}
