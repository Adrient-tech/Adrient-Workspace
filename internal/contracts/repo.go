package contracts

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Types, billing periods and statuses.
var (
	Types    = []string{"service", "vendor", "nda", "employment", "lease", "license", "other"}
	Billings = map[string]int{"one-time": 0, "monthly": 12, "quarterly": 4, "annual": 1}
)

// Deps are the other modules' capabilities; each is optional.
type Deps struct {
	Audit knowkit.Audit
	// Calendar creates or moves an all-day event owned by owner; id "" creates one.
	Calendar func(ctx context.Context, org, owner, id, title, desc string, day Date) (string, error)
	// DeleteEvent removes an event created by Calendar.
	DeleteEvent func(ctx context.Context, org, id string) error
	// Task creates or updates a task for assignee; id "" creates one.
	Task func(ctx context.Context, org, assignee, id, title, notes string, due Date, done bool) (string, error)
	// DeleteTask removes a task created by Task.
	DeleteTask func(ctx context.Context, org, id string) error
	// PartyNames resolves Contacts ids to display names.
	PartyNames func(ctx context.Context, org string, ids []string) map[string]string
	// FileMeta checks that the user may attach a Drive file and returns its name.
	FileMeta func(ctx context.Context, org, user, fileID string) (name string, err error)
	Now      func() time.Time
	// Location is the organisation's time zone (default UTC).
	Location *time.Location
}

// Repository is the Postgres store. Every query is org-scoped.
type Repository struct {
	pool *pgxpool.Pool
	Deps Deps
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) now() time.Time { return knowkit.Clock(r.Deps.Now) }
func (r *Repository) today() Date    { return Today(r.now(), r.Deps.Location) }
func (r *Repository) audit(ctx context.Context, a, t, id string) {
	if r.Deps.Audit != nil {
		r.Deps.Audit(ctx, a, t, id)
	}
}

// Full is a contract with its clock, obligations and documents.
type Full struct {
	Contract
	Type        string       `json:"type"`
	PartyName   string       `json:"party_name"`
	ContactID   string       `json:"contact_id,omitempty"`
	PartyLabel  string       `json:"party_label"`
	Value       float64      `json:"value"`
	Currency    string       `json:"currency"`
	Billing     string       `json:"billing"`
	OwnerID     string       `json:"owner_id"`
	Access      string       `json:"access"`
	SharedWith  []string     `json:"shared_with"`
	Remind      bool         `json:"remind"`
	Tags        []string     `json:"tags"`
	Notes       string       `json:"notes"`
	Annual      float64      `json:"annual_value"`
	Renewal     Renewal      `json:"renewal"`
	Terms       []Term       `json:"completed_terms,omitempty"`
	Obligations []Obligation `json:"obligations,omitempty"`
	Documents   []Document   `json:"documents,omitempty"`
	CanEdit     bool         `json:"can_edit"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Document is a Drive file attached to a contract.
type Document struct {
	ID     string `json:"id"`
	FileID string `json:"file_id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
}

// Input creates or edits a contract.
type Input struct {
	Title      string   `json:"title"`
	Type       string   `json:"type"`
	Status     string   `json:"status"`
	PartyName  string   `json:"party_name"`
	ContactID  string   `json:"contact_id"`
	Value      float64  `json:"value"`
	Currency   string   `json:"currency"`
	Billing    string   `json:"billing"`
	StartDate  Date     `json:"start_date"`
	EndDate    Date     `json:"end_date"`
	AutoRenew  bool     `json:"auto_renew"`
	TermMonths int      `json:"term_months"`
	NoticeDays int      `json:"notice_days"`
	Access     string   `json:"access"`
	SharedWith []string `json:"shared_with"`
	Remind     *bool    `json:"remind"`
	Tags       []string `json:"tags"`
	Notes      string   `json:"notes"`
	OwnerID    string   `json:"owner_id"`
}

func (in *Input) clean() error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 200 {
		return knowkit.Invalid("a contract needs a title of up to 200 characters")
	}
	if in.Type == "" {
		in.Type = "other"
	}
	if !knowkit.OneOf(in.Type, Types...) {
		return knowkit.Invalid("unknown contract type %q", in.Type)
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if !knowkit.OneOf(in.Status, "draft", "active", "terminated") {
		return knowkit.Invalid("status must be draft, active or terminated")
	}
	if _, ok := Billings[in.Billing]; !ok {
		if in.Billing != "" {
			return knowkit.Invalid("billing must be one-time, monthly, quarterly or annual")
		}
		in.Billing = "one-time"
	}
	if in.Value < 0 || in.Value > 1e13 {
		return knowkit.Invalid("the value is out of range")
	}
	in.Value = roundMoney(in.Value)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = "INR"
	}
	if len(in.Currency) != 3 {
		return knowkit.Invalid("currency must be a 3-letter code")
	}
	for _, d := range []Date{in.StartDate, in.EndDate} {
		if d != "" && !IsValidDate(d) {
			return knowkit.Invalid("dates must look like 2026-03-31")
		}
	}
	if in.StartDate != "" && in.EndDate != "" && in.EndDate < in.StartDate {
		return knowkit.Invalid("the end date is before the start date")
	}
	if in.TermMonths == 0 {
		in.TermMonths = 12
	}
	if in.TermMonths < 1 || in.TermMonths > 600 {
		return knowkit.Invalid("the renewal term must be 1 to 600 months")
	}
	if in.NoticeDays < 0 || in.NoticeDays > 3650 {
		return knowkit.Invalid("notice days must be 0 to 3650")
	}
	if in.Access == "" {
		in.Access = "org"
	}
	if !knowkit.OneOf(in.Access, "org", "restricted") {
		return knowkit.Invalid("access must be org or restricted")
	}
	if in.ContactID != "" && !knowkit.IsUUID(in.ContactID) {
		return knowkit.Invalid("bad contact id")
	}
	for _, s := range in.SharedWith {
		if !knowkit.IsUUID(s) {
			return knowkit.Invalid("bad user id in shared_with")
		}
	}
	if len(in.Tags) > 30 {
		return knowkit.Invalid("at most 30 tags")
	}
	if utf8.RuneCountInString(in.Notes) > 20000 {
		return knowkit.Invalid("notes are too long")
	}
	in.PartyName = strings.TrimSpace(in.PartyName)
	return nil
}

func roundMoney(v float64) float64 {
	if v < 0 {
		return -roundMoney(-v)
	}
	return float64(int64(v*100+0.5+1e-9)) / 100
}

const cols = `id::text, title, type, status, party_name, COALESCE(contact_id::text,''), value::float8, currency, billing,
	COALESCE(to_char(start_date,'YYYY-MM-DD'),''), COALESCE(to_char(end_date,'YYYY-MM-DD'),''), auto_renew, term_months, notice_days,
	owner_id::text, access, COALESCE(shared_with::text[], '{}'), remind, tags, notes, decision, updated_at`

