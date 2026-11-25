package inventory

// Purchasing: settings, supplier price lists, purchase requests and purchase
// orders (draft, approve, send, acknowledge, receive, close).

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// PurchaseSettings are the per-org purchasing rules.
type PurchaseSettings struct {
	CompanyName        string  `json:"company_name"`
	CompanyAddress     string  `json:"company_address"`
	CompanyGSTIN       string  `json:"company_gstin"`
	Currency           string  `json:"currency"`
	DefaultTerms       string  `json:"default_terms"`
	RequireApproval    bool    `json:"require_approval"`
	ApprovalThreshold  float64 `json:"approval_threshold"`
	OverReceiptPct     float64 `json:"over_receipt_pct"`
	UnderReceiptPolicy string  `json:"under_receipt_policy"` // backorder | close
	QtyTolerancePct    float64 `json:"qty_tolerance_pct"`
	PriceTolerancePct  float64 `json:"price_tolerance_pct"`
	TaxSplit           string  `json:"tax_split"` // cgst_sgst | igst
}

// GetPurchaseSettings returns settings, with defaults when none were saved.
func (r *Repository) GetPurchaseSettings(ctx context.Context, orgID string) (PurchaseSettings, error) {
	return getSettings(ctx, r.pool, orgID)
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getSettings(ctx context.Context, q queryRower, orgID string) (PurchaseSettings, error) {
	s := PurchaseSettings{Currency: "INR", RequireApproval: true, UnderReceiptPolicy: "backorder", PriceTolerancePct: 2, TaxSplit: "cgst_sgst"}
	err := q.QueryRow(ctx,
		`SELECT company_name, company_address, company_gstin, currency, default_terms, require_approval, approval_threshold::float8,
		        over_receipt_pct::float8, under_receipt_policy, qty_tolerance_pct::float8, price_tolerance_pct::float8, tax_split
		 FROM adrient.inv_purchase_settings WHERE org_id=$1`, orgID).
		Scan(&s.CompanyName, &s.CompanyAddress, &s.CompanyGSTIN, &s.Currency, &s.DefaultTerms, &s.RequireApproval, &s.ApprovalThreshold,
			&s.OverReceiptPct, &s.UnderReceiptPolicy, &s.QtyTolerancePct, &s.PriceTolerancePct, &s.TaxSplit)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil
	}
	return s, err
}

