package legal

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5"
)

// ---- deadlines ----------------------------------------------------------------------------------------------

// Deadline is a dated item on a matter: a filing deadline, court date, meeting or limitation date.
type Deadline struct {
	ID          string `json:"id"`
	MatterID    string `json:"matter_id"`
	MatterTitle string `json:"matter_title,omitempty"`
	MatterNo    string `json:"matter_number,omitempty"`
	Title       string `json:"title"`
	Kind        string `json:"kind"`
	Due         string `json:"due_date"`
	AssigneeID  string `json:"assignee_id,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	Done        bool   `json:"done"`
	Notes       string `json:"notes"`
	Urgency     string `json:"urgency"`
	DaysLeft    int    `json:"days_left"`
	Reminders   bool   `json:"reminders"`
}

// DeadlineInput creates or edits a deadline.
type DeadlineInput struct {
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Due        string `json:"due_date"`
	AssigneeID string `json:"assignee_id"`
	Notes      string `json:"notes"`
}

const deadlineCols = `d.id::text, d.matter_id::text, m.title, m.number, d.title, d.kind, to_char(d.due_date,'YYYY-MM-DD'), COALESCE(d.assignee_id::text,''),
	COALESCE(NULLIF(u.display_name,''),u.email,''), d.done, d.notes, (d.event_id IS NOT NULL OR d.task_id IS NOT NULL)`
const deadlineFrom = ` FROM adrient.legal_deadlines d JOIN adrient.legal_matters m ON m.id=d.matter_id LEFT JOIN adrient.users u ON u.id=d.assignee_id `

func (r *Repository) scanDeadline(row pgx.Row) (Deadline, error) {
	var d Deadline
	err := row.Scan(&d.ID, &d.MatterID, &d.MatterTitle, &d.MatterNo, &d.Title, &d.Kind, &d.Due, &d.AssigneeID, &d.Assignee, &d.Done, &d.Notes, &d.Reminders)
	if err != nil {
		return d, notFound(err)
	}
	d.Urgency = Urgency(d.Due, r.today(), d.Done)
	d.DaysLeft = DaysBetween(r.today(), d.Due)
	return d, nil
}

func (r *Repository) deadlines(ctx context.Context, org, matter string) ([]Deadline, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+deadlineCols+deadlineFrom+`WHERE d.org_id=$1 AND d.matter_id=$2 ORDER BY d.done, d.due_date`, org, matter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deadline{}
	for rows.Next() {
		d, err := r.scanDeadline(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Deadlines lists open deadlines across the matters the actor can see, soonest first.
func (r *Repository) Deadlines(ctx context.Context, a knowkit.Actor, includeDone bool) ([]Deadline, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+deadlineCols+`, m.access, COALESCE(m.shared_with::text[],'{}'), COALESCE(m.created_by::text,''), COALESCE(m.responsible_id::text,''), COALESCE(m.originating_id::text,'')`+deadlineFrom+`
		WHERE d.org_id=$1 AND m.deleted_at IS NULL AND ($2 OR NOT d.done) ORDER BY d.done, d.due_date LIMIT 1000`, a.Org, includeDone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deadline{}
	for rows.Next() {
		var d Deadline
		var m Matter
		if err := rows.Scan(&d.ID, &d.MatterID, &d.MatterTitle, &d.MatterNo, &d.Title, &d.Kind, &d.Due, &d.AssigneeID, &d.Assignee, &d.Done, &d.Notes, &d.Reminders,
			&m.Access, &m.SharedWith, &m.CreatedBy, &m.ResponsibleID, &m.OriginatingID); err != nil {
			return nil, err
		}
		if !r.canView(a, m) {
			continue
		}
		d.Urgency = Urgency(d.Due, r.today(), d.Done)
		d.DaysLeft = DaysBetween(r.today(), d.Due)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (in *DeadlineInput) clean() error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || tooLong(in.Title, 200) || tooLong(in.Notes, 2000) {
		return knowkit.Invalid("a deadline needs a title of up to 200 characters")
	}
	if in.Kind == "" {
		in.Kind = "deadline"
	}
	if !knowkit.OneOf(in.Kind, "deadline", "court_date", "filing", "meeting", "limitation") {
		return knowkit.Invalid("unknown deadline kind")
	}
	if !ValidDate(in.Due) {
		return knowkit.Invalid("the due date must be YYYY-MM-DD")
	}
	if in.AssigneeID != "" && !knowkit.IsUUID(in.AssigneeID) {
		return knowkit.Invalid("bad assignee")
	}
	return nil
}

func kindLabel(k string) string {
	return map[string]string{"deadline": "Deadline", "court_date": "Court date", "filing": "Filing", "meeting": "Meeting", "limitation": "Limitation date"}[k]
}

// AddDeadline creates a deadline and, when Calendar and Tasks are connected, its reminders.
func (r *Repository) AddDeadline(ctx context.Context, a knowkit.Actor, matterID string, in DeadlineInput) (Deadline, error) {
	if err := in.clean(); err != nil {
		return Deadline{}, err
	}
	m, err := r.loadMatter(ctx, a, matterID)
	if err != nil {
		return Deadline{}, err
	}
	assignee, _ := optUUID(in.AssigneeID)
	if assignee == nil {
		assignee = a.User
	}
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.legal_deadlines (org_id, matter_id, title, kind, due_date, assignee_id, notes) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`,
		a.Org, matterID, in.Title, in.Kind, in.Due, assignee, in.Notes).Scan(&id); err != nil {
		return Deadline{}, err
	}
	r.syncDeadline(ctx, a.Org, id, m)
	r.audit(ctx, "deadline_create", "legal_deadline", id)
	return r.scanDeadline(r.pool.QueryRow(ctx, `SELECT `+deadlineCols+deadlineFrom+`WHERE d.id=$1`, id))
}