func scan(row pgx.Row) (*Full, error) {
	f := &Full{}
	var dec []byte
	err := row.Scan(&f.ID, &f.Title, &f.Type, &f.Status, &f.PartyName, &f.ContactID, &f.Value, &f.Currency, &f.Billing, &f.StartDate, &f.EndDate,
		&f.AutoRenew, &f.TermMonths, &f.NoticeDays, &f.OwnerID, &f.Access, &f.SharedWith, &f.Remind, &f.Tags, &f.Notes, &dec, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, knowkit.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(dec) > 0 && string(dec) != "null" {
		var d Decision
		if json.Unmarshal(dec, &d) == nil {
			f.Decision = &d
		}
	}
	f.Contract.ID, f.Contract.Title, f.Contract.Status, f.Contract.StartDate, f.Contract.EndDate = f.ID, f.Title, f.Status, f.StartDate, f.EndDate
	f.Contract.AutoRenew, f.Contract.TermMonths, f.Contract.NoticeDays = f.AutoRenew, f.TermMonths, f.NoticeDays
	if f.Tags == nil {
		f.Tags = []string{}
	}
	return f, nil
}

func annual(f *Full) float64 {
	return roundMoney(f.Value * float64(Billings[f.Billing]))
}

func (r *Repository) canView(a knowkit.Actor, f *Full) bool {
	return f.Access == "org" || a.Admin || f.OwnerID == a.User || contains(f.SharedWith, a.User)
}
func (r *Repository) canEdit(a knowkit.Actor, f *Full) bool {
	return a.Admin || f.OwnerID == a.User || contains(f.SharedWith, a.User)
}
func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func (r *Repository) decorate(ctx context.Context, a knowkit.Actor, list []*Full) {
	today := r.today()
	var ids []string
	for _, f := range list {
		if f.ContactID != "" {
			ids = append(ids, f.ContactID)
		}
	}
	names := map[string]string{}
	if len(ids) > 0 && r.Deps.PartyNames != nil {
		names = r.Deps.PartyNames(ctx, a.Org, ids)
	}
	for _, f := range list {
		f.Renewal = f.Contract.RenewalInfo(today, 60)
		f.Annual = annual(f)
		f.CanEdit = r.canEdit(a, f)
		f.PartyLabel = f.PartyName
		if n := names[f.ContactID]; n != "" {
			f.PartyLabel = n
		}
		if f.SharedWith == nil {
			f.SharedWith = []string{}
		}
	}
}

// List returns the contracts the caller can see, soonest renewal first.
func (r *Repository) List(ctx context.Context, a knowkit.Actor, q, status string) ([]*Full, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+cols+` FROM adrient.contracts_contracts WHERE org_id=$1 AND deleted_at IS NULL AND ($2='' OR status=$2) ORDER BY lower(title)`, a.Org, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Full
	for rows.Next() {
		f, err := scan(rows)
		if err != nil {
			return nil, err
		}
		if r.canView(a, f) {
			out = append(out, f)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	r.decorate(ctx, a, out)
	if q = strings.ToLower(strings.TrimSpace(q)); q != "" {
		kept := out[:0]
		for _, f := range out {
			hay := strings.ToLower(f.Title + " " + f.PartyLabel + " " + f.Notes + " " + strings.Join(f.Tags, " "))
			if strings.Contains(hay, q) {
				kept = append(kept, f)
			}
		}
		out = kept
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Renewal.Date, out[j].Renewal.Date
		if (a == "") != (b == "") {
			return a != ""
		}
		return a < b
	})
	if out == nil {
		out = []*Full{}
	}
	return out, nil
}

func (r *Repository) load(ctx context.Context, a knowkit.Actor, id string, needEdit bool) (*Full, error) {
	if !knowkit.IsUUID(id) {
		return nil, knowkit.ErrNotFound
	}
	f, err := scan(r.pool.QueryRow(ctx, `SELECT `+cols+` FROM adrient.contracts_contracts WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL`, a.Org, id))
	if err != nil {
		return nil, err
	}
	if !r.canView(a, f) {
		return nil, knowkit.ErrNotFound
	}
	if needEdit && !r.canEdit(a, f) {
		return nil, knowkit.ErrForbidden
	}
	return f, nil
}

// Get returns one contract with obligations, documents and past terms.
func (r *Repository) Get(ctx context.Context, a knowkit.Actor, id string) (*Full, error) {
	f, err := r.load(ctx, a, id, false)
	if err != nil {
		return nil, err
	}
	r.decorate(ctx, a, []*Full{f})
	f.Terms = f.Contract.CompletedTerms(r.today())
	if f.Obligations, err = r.obligations(ctx, a.Org, id); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, file_id::text, name, kind FROM adrient.contracts_documents WHERE contract_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	f.Documents = []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.FileID, &d.Name, &d.Kind); err != nil {
			return nil, err
		}
		f.Documents = append(f.Documents, d)
	}
	return f, rows.Err()
}

func (r *Repository) obligations(ctx context.Context, org, contractID string) ([]Obligation, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, contract_id::text, title, owner_side, COALESCE(to_char(due,'YYYY-MM-DD'),''), done, repeat, history, COALESCE(assignee_id::text,''), notes
		FROM adrient.contracts_obligations WHERE org_id=$1 AND contract_id=$2 ORDER BY done, due NULLS LAST, created_at`, org, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Obligation{}
	for rows.Next() {
		var o Obligation
		var rep, hist []byte
		if err := rows.Scan(&o.ID, &o.ContractID, &o.Title, &o.Owner, &o.Due, &o.Done, &rep, &hist, &o.AssigneeID, &o.Notes); err != nil {
			return nil, err
		}
		if len(rep) > 0 && string(rep) != "null" {
			var rp Repeat
			if json.Unmarshal(rep, &rp) == nil {
				o.Repeat = &rp
			}
		}
		_ = json.Unmarshal(hist, &o.History)
		if o.History == nil {
			o.History = []Done{}
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *Repository) checkContact(ctx context.Context, org, id string) error {
	if id == "" {
		return nil
	}
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.contacts WHERE org_id=$1 AND id=$2`, org, id).Scan(&n); err != nil || n == 0 {
		return knowkit.Invalid("that contact does not exist")
	}
	return nil
}

