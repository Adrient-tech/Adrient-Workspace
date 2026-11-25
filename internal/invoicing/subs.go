package invoicing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Plan is a subscription / membership plan billed on an interval.
type Plan struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Interval    string  `json:"interval"`
	Price       float64 `json:"price"`
	TaxRate     float64 `json:"tax_rate"`
	HSNSAC      string  `json:"hsn_sac"`
	Currency    string  `json:"currency"`
	Active      bool    `json:"active"`
}

// Subscription is a customer on a plan.
type Subscription struct {
	ID                string  `json:"id"`
	PlanID            string  `json:"plan_id"`
	PlanName          string  `json:"plan_name"`
	ContactID         string  `json:"contact_id"`
	ClientName        string  `json:"client_name"`
	ClientEmail       string  `json:"client_email"`
	PlaceOfSupply     string  `json:"place_of_supply"`
	Quantity          float64 `json:"quantity"`
	Status            string  `json:"status"` // active | past_due | paused | cancelled
	StartDate         string  `json:"start_date"`
	NextRenewal       string  `json:"next_renewal"`
	EndDate           string  `json:"end_date"`
	CancelAtPeriodEnd bool    `json:"cancel_at_period_end"`
	DunningDays       int     `json:"dunning_days"`
	Renewals          int     `json:"renewals"`
	Price             float64 `json:"price"`
	Interval          string  `json:"interval"`
}

var intervals = map[string]bool{"weekly": true, "monthly": true, "quarterly": true, "yearly": true}

// ListPlans returns the org's plans.
func (r *Repository) ListPlans(ctx context.Context, orgID string) ([]Plan, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, description, interval, price::float8, tax_rate::float8, hsn_sac, currency, active
		FROM adrient.sales_plans WHERE org_id=$1 ORDER BY lower(name)`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Interval, &p.Price, &p.TaxRate, &p.HSNSAC, &p.Currency, &p.Active); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SavePlan creates (id empty) or updates a plan. Changing a price affects the
// next renewal of existing subscriptions, never invoices already issued.
func (r *Repository) SavePlan(ctx context.Context, orgID, id string, p Plan) (Plan, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return p, invalid("name is required")
	}
	if !intervals[p.Interval] {
		return p, invalid("interval must be weekly, monthly, quarterly or yearly")
	}
	if p.Price < 0 || p.TaxRate < 0 {
		return p, invalid("price and tax must not be negative")
	}
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	if p.Currency == "" {
		p.Currency = "INR"
	}
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.sales_plans (org_id,name,description,interval,price,tax_rate,hsn_sac,currency,active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`, orgID, p.Name, p.Description, p.Interval, Round2(p.Price), p.TaxRate, p.HSNSAC, p.Currency, p.Active).Scan(&p.ID)
	} else {
		err = r.exec(ctx, `UPDATE adrient.sales_plans SET name=$3,description=$4,interval=$5,price=$6,tax_rate=$7,hsn_sac=$8,currency=$9,active=$10
			WHERE org_id=$1 AND id=$2::uuid`, orgID, id, p.Name, p.Description, p.Interval, Round2(p.Price), p.TaxRate, p.HSNSAC, p.Currency, p.Active)
		p.ID = id
	}
	return p, err
}

const subCols = `s.id::text, s.plan_id::text, p.name, COALESCE(s.contact_id::text,''), s.client_name, s.client_email, s.place_of_supply,
	s.quantity::float8, s.status, to_char(s.start_date,'YYYY-MM-DD'), to_char(s.next_renewal,'YYYY-MM-DD'),
	COALESCE(to_char(s.end_date,'YYYY-MM-DD'),''), s.cancel_at_period_end, s.dunning_days, s.renewals, p.price::float8, p.interval`

func scanSub(row pgx.Row) (Subscription, error) {
	var s Subscription
	err := row.Scan(&s.ID, &s.PlanID, &s.PlanName, &s.ContactID, &s.ClientName, &s.ClientEmail, &s.PlaceOfSupply, &s.Quantity, &s.Status,
		&s.StartDate, &s.NextRenewal, &s.EndDate, &s.CancelAtPeriodEnd, &s.DunningDays, &s.Renewals, &s.Price, &s.Interval)
	return s, err
}

