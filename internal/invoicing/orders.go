package invoicing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// QtyRef is a quantity against an order line.
type QtyRef struct {
	Ref string  `json:"ref"`
	Qty float64 `json:"qty"`
}

// LineProgress is where one order line stands. Delivered counts dispatched
// delivery notes, Packed counts draft ones (they already claim the quantity so
// two notes cannot promise the same goods), Invoiced counts live invoices.
type LineProgress struct {
	Ref       string  `json:"ref"`
	Ordered   float64 `json:"ordered"`
	Delivered float64 `json:"delivered"`
	Packed    float64 `json:"packed"`
	Invoiced  float64 `json:"invoiced"`
	ToPack    float64 `json:"to_pack"`    // ordered - delivered - packed
	ToInvoice float64 `json:"to_invoice"` // ordered - invoiced
	Billable  float64 `json:"billable"`   // delivered - invoiced, never below zero
}

// Progress folds delivery and invoice quantities into per-line progress. It is
// pure so the partial delivery / partial invoicing arithmetic can be tested
// without a database. dispatched and drafts are the quantities of delivery
// notes in each state; invoiced those of live invoices (keyed by order line ref).
func Progress(lines []Line, dispatched, drafts, invoiced []QtyRef) []LineProgress {
	sum := func(q []QtyRef) map[string]float64 {
		m := map[string]float64{}
		for _, x := range q {
			m[x.Ref] += x.Qty
		}
		return m
	}
	d, p, inv := sum(dispatched), sum(drafts), sum(invoiced)
	out := make([]LineProgress, len(lines))
	for i, l := range lines {
		lp := LineProgress{Ref: l.Ref, Ordered: l.Qty, Delivered: d[l.Ref], Packed: p[l.Ref], Invoiced: inv[l.Ref]}
		lp.ToPack = clamp0(l.Qty - lp.Delivered - lp.Packed)
		lp.ToInvoice = clamp0(l.Qty - lp.Invoiced)
		lp.Billable = clamp0(lp.Delivered - lp.Invoiced)
		out[i] = lp
	}
	return out
}