// UpdateDeadline edits a deadline and moves its reminders.
func (r *Repository) UpdateDeadline(ctx context.Context, a knowkit.Actor, id string, in DeadlineInput) (Deadline, error) {
	if err := in.clean(); err != nil {
		return Deadline{}, err
	}
	m, err := r.deadlineMatter(ctx, a, id)
	if err != nil {
		return Deadline{}, err
	}
	assignee, _ := optUUID(in.AssigneeID)
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.legal_deadlines SET title=$3, kind=$4, due_date=$5, assignee_id=$6, notes=$7 WHERE org_id=$1 AND id=$2`,
		a.Org, id, in.Title, in.Kind, in.Due, assignee, in.Notes); err != nil {
		return Deadline{}, err
	}
	r.syncDeadline(ctx, a.Org, id, m)
	r.audit(ctx, "deadline_update", "legal_deadline", id)
	return r.scanDeadline(r.pool.QueryRow(ctx, `SELECT `+deadlineCols+deadlineFrom+`WHERE d.id=$1`, id))
}

func (r *Repository) deadlineMatter(ctx context.Context, a knowkit.Actor, id string) (*Matter, error) {
	if !knowkit.IsUUID(id) {
		return nil, knowkit.ErrNotFound
	}
	var mid string
	if err := r.pool.QueryRow(ctx, `SELECT matter_id::text FROM adrient.legal_deadlines WHERE org_id=$1 AND id=$2`, a.Org, id).Scan(&mid); err != nil {
		return nil, notFound(err)
	}
	return r.loadMatter(ctx, a, mid)
}

// SetDeadlineDone ticks or unticks a deadline.
func (r *Repository) SetDeadlineDone(ctx context.Context, a knowkit.Actor, id string, done bool) (Deadline, error) {
	m, err := r.deadlineMatter(ctx, a, id)
	if err != nil {
		return Deadline{}, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.legal_deadlines SET done=$3 WHERE org_id=$1 AND id=$2`, a.Org, id, done); err != nil {
		return Deadline{}, err
	}
	r.syncDeadline(ctx, a.Org, id, m)
	r.audit(ctx, "deadline_done", "legal_deadline", id)
	return r.scanDeadline(r.pool.QueryRow(ctx, `SELECT `+deadlineCols+deadlineFrom+`WHERE d.id=$1`, id))
}

