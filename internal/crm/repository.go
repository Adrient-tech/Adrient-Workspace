// Package crm is the native CRM: leads (Contacts + CRM profile), companies,
// deals pipeline with stage history and buying committee, follow-ups and an
// activity log.
package crm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

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

// Repository is the Postgres store for the CRM. Every query is org-scoped.
type Repository struct {
	pool  *pgxpool.Pool
	hooks *Hooks
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// SetHooks wires the optional integrations (mail, chat, calendar) used by automation.
func (r *Repository) SetHooks(h *Hooks) { r.hooks = h }

// SetOnDealWon registers the won-deal hand-off without replacing other hooks.
func (r *Repository) SetOnDealWon(fn func(ctx context.Context, org, dealID string)) {
	if r.hooks == nil {
		r.hooks = &Hooks{}
	}
	r.hooks.OnDealWon = fn
}

// Pool exposes the connection pool to sibling packages (tests, glue).
func (r *Repository) Pool() *pgxpool.Pool { return r.pool }

// Stage is one open funnel stage.
type Stage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Settings are per-org funnel and goal settings.
type Settings struct {
	Stages       []Stage        `json:"stages"`
	Currency     string         `json:"currency"`
	MonthlyGoal  float64        `json:"monthly_goal"`
	StaleDays    int            `json:"stale_days"`
	ColdDays     int            `json:"cold_days"`
	WeeklyTarget int            `json:"weekly_target"`
	SLALeadHours int            `json:"sla_lead_hours"`
	SLAStageDays map[string]int `json:"sla_stage_days"`
	DefaultScope string         `json:"default_scope"`
}

// Terminal stage ids, always present.
const (
	StageWon  = "won"
	StageLost = "lost"
)

// ValidateStages checks custom open stages (1..8, unique non-empty ids, no terminals).
func ValidateStages(st []Stage) error {
	if len(st) < 1 || len(st) > 8 {
		return invalid("between 1 and 8 open stages are required")
	}
	seen := map[string]bool{}
	for _, s := range st {
		id := strings.TrimSpace(s.ID)
		if id == "" || strings.TrimSpace(s.Name) == "" {
			return invalid("every stage needs an id and a name")
		}
		if id == StageWon || id == StageLost {
			return invalid("%q is reserved", id)
		}
		if seen[id] {
			return invalid("duplicate stage id %q", id)
		}
		seen[id] = true
	}
	return nil
}

func (r *Repository) GetSettings(ctx context.Context, org string) (Settings, error) {
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.crm_settings (org_id) VALUES ($1) ON CONFLICT DO NOTHING`, org); err != nil {
		return Settings{}, err
	}
	var s Settings
	var raw, sla []byte
	err := r.pool.QueryRow(ctx, `SELECT stages, currency, monthly_goal::float8, stale_days, cold_days, weekly_target,
		sla_lead_hours, sla_stage_days, default_scope FROM adrient.crm_settings WHERE org_id=$1`, org).
		Scan(&raw, &s.Currency, &s.MonthlyGoal, &s.StaleDays, &s.ColdDays, &s.WeeklyTarget, &s.SLALeadHours, &sla, &s.DefaultScope)
	if err != nil {
		return s, err
	}
	_ = json.Unmarshal(raw, &s.Stages)
	_ = json.Unmarshal(sla, &s.SLAStageDays)
	if s.SLAStageDays == nil {
		s.SLAStageDays = map[string]int{}
	}
	return s, nil
}

func (r *Repository) SaveSettings(ctx context.Context, org string, s Settings) (Settings, error) {
	if err := ValidateStages(s.Stages); err != nil {
		return s, err
	}
	if s.Currency == "" {
		s.Currency = "INR"
	}
	if s.MonthlyGoal < 0 || s.StaleDays < 0 || s.ColdDays < 0 || s.WeeklyTarget < 0 || s.SLALeadHours < 0 {
		return s, invalid("values cannot be negative")
	}
	if s.DefaultScope == "" {
		s.DefaultScope = ScopeAll
	}
	if s.DefaultScope != ScopeAll && s.DefaultScope != ScopeTeam && s.DefaultScope != ScopeOwn {
		return s, invalid("default_scope must be own, team or all")
	}
	if s.SLAStageDays == nil {
		s.SLAStageDays = map[string]int{}
	}
	for k, d := range s.SLAStageDays {
		if d < 0 || k == "" {
			return s, invalid("stage SLA days cannot be negative")
		}
	}
	cur, err := r.GetSettings(ctx, org)
	if err != nil {
		return s, err
	}
	// Deals in a removed open stage move to the first remaining stage.
	keep := map[string]bool{StageWon: true, StageLost: true}
	for _, st := range s.Stages {
		keep[st.ID] = true
	}
	for _, old := range cur.Stages {
		if !keep[old.ID] {
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.crm_deals SET stage=$3, stage_entered_at=now(), updated_at=now()
				WHERE org_id=$1 AND stage=$2`, org, old.ID, s.Stages[0].ID); err != nil {
				return s, err
			}
		}
	}
	b, _ := json.Marshal(s.Stages)
	sla, _ := json.Marshal(s.SLAStageDays)
	_, err = r.pool.Exec(ctx, `UPDATE adrient.crm_settings SET stages=$2, currency=$3, monthly_goal=$4, stale_days=$5,
		cold_days=$6, weekly_target=$7, sla_lead_hours=$8, sla_stage_days=$9, default_scope=$10, updated_at=now() WHERE org_id=$1`,
		org, b, strings.ToUpper(s.Currency), s.MonthlyGoal, s.StaleDays, s.ColdDays, s.WeeklyTarget, s.SLALeadHours, sla, s.DefaultScope)
	if err != nil {
		return s, err
	}
	return r.GetSettings(ctx, org)
}