func clamp0(v float64) float64 {
	v = float64(int64(v*1000+0.5*sign(v))) / 1000 // 3 decimals, quantities are not money
	if v < 0 {
		return 0
	}
	return v
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// Fulfilment and billing labels derived from progress.
func orderStates(ps []LineProgress) (fulfilment, billing string) {
	var ord, del, inv float64
	for _, p := range ps {
		ord += p.Ordered
		del += p.Delivered
		inv += p.Invoiced
	}
	switch {
	case ord <= 0 || del <= 0:
		fulfilment = "not_delivered"
	case del+0.0005 >= ord:
		fulfilment = "delivered"
	default:
		fulfilment = "partially_delivered"
	}
	switch {
	case inv <= 0:
		billing = "not_invoiced"
	case inv+0.0005 >= ord:
		billing = "invoiced"
	default:
		billing = "partially_invoiced"
	}
	return
}

// CheckQty validates requested quantities against a limit per line ref.
func CheckQty(req []QtyRef, limit map[string]float64, what string) error {
	seen := map[string]bool{}
	any := false
	for _, q := range req {
		if q.Qty < 0 {
			return invalid("quantities must not be negative")
		}
		if q.Qty == 0 {
			continue
		}
		any = true
		if seen[q.Ref] {
			return invalid("line %s is listed twice", q.Ref)
		}
		seen[q.Ref] = true
		lim, ok := limit[q.Ref]
		if !ok {
			return invalid("unknown order line %s", q.Ref)
		}
		if q.Qty > lim+0.0005 {
			return invalid("line %s: %s %.3g exceeds what is left (%.3g)", q.Ref, what, q.Qty, lim)
		}
	}
	if !any {
		return invalid("enter a quantity for at least one line")
	}
	return nil
}

// ---- types ----

// OrderInput is the editable part of a sales order.
type OrderInput struct {
	ContactID     string  `json:"contact_id"`
	ClientName    string  `json:"client_name"`
	ClientEmail   string  `json:"client_email"`
	ClientAddress string  `json:"client_address"`
	ClientGSTIN   string  `json:"client_gstin"`
	ShipTo        string  `json:"ship_to"`
	Currency      string  `json:"currency"`
	OrderDate     string  `json:"order_date"`
	ExpectedDate  string  `json:"expected_date"`
	PlaceOfSupply string  `json:"place_of_supply"`
	Lines         []Line  `json:"lines"`
	DiscountKind  string  `json:"discount_kind"`
	DiscountValue float64 `json:"discount_value"`
	Notes         string  `json:"notes"`
	Terms         string  `json:"terms"`
	PriceListID   string  `json:"price_list_id"`
	EstimateID    string  `json:"estimate_id"`
	SourceRef     string  `json:"source_ref"`
}

// Order is a sales order with derived progress.
type Order struct {
	OrderInput
	ID            string         `json:"id"`
	Number        string         `json:"number"`
	Status        string         `json:"status"` // draft | confirmed | cancelled
	Subtotal      float64        `json:"subtotal"`
	DiscountTotal float64        `json:"discount_total"`
	TaxTotal      float64        `json:"tax_total"`
	Total         float64        `json:"total"`
	CGST          float64        `json:"cgst_total"`
	SGST          float64        `json:"sgst_total"`
	IGST          float64        `json:"igst_total"`
	Progress      []LineProgress `json:"progress"`
	Fulfilment    string         `json:"fulfilment"`
	Billing       string         `json:"billing"`
	Deliveries    []Delivery     `json:"deliveries"`
	Invoices      []OrderInvoice `json:"invoices"`
	EstimateNo    string         `json:"estimate_number"`
	CreatedAt     string         `json:"created_at"`
}

// OrderInvoice is a short reference to an invoice raised from an order.
type OrderInvoice struct {
	ID     string  `json:"id"`
	Number string  `json:"number"`
	Total  float64 `json:"total"`
	Status string  `json:"status"`
}

// DeliveryLine is one packed line.
type DeliveryLine struct {
	Ref         string  `json:"ref"`
	Description string  `json:"description"`
	Unit        string  `json:"unit"`
	ProductID   string  `json:"product_id"`
	HSNSAC      string  `json:"hsn_sac"`
	Qty         float64 `json:"qty"`
}

// Delivery is a delivery note (packing slip).
type Delivery struct {
	ID           string         `json:"id"`
	OrderID      string         `json:"order_id"`
	OrderNumber  string         `json:"order_number"`
	Number       string         `json:"number"`
	Status       string         `json:"status"` // draft | dispatched | cancelled
	Lines        []DeliveryLine `json:"lines"`
	ShipTo       string         `json:"ship_to"`
	Carrier      string         `json:"carrier"`
	Tracking     string         `json:"tracking"`
	Notes        string         `json:"notes"`
	DeliveryDate string         `json:"delivery_date"`
	StockMoved   bool           `json:"stock_moved"`
	ClientName   string         `json:"client_name"`
	ClientAddr   string         `json:"client_address"`
	CreatedAt    string         `json:"created_at"`
}

// ---- orders ----

func (r *Repository) orderTotals(ctx context.Context, orgID string, in *OrderInput) (Totals, *Breakup, error) {
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return Totals{}, nil, err
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = s.DefaultCurrency
	}
	if in.DiscountKind != "amount" {
		in.DiscountKind = "percent"
	}
	if in.DiscountValue < 0 {
		return Totals{}, nil, invalid("discount must not be negative")
	}
	if in.OrderDate == "" {
		in.OrderDate = today().Format("2006-01-02")
	}
	if parseDate(in.OrderDate) == nil || (in.ExpectedDate != "" && parseDate(in.ExpectedDate) == nil) {
		return Totals{}, nil, invalid("dates must be YYYY-MM-DD")
	}
	if len(in.Lines) == 0 {
		return Totals{}, nil, invalid("add at least one line")
	}
	used := map[string]bool{}
	for i := range in.Lines {
		l := &in.Lines[i]
		if l.Qty <= 0 || l.Price < 0 || l.TaxRate < 0 {
			return Totals{}, nil, invalid("line %d: quantity must be positive, price and tax not negative", i+1)
		}
		if l.Ref != "" {
			if used[l.Ref] {
				return Totals{}, nil, invalid("duplicate line reference %s", l.Ref)
			}
			used[l.Ref] = true
		}
	}
	n := 0
	for i := range in.Lines { // new lines get the next free stable key
		if in.Lines[i].Ref == "" {
			for {
				n++
				k := "l" + strconv.Itoa(n)
				if !used[k] {
					in.Lines[i].Ref, used[k] = k, true
					break
				}
			}
		}
	}
	if err := validateExtInput(&DocInput{PlaceOfSupply: in.PlaceOfSupply, ClientGSTIN: in.ClientGSTIN, PriceListID: in.PriceListID}); err != nil {
		return Totals{}, nil, err
	}
	di := DocInput{Kind: "invoice", Lines: in.Lines, DiscountKind: in.DiscountKind, DiscountValue: in.DiscountValue,
		PlaceOfSupply: in.PlaceOfSupply, ClientGSTIN: in.ClientGSTIN, IssueDate: in.OrderDate}
	t := Compute(in.Lines, in.DiscountKind, in.DiscountValue)
	b := applyGST(&di, &t, s)
	in.PlaceOfSupply, in.ClientGSTIN = di.PlaceOfSupply, di.ClientGSTIN
	return t, b, nil
}

