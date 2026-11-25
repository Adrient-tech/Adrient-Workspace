package invoicing

import (
	"context"
	"sort"
	"strings"
)

// PriceRow is one price in a price list. ContactID set means a price that
// only applies to that customer. MinQty makes it a quantity tier.
type PriceRow struct {
	ListID    string  `json:"price_list_id"`
	ProductID string  `json:"product_id"`
	ContactID string  `json:"contact_id"`
	MinQty    float64 `json:"min_qty"`
	Price     float64 `json:"price"`
}

// PriceSource says where a resolved price came from.
type PriceSource string

// Price sources, strongest first.
const (
	SrcCustomer PriceSource = "customer"   // a price set for this customer
	SrcAssigned PriceSource = "assigned"   // the price list assigned to this customer, or chosen on the document
	SrcDefault  PriceSource = "default"    // the org's default price list
	SrcBase     PriceSource = "base"       // the item's own price
)

// PriceCtx carries the lists that may apply to one customer.
type PriceCtx struct {
	ContactID string
	Assigned  map[string]bool // price list ids assigned to the customer or chosen on the document
	Default   map[string]bool // default price list ids
}

// ResolvePrice returns the unit price for a product and quantity. The order is:
// customer-specific rows, then rows of an assigned list, then rows of the
// default list, then the base price. Inside one level the quantity tier with
// the highest min_qty not above qty wins; a level without a matching tier is skipped.
func ResolvePrice(base float64, productID string, qty float64, ctx PriceCtx, rows []PriceRow) (float64, PriceSource) {
	levels := []struct {
		src PriceSource
		ok  func(PriceRow) bool
	}{
		{SrcCustomer, func(r PriceRow) bool { return ctx.ContactID != "" && r.ContactID == ctx.ContactID }},
		{SrcAssigned, func(r PriceRow) bool { return r.ContactID == "" && ctx.Assigned[r.ListID] }},
		{SrcDefault, func(r PriceRow) bool { return r.ContactID == "" && ctx.Default[r.ListID] }},
	}
	for _, lv := range levels {
		best, found := PriceRow{}, false
		for _, r := range rows {
			if r.ProductID != productID || !lv.ok(r) || r.MinQty > qty+1e-9 {
				continue
			}
			if !found || r.MinQty > best.MinQty || (r.MinQty == best.MinQty && r.Price < best.Price) {
				best, found = r, true
			}
		}
		if found {
			return Round2(best.Price), lv.src
		}
	}
	return Round2(base), SrcBase
}

// DiscountRule is a reusable discount. Scope "line" gives a percent discount on
// matching lines; scope "order" discounts the document (percent or amount).
type DiscountRule struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Scope     string  `json:"scope"`
	Kind      string  `json:"kind"`
	Value     float64 `json:"value"`
	MinQty    float64 `json:"min_qty"`
	MinAmount float64 `json:"min_amount"`
	ProductID string  `json:"product_id"`
	ContactID string  `json:"contact_id"`
	ValidFrom string  `json:"valid_from"`
	ValidTo   string  `json:"valid_to"`
	Priority  int     `json:"priority"`
	Active    bool    `json:"active"`
}

func (d DiscountRule) liveOn(date, contactID string) bool {
	if !d.Active {
		return false
	}
	if d.ContactID != "" && d.ContactID != contactID {
		return false
	}
	if d.ValidFrom != "" && date < d.ValidFrom {
		return false
	}
	if d.ValidTo != "" && date > d.ValidTo {
		return false
	}
	return true
}

// AppliedDiscount reports which rule produced a discount.
type AppliedDiscount struct {
	Rule  string  `json:"rule"`
	Scope string  `json:"scope"`
	Line  int     `json:"line"` // -1 for an order discount
	Value float64 `json:"value"`
	Kind  string  `json:"kind"`
}