func nilIf(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Create adds a contract owned by the caller and sets up its reminders.
func (r *Repository) Create(ctx context.Context, a knowkit.Actor, in Input) (*Full, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	if err := r.checkContact(ctx, a.Org, in.ContactID); err != nil {
		return nil, err
	}
	remind := in.Remind == nil || *in.Remind
	var id string
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.contracts_contracts (org_id,title,type,status,party_name,contact_id,value,currency,billing,start_date,end_date,auto_renew,term_months,notice_days,owner_id,access,shared_with,remind,tags,notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) RETURNING id::text`,
		a.Org, in.Title, in.Type, in.Status, in.PartyName, nilIf(in.ContactID), in.Value, in.Currency, in.Billing, nilIf(in.StartDate), nilIf(in.EndDate), in.AutoRenew, in.TermMonths, in.NoticeDays,
		a.User, in.Access, nonNil(in.SharedWith), remind, nonNil(in.Tags), in.Notes).Scan(&id)
	if err != nil {
		return nil, err
	}
	r.audit(ctx, "contract.create", "contract", id)
	_ = r.SyncDeadlines(ctx, a, id)
	return r.Get(ctx, a, id)
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// Update edits a contract. When the schedule changes, a recorded decision that no longer names a term end is dropped.
func (r *Repository) Update(ctx context.Context, a knowkit.Actor, id string, in Input) (*Full, error) {
	cur, err := r.load(ctx, a, id, true)
	if err != nil {
		return nil, err
	}
	if err := in.clean(); err != nil {
		return nil, err
	}
	if err := r.checkContact(ctx, a.Org, in.ContactID); err != nil {
		return nil, err
	}
	owner := cur.OwnerID
	if in.OwnerID != "" && in.OwnerID != owner && (a.Admin || cur.OwnerID == a.User) {
		if !knowkit.IsUUID(in.OwnerID) {
			return nil, knowkit.Invalid("bad owner id")
		}
		owner = in.OwnerID
	}
	remind := cur.Remind
	if in.Remind != nil {
		remind = *in.Remind
	}
	next := cur.Contract
	next.EndDate, next.AutoRenew, next.TermMonths = in.EndDate, in.AutoRenew, in.TermMonths
	dec := cur.Decision
	if dec != nil && dec.ForTermEnd != "" && !next.IsTermEnd(dec.ForTermEnd) {
		dec = nil
	}
	var decRaw any
	if dec != nil {
		b, _ := json.Marshal(dec)
		decRaw = b
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.contracts_contracts SET title=$3,type=$4,status=$5,party_name=$6,contact_id=$7,value=$8,currency=$9,billing=$10,start_date=$11,end_date=$12,
		auto_renew=$13,term_months=$14,notice_days=$15,owner_id=$16,access=$17,shared_with=$18,remind=$19,tags=$20,notes=$21,decision=$22,updated_at=now() WHERE org_id=$1 AND id=$2`,
		a.Org, id, in.Title, in.Type, in.Status, in.PartyName, nilIf(in.ContactID), in.Value, in.Currency, in.Billing, nilIf(in.StartDate), nilIf(in.EndDate), in.AutoRenew, in.TermMonths, in.NoticeDays,
		owner, in.Access, nonNil(in.SharedWith), remind, nonNil(in.Tags), in.Notes, decRaw); err != nil {
		return nil, err
	}
	// A finished series may have room again after the contract end moved.
	if obs, err := r.obligations(ctx, a.Org, id); err == nil {
		c := next
		c.Decision = dec
		for _, o := range obs {
			if n := o.Reopened(&c, r.today()); n.Done != o.Done {
				_ = r.saveObligation(ctx, a.Org, n)
			}
		}
	}
	r.audit(ctx, "contract.update", "contract", id)
	_ = r.SyncDeadlines(ctx, a, id)
	return r.Get(ctx, a, id)
}

