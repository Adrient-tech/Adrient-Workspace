// Shrinkage and margin reports, valued from the Adrient cost ledger instead of the current cost.

package inventory

import (
	"context"
	"sort"
	"strings"
)

// LossReasons are the reasons a write-off can carry. "internal" (used in-house) is not shrinkage.
var LossReasons = map[string]bool{"damaged": true, "expired": true, "lost": true, "internal": true}

func isShrinkage(reason string) bool {
	return reason == "damaged" || reason == "expired" || reason == "lost"
}

// WriteOffInput removes stock for a stated reason.
type WriteOffInput struct {
	ProductID  string  `json:"product_id"`
	LocationID string  `json:"location_id"`
	Qty        float64 `json:"qty"`
	LossReason string  `json:"loss_reason"`
	Note       string  `json:"note"`
}

// WriteOff takes stock out with a loss reason. It is a normal "remove": stock cannot go below
// zero, reservations are respected, and Accounting receives the stock adjustment at cost.
func (r *Repository) WriteOff(ctx context.Context, orgID, userID string, in WriteOffInput) (Product, error) {
	reason := strings.ToLower(strings.TrimSpace(in.LossReason))
	if !LossReasons[reason] {
		return Product{}, invalid("choose why the stock is leaving: damaged, expired, lost or internal use")
	}
	if in.Qty <= 0 {
		return Product{}, invalid("quantity must be above zero")
	}
	if in.ProductID == "" {
		return Product{}, invalid("choose a product")
	}
	return r.AdjustStock(ctx, orgID, userID, in.ProductID, AdjustInput{LocationID: in.LocationID, Qty: in.Qty, Reason: "remove",
		Note: strings.TrimSpace(in.Note), Ref: "loss:" + reason, LossReason: reason})
}

// ShrinkRow is what one product lost in the window.
type ShrinkRow struct {
	ProductID string             `json:"product_id"`
	Name      string             `json:"name"`
	SKU       string             `json:"sku"`
	Units     float64            `json:"units"`
	Value     float64            `json:"value"`
	Reasons   map[string]float64 `json:"reasons"`
}

// ShrinkageReport lists losses (damaged, expired, lost) by product, biggest value first.
// Internal use is counted separately because it is not a loss.
type ShrinkageReport struct {
	Days        int         `json:"days"`
	Rows        []ShrinkRow `json:"rows"`
	TotalUnits  float64     `json:"total_units"`
	TotalValue  float64     `json:"total_value"`
	InternalUse float64     `json:"internal_use_units"`
	// Unspecified counts plain removals with no loss reason: they are invisible here.
	Unspecified int `json:"unspecified_removals"`
}

// Shrinkage adds up write-offs of the last `days` days. Value comes from the cost ledger (the
// cost the stock was actually carried at), not today's product cost.
func (r *Repository) Shrinkage(ctx context.Context, orgID string, days int) (ShrinkageReport, error) {
	if days <= 0 {
		days = 30
	}
	rep := ShrinkageReport{Days: days, Rows: []ShrinkRow{}}
	rows, err := r.pool.Query(ctx,
		`SELECT h.product_id::text, p.name || CASE WHEN p.variant_label<>'' THEN ' / '||p.variant_label ELSE '' END, p.sku, h.loss_reason, sum(-h.delta)::float8
		 FROM adrient.inv_history h JOIN adrient.inv_products p ON p.id=h.product_id
		 WHERE h.org_id=$1 AND h.reason='remove' AND h.loss_reason<>'' AND h.created_at >= now() - make_interval(days => $2)
		 GROUP BY 1,2,3,4`, orgID, days)
	if err != nil {
		return rep, err
	}
	idx := map[string]*ShrinkRow{}
	for rows.Next() {
		var id, name, sku, reason string
		var units float64
		if err := rows.Scan(&id, &name, &sku, &reason, &units); err != nil {
			rows.Close()
			return rep, err
		}
		if reason == "internal" {
			rep.InternalUse += units
			continue
		}
		if !isShrinkage(reason) {
			continue
		}
		row := idx[id]
		if row == nil {
			row = &ShrinkRow{ProductID: id, Name: name, SKU: sku, Reasons: map[string]float64{}}
			idx[id] = row
		}
		row.Units += units
		row.Reasons[reason] += units
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return rep, err
	}
	vrows, err := r.pool.Query(ctx,
		`SELECT product_id::text, sum(-value)::float8 FROM adrient.inv_value_ledger
		 WHERE org_id=$1 AND reason='remove' AND ref IN ('loss:damaged','loss:expired','loss:lost') AND ts >= now() - make_interval(days => $2)
		 GROUP BY 1`, orgID, days)
	if err != nil {
		return rep, err
	}
	for vrows.Next() {
		var id string
		var v float64
		if err := vrows.Scan(&id, &v); err != nil {
			vrows.Close()
			return rep, err
		}
		if row := idx[id]; row != nil {
			row.Value = v
		}
	}
	vrows.Close()
	if err := vrows.Err(); err != nil {
		return rep, err
	}
	for _, row := range idx {
		row.Units, row.Value = round3(row.Units), round2(row.Value)
		rep.TotalUnits += row.Units
		rep.TotalValue += row.Value
		rep.Rows = append(rep.Rows, *row)
	}
	sort.Slice(rep.Rows, func(i, j int) bool {
		if rep.Rows[i].Value != rep.Rows[j].Value {
			return rep.Rows[i].Value > rep.Rows[j].Value
		}
		return rep.Rows[i].Units > rep.Rows[j].Units
	})
	rep.TotalUnits, rep.TotalValue, rep.InternalUse = round3(rep.TotalUnits), round2(rep.TotalValue), round3(rep.InternalUse)
	err = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.inv_history WHERE org_id=$1 AND reason='remove' AND loss_reason='' AND delta<0
		AND ref NOT LIKE 'transfer%' AND created_at >= now() - make_interval(days => $2)`, orgID, days).Scan(&rep.Unspecified)
	return rep, err
}

// MarginRow is the selling margin of one product.
type MarginRow struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	SKU       string  `json:"sku"`
	Price     float64 `json:"price"`
	Cost      float64 `json:"cost"`
	Margin    float64 `json:"margin"`
	Percent   float64 `json:"percent"`
	BelowCost bool    `json:"below_cost"`
}

// MarginRows computes the margin of every product with both a price and a cost, thinnest first
// (so below-cost items head the list). Percent is margin over price, to one decimal.
func MarginRows(ps []Product) []MarginRow {
	out := []MarginRow{}
	for _, p := range ps {
		if p.Archived || p.Price <= 0 || p.Cost <= 0 {
			continue
		}
		m := round2(p.Price - p.Cost)
		pct := float64(int64((m/p.Price)*1000+signHalf(m))) / 10
		name := p.Name
		if p.VariantLabel != "" {
			name += " / " + p.VariantLabel
		}
		out = append(out, MarginRow{ProductID: p.ID, Name: name, SKU: p.SKU, Price: p.Price, Cost: p.Cost, Margin: m, Percent: pct, BelowCost: m < 0})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Percent < out[j].Percent })
	return out
}

func signHalf(f float64) float64 {
	if f < 0 {
		return -0.5
	}
	return 0.5
}

// MarginReport lists margins for all active products (variants as rows).
func (r *Repository) MarginReport(ctx context.Context, orgID string) ([]MarginRow, error) {
	ps, err := r.ListProducts(ctx, orgID, ProductFilter{Flat: true})
	if err != nil {
		return nil, err
	}
	return MarginRows(ps), nil
}
