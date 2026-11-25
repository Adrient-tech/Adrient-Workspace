package fieldservice

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opsdoc"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

// Caller is the signed-in person; Admin = org admin (manages the catalogue and settings).
type Caller struct {
	OrgID, UserID string
	Admin         bool
}

// Settings are the organisation defaults.
type Settings struct {
	TaxRate        float64 `json:"tax_rate"`
	Currency       string  `json:"currency"`
	TermsDays      int     `json:"terms_days"`
	DefaultBilling string  `json:"default_billing"` // on_completion | per_visit
}

// Service is a catalogue entry used to fill quote and job lines.
type Service struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	UnitPrice   float64 `json:"unit_price"`
	UnitCost    float64 `json:"unit_cost"`
	Taxable     bool    `json:"taxable"`
	Archived    bool    `json:"archived"`
}

// Property is a place a customer wants work done.
type Property struct {
	ID        string `json:"id"`
	ContactID string `json:"contact_id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Notes     string `json:"notes"`
}

// Request is incoming work, before a quote or job exists.
type Request struct {
	ID          string  `json:"id"`
	ContactID   string  `json:"contact_id"`
	ClientName  string  `json:"client_name"`
	PropertyID  string  `json:"property_id"`
	Title       string  `json:"title"`
	Details     string  `json:"details"`
	Preferred   string  `json:"preferred_date"`
	Status      string  `json:"status"`
	Items       []Item  `json:"items"`
	TaxRate     float64 `json:"tax_rate"`
	QuoteID     string  `json:"quote_id"`
	QuoteNumber string  `json:"quote_number"`
	QuoteStatus string  `json:"quote_status,omitempty"`
	JobID       string  `json:"job_id"`
	CreatedAt   string  `json:"created_at,omitempty"`
}

// Job is agreed work with a schedule and its visits.
type Job struct {
	ID            string    `json:"id"`
	ContactID     string    `json:"contact_id"`
	ClientName    string    `json:"client_name"`
	PropertyID    string    `json:"property_id"`
	Title         string    `json:"title"`
	Instructions  string    `json:"instructions"`
	Status        string    `json:"status"` // stored: active | cancelled
	DisplayStatus string    `json:"display_status"`
	Schedule      Schedule  `json:"schedule"`
	Assignees     []string  `json:"assignees"`
	Items         []Item    `json:"items"`
	Discount      *Discount `json:"discount"`
	TaxRate       float64   `json:"tax_rate"`
	Billing       string    `json:"billing"`
	QuoteID       string    `json:"quote_id"`
	RequestID     string    `json:"request_id"`
	InvoiceIDs    []string  `json:"invoice_ids"`
	Totals        Totals    `json:"totals"`
	Visits        []Visit   `json:"visits,omitempty"`
	CreatedAt     string    `json:"created_at,omitempty"`
}

// Visit is one appointment of a job.
type Visit struct {
	ID          string            `json:"id"`
	JobID       string            `json:"job_id"`
	Date        string            `json:"date"`
	Start       string            `json:"start"`
	End         string            `json:"end"`
	Assignees   []string          `json:"assignees"`
	Status      string            `json:"status"` // scheduled | completed | cancelled
	Notes       string            `json:"notes"`
	CompletedAt string            `json:"completed_at"`
	InvoiceID   string            `json:"invoice_id"`
	EventIDs    map[string]string `json:"event_ids,omitempty"`
	JobTitle    string            `json:"job_title,omitempty"`
	ClientName  string            `json:"client_name,omitempty"`
	Address     string            `json:"address,omitempty"`
}

// Repository is the Postgres store for Field service.
type Repository struct {
	pool     *pgxpool.Pool
	docs     *opsdoc.Store
	contacts opshttp.ContactDirectory
	inv      opshttp.InvoiceWriter
	cal      opshttp.CalendarWriter
	now      func() time.Time
}

// NewRepository builds the store; the ports may be nil (the features that need them report so).
func NewRepository(pool *pgxpool.Pool, contacts opshttp.ContactDirectory, inv opshttp.InvoiceWriter, cal opshttp.CalendarWriter) *Repository {
	return &Repository{pool: pool, docs: opsdoc.New(pool, "fs_docs"), contacts: contacts, inv: inv, cal: cal, now: time.Now}
}

func (r *Repository) today() string { return r.now().Format("2006-01-02") }

func setID[T any](f func(*T, string)) func(*T, opsdoc.Doc) {
	return func(v *T, d opsdoc.Doc) { f(v, d.ID) }
}

// ---- settings ----

// GetSettings returns the defaults.
func (r *Repository) GetSettings(ctx context.Context, org string) (Settings, string, error) {
	s := Settings{Currency: "INR", TermsDays: 15, DefaultBilling: "on_completion"}
	l, err := r.docs.List(ctx, org, "settings", opsdoc.Filter{Limit: 1})
	if err != nil || len(l) == 0 {
		return s, "", err
	}
	v, err := opsdoc.Decode[Settings](l[0], nil)
	if err == nil {
		s = v
	}
	return s, l[0].ID, err
}

// SaveSettings stores the defaults (admins).
func (r *Repository) SaveSettings(ctx context.Context, c Caller, s Settings) (Settings, error) {
	if !c.Admin {
		return s, opshttp.Forbidden("only admins can change Field service settings")
	}
	if s.TaxRate < 0 || s.TaxRate > 100 || s.TermsDays < 0 || s.TermsDays > 365 || (s.DefaultBilling != "on_completion" && s.DefaultBilling != "per_visit") {
		return s, opshttp.Invalid("tax, payment terms or billing mode are out of range")
	}
	s.Currency = strings.ToUpper(strings.TrimSpace(s.Currency))
	if len(s.Currency) != 3 {
		s.Currency = "INR"
	}
	_, id, err := r.GetSettings(ctx, c.OrgID)
	if err != nil {
		return s, err
	}
	_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "settings", id, "", "", s)
	return s, err
}

// ---- customers (Contacts) ----

// Customers searches Contacts.
func (r *Repository) Customers(ctx context.Context, c Caller, q string) ([]opshttp.ContactRef, error) {
	if r.contacts == nil {
		return []opshttp.ContactRef{}, nil
	}
	return r.contacts.Search(ctx, c.OrgID, q, 25)
}

// CreateCustomer adds a customer to Contacts.
func (r *Repository) CreateCustomer(ctx context.Context, c Caller, in opshttp.ContactRef) (opshttp.ContactRef, error) {
	if r.contacts == nil {
		return in, opshttp.Invalid("Contacts is not available")
	}
	return r.contacts.Create(ctx, c.OrgID, c.UserID, in)
}

func (r *Repository) customerName(ctx context.Context, org, contactID, fallback string) (string, error) {
	if contactID == "" {
		if strings.TrimSpace(fallback) == "" {
			return "", opshttp.Invalid("choose a customer")
		}
		return strings.TrimSpace(fallback), nil
	}
	if r.contacts == nil {
		return fallback, nil
	}
	ref, err := r.contacts.Get(ctx, org, contactID)
	if err != nil {
		return "", opshttp.Invalid("that customer does not exist in Contacts")
	}
	return ref.Name, nil
}

// ---- catalogue and properties ----

// ListServices returns the catalogue.
func (r *Repository) ListServices(ctx context.Context, c Caller) ([]Service, error) {
	l, err := r.docs.List(ctx, c.OrgID, "service", opsdoc.Filter{})
	if err != nil {
		return nil, err
	}
	out := []Service{}
	for _, s := range opsdoc.DecodeAll(l, setID(func(v *Service, id string) { v.ID = id })) {
		if !s.Archived {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// SaveService creates or updates a catalogue entry (admins).
func (r *Repository) SaveService(ctx context.Context, c Caller, id string, s Service) (Service, error) {
	if !c.Admin {
		return s, opshttp.Forbidden("only admins can change the services catalogue")
	}
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" || len(s.Name) > 120 || s.UnitPrice < 0 || s.UnitCost < 0 || s.UnitPrice > 1e8 {
		return s, opshttp.Invalid("a service needs a name and non-negative prices")
	}
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "service", id, "", "", s)
	s.ID = d.ID
	return s, err
}

// ListProperties returns the properties of a customer (all when contactID is empty).
func (r *Repository) ListProperties(ctx context.Context, c Caller, contactID string) ([]Property, error) {
	l, err := r.docs.List(ctx, c.OrgID, "property", opsdoc.Filter{Ref: contactID})
	if err != nil {
		return nil, err
	}
	return opsdoc.DecodeAll(l, func(v *Property, d opsdoc.Doc) { v.ID, v.ContactID = d.ID, d.Ref }), nil
}

// SaveProperty creates or updates a property.
func (r *Repository) SaveProperty(ctx context.Context, c Caller, id string, p Property) (Property, error) {
	p.Name, p.Address = strings.TrimSpace(p.Name), strings.TrimSpace(p.Address)
	if p.ContactID == "" || (p.Name == "" && p.Address == "") || len(p.Address) > 500 {
		return p, opshttp.Invalid("a property needs a customer and a name or an address")
	}
	if _, err := r.customerName(ctx, c.OrgID, p.ContactID, ""); err != nil {
		return p, err
	}
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "property", id, p.ContactID, "", p)
	p.ID = d.ID
	return p, err
}

func (r *Repository) propertyLabel(ctx context.Context, org, id string) string {
	if id == "" {
		return ""
	}
	d, err := r.docs.Get(ctx, org, "property", id)
	if err != nil {
		return ""
	}
	p, _ := opsdoc.Decode[Property](d, nil)
	if p.Address != "" {
		return p.Address
	}
	return p.Name
}

func cleanItems(items []Item) ([]Item, error) {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		it.Name = strings.TrimSpace(it.Name)
		if it.Name == "" {
			continue
		}
		if it.Qty <= 0 || it.Qty > 1e6 || it.UnitPrice < 0 || it.UnitPrice > 1e8 || it.UnitCost < 0 {
			return nil, opshttp.Invalid("line %q needs a positive quantity and a non-negative price", it.Name)
		}
		out = append(out, it)
	}
	return out, nil
}

// ---- requests and quotes ----

func (r *Repository) requestOf(ctx context.Context, org string, d opsdoc.Doc) Request {
	q, _ := opsdoc.Decode(d, func(v *Request, d opsdoc.Doc) { v.ID, v.CreatedAt = d.ID, d.CreatedAt.Format("2006-01-02") })
	if q.Items == nil {
		q.Items = []Item{}
	}
	return q
}

// ListRequests returns requests, newest first.
func (r *Repository) ListRequests(ctx context.Context, c Caller, status string) ([]Request, error) {
	l, err := r.docs.List(ctx, c.OrgID, "request", opsdoc.Filter{})
	if err != nil {
		return nil, err
	}
	out := []Request{}
	for _, d := range l {
		q := r.requestOf(ctx, c.OrgID, d)
		if status == "" || q.Status == status {
			out = append(out, q)
		}
	}
	return out, nil
}

// SaveRequest creates (id == "") or edits a request.
func (r *Repository) SaveRequest(ctx context.Context, c Caller, id string, in Request) (Request, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 160 || len(in.Details) > 4000 {
		return in, opshttp.Invalid("a request needs a title of up to 160 characters")
	}
	name, err := r.customerName(ctx, c.OrgID, in.ContactID, in.ClientName)
	if err != nil {
		return in, err
	}
	in.ClientName = name
	if in.Preferred != "" && !opshttp.ValidDate(in.Preferred) {
		return in, opshttp.Invalid("the preferred date must be a date")
	}
	if in.Items, err = cleanItems(in.Items); err != nil {
		return in, err
	}
	if id == "" {
		in.Status, in.QuoteID, in.QuoteNumber, in.JobID = ReqNew, "", "", ""
	} else {
		d, err := r.docs.Get(ctx, c.OrgID, "request", id)
		if err != nil {
			return in, err
		}
		old := r.requestOf(ctx, c.OrgID, d)
		in.Status, in.QuoteID, in.QuoteNumber, in.JobID = old.Status, old.QuoteID, old.QuoteNumber, old.JobID
	}
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "request", id, in.ContactID, in.Preferred, in)
	if err != nil {
		return in, err
	}
	in.ID = d.ID
	return in, nil
}

// MoveRequest changes the status (new, assessment, quoted, closed).
func (r *Repository) MoveRequest(ctx context.Context, c Caller, id, status string) (Request, error) {
	d, err := r.docs.Get(ctx, c.OrgID, "request", id)
	if err != nil {
		return Request{}, err
	}
	q := r.requestOf(ctx, c.OrgID, d)
	if status == ReqConverted {
		return q, opshttp.Invalid("use Create job to convert a request")
	}
	if !CanMoveRequest(q.Status, status) {
		return q, opshttp.Conflict("a %s request cannot become %s", q.Status, status)
	}
	q.Status = status
	_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "request", id, q.ContactID, q.Preferred, q)
	return q, err
}

// CreateQuote creates an estimate in Invoicing from the request's lines and
// marks the request as quoted. Creating it twice returns the first quote.
func (r *Repository) CreateQuote(ctx context.Context, c Caller, id string) (Request, error) {
	if r.inv == nil {
		return Request{}, opshttp.Invalid("Invoicing is not available")
	}
	d, err := r.docs.Get(ctx, c.OrgID, "request", id)
	if err != nil {
		return Request{}, err
	}
	q := r.requestOf(ctx, c.OrgID, d)
	if q.QuoteID != "" {
		return q, opshttp.Conflict("this request already has quote %s", q.QuoteNumber)
	}
	if len(q.Items) == 0 {
		return q, opshttp.Invalid("add at least one line to the request before quoting")
	}
	set, _, _ := r.GetSettings(ctx, c.OrgID)
	tax := q.TaxRate
	if tax == 0 {
		tax = set.TaxRate
	}
	doc, err := r.inv.Create(ctx, c.OrgID, c.UserID, opshttp.InvoiceDraft{Kind: "estimate", ContactID: q.ContactID, ClientName: q.ClientName, Currency: set.Currency,
		IssueDate: r.today(), Lines: invLines(q.Items, 1, tax), Notes: "Quote for: " + q.Title, SourceRef: "fieldservice"})
	if err != nil {
		return q, err
	}
	q.QuoteID, q.QuoteNumber = doc.ID, doc.Number
	if q.Status == ReqNew || q.Status == ReqAssessment {
		q.Status = ReqQuoted
	}
	_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "request", id, q.ContactID, q.Preferred, q)
	return q, err
}

func invLines(items []Item, mult float64, tax float64) []opshttp.InvLine {
	var out []opshttp.InvLine
	for _, it := range items {
		if !it.Included() {
			continue
		}
		t := 0.0
		if it.Taxable {
			t = tax
		}
		out = append(out, opshttp.InvLine{Description: it.Name, Qty: it.Qty * mult, Unit: "", Price: it.UnitPrice, TaxRate: t})
	}
	return out
}

// QuoteStatus reads the live status of the request's quote in Invoicing.
func (r *Repository) QuoteStatus(ctx context.Context, c Caller, q Request) string {
	if q.QuoteID == "" || r.inv == nil {
		return ""
	}
	d, err := r.inv.Get(ctx, c.OrgID, q.QuoteID)
	if err != nil {
		return ""
	}
	return d.Status
}

// ---- jobs ----

func (r *Repository) visitsOf(ctx context.Context, org, jobID string) ([]Visit, error) {
	l, err := r.docs.List(ctx, org, "visit", opsdoc.Filter{Ref: jobID})
	if err != nil {
		return nil, err
	}
	v := opsdoc.DecodeAll(l, func(v *Visit, d opsdoc.Doc) { v.ID, v.JobID, v.Date = d.ID, d.Ref, d.Day })
	sort.Slice(v, func(i, j int) bool { return v[i].Date < v[j].Date })
	return v, nil
}

func (r *Repository) jobOf(ctx context.Context, org string, d opsdoc.Doc, withVisits bool) (Job, error) {
	j, err := opsdoc.Decode(d, func(v *Job, d opsdoc.Doc) { v.ID, v.CreatedAt = d.ID, d.CreatedAt.Format("2006-01-02") })
	if err != nil {
		return j, err
	}
	if j.Items == nil {
		j.Items = []Item{}
	}
	if j.InvoiceIDs == nil {
		j.InvoiceIDs = []string{}
	}
	j.Totals = Compute(j.Items, j.Discount, j.TaxRate)
	vs, err := r.visitsOf(ctx, org, j.ID)
	if err != nil {
		return j, err
	}
	st := make([]VisitState, 0, len(vs))
	allBilled := len(vs) > 0
	for _, v := range vs {
		st = append(st, VisitState{Date: v.Date, Status: v.Status})
		if v.Status == "completed" && v.InvoiceID == "" {
			allBilled = false
		}
	}
	j.DisplayStatus = JobStatus(j.Status, st, allBilled && len(j.InvoiceIDs) > 0, r.today())
	if withVisits {
		j.Visits = vs
	}
	return j, nil
}

// JobFilter narrows ListJobs.
type JobFilter struct{ Status, Assignee, ContactID string }

// ListJobs returns jobs with their derived status.
func (r *Repository) ListJobs(ctx context.Context, c Caller, f JobFilter) ([]Job, error) {
	l, err := r.docs.List(ctx, c.OrgID, "job", opsdoc.Filter{Ref: f.ContactID})
	if err != nil {
		return nil, err
	}
	out := []Job{}
	for _, d := range l {
		j, err := r.jobOf(ctx, c.OrgID, d, false)
		if err != nil {
			continue
		}
		if f.Status != "" && j.DisplayStatus != f.Status {
			continue
		}
		if f.Assignee != "" && !contains(j.Assignees, f.Assignee) {
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// GetJob returns one job with its visits.
func (r *Repository) GetJob(ctx context.Context, c Caller, id string) (Job, error) {
	d, err := r.docs.Get(ctx, c.OrgID, "job", id)
	if err != nil {
		return Job{}, err
	}
	return r.jobOf(ctx, c.OrgID, d, true)
}

func validSchedule(s *Schedule) error {
	if !opshttp.ValidDate(s.Start) {
		return opshttp.Invalid("the first visit needs a valid date")
	}
	switch s.Repeat {
	case "", "none", "daily", "weekly", "monthly", "yearly":
	default:
		return opshttp.Invalid("repeat must be none, daily, weekly, monthly or yearly")
	}
	if s.Interval < 0 || s.Interval > 52 || s.EndCount < 0 || s.EndCount > MaxVisits {
		return opshttp.Invalid("interval or visit count is out of range")
	}
	if s.EndType == "until" && !opshttp.ValidDate(s.EndDate) {
		return opshttp.Invalid("the end date must be a date")
	}
	if (s.StartTime != "" && !opshttp.ValidTime(s.StartTime)) || (s.EndTime != "" && !opshttp.ValidTime(s.EndTime)) {
		return opshttp.Invalid("times must be HH:MM")
	}
	if s.StartTime != "" && s.EndTime != "" && s.EndTime <= s.StartTime {
		return opshttp.Invalid("a visit must end after it starts (split overnight work in two visits)")
	}
	return nil
}

// SaveJob creates (id == "") or edits a job. Visits are generated from the
// schedule; on edit, completed and invoiced visits are kept and the open ones
// are rebuilt (their notes are kept per date).
func (r *Repository) SaveJob(ctx context.Context, c Caller, id string, in Job) (Job, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 160 || len(in.Instructions) > 4000 {
		return in, opshttp.Invalid("a job needs a title of up to 160 characters")
	}
	name, err := r.customerName(ctx, c.OrgID, in.ContactID, in.ClientName)
	if err != nil {
		return in, err
	}
	in.ClientName = name
	if err := validSchedule(&in.Schedule); err != nil {
		return in, err
	}
	if in.Items, err = cleanItems(in.Items); err != nil {
		return in, err
	}
	if in.TaxRate < 0 || in.TaxRate > 100 {
		return in, opshttp.Invalid("tax must be between 0 and 100")
	}
	set, _, _ := r.GetSettings(ctx, c.OrgID)
	if in.Billing != "per_visit" && in.Billing != "on_completion" {
		in.Billing = set.DefaultBilling
	}
	if in.Assignees == nil {
		in.Assignees = []string{}
	}
	in.Status = "active"
	var old Job
	if id != "" {
		od, err := r.docs.Get(ctx, c.OrgID, "job", id)
		if err != nil {
			return in, err
		}
		old, _ = r.jobOf(ctx, c.OrgID, od, true)
		in.InvoiceIDs, in.QuoteID, in.RequestID, in.Status = old.InvoiceIDs, old.QuoteID, old.RequestID, old.Status
	} else {
		in.InvoiceIDs = []string{}
	}
	in.Visits, in.Totals, in.DisplayStatus = nil, Totals{}, ""
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "job", id, in.ContactID, in.Schedule.Start, in)
	if err != nil {
		return in, err
	}
	in.ID = d.ID
	if err := r.rebuildVisits(ctx, c, in, old.Visits); err != nil {
		return in, err
	}
	return r.GetJob(ctx, c, in.ID)
}

func (r *Repository) rebuildVisits(ctx context.Context, c Caller, j Job, existing []Visit) error {
	keep := map[string]bool{}
	open := map[string]Visit{}
	for _, v := range existing {
		if v.Status == "completed" || v.InvoiceID != "" {
			keep[v.Date] = true
		} else if v.Status == "scheduled" {
			open[v.Date] = v
		}
	}
	want := map[string]bool{}
	for _, date := range VisitDates(j.Schedule) {
		want[date] = true
		if keep[date] {
			continue
		}
		v, had := open[date]
		if !had {
			v = Visit{JobID: j.ID, Date: date, Status: "scheduled"}
		}
		v.Start, v.End, v.Assignees = j.Schedule.StartTime, j.Schedule.EndTime, j.Assignees
		saved, err := r.docs.Put(ctx, c.OrgID, c.UserID, "visit", v.ID, j.ID, date, v)
		if err != nil {
			return err
		}
		v.ID = saved.ID
		r.syncCalendar(ctx, c, j, &v)
		if _, err := r.docs.Put(ctx, c.OrgID, c.UserID, "visit", v.ID, j.ID, date, v); err != nil {
			return err
		}
	}
	for date, v := range open {
		if !want[date] {
			r.dropCalendar(ctx, c, &v)
			if err := r.docs.Delete(ctx, c.OrgID, "visit", v.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// CancelJob cancels a job and its open visits (their Calendar items go away).
func (r *Repository) CancelJob(ctx context.Context, c Caller, id string) (Job, error) {
	j, err := r.GetJob(ctx, c, id)
	if err != nil {
		return j, err
	}
	for _, v := range j.Visits {
		if v.Status == "scheduled" {
			r.dropCalendar(ctx, c, &v)
			v.Status = "cancelled"
			if _, err := r.docs.Put(ctx, c.OrgID, c.UserID, "visit", v.ID, id, v.Date, v); err != nil {
				return j, err
			}
		}
	}
	j.Status, j.Visits, j.Totals, j.DisplayStatus = "cancelled", nil, Totals{}, ""
	if _, err := r.docs.Put(ctx, c.OrgID, c.UserID, "job", id, j.ContactID, j.Schedule.Start, j); err != nil {
		return j, err
	}
	return r.GetJob(ctx, c, id)
}

// CreateJobFromRequest turns a request (and its quote lines) into a job.
func (r *Repository) CreateJobFromRequest(ctx context.Context, c Caller, id string, sched Schedule, assignees []string) (Job, error) {
	d, err := r.docs.Get(ctx, c.OrgID, "request", id)
	if err != nil {
		return Job{}, err
	}
	q := r.requestOf(ctx, c.OrgID, d)
	if q.JobID != "" {
		return Job{}, opshttp.Conflict("this request already became a job")
	}
	if sched.Start == "" {
		sched.Start = q.Preferred
	}
	if sched.Start == "" {
		sched.Start = r.today()
	}
	j, err := r.SaveJob(ctx, c, "", Job{ContactID: q.ContactID, ClientName: q.ClientName, PropertyID: q.PropertyID, Title: q.Title, Instructions: q.Details,
		Schedule: sched, Assignees: assignees, Items: q.Items, TaxRate: q.TaxRate})
	if err != nil {
		return j, err
	}
	// Remember where the job came from, on both records.
	od, _ := r.docs.Get(ctx, c.OrgID, "job", j.ID)
	raw, _ := opsdoc.Decode[Job](od, nil)
	raw.QuoteID, raw.RequestID = q.QuoteID, q.ID
	raw.Visits, raw.Totals = nil, Totals{}
	if _, err := r.docs.Put(ctx, c.OrgID, c.UserID, "job", j.ID, j.ContactID, j.Schedule.Start, raw); err != nil {
		return j, err
	}
	q.JobID, q.Status = j.ID, ReqConverted
	if _, err := r.docs.Put(ctx, c.OrgID, c.UserID, "request", id, q.ContactID, q.Preferred, q); err != nil {
		return j, err
	}
	return r.GetJob(ctx, c, j.ID)
}

// ---- visits, calendar ----

// ListVisits returns visits in a date range for the schedule, optionally for one person.
func (r *Repository) ListVisits(ctx context.Context, c Caller, from, to, assignee string) ([]Visit, error) {
	if !opshttp.ValidDate(from) || !opshttp.ValidDate(to) || to < from {
		return nil, opshttp.Invalid("from and to must be dates")
	}
	l, err := r.docs.List(ctx, c.OrgID, "visit", opsdoc.Filter{From: from, To: to})
	if err != nil {
		return nil, err
	}
	vs := opsdoc.DecodeAll(l, func(v *Visit, d opsdoc.Doc) { v.ID, v.JobID, v.Date = d.ID, d.Ref, d.Day })
	jobs := map[string]Job{}
	out := []Visit{}
	for _, v := range vs {
		if assignee != "" && !contains(v.Assignees, assignee) {
			continue
		}
		j, ok := jobs[v.JobID]
		if !ok {
			if d, err := r.docs.Get(ctx, c.OrgID, "job", v.JobID); err == nil {
				j, _ = opsdoc.Decode(d, func(x *Job, d opsdoc.Doc) { x.ID = d.ID })
			}
			jobs[v.JobID] = j
		}
		v.JobTitle, v.ClientName = j.Title, j.ClientName
		v.Address = r.propertyLabel(ctx, c.OrgID, j.PropertyID)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].Start < out[j].Start
	})
	return out, nil
}

func (r *Repository) visitTimes(v Visit) (time.Time, time.Time, bool) {
	d, err := time.ParseInLocation("2006-01-02", v.Date, time.Local)
	if err != nil {
		return d, d, false
	}
	if v.Start == "" {
		return d, d.AddDate(0, 0, 1), true
	}
	s, _ := time.ParseInLocation("2006-01-02 15:04", v.Date+" "+v.Start, time.Local)
	e := s.Add(time.Hour)
	if v.End != "" {
		e, _ = time.ParseInLocation("2006-01-02 15:04", v.Date+" "+v.End, time.Local)
	}
	return s, e, true
}

// syncCalendar creates or moves one Calendar item per assignee for a scheduled visit.
func (r *Repository) syncCalendar(ctx context.Context, c Caller, j Job, v *Visit) {
	if r.cal == nil || v.Status != "scheduled" {
		return
	}
	s, e, ok := r.visitTimes(*v)
	if !ok {
		return
	}
	ev := opshttp.CalEvent{Title: j.Title + " – " + j.ClientName, Description: strings.TrimSpace(j.Instructions + "\n" + v.Notes), Location: r.propertyLabel(ctx, c.OrgID, j.PropertyID),
		Start: s, End: e, AllDay: v.Start == "", Color: "#C2410C"}
	if v.EventIDs == nil {
		v.EventIDs = map[string]string{}
	}
	for user, id := range v.EventIDs {
		if !contains(v.Assignees, user) {
			_ = r.cal.Delete(ctx, c.OrgID, id)
			delete(v.EventIDs, user)
		}
	}
	for _, user := range v.Assignees {
		if id, ok := v.EventIDs[user]; ok {
			if err := r.cal.Update(ctx, c.OrgID, id, ev); err == nil {
				continue
			}
		}
		if id, err := r.cal.Create(ctx, c.OrgID, user, ev); err == nil {
			v.EventIDs[user] = id
		}
	}
}

func (r *Repository) dropCalendar(ctx context.Context, c Caller, v *Visit) {
	if r.cal == nil {
		return
	}
	for u, id := range v.EventIDs {
		_ = r.cal.Delete(ctx, c.OrgID, id)
		delete(v.EventIDs, u)
	}
}

func (r *Repository) getVisit(ctx context.Context, c Caller, id string) (Visit, Job, error) {
	d, err := r.docs.Get(ctx, c.OrgID, "visit", id)
	if err != nil {
		return Visit{}, Job{}, err
	}
	v, _ := opsdoc.Decode(d, func(v *Visit, d opsdoc.Doc) { v.ID, v.JobID, v.Date = d.ID, d.Ref, d.Day })
	jd, err := r.docs.Get(ctx, c.OrgID, "job", v.JobID)
	if err != nil {
		return v, Job{}, err
	}
	j, err := r.jobOf(ctx, c.OrgID, jd, false)
	return v, j, err
}

// VisitUpdate changes when and who. Zero fields keep their value.
type VisitUpdate struct {
	Date      string    `json:"date"`
	Start     *string   `json:"start"`
	End       *string   `json:"end"`
	Assignees *[]string `json:"assignees"`
	Notes     *string   `json:"notes"`
}

// UpdateVisit reschedules or reassigns an open visit and moves its Calendar items.
func (r *Repository) UpdateVisit(ctx context.Context, c Caller, id string, u VisitUpdate) (Visit, error) {
	v, j, err := r.getVisit(ctx, c, id)
	if err != nil {
		return v, err
	}
	if v.Status != "scheduled" {
		return v, opshttp.Conflict("only scheduled visits can be changed")
	}
	if u.Date != "" {
		if !opshttp.ValidDate(u.Date) {
			return v, opshttp.Invalid("the date must be a date")
		}
		v.Date = u.Date
	}
	if u.Start != nil {
		v.Start = *u.Start
	}
	if u.End != nil {
		v.End = *u.End
	}
	if (v.Start != "" && !opshttp.ValidTime(v.Start)) || (v.End != "" && !opshttp.ValidTime(v.End)) || (v.Start != "" && v.End != "" && v.End <= v.Start) {
		return v, opshttp.Invalid("a visit needs valid times and must end after it starts")
	}
	if u.Assignees != nil {
		v.Assignees = *u.Assignees
	}
	if u.Notes != nil {
		if len(*u.Notes) > 2000 {
			return v, opshttp.Invalid("notes are too long")
		}
		v.Notes = *u.Notes
	}
	r.syncCalendar(ctx, c, j, &v)
	_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "visit", id, v.JobID, v.Date, v)
	return v, err
}

// CompleteVisit marks a visit done; with per-visit billing it invoices it at once.
func (r *Repository) CompleteVisit(ctx context.Context, c Caller, id string) (Visit, *opshttp.DocRef, error) {
	v, j, err := r.getVisit(ctx, c, id)
	if err != nil {
		return v, nil, err
	}
	if v.Status != "scheduled" {
		return v, nil, opshttp.Conflict("this visit is already %s", v.Status)
	}
	v.Status, v.CompletedAt = "completed", r.now().Format(time.RFC3339)
	if _, err := r.docs.Put(ctx, c.OrgID, c.UserID, "visit", id, v.JobID, v.Date, v); err != nil {
		return v, nil, err
	}
	if j.Billing == "per_visit" && r.inv != nil && len(j.Items) > 0 {
		doc, err := r.invoiceVisits(ctx, c, j, []Visit{v})
		if err != nil {
			return v, nil, err
		}
		v.InvoiceID = doc.ID
		return v, &doc, nil
	}
	return v, nil, nil
}

// ReopenVisit puts a completed, uninvoiced visit back on the schedule.
func (r *Repository) ReopenVisit(ctx context.Context, c Caller, id string) (Visit, error) {
	v, j, err := r.getVisit(ctx, c, id)
	if err != nil {
		return v, err
	}
	if v.Status != "completed" || v.InvoiceID != "" {
		return v, opshttp.Conflict("only completed visits that are not invoiced can be reopened")
	}
	v.Status, v.CompletedAt = "scheduled", ""
	r.syncCalendar(ctx, c, j, &v)
	_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "visit", id, v.JobID, v.Date, v)
	return v, err
}

// CancelVisit cancels one open visit.
func (r *Repository) CancelVisit(ctx context.Context, c Caller, id string) (Visit, error) {
	v, _, err := r.getVisit(ctx, c, id)
	if err != nil {
		return v, err
	}
	if v.Status != "scheduled" {
		return v, opshttp.Conflict("only scheduled visits can be cancelled")
	}
	r.dropCalendar(ctx, c, &v)
	v.Status = "cancelled"
	_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "visit", id, v.JobID, v.Date, v)
	return v, err
}

// ---- invoicing ----

func (r *Repository) invoiceVisits(ctx context.Context, c Caller, j Job, vs []Visit) (opshttp.DocRef, error) {
	set, _, _ := r.GetSettings(ctx, c.OrgID)
	tax := j.TaxRate
	if tax == 0 {
		tax = set.TaxRate
	}
	dk, dv := "", 0.0
	if j.Discount != nil && j.Discount.Value > 0 {
		dk, dv = "percent", j.Discount.Value
		if j.Discount.Kind != "pct" {
			dk, dv = "amount", j.Discount.Value*float64(len(vs))
		}
	}
	note := fmt.Sprintf("Job: %s", j.Title)
	if len(vs) == 1 {
		note += " (visit " + vs[0].Date + ")"
	}
	issue := r.today()
	doc, err := r.inv.Create(ctx, c.OrgID, c.UserID, opshttp.InvoiceDraft{Kind: "invoice", ContactID: j.ContactID, ClientName: j.ClientName, Currency: set.Currency,
		IssueDate: issue, DueDate: opshttp.AddDays(issue, set.TermsDays), Lines: invLines(j.Items, float64(len(vs)), tax), Notes: note,
		SourceRef: "fieldservice", DiscountKind: dk, DiscountValue: dv})
	if err != nil {
		return doc, err
	}
	for _, v := range vs {
		v.InvoiceID = doc.ID
		if _, err := r.docs.Put(ctx, c.OrgID, c.UserID, "visit", v.ID, v.JobID, v.Date, v); err != nil {
			return doc, err
		}
	}
	jd, err := r.docs.Get(ctx, c.OrgID, "job", j.ID)
	if err == nil {
		raw, _ := opsdoc.Decode[Job](jd, nil)
		raw.InvoiceIDs = append(raw.InvoiceIDs, doc.ID)
		raw.Visits, raw.Totals = nil, Totals{}
		_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "job", j.ID, raw.ContactID, raw.Schedule.Start, raw)
	}
	return doc, err
}

// InvoiceJob invoices every completed, not yet invoiced visit of a job in one
// invoice (lines = job lines × number of visits).
func (r *Repository) InvoiceJob(ctx context.Context, c Caller, id string) (opshttp.DocRef, int, error) {
	if r.inv == nil {
		return opshttp.DocRef{}, 0, opshttp.Invalid("Invoicing is not available")
	}
	j, err := r.GetJob(ctx, c, id)
	if err != nil {
		return opshttp.DocRef{}, 0, err
	}
	if len(j.Items) == 0 {
		return opshttp.DocRef{}, 0, opshttp.Invalid("add lines to the job before invoicing it")
	}
	var todo []Visit
	for _, v := range j.Visits {
		if v.Status == "completed" && v.InvoiceID == "" {
			todo = append(todo, v)
		}
	}
	if len(todo) == 0 {
		return opshttp.DocRef{}, 0, opshttp.Invalid("there are no completed visits waiting to be invoiced")
	}
	doc, err := r.invoiceVisits(ctx, c, j, todo)
	return doc, len(todo), err
}

// InvoiceStatus lists the live status of a job's invoices.
func (r *Repository) InvoiceStatus(ctx context.Context, c Caller, j Job) []opshttp.DocRef {
	out := []opshttp.DocRef{}
	if r.inv == nil {
		return out
	}
	for _, id := range j.InvoiceIDs {
		if d, err := r.inv.Get(ctx, c.OrgID, id); err == nil {
			out = append(out, d)
		}
	}
	return out
}

// ---- dashboard ----

// Dashboard is the home screen summary.
type Dashboard struct {
	NewRequests      int     `json:"new_requests"`
	QuotesOut        int     `json:"quotes_awaiting"`
	ActiveJobs       int     `json:"active_jobs"`
	LateJobs         int     `json:"late_jobs"`
	RequiresInvoice  int     `json:"requires_invoicing"`
	VisitsToday      []Visit `json:"visits_today"`
	VisitsThisWeek   int     `json:"visits_this_week"`
	OutstandingTotal float64 `json:"outstanding_amount"`
}

// Dashboard summarises the pipeline.
func (r *Repository) Dashboard(ctx context.Context, c Caller) (Dashboard, error) {
	var d Dashboard
	reqs, err := r.ListRequests(ctx, c, "")
	if err != nil {
		return d, err
	}
	for _, q := range reqs {
		switch q.Status {
		case ReqNew:
			d.NewRequests++
		case ReqQuoted:
			d.QuotesOut++
		}
	}
	jobs, err := r.ListJobs(ctx, c, JobFilter{})
	if err != nil {
		return d, err
	}
	seen := map[string]bool{}
	for _, j := range jobs {
		switch j.DisplayStatus {
		case "active":
			d.ActiveJobs++
		case "late":
			d.ActiveJobs++
			d.LateJobs++
		case "requires_invoicing":
			d.RequiresInvoice++
		}
		for _, id := range j.InvoiceIDs {
			if seen[id] || r.inv == nil {
				continue
			}
			seen[id] = true
			if doc, err := r.inv.Get(ctx, c.OrgID, id); err == nil {
				d.OutstandingTotal += doc.Balance
			}
		}
	}
	t := r.today()
	d.VisitsToday, err = r.ListVisits(ctx, c, t, t, "")
	if err != nil {
		return d, err
	}
	week, _ := r.ListVisits(ctx, c, t, opshttp.AddDays(t, 6), "")
	d.VisitsThisWeek = len(week)
	d.OutstandingTotal = opshttp.Round2(d.OutstandingTotal)
	return d, nil
}