// ApplyDiscountRules sets DiscountPct on lines and returns the document
// discount. Rules do not stack: per line the rule with the best priority wins
// (then the larger value), and the same for the document level. A discount the
// user already typed on a line is kept when it is larger than the rule's.
func ApplyDiscountRules(lines []Line, rules []DiscountRule, contactID, date string) ([]Line, string, float64, []AppliedDiscount) {
	out := make([]Line, len(lines))
	copy(out, lines)
	var applied []AppliedDiscount
	live := make([]DiscountRule, 0, len(rules))
	for _, r := range rules {
		if r.liveOn(date, contactID) {
			live = append(live, r)
		}
	}
	sort.SliceStable(live, func(i, j int) bool {
		if live[i].Priority != live[j].Priority {
			return live[i].Priority > live[j].Priority
		}
		return live[i].Value > live[j].Value
	})
	for i, l := range out {
		for _, r := range live {
			if r.Scope != "line" || r.Kind != "percent" || r.MinQty > l.Qty+1e-9 {
				continue
			}
			if r.ProductID != "" && r.ProductID != l.ProductID {
				continue
			}
			if r.MinAmount > 0 && l.Qty*l.Price < r.MinAmount {
				continue
			}
			if r.Value > l.DiscountPct {
				out[i].DiscountPct = clampPct(r.Value)
				applied = append(applied, AppliedDiscount{Rule: r.Name, Scope: "line", Line: i, Value: r.Value, Kind: "percent"})
			}
			break
		}
	}
	var sub float64
	for _, l := range out {
		sub += l.Qty * l.Price * (1 - clampPct(l.DiscountPct)/100)
	}
	kind, val := "percent", 0.0
	for _, r := range live {
		if r.Scope != "order" || r.MinAmount > sub+1e-9 {
			continue
		}
		k := r.Kind
		if k != "amount" {
			k = "percent"
		}
		kind, val = k, r.Value
		applied = append(applied, AppliedDiscount{Rule: r.Name, Scope: "order", Line: -1, Value: r.Value, Kind: k})
		break
	}
	return out, kind, val, applied
}

// ---- storage ----

// PriceList is a named price list.
type PriceList struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Currency  string   `json:"currency"`
	IsDefault bool     `json:"is_default"`
	Active    bool     `json:"active"`
	Customers []string `json:"customers"`
	Rows      []PriceRow `json:"rows"`
}