// listJSON runs a query that returns a single json array column.
func (r *Repository) listJSON(ctx context.Context, q string, args ...any) (json.RawMessage, error) {
	var raw []byte
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (r *Repository) oneJSON(ctx context.Context, q string, args ...any) (json.RawMessage, error) {
	raw, err := r.listJSON(ctx, q, args...)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (len(raw) == 0 || string(raw) == "null")) {
		return nil, ErrNotFound
	}
	return raw, err
}

func nz(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// ---- companies ----

type Company struct {
	Name        string         `json:"name"`
	Industry    string         `json:"industry"`
	Website     string         `json:"website"`
	Phone       string         `json:"phone"`
	Address     string         `json:"address"`
	Notes       string         `json:"notes"`
	Tags        []string       `json:"tags"`
	OwnerID     string         `json:"owner_id"`
	TerritoryID string         `json:"territory_id"`
	Custom      map[string]any `json:"custom"`
}

const companyCols = `id, name, industry, website, phone, address, notes, tags, owner_id, territory_id, custom, created_at`

// jsonOrNil marshals v, or returns nil (SQL NULL) when v is nil so updates keep the stored value.
func jsonOrNil(v map[string]any) any {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
}

func uidOrNil(ctx context.Context) any {
	return nz(ViewerFrom(ctx).UserID)
}

func (r *Repository) ListCompanies(ctx context.Context, org string) (json.RawMessage, error) {
	return r.listJSON(ctx, `SELECT coalesce(json_agg(t ORDER BY lower(t.name)),'[]') FROM (SELECT `+companyCols+`
		FROM adrient.crm_companies co WHERE org_id=$1 AND `+ViewerFrom(ctx).ownerSQL("co")+`) t`, org)
}

func (r *Repository) SaveCompany(ctx context.Context, org, id string, c Company) (json.RawMessage, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return nil, invalid("company name is required")
	}
	if c.Tags == nil {
		c.Tags = []string{}
	}
	custom, err := r.checkCustom(ctx, org, "company", c.Custom, id == "")
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if id == "" {
		if custom == nil {
			custom = map[string]any{}
			if _, err := r.checkCustom(ctx, org, "company", custom, true); err != nil {
				return nil, err
			}
		}
		owner := nz(c.OwnerID)
		if owner == nil {
			owner = uidOrNil(ctx)
		}
		raw, err = r.oneJSON(ctx, `WITH t AS (INSERT INTO adrient.crm_companies (org_id,name,industry,website,phone,address,notes,tags,owner_id,created_by,territory_id,custom)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING `+companyCols+`) SELECT row_to_json(t) FROM t`,
			org, c.Name, c.Industry, c.Website, c.Phone, c.Address, c.Notes, c.Tags, owner, uidOrNil(ctx), nz(c.TerritoryID), jsonOrNil(custom))
	} else {
		raw, err = r.oneJSON(ctx, `WITH t AS (UPDATE adrient.crm_companies co SET name=$3,industry=$4,website=$5,phone=$6,address=$7,notes=$8,tags=$9,
			owner_id=coalesce($10::uuid, owner_id), territory_id=coalesce($11::uuid, territory_id), custom=coalesce($12::jsonb, custom)
			WHERE org_id=$1 AND id=$2 AND `+ViewerFrom(ctx).ownerSQL("co")+` RETURNING `+companyCols+`) SELECT row_to_json(t) FROM t`,
			org, id, c.Name, c.Industry, c.Website, c.Phone, c.Address, c.Notes, c.Tags, nz(c.OwnerID), nz(c.TerritoryID), jsonOrNil(custom))
	}
	return raw, err
}

