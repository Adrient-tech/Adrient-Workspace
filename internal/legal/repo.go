package legal

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Deps are the other modules' capabilities; each is optional and a missing one reports "not available".
type Deps struct {
	Audit knowkit.Audit
	// Calendar creates or moves an all-day event; id "" creates one. DeleteEvent removes it.
	Calendar    func(ctx context.Context, org, owner, id, title, desc, day string) (string, error)
	DeleteEvent func(ctx context.Context, org, id string) error
	// Task creates or updates a task for the assignee; id "" creates one. DeleteTask removes it.
	Task       func(ctx context.Context, org, assignee, id, title, notes, due string, done bool) (string, error)
	DeleteTask func(ctx context.Context, org, id string) error
	// FileMeta checks that the user may attach a Drive file and returns its name.
	FileMeta func(ctx context.Context, org, user, fileID string) (string, error)
	// Invoices creates draft invoices in Invoicing; Contacts reads Contacts.
	Invoices opshttp.InvoiceWriter
	Contacts opshttp.ContactDirectory
	// TimeEntries lists the caller's Time tracking entries between two dates.
	TimeEntries func(ctx context.Context, org, user string, admin bool, from, to string) ([]TimeEntry, error)
	Now         func() time.Time
}

// TimeEntry is an entry read from Time tracking.
type TimeEntry struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Date     string `json:"date"`
	Seconds  int    `json:"seconds"`
	Note     string `json:"note"`
	Project  string `json:"project"`
	Billable bool   `json:"billable"`
}

// Repository is the Postgres store. Every query is org-scoped.
type Repository struct {
	pool *pgxpool.Pool
	Deps Deps
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) now() time.Time { return knowkit.Clock(r.Deps.Now) }
func (r *Repository) today() string  { return r.now().Format("2006-01-02") }
func (r *Repository) audit(ctx context.Context, action, typ, id string) {
	if r.Deps.Audit != nil {
		r.Deps.Audit(ctx, action, typ, id)
	}
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return knowkit.ErrNotFound
	}
	return err
}

func optUUID(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if !knowkit.IsUUID(s) {
		return nil, knowkit.Invalid("%q is not a valid id", s)
	}
	return s, nil
}

func optDate(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if !ValidDate(s) {
		return nil, knowkit.Invalid("%q is not a date (use YYYY-MM-DD)", s)
	}
	return s, nil
}

func tooLong(s string, n int) bool { return utf8.RuneCountInString(s) > n }

// ---- settings ---------------------------------------------------------------------------------------------

// Settings are the firm's billing defaults.
type Settings struct {
	NumberFormat  string   `json:"number_format"`
	IncrementMin  int      `json:"increment_min"`
	DefaultRate   int64    `json:"default_rate"`
	Currency      string   `json:"currency"`
	TermsDays     int      `json:"terms_days"`
	TaxRateBp     int      `json:"tax_rate_bp"`
	TaxName       string   `json:"tax_name"`
	PracticeAreas []string `json:"practice_areas"`
}