const orderCols = `id::text, number, status, COALESCE(contact_id::text,''), client_name, client_email, client_address, client_gstin,
	ship_to, currency, to_char(order_date,'YYYY-MM-DD'), COALESCE(to_char(expected_date,'YYYY-MM-DD'),''), place_of_supply, lines,
	discount_kind, discount_value::float8, subtotal::float8, discount_total::float8, tax_total::float8, total::float8, notes, terms,
	COALESCE(estimate_id::text,''), source_ref, COALESCE(price_list_id::text,''), to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')`

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	var lines []byte
	err := row.Scan(&o.ID, &o.Number, &o.Status, &o.ContactID, &o.ClientName, &o.ClientEmail, &o.ClientAddress, &o.ClientGSTIN,
		&o.ShipTo, &o.Currency, &o.OrderDate, &o.ExpectedDate, &o.PlaceOfSupply, &lines, &o.DiscountKind, &o.DiscountValue,
		&o.Subtotal, &o.DiscountTotal, &o.TaxTotal, &o.Total, &o.Notes, &o.Terms, &o.EstimateID, &o.SourceRef, &o.PriceListID, &o.CreatedAt)
	o.Lines = []Line{}
	_ = json.Unmarshal(lines, &o.Lines)
	return o, err
}

// SaveOrder creates (id empty) or updates an order. A cancelled order and an
// order that already has deliveries or invoices keep their lines (only notes,
// dates and addresses can change) so history never disagrees with the order.
func (r *Repository) SaveOrder(ctx context.Context, orgID, userID, id string, in OrderInput) (Order, error) {
	if id != "" {
		cur, err := r.GetOrder(ctx, orgID, id)
		if err != nil {
			return cur, err
		}
		if cur.Status == "cancelled" {
			return cur, invalid("a cancelled order cannot be edited")
		}
		if len(cur.Deliveries) > 0 || len(cur.Invoices) > 0 {
			in.Lines, in.DiscountKind, in.DiscountValue = cur.Lines, cur.DiscountKind, cur.DiscountValue
		}
	}
	t, b, err := r.orderTotals(ctx, orgID, &in)
	if err != nil {
		return Order{}, err
	}
	lines, _ := json.Marshal(in.Lines)
	var cg, sg, ig float64
	if b != nil {
		cg, sg, ig = b.CGST, b.SGST, b.IGST
	}
	_ = cg
	_ = sg
	_ = ig
	if id != "" {
		err := r.exec(ctx, `UPDATE adrient.sales_orders SET contact_id=NULLIF($3,'')::uuid, client_name=$4, client_email=$5, client_address=$6,
			client_gstin=$7, ship_to=$8, currency=$9, order_date=$10::date, expected_date=NULLIF($11,'')::date, place_of_supply=$12, lines=$13,
			discount_kind=$14, discount_value=$15, subtotal=$16, discount_total=$17, tax_total=$18, total=$19, notes=$20, terms=$21,
			price_list_id=NULLIF($22,'')::uuid, updated_at=now() WHERE org_id=$1 AND id=$2::uuid AND deleted_at IS NULL`,
			orgID, id, in.ContactID, in.ClientName, in.ClientEmail, in.ClientAddress, in.ClientGSTIN, in.ShipTo, in.Currency,
			in.OrderDate, in.ExpectedDate, in.PlaceOfSupply, lines, in.DiscountKind, in.DiscountValue, t.Subtotal, t.DiscountTotal,
			t.TaxTotal, t.Total, in.Notes, in.Terms, in.PriceListID)
		if err != nil {
			return Order{}, err
		}
		r.SyncReservations(ctx, orgID, userID, id)
		return r.GetOrder(ctx, orgID, id)
	}
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return Order{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	n, err := nextSeq(ctx, tx, orgID, "order")
	if err != nil {
		return Order{}, err
	}
	var nid string
	err = tx.QueryRow(ctx, `INSERT INTO adrient.sales_orders (org_id, number, contact_id, client_name, client_email, client_address, client_gstin,
		ship_to, currency, order_date, expected_date, place_of_supply, lines, discount_kind, discount_value, subtotal, discount_total, tax_total,
		total, notes, terms, estimate_id, source_ref, price_list_id, created_by)
		VALUES ($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10::date,NULLIF($11,'')::date,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,
		NULLIF($22,'')::uuid,$23,NULLIF($24,'')::uuid,NULLIF($25,'')::uuid) RETURNING id::text`,
		orgID, fmt.Sprintf("%s%04d", s.Sales.OrderPrefix, n), in.ContactID, in.ClientName, in.ClientEmail, in.ClientAddress, in.ClientGSTIN,
		in.ShipTo, in.Currency, in.OrderDate, in.ExpectedDate, in.PlaceOfSupply, lines, in.DiscountKind, in.DiscountValue, t.Subtotal,
		t.DiscountTotal, t.TaxTotal, t.Total, in.Notes, in.Terms, in.EstimateID, in.SourceRef, in.PriceListID, userID).Scan(&nid)
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return r.GetOrder(ctx, orgID, nid)
}

// GetOrder returns an order with progress, deliveries and invoices.
func (r *Repository) GetOrder(ctx context.Context, orgID, id string) (Order, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx, `SELECT `+orderCols+` FROM adrient.sales_orders WHERE org_id=$1 AND id=$2::uuid AND deleted_at IS NULL`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return o, ErrNotFound
	}
	if err != nil {
		return o, err
	}
	return r.fillOrder(ctx, orgID, o)
}