// Delete removes a contract (soft delete) and its reminders.
func (r *Repository) Delete(ctx context.Context, a knowkit.Actor, id string) error {
	if _, err := r.load(ctx, a, id, true); err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.contracts_contracts SET deleted_at=now() WHERE org_id=$1 AND id=$2`, a.Org, id); err != nil {
		return err
	}
	_ = r.SyncDeadlines(ctx, a, id)
	r.audit(ctx, "contract.delete", "contract", id)
	return nil
}

// Decide records what the user decided about the current renewal.
func (r *Repository) Decide(ctx context.Context, a knowkit.Actor, id string, d Decision) (*Full, error) {
	f, err := r.load(ctx, a, id, true)
	if err != nil {
		return nil, err
	}
	if d.Kind == "" || d.Kind == "clear" {
		if _, err := r.pool.Exec(ctx, `UPDATE adrient.contracts_contracts SET decision=NULL, updated_at=now() WHERE id=$1`, id); err != nil {
			return nil, err
		}
		_ = r.SyncDeadlines(ctx, a, id)
		return r.Get(ctx, a, id)
	}
	if !validDecisionKind(d.Kind) {
		return nil, knowkit.Invalid("unknown decision %q", d.Kind)
	}
	end := f.Contract.NextTermEnd(r.today())
	if end == "" {
		return nil, knowkit.Invalid("this contract has no end date to decide about")
	}
	if d.Kind == DecSnoozed {
		if !IsValidDate(d.Until) || DaysBetween(r.today(), d.Until) < 0 {
			return nil, knowkit.Invalid("choose a snooze date today or later")
		}
	} else {
		d.Until = ""
	}
	if len(d.Note) > 1000 {
		return nil, knowkit.Invalid("the note is too long")
	}
	d.ForTermEnd, d.At = end, r.now().UTC().Format(time.RFC3339)
	b, _ := json.Marshal(d)
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.contracts_contracts SET decision=$2, updated_at=now() WHERE id=$1`, id, b); err != nil {
		return nil, err
	}
	r.audit(ctx, "contract.decision", "contract", id)
	_ = r.SyncDeadlines(ctx, a, id)
	return r.Get(ctx, a, id)
}

// ---- obligations -------------------------------------------------------------

// ObligationInput creates or edits an obligation.
type ObligationInput struct {
	Title      string  `json:"title"`
	Owner      string  `json:"owner"`
	Due        Date    `json:"due"`
	Repeat     *Repeat `json:"repeat"`
	AssigneeID string  `json:"assignee_id"`
	Notes      string  `json:"notes"`
}

func (in *ObligationInput) clean() error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 300 {
		return knowkit.Invalid("an obligation needs a title of up to 300 characters")
	}
	if in.Owner == "" {
		in.Owner = "us"
	}
	if !knowkit.OneOf(in.Owner, "us", "them", "mutual") {
		return knowkit.Invalid("owner must be us, them or mutual")
	}
	if in.Due != "" && !IsValidDate(in.Due) {
		return knowkit.Invalid("the due date must look like 2026-03-31")
	}
	if in.AssigneeID != "" && !knowkit.IsUUID(in.AssigneeID) {
		return knowkit.Invalid("bad assignee")
	}
	if in.Repeat != nil {
		if in.Repeat.Every == "" || in.Repeat.Every == "none" {
			in.Repeat = nil
		} else {
			if repeatMonthsOf[in.Repeat.Every] == 0 {
				return knowkit.Invalid("repeat must be monthly, quarterly or annually")
			}
			if in.Due == "" {
				return knowkit.Invalid("a repeating obligation needs a due date")
			}
			if in.Repeat.Until != "" && !IsValidDate(in.Repeat.Until) {
				return knowkit.Invalid("the repeat end date is not valid")
			}
			if in.Repeat.Count != nil && (*in.Repeat.Count < 1 || *in.Repeat.Count > 1200) {
				return knowkit.Invalid("the repeat count must be 1 to 1200")
			}
		}
	}
	if len(in.Notes) > 5000 {
		return knowkit.Invalid("notes are too long")
	}
	return nil
}

func (r *Repository) saveObligation(ctx context.Context, org string, o Obligation) error {
	var rep any
	if o.Repeat != nil {
		b, _ := json.Marshal(o.Repeat)
		rep = b
	}
	hist, _ := json.Marshal(nonNilHist(o.History))
	_, err := r.pool.Exec(ctx, `UPDATE adrient.contracts_obligations SET title=$3, owner_side=$4, due=$5, done=$6, repeat=$7, history=$8, assignee_id=$9, notes=$10 WHERE org_id=$1 AND id=$2`,
		org, o.ID, o.Title, o.Owner, nilIf(o.Due), o.Done, rep, hist, nilIf(o.AssigneeID), o.Notes)
	return err
}