func (r *Repository) DeleteCompany(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_companies co WHERE org_id=$1 AND id=$2 AND `+ViewerFrom(ctx).ownerSQL("co"), org, id)
}

func (r *Repository) del(ctx context.Context, q string, args ...any) error {
	ct, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- people (CRM profile of a Contact) ----

type Person struct {
	ContactID string         `json:"contact_id"`
	CompanyID string         `json:"company_id"`
	Status    string         `json:"status"`
	Source    string         `json:"source"`
	Tags      []string       `json:"tags"`
	Notes     string         `json:"notes"`
	OwnerID   string         `json:"owner_id"`
	Custom    map[string]any `json:"custom"`
}

const personCols = `id, contact_id, company_id, status, source, tags, notes, owner_id, score, custom, created_at`

// PersonStatuses are the allowed lead lifecycle values.
var PersonStatuses = map[string]bool{"lead": true, "customer": true, "inactive": true}

func (r *Repository) ListPeople(ctx context.Context, org string) (json.RawMessage, error) {
	return r.listJSON(ctx, `SELECT coalesce(json_agg(t ORDER BY t.created_at DESC),'[]') FROM (SELECT p.id, p.contact_id, p.company_id,
		p.status, p.source, p.tags, p.notes, p.owner_id, p.score, p.custom, p.created_at, c.display_name, c.emails, c.phones, c.job_title,
		CASE WHEN s.sla_lead_hours > 0 THEN p.created_at + make_interval(hours => s.sla_lead_hours) END AS sla_due_at,
		(SELECT min(a.occurred_at) FROM adrient.crm_activities a WHERE a.org_id=p.org_id AND a.contact_id=p.contact_id
		   AND a.kind IN ('call','meeting','email') AND a.occurred_at >= p.created_at) AS first_response_at
		FROM adrient.crm_people p JOIN adrient.contacts c ON c.id=p.contact_id
		LEFT JOIN adrient.crm_settings s ON s.org_id=p.org_id
		WHERE p.org_id=$1 AND c.trashed_at IS NULL AND `+ViewerFrom(ctx).ownerSQL("p")+`) t`, org)
}

func (r *Repository) SavePerson(ctx context.Context, org, id string, p Person) (json.RawMessage, error) {
	if p.Status == "" {
		p.Status = "lead"
	}
	if !PersonStatuses[p.Status] {
		return nil, invalid("unknown status %q", p.Status)
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	custom, err := r.checkCustom(ctx, org, "person", p.Custom, id == "")
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	created := false
	if id == "" {
		if p.ContactID == "" {
			return nil, invalid("contact_id is required")
		}
		var ok bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.contacts WHERE id=$1 AND org_id=$2)`, p.ContactID, org).Scan(&ok); err != nil || !ok {
			return nil, invalid("contact not found")
		}
		if custom == nil {
			custom = map[string]any{}
			if _, err := r.checkCustom(ctx, org, "person", custom, true); err != nil {
				return nil, err
			}
		}
		owner := nz(p.OwnerID)
		if owner == nil {
			owner = uidOrNil(ctx)
		}
		var existed bool
		_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.crm_people WHERE org_id=$1 AND contact_id=$2)`, org, p.ContactID).Scan(&existed)
		created = !existed
		raw, err = r.oneJSON(ctx, `WITH t AS (INSERT INTO adrient.crm_people (org_id,contact_id,company_id,status,source,tags,notes,owner_id,created_by,custom)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (org_id,contact_id) DO UPDATE SET company_id=EXCLUDED.company_id, status=EXCLUDED.status,
			  source=EXCLUDED.source, tags=EXCLUDED.tags, notes=EXCLUDED.notes, custom=adrient.crm_people.custom || EXCLUDED.custom
			RETURNING `+personCols+`) SELECT row_to_json(t) FROM t`,
			org, p.ContactID, nz(p.CompanyID), p.Status, p.Source, p.Tags, p.Notes, owner, uidOrNil(ctx), jsonOrNil(custom))
	} else {
		raw, err = r.oneJSON(ctx, `WITH t AS (UPDATE adrient.crm_people p SET company_id=$3,status=$4,source=$5,tags=$6,notes=$7,
			owner_id=coalesce($8::uuid, owner_id), custom=coalesce($9::jsonb, custom)
			WHERE org_id=$1 AND id=$2 AND `+ViewerFrom(ctx).ownerSQL("p")+` RETURNING `+personCols+`) SELECT row_to_json(t) FROM t`,
			org, id, nz(p.CompanyID), p.Status, p.Source, p.Tags, p.Notes, nz(p.OwnerID), jsonOrNil(custom))
	}
	if err != nil {
		return nil, err
	}
	var pid struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &pid)
	if err := r.RescorePerson(ctx, org, pid.ID); err != nil {
		slog.Warn("crm rescore", "err", err)
	}
	if created {
		r.fire(ctx, Event{Org: org, Trigger: TrigLeadCreated, PersonID: pid.ID, Key: "lead:" + pid.ID})
		raw, _ = r.oneJSON(ctx, `SELECT row_to_json(t) FROM (SELECT `+personCols+` FROM adrient.crm_people WHERE id=$1) t`, pid.ID)
	}
	return raw, nil
}

func (r *Repository) DeletePerson(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_people p WHERE org_id=$1 AND id=$2 AND `+ViewerFrom(ctx).ownerSQL("p"), org, id)
}

