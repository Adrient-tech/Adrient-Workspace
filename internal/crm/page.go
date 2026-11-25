package crm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Paged list reads. The plain list endpoints now return one window (default
// 1,000 rows, ?limit= up to 2,000, ?offset=, ?q= search) instead of every
// record, and /summary gives the dashboard its totals from SQL.

// ListOpts narrows a paged list.
type ListOpts struct {
	Q      string
	Status string // people only: lead | customer | inactive
	Limit  int
	Offset int
}

const (
	defaultListLimit = 1000
	maxListLimit     = 2000
)

func listOptsFrom(r *http.Request) ListOpts {
	q := r.URL.Query()
	o := ListOpts{Q: strings.TrimSpace(q.Get("q")), Status: q.Get("status")}
	o.Limit, _ = strconv.Atoi(q.Get("limit"))
	o.Offset, _ = strconv.Atoi(q.Get("offset"))
	if o.Limit <= 0 {
		o.Limit = defaultListLimit
	}
	if o.Limit > maxListLimit {
		o.Limit = maxListLimit
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	return o
}

func likeArg(q string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
}

// ListPeoplePage is ListPeople for one window, newest first.
func (r *Repository) ListPeoplePage(ctx context.Context, org string, o ListOpts) (json.RawMessage, error) {
	args := []any{org}
	extra := ""
	if o.Q != "" {
		args = append(args, likeArg(o.Q))
		n := len(args)
		extra += fmt.Sprintf(` AND (c.display_name ILIKE $%d OR c.emails::text ILIKE $%d OR p.tags::text ILIKE $%d)`, n, n, n)
	}
	if PersonStatuses[o.Status] {
		args = append(args, o.Status)
		extra += fmt.Sprintf(` AND p.status=$%d`, len(args))
	}
	return r.listJSON(ctx, `SELECT coalesce(json_agg(t ORDER BY t.created_at DESC, t.id DESC),'[]') FROM (SELECT p.id, p.contact_id, p.company_id,
		p.status, p.source, p.tags, p.notes, p.owner_id, p.score, p.custom, p.created_at, c.display_name, c.emails, c.phones, c.job_title,
		CASE WHEN s.sla_lead_hours > 0 THEN p.created_at + make_interval(hours => s.sla_lead_hours) END AS sla_due_at,
		(SELECT min(a.occurred_at) FROM adrient.crm_activities a WHERE a.org_id=p.org_id AND a.contact_id=p.contact_id
		   AND a.kind IN ('call','meeting','email') AND a.occurred_at >= p.created_at) AS first_response_at
		FROM adrient.crm_people p JOIN adrient.contacts c ON c.id=p.contact_id
		LEFT JOIN adrient.crm_settings s ON s.org_id=p.org_id
		WHERE p.org_id=$1 AND c.trashed_at IS NULL AND `+ViewerFrom(ctx).ownerSQL("p")+extra+
		fmt.Sprintf(` ORDER BY p.created_at DESC, p.id DESC LIMIT %d OFFSET %d) t`, o.Limit, o.Offset), args...)
}

// ListCompaniesPage is ListCompanies for one window, by name.
func (r *Repository) ListCompaniesPage(ctx context.Context, org string, o ListOpts) (json.RawMessage, error) {
	args := []any{org}
	extra := ""
	if o.Q != "" {
		args = append(args, likeArg(o.Q))
		extra = fmt.Sprintf(` AND (co.name ILIKE $%d OR co.industry ILIKE $%d OR co.tags::text ILIKE $%d)`, len(args), len(args), len(args))
	}
	return r.listJSON(ctx, `SELECT coalesce(json_agg(t ORDER BY lower(t.name), t.id),'[]') FROM (SELECT `+companyCols+`
		FROM adrient.crm_companies co WHERE org_id=$1 AND `+ViewerFrom(ctx).ownerSQL("co")+extra+
		fmt.Sprintf(` ORDER BY lower(co.name), co.id LIMIT %d OFFSET %d) t`, o.Limit, o.Offset), args...)
}

// ListDealsPage is ListDeals for one window. Open deals come first (they are
// what the board and dashboard act on), then the most recently updated.
func (r *Repository) ListDealsPage(ctx context.Context, org string, o ListOpts) (json.RawMessage, error) {
	args := []any{org}
	extra := ""
	if o.Q != "" {
		args = append(args, likeArg(o.Q))
		extra = fmt.Sprintf(` AND d.title ILIKE $%d`, len(args))
	}
	return r.listJSON(ctx, `SELECT coalesce(json_agg(t ORDER BY (t.stage IN ('won','lost')), t.updated_at DESC, t.id DESC),'[]') FROM (`+dealSelect+
		` WHERE d.org_id=$1 AND `+ViewerFrom(ctx).ownerSQL("d")+extra+
		fmt.Sprintf(` ORDER BY (d.stage IN ('won','lost')), d.updated_at DESC, d.id DESC LIMIT %d OFFSET %d) t`, o.Limit, o.Offset), args...)
}

// StageTotal is one stage's open/won totals for the dashboard.
type StageTotal struct {
	Stage string  `json:"stage"`
	Count int64   `json:"count"`
	Value float64 `json:"value"`
	MRR   float64 `json:"mrr"`
}

// Summary is the dashboard's server-side totals over every deal and lead the
// caller may see (not just the loaded window).
type Summary struct {
	ByStage      []StageTotal `json:"by_stage"`
	WonThisMonth float64      `json:"won_this_month"`
	People       int64        `json:"people"`
	Leads        int64        `json:"leads"`
	Companies    int64        `json:"companies"`
	Deals        int64        `json:"deals"`
}

// GetSummary computes the dashboard totals in SQL.
func (r *Repository) GetSummary(ctx context.Context, org string) (Summary, error) {
	v := ViewerFrom(ctx)
	s := Summary{ByStage: []StageTotal{}}
	rows, err := r.pool.Query(ctx, `SELECT d.stage, count(*), coalesce(sum(d.value),0)::float8,
		coalesce(sum(CASE d.billing WHEN 'monthly' THEN d.value WHEN 'quarterly' THEN d.value/3 WHEN 'yearly' THEN d.value/12 ELSE 0 END),0)::float8
		FROM adrient.crm_deals d WHERE d.org_id=$1 AND `+v.ownerSQL("d")+` GROUP BY d.stage`, org)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var t StageTotal
		if err := rows.Scan(&t.Stage, &t.Count, &t.Value, &t.MRR); err != nil {
			rows.Close()
			return s, err
		}
		s.Deals += t.Count
		s.ByStage = append(s.ByStage, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return s, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT coalesce(sum(d.value),0)::float8 FROM adrient.crm_deals d
		WHERE d.org_id=$1 AND d.stage='won' AND d.closed_at >= date_trunc('month', now()) AND `+v.ownerSQL("d"), org).Scan(&s.WonThisMonth); err != nil {
		return s, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE p.status='lead') FROM adrient.crm_people p
		JOIN adrient.contacts c ON c.id=p.contact_id WHERE p.org_id=$1 AND c.trashed_at IS NULL AND `+v.ownerSQL("p"), org).Scan(&s.People, &s.Leads); err != nil {
		return s, err
	}
	err = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.crm_companies co WHERE org_id=$1 AND `+v.ownerSQL("co"), org).Scan(&s.Companies)
	return s, err
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