func nonNilHist(h []Done) []Done {
	if h == nil {
		return []Done{}
	}
	return h
}

// AddObligation adds an obligation to a contract.
func (r *Repository) AddObligation(ctx context.Context, a knowkit.Actor, contractID string, in ObligationInput) (*Obligation, error) {
	if _, err := r.load(ctx, a, contractID, true); err != nil {
		return nil, err
	}
	if err := in.clean(); err != nil {
		return nil, err
	}
	if in.Repeat != nil {
		in.Repeat.Anchor = in.Due
	}
	var rep any
	if in.Repeat != nil {
		b, _ := json.Marshal(in.Repeat)
		rep = b
	}
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.contracts_obligations (org_id, contract_id, title, owner_side, due, repeat, assignee_id, notes) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`,
		a.Org, contractID, in.Title, in.Owner, nilIf(in.Due), rep, nilIf(in.AssigneeID), in.Notes).Scan(&id); err != nil {
		return nil, err
	}
	_ = r.SyncDeadlines(ctx, a, contractID)
	return r.obligation(ctx, a.Org, id)
}

func (r *Repository) obligation(ctx context.Context, org, id string) (*Obligation, error) {
	if !knowkit.IsUUID(id) {
		return nil, knowkit.ErrNotFound
	}
	var cid string
	if err := r.pool.QueryRow(ctx, `SELECT contract_id::text FROM adrient.contracts_obligations WHERE org_id=$1 AND id=$2`, org, id).Scan(&cid); err != nil {
		return nil, knowkit.ErrNotFound
	}
	l, err := r.obligations(ctx, org, cid)
	if err != nil {
		return nil, err
	}
	for i := range l {
		if l[i].ID == id {
			return &l[i], nil
		}
	}
	return nil, knowkit.ErrNotFound
}

// UpdateObligation edits an obligation. Editing a repeating one keeps its anchor when the due date still fits.
func (r *Repository) UpdateObligation(ctx context.Context, a knowkit.Actor, id string, in ObligationInput) (*Obligation, error) {
	cur, err := r.obligation(ctx, a.Org, id)
	if err != nil {
		return nil, err
	}
	c, err := r.load(ctx, a, cur.ContractID, true)
	if err != nil {
		return nil, err
	}
	if err := in.clean(); err != nil {
		return nil, err
	}
	n := *cur
	n.Title, n.Owner, n.Due, n.AssigneeID, n.Notes = in.Title, in.Owner, in.Due, in.AssigneeID, in.Notes
	if in.Repeat != nil {
		rp := *in.Repeat
		rp.Anchor = RepeatAnchor(in.Due, rp.Every, len(cur.History), cur.Repeat)
		n.Repeat = &rp
	} else {
		n.Repeat = nil
	}
	if !n.Done || n.Repeat != nil {
		n = n.Reopened(&c.Contract, r.today())
	}
	if err := r.saveObligation(ctx, a.Org, n); err != nil {
		return nil, err
	}
	_ = r.SyncDeadlines(ctx, a, cur.ContractID)
	return r.obligation(ctx, a.Org, id)
}

// CompleteObligation ticks off the live occurrence (or un-ticks the last one when undo is true).
func (r *Repository) CompleteObligation(ctx context.Context, a knowkit.Actor, id string, undo bool) (*Obligation, error) {
	cur, err := r.obligation(ctx, a.Org, id)
	if err != nil {
		return nil, err
	}
	c, err := r.load(ctx, a, cur.ContractID, true)
	if err != nil {
		return nil, err
	}
	var n Obligation
	if undo {
		n = cur.Rewind()
		if cur.Repeat == nil && len(cur.History) == 0 {
			n = *cur
			n.Done = false
		}
	} else {
		if cur.Done {
			return cur, nil
		}
		n = cur.Advance(&c.Contract, r.now(), r.today())
	}
	if err := r.saveObligation(ctx, a.Org, n); err != nil {
		return nil, err
	}
	_ = r.SyncDeadlines(ctx, a, cur.ContractID)
	return r.obligation(ctx, a.Org, id)
}

// DeleteObligation removes an obligation.
func (r *Repository) DeleteObligation(ctx context.Context, a knowkit.Actor, id string) error {
	cur, err := r.obligation(ctx, a.Org, id)
	if err != nil {
		return err
	}
	if _, err := r.load(ctx, a, cur.ContractID, true); err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM adrient.contracts_obligations WHERE org_id=$1 AND id=$2`, a.Org, id); err != nil {
		return err
	}
	return r.SyncDeadlines(ctx, a, cur.ContractID)
}

