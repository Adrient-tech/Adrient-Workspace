package inventory

// Stock counts (cycle and full) with variance review and approval, plus the
// planning reports: ABC analysis, days of cover with a moving-average demand
// forecast, and dead stock.

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// CountLine is one product in a count.
type CountLine struct {
	ID          string   `json:"id"`
	ProductID   string   `json:"product_id"`
	ProductName string   `json:"product_name"`
	SKU         string   `json:"sku"`
	ShelfCode   string   `json:"shelf_code"`
	Expected    float64  `json:"expected_qty"`
	Counted     *float64 `json:"counted_qty"`
	UnitCost    float64  `json:"unit_cost"`
	Variance    float64  `json:"variance"`
	VarianceVal float64  `json:"variance_value"`
	Accepted    bool     `json:"accepted"`
	Note        string   `json:"note"`
}

// Count is a stock count session.
type Count struct {
	ID          string      `json:"id"`
	Number      string      `json:"number"`
	Kind        string      `json:"kind"`
	LocationID  string      `json:"location_id"`
	Location    string      `json:"location"`
	Category    string      `json:"category"`
	State       string      `json:"state"`
	Reason      string      `json:"reason"`
	Note        string      `json:"note"`
	CreatedAt   time.Time   `json:"created_at"`
	ApprovedAt  *time.Time  `json:"approved_at"`
	Lines       []CountLine `json:"lines"`
	Counted     int         `json:"counted"`
	Total       int         `json:"total"`
	NetVariance float64     `json:"net_variance_value"`
}

// CountInput starts a count of a location (subtree), optionally one category
// or an ABC class (cycle counts).
type CountInput struct {
	Kind       string `json:"kind"` // cycle | full
	LocationID string `json:"location_id"`
	Category   string `json:"category"`
	ABC        string `json:"abc"` // A | B | C to count only that class (cycle)
	Note       string `json:"note"`
}