// DeleteDeadline removes a deadline and its reminders.
func (r *Repository) DeleteDeadline(ctx context.Context, a knowkit.Actor, id string) error {
	if _, err := r.deadlineMatter(ctx, a, id); err != nil {
		return err
	}
	var ev, tk *string
	_ = r.pool.QueryRow(ctx, `SELECT event_id::text, task_id::text FROM adrient.legal_deadlines WHERE id=$1`, id).Scan(&ev, &tk)
	r.dropLinked(ctx, a.Org, ev, tk)
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.legal_deadlines WHERE org_id=$1 AND id=$2`, a.Org, id)
	r.audit(ctx, "deadline_delete", "legal_deadline", id)
	return err
}

func (r *Repository) dropLinked(ctx context.Context, org string, ev, tk *string) {
	if ev != nil && r.Deps.DeleteEvent != nil {
		_ = r.Deps.DeleteEvent(ctx, org, *ev)
	}
	if tk != nil && r.Deps.DeleteTask != nil {
		_ = r.Deps.DeleteTask(ctx, org, *tk)
	}
}

// syncDeadline creates or moves the Calendar event and the Task for a deadline (no-ops without those apps).
func (r *Repository) syncDeadline(ctx context.Context, org, id string, m *Matter) {
	var title, kind, due, notes string
	var assignee, ev, tk *string
	var done bool
	if err := r.pool.QueryRow(ctx, `SELECT title, kind, to_char(due_date,'YYYY-MM-DD'), notes, assignee_id::text, event_id::text, task_id::text, done FROM adrient.legal_deadlines WHERE id=$1`, id).
		Scan(&title, &kind, &due, &notes, &assignee, &ev, &tk, &done); err != nil {
		return
	}
	owner := m.ResponsibleID
	if assignee != nil {
		owner = *assignee
	}
	if owner == "" {
		owner = m.CreatedBy
	}
	label := fmt.Sprintf("%s: %s (%s)", kindLabel(kind), title, m.Number)
	desc := "Matter " + m.Number + " " + m.Title
	if notes != "" {
		desc += "\n" + notes
	}
	if r.Deps.Calendar != nil {
		if done {
			if ev != nil && r.Deps.DeleteEvent != nil {
				_ = r.Deps.DeleteEvent(ctx, org, *ev)
				_, _ = r.pool.Exec(ctx, `UPDATE adrient.legal_deadlines SET event_id=NULL WHERE id=$1`, id)
			}
		} else {
			cur := ""
			if ev != nil {
				cur = *ev
			}
			if nid, err := r.Deps.Calendar(ctx, org, owner, cur, label, desc, due); err == nil && nid != "" {
				_, _ = r.pool.Exec(ctx, `UPDATE adrient.legal_deadlines SET event_id=$2 WHERE id=$1`, id, nid)
			}
		}
	}
	if r.Deps.Task != nil && owner != "" {
		cur := ""
		if tk != nil {
			cur = *tk
		}
		if nid, err := r.Deps.Task(ctx, org, owner, cur, label, desc, due, done); err == nil && nid != "" {
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.legal_deadlines SET task_id=$2 WHERE id=$1`, id, nid)
		}
	}
}