// SavePurchaseSettings stores settings.
func (r *Repository) SavePurchaseSettings(ctx context.Context, orgID string, s PurchaseSettings) (PurchaseSettings, error) {
	if s.UnderReceiptPolicy != "backorder" && s.UnderReceiptPolicy != "close" {
		return s, invalid("under-receipt policy must be backorder or close")
	}
	if s.TaxSplit != "cgst_sgst" && s.TaxSplit != "igst" {
		return s, invalid("tax split must be cgst_sgst or igst")
	}
	for _, v := range []float64{s.ApprovalThreshold, s.OverReceiptPct, s.QtyTolerancePct, s.PriceTolerancePct} {
		if v < 0 || v > 1000000000 {
			return s, invalid("tolerances and thresholds cannot be negative")
		}
	}
	if strings.TrimSpace(s.Currency) == "" {
		s.Currency = "INR"
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO adrient.inv_purchase_settings (org_id, company_name, company_address, company_gstin, currency, default_terms, require_approval,
		   approval_threshold, over_receipt_pct, under_receipt_policy, qty_tolerance_pct, price_tolerance_pct, tax_split)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 ON CONFLICT (org_id) DO UPDATE SET company_name=EXCLUDED.company_name, company_address=EXCLUDED.company_address, company_gstin=EXCLUDED.company_gstin,
		   currency=EXCLUDED.currency, default_terms=EXCLUDED.default_terms, require_approval=EXCLUDED.require_approval, approval_threshold=EXCLUDED.approval_threshold,
		   over_receipt_pct=EXCLUDED.over_receipt_pct, under_receipt_policy=EXCLUDED.under_receipt_policy, qty_tolerance_pct=EXCLUDED.qty_tolerance_pct,
		   price_tolerance_pct=EXCLUDED.price_tolerance_pct, tax_split=EXCLUDED.tax_split`,
		orgID, s.CompanyName, s.CompanyAddress, s.CompanyGSTIN, s.Currency, s.DefaultTerms, s.RequireApproval, s.ApprovalThreshold,
		s.OverReceiptPct, s.UnderReceiptPolicy, s.QtyTolerancePct, s.PriceTolerancePct, s.TaxSplit)
	return s, err
}

// ------------------------------------------------------------ supplier prices

// SupplierPrice is one tier of a supplier's price list.
type SupplierPrice struct {
	ID           string  `json:"id"`
	SupplierID   string  `json:"supplier_id"`
	SupplierName string  `json:"supplier_name"`
	ProductID    string  `json:"product_id"`
	ProductName  string  `json:"product_name"`
	MinQty       float64 `json:"min_qty"`
	UnitCost     float64 `json:"unit_cost"`
	LeadDays     int     `json:"lead_days"`
	ValidFrom    string  `json:"valid_from"`
	ValidTo      string  `json:"valid_to"`
	SupplierSKU  string  `json:"supplier_sku"`
	Note         string  `json:"note"`
}

// ListSupplierPrices lists price tiers by supplier and/or product.
func (r *Repository) ListSupplierPrices(ctx context.Context, orgID, supplierID, productID string) ([]SupplierPrice, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT x.id::text, x.supplier_id::text, s.name, x.product_id::text, p.name, x.min_qty::float8, x.unit_cost::float8, x.lead_days,
		        COALESCE(to_char(x.valid_from,'YYYY-MM-DD'),''), COALESCE(to_char(x.valid_to,'YYYY-MM-DD'),''), x.supplier_sku, x.note
		 FROM adrient.inv_supplier_prices x JOIN adrient.inv_suppliers s ON s.id=x.supplier_id JOIN adrient.inv_products p ON p.id=x.product_id
		 WHERE x.org_id=$1 AND ($2='' OR x.supplier_id::text=$2) AND ($3='' OR x.product_id::text=$3) ORDER BY s.name, p.name, x.min_qty`, orgID, supplierID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SupplierPrice{}
	for rows.Next() {
		var x SupplierPrice
		if err := rows.Scan(&x.ID, &x.SupplierID, &x.SupplierName, &x.ProductID, &x.ProductName, &x.MinQty, &x.UnitCost, &x.LeadDays, &x.ValidFrom, &x.ValidTo, &x.SupplierSKU, &x.Note); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SaveSupplierPrice creates or updates a price tier.
func (r *Repository) SaveSupplierPrice(ctx context.Context, orgID, id string, x SupplierPrice) (SupplierPrice, error) {
	if x.UnitCost < 0 || x.MinQty < 0 || x.LeadDays < 0 {
		return x, invalid("cost, minimum quantity and lead time cannot be negative")
	}
	if x.MinQty == 0 {
		x.MinQty = 1
	}
	vf, err := parseDate(x.ValidFrom)
	if err != nil {
		return x, err
	}
	vt, err := parseDate(x.ValidTo)
	if err != nil {
		return x, err
	}
	if vf != nil && vt != nil && vt.Before(*vf) {
		return x, invalid("valid-to is before valid-from")
	}
	if id == "" {
		err = r.pool.QueryRow(ctx,
			`INSERT INTO adrient.inv_supplier_prices (org_id, supplier_id, product_id, min_qty, unit_cost, lead_days, valid_from, valid_to, supplier_sku, note)
			 SELECT $1,s.id,p.id,$4,$5,$6,$7,$8,$9,$10 FROM adrient.inv_suppliers s, adrient.inv_products p
			 WHERE s.org_id=$1 AND s.id=$2 AND p.org_id=$1 AND p.id=$3 RETURNING id::text`,
			orgID, x.SupplierID, x.ProductID, x.MinQty, x.UnitCost, x.LeadDays, vf, vt, x.SupplierSKU, x.Note).Scan(&x.ID)
	} else {
		err = r.pool.QueryRow(ctx,
			`UPDATE adrient.inv_supplier_prices SET min_qty=$3, unit_cost=$4, lead_days=$5, valid_from=$6, valid_to=$7, supplier_sku=$8, note=$9
			 WHERE org_id=$1 AND id=$2 RETURNING id::text`, orgID, id, x.MinQty, x.UnitCost, x.LeadDays, vf, vt, x.SupplierSKU, x.Note).Scan(&x.ID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return x, ErrNotFound
	}
	return x, err
}

// DeleteSupplierPrice removes a tier.
func (r *Repository) DeleteSupplierPrice(ctx context.Context, orgID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.inv_supplier_prices WHERE org_id=$1 AND id=$2`, orgID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// PriceFor finds the unit cost and lead time a supplier offers for a quantity:
// the best valid price tier, else the supplier's default on the product, else the
// product cost.
func (r *Repository) priceFor(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}, orgID, supplierID, productID string, qty float64) (cost float64, lead int, err error) {
	rows, err := q.Query(ctx,
		`SELECT min_qty::float8, unit_cost::float8, valid_from, valid_to, lead_days FROM adrient.inv_supplier_prices
		 WHERE org_id=$1 AND supplier_id=$2 AND product_id=$3`, orgID, supplierID, productID)
	if err != nil {
		return 0, 0, err
	}
	var tiers []PriceTier
	for rows.Next() {
		var t PriceTier
		if err := rows.Scan(&t.MinQty, &t.Cost, &t.ValidFrom, &t.ValidTo, &t.LeadDays); err != nil {
			rows.Close()
			return 0, 0, err
		}
		tiers = append(tiers, t)
	}
	rows.Close()
	if t, ok := PickTier(tiers, qty, time.Now()); ok {
		return t.Cost, t.LeadDays, nil
	}
	var c float64
	var l int
	err = q.QueryRow(ctx, `SELECT cost::float8, lead_days FROM adrient.inv_product_suppliers WHERE org_id=$1 AND supplier_id=$2 AND product_id=$3`, orgID, supplierID, productID).Scan(&c, &l)
	if err == nil && c > 0 {
		return c, l, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, err
	}
	err = q.QueryRow(ctx, `SELECT cost::float8 FROM adrient.inv_products WHERE org_id=$1 AND id=$2`, orgID, productID).Scan(&c)
	return c, l, err
}

// ------------------------------------------------------------ purchase orders

// POLine is one line of a purchase order.
type POLine struct {
	ID          string  `json:"id"`
	ProductID   string  `json:"product_id"`
	ProductName string  `json:"product_name"`
	SKU         string  `json:"sku"`
	Unit        string  `json:"unit"`
	Tracking    string  `json:"tracking"`
	Description string  `json:"description"`
	Qty         float64 `json:"qty"`
	UnitCost    float64 `json:"unit_cost"`
	TaxPct      float64 `json:"tax_pct"`
	QtyReceived float64 `json:"qty_received"`
	QtyReturned float64 `json:"qty_returned"`
	QtyBilled   float64 `json:"qty_billed"`
	LineTotal   float64 `json:"line_total"`
}

// PurchaseOrder is the PO header with lines.
type PurchaseOrder struct {
	ID            string     `json:"id"`
	Number        string     `json:"number"`
	SupplierID    string     `json:"supplier_id"`
	SupplierName  string     `json:"supplier_name"`
	SupplierEmail string     `json:"supplier_email"`
	State         string     `json:"state"`
	OrderDate     string     `json:"order_date"`
	ExpectedDate  string     `json:"expected_date"`
	ShipToLocID   string     `json:"ship_to_loc_id"`
	ShipToName    string     `json:"ship_to_name"`
	Currency      string     `json:"currency"`
	SupplierRef   string     `json:"supplier_ref"`
	Notes         string     `json:"notes"`
	Terms         string     `json:"terms"`
	Subtotal      float64    `json:"subtotal"`
	TaxTotal      float64    `json:"tax_total"`
	Total         float64    `json:"total"`
	RequestID     string     `json:"request_id"`
	ApprovedAt    *time.Time `json:"approved_at"`
	SentAt        *time.Time `json:"sent_at"`
	SentTo        string     `json:"sent_to"`
	AckAt         *time.Time `json:"ack_at"`
	AckRef        string     `json:"ack_ref"`
	AckNote       string     `json:"ack_note"`
	PromisedDate  string     `json:"promised_date"`
	CreatedAt     time.Time  `json:"created_at"`
	Lines         []POLine   `json:"lines"`
	NeedsApproval bool       `json:"needs_approval"`
}

// POInput creates or replaces a draft purchase order.
type POInput struct {
	SupplierID   string `json:"supplier_id"`
	ExpectedDate string `json:"expected_date"`
	ShipToLocID  string `json:"ship_to_loc_id"`
	SupplierRef  string `json:"supplier_ref"`
	Notes        string `json:"notes"`
	Terms        string `json:"terms"`
	RequestID    string `json:"request_id"`
	Lines        []struct {
		ProductID   string   `json:"product_id"`
		Description string   `json:"description"`
		Qty         float64  `json:"qty"`
		UnitCost    *float64 `json:"unit_cost"`
		TaxPct      float64  `json:"tax_pct"`
	} `json:"lines"`
}

const poSelect = `SELECT o.id::text, o.number, o.supplier_id::text, s.name, s.email, o.state, to_char(o.order_date,'YYYY-MM-DD'),
	COALESCE(to_char(o.expected_date,'YYYY-MM-DD'),''), COALESCE(o.ship_to_loc_id::text,''), COALESCE(l.name,''), o.currency, o.supplier_ref,
	o.notes, o.terms, o.subtotal::float8, o.tax_total::float8, o.total::float8, COALESCE(o.request_id::text,''), o.approved_at, o.sent_at, o.sent_to,
	o.ack_at, o.ack_ref, o.ack_note, COALESCE(to_char(o.promised_date,'YYYY-MM-DD'),''), o.created_at
	FROM adrient.inv_purchase_orders o JOIN adrient.inv_suppliers s ON s.id=o.supplier_id
	LEFT JOIN adrient.inv_locations l ON l.id=o.ship_to_loc_id`

func scanPO(row pgx.Row) (PurchaseOrder, error) {
	var o PurchaseOrder
	err := row.Scan(&o.ID, &o.Number, &o.SupplierID, &o.SupplierName, &o.SupplierEmail, &o.State, &o.OrderDate, &o.ExpectedDate, &o.ShipToLocID, &o.ShipToName,
		&o.Currency, &o.SupplierRef, &o.Notes, &o.Terms, &o.Subtotal, &o.TaxTotal, &o.Total, &o.RequestID, &o.ApprovedAt, &o.SentAt, &o.SentTo,
		&o.AckAt, &o.AckRef, &o.AckNote, &o.PromisedDate, &o.CreatedAt)
	return o, err
}

func (r *Repository) poLines(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, orgID string, ids []string) (map[string][]POLine, error) {
	rows, err := q.Query(ctx,
		`SELECT l.po_id::text, l.id::text, l.product_id::text, p.name || CASE WHEN p.variant_label<>'' THEN ' / '||p.variant_label ELSE '' END, p.sku, p.unit, p.tracking,
		        l.description, l.qty::float8, l.unit_cost::float8, l.tax_pct::float8, l.qty_received::float8, l.qty_returned::float8, l.qty_billed::float8
		 FROM adrient.inv_purchase_order_lines l JOIN adrient.inv_products p ON p.id=l.product_id
		 WHERE l.org_id=$1 AND l.po_id = ANY($2::uuid[]) ORDER BY l.sort, l.id`, orgID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]POLine{}
	for rows.Next() {
		var pid string
		var l POLine
		if err := rows.Scan(&pid, &l.ID, &l.ProductID, &l.ProductName, &l.SKU, &l.Unit, &l.Tracking, &l.Description, &l.Qty, &l.UnitCost, &l.TaxPct, &l.QtyReceived, &l.QtyReturned, &l.QtyBilled); err != nil {
			return nil, err
		}
		l.LineTotal = round2(l.Qty * l.UnitCost)
		out[pid] = append(out[pid], l)
	}
	return out, rows.Err()
}

// GetPO loads one purchase order.
func (r *Repository) GetPO(ctx context.Context, orgID, id string) (PurchaseOrder, error) {
	o, err := scanPO(r.pool.QueryRow(ctx, poSelect+` WHERE o.org_id=$1 AND o.id=$2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return o, ErrNotFound
	}
	if err != nil {
		return o, err
	}
	m, err := r.poLines(ctx, r.pool, orgID, []string{id})
	if err != nil {
		return o, err
	}
	o.Lines = m[id]
	if o.Lines == nil {
		o.Lines = []POLine{}
	}
	st, err := r.GetPurchaseSettings(ctx, orgID)
	if err == nil {
		o.NeedsApproval = st.RequireApproval && o.Total >= st.ApprovalThreshold
	}
	return o, nil
}

// ListPOs lists purchase orders.
func (r *Repository) ListPOs(ctx context.Context, orgID, state, supplierID, query string) ([]PurchaseOrder, error) {
	rows, err := r.pool.Query(ctx, poSelect+` WHERE o.org_id=$1 AND ($2='' OR o.state=$2) AND ($3='' OR o.supplier_id::text=$3)
		AND ($4='' OR o.number ILIKE '%'||$4||'%' OR s.name ILIKE '%'||$4||'%' OR o.supplier_ref ILIKE '%'||$4||'%')
		ORDER BY o.created_at DESC LIMIT 500`, orgID, state, supplierID, query)
	if err != nil {
		return nil, err
	}
	out := []PurchaseOrder{}
	var ids []string
	for rows.Next() {
		o, err := scanPO(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, o)
		ids = append(ids, o.ID)
	}
	rows.Close()
	m, err := r.poLines(ctx, r.pool, orgID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Lines = m[out[i].ID]
		if out[i].Lines == nil {
			out[i].Lines = []POLine{}
		}
	}
	return out, nil
}

func (r *Repository) writePOLines(ctx context.Context, tx pgx.Tx, orgID, poID, supplierID string, in POInput) (sub, tax float64, err error) {
	if len(in.Lines) == 0 {
		return 0, 0, invalid("add at least one line")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.inv_purchase_order_lines WHERE po_id=$1`, poID); err != nil {
		return 0, 0, err
	}
	for i, l := range in.Lines {
		if l.Qty <= 0 {
			return 0, 0, invalid("quantities must be positive")
		}
		if l.TaxPct < 0 || l.TaxPct > 100 {
			return 0, 0, invalid("tax percentage must be between 0 and 100")
		}
		var cost float64
		if l.UnitCost != nil {
			if *l.UnitCost < 0 {
				return 0, 0, invalid("unit cost cannot be negative")
			}
			cost = *l.UnitCost
		} else if cost, _, err = r.priceFor(ctx, tx, orgID, supplierID, l.ProductID, l.Qty); err != nil {
			return 0, 0, err
		}
		tag, err := tx.Exec(ctx,
			`INSERT INTO adrient.inv_purchase_order_lines (po_id, org_id, product_id, description, qty, unit_cost, tax_pct, sort)
			 SELECT $1,$2,p.id,$4,$5,$6,$7,$8 FROM adrient.inv_products p WHERE p.org_id=$2 AND p.id=$3`,
			poID, orgID, l.ProductID, l.Description, round3(l.Qty), round4(cost), l.TaxPct, i)
		if err != nil {
			return 0, 0, err
		}
		if tag.RowsAffected() == 0 {
			return 0, 0, invalid("product not found")
		}
		line := round2(l.Qty * cost)
		sub += line
		tax += round2(line * l.TaxPct / 100)
	}
	return round2(sub), round2(tax), nil
}

// CreatePO saves a new draft purchase order.
func (r *Repository) CreatePO(ctx context.Context, orgID, userID string, in POInput) (PurchaseOrder, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	defer tx.Rollback(ctx)
	id, err := r.createPOTx(ctx, tx, orgID, userID, in)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, err
	}
	o, err := r.GetPO(ctx, orgID, id)
	if err != nil {
		return o, err
	}
	if !o.NeedsApproval { // below the approval threshold (or approval is off): no sign-off step
		return r.ApprovePO(ctx, orgID, userID, id)
	}
	return o, nil
}

func (r *Repository) createPOTx(ctx context.Context, tx pgx.Tx, orgID, userID string, in POInput) (string, error) {
	var supplier string
	if err := tx.QueryRow(ctx, `SELECT name FROM adrient.inv_suppliers WHERE org_id=$1 AND id=$2`, orgID, in.SupplierID).Scan(&supplier); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", invalid("pick a supplier")
		}
		return "", err
	}
	exp, err := parseDate(in.ExpectedDate)
	if err != nil {
		return "", err
	}
	st, err := getSettings(ctx, tx, orgID)
	if err != nil {
		return "", err
	}
	if in.Terms == "" {
		in.Terms = st.DefaultTerms
	}
	num, err := nextNumber(ctx, tx, orgID, "po", "PO")
	if err != nil {
		return "", err
	}
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_purchase_orders (org_id, number, supplier_id, expected_date, ship_to_loc_id, currency, supplier_ref, notes, terms, request_id, created_by)
		 VALUES ($1,$2,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8,$9,NULLIF($10,'')::uuid,NULLIF($11,'')::uuid) RETURNING id::text`,
		orgID, num, in.SupplierID, exp, in.ShipToLocID, st.Currency, in.SupplierRef, in.Notes, in.Terms, in.RequestID, userID).Scan(&id); err != nil {
		return "", err
	}
	sub, tax, err := r.writePOLines(ctx, tx, orgID, id, in.SupplierID, in)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `UPDATE adrient.inv_purchase_orders SET subtotal=$2, tax_total=$3, total=$4 WHERE id=$1`, id, sub, tax, round2(sub+tax))
	return id, err
}

// UpdatePO replaces the content of a draft purchase order.
func (r *Repository) UpdatePO(ctx context.Context, orgID, id string, in POInput) (PurchaseOrder, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM adrient.inv_purchase_orders WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&state); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PurchaseOrder{}, ErrNotFound
		}
		return PurchaseOrder{}, err
	}
	if !POCanEdit(state) {
		return PurchaseOrder{}, invalid("only a draft order can be edited")
	}
	exp, err := parseDate(in.ExpectedDate)
	if err != nil {
		return PurchaseOrder{}, err
	}
	sub, tax, err := r.writePOLines(ctx, tx, orgID, id, in.SupplierID, in)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE adrient.inv_purchase_orders SET supplier_id=$3, expected_date=$4, ship_to_loc_id=NULLIF($5,'')::uuid, supplier_ref=$6, notes=$7, terms=$8,
		   subtotal=$9, tax_total=$10, total=$11, updated_at=now() WHERE org_id=$1 AND id=$2`,
		orgID, id, in.SupplierID, exp, in.ShipToLocID, in.SupplierRef, in.Notes, in.Terms, sub, tax, round2(sub+tax)); err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, err
	}
	return r.GetPO(ctx, orgID, id)
}

// movePO changes state if the state machine allows it.
func (r *Repository) movePO(ctx context.Context, tx pgx.Tx, orgID, id, to string) (cur string, err error) {
	if err := tx.QueryRow(ctx, `SELECT state FROM adrient.inv_purchase_orders WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&cur); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if !POCanMove(cur, to) {
		return cur, invalid("a %s order cannot become %s", cur, to)
	}
	return cur, nil
}