// ListSubscriptions returns subscriptions, newest first.
func (r *Repository) ListSubscriptions(ctx context.Context, orgID string) ([]Subscription, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+subCols+` FROM adrient.sales_subscriptions s JOIN adrient.sales_plans p ON p.id=s.plan_id
		WHERE s.org_id=$1 ORDER BY s.created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Subscription{}
	for rows.Next() {
		s, err := scanSub(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CreateSubscription puts a customer on a plan. The first invoice is raised on
// the start date (when the renewal runner next executes).
func (r *Repository) CreateSubscription(ctx context.Context, orgID string, in Subscription) (Subscription, error) {
	in.ClientName = strings.TrimSpace(in.ClientName)
	if in.ClientName == "" {
		return in, invalid("customer name is required")
	}
	if in.Quantity <= 0 {
		in.Quantity = 1
	}
	if in.StartDate == "" {
		in.StartDate = today().Format("2006-01-02")
	}
	if parseDate(in.StartDate) == nil || (in.EndDate != "" && parseDate(in.EndDate) == nil) {
		return in, invalid("dates must be YYYY-MM-DD")
	}
	if in.DunningDays <= 0 {
		in.DunningDays = 7
	}
	if in.ContactID != "" && len(in.ContactID) != 36 {
		return in, invalid("invalid customer")
	}
	if in.PlaceOfSupply != "" && StateCode(in.PlaceOfSupply) == "" {
		return in, invalid("place of supply must be a GST state code or name")
	}
	var id string
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.sales_subscriptions (org_id, plan_id, contact_id, client_name, client_email, place_of_supply, quantity,
		start_date, next_renewal, end_date, dunning_days) SELECT $1,p.id,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8::date,$8::date,NULLIF($9,'')::date,$10
		FROM adrient.sales_plans p WHERE p.org_id=$1 AND p.id=$2::uuid AND p.active RETURNING id::text`,
		orgID, in.PlanID, in.ContactID, in.ClientName, in.ClientEmail, StateCode(in.PlaceOfSupply), in.Quantity, in.StartDate, in.EndDate, in.DunningDays).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return in, invalid("choose an active plan")
	}
	if err != nil {
		return in, err
	}
	return r.getSub(ctx, orgID, id)
}