// ---- deals ----

// Member is one buying-committee entry.
type Member struct {
	ContactID string `json:"contact_id"`
	Role      string `json:"role"`
}

// DealItem is one product line on a deal.
type DealItem struct {
	ProductID   string  `json:"product_id"`
	Name        string  `json:"name"`
	Qty         float64 `json:"qty"`
	UnitPrice   float64 `json:"unit_price"`
	DiscountPct float64 `json:"discount_pct"`
}

// LineTotal is qty x price less the discount, rounded to 2 decimals.
func (i DealItem) LineTotal() float64 {
	return round2(i.Qty * i.UnitPrice * (1 - i.DiscountPct/100))
}

func round2(f float64) float64 {
	if f < 0 {
		return -float64(int64(-f*100+0.5)) / 100
	}
	return float64(int64(f*100+0.5)) / 100
}

type Deal struct {
	Title         string         `json:"title"`
	CompanyID     string         `json:"company_id"`
	ContactID     string         `json:"contact_id"`
	Stage         string         `json:"stage"`
	Value         float64        `json:"value"`
	Currency      string         `json:"currency"`
	Billing       string         `json:"billing"`
	ExpectedClose string         `json:"expected_close"`
	Source        string         `json:"source"`
	Notes         string         `json:"notes"`
	Committee     []Member       `json:"committee"`
	OwnerID       string         `json:"owner_id"`
	Custom        map[string]any `json:"custom"`
	Items         []DealItem     `json:"items"`
}