// ApprovePO approves a draft order.
func (r *Repository) ApprovePO(ctx context.Context, orgID, userID, id string) (PurchaseOrder, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := r.movePO(ctx, tx, orgID, id, POapproved); err != nil {
		return PurchaseOrder{}, err
	}
	var lines int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.inv_purchase_order_lines WHERE po_id=$1`, id).Scan(&lines); err != nil {
		return PurchaseOrder{}, err
	}
	if lines == 0 {
		return PurchaseOrder{}, invalid("the order has no lines")
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.inv_purchase_orders SET state='approved', approved_by=NULLIF($3,'')::uuid, approved_at=now(), updated_at=now() WHERE org_id=$1 AND id=$2`, orgID, id, userID); err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, err
	}
	return r.GetPO(ctx, orgID, id)
}

// MarkPOSent records that the order was sent (by mail from here, or by phone/other means).
func (r *Repository) MarkPOSent(ctx context.Context, orgID, id, to string) (PurchaseOrder, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	defer tx.Rollback(ctx)
	cur, err := r.movePO(ctx, tx, orgID, id, POsent)
	if err != nil && cur != POsent {
		return PurchaseOrder{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.inv_purchase_orders SET state='sent', sent_at=now(), sent_to=$3, updated_at=now() WHERE org_id=$1 AND id=$2`, orgID, id, to); err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, err
	}
	return r.GetPO(ctx, orgID, id)
}

// AckInput records the supplier's acknowledgement.
type AckInput struct {
	Ref          string `json:"ref"`
	Note         string `json:"note"`
	PromisedDate string `json:"promised_date"`
}

// AcknowledgePO records a supplier acknowledgement and their promised date.
func (r *Repository) AcknowledgePO(ctx context.Context, orgID, id string, in AckInput) (PurchaseOrder, error) {
	pd, err := parseDate(in.PromisedDate)
	if err != nil {
		return PurchaseOrder{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := r.movePO(ctx, tx, orgID, id, POacknowledged); err != nil {
		return PurchaseOrder{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.inv_purchase_orders SET state='acknowledged', ack_at=now(), ack_ref=$3, ack_note=$4, promised_date=$5, updated_at=now() WHERE org_id=$1 AND id=$2`,
		orgID, id, in.Ref, in.Note, pd); err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, err
	}
	return r.GetPO(ctx, orgID, id)
}