// StartCount snapshots expected quantities and opens a count.
func (r *Repository) StartCount(ctx context.Context, orgID, userID string, in CountInput) (Count, error) {
	if in.Kind != "cycle" && in.Kind != "full" {
		return Count{}, invalid("kind must be cycle or full")
	}
	if in.LocationID == "" {
		return Count{}, invalid("pick the location to count")
	}
	locs, err := r.SubtreeIDs(ctx, orgID, in.LocationID)
	if err != nil {
		return Count{}, err
	}
	if len(locs) == 0 {
		return Count{}, invalid("location not found")
	}
	var abc map[string]string
	if in.ABC != "" {
		rows, err := r.ABCAnalysis(ctx, orgID, 365)
		if err != nil {
			return Count{}, err
		}
		abc = map[string]string{}
		for _, x := range rows {
			abc[x.ProductID] = x.Class
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Count{}, err
	}
	defer tx.Rollback(ctx)
	num, err := nextNumber(ctx, tx, orgID, "count", "CNT")
	if err != nil {
		return Count{}, err
	}
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_counts (org_id, number, kind, location_id, category, note, created_by) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid) RETURNING id::text`,
		orgID, num, in.Kind, in.LocationID, in.Category, in.Note, userID).Scan(&id); err != nil {
		return Count{}, err
	}
	rows, err := tx.Query(ctx,
		`SELECT p.id::text, COALESCE(sum(s.qty) FILTER (WHERE s.location_id = ANY($2::uuid[])),0)::float8,
		        CASE WHEN COALESCE(sum(s.qty) FILTER (WHERE s.location_id = ANY($2::uuid[])),0) > 0 THEN p.cost::float8 ELSE p.cost::float8 END
		 FROM adrient.inv_products p LEFT JOIN adrient.inv_stock s ON s.product_id=p.id
		 WHERE p.org_id=$1 AND NOT p.archived AND ($3='' OR p.category=$3)
		   AND NOT EXISTS (SELECT 1 FROM adrient.inv_products c WHERE c.parent_id=p.id AND NOT c.archived)
		   AND NOT EXISTS (SELECT 1 FROM adrient.inv_boms b WHERE b.product_id=p.id AND b.kind='kit') AND p.tracking <> 'serial'
		 GROUP BY p.id`, orgID, locs, in.Category)
	if err != nil {
		return Count{}, err
	}
	type pl struct {
		id        string
		qty, cost float64
	}
	var picks []pl
	for rows.Next() {
		var x pl
		if err := rows.Scan(&x.id, &x.qty, &x.cost); err != nil {
			rows.Close()
			return Count{}, err
		}
		picks = append(picks, x)
	}
	rows.Close()
	n := 0
	for _, x := range picks {
		if in.Kind == "cycle" && x.qty == 0 && in.ABC == "" && in.Category == "" {
			continue // an open-ended cycle count only lists what is expected to be there
		}
		if in.Kind == "full" && x.qty == 0 {
			// a full count still lists items with a shelf record here so a surprise find is possible
			var has bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.inv_stock WHERE product_id=$1 AND location_id = ANY($2::uuid[]))`, x.id, locs).Scan(&has); err != nil {
				return Count{}, err
			}
			if !has {
				continue
			}
		}
		if abc != nil && abc[x.id] != in.ABC {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.inv_count_lines (count_id, org_id, product_id, expected_qty, unit_cost) VALUES ($1,$2,$3,$4,$5)`, id, orgID, x.id, x.qty, x.cost); err != nil {
			return Count{}, err
		}
		n++
	}
	if n == 0 {
		return Count{}, invalid("nothing to count with those filters")
	}
	if err := tx.Commit(ctx); err != nil {
		return Count{}, err
	}
	return r.GetCount(ctx, orgID, id)
}

// GetCount loads a count with lines and variances.
func (r *Repository) GetCount(ctx context.Context, orgID, id string) (Count, error) {
	cs, err := r.ListCounts(ctx, orgID, id)
	if err != nil {
		return Count{}, err
	}
	if len(cs) == 0 {
		return Count{}, ErrNotFound
	}
	return cs[0], nil
}

// ListCounts lists counts (lines are only loaded for a single count).
func (r *Repository) ListCounts(ctx context.Context, orgID, onlyID string) ([]Count, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.id::text, c.number, c.kind, c.location_id::text, l.name, c.category, c.state, c.reason, c.note, c.created_at, c.approved_at,
		        (SELECT count(*) FROM adrient.inv_count_lines x WHERE x.count_id=c.id)::int,
		        (SELECT count(*) FROM adrient.inv_count_lines x WHERE x.count_id=c.id AND x.counted_qty IS NOT NULL)::int,
		        COALESCE((SELECT sum((x.counted_qty-x.expected_qty)*x.unit_cost) FROM adrient.inv_count_lines x WHERE x.count_id=c.id AND x.counted_qty IS NOT NULL AND x.accepted),0)::float8
		 FROM adrient.inv_counts c JOIN adrient.inv_locations l ON l.id=c.location_id
		 WHERE c.org_id=$1 AND ($2='' OR c.id::text=$2) ORDER BY c.created_at DESC LIMIT 200`, orgID, onlyID)
	if err != nil {
		return nil, err
	}
	out := []Count{}
	for rows.Next() {
		var c Count
		if err := rows.Scan(&c.ID, &c.Number, &c.Kind, &c.LocationID, &c.Location, &c.Category, &c.State, &c.Reason, &c.Note, &c.CreatedAt, &c.ApprovedAt, &c.Total, &c.Counted, &c.NetVariance); err != nil {
			rows.Close()
			return nil, err
		}
		c.NetVariance = round2(c.NetVariance)
		c.Lines = []CountLine{}
		out = append(out, c)
	}
	rows.Close()
	if onlyID == "" || len(out) != 1 {
		return out, nil
	}
	lr, err := r.pool.Query(ctx,
		`SELECT x.id::text, x.product_id::text, p.name || CASE WHEN p.variant_label<>'' THEN ' / '||p.variant_label ELSE '' END, p.sku,
		        COALESCE((SELECT string_agg(DISTINCT s.shelf_code, ', ') FROM adrient.inv_stock s WHERE s.product_id=x.product_id AND s.shelf_code<>''),''),
		        x.expected_qty::float8, x.counted_qty::float8, x.unit_cost::float8, x.accepted, x.note
		 FROM adrient.inv_count_lines x JOIN adrient.inv_products p ON p.id=x.product_id WHERE x.count_id=$1 ORDER BY p.name`, onlyID)
	if err != nil {
		return nil, err
	}
	defer lr.Close()
	for lr.Next() {
		var l CountLine
		if err := lr.Scan(&l.ID, &l.ProductID, &l.ProductName, &l.SKU, &l.ShelfCode, &l.Expected, &l.Counted, &l.UnitCost, &l.Accepted, &l.Note); err != nil {
			return nil, err
		}
		if l.Counted != nil {
			l.Variance = round3(*l.Counted - l.Expected)
			l.VarianceVal = round2(l.Variance * l.UnitCost)
		}
		out[0].Lines = append(out[0].Lines, l)
	}
	return out, lr.Err()
}

