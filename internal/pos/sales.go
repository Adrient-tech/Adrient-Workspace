package pos

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
)

var validMethods = map[string]bool{"cash": true, "card": true, "other": true, "store_credit": true, "loyalty": true}

func mapInvErr(err error) error {
	switch {
	case errors.Is(err, inventory.ErrNotFound):
		return invalid("a product in the cart no longer exists")
	case errors.Is(err, inventory.ErrInvalid):
		return invalid("%s", strings.TrimPrefix(err.Error(), "invalid: "))
	}
	return err
}

// ------------------------------------------------------------------ reading

const saleCols = `s.id::text, s.number, coalesce(s.register_id::text,''), coalesce(r.name,''), coalesce(s.shift_id::text,''),
	s.customer_id, s.customer_name, s.status, s.subtotal::float8, s.discount_total::float8, s.tax_total::float8,
	s.total::float8, s.paid::float8, s.change_due::float8, s.discount_kind, s.discount_value::float8, s.note, s.hold_label,
	s.refunded_total::float8, s.loyalty_earned::float8, coalesce(u.display_name, u.email, ''), s.created_at, s.completed_at`

const saleFrom = ` FROM adrient.pos_sales s LEFT JOIN adrient.pos_registers r ON r.id=s.register_id
	LEFT JOIN adrient.users u ON u.id=s.created_by`

func scanSale(row pgx.Row) (*adrientv1.PosSale, error) {
	var s adrientv1.PosSale
	var created time.Time
	var completed *time.Time
	if err := row.Scan(&s.Id, &s.Number, &s.RegisterId, &s.RegisterName, &s.ShiftId, &s.CustomerId, &s.CustomerName,
		&s.Status, &s.Subtotal, &s.DiscountTotal, &s.TaxTotal, &s.Total, &s.Paid, &s.ChangeDue, &s.DiscountKind,
		&s.DiscountValue, &s.Note, &s.HoldLabel, &s.RefundedTotal, &s.LoyaltyEarned, &s.CreatedBy, &created, &completed); err != nil {
		return nil, err
	}
	s.CreatedAt = ts(created)
	s.CompletedAt = tsPtr(completed)
	return &s, nil
}