// GetSettings returns the settings, creating the defaults on first use.
func (r *Repository) GetSettings(ctx context.Context, org string) (Settings, error) {
	var s Settings
	_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.legal_settings (org_id) VALUES ($1) ON CONFLICT DO NOTHING`, org)
	err := r.pool.QueryRow(ctx, `SELECT number_format, increment_min, default_rate, currency, terms_days, tax_rate_bp, tax_name, practice_areas
		FROM adrient.legal_settings WHERE org_id=$1`, org).Scan(&s.NumberFormat, &s.IncrementMin, &s.DefaultRate, &s.Currency, &s.TermsDays, &s.TaxRateBp, &s.TaxName, &s.PracticeAreas)
	return s, err
}

// SaveSettings updates the settings (admins only).
func (r *Repository) SaveSettings(ctx context.Context, a knowkit.Actor, in Settings) (Settings, error) {
	if !a.Admin {
		return in, knowkit.ErrForbidden
	}
	if !knowkit.OneOf(in.NumberFormat, "number", "number-client", "year-number") {
		return in, knowkit.Invalid("number format must be number, number-client or year-number")
	}
	if in.IncrementMin < 1 || in.IncrementMin > 60 {
		return in, knowkit.Invalid("the billing increment must be 1 to 60 minutes")
	}
	if in.DefaultRate < 0 || in.DefaultRate > 1e12 || in.TaxRateBp < 0 || in.TaxRateBp > 10000 || in.TermsDays < 0 || in.TermsDays > 365 {
		return in, knowkit.Invalid("rate, tax or payment terms are out of range")
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if len(in.Currency) != 3 {
		in.Currency = "INR"
	}
	areas := []string{}
	for _, p := range in.PracticeAreas {
		if p = strings.TrimSpace(p); p != "" && !tooLong(p, 60) {
			areas = append(areas, p)
		}
	}
	if len(areas) == 0 {
		return in, knowkit.Invalid("keep at least one practice area")
	}
	if _, err := r.GetSettings(ctx, a.Org); err != nil {
		return in, err
	}
	in.TaxName = strings.TrimSpace(in.TaxName)
	if in.TaxName == "" {
		in.TaxName = "Tax"
	}
	_, err := r.pool.Exec(ctx, `UPDATE adrient.legal_settings SET number_format=$2, increment_min=$3, default_rate=$4, currency=$5, terms_days=$6, tax_rate_bp=$7, tax_name=$8, practice_areas=$9 WHERE org_id=$1`,
		a.Org, in.NumberFormat, in.IncrementMin, in.DefaultRate, in.Currency, in.TermsDays, in.TaxRateBp, in.TaxName, areas)
	if err != nil {
		return in, err
	}
	in.PracticeAreas = areas
	r.audit(ctx, "settings_update", "legal_settings", a.Org)
	return in, nil
}

// ---- clients ----------------------------------------------------------------------------------------------

// Client is a person or company the firm acts for.
type Client struct {
	ID        string `json:"id"`
	ContactID string `json:"contact_id,omitempty"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	Rate      int64  `json:"rate"`
	Notes     string `json:"notes"`
	Archived  bool   `json:"archived"`
	Matters   int    `json:"matters"`
	Open      int    `json:"open_matters"`
}

// ClientInput creates or edits a client.
type ClientInput struct {
	ContactID string `json:"contact_id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	Rate      int64  `json:"rate"`
	Notes     string `json:"notes"`
	Archived  bool   `json:"archived"`
}

func (in *ClientInput) clean() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || tooLong(in.Name, 200) {
		return knowkit.Invalid("a client needs a name of up to 200 characters")
	}
	if in.Kind == "" {
		in.Kind = "person"
	}
	if !knowkit.OneOf(in.Kind, "person", "company") {
		return knowkit.Invalid("kind must be person or company")
	}
	if in.Rate < 0 || in.Rate > 1e12 {
		return knowkit.Invalid("the rate is out of range")
	}
	in.Email, in.Phone, in.Address = strings.TrimSpace(in.Email), strings.TrimSpace(in.Phone), strings.TrimSpace(in.Address)
	if tooLong(in.Email, 200) || tooLong(in.Phone, 60) || tooLong(in.Address, 500) || tooLong(in.Notes, 5000) {
		return knowkit.Invalid("a field is too long")
	}
	return nil
}

const clientCols = `c.id::text, COALESCE(c.contact_id::text,''), c.kind, c.name, c.email, c.phone, c.address, c.rate, c.notes, c.archived,
	(SELECT count(*) FROM adrient.legal_matters m WHERE m.client_id=c.id AND m.deleted_at IS NULL),
	(SELECT count(*) FROM adrient.legal_matters m WHERE m.client_id=c.id AND m.deleted_at IS NULL AND m.status IN ('open','pending','on_hold'))`

func scanClient(row pgx.Row) (Client, error) {
	var c Client
	err := row.Scan(&c.ID, &c.ContactID, &c.Kind, &c.Name, &c.Email, &c.Phone, &c.Address, &c.Rate, &c.Notes, &c.Archived, &c.Matters, &c.Open)
	return c, notFound(err)
}

// Clients lists clients matching q.
func (r *Repository) Clients(ctx context.Context, a knowkit.Actor, q string, archived bool) ([]Client, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+clientCols+` FROM adrient.legal_clients c WHERE c.org_id=$1 AND ($3 OR NOT c.archived)
		AND ($2='' OR c.name ILIKE '%'||$2||'%' OR c.email ILIKE '%'||$2||'%') ORDER BY lower(c.name) LIMIT 2000`, a.Org, q, archived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Client{}
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetClient returns one client.
func (r *Repository) GetClient(ctx context.Context, a knowkit.Actor, id string) (Client, error) {
	if !knowkit.IsUUID(id) {
		return Client{}, knowkit.ErrNotFound
	}
	return scanClient(r.pool.QueryRow(ctx, `SELECT `+clientCols+` FROM adrient.legal_clients c WHERE c.org_id=$1 AND c.id=$2`, a.Org, id))
}

// CreateClient adds a client.
func (r *Repository) CreateClient(ctx context.Context, a knowkit.Actor, in ClientInput) (Client, error) {
	if err := in.clean(); err != nil {
		return Client{}, err
	}
	cid, err := optUUID(in.ContactID)
	if err != nil {
		return Client{}, err
	}
	var id string
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.legal_clients (org_id, contact_id, kind, name, email, phone, address, rate, notes, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id::text`, a.Org, cid, in.Kind, in.Name, in.Email, in.Phone, in.Address, in.Rate, in.Notes, a.User).Scan(&id)
	if err != nil {
		return Client{}, err
	}
	r.audit(ctx, "client_create", "legal_client", id)
	return r.GetClient(ctx, a, id)
}