// CountEntry records one counted quantity.
type CountEntry struct {
	LineID    string   `json:"line_id"`
	ProductID string   `json:"product_id"` // alternative to line_id (scanning)
	Counted   *float64 `json:"counted_qty"`
	Accepted  *bool    `json:"accepted"`
	Note      *string  `json:"note"`
}

// RecordCount stores counted quantities while a count is open or in review.
func (r *Repository) RecordCount(ctx context.Context, orgID, id string, entries []CountEntry) (Count, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Count{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM adrient.inv_counts WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&state); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Count{}, ErrNotFound
		}
		return Count{}, err
	}
	if state != "counting" && state != "review" {
		return Count{}, invalid("this count is %s", state)
	}
	for _, e := range entries {
		if e.Counted != nil && *e.Counted < 0 {
			return Count{}, invalid("counted quantity cannot be negative")
		}
		tag, err := tx.Exec(ctx,
			`UPDATE adrient.inv_count_lines SET counted_qty=COALESCE($4, counted_qty), accepted=COALESCE($5, accepted), note=COALESCE($6, note)
			 WHERE count_id=$1 AND org_id=$2 AND (id::text=$3 OR ($3='' AND product_id::text=$7))`,
			id, orgID, e.LineID, e.Counted, e.Accepted, e.Note, e.ProductID)
		if err != nil {
			return Count{}, err
		}
		if tag.RowsAffected() == 0 {
			return Count{}, invalid("that product is not part of this count")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Count{}, err
	}
	return r.GetCount(ctx, orgID, id)
}

// SubmitCount moves a count to review (variances become visible for approval).
func (r *Repository) SubmitCount(ctx context.Context, orgID, id string) (Count, error) {
	var missing int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.inv_count_lines WHERE count_id=$1 AND org_id=$2 AND counted_qty IS NULL`, id, orgID).Scan(&missing); err != nil {
		return Count{}, err
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.inv_counts SET state='review' WHERE org_id=$1 AND id=$2 AND state='counting'`, orgID, id)
	if err != nil {
		return Count{}, err
	}
	if tag.RowsAffected() == 0 {
		return Count{}, invalid("only an open count can be submitted for review")
	}
	return r.GetCount(ctx, orgID, id)
}

// ApproveCount posts the accepted variances as stock adjustments (reason "count").
// The adjustment is counted minus the quantity expected when the count started,
// so stock that moved after the snapshot is not overwritten. Uncounted lines are
// skipped. A reason is required.
func (r *Repository) ApproveCount(ctx context.Context, orgID, userID, id, reason string) (Count, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Count{}, invalid("give a reason for the adjustments (for example 'annual count')")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Count{}, err
	}
	defer tx.Rollback(ctx)
	var state, number, loc string
	if err := tx.QueryRow(ctx, `SELECT state, number, location_id::text FROM adrient.inv_counts WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&state, &number, &loc); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Count{}, ErrNotFound
		}
		return Count{}, err
	}
	if state != "review" {
		return Count{}, invalid("submit the count for review before approving it")
	}
	rows, err := tx.Query(ctx, `SELECT product_id::text, expected_qty::float8, counted_qty::float8, note FROM adrient.inv_count_lines WHERE count_id=$1 AND accepted AND counted_qty IS NOT NULL ORDER BY id`, id)
	if err != nil {
		return Count{}, err
	}
	type adj struct {
		product, note string
		delta         float64
	}
	var list []adj
	for rows.Next() {
		var p, n string
		var e, c float64
		if err := rows.Scan(&p, &e, &c, &n); err != nil {
			rows.Close()
			return Count{}, err
		}
		if d := round3(c - e); d != 0 {
			list = append(list, adj{p, n, d})
		}
	}
	rows.Close()
	for _, a := range list {
		note := "Count " + number + ": " + reason
		if a.note != "" {
			note += " (" + a.note + ")"
		}
		in := AdjustInput{LocationID: loc, Qty: a.delta, Reason: "count", Ref: number, Note: note}
		// Counts take place at the counted location; spread over its subtree is not
		// attempted: the variance is booked at the location the count was opened for.
		if _, err := r.applyAdjust(ctx, tx, orgID, userID, a.product, in); err != nil {
			return Count{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.inv_counts SET state='approved', reason=$3, approved_by=NULLIF($4,'')::uuid, approved_at=now() WHERE org_id=$1 AND id=$2`, orgID, id, reason, userID); err != nil {
		return Count{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Count{}, err
	}
	return r.GetCount(ctx, orgID, id)
}