// Billings are the allowed deal billing intervals.
var Billings = map[string]bool{"one_time": true, "monthly": true, "quarterly": true, "yearly": true}

const dealSelect = `SELECT d.id, d.title, d.company_id, d.contact_id, d.stage, d.value::float8 AS value, d.currency, d.billing,
	d.expected_close, d.source, d.notes, d.close_reason, d.closed_at, d.stage_entered_at, d.created_at, d.updated_at, d.owner_id, d.custom,
	coalesce((SELECT json_agg(json_build_object('contact_id', dp.contact_id, 'role', dp.role))
	  FROM adrient.crm_deal_people dp WHERE dp.deal_id=d.id),'[]'::json) AS committee,
	coalesce((SELECT json_agg(json_build_object('product_id', i.product_id, 'name', i.name, 'qty', i.qty::float8,
	  'unit_price', i.unit_price::float8, 'discount_pct', i.discount_pct::float8) ORDER BY i.position)
	  FROM adrient.crm_deal_items i WHERE i.deal_id=d.id),'[]'::json) AS items
	FROM adrient.crm_deals d`

func (r *Repository) ListDeals(ctx context.Context, org string) (json.RawMessage, error) {
	return r.listJSON(ctx, `SELECT coalesce(json_agg(t ORDER BY t.updated_at DESC),'[]') FROM (`+dealSelect+` WHERE d.org_id=$1 AND `+ViewerFrom(ctx).ownerSQL("d")+`) t`, org)
}

func (r *Repository) GetDeal(ctx context.Context, org, id string) (json.RawMessage, error) {
	return r.oneJSON(ctx, `SELECT row_to_json(t) FROM (`+dealSelect+` WHERE d.org_id=$1 AND d.id=$2 AND `+ViewerFrom(ctx).ownerSQL("d")+`) t`, org, id)
}

func (r *Repository) validStage(ctx context.Context, org, stage string) error {
	if stage == StageWon || stage == StageLost {
		return nil
	}
	s, err := r.GetSettings(ctx, org)
	if err != nil {
		return err
	}
	for _, st := range s.Stages {
		if st.ID == stage {
			return nil
		}
	}
	return invalid("unknown stage %q", stage)
}