// ListPriceLists returns all lists with their rows and assigned customers.
func (r *Repository) ListPriceLists(ctx context.Context, orgID string) ([]PriceList, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, currency, is_default, active FROM adrient.sales_price_lists WHERE org_id=$1 ORDER BY lower(name)`, orgID)
	if err != nil {
		return nil, err
	}
	out := []PriceList{}
	idx := map[string]int{}
	for rows.Next() {
		var p PriceList
		if err := rows.Scan(&p.ID, &p.Name, &p.Currency, &p.IsDefault, &p.Active); err != nil {
			rows.Close()
			return nil, err
		}
		p.Customers, p.Rows = []string{}, []PriceRow{}
		idx[p.ID] = len(out)
		out = append(out, p)
	}
	rows.Close()
	pr, err := r.AllPriceRows(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, x := range pr {
		if i, ok := idx[x.ListID]; ok {
			out[i].Rows = append(out[i].Rows, x)
		}
	}
	cr, err := r.pool.Query(ctx, `SELECT price_list_id::text, contact_id::text FROM adrient.sales_price_list_customers WHERE org_id=$1`, orgID)
	if err != nil {
		return nil, err
	}
	defer cr.Close()
	for cr.Next() {
		var l, c string
		if err := cr.Scan(&l, &c); err != nil {
			return nil, err
		}
		if i, ok := idx[l]; ok {
			out[i].Customers = append(out[i].Customers, c)
		}
	}
	return out, cr.Err()
}

// AllPriceRows returns the rows of every active list.
func (r *Repository) AllPriceRows(ctx context.Context, orgID string) ([]PriceRow, error) {
	rows, err := r.pool.Query(ctx, `SELECT i.price_list_id::text, i.product_id, COALESCE(i.contact_id::text,''), i.min_qty::float8, i.price::float8
		FROM adrient.sales_price_list_items i JOIN adrient.sales_price_lists l ON l.id=i.price_list_id
		WHERE i.org_id=$1 AND l.active ORDER BY i.product_id, i.min_qty`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PriceRow{}
	for rows.Next() {
		var x PriceRow
		if err := rows.Scan(&x.ListID, &x.ProductID, &x.ContactID, &x.MinQty, &x.Price); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SavePriceList creates (id empty) or replaces a list with its rows and customers.
func (r *Repository) SavePriceList(ctx context.Context, orgID, id string, p PriceList) (PriceList, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return p, invalid("name is required")
	}
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	if p.Currency == "" {
		p.Currency = "INR"
	}
	for i, x := range p.Rows {
		if strings.TrimSpace(x.ProductID) == "" || x.Price < 0 || x.MinQty < 0 {
			return p, invalid("row %d: product, a non-negative price and quantity are required", i+1)
		}
		if x.ContactID != "" && len(x.ContactID) != 36 {
			return p, invalid("row %d: invalid customer", i+1)
		}
	}
	for _, c := range p.Customers {
		if len(c) != 36 {
			return p, invalid("invalid customer id")
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if p.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE adrient.sales_price_lists SET is_default=false WHERE org_id=$1`, orgID); err != nil {
			return p, err
		}
	}
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO adrient.sales_price_lists (org_id,name,currency,is_default,active) VALUES ($1,$2,$3,$4,$5) RETURNING id::text`,
			orgID, p.Name, p.Currency, p.IsDefault, p.Active).Scan(&id)
	} else {
		var n int
		err = tx.QueryRow(ctx, `WITH u AS (UPDATE adrient.sales_price_lists SET name=$3,currency=$4,is_default=$5,active=$6 WHERE org_id=$1 AND id=$2::uuid RETURNING 1) SELECT count(*) FROM u`,
			orgID, id, p.Name, p.Currency, p.IsDefault, p.Active).Scan(&n)
		if err == nil && n == 0 {
			err = ErrNotFound
		}
	}
	if err != nil {
		return p, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.sales_price_list_items WHERE org_id=$1 AND price_list_id=$2::uuid`, orgID, id); err != nil {
		return p, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.sales_price_list_customers WHERE org_id=$1 AND price_list_id=$2::uuid`, orgID, id); err != nil {
		return p, err
	}
	for _, x := range p.Rows {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.sales_price_list_items (org_id,price_list_id,product_id,contact_id,min_qty,price)
			VALUES ($1,$2::uuid,$3,NULLIF($4,'')::uuid,$5,$6)`, orgID, id, strings.TrimSpace(x.ProductID), x.ContactID, x.MinQty, Round2(x.Price)); err != nil {
			return p, err
		}
	}
	for _, c := range p.Customers {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.sales_price_list_customers (org_id,price_list_id,contact_id) VALUES ($1,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, orgID, id, c); err != nil {
			return p, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return p, err
	}
	p.ID = id
	return p, nil
}