// CancelPO cancels an order that has no receipts.
func (r *Repository) CancelPO(ctx context.Context, orgID, id string) (PurchaseOrder, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := r.movePO(ctx, tx, orgID, id, POcancelled); err != nil {
		return PurchaseOrder{}, err
	}
	var got float64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(qty_received),0)::float8 FROM adrient.inv_purchase_order_lines WHERE po_id=$1`, id).Scan(&got); err != nil {
		return PurchaseOrder{}, err
	}
	if got > 0 {
		return PurchaseOrder{}, invalid("goods were already received; close the order instead")
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.inv_purchase_orders SET state='cancelled', updated_at=now() WHERE org_id=$1 AND id=$2`, orgID, id); err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, err
	}
	return r.GetPO(ctx, orgID, id)
}

// ClosePO closes a received or part-received order (the remainder is not coming).
func (r *Repository) ClosePO(ctx context.Context, orgID, id string) (PurchaseOrder, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := r.movePO(ctx, tx, orgID, id, POclosed); err != nil {
		return PurchaseOrder{}, err
	}
	var got float64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(qty_received),0)::float8 FROM adrient.inv_purchase_order_lines WHERE po_id=$1`, id).Scan(&got); err != nil {
		return PurchaseOrder{}, err
	}
	if got <= 0 {
		return PurchaseOrder{}, invalid("nothing was received; cancel the order instead")
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.inv_purchase_orders SET state='closed', closed_at=now(), updated_at=now() WHERE org_id=$1 AND id=$2`, orgID, id); err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, err
	}
	return r.GetPO(ctx, orgID, id)
}

