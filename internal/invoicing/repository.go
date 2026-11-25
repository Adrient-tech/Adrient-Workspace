package invoicing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned when no record matches within the org.
	ErrNotFound = errors.New("not found")
	// ErrInvalid wraps validation failures.
	ErrInvalid = errors.New("invalid")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Repository is the Postgres store for invoicing.
type Repository struct {
	pool     *pgxpool.Pool
	reserver StockReserver
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// ---- types ----

// Settings are the per-org numbering, defaults and branding.
type Settings struct {
	BusinessName    string             `json:"business_name"`
	Address         string             `json:"address"`
	TaxID           string             `json:"tax_id"`
	Email           string             `json:"email"`
	Phone           string             `json:"phone"`
	LogoURL         string             `json:"logo_url"`
	InvoicePrefix   string             `json:"invoice_prefix"`
	EstimatePrefix  string             `json:"estimate_prefix"`
	NextInvoice     int                `json:"next_invoice"`
	NextEstimate    int                `json:"next_estimate"`
	DefaultCurrency string             `json:"default_currency"`
	DueDays         int                `json:"due_days"`
	DefaultNotes    string             `json:"default_notes"`
	DefaultTerms    string             `json:"default_terms"`
	TaxNote         string             `json:"tax_note"`
	PaymentInfo     string             `json:"payment_info"`
	Units           []string           `json:"units"`
	Rates           map[string]float64 `json:"rates"`
	DeductStock     bool               `json:"deduct_stock"`
	ReminderDays    int                `json:"reminder_days"`
	Sales           SalesSettings      `json:"sales"`
}

// Item is a price list entry.
type Item struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Unit        string  `json:"unit"`
	Price       float64 `json:"price"`
	TaxRate     float64 `json:"tax_rate"`
}

// Schedule is one instalment of a payment schedule.
type Schedule struct {
	Label   string  `json:"label"`
	DueDate string  `json:"due_date"`
	Amount  float64 `json:"amount"`
}

// Attachment is a link to a file (for example a Drive share link).
type Attachment struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// DocInput is the editable part of a document.
type DocInput struct {
	Kind          string       `json:"kind"`
	ContactID     string       `json:"contact_id"`
	ClientName    string       `json:"client_name"`
	ClientEmail   string       `json:"client_email"`
	ClientAddress string       `json:"client_address"`
	Currency      string       `json:"currency"`
	IssueDate     string       `json:"issue_date"`
	DueDate       string       `json:"due_date"`
	Lines         []Line       `json:"lines"`
	DiscountKind  string       `json:"discount_kind"`
	DiscountValue float64      `json:"discount_value"`
	Notes         string       `json:"notes"`
	Terms         string       `json:"terms"`
	TaxNote       string       `json:"tax_note"`
	Schedule      []Schedule   `json:"schedule"`
	Attachments   []Attachment `json:"attachments"`
	Number        string       `json:"number"`

	ValidUntil     string `json:"valid_until"`
	PlaceOfSupply  string `json:"place_of_supply"`
	ClientGSTIN    string `json:"client_gstin"`
	PriceListID    string `json:"price_list_id"`
	SourceRef      string `json:"source_ref"`
	SalesOrderID   string `json:"sales_order_id"`
	CreditFor      string `json:"credit_for"`
	SubscriptionID string `json:"subscription_id"`
	RecurringID    string `json:"-"`
	RecurringDate  string `json:"-"`
	VersionGroup   string `json:"-"`
	Version        int    `json:"-"`
	NoStock        bool   `json:"-"` // never deduct stock (the order moves it at dispatch)
}

// Payment is a payment, refund or write-off row.
type Payment struct {
	ID         string  `json:"id"`
	DocumentID string  `json:"document_id"`
	Kind       string  `json:"kind"`
	Amount     float64 `json:"amount"`
	Method     string  `json:"method"`
	Reference  string  `json:"reference"`
	Note       string  `json:"note"`
	PaidAt     string  `json:"paid_at"`
	// Joined for list views.
	DocNumber  string `json:"doc_number,omitempty"`
	ClientName string `json:"client_name,omitempty"`
	Currency   string `json:"currency,omitempty"`
}

// Document is an invoice or estimate with its derived state.
type Document struct {
	ID            string       `json:"id"`
	Kind          string       `json:"kind"`
	Number        string       `json:"number"`
	Status        string       `json:"status"`         // stored
	DisplayStatus string       `json:"display_status"` // derived
	ContactID     string       `json:"contact_id"`
	ClientName    string       `json:"client_name"`
	ClientEmail   string       `json:"client_email"`
	ClientAddress string       `json:"client_address"`
	Currency      string       `json:"currency"`
	IssueDate     string       `json:"issue_date"`
	DueDate       string       `json:"due_date"`
	Lines         []Line       `json:"lines"`
	DiscountKind  string       `json:"discount_kind"`
	DiscountValue float64      `json:"discount_value"`
	Subtotal      float64      `json:"subtotal"`
	DiscountTotal float64      `json:"discount_total"`
	TaxTotal      float64      `json:"tax_total"`
	Total         float64      `json:"total"`
	Notes         string       `json:"notes"`
	Terms         string       `json:"terms"`
	TaxNote       string       `json:"tax_note"`
	Schedule      []Schedule   `json:"schedule"`
	Attachments   []Attachment `json:"attachments"`
	ShareToken    string       `json:"share_token"`
	RecurringID   string       `json:"recurring_id"`
	ConvertedFrom string       `json:"converted_from"`
	StockDeducted bool         `json:"stock_deducted"`
	SentAt        string       `json:"sent_at"`
	ViewedAt      string       `json:"viewed_at"`
	RemindersSent int          `json:"reminders_sent"`
	Deleted       bool         `json:"deleted"`
	CreatedAt     string       `json:"created_at"`
	Paid          float64      `json:"paid"`
	WrittenOff    float64      `json:"written_off"`
	Balance       float64      `json:"balance"`
	Payments      []Payment    `json:"payments"`
	// Derived from the schedule and the payments; never stored.
	Instalments   []Instalment `json:"instalments"`
	EffectiveDue  string       `json:"effective_due"`
	OverdueAmount float64      `json:"overdue_amount"`

	ValidUntil     string  `json:"valid_until"`
	Version        int     `json:"version"`
	VersionGroup   string  `json:"version_group"`
	AcceptedAt     string  `json:"accepted_at"`
	AcceptedName   string  `json:"accepted_name"`
	AcceptedHash   string  `json:"accepted_hash"`
	DeclinedReason string  `json:"declined_reason"`
	SalesOrderID   string  `json:"sales_order_id"`
	CreditFor      string  `json:"credit_for"`
	PlaceOfSupply  string  `json:"place_of_supply"`
	SupplierState  string  `json:"supplier_state"`
	ClientGSTIN    string  `json:"client_gstin"`
	CGST           float64 `json:"cgst_total"`
	SGST           float64 `json:"sgst_total"`
	IGST           float64 `json:"igst_total"`
	PriceListID    string  `json:"price_list_id"`
	SourceRef      string  `json:"source_ref"`
	SubscriptionID string  `json:"subscription_id"`
	PaymentLinkURL string  `json:"payment_link_url"`
	Credited       float64 `json:"credited"`       // credit notes applied to this invoice
	CreditRemain   float64 `json:"credit_remaining"` // credit notes: not yet applied or refunded
	LatestID       string  `json:"latest_id,omitempty"`
	LatestToken    string  `json:"latest_token,omitempty"`
	Gst            *Breakup `json:"gst,omitempty"`
}