// CancelCount drops an open or in-review count.
func (r *Repository) CancelCount(ctx context.Context, orgID, id string) (Count, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.inv_counts SET state='cancelled' WHERE org_id=$1 AND id=$2 AND state IN ('counting','review')`, orgID, id)
	if err != nil {
		return Count{}, err
	}
	if tag.RowsAffected() == 0 {
		return Count{}, invalid("this count can no longer be cancelled")
	}
	return r.GetCount(ctx, orgID, id)
}

// ------------------------------------------------------------- planning

// ABCRow classifies a product by its share of consumption value.
type ABCRow struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	SKU       string  `json:"sku"`
	Units     float64 `json:"units"`
	Value     float64 `json:"value"`
	SharePct  float64 `json:"share_pct"`
	CumPct    float64 `json:"cum_pct"`
	Class     string  `json:"class"`
}

// ABCAnalysis ranks products by the value that left stock in the last `days`
// days (sales, from the value ledger).
func (r *Repository) ABCAnalysis(ctx context.Context, orgID string, days int) ([]ABCRow, error) {
	if days <= 0 || days > 3650 {
		days = 365
	}
	rows, err := r.pool.Query(ctx,
		`SELECT p.id::text, p.name || CASE WHEN p.variant_label<>'' THEN ' / '||p.variant_label ELSE '' END, p.sku,
		        COALESCE(sum(-v.qty) FILTER (WHERE v.reason='sale' AND v.ts >= now() - make_interval(days => $2)),0)::float8,
		        COALESCE(sum(-v.value) FILTER (WHERE v.reason='sale' AND v.ts >= now() - make_interval(days => $2)),0)::float8
		 FROM adrient.inv_products p LEFT JOIN adrient.inv_value_ledger v ON v.product_id=p.id
		 WHERE p.org_id=$1 AND NOT p.archived AND NOT EXISTS (SELECT 1 FROM adrient.inv_products c WHERE c.parent_id=p.id AND NOT c.archived)
		 GROUP BY p.id ORDER BY 5 DESC, p.name`, orgID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ABCRow{}
	var vals []float64
	var total float64
	for rows.Next() {
		var x ABCRow
		if err := rows.Scan(&x.ProductID, &x.Name, &x.SKU, &x.Units, &x.Value); err != nil {
			return nil, err
		}
		x.Value = round2(x.Value)
		out = append(out, x)
		vals = append(vals, x.Value)
		total += x.Value
	}
	cls := ABCClass(vals)
	var cum float64
	for i := range out {
		out[i].Class = cls[i]
		if total > 0 {
			out[i].SharePct = round2(out[i].Value * 100 / total)
			cum += out[i].Value
			out[i].CumPct = round2(cum * 100 / total)
		}
	}
	return out, rows.Err()
}

// CoverRow is the demand outlook of one product.
type CoverRow struct {
	ProductID   string    `json:"product_id"`
	Name        string    `json:"name"`
	SKU         string    `json:"sku"`
	OnHand      float64   `json:"on_hand"`
	Available   float64   `json:"available"`
	Incoming    float64   `json:"incoming"`
	DailyDemand float64   `json:"daily_demand"`
	DaysCover   float64   `json:"days_cover"` // -1: no demand
	WeeklySales []float64 `json:"weekly_sales"`
	Forecast4w  float64   `json:"forecast_4w"` // moving-average forecast for the next 4 weeks
	Status      string    `json:"status"`      // out | low | ok | overstock | no_demand
	LeadDays    int       `json:"lead_days"`
}

// DaysOfCoverReport computes demand as a moving average of weekly sales over
// `weeks` weeks (default 8, average of the last 4) and days of cover from it.
func (r *Repository) DaysOfCoverReport(ctx context.Context, orgID string, weeks int) ([]CoverRow, error) {
	if weeks <= 0 || weeks > 52 {
		weeks = 8
	}
	av, err := r.ListAvailability(ctx, orgID, "", "")
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT product_id::text, GREATEST(0, LEAST($2-1, floor(extract(epoch FROM (now() - ts)) / 604800)::int)), sum(-qty)::float8
		 FROM adrient.inv_value_ledger WHERE org_id=$1 AND reason='sale' AND ts >= now() - make_interval(weeks => $2) GROUP BY 1,2`, orgID, weeks)
	if err != nil {
		return nil, err
	}
	sales := map[string][]float64{}
	for rows.Next() {
		var pid string
		var b int
		var q float64
		if err := rows.Scan(&pid, &b, &q); err != nil {
			rows.Close()
			return nil, err
		}
		if sales[pid] == nil {
			sales[pid] = make([]float64, weeks)
		}
		sales[pid][weeks-1-b] += q // oldest first
	}
	rows.Close()
	lead := map[string]int{}
	lr, err := r.pool.Query(ctx, `SELECT product_id::text, min(lead_days) FILTER (WHERE lead_days>0) FROM adrient.inv_product_suppliers WHERE org_id=$1 GROUP BY 1`, orgID)
	if err == nil {
		for lr.Next() {
			var pid string
			var d *int
			if lr.Scan(&pid, &d) == nil && d != nil {
				lead[pid] = *d
			}
		}
		lr.Close()
	}
	out := []CoverRow{}
	for _, a := range av {
		w := sales[a.ProductID]
		if w == nil {
			w = make([]float64, weeks)
		}
		ma := MovingAverage(w, 4)
		x := CoverRow{ProductID: a.ProductID, Name: a.Name, SKU: a.SKU, OnHand: a.OnHand, Available: a.Available, Incoming: a.Incoming + a.InTransit,
			WeeklySales: w, DailyDemand: round3(ma / 7), Forecast4w: round3(ma * 4), LeadDays: lead[a.ProductID]}
		x.DaysCover = DaysOfCover(a.Available, ma/7)
		switch {
		case x.DaysCover < 0:
			x.Status = "no_demand"
			if a.OnHand <= 0 {
				x.Status = "out"
			}
		case a.Available <= 0:
			x.Status = "out"
		case x.DaysCover < float64(maxInt(x.LeadDays, 7)):
			x.Status = "low"
		case x.DaysCover > 180:
			x.Status = "overstock"
		default:
			x.Status = "ok"
		}
		out = append(out, x)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ci, cj := out[i].DaysCover, out[j].DaysCover
		if ci < 0 {
			ci = 1e9
		}
		if cj < 0 {
			cj = 1e9
		}
		return ci < cj
	})
	return out, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// DeadRow is stock that has not moved out for a long time.
