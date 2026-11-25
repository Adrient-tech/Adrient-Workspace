package inventory

// Reorder rules that create draft purchase orders grouped by supplier, and the
// supplier performance report.

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReorderRule sets min/max for a product.
type ReorderRule struct {
	ID          string  `json:"id"`
	ProductID   string  `json:"product_id"`
	ProductName string  `json:"product_name"`
	SKU         string  `json:"sku"`
	MinQty      float64 `json:"min_qty"`
	MaxQty      float64 `json:"max_qty"`
	Multiple    float64 `json:"multiple"`
	SupplierID  string  `json:"supplier_id"`
	Active      bool    `json:"active"`
}

// ListReorderRules lists rules.
func (r *Repository) ListReorderRules(ctx context.Context, orgID string) ([]ReorderRule, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT x.id::text, x.product_id::text, p.name, p.sku, x.min_qty::float8, x.max_qty::float8, x.multiple::float8, COALESCE(x.supplier_id::text,''), x.active
		 FROM adrient.inv_reorder_rules x JOIN adrient.inv_products p ON p.id=x.product_id WHERE x.org_id=$1 ORDER BY p.name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReorderRule{}
	for rows.Next() {
		var x ReorderRule
		if err := rows.Scan(&x.ID, &x.ProductID, &x.ProductName, &x.SKU, &x.MinQty, &x.MaxQty, &x.Multiple, &x.SupplierID, &x.Active); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SaveReorderRule creates or replaces the rule of a product.
func (r *Repository) SaveReorderRule(ctx context.Context, orgID string, x ReorderRule) (ReorderRule, error) {
	if x.MinQty < 0 || x.MaxQty < 0 || x.Multiple < 0 {
		return x, invalid("quantities cannot be negative")
	}
	if x.MaxQty > 0 && x.MaxQty < x.MinQty {
		return x, invalid("maximum must be at least the minimum")
	}
	if x.Multiple == 0 {
		x.Multiple = 1
	}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO adrient.inv_reorder_rules (org_id, product_id, min_qty, max_qty, multiple, supplier_id, active)
		 SELECT $1,p.id,$3,$4,$5,NULLIF($6,'')::uuid,$7 FROM adrient.inv_products p WHERE p.org_id=$1 AND p.id=$2
		 ON CONFLICT (org_id, product_id) DO UPDATE SET min_qty=EXCLUDED.min_qty, max_qty=EXCLUDED.max_qty, multiple=EXCLUDED.multiple,
		   supplier_id=EXCLUDED.supplier_id, active=EXCLUDED.active RETURNING id::text`,
		orgID, x.ProductID, x.MinQty, x.MaxQty, x.Multiple, x.SupplierID, x.Active).Scan(&x.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, invalid("product not found")
	}
	return x, err
}

// DeleteReorderRule removes a rule.
func (r *Repository) DeleteReorderRule(ctx context.Context, orgID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.inv_reorder_rules WHERE org_id=$1 AND id=$2`, orgID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Proposal is one product the reorder run wants to buy.
type Proposal struct {
	ProductID    string  `json:"product_id"`
	ProductName  string  `json:"product_name"`
	SKU          string  `json:"sku"`
	Position     float64 `json:"position"` // on hand + incoming - reserved
	OnHand       float64 `json:"on_hand"`
	Incoming     float64 `json:"incoming"`
	Reserved     float64 `json:"reserved"`
	MinQty       float64 `json:"min_qty"`
	Qty          float64 `json:"qty"`
	UnitCost     float64 `json:"unit_cost"`
	SupplierID   string  `json:"supplier_id"`
	SupplierName string  `json:"supplier_name"`
	LeadDays     int     `json:"lead_days"`
	Source       string  `json:"source"` // rule | minimum
}

// ReorderRun is the outcome of a reorder run.
type ReorderRun struct {
	Proposals  []Proposal      `json:"proposals"`
	Unassigned []Proposal      `json:"unassigned"` // no supplier known
	Created    []PurchaseOrder `json:"created"`    // draft POs created or extended
	Skipped    []Proposal      `json:"skipped"`    // already on an open draft
}

// RunReorder works out what to buy. With create=true it makes draft purchase
// orders, one per supplier; products already on an open auto-created draft are
// not added twice, so running it again is safe.
func (r *Repository) RunReorder(ctx context.Context, orgID, userID string, create bool) (ReorderRun, error) {
	run := ReorderRun{Proposals: []Proposal{}, Unassigned: []Proposal{}, Created: []PurchaseOrder{}, Skipped: []Proposal{}}
	av, err := r.ListAvailability(ctx, orgID, "", "")
	if err != nil {
		return run, err
	}
	rules, err := r.ListReorderRules(ctx, orgID)
	if err != nil {
		return run, err
	}
	ruleOf := map[string]ReorderRule{}
	for _, x := range rules {
		ruleOf[x.ProductID] = x
	}
	type extra struct {
		reorderQty float64
		kit        bool
	}
	rq := map[string]float64{}
	rows, err := r.pool.Query(ctx, `SELECT id::text, reorder_qty::float8 FROM adrient.inv_products WHERE org_id=$1`, orgID)
	if err != nil {
		return run, err
	}
	for rows.Next() {
		var id string
		var q float64
		if err := rows.Scan(&id, &q); err != nil {
			rows.Close()
			return run, err
		}
		rq[id] = q
	}
	rows.Close()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return run, err
	}
	defer tx.Rollback(ctx)
	for _, a := range av {
		if a.IsKit {
			continue
		}
		position := a.OnHand + a.Incoming + a.InTransit - a.Reserved
		min, max, mult, source, forced := a.MinStock, 0.0, 1.0, "minimum", ""
		if x, ok := ruleOf[a.ProductID]; ok {
			if !x.Active {
				continue
			}
			min, max, mult, source, forced = x.MinQty, x.MaxQty, x.Multiple, "rule", x.SupplierID
		} else {
			if min <= 0 {
				continue
			}
			if rq[a.ProductID] > 0 {
				max = min + rq[a.ProductID]
			}
		}
		qty := ReorderQty(position, min, max, mult)
		if qty <= 0 {
			continue
		}
		p := Proposal{ProductID: a.ProductID, ProductName: a.Name, SKU: a.SKU, Position: round3(position), OnHand: a.OnHand, Incoming: round3(a.Incoming + a.InTransit),
			Reserved: a.Reserved, MinQty: min, Qty: qty, Source: source}
		sup := forced
		if sup == "" {
			if sup, err = r.bestSupplier(ctx, tx, orgID, a.ProductID, qty); err != nil {
				return run, err
			}
		}
		if sup == "" {
			run.Unassigned = append(run.Unassigned, p)
			continue
		}
		p.SupplierID = sup
		_ = tx.QueryRow(ctx, `SELECT name FROM adrient.inv_suppliers WHERE org_id=$1 AND id=$2`, orgID, sup).Scan(&p.SupplierName)
		if p.UnitCost, p.LeadDays, err = r.priceFor(ctx, tx, orgID, sup, a.ProductID, qty); err != nil {
			return run, err
		}
		// Respect the supplier pack size from the default supplier record.
		var pack float64
		if err := tx.QueryRow(ctx, `SELECT pack_size::float8 FROM adrient.inv_product_suppliers WHERE product_id=$1 AND supplier_id=$2`, a.ProductID, sup).Scan(&pack); err == nil && pack > 1 {
			p.Qty = round3(RoundUpToMultiple(p.Qty, pack))
		}
		run.Proposals = append(run.Proposals, p)
	}
	if !create {
		return run, nil
	}
	const marker = "Auto-created by reorder rules"
	bySupplier := map[string][]Proposal{}
	var order []string
	for _, p := range run.Proposals {
		if _, ok := bySupplier[p.SupplierID]; !ok {
			order = append(order, p.SupplierID)
		}
		bySupplier[p.SupplierID] = append(bySupplier[p.SupplierID], p)
	}
	var touched []string
	for _, sup := range order {
		var poID string
		err := tx.QueryRow(ctx,
			`SELECT id::text FROM adrient.inv_purchase_orders WHERE org_id=$1 AND supplier_id=$2 AND state='draft' AND notes LIKE $3 ORDER BY created_at DESC LIMIT 1`,
			orgID, sup, marker+"%").Scan(&poID)
		var existing map[string]bool
		if err == nil {
			existing = map[string]bool{}
			rs, err := tx.Query(ctx, `SELECT product_id::text FROM adrient.inv_purchase_order_lines WHERE po_id=$1`, poID)
			if err != nil {
				return run, err
			}
			for rs.Next() {
				var pid string
				if rs.Scan(&pid) == nil {
					existing[pid] = true
				}
			}
			rs.Close()
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return run, err
		}
		var fresh []Proposal
		for _, p := range bySupplier[sup] {
			if existing[p.ProductID] {
				run.Skipped = append(run.Skipped, p)
			} else {
				fresh = append(fresh, p)
			}
		}
		if len(fresh) == 0 {
			continue
		}
		if poID == "" {
			in := POInput{SupplierID: sup, Notes: marker + " on " + time.Now().Format("2006-01-02")}
			lead := 0
			for _, p := range fresh {
				cost := p.UnitCost
				in.Lines = append(in.Lines, struct {
					ProductID   string   `json:"product_id"`
					Description string   `json:"description"`
					Qty         float64  `json:"qty"`
					UnitCost    *float64 `json:"unit_cost"`
					TaxPct      float64  `json:"tax_pct"`
				}{ProductID: p.ProductID, Qty: p.Qty, UnitCost: &cost})
				lead = int(math.Max(float64(lead), float64(p.LeadDays)))
			}
			if lead > 0 {
				in.ExpectedDate = time.Now().AddDate(0, 0, lead).Format("2006-01-02")
			}
			if poID, err = r.createPOTx(ctx, tx, orgID, userID, in); err != nil {
				return run, err
			}
		} else {
			var n int
			_ = tx.QueryRow(ctx, `SELECT count(*) FROM adrient.inv_purchase_order_lines WHERE po_id=$1`, poID).Scan(&n)
			for i, p := range fresh {
				if _, err := tx.Exec(ctx,
					`INSERT INTO adrient.inv_purchase_order_lines (po_id, org_id, product_id, qty, unit_cost, sort) VALUES ($1,$2,$3,$4,$5,$6)`,
					poID, orgID, p.ProductID, p.Qty, round4(p.UnitCost), n+i); err != nil {
					return run, err
				}
			}
			if _, err := tx.Exec(ctx,
				`UPDATE adrient.inv_purchase_orders o SET subtotal=s.sub, tax_total=s.tax, total=s.sub+s.tax, updated_at=now()
				 FROM (SELECT round(sum(qty*unit_cost),2) sub, round(sum(qty*unit_cost*tax_pct/100),2) tax FROM adrient.inv_purchase_order_lines WHERE po_id=$1) s WHERE o.id=$1`, poID); err != nil {
				return run, err
			}
		}
		touched = append(touched, poID)
	}
	if err := tx.Commit(ctx); err != nil {
		return run, err
	}
	for _, id := range touched {
		o, err := r.GetPO(ctx, orgID, id)
		if err != nil {
			return run, err
		}
		run.Created = append(run.Created, o)
	}
	return run, nil
}

// ------------------------------------------------------ supplier performance

// SupplierScore summarises how a supplier delivers.
type SupplierScore struct {
	SupplierID    string  `json:"supplier_id"`
	SupplierName  string  `json:"supplier_name"`
	Orders        int     `json:"orders"`
	Receipts      int     `json:"receipts"`
	OnTimePct     float64 `json:"on_time_pct"` // receipts on or before the promised/expected date; -1 when no dates
	FillRatePct   float64 `json:"fill_rate_pct"`
	AvgLeadDays   float64 `json:"avg_lead_days"`
	ReturnRatePct float64 `json:"return_rate_pct"`
	Spend         float64 `json:"spend"`          // received at order price
	PriceVariance float64 `json:"price_variance"` // billed minus ordered price, over billed quantities
	Score         float64 `json:"score"`
}

// SupplierPerformance scores suppliers over a period (dates inclusive, empty = all time).
func (r *Repository) SupplierPerformance(ctx context.Context, orgID, from, to string) ([]SupplierScore, error) {
	f, err := parseDate(from)
	if err != nil {
		return nil, err
	}
	t, err := parseDate(to)
	if err != nil {
		return nil, err
	}
	var tEnd *time.Time
	if t != nil {
		e := t.AddDate(0, 0, 1)
		tEnd = &e
	}
	rows, err := r.pool.Query(ctx,
		`SELECT s.id::text, s.name,
		   (SELECT count(*) FROM adrient.inv_purchase_orders o WHERE o.org_id=$1 AND o.supplier_id=s.id AND o.state<>'draft' AND o.state<>'cancelled'
		      AND ($2::date IS NULL OR o.order_date >= $2) AND ($3::date IS NULL OR o.order_date < $3))::int,
		   (SELECT count(*) FROM adrient.inv_grns g WHERE g.org_id=$1 AND g.supplier_id=s.id AND ($2::date IS NULL OR g.received_at >= $2) AND ($3::date IS NULL OR g.received_at < $3))::int,
		   (SELECT count(*) FROM adrient.inv_grns g JOIN adrient.inv_purchase_orders o ON o.id=g.po_id
		      WHERE g.org_id=$1 AND g.supplier_id=s.id AND COALESCE(o.promised_date,o.expected_date) IS NOT NULL
		      AND ($2::date IS NULL OR g.received_at >= $2) AND ($3::date IS NULL OR g.received_at < $3))::int,
		   (SELECT count(*) FROM adrient.inv_grns g JOIN adrient.inv_purchase_orders o ON o.id=g.po_id
		      WHERE g.org_id=$1 AND g.supplier_id=s.id AND COALESCE(o.promised_date,o.expected_date) IS NOT NULL
		      AND g.received_at::date <= COALESCE(o.promised_date,o.expected_date)
		      AND ($2::date IS NULL OR g.received_at >= $2) AND ($3::date IS NULL OR g.received_at < $3))::int,
		   COALESCE((SELECT sum(l.qty) FROM adrient.inv_purchase_order_lines l JOIN adrient.inv_purchase_orders o ON o.id=l.po_id
		      WHERE o.org_id=$1 AND o.supplier_id=s.id AND o.state IN ('partial','received','closed')
		      AND ($2::date IS NULL OR o.order_date >= $2) AND ($3::date IS NULL OR o.order_date < $3)),0)::float8,
		   COALESCE((SELECT sum(l.qty_received) FROM adrient.inv_purchase_order_lines l JOIN adrient.inv_purchase_orders o ON o.id=l.po_id
		      WHERE o.org_id=$1 AND o.supplier_id=s.id AND o.state IN ('partial','received','closed')
		      AND ($2::date IS NULL OR o.order_date >= $2) AND ($3::date IS NULL OR o.order_date < $3)),0)::float8,
		   COALESCE((SELECT sum(l.qty_returned) FROM adrient.inv_purchase_order_lines l JOIN adrient.inv_purchase_orders o ON o.id=l.po_id
		      WHERE o.org_id=$1 AND o.supplier_id=s.id AND ($2::date IS NULL OR o.order_date >= $2) AND ($3::date IS NULL OR o.order_date < $3)),0)::float8,
		   COALESCE((SELECT sum(l.qty_received*l.unit_cost) FROM adrient.inv_purchase_order_lines l JOIN adrient.inv_purchase_orders o ON o.id=l.po_id
		      WHERE o.org_id=$1 AND o.supplier_id=s.id AND ($2::date IS NULL OR o.order_date >= $2) AND ($3::date IS NULL OR o.order_date < $3)),0)::float8,
		   COALESCE((SELECT avg(first.d) FROM (SELECT (min(g.received_at)::date - o.order_date) d FROM adrient.inv_grns g JOIN adrient.inv_purchase_orders o ON o.id=g.po_id
		      WHERE g.org_id=$1 AND g.supplier_id=s.id AND ($2::date IS NULL OR o.order_date >= $2) AND ($3::date IS NULL OR o.order_date < $3) GROUP BY o.id) first),0)::float8,
		   COALESCE((SELECT sum((b.unit_cost - l.unit_cost) * b.qty) FROM adrient.inv_supplier_bill_lines b JOIN adrient.inv_supplier_bills sb ON sb.id=b.bill_id
		      JOIN adrient.inv_purchase_order_lines l ON l.id=b.po_line_id WHERE sb.org_id=$1 AND sb.supplier_id=s.id AND sb.state='posted'
		      AND ($2::date IS NULL OR sb.bill_date >= $2) AND ($3::date IS NULL OR sb.bill_date < $3)),0)::float8
		 FROM adrient.inv_suppliers s WHERE s.org_id=$1 ORDER BY s.name`, orgID, f, tEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SupplierScore{}
	for rows.Next() {
		var x SupplierScore
		var dated, onTime int
		var ordered, got, ret float64
		if err := rows.Scan(&x.SupplierID, &x.SupplierName, &x.Orders, &x.Receipts, &dated, &onTime, &ordered, &got, &ret, &x.Spend, &x.AvgLeadDays, &x.PriceVariance); err != nil {
			return nil, err
		}
		if x.Orders == 0 && x.Receipts == 0 {
			continue
		}
		x.OnTimePct = -1
		if dated > 0 {
			x.OnTimePct = round2(float64(onTime) * 100 / float64(dated))
		}
		if ordered > 0 {
			x.FillRatePct = round2(math.Min(got, ordered) * 100 / ordered)
		}
		if got > 0 {
			x.ReturnRatePct = round2(ret * 100 / got)
		}
		x.Spend, x.PriceVariance, x.AvgLeadDays = round2(x.Spend), round2(x.PriceVariance), round2(x.AvgLeadDays)
		ot := x.OnTimePct
		if ot < 0 {
			ot = x.FillRatePct
		}
		x.Score = round2(0.5*ot + 0.3*x.FillRatePct + 0.2*(100-x.ReturnRatePct))
		out = append(out, x)
	}
	return out, rows.Err()
}

func lower(s string) string { return strings.ToLower(s) }