// Recurring is a template that generates invoices on a schedule.
type Recurring struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Frequency string   `json:"frequency"`
	NextRun   string   `json:"next_run"`
	EndDate   string   `json:"end_date"`
	Active    bool     `json:"active"`
	AutoSend  bool     `json:"auto_send"`
	Template  DocInput `json:"template"`
	Generated int      `json:"generated"`
}

func newToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func today() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func parseDate(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// ---- settings ----

const settingsCols = `business_name, address, tax_id, email, phone, logo_url, invoice_prefix, estimate_prefix,
	next_invoice, next_estimate, default_currency, due_days, default_notes, default_terms, tax_note,
	payment_info, units, rates, deduct_stock, reminder_days, sales`

// GetSettings returns the org settings, creating defaults on first use.
func (r *Repository) GetSettings(ctx context.Context, orgID string) (Settings, error) {
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.invc_settings (org_id) VALUES ($1) ON CONFLICT DO NOTHING`, orgID); err != nil {
		return Settings{}, err
	}
	return scanSettings(r.pool.QueryRow(ctx, `SELECT `+settingsCols+` FROM adrient.invc_settings WHERE org_id=$1`, orgID))
}

func scanSettings(row pgx.Row) (Settings, error) {
	var s Settings
	var rates, sales []byte
	err := row.Scan(&s.BusinessName, &s.Address, &s.TaxID, &s.Email, &s.Phone, &s.LogoURL, &s.InvoicePrefix, &s.EstimatePrefix,
		&s.NextInvoice, &s.NextEstimate, &s.DefaultCurrency, &s.DueDays, &s.DefaultNotes, &s.DefaultTerms, &s.TaxNote,
		&s.PaymentInfo, &s.Units, &rates, &s.DeductStock, &s.ReminderDays, &sales)
	if err != nil {
		return s, err
	}
	s.Rates = map[string]float64{}
	_ = json.Unmarshal(rates, &s.Rates)
	s.Sales = parseSalesSettings(sales)
	if s.Units == nil {
		s.Units = []string{}
	}
	return s, nil
}

// SaveSettings replaces the settings.
func (r *Repository) SaveSettings(ctx context.Context, orgID string, s Settings) (Settings, error) {
	if _, err := r.GetSettings(ctx, orgID); err != nil {
		return s, err
	}
	s.DefaultCurrency = strings.ToUpper(strings.TrimSpace(s.DefaultCurrency))
	if s.DefaultCurrency == "" {
		s.DefaultCurrency = "INR"
	}
	if s.NextInvoice < 1 {
		s.NextInvoice = 1
	}
	if s.NextEstimate < 1 {
		s.NextEstimate = 1
	}
	if s.DueDays < 0 {
		s.DueDays = 0
	}
	if s.Units == nil {
		s.Units = []string{}
	}
	rates, _ := json.Marshal(cleanRates(s.Rates))
	sales, err := marshalSalesSettings(s.Sales)
	if err != nil {
		return s, err
	}
	_, err = r.pool.Exec(ctx, `UPDATE adrient.invc_settings SET business_name=$2, address=$3, tax_id=$4, email=$5, phone=$6,
		logo_url=$7, invoice_prefix=$8, estimate_prefix=$9, next_invoice=$10, next_estimate=$11, default_currency=$12,
		due_days=$13, default_notes=$14, default_terms=$15, tax_note=$16, payment_info=$17, units=$18, rates=$19,
		deduct_stock=$20, reminder_days=$21, sales=$22, updated_at=now() WHERE org_id=$1`,
		orgID, s.BusinessName, s.Address, s.TaxID, s.Email, s.Phone, s.LogoURL, s.InvoicePrefix, s.EstimatePrefix,
		s.NextInvoice, s.NextEstimate, s.DefaultCurrency, s.DueDays, s.DefaultNotes, s.DefaultTerms, s.TaxNote,
		s.PaymentInfo, s.Units, rates, s.DeductStock, s.ReminderDays, sales)
	if err != nil {
		return s, err
	}
	return r.GetSettings(ctx, orgID)
}

func cleanRates(in map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for k, v := range in {
		k = strings.ToUpper(strings.TrimSpace(k))
		if k != "" && v > 0 {
			out[k] = v
		}
	}
	return out
}

// ---- price list ----

// ListItems returns the price list.
func (r *Repository) ListItems(ctx context.Context, orgID string) ([]Item, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, description, unit, price::float8, tax_rate::float8
		FROM adrient.invc_items WHERE org_id=$1 ORDER BY lower(name)`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Description, &it.Unit, &it.Price, &it.TaxRate); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// SaveItem creates (id empty) or updates a price list item.
func (r *Repository) SaveItem(ctx context.Context, orgID, id string, it Item) (Item, error) {
	it.Name = strings.TrimSpace(it.Name)
	if it.Name == "" {
		return it, invalid("name is required")
	}
	if it.Price < 0 || it.TaxRate < 0 {
		return it, invalid("price and tax must not be negative")
	}
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.invc_items (org_id,name,description,unit,price,tax_rate)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`, orgID, it.Name, it.Description, it.Unit, it.Price, it.TaxRate).Scan(&it.ID)
	} else {
		tag, e := r.pool.Exec(ctx, `UPDATE adrient.invc_items SET name=$3,description=$4,unit=$5,price=$6,tax_rate=$7
			WHERE org_id=$1 AND id=$2`, orgID, id, it.Name, it.Description, it.Unit, it.Price, it.TaxRate)
		if e == nil && tag.RowsAffected() == 0 {
			e = ErrNotFound
		}
		err = e
		it.ID = id
	}
	return it, err
}

// DeleteItem removes a price list item.
func (r *Repository) DeleteItem(ctx context.Context, orgID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.invc_items WHERE org_id=$1 AND id=$2`, orgID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ---- documents ----

const docCols = `id::text, kind, number, status, COALESCE(contact_id::text,''), client_name, client_email, client_address,
	currency, to_char(issue_date,'YYYY-MM-DD'), COALESCE(to_char(due_date,'YYYY-MM-DD'),''), lines,
	discount_kind, discount_value::float8, subtotal::float8, discount_total::float8, tax_total::float8, total::float8,
	notes, terms, tax_note, schedule, attachments, share_token, COALESCE(recurring_id::text,''),
	COALESCE(converted_from::text,''), stock_deducted, COALESCE(to_char(sent_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
	COALESCE(to_char(viewed_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),''), reminders_sent, deleted_at IS NOT NULL,
	to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')`

func scanDoc(row pgx.Row) (Document, error) {
	var d Document
	var lines, sched, att []byte
	err := row.Scan(&d.ID, &d.Kind, &d.Number, &d.Status, &d.ContactID, &d.ClientName, &d.ClientEmail, &d.ClientAddress,
		&d.Currency, &d.IssueDate, &d.DueDate, &lines, &d.DiscountKind, &d.DiscountValue, &d.Subtotal, &d.DiscountTotal,
		&d.TaxTotal, &d.Total, &d.Notes, &d.Terms, &d.TaxNote, &sched, &att, &d.ShareToken, &d.RecurringID,
		&d.ConvertedFrom, &d.StockDeducted, &d.SentAt, &d.ViewedAt, &d.RemindersSent, &d.Deleted, &d.CreatedAt)
	if err != nil {
		return d, err
	}
	d.Lines, d.Schedule, d.Attachments = []Line{}, []Schedule{}, []Attachment{}
	_ = json.Unmarshal(lines, &d.Lines)
	_ = json.Unmarshal(sched, &d.Schedule)
	_ = json.Unmarshal(att, &d.Attachments)
	d.Payments = []Payment{}
	return d, nil
}

// derive fills Paid/Balance/DisplayStatus from d.Payments.
func (d *Document) derive(now time.Time) {
	if d.Kind == "credit_note" {
		d.deriveCredit()
		return
	}
	s := Stand(d.Total, d.Payments)
	d.Paid, d.WrittenOff, d.Balance, d.Credited = s.Paid, s.WrittenOff, s.Balance, s.Credited
	d.Instalments = Instalments(d.Total, d.Total-s.Balance, d.Schedule)
	d.EffectiveDue = effectiveDue(d.DueDate, d.Instalments)
	d.DisplayStatus = DisplayStatus(d.Kind, d.Status, parseDate(d.EffectiveDue), d.Total, s, now)
	d.OverdueAmount = OverdueAmount(*d, now)
	if d.Kind == "estimate" && d.Status == "sent" && d.ValidUntil != "" && d.ValidUntil < now.Format("2006-01-02") {
		d.DisplayStatus = "expired"
	}
}

// deriveCredit folds a credit note's rows: "applied" rows moved it onto
// invoices, "refund" rows paid it back to the customer.
func (d *Document) deriveCredit() {
	var applied, refunded float64
	for _, p := range d.Payments {
		switch p.Kind {
		case "applied":
			applied += p.Amount
		case "refund":
			refunded += p.Amount
		}
	}
	d.Paid, d.Credited = Round2(refunded), Round2(applied)
	d.CreditRemain = Round2(d.Total - applied - refunded)
	d.Balance = 0
	switch {
	case d.Status == "draft":
		d.DisplayStatus = "draft"
	case d.CreditRemain > 0.004 && applied+refunded <= 0.004:
		d.DisplayStatus = "open"
	case d.CreditRemain > 0.004:
		d.DisplayStatus = "partly_used"
	case refunded > 0.004 && applied <= 0.004:
		d.DisplayStatus = "refunded"
	default:
		d.DisplayStatus = "applied"
	}
}

func (r *Repository) loadPayments(ctx context.Context, orgID string, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	if err := r.loadExt(ctx, orgID, docs); err != nil {
		return err
	}
	ids := make([]string, len(docs))
	idx := map[string]int{}
	for i, d := range docs {
		ids[i] = d.ID
		idx[d.ID] = i
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, document_id::text, kind, amount::float8, method, reference, note,
		to_char(paid_at,'YYYY-MM-DD') FROM adrient.invc_payments WHERE org_id=$1 AND document_id = ANY($2::uuid[])
		ORDER BY paid_at, created_at`, orgID, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.DocumentID, &p.Kind, &p.Amount, &p.Method, &p.Reference, &p.Note, &p.PaidAt); err != nil {
			return err
		}
		i := idx[p.DocumentID]
		docs[i].Payments = append(docs[i].Payments, p)
	}
	return rows.Err()
}

// DocFilter narrows ListDocuments.
type DocFilter struct {
	Kind      string
	Status    string // display status
	Query     string
	ContactID string
	Trash     bool
	// Limit/Offset page the list (0 = everything up to the 2,000 cap, as the
	// reports need). With a display-status filter the page is cut after the
	// status is derived, still bounded by the 2,000-row scan.
	Limit  int
	Offset int
}

// ListDocuments returns documents (newest first) with payments and derived state.
func (r *Repository) ListDocuments(ctx context.Context, orgID string, f DocFilter) ([]Document, error) {
	where := `org_id=$1`
	args := []any{orgID}
	if f.Trash {
		where += ` AND deleted_at IS NOT NULL`
	} else {
		where += ` AND deleted_at IS NULL`
	}
	if f.Kind != "" {
		args = append(args, f.Kind)
		where += fmt.Sprintf(` AND kind=$%d`, len(args))
	}
	if f.ContactID != "" {
		args = append(args, f.ContactID)
		where += fmt.Sprintf(` AND contact_id=$%d::uuid`, len(args))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where += fmt.Sprintf(` AND (lower(number) LIKE $%d OR lower(client_name) LIKE $%d OR lower(notes) LIKE $%d)`, len(args), len(args), len(args))
	}
	limitSQL := ` LIMIT 2000`
	if f.Limit > 0 && f.Status == "" {
		limitSQL = fmt.Sprintf(` LIMIT %d OFFSET %d`, f.Limit, max(f.Offset, 0))
	}
	rows, err := r.pool.Query(ctx, `SELECT `+docCols+` FROM adrient.invc_documents WHERE `+where+
		` ORDER BY issue_date DESC, created_at DESC, id DESC`+limitSQL, args...)
	if err != nil {
		return nil, err
	}
	docs := []Document{}
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		docs = append(docs, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.loadPayments(ctx, orgID, docs); err != nil {
		return nil, err
	}
	now := today()
	out := docs[:0]
	for i := range docs {
		docs[i].derive(now)
		if f.Status != "" && docs[i].DisplayStatus != f.Status {
			continue
		}
		out = append(out, docs[i])
	}
	if f.Limit > 0 && f.Status != "" {
		lo := min(max(f.Offset, 0), len(out))
		out = out[lo:min(lo+f.Limit, len(out))]
	}
	return out, nil
}

// GetDocument returns one document (trashed ones included).
func (r *Repository) GetDocument(ctx context.Context, orgID, id string) (Document, error) {
	d, err := scanDoc(r.pool.QueryRow(ctx, `SELECT `+docCols+` FROM adrient.invc_documents WHERE org_id=$1 AND id=$2::uuid`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	return r.finish(ctx, orgID, d)
}

func (r *Repository) finish(ctx context.Context, orgID string, d Document) (Document, error) {
	ds := []Document{d}
	if err := r.loadPayments(ctx, orgID, ds); err != nil {
		return d, err
	}
	ds[0].derive(today())
	if ds[0].SupplierState != "" {
		b := GST(ds[0].Lines, ds[0].DiscountKind, ds[0].DiscountValue, ds[0].SupplierState, ds[0].PlaceOfSupply)
		ds[0].Gst = &b
	}
	if ds[0].Kind == "estimate" && ds[0].Status == "superseded" && ds[0].VersionGroup != "" {
		_ = r.pool.QueryRow(ctx, `SELECT id::text, share_token FROM adrient.invc_documents WHERE org_id=$1 AND version_group=$2::uuid
			AND deleted_at IS NULL ORDER BY version DESC LIMIT 1`, orgID, ds[0].VersionGroup).Scan(&ds[0].LatestID, &ds[0].LatestToken)
	}
	return ds[0], nil
}

// PublicByToken returns a document for the signed-out share page, with the org
// branding. It records the first view.
func (r *Repository) PublicByToken(ctx context.Context, token string) (Document, Settings, error) {
	var orgID string
	err := r.pool.QueryRow(ctx, `UPDATE adrient.invc_documents SET viewed_at=COALESCE(viewed_at, now())
		WHERE share_token=$1 AND deleted_at IS NULL AND status <> 'draft' RETURNING org_id::text`, token).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, Settings{}, ErrNotFound
	}
	if err != nil {
		return Document{}, Settings{}, err
	}
	d, err := scanDoc(r.pool.QueryRow(ctx, `SELECT `+docCols+` FROM adrient.invc_documents WHERE share_token=$1`, token))
	if err != nil {
		return d, Settings{}, err
	}
	d, err = r.finish(ctx, orgID, d)
	if err != nil {
		return d, Settings{}, err
	}
	s, err := r.GetSettings(ctx, orgID)
	return d, s, err
}

func normalizeInput(in *DocInput, s Settings) error {
	if in.Kind != "estimate" && in.Kind != "credit_note" {
		in.Kind = "invoice"
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = s.DefaultCurrency
	}
	if in.DiscountKind != "amount" {
		in.DiscountKind = "percent"
	}
	if in.DiscountValue < 0 {
		return invalid("discount must not be negative")
	}
	if in.IssueDate == "" {
		in.IssueDate = today().Format("2006-01-02")
	}
	if parseDate(in.IssueDate) == nil {
		return invalid("issue date must be YYYY-MM-DD")
	}
	if in.DueDate != "" && parseDate(in.DueDate) == nil {
		return invalid("due date must be YYYY-MM-DD")
	}
	if in.Lines == nil {
		in.Lines = []Line{}
	}
	for i, l := range in.Lines {
		if l.Qty < 0 || l.Price < 0 || l.TaxRate < 0 {
			return invalid("line %d: quantity, price and tax must not be negative", i+1)
		}
	}
	if in.Schedule == nil {
		in.Schedule = []Schedule{}
	}
	if in.Attachments == nil {
		in.Attachments = []Attachment{}
	}
	if in.ContactID != "" && len(in.ContactID) != 36 {
		return invalid("contact id")
	}
	return validateExtInput(in)
}

// SaveDocument creates (id empty) or updates a document and recomputes totals.
func (r *Repository) SaveDocument(ctx context.Context, orgID, userID, id string, in DocInput) (Document, error) {
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return Document{}, err
	}
	if err := normalizeInput(&in, s); err != nil {
		return Document{}, err
	}
	t := Compute(in.Lines, in.DiscountKind, in.DiscountValue)
	gx := applyGST(&in, &t, s)
	lines, _ := json.Marshal(in.Lines)
	sched, _ := json.Marshal(in.Schedule)
	att, _ := json.Marshal(in.Attachments)
	if id != "" {
		tag, err := r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET contact_id=NULLIF($3,'')::uuid, client_name=$4, client_email=$5,
			client_address=$6, currency=$7, issue_date=$8::date, due_date=NULLIF($9,'')::date, lines=$10, discount_kind=$11,
			discount_value=$12, subtotal=$13, discount_total=$14, tax_total=$15, total=$16, notes=$17, terms=$18, tax_note=$19,
			schedule=$20, attachments=$21, number=CASE WHEN $22 <> '' THEN $22 ELSE number END, updated_at=now()
			WHERE org_id=$1 AND id=$2::uuid AND deleted_at IS NULL`,
			orgID, id, in.ContactID, in.ClientName, in.ClientEmail, in.ClientAddress, in.Currency, in.IssueDate, in.DueDate,
			lines, in.DiscountKind, in.DiscountValue, t.Subtotal, t.DiscountTotal, t.TaxTotal, t.Total, in.Notes, in.Terms,
			in.TaxNote, sched, att, strings.TrimSpace(in.Number))
		if err != nil {
			return Document{}, err
		}
		if tag.RowsAffected() == 0 {
			return Document{}, ErrNotFound
		}
		if err := r.writeExt(ctx, r.pool, orgID, id, in, gx, false); err != nil {
			return Document{}, err
		}
		return r.GetDocument(ctx, orgID, id)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Document{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	number := strings.TrimSpace(in.Number)
	if number == "" {
		number, err = nextNumber(ctx, tx, orgID, in.Kind, s)
		if err != nil {
			return Document{}, err
		}
	}
	if in.DueDate == "" && in.Kind == "invoice" {
		in.DueDate = parseDate(in.IssueDate).AddDate(0, 0, s.DueDays).Format("2006-01-02")
	}
	var nid string
	err = tx.QueryRow(ctx, `INSERT INTO adrient.invc_documents (org_id, kind, number, contact_id, client_name, client_email,
		client_address, currency, issue_date, due_date, lines, discount_kind, discount_value, subtotal, discount_total,
		tax_total, total, notes, terms, tax_note, schedule, attachments, share_token, created_by)
		VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9::date,NULLIF($10,'')::date,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,NULLIF($24,'')::uuid)
		RETURNING id::text`,
		orgID, in.Kind, number, in.ContactID, in.ClientName, in.ClientEmail, in.ClientAddress, in.Currency, in.IssueDate,
		in.DueDate, lines, in.DiscountKind, in.DiscountValue, t.Subtotal, t.DiscountTotal, t.TaxTotal, t.Total, in.Notes,
		in.Terms, in.TaxNote, sched, att, newToken(), userID).Scan(&nid)
	if err != nil {
		return Document{}, err
	}
	if err := r.writeExt(ctx, tx, orgID, nid, in, gx, true); err != nil {
		return Document{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Document{}, err
	}
	return r.GetDocument(ctx, orgID, nid)
}

func nextNumber(ctx context.Context, tx pgx.Tx, orgID, kind string, s Settings) (string, error) {
	var prefix string
	var n int
	var err error
	if kind == "credit_note" {
		n, err = nextSeq(ctx, tx, orgID, "credit")
		return fmt.Sprintf("%s%04d", s.Sales.CreditPrefix, n), err
	}
	if kind == "estimate" {
		err = tx.QueryRow(ctx, `UPDATE adrient.invc_settings SET next_estimate=next_estimate+1 WHERE org_id=$1
			RETURNING estimate_prefix, next_estimate-1`, orgID).Scan(&prefix, &n)
	} else {
		err = tx.QueryRow(ctx, `UPDATE adrient.invc_settings SET next_invoice=next_invoice+1 WHERE org_id=$1
			RETURNING invoice_prefix, next_invoice-1`, orgID).Scan(&prefix, &n)
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%04d", prefix, n), nil
}

// Trash moves a document to the trash (never voids it).
func (r *Repository) Trash(ctx context.Context, orgID, id string) error {
	return r.exec(ctx, `UPDATE adrient.invc_documents SET deleted_at=now() WHERE org_id=$1 AND id=$2::uuid AND deleted_at IS NULL`, orgID, id)
}

// Restore takes a document out of the trash.
func (r *Repository) Restore(ctx context.Context, orgID, id string) error {
	return r.exec(ctx, `UPDATE adrient.invc_documents SET deleted_at=NULL WHERE org_id=$1 AND id=$2::uuid AND deleted_at IS NOT NULL`, orgID, id)
}

// Purge permanently deletes a document that is already in the trash.
func (r *Repository) Purge(ctx context.Context, orgID, id string) error {
	return r.exec(ctx, `DELETE FROM adrient.invc_documents WHERE org_id=$1 AND id=$2::uuid AND deleted_at IS NOT NULL`, orgID, id)
}

func (r *Repository) exec(ctx context.Context, q string, args ...any) error {
	tag, err := r.pool.Exec(ctx, q, args...)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// MarkSent flags a draft as sent. first is true when this call changed it.
func (r *Repository) MarkSent(ctx context.Context, orgID, id string) (doc Document, first bool, err error) {
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET status='sent', sent_at=COALESCE(sent_at, now()), updated_at=now()
		WHERE org_id=$1 AND id=$2::uuid AND deleted_at IS NULL AND status='draft'`, orgID, id)
	if err != nil {
		return doc, false, err
	}
	first = tag.RowsAffected() > 0
	doc, err = r.GetDocument(ctx, orgID, id)
	return doc, first, err
}

// SetStockDeducted records that stock was moved for an invoice.
func (r *Repository) SetStockDeducted(ctx context.Context, orgID, id string) error {
	return r.exec(ctx, `UPDATE adrient.invc_documents SET stock_deducted=true WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
}

// NoteReminder records a payment reminder.
func (r *Repository) NoteReminder(ctx context.Context, orgID, id string) error {
	return r.exec(ctx, `UPDATE adrient.invc_documents SET reminders_sent=reminders_sent+1, last_reminder_at=now() WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
}

// SetEstimateStatus moves an estimate between sent, accepted and declined.
func (r *Repository) SetEstimateStatus(ctx context.Context, orgID, id, status string) (Document, error) {
	switch status {
	case "draft", "sent", "accepted", "declined":
	default:
		return Document{}, invalid("status must be draft, sent, accepted or declined")
	}
	if err := r.exec(ctx, `UPDATE adrient.invc_documents SET status=$3, updated_at=now()
		WHERE org_id=$1 AND id=$2::uuid AND kind='estimate' AND deleted_at IS NULL`, orgID, id, status); err != nil {
		return Document{}, err
	}
	return r.GetDocument(ctx, orgID, id)
}

func inputOf(d Document) DocInput {
	return DocInput{Kind: d.Kind, ContactID: d.ContactID, ClientName: d.ClientName, ClientEmail: d.ClientEmail,
		ClientAddress: d.ClientAddress, Currency: d.Currency, IssueDate: d.IssueDate, DueDate: d.DueDate, Lines: d.Lines,
		DiscountKind: d.DiscountKind, DiscountValue: d.DiscountValue, Notes: d.Notes, Terms: d.Terms, TaxNote: d.TaxNote,
		Schedule: d.Schedule, Attachments: d.Attachments, PlaceOfSupply: d.PlaceOfSupply, ClientGSTIN: d.ClientGSTIN,
		PriceListID: d.PriceListID, SourceRef: d.SourceRef, ValidUntil: d.ValidUntil}
}

// Duplicate copies a document as a new draft dated today.
func (r *Repository) Duplicate(ctx context.Context, orgID, userID, id string) (Document, error) {
	d, err := r.GetDocument(ctx, orgID, id)
	if err != nil {
		return d, err
	}
	in := inputOf(d)
	in.IssueDate, in.DueDate, in.Schedule, in.ValidUntil, in.SourceRef = "", "", nil, "", ""
	return r.SaveDocument(ctx, orgID, userID, "", in)
}

// ConvertEstimate creates an invoice from an estimate and marks it converted.
func (r *Repository) ConvertEstimate(ctx context.Context, orgID, userID, id string) (Document, error) {
	d, err := r.GetDocument(ctx, orgID, id)
	if err != nil {
		return d, err
	}
	if d.Kind != "estimate" {
		return d, invalid("only an estimate can be converted")
	}
	if d.Status == "converted" {
		return d, invalid("estimate was already converted")
	}
	in := inputOf(d)
	in.Kind, in.IssueDate, in.DueDate = "invoice", "", ""
	inv, err := r.SaveDocument(ctx, orgID, userID, "", in)
	if err != nil {
		return inv, err
	}
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET converted_from=$3::uuid WHERE org_id=$1 AND id=$2::uuid`, orgID, inv.ID, id)
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET status='converted' WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	return r.GetDocument(ctx, orgID, inv.ID)
}

// ---- payments ----

// AddPayment records a payment, refund or write-off. A draft invoice becomes sent.
func (r *Repository) AddPayment(ctx context.Context, orgID, userID, docID string, p Payment) (Document, error) {
	d, err := r.GetDocument(ctx, orgID, docID)
	if err != nil {
		return d, err
	}
	if d.Kind != "invoice" || d.Deleted {
		return d, invalid("payments can only be recorded on a live invoice")
	}
	switch p.Kind {
	case "payment", "refund", "writeoff":
	default:
		p.Kind = "payment"
	}
	p.Amount = Round2(p.Amount)
	if p.Amount <= 0 {
		return d, invalid("amount must be greater than zero")
	}
	switch p.Kind {
	case "refund":
		if p.Amount > d.Paid+0.004 {
			return d, invalid("refund exceeds the amount paid (%.2f)", d.Paid)
		}
	default:
		if p.Amount > d.Balance+0.004 {
			return d, invalid("amount exceeds the balance (%.2f)", d.Balance)
		}
	}
	if p.PaidAt == "" {
		p.PaidAt = today().Format("2006-01-02")
	}
	if parseDate(p.PaidAt) == nil {
		return d, invalid("date must be YYYY-MM-DD")
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.invc_payments (org_id, document_id, kind, amount, method, reference, note, paid_at, created_by)
		VALUES ($1,$2::uuid,$3,$4,$5,$6,$7,$8::date,NULLIF($9,'')::uuid)`,
		orgID, docID, p.Kind, p.Amount, p.Method, p.Reference, p.Note, p.PaidAt, userID)
	if err != nil {
		return d, err
	}
	if d.Status == "draft" {
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET status='sent', sent_at=COALESCE(sent_at,now()) WHERE id=$1::uuid`, docID)
	}
	return r.GetDocument(ctx, orgID, docID)
}

// DeletePayment removes a payment row.
func (r *Repository) DeletePayment(ctx context.Context, orgID, docID, payID string) (Document, error) {
	if err := r.exec(ctx, `DELETE FROM adrient.invc_payments WHERE org_id=$1 AND document_id=$2::uuid AND id=$3::uuid AND kind NOT IN ('credit','applied')`, orgID, docID, payID); err != nil {
		return Document{}, err
	}
	return r.GetDocument(ctx, orgID, docID)
}

// ListPayments is the read API for Accounting: all payment rows in a date range.
func (r *Repository) ListPayments(ctx context.Context, orgID, from, to string) ([]Payment, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.id::text, p.document_id::text, p.kind, p.amount::float8, p.method, p.reference, p.note,
		to_char(p.paid_at,'YYYY-MM-DD'), d.number, d.client_name, d.currency
		FROM adrient.invc_payments p JOIN adrient.invc_documents d ON d.id=p.document_id
		WHERE p.org_id=$1 AND d.deleted_at IS NULL AND p.kind NOT IN ('credit','applied') AND ($2='' OR p.paid_at >= $2::date) AND ($3='' OR p.paid_at <= $3::date)
		ORDER BY p.paid_at DESC, p.created_at DESC LIMIT 5000`, orgID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Payment{}
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.DocumentID, &p.Kind, &p.Amount, &p.Method, &p.Reference, &p.Note, &p.PaidAt,
			&p.DocNumber, &p.ClientName, &p.Currency); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- recurring ----

func scanRecurring(row pgx.Row) (Recurring, error) {
	var rc Recurring
	var tpl []byte
	err := row.Scan(&rc.ID, &rc.Name, &rc.Frequency, &rc.NextRun, &rc.EndDate, &rc.Active, &rc.AutoSend, &tpl, &rc.Generated)
	if err != nil {
		return rc, err
	}
	_ = json.Unmarshal(tpl, &rc.Template)
	return rc, nil
}

const recCols = `id::text, name, frequency, to_char(next_run,'YYYY-MM-DD'), COALESCE(to_char(end_date,'YYYY-MM-DD'),''), active, auto_send, template, generated`

// ListRecurring returns recurring templates.
func (r *Repository) ListRecurring(ctx context.Context, orgID string) ([]Recurring, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+recCols+` FROM adrient.invc_recurring WHERE org_id=$1 ORDER BY next_run`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recurring{}
	for rows.Next() {
		rc, err := scanRecurring(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

// SaveRecurring creates (id empty) or updates a template.
func (r *Repository) SaveRecurring(ctx context.Context, orgID, id string, rc Recurring) (Recurring, error) {
	switch rc.Frequency {
	case "weekly", "monthly", "quarterly", "yearly":
	default:
		return rc, invalid("frequency must be weekly, monthly, quarterly or yearly")
	}
	if rc.NextRun == "" {
		rc.NextRun = today().Format("2006-01-02")
	}
	if parseDate(rc.NextRun) == nil || (rc.EndDate != "" && parseDate(rc.EndDate) == nil) {
		return rc, invalid("dates must be YYYY-MM-DD")
	}
	rc.Template.Kind = "invoice"
	rc.Template.Number, rc.Template.IssueDate, rc.Template.DueDate = "", "", ""
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return rc, err
	}
	if err := normalizeInput(&rc.Template, s); err != nil {
		return rc, err
	}
	tpl, _ := json.Marshal(rc.Template)
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.invc_recurring (org_id,name,frequency,next_run,end_date,active,auto_send,template)
			VALUES ($1,$2,$3,$4::date,NULLIF($5,'')::date,$6,$7,$8) RETURNING `+recCols,
			orgID, rc.Name, rc.Frequency, rc.NextRun, rc.EndDate, rc.Active, rc.AutoSend, tpl).
			Scan(&rc.ID, &rc.Name, &rc.Frequency, &rc.NextRun, &rc.EndDate, &rc.Active, &rc.AutoSend, &tpl, &rc.Generated)
		_ = json.Unmarshal(tpl, &rc.Template)
		return rc, err
	}
	out, err := scanRecurring(r.pool.QueryRow(ctx, `UPDATE adrient.invc_recurring SET name=$3,frequency=$4,next_run=$5::date,
		end_date=NULLIF($6,'')::date,active=$7,auto_send=$8,template=$9 WHERE org_id=$1 AND id=$2::uuid RETURNING `+recCols,
		orgID, id, rc.Name, rc.Frequency, rc.NextRun, rc.EndDate, rc.Active, rc.AutoSend, tpl))
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	return out, err
}

// DeleteRecurring removes a template (generated invoices stay).
func (r *Repository) DeleteRecurring(ctx context.Context, orgID, id string) error {
	return r.exec(ctx, `DELETE FROM adrient.invc_recurring WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
}

// RunRecurring generates every invoice that is due (catching up missed periods,
// at most 12 per template per run). Generated invoices are drafts, or sent when
// the template has auto_send. It returns the new documents.
func (r *Repository) RunRecurring(ctx context.Context, orgID, userID string) ([]Document, []Recurring, error) {
	list, err := r.ListRecurring(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	now := today()
	var made []Document
	var autoSend []Recurring
	for _, rc := range list {
		if !rc.Active {
			continue
		}
		next := parseDate(rc.NextRun)
		end := parseDate(rc.EndDate)
		n := 0
		for next != nil && !next.After(now) && n < 12 {
			if end != nil && next.After(*end) {
				break
			}
			in := rc.Template
			in.Kind = "invoice"
			in.IssueDate = next.Format("2006-01-02")
			in.DueDate = next.AddDate(0, 0, s.DueDays).Format("2006-01-02")
			in.RecurringID, in.RecurringDate = rc.ID, in.IssueDate
			var exists bool
			if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM adrient.invc_documents WHERE recurring_id=$1::uuid AND recurring_period=$2::date)`,
				rc.ID, in.IssueDate).Scan(&exists); err != nil {
				return made, autoSend, err
			}
			if !exists { // a period is billed once, however often the runner fires
				d, err := r.SaveDocument(ctx, orgID, userID, "", in)
				if err != nil {
					return made, autoSend, err
				}
				if rc.AutoSend {
					d, _, _ = r.MarkSent(ctx, orgID, d.ID)
				}
				made = append(made, d)
			}
			nx := NextDate(*next, rc.Frequency)
			next = &nx
			n++
		}
		if n > 0 {
			_, err := r.pool.Exec(ctx, `UPDATE adrient.invc_recurring SET next_run=$3::date, generated=generated+$4,
				active = CASE WHEN end_date IS NOT NULL AND $3::date > end_date THEN false ELSE active END
				WHERE org_id=$1 AND id=$2::uuid`, orgID, rc.ID, next.Format("2006-01-02"), n)
			if err != nil {
				return made, autoSend, err
			}
			if rc.AutoSend {
				autoSend = append(autoSend, rc)
			}
		}
	}
	return made, autoSend, nil
}

// ---- clients and reports ----

// ClientRow aggregates invoices per client.
type ClientRow struct {
	ContactID   string  `json:"contact_id"`
	ClientName  string  `json:"client_name"`
	ClientEmail string  `json:"client_email"`
	Invoices    int     `json:"invoices"`
	Billed      float64 `json:"billed"`      // in the base currency
	Paid        float64 `json:"paid"`        // in the base currency
	Outstanding float64 `json:"outstanding"` // in the base currency
}

// Clients groups live invoices by contact (or by name when no contact is linked).
func (r *Repository) Clients(ctx context.Context, orgID string) ([]ClientRow, error) {
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	docs, err := r.ListDocuments(ctx, orgID, DocFilter{Kind: "invoice"})
	if err != nil {
		return nil, err
	}
	idx := map[string]*ClientRow{}
	var order []string
	for _, d := range docs {
		if d.Status == "draft" {
			continue
		}
		key := d.ContactID
		if key == "" {
			key = "name:" + strings.ToLower(d.ClientName)
		}
		c := idx[key]
		if c == nil {
			c = &ClientRow{ContactID: d.ContactID, ClientName: d.ClientName, ClientEmail: d.ClientEmail}
			idx[key] = c
			order = append(order, key)
		}
		c.Invoices++
		c.Billed += ToBase(d.Total, d.Currency, s.DefaultCurrency, s.Rates)
		c.Paid += ToBase(d.Paid, d.Currency, s.DefaultCurrency, s.Rates)
		c.Outstanding += ToBase(d.Balance, d.Currency, s.DefaultCurrency, s.Rates)
	}
	out := make([]ClientRow, 0, len(order))
	for _, k := range order {
		c := idx[k]
		c.Billed, c.Paid, c.Outstanding = Round2(c.Billed), Round2(c.Paid), Round2(c.Outstanding)
		out = append(out, *c)
	}
	return out, nil
}

// ReportRow is a labelled amount.
type ReportRow struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

// Report is the summary for a date range, all amounts in the base currency.
type Report struct {
	Currency    string      `json:"currency"`
	Invoiced    float64     `json:"invoiced"`
	Collected   float64     `json:"collected"`
	Outstanding float64     `json:"outstanding"`
	Overdue     float64     `json:"overdue"`
	WrittenOff  float64     `json:"written_off"`
	ByMonth     []ReportRow `json:"by_month"`
	ByClient    []ReportRow `json:"by_client"`
	ByStatus    []ReportRow `json:"by_status"`
	Aging       []ReportRow `json:"aging"`
	ByCurrency  []ReportRow `json:"by_currency"` // native totals invoiced per currency
}

// BuildReport computes the report (from/to are inclusive YYYY-MM-DD, either may be empty).
func (r *Repository) BuildReport(ctx context.Context, orgID, from, to string) (Report, error) {
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return Report{}, err
	}
	docs, err := r.ListDocuments(ctx, orgID, DocFilter{Kind: "invoice"})
	if err != nil {
		return Report{}, err
	}
	return buildReport(docs, s, from, to, today()), nil
}