func (r *Repository) loadDetail(ctx context.Context, q db, s *adrientv1.PosSale) error {
	rows, err := q.Query(ctx, `SELECT id::text, coalesce(product_id::text,''), name, sku, qty::float8, unit_price::float8,
		discount_kind, discount_value::float8, tax_rate_id, tax_name, tax_rate::float8, tax_inclusive,
		discount_amount::float8, tax_amount::float8, line_total::float8, refunded_qty::float8, refunded_amount::float8, note
		FROM adrient.pos_sale_lines WHERE sale_id=$1 ORDER BY position`, s.Id)
	if err != nil {
		return err
	}
	s.Lines = []*adrientv1.PosSaleLine{}
	for rows.Next() {
		var l adrientv1.PosSaleLine
		if err := rows.Scan(&l.Id, &l.ProductId, &l.Name, &l.Sku, &l.Qty, &l.UnitPrice, &l.DiscountKind, &l.DiscountValue,
			&l.TaxRateId, &l.TaxName, &l.TaxRate, &l.TaxInclusive, &l.DiscountAmount, &l.TaxAmount, &l.LineTotal,
			&l.RefundedQty, &l.RefundedAmount, &l.Note); err != nil {
			rows.Close()
			return err
		}
		s.Lines = append(s.Lines, &l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	prows, err := q.Query(ctx, `SELECT id::text, method, amount::float8, tendered::float8, ref, created_at
		FROM adrient.pos_payments WHERE sale_id=$1 ORDER BY created_at, method`, s.Id)
	if err != nil {
		return err
	}
	s.Payments = []*adrientv1.PosPayment{}
	for prows.Next() {
		var p adrientv1.PosPayment
		var t time.Time
		if err := prows.Scan(&p.Id, &p.Method, &p.Amount, &p.Tendered, &p.Ref, &t); err != nil {
			prows.Close()
			return err
		}
		p.CreatedAt = ts(t)
		s.Payments = append(s.Payments, &p)
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		return err
	}
	rrows, err := q.Query(ctx, `SELECT id::text, total::float8, method, restock, note, created_at
		FROM adrient.pos_refunds WHERE sale_id=$1 ORDER BY created_at`, s.Id)
	if err != nil {
		return err
	}
	s.Refunds = []*adrientv1.PosRefund{}
	for rrows.Next() {
		var f adrientv1.PosRefund
		var t time.Time
		if err := rrows.Scan(&f.Id, &f.Total, &f.Method, &f.Restock, &f.Note, &t); err != nil {
			rrows.Close()
			return err
		}
		f.SaleId = s.Id
		f.CreatedAt = ts(t)
		s.Refunds = append(s.Refunds, &f)
	}
	rrows.Close()
	if err := rrows.Err(); err != nil {
		return err
	}
	for i, f := range s.Refunds {
		f.Number = fmt.Sprintf("R%d-%d", s.Number, i+1)
		lrows, err := q.Query(ctx, `SELECT rl.line_id::text, l.name, rl.qty::float8, rl.amount::float8
			FROM adrient.pos_refund_lines rl JOIN adrient.pos_sale_lines l ON l.id=rl.line_id
			WHERE rl.refund_id=$1 ORDER BY l.position`, f.Id)
		if err != nil {
			return err
		}
		for lrows.Next() {
			var rl adrientv1.PosRefundLine
			if err := lrows.Scan(&rl.LineId, &rl.Name, &rl.Qty, &rl.Amount); err != nil {
				lrows.Close()
				return err
			}
			f.Lines = append(f.Lines, &rl)
		}
		lrows.Close()
		if err := lrows.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) getSale(ctx context.Context, q db, orgID, id string) (*adrientv1.PosSale, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	s, err := scanSale(q.QueryRow(ctx, `SELECT `+saleCols+saleFrom+` WHERE s.org_id=$1 AND s.id=$2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.loadDetail(ctx, q, s); err != nil {
		return nil, err
	}
	return s, nil
}

// GetSale returns one sale with lines, payments and refunds.
func (r *Repository) GetSale(ctx context.Context, orgID, id string) (*adrientv1.PosSale, error) {
	return r.getSale(ctx, r.pool, orgID, id)
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// ListSales lists sales newest first. This is the read API for Accounting.
func (r *Repository) ListSales(ctx context.Context, orgID string, req *adrientv1.ListPosSalesRequest) ([]*adrientv1.PosSale, error) {
	return r.listSales(ctx, orgID, req, time.Time{}, "")
}

// listSales is ListSales after an optional keyset (beforeTime, beforeID) in
// the newest-first order; see sales_page.go.
func (r *Repository) listSales(ctx context.Context, orgID string, req *adrientv1.ListPosSalesRequest, beforeTime time.Time, beforeID string) ([]*adrientv1.PosSale, error) {
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	where := []string{"s.org_id=$1"}
	args := []any{orgID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.Replace(cond, "?", fmt.Sprintf("$%d", len(args)), -1))
	}
	switch req.GetStatus() {
	case "held":
		where = append(where, "s.status='held'")
	case "completed":
		where = append(where, "s.status='completed'")
	case "refunded":
		where = append(where, "s.status IN ('refunded','partially_refunded')")
	default:
		where = append(where, "s.status<>'held'")
	}
	if t, ok := parseTS(req.GetSince()); ok {
		add("coalesce(s.completed_at, s.created_at) >= ?", t)
	}
	if t, ok := parseTS(req.GetUntil()); ok {
		add("coalesce(s.completed_at, s.created_at) < ?", t)
	}
	if req.GetCustomerId() != "" {
		add("s.customer_id = ?", req.GetCustomerId())
	}
	if isUUID(req.GetRegisterId()) {
		add("s.register_id = ?::uuid", req.GetRegisterId())
	}
	if q := strings.TrimSpace(req.GetQuery()); q != "" {
		args = append(args, strings.TrimPrefix(q, "#"), "%"+escapeLike(q)+"%")
		n, l := len(args)-1, len(args)
		where = append(where, fmt.Sprintf(`(s.number::text = $%d OR s.customer_name ILIKE $%d ESCAPE '\' OR s.hold_label ILIKE $%d ESCAPE '\' OR s.note ILIKE $%d ESCAPE '\')`, n, l, l, l))
	}
	if !beforeTime.IsZero() && isUUID(beforeID) {
		args = append(args, beforeTime, beforeID)
		where = append(where, fmt.Sprintf("(date_trunc('second', coalesce(s.completed_at, s.created_at)), s.id) < ($%d, $%d::uuid)", len(args)-1, len(args)))
	}
	args = append(args, limit)
	rows, err := r.pool.Query(ctx, `SELECT `+saleCols+saleFrom+` WHERE `+strings.Join(where, " AND ")+
		fmt.Sprintf(` ORDER BY date_trunc('second', coalesce(s.completed_at, s.created_at)) DESC, s.id DESC LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	out := []*adrientv1.PosSale{}
	for rows.Next() {
		s, err := scanSale(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if req.GetDetail() {
		for _, s := range out {
			if err := r.loadDetail(ctx, r.pool, s); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// DeleteHeldSale discards a held sale.
func (r *Repository) DeleteHeldSale(ctx context.Context, orgID, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.pos_sales WHERE org_id=$1 AND id=$2 AND status='held'`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------ writing

// SaveSale holds (complete=false) or completes (complete=true) a sale. When
// completing, stock is moved through the inventory repository in the same
// transaction, so a failed stock movement rolls back the whole sale.
func (r *Repository) SaveSale(ctx context.Context, orgID, userID string, in *adrientv1.PosSaleInput, complete bool) (*adrientv1.PosSale, error) {
	if len(in.GetLines()) == 0 {
		return nil, invalid("the cart is empty")
	}
	if len(in.GetLines()) > 500 {
		return nil, invalid("too many lines")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	reg, err := r.getRegister(ctx, tx, orgID, in.GetRegisterId())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, invalid("choose a register")
		}
		return nil, err
	}
	if reg.Archived {
		return nil, invalid("register is archived")
	}
	saleID := in.GetId()
	if saleID != "" {
		var st string
		err := tx.QueryRow(ctx, `SELECT status FROM adrient.pos_sales WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, saleID).Scan(&st)
		if errors.Is(err, pgx.ErrNoRows) || (err != nil && !isUUID(saleID)) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if st != "held" {
			return nil, conflict("this sale is already completed")
		}
	}
	rates, err := r.taxRates(ctx, tx, orgID)
	if err != nil {
		return nil, err
	}
	rateByID := map[string]*adrientv1.PosTaxRate{}
	for _, t := range rates {
		rateByID[t.Id] = t
	}

	type prepared struct {
		in   *adrientv1.PosLineInput
		name string
		sku  string
		tax  *adrientv1.PosTaxRate
	}
	prep := make([]prepared, len(in.GetLines()))
	calc := make([]CalcLine, len(in.GetLines()))
	for i, l := range in.GetLines() {
		if l.GetQty() <= 0 || math.IsNaN(l.GetQty()) || l.GetQty() > 1e6 {
			return nil, invalid("line %d: quantity must be positive", i+1)
		}
		if l.GetUnitPrice() < 0 || math.IsNaN(l.GetUnitPrice()) || l.GetUnitPrice() > 1e9 {
			return nil, invalid("line %d: price cannot be negative", i+1)
		}
		if k := l.GetDiscountKind(); k != "" && k != "percent" && k != "amount" {
			return nil, invalid("line %d: unknown discount kind", i+1)
		}
		name, sku := strings.TrimSpace(l.GetName()), l.GetSku()
		if l.GetProductId() != "" {
			if !isUUID(l.GetProductId()) {
				return nil, invalid("line %d: unknown product", i+1)
			}
			pn, ps, _, err := r.inv.ProductBasics(ctx, tx, orgID, l.GetProductId())
			if err != nil {
				return nil, mapInvErr(err)
			}
			if name == "" {
				name = pn
			}
			sku = ps
		}
		if name == "" {
			return nil, invalid("line %d: name is required", i+1)
		}
		var tax *adrientv1.PosTaxRate
		if l.GetTaxRateId() != "" {
			tax = rateByID[l.GetTaxRateId()]
			if tax == nil {
				return nil, invalid("line %d: unknown tax rate", i+1)
			}
		}
		prep[i] = prepared{in: l, name: name, sku: sku, tax: tax}
		calc[i] = CalcLine{Qty: l.GetQty(), UnitPrice: l.GetUnitPrice(), DiscountKind: l.GetDiscountKind(), DiscountValue: l.GetDiscountValue()}
		if tax != nil {
			calc[i].TaxRate, calc[i].TaxInclusive = tax.Rate, tax.Inclusive
		}
	}
	if k := in.GetDiscountKind(); k != "" && k != "percent" && k != "amount" {
		return nil, invalid("unknown discount kind")
	}
	totals := ComputeTotals(calc, in.GetDiscountKind(), in.GetDiscountValue())

	status := "held"
	if complete {
		status = "completed"
	}
	custID := strings.TrimSpace(in.GetCustomerId())
	custName := strings.TrimSpace(in.GetCustomerName())

	if saleID == "" {
		err = tx.QueryRow(ctx, `INSERT INTO adrient.pos_sales (org_id, register_id, created_by) VALUES ($1,$2,NULLIF($3,'')::uuid)
			RETURNING id::text`, orgID, reg.Id, userID).Scan(&saleID)
		if err != nil {
			return nil, err
		}
	} else if _, err := tx.Exec(ctx, `DELETE FROM adrient.pos_sale_lines WHERE sale_id=$1`, saleID); err != nil {
		return nil, err
	}
	for i, p := range prep {
		res := totals.Lines[i]
		var tn, tid string
		var tr float64
		var ti bool
		if p.tax != nil {
			tn, tid, tr, ti = p.tax.Name, p.tax.Id, p.tax.Rate, p.tax.Inclusive
		}
		var pid any
		if p.in.GetProductId() != "" {
			pid = p.in.GetProductId()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.pos_sale_lines
			(org_id, sale_id, position, product_id, name, sku, qty, unit_price, discount_kind, discount_value,
			 tax_rate_id, tax_name, tax_rate, tax_inclusive, discount_amount, tax_amount, line_total, note)
			VALUES ($1,$2,$3,$4::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			orgID, saleID, i, pid, p.name, p.sku, p.in.GetQty(), p.in.GetUnitPrice(), p.in.GetDiscountKind(), p.in.GetDiscountValue(),
			tid, tn, tr, ti, res.DiscountAmount, res.TaxAmount, res.LineTotal, strings.TrimSpace(p.in.GetNote())); err != nil {
			return nil, err
		}
	}

	var shiftID string
	var number int64
	var paid, change, earned float64
	if complete {
		if err := tx.QueryRow(ctx, `SELECT id::text FROM adrient.pos_shifts WHERE register_id=$1 AND closed_at IS NULL`, reg.Id).Scan(&shiftID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, conflict("open a shift on this register before selling")
			}
			return nil, err
		}
		tenders := make([]Tender, 0, len(in.GetPayments()))
		for _, p := range in.GetPayments() {
			if !validMethods[p.GetMethod()] {
				return nil, invalid("unknown payment method %q", p.GetMethod())
			}
			if p.GetAmount() < 0 || math.IsNaN(p.GetAmount()) {
				return nil, invalid("payment amount cannot be negative")
			}
			tenders = append(tenders, Tender{Method: p.GetMethod(), Amount: p.GetAmount()})
		}
		applied, ch, ok := AllocateTenders(totals.Total, tenders)
		if !ok {
			return nil, invalid("payments do not cover the total of %.2f", totals.Total)
		}
		change = ch
		set, err := r.settings(ctx, tx, orgID)
		if err != nil {
			return nil, err
		}
		// Receipt number: per-org sequence guarded by an advisory lock.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('pos-sale:'||$1))`, orgID); err != nil {
			return nil, err
		}
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(number),0)+1 FROM adrient.pos_sales WHERE org_id=$1`, orgID).Scan(&number); err != nil {
			return nil, err
		}
		for _, a := range applied {
			var ref string
			for _, p := range in.GetPayments() {
				if p.GetMethod() == a.Method && p.GetRef() != "" {
					ref = p.GetRef()
					break
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO adrient.pos_payments (org_id, sale_id, method, amount, tendered, ref)
				VALUES ($1,$2,$3,$4,$5,$6)`, orgID, saleID, a.Method, a.Amount, a.Tendered, ref); err != nil {
				return nil, err
			}
			paid += a.Amount
			switch a.Method {
			case "store_credit", "loyalty":
				if custID == "" {
					return nil, invalid("choose a customer to pay with %s", strings.ReplaceAll(a.Method, "_", " "))
				}
			}
			if a.Method == "other" {
				if code, ok := GiftRef(ref); ok {
					if err := r.redeemGift(ctx, tx, orgID, code, a.Amount, saleID, time.Now()); err != nil {
						return nil, err
					}
				}
			}
			if a.Method == "store_credit" {
				if err := r.moveBalance(ctx, tx, orgID, custID, "store_credit", -a.Amount, "sale", saleID, fmt.Sprintf("Sale #%d", number), false); err != nil {
					return nil, err
				}
			}
			if a.Method == "loyalty" {
				if set.LoyaltyPointValue <= 0 {
					return nil, invalid("loyalty points are not redeemable")
				}
				pts := math.Ceil(a.Amount/set.LoyaltyPointValue*100) / 100
				if err := r.moveBalance(ctx, tx, orgID, custID, "loyalty", -pts, "redeem", saleID, fmt.Sprintf("Sale #%d", number), false); err != nil {
					return nil, err
				}
			}
		}
		// Stock moves in the same transaction.
		loc := reg.LocationId
		for i, p := range prep {
			if p.in.GetProductId() == "" {
				continue
			}
			out := &inventory.AdjustOut{}
			if _, err := r.inv.AdjustInTx(ctx, tx, orgID, userID, p.in.GetProductId(), inventory.AdjustInput{
				LocationID: loc, Qty: p.in.GetQty(), Reason: "sale", Ref: saleID, Note: fmt.Sprintf("POS sale #%d", number), Out: out,
			}); err != nil {
				return nil, mapInvErr(err)
			}
			// Freeze the cost of goods on the line (what the stock layers actually released).
			if _, err := tx.Exec(ctx, `UPDATE adrient.pos_sale_lines SET unit_cost=$3, cost_total=$4 WHERE sale_id=$1 AND position=$2`,
				saleID, i, out.UnitCost, out.Value); err != nil {
				return nil, err
			}
		}
		if custID != "" && set.LoyaltyEarnPerUnit > 0 {
			earned, err = r.tierPoints(ctx, tx, orgID, custID, totals.Total, set.LoyaltyEarnPerUnit)
			if err != nil {
				return nil, err
			}
			if earned > 0 {
				if err := r.moveBalance(ctx, tx, orgID, custID, "loyalty", earned, "earn", saleID, fmt.Sprintf("Sale #%d", number), false); err != nil {
					return nil, err
				}
			}
		}
	}

	if complete && isUUID(custID) {
		if err := r.addStamp(ctx, tx, orgID, custID, saleID, number, totals.Total); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE adrient.pos_sales SET number=$3, register_id=$4, shift_id=NULLIF($5,'')::uuid,
		customer_id=$6, customer_name=$7, status=$8, subtotal=$9, discount_total=$10, tax_total=$11, total=$12,
		paid=$13, change_due=$14, discount_kind=$15, discount_value=$16, note=$17, hold_label=$18, loyalty_earned=$19,
		completed_at=CASE WHEN $8='completed' THEN now() ELSE NULL END
		WHERE org_id=$1 AND id=$2`,
		orgID, saleID, number, reg.Id, shiftID, custID, custName, status, totals.Subtotal, totals.DiscountTotal,
		totals.TaxTotal, totals.Total, paid, change, in.GetDiscountKind(), in.GetDiscountValue(),
		strings.TrimSpace(in.GetNote()), strings.TrimSpace(in.GetHoldLabel()), earned); err != nil {
		return nil, err
	}
	out, err := r.getSale(ctx, tx, orgID, saleID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// RefundSale returns lines of a completed sale and pays the customer back.
func (r *Repository) RefundSale(ctx context.Context, orgID, userID string, req *adrientv1.RefundPosSaleRequest) (*adrientv1.PosSale, error) {
	method := req.GetMethod()
	if method != "cash" && method != "card" && method != "other" && method != "store_credit" {
		return nil, invalid("refund method must be cash, card, other or store_credit")
	}
	if len(req.GetLines()) == 0 {
		return nil, invalid("choose the lines to refund")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if !isUUID(req.GetId()) {
		return nil, ErrNotFound
	}
	var status, custID, regID string
	var saleTotal, loyaltyEarned float64
	var number int64
	err = tx.QueryRow(ctx, `SELECT status, customer_id, coalesce(register_id::text,''), total::float8, loyalty_earned::float8, number
		FROM adrient.pos_sales WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, req.GetId()).
		Scan(&status, &custID, &regID, &saleTotal, &loyaltyEarned, &number)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != "completed" && status != "partially_refunded" {
		return nil, conflict("only completed sales can be refunded")
	}
	if method == "store_credit" && custID == "" {
		return nil, invalid("a store-credit refund needs a customer on the sale")
	}
	// Cash leaves the drawer of the register doing the refund.
	var shiftID string
	if rid := req.GetRegisterId(); isUUID(rid) {
		_ = tx.QueryRow(ctx, `SELECT id::text FROM adrient.pos_shifts WHERE org_id=$1 AND register_id=$2 AND closed_at IS NULL`, orgID, rid).Scan(&shiftID)
	}
	if method == "cash" && shiftID == "" {
		return nil, conflict("open a shift on a register to pay a cash refund")
	}
	var location string
	if regID != "" {
		_ = tx.QueryRow(ctx, `SELECT coalesce(location_id::text,'') FROM adrient.pos_registers WHERE id=$1`, regID).Scan(&location)
	}
	var refundID string
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.pos_refunds (org_id, sale_id, shift_id, total, method, restock, note, created_by)
		VALUES ($1,$2,NULLIF($3,'')::uuid,0,$4,$5,$6,NULLIF($7,'')::uuid) RETURNING id::text`,
		orgID, req.GetId(), shiftID, method, req.GetRestock(), strings.TrimSpace(req.GetNote()), userID).Scan(&refundID); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var total float64
	for _, li := range req.GetLines() {
		if seen[li.GetLineId()] || !isUUID(li.GetLineId()) {
			return nil, invalid("invalid refund line")
		}
		seen[li.GetLineId()] = true
		if li.GetQty() <= 0 {
			return nil, invalid("refund quantity must be positive")
		}
		var productID, name string
		var qty, rq, ramt, lt float64
		err := tx.QueryRow(ctx, `SELECT coalesce(product_id::text,''), name, qty::float8, refunded_qty::float8, refunded_amount::float8, line_total::float8
			FROM adrient.pos_sale_lines WHERE id=$1 AND sale_id=$2 FOR UPDATE`, li.GetLineId(), req.GetId()).
			Scan(&productID, &name, &qty, &rq, &ramt, &lt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invalid("refund line does not belong to this sale")
		}
		if err != nil {
			return nil, err
		}
		if round3(li.GetQty()) > round3(qty-rq) {
			return nil, invalid("cannot refund more than was sold of %s", name)
		}
		amt := RefundAmount(lt, qty, rq, ramt, li.GetQty())
		if _, err := tx.Exec(ctx, `UPDATE adrient.pos_sale_lines SET refunded_qty=refunded_qty+$2, refunded_amount=refunded_amount+$3 WHERE id=$1`,
			li.GetLineId(), li.GetQty(), amt); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.pos_refund_lines (refund_id, line_id, qty, amount) VALUES ($1,$2,$3,$4)`,
			refundID, li.GetLineId(), li.GetQty(), amt); err != nil {
			return nil, err
		}
		total += amt
		if req.GetRestock() && productID != "" {
			if _, err := r.inv.AdjustInTx(ctx, tx, orgID, userID, productID, inventory.AdjustInput{
				LocationID: location, Qty: li.GetQty(), Reason: "refund", Ref: req.GetId(), Note: fmt.Sprintf("POS refund of sale #%d", number),
			}); err != nil {
				return nil, mapInvErr(err)
			}
		}
	}
	total = round2(total)
	var open int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.pos_sale_lines WHERE sale_id=$1 AND qty-refunded_qty > 0.0005`, req.GetId()).Scan(&open); err != nil {
		return nil, err
	}
	next := "partially_refunded"
	if open == 0 {
		next = "refunded"
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.pos_refunds SET total=$2 WHERE id=$1`, refundID, total); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.pos_sales SET refunded_total=refunded_total+$2, status=$3 WHERE id=$1`, req.GetId(), total, next); err != nil {
		return nil, err
	}
	if method == "store_credit" {
		if err := r.moveBalance(ctx, tx, orgID, custID, "store_credit", total, "refund", req.GetId(), fmt.Sprintf("Refund of sale #%d", number), false); err != nil {
			return nil, err
		}
	}
	if custID != "" && loyaltyEarned > 0 && saleTotal > 0 {
		back := math.Floor(loyaltyEarned * total / saleTotal)
		if back > 0 {
			if err := r.moveBalance(ctx, tx, orgID, custID, "loyalty", -back, "reverse", req.GetId(), fmt.Sprintf("Refund of sale #%d", number), true); err != nil {
				return nil, err
			}
		}
	}
	out, err := r.getSale(ctx, tx, orgID, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// ----------------------------------------------------------------- balances

// moveBalance adds delta to a customer's loyalty points or store credit and
// logs it. A debit that would go below zero is rejected unless clamp is set.
func (r *Repository) moveBalance(ctx context.Context, q db, orgID, contactID, kind string, delta float64, reason, saleID, note string, clamp bool) error {
	col := "loyalty_points"
	switch kind {
	case "loyalty":
	case "store_credit":
		col = "store_credit"
	default:
		return invalid("balance kind must be loyalty or store_credit")
	}
	if contactID == "" {
		return invalid("customer is required")
	}
	if _, err := q.Exec(ctx, `INSERT INTO adrient.pos_balances (org_id, contact_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, orgID, contactID); err != nil {
		return err
	}
	var cur float64
	if err := q.QueryRow(ctx, `SELECT `+col+`::float8 FROM adrient.pos_balances WHERE org_id=$1 AND contact_id=$2 FOR UPDATE`, orgID, contactID).Scan(&cur); err != nil {
		return err
	}
	next := round2(cur + delta)
	if next < 0 {
		if !clamp {
			label := "store credit"
			if kind == "loyalty" {
				label = "loyalty points"
			}
			return invalid("not enough %s (customer has %.2f)", label, cur)
		}
		delta = round2(-cur)
		next = 0
	}
	if _, err := q.Exec(ctx, `UPDATE adrient.pos_balances SET `+col+`=$3, updated_at=now() WHERE org_id=$1 AND contact_id=$2`, orgID, contactID, next); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `INSERT INTO adrient.pos_balance_entries (org_id, contact_id, kind, delta, reason, sale_id, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, orgID, contactID, kind, delta, reason, saleID, note)
	return err
}

// AdjustBalance is a manual correction (top up store credit, grant points).
func (r *Repository) AdjustBalance(ctx context.Context, orgID, contactID, kind string, delta float64, note string) (*adrientv1.PosBalance, error) {
	if delta == 0 || math.IsNaN(delta) {
		return nil, invalid("amount is required")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := r.moveBalance(ctx, tx, orgID, contactID, kind, round2(delta), "adjust", "", strings.TrimSpace(note), false); err != nil {
		return nil, err
	}
	out, err := r.getBalance(ctx, tx, orgID, contactID)
	if err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

func (r *Repository) GetBalance(ctx context.Context, orgID, contactID string) (*adrientv1.PosBalance, error) {
	return r.getBalance(ctx, r.pool, orgID, contactID)
}

func (r *Repository) getBalance(ctx context.Context, q db, orgID, contactID string) (*adrientv1.PosBalance, error) {
	b := &adrientv1.PosBalance{ContactId: contactID, Entries: []*adrientv1.PosBalanceEntry{}}
	var t time.Time
	err := q.QueryRow(ctx, `SELECT loyalty_points::float8, store_credit::float8, updated_at FROM adrient.pos_balances
		WHERE org_id=$1 AND contact_id=$2`, orgID, contactID).Scan(&b.LoyaltyPoints, &b.StoreCredit, &t)
	if err == nil {
		b.UpdatedAt = ts(t)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT id::text, kind, delta::float8, reason, sale_id, note, created_at
		FROM adrient.pos_balance_entries WHERE org_id=$1 AND contact_id=$2 ORDER BY created_at DESC LIMIT 30`, orgID, contactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e adrientv1.PosBalanceEntry
		var c time.Time
		if err := rows.Scan(&e.Id, &e.Kind, &e.Delta, &e.Reason, &e.SaleId, &e.Note, &c); err != nil {
			return nil, err
		}
		e.CreatedAt = ts(c)
		b.Entries = append(b.Entries, &e)
	}
	return b, rows.Err()
}