type DeadRow struct {
	ProductID string     `json:"product_id"`
	Name      string     `json:"name"`
	SKU       string     `json:"sku"`
	OnHand    float64    `json:"on_hand"`
	Value     float64    `json:"value"`
	LastOut   *time.Time `json:"last_out"`
	DaysIdle  int        `json:"days_idle"`
}

// DeadStock lists products with stock that had no outward movement (sale,
// assembly use, vendor return) for `days` days, most valuable first.
func (r *Repository) DeadStock(ctx context.Context, orgID string, days int) ([]DeadRow, error) {
	if days <= 0 {
		days = 90
	}
	rows, err := r.pool.Query(ctx,
		`SELECT p.id::text, p.name || CASE WHEN p.variant_label<>'' THEN ' / '||p.variant_label ELSE '' END, p.sku,
		        q.qty::float8, COALESCE(v.val,0)::float8, o.last_out, p.created_at
		 FROM adrient.inv_products p
		 JOIN (SELECT product_id, sum(qty) qty FROM adrient.inv_stock WHERE org_id=$1 GROUP BY product_id HAVING sum(qty) > 0) q ON q.product_id=p.id
		 LEFT JOIN (SELECT product_id, sum(value) val FROM adrient.inv_value_ledger WHERE org_id=$1 GROUP BY product_id) v ON v.product_id=p.id
		 LEFT JOIN (SELECT product_id, max(created_at) last_out FROM adrient.inv_history WHERE org_id=$1 AND delta < 0
		            AND reason IN ('sale','assembly_out','vendor_return') GROUP BY product_id) o ON o.product_id=p.id
		 WHERE p.org_id=$1 AND NOT p.archived AND COALESCE(o.last_out, p.created_at) < now() - make_interval(days => $2)
		 ORDER BY COALESCE(v.val,0) DESC`, orgID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeadRow{}
	for rows.Next() {
		var x DeadRow
		var created time.Time
		if err := rows.Scan(&x.ProductID, &x.Name, &x.SKU, &x.OnHand, &x.Value, &x.LastOut, &created); err != nil {
			return nil, err
		}
		ref := created
		if x.LastOut != nil {
			ref = *x.LastOut
		}
		x.DaysIdle = int(time.Since(ref).Hours() / 24)
		x.Value = round2(x.Value)
		out = append(out, x)
	}
	return out, rows.Err()
}