func (r *Repository) fillOrder(ctx context.Context, orgID string, o Order) (Order, error) {
	var err error
	o.Deliveries, err = r.deliveries(ctx, orgID, `d.order_id=$2::uuid`, o.ID)
	if err != nil {
		return o, err
	}
	var disp, draft []QtyRef
	for _, d := range o.Deliveries {
		for _, l := range d.Lines {
			switch d.Status {
			case "dispatched":
				disp = append(disp, QtyRef{l.Ref, l.Qty})
			case "draft":
				draft = append(draft, QtyRef{l.Ref, l.Qty})
			}
		}
	}
	var inv []QtyRef
	o.Invoices = []OrderInvoice{}
	rows, err := r.pool.Query(ctx, `SELECT id::text, number, total::float8, status, lines FROM adrient.invc_documents
		WHERE org_id=$1 AND sales_order_id=$2::uuid AND kind='invoice' AND deleted_at IS NULL ORDER BY created_at`, orgID, o.ID)
	if err != nil {
		return o, err
	}
	defer rows.Close()
	for rows.Next() {
		var oi OrderInvoice
		var lb []byte
		if err := rows.Scan(&oi.ID, &oi.Number, &oi.Total, &oi.Status, &lb); err != nil {
			return o, err
		}
		o.Invoices = append(o.Invoices, oi)
		var ls []Line
		_ = json.Unmarshal(lb, &ls)
		for _, l := range ls {
			if l.SOLine != "" {
				inv = append(inv, QtyRef{l.SOLine, l.Qty})
			}
		}
	}
	o.Progress = Progress(o.Lines, disp, draft, inv)
	o.Fulfilment, o.Billing = orderStates(o.Progress)
	if o.EstimateID != "" {
		_ = r.pool.QueryRow(ctx, `SELECT number FROM adrient.invc_documents WHERE org_id=$1 AND id=$2::uuid`, orgID, o.EstimateID).Scan(&o.EstimateNo)
	}
	if o.Status != "cancelled" && o.Status != "draft" {
		o.Status = "confirmed"
	}
	return o, rows.Err()
}