// DeletePriceList removes a list (rows and assignments go with it).
func (r *Repository) DeletePriceList(ctx context.Context, orgID, id string) error {
	return r.exec(ctx, `DELETE FROM adrient.sales_price_lists WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
}

// ListDiscountRules returns the org's rules.
func (r *Repository) ListDiscountRules(ctx context.Context, orgID string) ([]DiscountRule, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, scope, kind, value::float8, min_qty::float8, min_amount::float8, product_id,
		COALESCE(contact_id::text,''), COALESCE(to_char(valid_from,'YYYY-MM-DD'),''), COALESCE(to_char(valid_to,'YYYY-MM-DD'),''), priority, active
		FROM adrient.sales_discount_rules WHERE org_id=$1 ORDER BY priority DESC, lower(name)`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DiscountRule{}
	for rows.Next() {
		var d DiscountRule
		if err := rows.Scan(&d.ID, &d.Name, &d.Scope, &d.Kind, &d.Value, &d.MinQty, &d.MinAmount, &d.ProductID, &d.ContactID, &d.ValidFrom, &d.ValidTo, &d.Priority, &d.Active); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveDiscountRule creates (id empty) or updates a rule.
func (r *Repository) SaveDiscountRule(ctx context.Context, orgID, id string, d DiscountRule) (DiscountRule, error) {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		return d, invalid("name is required")
	}
	if d.Scope != "order" {
		d.Scope = "line"
	}
	if d.Kind != "amount" {
		d.Kind = "percent"
	}
	if d.Scope == "line" && d.Kind == "amount" {
		return d, invalid("a line rule must be a percentage")
	}
	if d.Value <= 0 || (d.Kind == "percent" && d.Value > 100) || d.MinQty < 0 || d.MinAmount < 0 {
		return d, invalid("value must be above zero (and at most 100 for a percentage)")
	}
	for _, s := range []string{d.ValidFrom, d.ValidTo} {
		if s != "" && parseDate(s) == nil {
			return d, invalid("dates must be YYYY-MM-DD")
		}
	}
	if d.ContactID != "" && len(d.ContactID) != 36 {
		return d, invalid("invalid customer")
	}
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.sales_discount_rules (org_id,name,scope,kind,value,min_qty,min_amount,product_id,contact_id,valid_from,valid_to,priority,active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid,NULLIF($10,'')::date,NULLIF($11,'')::date,$12,$13) RETURNING id::text`,
			orgID, d.Name, d.Scope, d.Kind, d.Value, d.MinQty, d.MinAmount, d.ProductID, d.ContactID, d.ValidFrom, d.ValidTo, d.Priority, d.Active).Scan(&d.ID)
	} else {
		err = r.exec(ctx, `UPDATE adrient.sales_discount_rules SET name=$3,scope=$4,kind=$5,value=$6,min_qty=$7,min_amount=$8,product_id=$9,
			contact_id=NULLIF($10,'')::uuid,valid_from=NULLIF($11,'')::date,valid_to=NULLIF($12,'')::date,priority=$13,active=$14 WHERE org_id=$1 AND id=$2::uuid`,
			orgID, id, d.Name, d.Scope, d.Kind, d.Value, d.MinQty, d.MinAmount, d.ProductID, d.ContactID, d.ValidFrom, d.ValidTo, d.Priority, d.Active)
		d.ID = id
	}
	return d, err
}

// DeleteDiscountRule removes a rule.
func (r *Repository) DeleteDiscountRule(ctx context.Context, orgID, id string) error {
	return r.exec(ctx, `DELETE FROM adrient.sales_discount_rules WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
}

// PricingRequest asks for prices and discounts for a set of lines.
type PricingRequest struct {
	ContactID   string `json:"contact_id"`
	PriceListID string `json:"price_list_id"`
	Date        string `json:"date"`
	Lines       []Line `json:"lines"`
}

// PricingResult is the repriced document body.
type PricingResult struct {
	Lines         []Line            `json:"lines"`
	DiscountKind  string            `json:"discount_kind"`
	DiscountValue float64           `json:"discount_value"`
	Sources       []PriceSource     `json:"sources"`
	Applied       []AppliedDiscount `json:"applied"`
}

// Price applies price lists then discount rules to lines. A line is repriced
// only when it has a product id (an Inventory product or a price-list item).
func (r *Repository) Price(ctx context.Context, orgID string, q PricingRequest) (PricingResult, error) {
	pc := PriceCtx{ContactID: q.ContactID, Assigned: map[string]bool{}, Default: map[string]bool{}}
	lists, err := r.ListPriceLists(ctx, orgID)
	if err != nil {
		return PricingResult{}, err
	}
	var rows []PriceRow
	for _, l := range lists {
		if !l.Active {
			continue
		}
		rows = append(rows, l.Rows...)
		if l.IsDefault {
			pc.Default[l.ID] = true
		}
		for _, c := range l.Customers {
			if c == q.ContactID {
				pc.Assigned[l.ID] = true
			}
		}
		if l.ID == q.PriceListID {
			pc.Assigned[l.ID] = true
		}
	}
	date := q.Date
	if date == "" {
		date = today().Format("2006-01-02")
	}
	res := PricingResult{Lines: make([]Line, len(q.Lines)), Sources: make([]PriceSource, len(q.Lines))}
	copy(res.Lines, q.Lines)
	for i, l := range res.Lines {
		res.Sources[i] = SrcBase
		if l.ProductID == "" {
			continue
		}
		res.Lines[i].Price, res.Sources[i] = ResolvePrice(l.Price, l.ProductID, l.Qty, pc, rows)
	}
	rules, err := r.ListDiscountRules(ctx, orgID)
	if err != nil {
		return res, err
	}
	res.Lines, res.DiscountKind, res.DiscountValue, res.Applied = ApplyDiscountRules(res.Lines, rules, q.ContactID, date)
	return res, nil
}