// AddDocument attaches a Drive file.
func (r *Repository) AddDocument(ctx context.Context, a knowkit.Actor, contractID, fileID, kind string) (*Document, error) {
	if _, err := r.load(ctx, a, contractID, true); err != nil {
		return nil, err
	}
	if !knowkit.IsUUID(fileID) {
		return nil, knowkit.Invalid("choose a Drive file")
	}
	if kind == "" {
		kind = "agreement"
	}
	if !knowkit.OneOf(kind, "agreement", "amendment", "invoice", "other") {
		return nil, knowkit.Invalid("kind must be agreement, amendment, invoice or other")
	}
	name := "file"
	if r.Deps.FileMeta == nil {
		return nil, knowkit.ErrUnavailable
	}
	n, err := r.Deps.FileMeta(ctx, a.Org, a.User, fileID)
	if err != nil {
		return nil, knowkit.Invalid("that Drive file cannot be attached: %v", err)
	}
	name = n
	d := &Document{FileID: fileID, Name: name, Kind: kind}
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.contracts_documents (org_id, contract_id, file_id, name, kind, added_by) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		a.Org, contractID, fileID, name, kind, a.User).Scan(&d.ID); err != nil {
		return nil, err
	}
	return d, nil
}

// RemoveDocument detaches a file (the Drive file itself is kept).
func (r *Repository) RemoveDocument(ctx context.Context, a knowkit.Actor, contractID, docID string) error {
	if _, err := r.load(ctx, a, contractID, true); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.contracts_documents WHERE org_id=$1 AND contract_id=$2 AND id=$3`, a.Org, contractID, docID)
	return err
}

// ---- dashboard -----------------------------------------------------------------

// Stats summarises what needs attention.
type Stats struct {
	Active         int                `json:"active"`
	NeedsAttention int                `json:"needs_attention"`
	Renewing60     int                `json:"renewing_60_days"`
	OverdueOb      int                `json:"overdue_obligations"`
	DueSoonOb      int                `json:"due_soon_obligations"`
	AnnualByCur    map[string]float64 `json:"annual_by_currency"`
}

// Overview returns the dashboard numbers and the open obligations across contracts.
type ObItem struct {
	Obligation
	ContractTitle string `json:"contract_title"`
	Urgency       string `json:"urgency"`
	DaysLeft      *int   `json:"days_left,omitempty"`
}

// Overview computes stats and the open obligations (soonest first) over the contracts the caller can see.
func (r *Repository) Overview(ctx context.Context, a knowkit.Actor) (*Stats, []ObItem, error) {
	list, err := r.List(ctx, a, "", "")
	if err != nil {
		return nil, nil, err
	}
	today := r.today()
	st := &Stats{AnnualByCur: map[string]float64{}}
	byID := map[string]*Full{}
	for _, f := range list {
		byID[f.ID] = f
		if f.Status != "active" {
			continue
		}
		st.Active++
		if f.Renewal.Attention {
			st.NeedsAttention++
		}
		if f.Renewal.DaysLeft != nil && *f.Renewal.DaysLeft >= 0 && *f.Renewal.DaysLeft <= 60 {
			st.Renewing60++
		}
		if f.Annual > 0 {
			st.AnnualByCur[f.Currency] = roundMoney(st.AnnualByCur[f.Currency] + f.Annual)
		}
	}
	items := []ObItem{}
	seen := map[string]bool{}
	for _, f := range byID {
		if seen[f.ID] || f.Status != "active" {
			continue
		}
		seen[f.ID] = true
		obs, err := r.obligations(ctx, a.Org, f.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, o := range obs {
			if o.Done {
				continue
			}
			it := ObItem{Obligation: o, ContractTitle: f.Title, Urgency: Urgency(o.Due, today)}
			if o.Due != "" {
				it.DaysLeft = ip(DaysBetween(today, o.Due))
			}
			items = append(items, it)
			switch it.Urgency {
			case UrgOverdue:
				st.OverdueOb++
			case UrgSoon:
				st.DueSoonOb++
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if (items[i].Due == "") != (items[j].Due == "") {
			return items[i].Due != ""
		}
		return items[i].Due < items[j].Due
	})
	return st, items, nil
}