// UpdateClient edits a client.
func (r *Repository) UpdateClient(ctx context.Context, a knowkit.Actor, id string, in ClientInput) (Client, error) {
	if err := in.clean(); err != nil {
		return Client{}, err
	}
	if _, err := r.GetClient(ctx, a, id); err != nil {
		return Client{}, err
	}
	cid, err := optUUID(in.ContactID)
	if err != nil {
		return Client{}, err
	}
	if _, err = r.pool.Exec(ctx, `UPDATE adrient.legal_clients SET contact_id=$3, kind=$4, name=$5, email=$6, phone=$7, address=$8, rate=$9, notes=$10, archived=$11, updated_at=now()
		WHERE org_id=$1 AND id=$2`, a.Org, id, cid, in.Kind, in.Name, in.Email, in.Phone, in.Address, in.Rate, in.Notes, in.Archived); err != nil {
		return Client{}, err
	}
	r.audit(ctx, "client_update", "legal_client", id)
	return r.GetClient(ctx, a, id)
}

// DeleteClient removes a client that has no matters (admins only); otherwise archive it.
func (r *Repository) DeleteClient(ctx context.Context, a knowkit.Actor, id string) error {
	if !a.Admin {
		return knowkit.ErrForbidden
	}
	c, err := r.GetClient(ctx, a, id)
	if err != nil {
		return err
	}
	if c.Matters > 0 {
		return knowkit.Invalid("this client has matters; archive the client instead")
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM adrient.legal_clients WHERE org_id=$1 AND id=$2`, a.Org, id); err != nil {
		return err
	}
	r.audit(ctx, "client_delete", "legal_client", id)
	return nil
}

// ClientFromContact makes (or returns) a client for a Contacts entry.
func (r *Repository) ClientFromContact(ctx context.Context, a knowkit.Actor, contactID string) (Client, error) {
	if r.Deps.Contacts == nil {
		return Client{}, knowkit.ErrUnavailable
	}
	if !knowkit.IsUUID(contactID) {
		return Client{}, knowkit.ErrNotFound
	}
	var id string
	if err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.legal_clients WHERE org_id=$1 AND contact_id=$2 LIMIT 1`, a.Org, contactID).Scan(&id); err == nil {
		return r.GetClient(ctx, a, id)
	}
	c, err := r.Deps.Contacts.Get(ctx, a.Org, contactID)
	if err != nil {
		return Client{}, knowkit.ErrNotFound
	}
	kind := "person"
	if c.Company != "" && c.Name == c.Company {
		kind = "company"
	}
	return r.CreateClient(ctx, a, ClientInput{ContactID: contactID, Kind: kind, Name: c.Name, Email: c.Email, Phone: c.Phone})
}