func (r *Repository) getSub(ctx context.Context, orgID, id string) (Subscription, error) {
	s, err := scanSub(r.pool.QueryRow(ctx, `SELECT `+subCols+` FROM adrient.sales_subscriptions s JOIN adrient.sales_plans p ON p.id=s.plan_id
		WHERE s.org_id=$1 AND s.id=$2::uuid`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

// SetSubscription changes status: pause, resume, cancel (now) or cancel_at_end.
func (r *Repository) SetSubscription(ctx context.Context, orgID, id, action string) (Subscription, error) {
	var q string
	switch action {
	case "pause":
		q = `status='paused'`
	case "resume":
		q = `status='active'`
	case "cancel":
		q = `status='cancelled', cancel_at_period_end=false`
	case "cancel_at_end":
		q = `cancel_at_period_end=true`
	case "keep":
		q = `cancel_at_period_end=false`
	default:
		return Subscription{}, invalid("action must be pause, resume, cancel, cancel_at_end or keep")
	}
	if err := r.exec(ctx, `UPDATE adrient.sales_subscriptions SET `+q+`, updated_at=now() WHERE org_id=$1 AND id=$2::uuid AND status <> 'cancelled'`, orgID, id); err != nil {
		return Subscription{}, err
	}
	return r.getSub(ctx, orgID, id)
}

// RenewalResult summarises one run of the subscription runner.
type RenewalResult struct {
	Invoiced []Document `json:"invoiced"`
	PastDue  int        `json:"past_due"`
	Paused   int        `json:"paused"`
	Resumed  int        `json:"resumed"`
	Ended    int        `json:"ended"`
}

// RunSubscriptions bills every active subscription whose renewal date has
// arrived (catching up at most 12 missed periods) and applies dunning.
//
//   - One invoice per subscription period: the period start date and the
//     subscription id are unique on the invoice, so a repeated or parallel run
//     never bills a period twice.
//   - Dunning: a subscription with an overdue unpaid invoice becomes past_due;
//     when the oldest unpaid invoice is more than dunning_days overdue it is
//     paused and stops renewing. Paying it brings the subscription back to active.
func (r *Repository) RunSubscriptions(ctx context.Context, orgID, userID string) (RenewalResult, error) {
	var res RenewalResult
	set, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return res, err
	}
	subs, err := r.ListSubscriptions(ctx, orgID)
	if err != nil {
		return res, err
	}
	plans, err := r.ListPlans(ctx, orgID)
	if err != nil {
		return res, err
	}
	pm := map[string]Plan{}
	for _, p := range plans {
		pm[p.ID] = p
	}
	now := today()
	for _, sub := range subs {
		if sub.Status == "cancelled" {
			continue
		}
		// Dunning first, from the invoices already issued.
		worst, err := r.worstOverdue(ctx, orgID, sub.ID, now)
		if err != nil {
			return res, err
		}
		switch {
		case worst > sub.DunningDays && sub.Status != "paused":
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.sales_subscriptions SET status='paused', updated_at=now() WHERE id=$1::uuid`, sub.ID)
			sub.Status = "paused"
			res.Paused++
		case worst > 0 && sub.Status == "active":
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.sales_subscriptions SET status='past_due', updated_at=now() WHERE id=$1::uuid`, sub.ID)
			sub.Status = "past_due"
			res.PastDue++
		case worst == 0 && sub.Status == "past_due":
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.sales_subscriptions SET status='active', updated_at=now() WHERE id=$1::uuid`, sub.ID)
			sub.Status = "active"
			res.Resumed++
		}
		if sub.Status == "paused" {
			continue
		}
		plan, ok := pm[sub.PlanID]
		next := parseDate(sub.NextRenewal)
		end := parseDate(sub.EndDate)
		if !ok || next == nil {
			continue
		}
		n := 0
		for !next.After(now) && n < 12 {
			if end != nil && next.After(*end) {
				break
			}
			period := next.Format("2006-01-02")
			var exists bool
			if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM adrient.invc_documents WHERE subscription_id=$1::uuid AND recurring_period=$2::date)`, sub.ID, period).Scan(&exists); err != nil {
				return res, err
			}
			if !exists {
				d, err := r.SaveDocument(ctx, orgID, userID, "", DocInput{Kind: "invoice", ContactID: sub.ContactID, ClientName: sub.ClientName,
					ClientEmail: sub.ClientEmail, Currency: plan.Currency, IssueDate: period, PlaceOfSupply: sub.PlaceOfSupply,
					DueDate:        next.AddDate(0, 0, set.DueDays).Format("2006-01-02"),
					Lines:          []Line{{Description: plan.Name + " (" + period + ")", Qty: sub.Quantity, Unit: "", Price: plan.Price, TaxRate: plan.TaxRate, HSNSAC: plan.HSNSAC}},
					SubscriptionID: sub.ID, RecurringDate: period, SourceRef: "subscription:" + sub.ID, NoStock: true})
				if err != nil {
					return res, err
				}
				d, _, _ = r.MarkSent(ctx, orgID, d.ID)
				res.Invoiced = append(res.Invoiced, d)
			}
			nx := NextDate(*next, plan.Interval)
			next = &nx
			n++
			if sub.CancelAtPeriodEnd { // the period just billed is the last one
				break
			}
		}
		if n > 0 {
			status := sub.Status
			if sub.CancelAtPeriodEnd {
				status = "cancelled"
				res.Ended++
			} else if end != nil && next.After(*end) {
				status = "cancelled"
				res.Ended++
			}
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.sales_subscriptions SET next_renewal=$3::date, renewals=renewals+$4, status=$5, updated_at=now()
				WHERE org_id=$1 AND id=$2::uuid`, orgID, sub.ID, next.Format("2006-01-02"), n, status); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// worstOverdue returns the largest number of days any unpaid invoice of the
// subscription is past due (0 when none is).
func (r *Repository) worstOverdue(ctx context.Context, orgID, subID string, now time.Time) (int, error) {
	docs, err := r.docsOfSubscription(ctx, orgID, subID)
	if err != nil {
		return 0, err
	}
	worst := 0
	for _, d := range docs {
		if d.Balance <= 0.004 || d.Status == "draft" {
			continue
		}
		if due := parseDate(orDue(d)); due != nil && due.Before(now) {
			if days := int(now.Sub(*due).Hours() / 24); days > worst {
				worst = days
			}
		}
	}
	return worst, nil
}

func (r *Repository) docsOfSubscription(ctx context.Context, orgID, subID string) ([]Document, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+docCols+` FROM adrient.invc_documents WHERE org_id=$1 AND subscription_id=$2::uuid AND deleted_at IS NULL`, orgID, subID)
	if err != nil {
		return nil, err
	}
	var docs []Document
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		docs = append(docs, d)
	}
	rows.Close()
	if err := r.loadPayments(ctx, orgID, docs); err != nil {
		return nil, err
	}
	for i := range docs {
		docs[i].derive(today())
	}
	return docs, nil
}