// ---------------------------------------------------------- purchase requests

// PurchaseRequest asks for goods to be bought; once approved it converts to POs.
type PurchaseRequest struct {
	ID        string        `json:"id"`
	Number    string        `json:"number"`
	State     string        `json:"state"`
	Note      string        `json:"note"`
	NeededBy  string        `json:"needed_by"`
	CreatedAt time.Time     `json:"created_at"`
	POIDs     []string      `json:"po_ids"`
	Lines     []RequestLine `json:"lines"`
}

// RequestLine is one requested product.
type RequestLine struct {
	ProductID   string  `json:"product_id"`
	ProductName string  `json:"product_name"`
	Qty         float64 `json:"qty"`
	Note        string  `json:"note"`
}

// RequestInput creates a purchase request.
type RequestInput struct {
	Note     string `json:"note"`
	NeededBy string `json:"needed_by"`
	Submit   bool   `json:"submit"`
	Lines    []struct {
		ProductID string  `json:"product_id"`
		Qty       float64 `json:"qty"`
		Note      string  `json:"note"`
	} `json:"lines"`
}

// CreateRequest saves a purchase request (submitted when asked).
func (r *Repository) CreateRequest(ctx context.Context, orgID, userID string, in RequestInput) (PurchaseRequest, error) {
	if len(in.Lines) == 0 {
		return PurchaseRequest{}, invalid("add at least one product")
	}
	nb, err := parseDate(in.NeededBy)
	if err != nil {
		return PurchaseRequest{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PurchaseRequest{}, err
	}
	defer tx.Rollback(ctx)
	num, err := nextNumber(ctx, tx, orgID, "pr", "PR")
	if err != nil {
		return PurchaseRequest{}, err
	}
	state := "draft"
	if in.Submit {
		state = "submitted"
	}
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_purchase_requests (org_id, number, state, note, needed_by, requested_by) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid) RETURNING id::text`,
		orgID, num, state, in.Note, nb, userID).Scan(&id); err != nil {
		return PurchaseRequest{}, err
	}
	for _, l := range in.Lines {
		if l.Qty <= 0 {
			return PurchaseRequest{}, invalid("quantities must be positive")
		}
		tag, err := tx.Exec(ctx, `INSERT INTO adrient.inv_purchase_request_lines (request_id, org_id, product_id, qty, note)
			SELECT $1,$2,p.id,$4,$5 FROM adrient.inv_products p WHERE p.org_id=$2 AND p.id=$3`, id, orgID, l.ProductID, round3(l.Qty), l.Note)
		if err != nil {
			return PurchaseRequest{}, err
		}
		if tag.RowsAffected() == 0 {
			return PurchaseRequest{}, invalid("product not found")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseRequest{}, err
	}
	return r.GetRequest(ctx, orgID, id)
}

// GetRequest loads one request.
func (r *Repository) GetRequest(ctx context.Context, orgID, id string) (PurchaseRequest, error) {
	rs, err := r.ListRequests(ctx, orgID, id)
	if err != nil {
		return PurchaseRequest{}, err
	}
	if len(rs) == 0 {
		return PurchaseRequest{}, ErrNotFound
	}
	return rs[0], nil
}

// ListRequests lists requests (or one by id).
func (r *Repository) ListRequests(ctx context.Context, orgID, onlyID string) ([]PurchaseRequest, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, number, state, note, COALESCE(to_char(needed_by,'YYYY-MM-DD'),''), created_at
		 FROM adrient.inv_purchase_requests WHERE org_id=$1 AND ($2='' OR id::text=$2) ORDER BY created_at DESC LIMIT 300`, orgID, onlyID)
	if err != nil {
		return nil, err
	}
	out := []PurchaseRequest{}
	idx := map[string]int{}
	var ids []string
	for rows.Next() {
		var x PurchaseRequest
		if err := rows.Scan(&x.ID, &x.Number, &x.State, &x.Note, &x.NeededBy, &x.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		x.Lines, x.POIDs = []RequestLine{}, []string{}
		idx[x.ID] = len(out)
		out = append(out, x)
		ids = append(ids, x.ID)
	}
	rows.Close()
	lr, err := r.pool.Query(ctx,
		`SELECT l.request_id::text, l.product_id::text, p.name, l.qty::float8, l.note FROM adrient.inv_purchase_request_lines l
		 JOIN adrient.inv_products p ON p.id=l.product_id WHERE l.org_id=$1 AND l.request_id = ANY($2::uuid[]) ORDER BY l.id`, orgID, ids)
	if err != nil {
		return nil, err
	}
	for lr.Next() {
		var rid string
		var l RequestLine
		if err := lr.Scan(&rid, &l.ProductID, &l.ProductName, &l.Qty, &l.Note); err != nil {
			lr.Close()
			return nil, err
		}
		out[idx[rid]].Lines = append(out[idx[rid]].Lines, l)
	}
	lr.Close()
	pr, err := r.pool.Query(ctx, `SELECT request_id::text, id::text FROM adrient.inv_purchase_orders WHERE org_id=$1 AND request_id = ANY($2::uuid[]) ORDER BY created_at`, orgID, ids)
	if err != nil {
		return nil, err
	}
	defer pr.Close()
	for pr.Next() {
		var rid, pid string
		if err := pr.Scan(&rid, &pid); err != nil {
			return nil, err
		}
		out[idx[rid]].POIDs = append(out[idx[rid]].POIDs, pid)
	}
	return out, pr.Err()
}