func (r *Repository) SaveDeal(ctx context.Context, org, id string, d Deal) (json.RawMessage, error) {
	d.Title = strings.TrimSpace(d.Title)
	if d.Title == "" {
		return nil, invalid("deal title is required")
	}
	if d.Value < 0 {
		return nil, invalid("value cannot be negative")
	}
	if d.Billing == "" {
		d.Billing = "one_time"
	}
	if !Billings[d.Billing] {
		return nil, invalid("unknown billing %q", d.Billing)
	}
	for _, it := range d.Items {
		if strings.TrimSpace(it.Name) == "" || it.Qty <= 0 || it.UnitPrice < 0 || it.DiscountPct < 0 || it.DiscountPct > 100 {
			return nil, invalid("each product line needs a name, a positive quantity, a price and a 0-100 discount")
		}
	}
	if len(d.Items) > 0 {
		d.Value = 0
		for _, it := range d.Items {
			d.Value += it.LineTotal()
		}
		d.Value = round2(d.Value)
	}
	set, err := r.GetSettings(ctx, org)
	if err != nil {
		return nil, err
	}
	if d.Currency == "" {
		d.Currency = set.Currency
	}
	if d.Stage == "" {
		d.Stage = set.Stages[0].ID
	}
	if err := r.validStage(ctx, org, d.Stage); err != nil {
		return nil, err
	}
	custom, err := r.checkCustom(ctx, org, "deal", d.Custom, id == "")
	if err != nil {
		return nil, err
	}
	if id == "" && custom == nil {
		custom = map[string]any{}
		if _, err := r.checkCustom(ctx, org, "deal", custom, true); err != nil {
			return nil, err
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created := false
	if id == "" {
		created = true
		owner := nz(d.OwnerID)
		if owner == nil {
			owner = uidOrNil(ctx)
		}
		err = tx.QueryRow(ctx, `INSERT INTO adrient.crm_deals (org_id,title,company_id,contact_id,stage,value,currency,billing,expected_close,source,notes,closed_at,owner_id,created_by,custom)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, CASE WHEN $5 IN ('won','lost') THEN now() END,$12,$13,$14) RETURNING id::text`,
			org, d.Title, nz(d.CompanyID), nz(d.ContactID), d.Stage, d.Value, strings.ToUpper(d.Currency), d.Billing, nz(d.ExpectedClose), d.Source, d.Notes,
			owner, uidOrNil(ctx), jsonOrNil(custom)).Scan(&id)
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO adrient.crm_stage_history (org_id,deal_id,stage) VALUES ($1,$2,$3)`, org, id, d.Stage); err != nil {
			return nil, err
		}
	} else {
		ct, err := tx.Exec(ctx, `UPDATE adrient.crm_deals d SET title=$3,company_id=$4,contact_id=$5,value=$6,currency=$7,billing=$8,
			expected_close=$9,source=$10,notes=$11,owner_id=coalesce($12::uuid, owner_id),custom=coalesce($13::jsonb, custom),updated_at=now()
			WHERE org_id=$1 AND id=$2 AND `+ViewerFrom(ctx).ownerSQL("d"),
			org, id, d.Title, nz(d.CompanyID), nz(d.ContactID), d.Value, strings.ToUpper(d.Currency), d.Billing, nz(d.ExpectedClose), d.Source, d.Notes,
			nz(d.OwnerID), jsonOrNil(custom))
		if err != nil {
			return nil, err
		}
		if ct.RowsAffected() == 0 {
			return nil, ErrNotFound
		}
	}
	if d.Committee != nil {
		if _, err = tx.Exec(ctx, `DELETE FROM adrient.crm_deal_people WHERE deal_id=$1`, id); err != nil {
			return nil, err
		}
		for _, m := range d.Committee {
			if m.ContactID == "" {
				continue
			}
			if _, err = tx.Exec(ctx, `INSERT INTO adrient.crm_deal_people (deal_id,contact_id,role)
				SELECT $1,$2,$3 WHERE EXISTS (SELECT 1 FROM adrient.contacts WHERE id=$2 AND org_id=$4)
				ON CONFLICT (deal_id,contact_id) DO UPDATE SET role=EXCLUDED.role`, id, m.ContactID, m.Role, org); err != nil {
				return nil, err
			}
		}
	}
	if d.Items != nil {
		if _, err = tx.Exec(ctx, `DELETE FROM adrient.crm_deal_items WHERE deal_id=$1`, id); err != nil {
			return nil, err
		}
		for i, it := range d.Items {
			if _, err = tx.Exec(ctx, `INSERT INTO adrient.crm_deal_items (deal_id,product_id,name,qty,unit_price,discount_pct,position)
				VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, nz(it.ProductID), strings.TrimSpace(it.Name), it.Qty, it.UnitPrice, it.DiscountPct, i); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if created {
		r.onStageEntered(ctx, org, id, d.Stage, "")
	}
	return r.GetDeal(ctx, org, id)
}

// MoveDeal changes a deal's stage, recording history and (for won/lost) the reason.
// Leaving a stage whose playbook is marked required needs its checklist complete
// (except when the deal is lost). Moving fires stage_changed automation once.
func (r *Repository) MoveDeal(ctx context.Context, org, id, stage, reason string) (json.RawMessage, error) {
	if err := r.validStage(ctx, org, stage); err != nil {
		return nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var cur string
	if err := tx.QueryRow(ctx, `SELECT stage FROM adrient.crm_deals d WHERE org_id=$1 AND id=$2 AND `+ViewerFrom(ctx).ownerSQL("d")+` FOR UPDATE`, org, id).Scan(&cur); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	moved := false
	if cur != stage {
		if stage != StageLost {
			var open int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.crm_playbooks pb JOIN adrient.crm_deal_checklist c
				ON c.deal_id=$2 AND c.stage=pb.stage AND NOT c.done WHERE pb.org_id=$1 AND pb.stage=$3 AND pb.required`, org, id, cur).Scan(&open); err != nil {
				return nil, err
			}
			if open > 0 {
				return nil, invalid("complete the %d open checklist item(s) for the current stage first", open)
			}
		}
		terminal := stage == StageWon || stage == StageLost
		if _, err := tx.Exec(ctx, `UPDATE adrient.crm_deals SET stage=$3, stage_entered_at=now(), updated_at=now(),
			closed_at=CASE WHEN $4 THEN now() ELSE NULL END, close_reason=CASE WHEN $4 THEN $5 ELSE '' END
			WHERE org_id=$1 AND id=$2`, org, id, stage, terminal, reason); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.crm_stage_history (org_id,deal_id,stage) VALUES ($1,$2,$3)`, org, id, stage); err != nil {
			return nil, err
		}
		moved = true
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if moved {
		r.onStageEntered(ctx, org, id, stage, cur)
	}
	return r.GetDeal(ctx, org, id)
}

func (r *Repository) DeleteDeal(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_deals d WHERE org_id=$1 AND id=$2 AND `+ViewerFrom(ctx).ownerSQL("d"), org, id)
}

func (r *Repository) ListHistory(ctx context.Context, org string) (json.RawMessage, error) {
	return r.listJSON(ctx, `SELECT coalesce(json_agg(t ORDER BY t.entered_at),'[]') FROM (SELECT h.id, h.deal_id, h.stage, h.entered_at
		FROM adrient.crm_stage_history h JOIN adrient.crm_deals d ON d.id=h.deal_id WHERE h.org_id=$1 AND `+ViewerFrom(ctx).ownerSQL("d")+`
		ORDER BY h.entered_at DESC LIMIT 20000) t`, org)
}

// ---- follow-ups ----

type Followup struct {
	Title           string `json:"title"`
	DueDate         string `json:"due_date"`
	DueTime         string `json:"due_time"`
	DealID          string `json:"deal_id"`
	ContactID       string `json:"contact_id"`
	Done            bool   `json:"done"`
	CalendarEventID string `json:"calendar_event_id"`
}

const followupCols = `id, title, due_date, due_time, deal_id, contact_id, done, calendar_event_id, user_id, created_at`

func (r *Repository) ListFollowups(ctx context.Context, org string) (json.RawMessage, error) {
	return r.listJSON(ctx, `SELECT coalesce(json_agg(t ORDER BY t.done, t.due_date, t.due_time),'[]') FROM (SELECT `+followupCols+`
		FROM adrient.crm_followups a WHERE org_id=$1 AND `+ViewerFrom(ctx).childSQL("a")+`) t`, org)
}

func (r *Repository) SaveFollowup(ctx context.Context, org, id string, f Followup) (json.RawMessage, error) {
	f.Title = strings.TrimSpace(f.Title)
	if f.Title == "" || f.DueDate == "" {
		return nil, invalid("title and due date are required")
	}
	if id == "" {
		return r.oneJSON(ctx, `WITH t AS (INSERT INTO adrient.crm_followups (org_id,title,due_date,due_time,deal_id,contact_id,done,calendar_event_id,user_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+followupCols+`) SELECT row_to_json(t) FROM t`,
			org, f.Title, f.DueDate, f.DueTime, nz(f.DealID), nz(f.ContactID), f.Done, f.CalendarEventID, uidOrNil(ctx))
	}
	return r.oneJSON(ctx, `WITH t AS (UPDATE adrient.crm_followups a SET title=$3,due_date=$4,due_time=$5,deal_id=$6,contact_id=$7,done=$8,calendar_event_id=$9
		WHERE org_id=$1 AND id=$2 AND `+ViewerFrom(ctx).childSQL("a")+` RETURNING `+followupCols+`) SELECT row_to_json(t) FROM t`,
		org, id, f.Title, f.DueDate, f.DueTime, nz(f.DealID), nz(f.ContactID), f.Done, f.CalendarEventID)
}

func (r *Repository) DeleteFollowup(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_followups a WHERE org_id=$1 AND id=$2 AND `+ViewerFrom(ctx).childSQL("a"), org, id)
}

// ---- activities ----

type Activity struct {
	Kind            string `json:"kind"`
	Summary         string `json:"summary"`
	OccurredAt      string `json:"occurred_at"`
	DealID          string `json:"deal_id"`
	ContactID       string `json:"contact_id"`
	CompanyID       string `json:"company_id"`
	CalendarEventID string `json:"calendar_event_id"`
	MailThreadID    string `json:"mail_thread_id"`
	Direction       string `json:"direction"`
	DurationSec     int    `json:"duration_sec"`
	MeetURL         string `json:"meet_url"`
}

// ActivityKinds are the allowed activity types.
var ActivityKinds = map[string]bool{"call": true, "meeting": true, "email": true, "note": true}

const activityCols = `id, kind, summary, occurred_at, deal_id, contact_id, company_id, calendar_event_id, mail_thread_id, direction, duration_sec, meet_url, user_id, created_at`

func (r *Repository) ListActivities(ctx context.Context, org string) (json.RawMessage, error) {
	return r.listJSON(ctx, `SELECT coalesce(json_agg(t ORDER BY t.occurred_at DESC),'[]') FROM (SELECT `+activityCols+`
		FROM adrient.crm_activities a WHERE org_id=$1 AND `+ViewerFrom(ctx).childSQL("a")+` ORDER BY occurred_at DESC LIMIT 2000) t`, org)
}

func (r *Repository) SaveActivity(ctx context.Context, org string, a Activity) (json.RawMessage, error) {
	if a.Kind == "" {
		a.Kind = "note"
	}
	if !ActivityKinds[a.Kind] {
		return nil, invalid("unknown activity kind %q", a.Kind)
	}
	if strings.TrimSpace(a.Summary) == "" {
		return nil, invalid("a summary is required")
	}
	if a.Direction != "" && a.Direction != "in" && a.Direction != "out" {
		return nil, invalid("direction must be in or out")
	}
	if a.DurationSec < 0 {
		return nil, invalid("duration cannot be negative")
	}
	raw, err := r.oneJSON(ctx, `WITH t AS (INSERT INTO adrient.crm_activities (org_id,kind,summary,occurred_at,deal_id,contact_id,company_id,calendar_event_id,mail_thread_id,direction,duration_sec,meet_url,user_id)
		VALUES ($1,$2,$3,coalesce($4::timestamptz, now()),$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING `+activityCols+`) SELECT row_to_json(t) FROM t`,
		org, a.Kind, a.Summary, nz(a.OccurredAt), nz(a.DealID), nz(a.ContactID), nz(a.CompanyID), a.CalendarEventID, a.MailThreadID, a.Direction, a.DurationSec, a.MeetURL, uidOrNil(ctx))
	if err == nil && a.ContactID != "" {
		var pid string
		if r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.crm_people WHERE org_id=$1 AND contact_id=$2`, org, a.ContactID).Scan(&pid) == nil {
			_ = r.RescorePerson(ctx, org, pid)
		}
	}
	return raw, err
}

func (r *Repository) DeleteActivity(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_activities a WHERE org_id=$1 AND id=$2 AND `+ViewerFrom(ctx).childSQL("a"), org, id)
}