func (r *Repository) ListBalances(ctx context.Context, orgID string) ([]*adrientv1.PosBalance, error) {
	rows, err := r.pool.Query(ctx, `SELECT contact_id, loyalty_points::float8, store_credit::float8, updated_at
		FROM adrient.pos_balances WHERE org_id=$1 AND (loyalty_points<>0 OR store_credit<>0) ORDER BY updated_at DESC LIMIT 500`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*adrientv1.PosBalance{}
	for rows.Next() {
		var b adrientv1.PosBalance
		var t time.Time
		if err := rows.Scan(&b.ContactId, &b.LoyaltyPoints, &b.StoreCredit, &t); err != nil {
			return nil, err
		}
		b.UpdatedAt = ts(t)
		out = append(out, &b)
	}
	return out, rows.Err()
}

// ------------------------------------------------------------------- report

// Report summarizes completed sales and refunds in a period (default 30 days).
func (r *Repository) Report(ctx context.Context, orgID, since, until string) (*adrientv1.PosReport, error) {
	from, ok := parseTS(since)
	if !ok {
		from = time.Now().AddDate(0, 0, -30)
	}
	to, ok := parseTS(until)
	if !ok {
		to = time.Now().Add(time.Hour)
	}
	rep := &adrientv1.PosReport{}
	const inRange = `s.org_id=$1 AND s.status<>'held' AND s.completed_at >= $2 AND s.completed_at < $3`
	if err := r.pool.QueryRow(ctx, `SELECT count(*), coalesce(sum(subtotal),0)::float8, coalesce(sum(discount_total),0)::float8,
		coalesce(sum(tax_total),0)::float8, coalesce(sum(total),0)::float8 FROM adrient.pos_sales s WHERE `+inRange, orgID, from, to).
		Scan(&rep.SalesCount, &rep.Gross, &rep.Discounts, &rep.Tax, &rep.Total); err != nil {
		return nil, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*), coalesce(sum(total),0)::float8 FROM adrient.pos_refunds
		WHERE org_id=$1 AND created_at >= $2 AND created_at < $3`, orgID, from, to).Scan(&rep.RefundsCount, &rep.Refunds); err != nil {
		return nil, err
	}
	rep.Net = round2(rep.Total - rep.Refunds)
	if rep.SalesCount > 0 {
		rep.AverageSale = round2(rep.Total / float64(rep.SalesCount))
	}
	collect := func(sql string, args ...any) ([]*adrientv1.PosReportRow, error) {
		rows, err := r.pool.Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []*adrientv1.PosReportRow{}
		for rows.Next() {
			var x adrientv1.PosReportRow
			if err := rows.Scan(&x.Key, &x.Label, &x.Count, &x.Qty, &x.Amount, &x.Extra, &x.Base); err != nil {
				return nil, err
			}
			out = append(out, &x)
		}
		return out, rows.Err()
	}
	var err error
	if rep.ByDay, err = collect(`SELECT to_char(s.completed_at AT TIME ZONE 'UTC','YYYY-MM-DD') d, to_char(s.completed_at AT TIME ZONE 'UTC','YYYY-MM-DD'),
		count(*), 0::float8, sum(s.total)::float8, 0::float8, 0::float8 FROM adrient.pos_sales s WHERE `+inRange+` GROUP BY d ORDER BY d`, orgID, from, to); err != nil {
		return nil, err
	}
	if rep.ByMethod, err = collect(`SELECT p.method, p.method, count(*), 0::float8, sum(p.amount)::float8, 0::float8, 0::float8
		FROM adrient.pos_payments p JOIN adrient.pos_sales s ON s.id=p.sale_id WHERE `+inRange+` GROUP BY p.method ORDER BY 5 DESC`, orgID, from, to); err != nil {
		return nil, err
	}
	if rep.TopProducts, err = collect(`SELECT coalesce(l.product_id::text, l.name), max(l.name), count(*), sum(l.qty)::float8, sum(l.line_total)::float8, 0::float8, 0::float8
		FROM adrient.pos_sale_lines l JOIN adrient.pos_sales s ON s.id=l.sale_id WHERE `+inRange+` GROUP BY 1 ORDER BY 5 DESC LIMIT 10`, orgID, from, to); err != nil {
		return nil, err
	}
	if rep.TaxSummary, err = collect(`SELECT l.tax_name, l.tax_name, count(*), 0::float8, sum(l.tax_amount)::float8, l.tax_rate::float8,
		sum(l.line_total - l.tax_amount)::float8
		FROM adrient.pos_sale_lines l JOIN adrient.pos_sales s ON s.id=l.sale_id WHERE `+inRange+` AND l.tax_rate > 0
		GROUP BY l.tax_name, l.tax_rate ORDER BY l.tax_name`, orgID, from, to); err != nil {
		return nil, err
	}
	return rep, nil
}