// ListOrders returns orders newest first with progress.
func (r *Repository) ListOrders(ctx context.Context, orgID, status, query string) ([]Order, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+orderCols+` FROM adrient.sales_orders WHERE org_id=$1 AND deleted_at IS NULL
		AND ($2='' OR status=$2) AND ($3='' OR lower(number||' '||client_name) LIKE '%'||lower($3)||'%')
		ORDER BY order_date DESC, created_at DESC LIMIT 1000`, orgID, status, strings.TrimSpace(query))
	if err != nil {
		return nil, err
	}
	var os []Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		os = append(os, o)
	}
	rows.Close()
	out := make([]Order, 0, len(os))
	for _, o := range os {
		o, err = r.fillOrder(ctx, orgID, o)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// SetOrderStatus confirms or cancels an order. An order with dispatched
// deliveries or invoices cannot be cancelled.
func (r *Repository) SetOrderStatus(ctx context.Context, orgID, id, status string) (Order, error) {
	o, err := r.GetOrder(ctx, orgID, id)
	if err != nil {
		return o, err
	}
	switch status {
	case "confirmed":
		if o.Status == "cancelled" {
			return o, invalid("a cancelled order cannot be confirmed")
		}
	case "cancelled":
		for _, d := range o.Deliveries {
			if d.Status != "cancelled" {
				return o, invalid("cancel its delivery notes first")
			}
		}
		if len(o.Invoices) > 0 {
			return o, invalid("an order with invoices cannot be cancelled")
		}
	default:
		return o, invalid("status must be confirmed or cancelled")
	}
	if err := r.exec(ctx, `UPDATE adrient.sales_orders SET status=$3, updated_at=now() WHERE org_id=$1 AND id=$2::uuid`, orgID, id, status); err != nil {
		return o, err
	}
	r.SyncReservations(ctx, orgID, "", id)
	return r.GetOrder(ctx, orgID, id)
}

// OrderFromEstimate creates a confirmed sales order from an accepted (or sent)
// estimate and marks the estimate converted. It is idempotent: calling it again
// returns the same order.
func (r *Repository) OrderFromEstimate(ctx context.Context, orgID, userID, estimateID string) (Order, error) {
	var existing string
	err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.sales_orders WHERE org_id=$1 AND estimate_id=$2::uuid AND deleted_at IS NULL`, orgID, estimateID).Scan(&existing)
	if err == nil {
		return r.GetOrder(ctx, orgID, existing)
	}
	e, err := r.GetDocument(ctx, orgID, estimateID)
	if err != nil {
		return Order{}, err
	}
	if e.Kind != "estimate" || e.Deleted {
		return Order{}, invalid("not an estimate")
	}
	if e.Status != "accepted" && e.Status != "sent" {
		return Order{}, invalid("an estimate must be sent or accepted to become an order (it is %s)", e.DisplayStatus)
	}
	if e.DisplayStatus == "expired" {
		return Order{}, invalid("this estimate has expired")
	}
	lines := make([]Line, len(e.Lines))
	copy(lines, e.Lines)
	for i := range lines {
		lines[i].Ref = ""
	}
	o, err := r.SaveOrder(ctx, orgID, userID, "", OrderInput{ContactID: e.ContactID, ClientName: e.ClientName, ClientEmail: e.ClientEmail,
		ClientAddress: e.ClientAddress, ClientGSTIN: e.ClientGSTIN, Currency: e.Currency, PlaceOfSupply: e.PlaceOfSupply, Lines: lines,
		DiscountKind: e.DiscountKind, DiscountValue: e.DiscountValue, Notes: e.Notes, Terms: e.Terms, EstimateID: e.ID,
		SourceRef: "estimate:" + e.Number, PriceListID: e.PriceListID})
	if err != nil {
		return o, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.sales_orders SET status='confirmed' WHERE org_id=$1 AND id=$2::uuid`, orgID, o.ID); err != nil {
		return o, err
	}
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET status='converted', sales_order_id=$3::uuid WHERE org_id=$1 AND id=$2::uuid`, orgID, e.ID, o.ID)
	r.SyncReservations(ctx, orgID, userID, o.ID)
	return r.GetOrder(ctx, orgID, o.ID)
}

// ---- deliveries ----

func (r *Repository) deliveries(ctx context.Context, orgID, where string, arg string) ([]Delivery, error) {
	rows, err := r.pool.Query(ctx, `SELECT d.id::text, d.order_id::text, o.number, d.number, d.status, d.lines, d.ship_to, d.carrier, d.tracking,
		d.notes, COALESCE(to_char(d.delivery_date,'YYYY-MM-DD'),''), d.stock_moved, o.client_name, o.client_address,
		to_char(d.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM adrient.sales_delivery_notes d JOIN adrient.sales_orders o ON o.id=d.order_id
		WHERE d.org_id=$1 AND `+where+` ORDER BY d.created_at`, orgID, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Delivery{}
	for rows.Next() {
		var d Delivery
		var lb []byte
		if err := rows.Scan(&d.ID, &d.OrderID, &d.OrderNumber, &d.Number, &d.Status, &lb, &d.ShipTo, &d.Carrier, &d.Tracking, &d.Notes,
			&d.DeliveryDate, &d.StockMoved, &d.ClientName, &d.ClientAddr, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Lines = []DeliveryLine{}
		_ = json.Unmarshal(lb, &d.Lines)
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeliveryInput creates a delivery note for part of an order.
type DeliveryInput struct {
	Lines    []QtyRef `json:"lines"`
	ShipTo   string   `json:"ship_to"`
	Carrier  string   `json:"carrier"`
	Tracking string   `json:"tracking"`
	Notes    string   `json:"notes"`
}

// ListDeliveries returns every delivery note, newest first.
func (r *Repository) ListDeliveries(ctx context.Context, orgID string) ([]Delivery, error) {
	d, err := r.deliveries(ctx, orgID, `$2=$2`, "x")
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(d)-1; i < j; i, j = i+1, j-1 {
		d[i], d[j] = d[j], d[i]
	}
	return d, nil
}

// GetDelivery returns one delivery note.
func (r *Repository) GetDelivery(ctx context.Context, orgID, id string) (Delivery, error) {
	ds, err := r.deliveries(ctx, orgID, `d.id=$2::uuid`, id)
	if err != nil {
		return Delivery{}, err
	}
	if len(ds) == 0 {
		return Delivery{}, ErrNotFound
	}
	return ds[0], nil
}

// CreateDelivery packs part of an order. Quantities are limited to what is not
// already delivered or packed, so the sum over all notes never exceeds the order.
func (r *Repository) CreateDelivery(ctx context.Context, orgID, userID, orderID string, in DeliveryInput) (Delivery, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Delivery{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// Serialise concurrent deliveries of one order so two requests cannot both
	// claim the last units.
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM adrient.sales_orders WHERE org_id=$1 AND id=$2::uuid AND deleted_at IS NULL FOR UPDATE`, orgID, orderID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Delivery{}, ErrNotFound
		}
		return Delivery{}, err
	}
	if status != "confirmed" {
		return Delivery{}, invalid("confirm the order before delivering it")
	}
	o, err := r.GetOrder(ctx, orgID, orderID)
	if err != nil {
		return Delivery{}, err
	}
	limit := map[string]float64{}
	byRef := map[string]Line{}
	for i, l := range o.Lines {
		limit[l.Ref] = o.Progress[i].ToPack
		byRef[l.Ref] = l
	}
	if err := CheckQty(in.Lines, limit, "delivery quantity"); err != nil {
		return Delivery{}, err
	}
	var dl []DeliveryLine
	for _, q := range in.Lines {
		if q.Qty <= 0 {
			continue
		}
		l := byRef[q.Ref]
		dl = append(dl, DeliveryLine{Ref: q.Ref, Description: l.Description, Unit: l.Unit, ProductID: l.ProductID, HSNSAC: l.HSNSAC, Qty: q.Qty})
	}
	s, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return Delivery{}, err
	}
	n, err := nextSeq(ctx, tx, orgID, "delivery")
	if err != nil {
		return Delivery{}, err
	}
	lb, _ := json.Marshal(dl)
	ship := strings.TrimSpace(in.ShipTo)
	if ship == "" {
		ship = o.ShipTo
	}
	if ship == "" {
		ship = o.ClientAddress
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.sales_delivery_notes (org_id, order_id, number, lines, ship_to, carrier, tracking, notes, created_by)
		VALUES ($1,$2::uuid,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid) RETURNING id::text`, orgID, orderID,
		fmt.Sprintf("%s%04d", s.Sales.DeliveryPrefix, n), lb, ship, in.Carrier, in.Tracking, in.Notes, userID).Scan(&id); err != nil {
		return Delivery{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Delivery{}, err
	}
	return r.GetDelivery(ctx, orgID, id)
}

// MarkDispatched flips a draft delivery note to dispatched. The caller moves
// stock (once) afterwards with MarkStockMoved.
func (r *Repository) MarkDispatched(ctx context.Context, orgID, id, date string) (Delivery, error) {
	if date == "" {
		date = today().Format("2006-01-02")
	}
	if parseDate(date) == nil {
		return Delivery{}, invalid("date must be YYYY-MM-DD")
	}
	if err := r.exec(ctx, `UPDATE adrient.sales_delivery_notes SET status='dispatched', delivery_date=$3::date, updated_at=now()
		WHERE org_id=$1 AND id=$2::uuid AND status='draft'`, orgID, id, date); err != nil {
		if errors.Is(err, ErrNotFound) {
			return Delivery{}, invalid("only a draft delivery note can be dispatched")
		}
		return Delivery{}, err
	}
	return r.GetDelivery(ctx, orgID, id)
}

// MarkStockMoved records that dispatch moved stock for this note.
func (r *Repository) MarkStockMoved(ctx context.Context, orgID, id string) error {
	return r.exec(ctx, `UPDATE adrient.sales_delivery_notes SET stock_moved=true WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
}