func buildReport(docs []Document, s Settings, from, to string, now time.Time) Report {
	rep := Report{Currency: s.DefaultCurrency, ByMonth: []ReportRow{}, ByClient: []ReportRow{}, ByStatus: []ReportRow{}, ByCurrency: []ReportRow{}}
	conv := func(a float64, cur string) float64 { return ToBase(a, cur, s.DefaultCurrency, s.Rates) }
	month, client, status, curr := map[string]*ReportRow{}, map[string]*ReportRow{}, map[string]*ReportRow{}, map[string]*ReportRow{}
	add := func(m map[string]*ReportRow, k string, amt float64) {
		row := m[k]
		if row == nil {
			row = &ReportRow{Label: k}
			m[k] = row
		}
		row.Amount += amt
		row.Count++
	}
	aging := []ReportRow{{Label: "Not yet due"}, {Label: "1-30 days"}, {Label: "31-60 days"}, {Label: "61+ days"}}
	for _, d := range docs {
		if d.Status == "draft" {
			continue
		}
		for _, p := range d.Payments {
			if (from != "" && p.PaidAt < from) || (to != "" && p.PaidAt > to) {
				continue
			}
			switch p.Kind {
			case "refund":
				rep.Collected -= conv(p.Amount, d.Currency)
			case "writeoff":
				rep.WrittenOff += conv(p.Amount, d.Currency)
			case "credit", "applied":
			default:
				rep.Collected += conv(p.Amount, d.Currency)
			}
		}
		if d.Balance > 0.004 { // open balance is always reported as of today
			rep.Outstanding += conv(d.Balance, d.Currency)
			rep.Overdue += conv(d.OverdueAmount, d.Currency)
			worst := 0
			for _, part := range AgingParts(d, now) { // a payment plan ages each open instalment by its own date
				i := 0
				switch {
				case part.Days > 60:
					i = 3
				case part.Days > 30:
					i = 2
				case part.Days > 0:
					i = 1
				}
				aging[i].Amount += conv(part.Amount, d.Currency)
				worst = max(worst, i)
			}
			aging[worst].Count++
		}
		if (from != "" && d.IssueDate < from) || (to != "" && d.IssueDate > to) {
			continue
		}
		t := conv(d.Total, d.Currency)
		rep.Invoiced += t
		add(month, d.IssueDate[:7], t)
		name := d.ClientName
		if name == "" {
			name = "(no client)"
		}
		add(client, name, t)
		add(status, d.DisplayStatus, t)
		add(curr, d.Currency, d.Total)
	}
	flat := func(m map[string]*ReportRow, sortByKey bool) []ReportRow {
		out := make([]ReportRow, 0, len(m))
		for _, v := range m {
			v.Amount = Round2(v.Amount)
			out = append(out, *v)
		}
		sortRows(out, sortByKey)
		return out
	}
	rep.ByMonth, rep.ByClient, rep.ByStatus, rep.ByCurrency = flat(month, true), flat(client, false), flat(status, false), flat(curr, false)
	if len(rep.ByClient) > 10 {
		rep.ByClient = rep.ByClient[:10]
	}
	for i := range aging {
		aging[i].Amount = Round2(aging[i].Amount)
	}
	rep.Aging = aging
	rep.Invoiced, rep.Collected, rep.Outstanding = Round2(rep.Invoiced), Round2(rep.Collected), Round2(rep.Outstanding)
	rep.Overdue, rep.WrittenOff = Round2(rep.Overdue), Round2(rep.WrittenOff)
	return rep
}

func sortRows(rows []ReportRow, byLabel bool) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0; j-- {
			a, b := rows[j-1], rows[j]
			less := b.Amount > a.Amount
			if byLabel {
				less = b.Label < a.Label
			}
			if !less {
				break
			}
			rows[j-1], rows[j] = b, a
		}
	}
}