// syncSol keeps one "Limitation date" deadline in step with the matter's limitation date.
func (r *Repository) syncSol(ctx context.Context, a knowkit.Actor, matterID string) {
	m, err := r.loadMatter(ctx, a, matterID)
	if err != nil {
		return
	}
	var id string
	err = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.legal_deadlines WHERE matter_id=$1 AND kind='limitation' ORDER BY created_at LIMIT 1`, matterID).Scan(&id)
	if m.SolDate == "" {
		if err == nil {
			_ = r.DeleteDeadline(ctx, a, id)
		}
		return
	}
	in := DeadlineInput{Title: "Limitation period ends", Kind: "limitation", Due: m.SolDate, AssigneeID: m.ResponsibleID}
	if err == nil {
		_, _ = r.UpdateDeadline(ctx, a, id, in)
		return
	}
	_, _ = r.AddDeadline(ctx, a, matterID, in)
}

// dropReminders removes the Calendar events and Tasks of a matter (when it closes or is deleted).
func (r *Repository) dropReminders(ctx context.Context, org, matterID string) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, event_id::text, task_id::text FROM adrient.legal_deadlines WHERE org_id=$1 AND matter_id=$2 AND (event_id IS NOT NULL OR task_id IS NOT NULL)`, org, matterID)
	if err != nil {
		return
	}
	type row struct {
		id     string
		ev, tk *string
	}
	var list []row
	for rows.Next() {
		var x row
		if rows.Scan(&x.id, &x.ev, &x.tk) == nil {
			list = append(list, x)
		}
	}
	rows.Close()
	for _, x := range list {
		r.dropLinked(ctx, org, x.ev, x.tk)
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.legal_deadlines SET event_id=NULL, task_id=NULL WHERE id=$1`, x.id)
	}
}

// ---- time, expenses and flat fees ------------------------------------------------------------------------------

// Entry is a time, expense or flat-fee record on a matter.
type Entry struct {
	ID          string `json:"id"`
	MatterID    string `json:"matter_id"`
	MatterNo    string `json:"matter_number,omitempty"`
	MatterTitle string `json:"matter_title,omitempty"`
	Kind        string `json:"kind"`
	UserID      string `json:"user_id"`
	User        string `json:"user"`
	Date        string `json:"date"`
	Minutes     int    `json:"minutes"`
	Rate        int64  `json:"rate"`
	Qty         string `json:"qty,omitempty"`
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
	Billable    bool   `json:"billable"`
	BillID      string `json:"bill_id,omitempty"`
	Source      string `json:"source,omitempty"`
}

// EntryInput creates or edits an entry. For time, give Duration ("1:30", "1.5", "90m") or Minutes. For an
// expense give Price (minor units) and Qty (default 1). For a flat fee give Amount.
type EntryInput struct {
	Kind        string `json:"kind"`
	Date        string `json:"date"`
	Duration    string `json:"duration"`
	Minutes     int    `json:"minutes"`
	Rate        int64  `json:"rate"`
	Price       int64  `json:"price"`
	Qty         string `json:"qty"`
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
	Billable    *bool  `json:"billable"`
	UserID      string `json:"user_id"`
}

const entryCols = `e.id::text, e.matter_id::text, m.number, m.title, e.kind, e.user_id::text, COALESCE(NULLIF(u.display_name,''),u.email,''), to_char(e.entry_date,'YYYY-MM-DD'),
	e.minutes, e.rate, e.qty_milli, e.amount, e.description, e.billable, COALESCE(e.bill_id::text,''), e.source_ref`
const entryFrom = ` FROM adrient.legal_entries e JOIN adrient.legal_matters m ON m.id=e.matter_id LEFT JOIN adrient.users u ON u.id=e.user_id `

func scanEntry(row pgx.Row) (Entry, error) {
	var e Entry
	var qty int64
	err := row.Scan(&e.ID, &e.MatterID, &e.MatterNo, &e.MatterTitle, &e.Kind, &e.UserID, &e.User, &e.Date, &e.Minutes, &e.Rate, &qty, &e.Amount, &e.Description, &e.Billable, &e.BillID, &e.Source)
	if e.Kind == "expense" {
		e.Qty = fmtMilli(qty)
	}
	return e, notFound(err)
}

func fmtMilli(q int64) string {
	s := strconv.FormatInt(q/1000, 10)
	if f := q % 1000; f != 0 {
		s += "." + strings.TrimRight(fmt.Sprintf("%03d", f), "0")
	}
	return s
}

func parseMilli(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 1000, true
	}
	whole, frac, _ := strings.Cut(s, ".")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w < 0 || w > 1e9 || len(frac) > 3 {
		return 0, false
	}
	f := int64(0)
	if frac != "" {
		fv, err := strconv.ParseInt(frac+strings.Repeat("0", 3-len(frac)), 10, 64)
		if err != nil {
			return 0, false
		}
		f = fv
	}
	return w*1000 + f, true
}

// Entries lists entries; admins see everyone's, other people their own and those on matters they can see.
func (r *Repository) Entries(ctx context.Context, a knowkit.Actor, matterID, billed, from, to, kind string) ([]Entry, error) {
	if matterID != "" {
		if _, err := r.loadMatter(ctx, a, matterID); err != nil {
			return nil, err
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT `+entryCols+`, m.access, COALESCE(m.shared_with::text[],'{}'), COALESCE(m.created_by::text,''), COALESCE(m.responsible_id::text,''), COALESCE(m.originating_id::text,'')`+entryFrom+`
		WHERE e.org_id=$1 AND m.deleted_at IS NULL AND ($2='' OR e.matter_id::text=$2)
		AND ($3='' OR ($3='unbilled' AND e.bill_id IS NULL AND e.billable) OR ($3='billed' AND e.bill_id IS NOT NULL) OR ($3='nonbillable' AND NOT e.billable))
		AND ($4='' OR e.entry_date >= $4::date) AND ($5='' OR e.entry_date <= $5::date) AND ($6='' OR e.kind=$6)
		ORDER BY e.entry_date DESC, e.created_at DESC LIMIT 1500`, a.Org, matterID, billed, from, to, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var m Matter
		var qty int64
		if err := rows.Scan(&e.ID, &e.MatterID, &e.MatterNo, &e.MatterTitle, &e.Kind, &e.UserID, &e.User, &e.Date, &e.Minutes, &e.Rate, &qty, &e.Amount, &e.Description, &e.Billable, &e.BillID, &e.Source,
			&m.Access, &m.SharedWith, &m.CreatedBy, &m.ResponsibleID, &m.OriginatingID); err != nil {
			return nil, err
		}
		if !r.canView(a, m) {
			continue
		}
		if e.Kind == "expense" {
			e.Qty = fmtMilli(qty)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type entryNumbers struct {
	minutes  int
	rate     int64
	qtyMilli int64
	amount   int64
}

// price turns the input into stored numbers: time is rounded up to the billing increment and valued at the rate.
func (r *Repository) price(ctx context.Context, org string, m *Matter, in EntryInput, s Settings) (entryNumbers, error) {
	var n entryNumbers
	n.qtyMilli = 1000
	switch in.Kind {
	case "time":
		mins := in.Minutes
		if in.Duration != "" {
			v, ok := ParseDuration(in.Duration)
			if !ok {
				return n, knowkit.Invalid("cannot read the duration %q; try 1.5, 1:30 or 90m", in.Duration)
			}
			mins = v
		}
		if mins <= 0 || mins > MaxMinutes {
			return n, knowkit.Invalid("enter a duration above zero")
		}
		n.minutes = RoundUpMinutes(mins, s.IncrementMin)
		if in.Rate > 0 {
			n.rate = in.Rate
		} else {
			var clientRate int64
			if m.ClientID != "" {
				_ = r.pool.QueryRow(ctx, `SELECT rate FROM adrient.legal_clients WHERE id=$1`, m.ClientID).Scan(&clientRate)
			}
			matterRate := int64(0)
			if m.BillingMethod == "hourly" {
				matterRate = m.Rate
			}
			n.rate = Rate(matterRate, clientRate, s.DefaultRate)
		}
		if n.rate < 0 || n.rate > 1e12 {
			return n, knowkit.Invalid("the rate is out of range")
		}
		n.amount = TimeValue(n.minutes, n.rate)
	case "expense":
		q, ok := parseMilli(in.Qty)
		if !ok || q <= 0 {
			return n, knowkit.Invalid("the quantity must be a positive number with up to 3 decimals")
		}
		if in.Price < 0 || in.Price > 1e12 {
			return n, knowkit.Invalid("the price is out of range")
		}
		n.qtyMilli, n.rate = q, in.Price
		n.amount = ExpenseValue(in.Price, q)
	case "flat":
		if in.Amount == 0 || in.Amount > 1e13 || in.Amount < -1e13 {
			return n, knowkit.Invalid("enter a flat fee amount (negative for a credit)")
		}
		n.amount = in.Amount
	default:
		return n, knowkit.Invalid("kind must be time, expense or flat")
	}
	return n, nil
}

// AddEntry records time, an expense or a flat fee on a matter.
func (r *Repository) AddEntry(ctx context.Context, a knowkit.Actor, matterID string, in EntryInput) (Entry, error) {
	m, err := r.loadMatter(ctx, a, matterID)
	if err != nil {
		return Entry{}, err
	}
	if m.Status == "closed" {
		return Entry{}, knowkit.Invalid("this matter is closed; reopen it to record work")
	}
	if in.Date == "" {
		in.Date = r.today()
	}
	if !ValidDate(in.Date) || tooLong(in.Description, 2000) {
		return Entry{}, knowkit.Invalid("the date or description is not valid")
	}
	s, err := r.GetSettings(ctx, a.Org)
	if err != nil {
		return Entry{}, err
	}
	n, err := r.price(ctx, a.Org, m, in, s)
	if err != nil {
		return Entry{}, err
	}
	user := a.User
	if in.UserID != "" && in.UserID != a.User {
		if !a.Admin {
			return Entry{}, knowkit.ErrForbidden
		}
		if !knowkit.IsUUID(in.UserID) {
			return Entry{}, knowkit.Invalid("bad user")
		}
		var ok bool
		_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE org_id=$1 AND id=$2)`, a.Org, in.UserID).Scan(&ok)
		if !ok {
			return Entry{}, knowkit.Invalid("that person is not in this organization")
		}
		user = in.UserID
	}
	billable := in.Billable == nil || *in.Billable
	var id string
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.legal_entries (org_id, matter_id, kind, user_id, entry_date, minutes, rate, qty_milli, amount, description, billable)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, a.Org, matterID, in.Kind, user, in.Date, n.minutes, n.rate, n.qtyMilli, n.amount, strings.TrimSpace(in.Description), billable).Scan(&id)
	if err != nil {
		return Entry{}, err
	}
	r.audit(ctx, "entry_create", "legal_entry", id)
	return scanEntry(r.pool.QueryRow(ctx, `SELECT `+entryCols+entryFrom+`WHERE e.id=$1`, id))
}

func (r *Repository) loadEntry(ctx context.Context, a knowkit.Actor, id string) (Entry, *Matter, error) {
	if !knowkit.IsUUID(id) {
		return Entry{}, nil, knowkit.ErrNotFound
	}
	e, err := scanEntry(r.pool.QueryRow(ctx, `SELECT `+entryCols+entryFrom+`WHERE e.org_id=$1 AND e.id=$2`, a.Org, id))
	if err != nil {
		return e, nil, err
	}
	m, err := r.loadMatter(ctx, a, e.MatterID)
	if err != nil {
		return e, nil, err
	}
	if e.BillID != "" {
		return e, m, knowkit.Invalid("this entry is on a bill; void the bill first")
	}
	if e.UserID != a.User && !a.Admin {
		return e, m, knowkit.ErrForbidden
	}
	return e, m, nil
}

// UpdateEntry edits an unbilled entry (its owner or an admin).
func (r *Repository) UpdateEntry(ctx context.Context, a knowkit.Actor, id string, in EntryInput) (Entry, error) {
	e, m, err := r.loadEntry(ctx, a, id)
	if err != nil {
		return e, err
	}
	in.Kind = e.Kind
	if in.Date == "" {
		in.Date = e.Date
	}
	if !ValidDate(in.Date) || tooLong(in.Description, 2000) {
		return Entry{}, knowkit.Invalid("the date or description is not valid")
	}
	s, err := r.GetSettings(ctx, a.Org)
	if err != nil {
		return Entry{}, err
	}
	n, err := r.price(ctx, a.Org, m, in, s)
	if err != nil {
		return Entry{}, err
	}
	billable := in.Billable == nil || *in.Billable
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.legal_entries SET entry_date=$3, minutes=$4, rate=$5, qty_milli=$6, amount=$7, description=$8, billable=$9 WHERE org_id=$1 AND id=$2`,
		a.Org, id, in.Date, n.minutes, n.rate, n.qtyMilli, n.amount, strings.TrimSpace(in.Description), billable); err != nil {
		return Entry{}, err
	}
	r.audit(ctx, "entry_update", "legal_entry", id)
	return scanEntry(r.pool.QueryRow(ctx, `SELECT `+entryCols+entryFrom+`WHERE e.id=$1`, id))
}

// DeleteEntry removes an unbilled entry.
func (r *Repository) DeleteEntry(ctx context.Context, a knowkit.Actor, id string) error {
	if _, _, err := r.loadEntry(ctx, a, id); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.legal_entries WHERE org_id=$1 AND id=$2`, a.Org, id)
	r.audit(ctx, "entry_delete", "legal_entry", id)
	return err
}

// TimeCandidates lists Time tracking entries that are not yet imported into Legal.
func (r *Repository) TimeCandidates(ctx context.Context, a knowkit.Actor, from, to string) ([]TimeEntry, error) {
	if r.Deps.TimeEntries == nil {
		return nil, knowkit.ErrUnavailable
	}
	if (from != "" && !ValidDate(from)) || (to != "" && !ValidDate(to)) {
		return nil, knowkit.Invalid("from and to must be dates")
	}
	list, err := r.Deps.TimeEntries(ctx, a.Org, a.User, a.Admin, from, to)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	rows, err := r.pool.Query(ctx, `SELECT source_ref FROM adrient.legal_entries WHERE org_id=$1 AND source_ref LIKE 'tt:%'`, a.Org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			seen[s] = true
		}
	}
	out := []TimeEntry{}
	for _, e := range list {
		if !seen["tt:"+e.ID] && e.Seconds > 0 {
			out = append(out, e)
		}
	}
	return out, nil
}

// ImportTime copies the chosen Time tracking entries onto a matter. Each source entry imports once.
func (r *Repository) ImportTime(ctx context.Context, a knowkit.Actor, matterID string, ids []string) ([]Entry, error) {
	if len(ids) == 0 || len(ids) > 200 {
		return nil, knowkit.Invalid("choose between 1 and 200 entries")
	}
	cands, err := r.TimeCandidates(ctx, a, "", "")
	if err != nil {
		return nil, err
	}
	byID := map[string]TimeEntry{}
	for _, c := range cands {
		byID[c.ID] = c
	}
	out := []Entry{}
	for _, id := range ids {
		c, ok := byID[id]
		if !ok {
			return nil, knowkit.Invalid("a chosen entry was already imported or is not yours to import")
		}
		mins := (c.Seconds + 59) / 60
		bill := c.Billable
		e, err := r.AddEntry(ctx, a, matterID, EntryInput{Kind: "time", Date: c.Date, Minutes: mins, Description: strings.TrimSpace(c.Project + " " + c.Note), Billable: &bill, UserID: c.UserID})
		if err != nil {
			return out, err
		}
		if _, err := r.pool.Exec(ctx, `UPDATE adrient.legal_entries SET source_ref=$2 WHERE id=$1`, e.ID, "tt:"+id); err != nil {
			return out, knowkit.ErrConflict
		}
		e.Source = "tt:" + id
		out = append(out, e)
	}
	return out, nil
}