// CancelDelivery cancels a draft delivery note (a dispatched one has moved
// goods and stock and cannot be cancelled).
func (r *Repository) CancelDelivery(ctx context.Context, orgID, id string) (Delivery, error) {
	if err := r.exec(ctx, `UPDATE adrient.sales_delivery_notes SET status='cancelled', updated_at=now() WHERE org_id=$1 AND id=$2::uuid AND status='draft'`, orgID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return Delivery{}, invalid("only a draft delivery note can be cancelled")
		}
		return Delivery{}, err
	}
	return r.GetDelivery(ctx, orgID, id)
}

// ---- invoicing an order ----

// OrderInvoiceInput says what to bill: explicit quantities, or mode
// "delivered" (everything delivered and not yet billed), or "remaining".
type OrderInvoiceInput struct {
	Mode  string   `json:"mode"`
	Lines []QtyRef `json:"lines"`
}

// InvoiceOrder raises a draft invoice for part (or all) of an order. Billed
// quantities are limited to ordered minus already invoiced, so the invoices of
// one order never add up to more than the order. Orders move stock at dispatch,
// so these invoices never deduct stock again.
func (r *Repository) InvoiceOrder(ctx context.Context, orgID, userID, orderID string, in OrderInvoiceInput) (Document, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Document{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM adrient.sales_orders WHERE org_id=$1 AND id=$2::uuid AND deleted_at IS NULL FOR UPDATE`, orgID, orderID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Document{}, ErrNotFound
		}
		return Document{}, err
	}
	if status != "confirmed" {
		return Document{}, invalid("confirm the order before invoicing it")
	}
	o, err := r.GetOrder(ctx, orgID, orderID)
	if err != nil {
		return Document{}, err
	}
	req := in.Lines
	limit := map[string]float64{}
	switch in.Mode {
	case "delivered", "remaining":
		req = nil
		for i, l := range o.Lines {
			q := o.Progress[i].ToInvoice
			if in.Mode == "delivered" {
				q = o.Progress[i].Billable
			}
			limit[l.Ref] = o.Progress[i].ToInvoice
			if q > 0 {
				req = append(req, QtyRef{l.Ref, q})
			}
		}
		if len(req) == 0 {
			return Document{}, invalid("nothing to invoice")
		}
	default:
		for i, l := range o.Lines {
			limit[l.Ref] = o.Progress[i].ToInvoice
		}
	}
	if err := CheckQty(req, limit, "invoice quantity"); err != nil {
		return Document{}, err
	}
	var lines []Line
	var billed, full float64
	byRef := map[string]Line{}
	for _, l := range o.Lines {
		byRef[l.Ref] = l
		full += l.Qty * l.Price * (1 - clampPct(l.DiscountPct)/100)
	}
	for _, q := range req {
		if q.Qty <= 0 {
			continue
		}
		l := byRef[q.Ref]
		l.Qty, l.SOLine, l.Ref = q.Qty, q.Ref, ""
		billed += l.Qty * l.Price * (1 - clampPct(l.DiscountPct)/100)
		lines = append(lines, l)
	}
	dv := o.DiscountValue
	if o.DiscountKind == "amount" && full > 0 { // a fixed discount is billed pro rata
		dv = Round2(dv * billed / full)
	}
	doc, err := r.SaveDocument(ctx, orgID, userID, "", DocInput{Kind: "invoice", ContactID: o.ContactID, ClientName: o.ClientName,
		ClientEmail: o.ClientEmail, ClientAddress: o.ClientAddress, ClientGSTIN: o.ClientGSTIN, Currency: o.Currency,
		PlaceOfSupply: o.PlaceOfSupply, Lines: lines, DiscountKind: o.DiscountKind, DiscountValue: dv, Notes: o.Notes, Terms: o.Terms,
		SalesOrderID: o.ID, SourceRef: "so:" + o.Number, PriceListID: o.PriceListID, NoStock: true})
	if err != nil {
		return doc, err
	}
	return doc, tx.Commit(ctx)
}