// DecideRequest submits, approves or rejects a request.
func (r *Repository) DecideRequest(ctx context.Context, orgID, userID, id, action string) (PurchaseRequest, error) {
	next := map[string]string{"submit": "submitted", "approve": "approved", "reject": "rejected"}[action]
	from := map[string]string{"submit": "draft", "approve": "submitted", "reject": "submitted"}[action]
	if next == "" {
		return PurchaseRequest{}, invalid("unknown action")
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.inv_purchase_requests SET state=$4, decided_by=CASE WHEN $4 IN ('approved','rejected') THEN NULLIF($5,'')::uuid ELSE decided_by END
		WHERE org_id=$1 AND id=$2 AND state=$3`, orgID, id, from, next, userID)
	if err != nil {
		return PurchaseRequest{}, err
	}
	if tag.RowsAffected() == 0 {
		return PurchaseRequest{}, invalid("a request must be %s before it can be %sed", from, action)
	}
	return r.GetRequest(ctx, orgID, id)
}

// ConvertRequest turns an approved request into draft purchase orders, one per
// supplier (the preferred supplier of each product, else the cheapest price list).
func (r *Repository) ConvertRequest(ctx context.Context, orgID, userID, id string) ([]PurchaseOrder, error) {
	req, err := r.GetRequest(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if req.State != "approved" {
		return nil, invalid("only an approved request can be converted")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	groups := map[string]*POInput{}
	var order []string
	for _, l := range req.Lines {
		sup, err := r.bestSupplier(ctx, tx, orgID, l.ProductID, l.Qty)
		if err != nil {
			return nil, err
		}
		if sup == "" {
			return nil, invalid("%s has no supplier; add one to the product first", l.ProductName)
		}
		g := groups[sup]
		if g == nil {
			g = &POInput{SupplierID: sup, RequestID: id, Notes: "From purchase request " + req.Number}
			groups[sup] = g
			order = append(order, sup)
		}
		g.Lines = append(g.Lines, struct {
			ProductID   string   `json:"product_id"`
			Description string   `json:"description"`
			Qty         float64  `json:"qty"`
			UnitCost    *float64 `json:"unit_cost"`
			TaxPct      float64  `json:"tax_pct"`
		}{ProductID: l.ProductID, Qty: l.Qty})
	}
	var ids []string
	for _, s := range order {
		pid, err := r.createPOTx(ctx, tx, orgID, userID, *groups[s])
		if err != nil {
			return nil, err
		}
		ids = append(ids, pid)
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.inv_purchase_requests SET state='converted' WHERE id=$1`, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	var out []PurchaseOrder
	for _, pid := range ids {
		o, err := r.GetPO(ctx, orgID, pid)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// bestSupplier picks the supplier for a product: preferred, else lowest price.
func (r *Repository) bestSupplier(ctx context.Context, tx pgx.Tx, orgID, productID string, qty float64) (string, error) {
	var sid string
	err := tx.QueryRow(ctx,
		`SELECT supplier_id::text FROM adrient.inv_product_suppliers WHERE org_id=$1 AND product_id=$2 AND preferred LIMIT 1`, orgID, productID).Scan(&sid)
	if err == nil {
		return sid, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	err = tx.QueryRow(ctx,
		`SELECT supplier_id::text FROM (
		   SELECT supplier_id, unit_cost FROM adrient.inv_supplier_prices WHERE org_id=$1 AND product_id=$2 AND min_qty <= $3
		     AND (valid_from IS NULL OR valid_from <= CURRENT_DATE) AND (valid_to IS NULL OR valid_to >= CURRENT_DATE)
		   UNION ALL SELECT supplier_id, cost FROM adrient.inv_product_suppliers WHERE org_id=$1 AND product_id=$2
		 ) x ORDER BY unit_cost LIMIT 1`, orgID, productID, qty).Scan(&sid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return sid, err
}
