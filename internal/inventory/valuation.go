package inventory

// Valuation reports, per-product stock settings and the hand-over of COGS and
// stock-adjustment entries to Accounting.

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// ProductSettings are the stock-control attributes of a product.
type ProductSettings struct {
	ProductID       string `json:"product_id"`
	ValuationMethod string `json:"valuation_method"` // avg | fifo
	Tracking        string `json:"tracking"`         // none | lot | serial
	ShelfLifeDays   int    `json:"shelf_life_days"`
}

// GetProductSettings reads stock settings.
func (r *Repository) GetProductSettings(ctx context.Context, orgID, id string) (ProductSettings, error) {
	s := ProductSettings{ProductID: id}
	err := r.pool.QueryRow(ctx, `SELECT valuation_method, tracking, shelf_life_days FROM adrient.inv_products WHERE org_id=$1 AND id=$2`, orgID, id).
		Scan(&s.ValuationMethod, &s.Tracking, &s.ShelfLifeDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

// SetProductSettings changes valuation method, tracking and shelf life. Turning
// lot tracking on moves existing stock into an UNASSIGNED lot; serial tracking
// can only be turned on while there is no stock.
func (r *Repository) SetProductSettings(ctx context.Context, orgID, id string, in ProductSettings) (ProductSettings, error) {
	if in.ValuationMethod != "avg" && in.ValuationMethod != "fifo" {
		return in, invalid("valuation must be avg or fifo")
	}
	if in.Tracking != "none" && in.Tracking != "lot" && in.Tracking != "serial" {
		return in, invalid("tracking must be none, lot or serial")
	}
	if in.ShelfLifeDays < 0 {
		return in, invalid("shelf life cannot be negative")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return in, err
	}
	defer tx.Rollback(ctx)
	var oldT string
	if err := tx.QueryRow(ctx, `SELECT tracking FROM adrient.inv_products WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&oldT); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return in, ErrNotFound
		}
		return in, err
	}
	if oldT != in.Tracking {
		var total, neg float64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(qty),0)::float8, COALESCE(sum(qty) FILTER (WHERE qty<0),0)::float8 FROM adrient.inv_stock WHERE product_id=$1`, id).Scan(&total, &neg); err != nil {
			return in, err
		}
		if neg < 0 && in.Tracking != "none" {
			return in, invalid("fix negative stock before turning on tracking")
		}
		switch {
		case in.Tracking == "serial" && total != 0:
			return in, invalid("serial tracking can only be turned on when the product has no stock; remove the stock first and receive it again with serial numbers")
		case oldT == "serial" && in.Tracking != "serial":
			return in, invalid("serial tracking cannot be turned off once serial numbers were recorded")
		case in.Tracking == "lot" && oldT == "none":
			lot, err := ensureLot(ctx, tx, orgID, id, "UNASSIGNED", "")
			if err != nil {
				return in, err
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO adrient.inv_lot_stock (lot_id, location_id, org_id, product_id, qty)
				 SELECT $1, location_id, org_id, product_id, qty FROM adrient.inv_stock WHERE product_id=$2 AND qty > 0
				 ON CONFLICT (lot_id, location_id) DO UPDATE SET qty = EXCLUDED.qty`, lot, id); err != nil {
				return in, err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.inv_products SET valuation_method=$3, tracking=$4, shelf_life_days=$5, updated_at=now() WHERE org_id=$1 AND id=$2`,
		orgID, id, in.ValuationMethod, in.Tracking, in.ShelfLifeDays); err != nil {
		return in, err
	}
	in.ProductID = id
	return in, tx.Commit(ctx)
}

// ------------------------------------------------------------ valuation report

// ValuationRow is one product in the stock valuation report.
type ValuationRow struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	SKU       string  `json:"sku"`
	Category  string  `json:"category"`
	Method    string  `json:"method"`
	Qty       float64 `json:"qty"`
	Value     float64 `json:"value"`
	AvgCost   float64 `json:"avg_cost"`
}

// CategoryValue is stock value of one category.
type CategoryValue struct {
	Category string  `json:"category"`
	Qty      float64 `json:"qty"`
	Value    float64 `json:"value"`
}

// ValuationReport is the stock value as of a date.
type ValuationReport struct {
	AsOf       string          `json:"as_of"`
	Rows       []ValuationRow  `json:"rows"`
	TotalQty   float64         `json:"total_qty"`
	TotalValue float64         `json:"total_value"`
	ByCategory []CategoryValue `json:"by_category"`
	InTransit  float64         `json:"in_transit_qty"`
}

// ValuationAsOf values stock on a date (YYYY-MM-DD, empty = now) from the value ledger.
func (r *Repository) ValuationAsOf(ctx context.Context, orgID, asOf, category string) (ValuationReport, error) {
	rep := ValuationReport{Rows: []ValuationRow{}, ByCategory: []CategoryValue{}}
	cut := time.Now().Add(time.Hour)
	if asOf != "" {
		d, err := time.Parse("2006-01-02", asOf)
		if err != nil {
			return rep, invalid("as_of must be YYYY-MM-DD")
		}
		cut = d.AddDate(0, 0, 1)
		rep.AsOf = asOf
	} else {
		rep.AsOf = time.Now().Format("2006-01-02")
	}
	rows, err := r.pool.Query(ctx,
		`SELECT p.id::text, p.name || CASE WHEN p.variant_label<>'' THEN ' / '||p.variant_label ELSE '' END, p.sku, p.category, p.valuation_method,
		        sum(v.qty)::float8, sum(v.value)::float8
		 FROM adrient.inv_value_ledger v JOIN adrient.inv_products p ON p.id=v.product_id
		 WHERE v.org_id=$1 AND v.ts < $2 AND ($3='' OR p.category=$3)
		 GROUP BY p.id HAVING sum(v.qty) <> 0 OR sum(v.value) <> 0 ORDER BY p.name`, orgID, cut, category)
	if err != nil {
		return rep, err
	}
	defer rows.Close()
	cat := map[string]*CategoryValue{}
	for rows.Next() {
		var x ValuationRow
		if err := rows.Scan(&x.ProductID, &x.Name, &x.SKU, &x.Category, &x.Method, &x.Qty, &x.Value); err != nil {
			return rep, err
		}
		x.Qty, x.Value = round3(x.Qty), round2(x.Value)
		if x.Qty > 0 {
			x.AvgCost = round4(x.Value / x.Qty)
		}
		rep.Rows = append(rep.Rows, x)
		rep.TotalQty += x.Qty
		rep.TotalValue += x.Value
		c := x.Category
		if c == "" {
			c = "Uncategorised"
		}
		if cat[c] == nil {
			cat[c] = &CategoryValue{Category: c}
		}
		cat[c].Qty += x.Qty
		cat[c].Value += x.Value
	}
	if err := rows.Err(); err != nil {
		return rep, err
	}
	rep.TotalQty, rep.TotalValue = round3(rep.TotalQty), round2(rep.TotalValue)
	for _, v := range cat {
		rep.ByCategory = append(rep.ByCategory, *v)
	}
	if asOf == "" || asOf == time.Now().Format("2006-01-02") {
		_ = r.pool.QueryRow(ctx,
			`SELECT COALESCE(sum(l.qty_shipped-l.qty_received),0)::float8 FROM adrient.inv_transfer_lines l JOIN adrient.inv_transfers t ON t.id=l.transfer_id
			 WHERE t.org_id=$1 AND t.state='in_transit'`, orgID).Scan(&rep.InTransit)
	}
	return rep, nil
}

// CostLayer is an open receipt layer.
type CostLayer struct {
	ID         string    `json:"id"`
	ReceivedAt time.Time `json:"received_at"`
	QtyIn      float64   `json:"qty_in"`
	QtyLeft    float64   `json:"qty_remaining"`
	UnitCost   float64   `json:"unit_cost"`
	Source     string    `json:"source"`
	Ref        string    `json:"ref"`
}

// ProductLayers lists the open cost layers of a product, oldest first.
func (r *Repository) ProductLayers(ctx context.Context, orgID, productID string) ([]CostLayer, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, received_at, qty_in::float8, qty_remaining::float8, unit_cost::float8, source, ref
		 FROM adrient.inv_cost_layers WHERE org_id=$1 AND product_id=$2 AND qty_remaining > 0 ORDER BY received_at, id`, orgID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CostLayer{}
	for rows.Next() {
		var l CostLayer
		if err := rows.Scan(&l.ID, &l.ReceivedAt, &l.QtyIn, &l.QtyLeft, &l.UnitCost, &l.Source, &l.Ref); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LedgerRow is one valued movement (COGS and adjustment data for Accounting).
type LedgerRow struct {
	ID           string    `json:"id"`
	ProductID    string    `json:"product_id"`
	ProductName  string    `json:"product_name"`
	SKU          string    `json:"sku"`
	TS           time.Time `json:"ts"`
	Qty          float64   `json:"qty"`
	UnitCost     float64   `json:"unit_cost"`
	Value        float64   `json:"value"`
	Reason       string    `json:"reason"`
	Ref          string    `json:"ref"`
	JournalKind  string    `json:"journal_kind"`
	JournalState string    `json:"journal_state"`
	JournalError string    `json:"journal_error"`
}

// ListLedger returns valued movements between two dates (inclusive).
func (r *Repository) ListLedger(ctx context.Context, orgID, from, to, reason, kind string, limit int) ([]LedgerRow, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	var f, t *time.Time
	if from != "" {
		d, err := time.Parse("2006-01-02", from)
		if err != nil {
			return nil, invalid("from must be YYYY-MM-DD")
		}
		f = &d
	}
	if to != "" {
		d, err := time.Parse("2006-01-02", to)
		if err != nil {
			return nil, invalid("to must be YYYY-MM-DD")
		}
		d = d.AddDate(0, 0, 1)
		t = &d
	}
	rows, err := r.pool.Query(ctx,
		`SELECT v.id::text, v.product_id::text, p.name, p.sku, v.ts, v.qty::float8, v.unit_cost::float8, v.value::float8, v.reason, v.ref,
		        v.journal_kind, v.journal_state, v.journal_error
		 FROM adrient.inv_value_ledger v JOIN adrient.inv_products p ON p.id=v.product_id
		 WHERE v.org_id=$1 AND ($2::timestamptz IS NULL OR v.ts >= $2) AND ($3::timestamptz IS NULL OR v.ts < $3)
		   AND ($4='' OR v.reason=$4) AND ($5='' OR v.journal_kind=$5)
		 ORDER BY v.ts DESC, v.id LIMIT $6`, orgID, f, t, reason, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LedgerRow{}
	for rows.Next() {
		var x LedgerRow
		if err := rows.Scan(&x.ID, &x.ProductID, &x.ProductName, &x.SKU, &x.TS, &x.Qty, &x.UnitCost, &x.Value, &x.Reason, &x.Ref, &x.JournalKind, &x.JournalState, &x.JournalError); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ------------------------------------------------------------ accounting hand-over

// JournalLine is one side of an entry; AccountKey is an Accounting system key
// such as "inventory", "cogs" or "ap".
type JournalLine struct {
	AccountKey string
	Debit      float64
	Credit     float64
	Memo       string
}

// JournalEntry is a balanced entry for Accounting. Source is one of
// stock_cogs, stock_adjustment, purchase_bill, purchase_return and SourceID is
// unique per entry so reposting replaces rather than duplicates.
type JournalEntry struct {
	Date        string
	Memo        string
	Ref         string
	Source      string
	SourceID    string
	ContactName string
	DueDate     string
	Lines       []JournalLine
}

// JournalPoster is implemented by an adapter around the Accounting module.
type JournalPoster interface {
	PostEntry(ctx context.Context, orgID, userID string, e JournalEntry) error
}

type posterBox struct{ p JournalPoster }

var activePoster atomic.Pointer[posterBox]

// SetJournalPoster wires Accounting in (nil switches posting off). It applies to
// every Repository in the process, so POS and Invoicing sales are covered too.
func SetJournalPoster(p JournalPoster) { activePoster.Store(&posterBox{p: p}) }

func currentPoster() JournalPoster {
	if b := activePoster.Load(); b != nil {
		return b.p
	}
	return nil
}

type pendingRow struct {
	id, kind, ref, reason string
	day                   string
	value                 float64
}

// FlushJournals posts pending COGS/adjustment entries for one org. Rows that
// cannot be posted (closed period, no chart of accounts) stay marked failed
// with the reason and are retried on the next flush.
func (r *Repository) FlushJournals(ctx context.Context, orgID string) (posted, failed int, err error) {
	p := currentPoster()
	if p == nil {
		return 0, 0, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, journal_kind, ref, reason, to_char(ts,'YYYY-MM-DD'), value::float8
		 FROM adrient.inv_value_ledger WHERE org_id=$1 AND journal_state IN ('pending','failed') AND journal_kind<>''
		 ORDER BY ts, id LIMIT 1000`, orgID)
	if err != nil {
		return 0, 0, err
	}
	var all []pendingRow
	for rows.Next() {
		var x pendingRow
		if err := rows.Scan(&x.id, &x.kind, &x.ref, &x.reason, &x.day, &x.value); err != nil {
			rows.Close()
			return 0, 0, err
		}
		all = append(all, x)
	}
	rows.Close()
	type group struct {
		key  string
		rows []pendingRow
	}
	var order []string
	groups := map[string]*group{}
	for _, x := range all {
		k := x.kind + "|" + x.day + "|" + x.ref
		if x.ref == "" {
			k = x.kind + "|" + x.day + "|" + x.reason
		}
		if groups[k] == nil {
			groups[k] = &group{key: k}
			order = append(order, k)
		}
		groups[k].rows = append(groups[k].rows, x)
	}
	for _, k := range order {
		g := groups[k]
		var net float64 // value that left stock (positive = expense)
		ids := make([]string, 0, len(g.rows))
		for _, x := range g.rows {
			net -= x.value
			ids = append(ids, x.id)
		}
		net = round2(net)
		first := g.rows[0]
		if net == 0 {
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.inv_value_ledger SET journal_state='posted', journal_error='' WHERE id = ANY($1::uuid[])`, ids)
			continue
		}
		memo := "Cost of goods sold"
		if first.kind == "stock_adjustment" {
			memo = "Stock adjustment (" + first.reason + ")"
		}
		ref := first.ref
		if ref != "" {
			memo += " " + ref
		}
		amt := net
		dr, cr := "cogs", "inventory"
		if net < 0 {
			amt = -net
			dr, cr = "inventory", "cogs"
		}
		entry := JournalEntry{Date: first.day, Memo: memo, Ref: ref, Source: first.kind, SourceID: first.kind + ":" + first.id,
			Lines: []JournalLine{{AccountKey: dr, Debit: amt, Memo: memo}, {AccountKey: cr, Credit: amt, Memo: memo}}}
		if err := p.PostEntry(ctx, orgID, "", entry); err != nil {
			failed += len(ids)
			msg := err.Error()
			if len(msg) > 300 {
				msg = msg[:300]
			}
			slog.Warn("inventory: journal post failed", "org", orgID, "err", err)
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.inv_value_ledger SET journal_state='failed', journal_error=$2 WHERE id = ANY($1::uuid[])`, ids, msg)
			continue
		}
		posted += len(ids)
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.inv_value_ledger SET journal_state='posted', journal_error='' WHERE id = ANY($1::uuid[])`, ids)
	}
	return posted, failed, nil
}

// FlushAllJournals posts pending entries for every org (call from a ticker).
func (r *Repository) FlushAllJournals(ctx context.Context) {
	if currentPoster() == nil {
		return
	}
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT org_id::text FROM adrient.inv_value_ledger WHERE journal_state='pending' AND journal_kind<>''`)
	if err != nil {
		return
	}
	var orgs []string
	for rows.Next() {
		var o string
		if rows.Scan(&o) == nil {
			orgs = append(orgs, o)
		}
	}
	rows.Close()
	for _, o := range orgs {
		_, _, _ = r.FlushJournals(ctx, o)
	}
}

// PendingJournalCount is how many valued movements still wait for Accounting.
func (r *Repository) PendingJournalCount(ctx context.Context, orgID string) (pending, failed int, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE journal_state='pending'), count(*) FILTER (WHERE journal_state='failed')
		 FROM adrient.inv_value_ledger WHERE org_id=$1 AND journal_kind<>''`, orgID).Scan(&pending, &failed)
	return
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }
