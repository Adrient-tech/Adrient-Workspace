package inventory

// Goods receipts (GRN) with partial receipts, over/under-receipt rules, landed
// cost, lots and serials, and returns to vendor.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GRNLine is one received line.
type GRNLine struct {
	ID          string   `json:"id"`
	POLineID    string   `json:"po_line_id"`
	ProductID   string   `json:"product_id"`
	ProductName string   `json:"product_name"`
	SKU         string   `json:"sku"`
	Qty         float64  `json:"qty"`
	UnitCost    float64  `json:"unit_cost"`
	LandedAlloc float64  `json:"landed_alloc"`
	LotNo       string   `json:"lot_no"`
	Expiry      string   `json:"expiry"`
	Serials     []string `json:"serials"`
	QtyReturned float64  `json:"qty_returned"`
}

// GRN is a goods receipt note.
type GRN struct {
	ID           string    `json:"id"`
	Number       string    `json:"number"`
	POID         string    `json:"po_id"`
	PONumber     string    `json:"po_number"`
	SupplierID   string    `json:"supplier_id"`
	SupplierName string    `json:"supplier_name"`
	LocationID   string    `json:"location_id"`
	LocationName string    `json:"location_name"`
	ReceivedAt   time.Time `json:"received_at"`
	DeliveryRef  string    `json:"delivery_ref"`
	Note         string    `json:"note"`
	LandedCost   float64   `json:"landed_cost"`
	LandedMethod string    `json:"landed_method"`
	LandedNote   string    `json:"landed_note"`
	Lines        []GRNLine `json:"lines"`
}

// GRNInput receives goods (against a PO or without one).
type GRNInput struct {
	POID         string  `json:"po_id"`
	SupplierID   string  `json:"supplier_id"`
	LocationID   string  `json:"location_id"`
	DeliveryRef  string  `json:"delivery_ref"`
	Note         string  `json:"note"`
	LandedCost   float64 `json:"landed_cost"`
	LandedMethod string  `json:"landed_method"`
	LandedNote   string  `json:"landed_note"`
	// CloseShort closes the order even if some lines were not fully received.
	CloseShort bool `json:"close_short"`
	Lines      []struct {
		POLineID  string   `json:"po_line_id"`
		ProductID string   `json:"product_id"`
		Qty       float64  `json:"qty"`
		UnitCost  *float64 `json:"unit_cost"`
		LotNo     string   `json:"lot_no"`
		Expiry    string   `json:"expiry"`
		Serials   []string `json:"serials"`
	} `json:"lines"`
}

// CreateGRN books a goods receipt: stock goes in, cost layers are created at the
// PO price plus allocated landed cost, and the PO moves to partial or received.
func (r *Repository) CreateGRN(ctx context.Context, orgID, userID string, in GRNInput) (GRN, error) {
	if len(in.Lines) == 0 {
		return GRN{}, invalid("receive at least one line")
	}
	if in.LandedCost < 0 {
		return GRN{}, invalid("landed cost cannot be negative")
	}
	if in.LandedMethod == "" {
		in.LandedMethod = "value"
	}
	if in.LandedMethod != "value" && in.LandedMethod != "qty" {
		return GRN{}, invalid("landed cost is split by value or qty")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return GRN{}, err
	}
	defer tx.Rollback(ctx)
	st, err := getSettings(ctx, tx, orgID)
	if err != nil {
		return GRN{}, err
	}
	supplier := in.SupplierID
	poState := ""
	if in.POID != "" {
		if err := tx.QueryRow(ctx, `SELECT state, supplier_id::text FROM adrient.inv_purchase_orders WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, in.POID).Scan(&poState, &supplier); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return GRN{}, ErrNotFound
			}
			return GRN{}, err
		}
		if !POCanReceive(poState) {
			return GRN{}, invalid("a %s purchase order cannot receive goods", poState)
		}
	}
	var supName string
	if err := tx.QueryRow(ctx, `SELECT name FROM adrient.inv_suppliers WHERE org_id=$1 AND id=$2`, orgID, supplier).Scan(&supName); err != nil {
		return GRN{}, invalid("pick a supplier")
	}
	loc := in.LocationID
	if loc == "" && in.POID != "" {
		_ = tx.QueryRow(ctx, `SELECT COALESCE(ship_to_loc_id::text,'') FROM adrient.inv_purchase_orders WHERE id=$1`, in.POID).Scan(&loc)
	}
	if loc == "" {
		if loc, err = r.ensureDefaultLocation(ctx, tx, orgID); err != nil {
			return GRN{}, err
		}
	}
	num, err := nextNumber(ctx, tx, orgID, "grn", "GRN")
	if err != nil {
		return GRN{}, err
	}
	grnID := uuid.NewString()
	if _, err := tx.Exec(ctx,
		`INSERT INTO adrient.inv_grns (id, org_id, number, po_id, supplier_id, location_id, delivery_ref, note, landed_cost, landed_method, landed_note, created_by)
		 VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,'')::uuid)`,
		grnID, orgID, num, in.POID, supplier, loc, in.DeliveryRef, in.Note, round2(in.LandedCost), in.LandedMethod, in.LandedNote, userID); err != nil {
		return GRN{}, err
	}

	type rl struct {
		poLine, product, lot, expiry string
		qty, cost                    float64
		serials                      []string
		tracking                     string
		name                         string
	}
	var lines []rl
	merged := map[string]int{}
	for _, l := range in.Lines {
		if l.Qty <= 0 {
			return GRN{}, invalid("received quantities must be positive")
		}
		x := rl{poLine: l.POLineID, product: l.ProductID, qty: round3(l.Qty), lot: strings.TrimSpace(l.LotNo), expiry: l.Expiry, serials: l.Serials}
		if l.POLineID != "" {
			var pid string
			if err := tx.QueryRow(ctx, `SELECT product_id::text FROM adrient.inv_purchase_order_lines WHERE org_id=$1 AND id=$2 AND po_id=$3`, orgID, l.POLineID, in.POID).Scan(&pid); err != nil {
				return GRN{}, invalid("a line does not belong to this purchase order")
			}
			x.product = pid
			if err := tx.QueryRow(ctx, `SELECT unit_cost::float8 FROM adrient.inv_purchase_order_lines WHERE id=$1`, l.POLineID).Scan(&x.cost); err != nil {
				return GRN{}, err
			}
		} else if in.POID != "" {
			return GRN{}, invalid("lines of a receipt against a purchase order must reference an order line")
		}
		if l.UnitCost != nil {
			if *l.UnitCost < 0 {
				return GRN{}, invalid("unit cost cannot be negative")
			}
			x.cost = *l.UnitCost
		} else if l.POLineID == "" {
			c, _, err := r.priceFor(ctx, tx, orgID, supplier, x.product, x.qty)
			if err != nil {
				return GRN{}, err
			}
			x.cost = c
		}
		if err := tx.QueryRow(ctx, `SELECT tracking, name FROM adrient.inv_products WHERE org_id=$1 AND id=$2`, orgID, x.product).Scan(&x.tracking, &x.name); err != nil {
			return GRN{}, invalid("product not found")
		}
		switch x.tracking {
		case "lot":
			if x.lot == "" {
				return GRN{}, invalid("%s needs a lot number", x.name)
			}
		case "serial":
			if len(x.serials) != int(x.qty) || x.qty != float64(int(x.qty)) {
				return GRN{}, invalid("%s needs exactly %v serial number(s)", x.name, x.qty)
			}
		}
		key := x.poLine + "|" + x.product + "|" + x.lot
		if i, ok := merged[key]; ok && x.tracking != "serial" {
			lines[i].qty = round3(lines[i].qty + x.qty)
			continue
		}
		merged[key] = len(lines)
		lines = append(lines, x)
	}

	// Over/under receipt rules, per PO line (summing lines that hit the same PO line).
	perPO := map[string]float64{}
	for _, l := range lines {
		if l.poLine != "" {
			perPO[l.poLine] += l.qty
		}
	}
	for pl, q := range perPO {
		var ordered, got float64
		var name string
		if err := tx.QueryRow(ctx, `SELECT l.qty::float8, l.qty_received::float8, p.name FROM adrient.inv_purchase_order_lines l JOIN adrient.inv_products p ON p.id=l.product_id WHERE l.id=$1 FOR UPDATE OF l`, pl).Scan(&ordered, &got, &name); err != nil {
			return GRN{}, err
		}
		if limit := ReceiptLimit(ordered, got, st.OverReceiptPct); round3(q) > limit+0.0005 {
			return GRN{}, invalid("%s: ordered %v, already received %v; at most %v more can be accepted (over-receipt allowed: %v%%)", name, ordered, got, limit, st.OverReceiptPct)
		}
	}

	qtys, costs := make([]float64, len(lines)), make([]float64, len(lines))
	for i, l := range lines {
		qtys[i], costs[i] = l.qty, l.cost
	}
	extra := AllocateLanded(in.LandedCost, in.LandedMethod, qtys, costs)

	for i, l := range lines {
		lineID := uuid.NewString()
		a := AdjustInput{LocationID: loc, Qty: l.qty, Reason: "receive", HistoryReason: "grn", UnitCost: round4(l.cost + extra[i]),
			Ref: num, LayerRef: lineID, Party: supName, Note: "Goods receipt " + num}
		if l.tracking == "lot" {
			a.Lots = []LotPick{{LotNo: l.lot, Expiry: l.expiry, Qty: l.qty}}
		}
		if l.tracking == "serial" {
			a.Serials = l.serials
		}
		if _, err := r.applyAdjust(ctx, tx, orgID, userID, l.product, a); err != nil {
			return GRN{}, err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO adrient.inv_grn_lines (id, grn_id, org_id, po_line_id, product_id, qty, unit_cost, landed_alloc, lot_no, expiry, serials)
			 VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,NULLIF($10,'')::date,$11)`,
			lineID, grnID, orgID, l.poLine, l.product, l.qty, round4(l.cost), extra[i], l.lot, l.expiry, nz(l.serials)); err != nil {
			return GRN{}, err
		}
		if l.poLine != "" {
			if _, err := tx.Exec(ctx, `UPDATE adrient.inv_purchase_order_lines SET qty_received = qty_received + $2 WHERE id=$1`, l.poLine, l.qty); err != nil {
				return GRN{}, err
			}
		}
	}

	if in.POID != "" {
		rows, err := tx.Query(ctx, `SELECT qty::float8, qty_received::float8 FROM adrient.inv_purchase_order_lines WHERE po_id=$1`, in.POID)
		if err != nil {
			return GRN{}, err
		}
		var pairs [][2]float64
		for rows.Next() {
			var p [2]float64
			if err := rows.Scan(&p[0], &p[1]); err != nil {
				rows.Close()
				return GRN{}, err
			}
			pairs = append(pairs, p)
		}
		rows.Close()
		next := POStateAfterReceipt(pairs, in.CloseShort || st.UnderReceiptPolicy == "close")
		if next != poState && !POCanMove(poState, next) {
			next = poState
		}
		if next == POsent { // nothing changed state-wise
			next = poState
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.inv_purchase_orders SET state=$2, closed_at=CASE WHEN $2='closed' THEN now() ELSE closed_at END, updated_at=now() WHERE id=$1`, in.POID, next); err != nil {
			return GRN{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return GRN{}, err
	}
	return r.GetGRN(ctx, orgID, grnID)
}

// GetGRN loads one receipt.
func (r *Repository) GetGRN(ctx context.Context, orgID, id string) (GRN, error) {
	gs, err := r.ListGRNs(ctx, orgID, id, "")
	if err != nil {
		return GRN{}, err
	}
	if len(gs) == 0 {
		return GRN{}, ErrNotFound
	}
	return gs[0], nil
}

// ListGRNs lists receipts (by id, or by purchase order).
func (r *Repository) ListGRNs(ctx context.Context, orgID, onlyID, poID string) ([]GRN, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT g.id::text, g.number, COALESCE(g.po_id::text,''), COALESCE(o.number,''), g.supplier_id::text, s.name, g.location_id::text, l.name,
		        g.received_at, g.delivery_ref, g.note, g.landed_cost::float8, g.landed_method, g.landed_note
		 FROM adrient.inv_grns g JOIN adrient.inv_suppliers s ON s.id=g.supplier_id JOIN adrient.inv_locations l ON l.id=g.location_id
		 LEFT JOIN adrient.inv_purchase_orders o ON o.id=g.po_id
		 WHERE g.org_id=$1 AND ($2='' OR g.id::text=$2) AND ($3='' OR g.po_id::text=$3) ORDER BY g.received_at DESC LIMIT 300`, orgID, onlyID, poID)
	if err != nil {
		return nil, err
	}
	out := []GRN{}
	idx := map[string]int{}
	var ids []string
	for rows.Next() {
		var g GRN
		if err := rows.Scan(&g.ID, &g.Number, &g.POID, &g.PONumber, &g.SupplierID, &g.SupplierName, &g.LocationID, &g.LocationName,
			&g.ReceivedAt, &g.DeliveryRef, &g.Note, &g.LandedCost, &g.LandedMethod, &g.LandedNote); err != nil {
			rows.Close()
			return nil, err
		}
		g.Lines = []GRNLine{}
		idx[g.ID] = len(out)
		out = append(out, g)
		ids = append(ids, g.ID)
	}
	rows.Close()
	lr, err := r.pool.Query(ctx,
		`SELECT l.grn_id::text, l.id::text, COALESCE(l.po_line_id::text,''), l.product_id::text, p.name, p.sku, l.qty::float8, l.unit_cost::float8, l.landed_alloc::float8,
		        l.lot_no, COALESCE(to_char(l.expiry,'YYYY-MM-DD'),''), l.serials, l.qty_returned::float8
		 FROM adrient.inv_grn_lines l JOIN adrient.inv_products p ON p.id=l.product_id WHERE l.org_id=$1 AND l.grn_id = ANY($2::uuid[]) ORDER BY l.id`, orgID, ids)
	if err != nil {
		return nil, err
	}
	defer lr.Close()
	for lr.Next() {
		var gid string
		var l GRNLine
		if err := lr.Scan(&gid, &l.ID, &l.POLineID, &l.ProductID, &l.ProductName, &l.SKU, &l.Qty, &l.UnitCost, &l.LandedAlloc, &l.LotNo, &l.Expiry, &l.Serials, &l.QtyReturned); err != nil {
			return nil, err
		}
		if l.Serials == nil {
			l.Serials = []string{}
		}
		out[idx[gid]].Lines = append(out[idx[gid]].Lines, l)
	}
	return out, lr.Err()
}

// ------------------------------------------------------------ vendor returns

// VendorReturn sends goods back to a supplier.
type VendorReturn struct {
	ID           string             `json:"id"`
	Number       string             `json:"number"`
	SupplierID   string             `json:"supplier_id"`
	SupplierName string             `json:"supplier_name"`
	GRNID        string             `json:"grn_id"`
	POID         string             `json:"po_id"`
	LocationID   string             `json:"location_id"`
	Reason       string             `json:"reason"`
	CreditNote   string             `json:"credit_note"`
	ReturnedAt   time.Time          `json:"returned_at"`
	Total        float64            `json:"total"`
	Lines        []VendorReturnLine `json:"lines"`
}

// VendorReturnLine is one returned line.
type VendorReturnLine struct {
	GRNLineID   string   `json:"grn_line_id"`
	ProductID   string   `json:"product_id"`
	ProductName string   `json:"product_name"`
	Qty         float64  `json:"qty"`
	UnitCost    float64  `json:"unit_cost"`
	LotNo       string   `json:"lot_no"`
	Serials     []string `json:"serials"`
}

// ReturnInput creates a return to vendor.
type ReturnInput struct {
	GRNID      string `json:"grn_id"`
	LocationID string `json:"location_id"`
	Reason     string `json:"reason"`
	CreditNote string `json:"credit_note"`
	Lines      []struct {
		GRNLineID string   `json:"grn_line_id"`
		Qty       float64  `json:"qty"`
		LotNo     string   `json:"lot_no"`
		Serials   []string `json:"serials"`
	} `json:"lines"`
}

// CreateReturn takes goods out of stock at their original receipt cost, lowers
// the PO's received quantity net of returns, and posts a reversal to Accounting
// for any part that was already billed.
func (r *Repository) CreateReturn(ctx context.Context, orgID, userID string, in ReturnInput) (VendorReturn, error) {
	if len(in.Lines) == 0 {
		return VendorReturn{}, invalid("return at least one line")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return VendorReturn{}, invalid("a reason is required")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return VendorReturn{}, err
	}
	defer tx.Rollback(ctx)
	var supplier, poID, supName string
	if err := tx.QueryRow(ctx,
		`SELECT g.supplier_id::text, COALESCE(g.po_id::text,''), s.name FROM adrient.inv_grns g JOIN adrient.inv_suppliers s ON s.id=g.supplier_id
		 WHERE g.org_id=$1 AND g.id=$2`, orgID, in.GRNID).Scan(&supplier, &poID, &supName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VendorReturn{}, invalid("pick the goods receipt the items came in on")
		}
		return VendorReturn{}, err
	}
	loc := in.LocationID
	if loc == "" {
		if err := tx.QueryRow(ctx, `SELECT location_id::text FROM adrient.inv_grns WHERE id=$1`, in.GRNID).Scan(&loc); err != nil {
			return VendorReturn{}, err
		}
	}
	num, err := nextNumber(ctx, tx, orgID, "vrn", "RTV")
	if err != nil {
		return VendorReturn{}, err
	}
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_vendor_returns (org_id, number, supplier_id, grn_id, po_id, location_id, reason, credit_note, created_by)
		 VALUES ($1,$2,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8,NULLIF($9,'')::uuid) RETURNING id::text`,
		orgID, num, supplier, in.GRNID, poID, loc, in.Reason, in.CreditNote, userID).Scan(&id); err != nil {
		return VendorReturn{}, err
	}
	st, err := getSettings(ctx, tx, orgID)
	if err != nil {
		return VendorReturn{}, err
	}
	var reverseNet, reverseTax float64
	for _, l := range in.Lines {
		if l.Qty <= 0 {
			return VendorReturn{}, invalid("returned quantities must be positive")
		}
		var product, poLine, lot string
		var qty, already, unit float64
		if err := tx.QueryRow(ctx,
			`SELECT product_id::text, COALESCE(po_line_id::text,''), qty::float8, qty_returned::float8, (unit_cost+landed_alloc)::float8, lot_no
			 FROM adrient.inv_grn_lines WHERE id=$1 AND grn_id=$2 AND org_id=$3 FOR UPDATE`, l.GRNLineID, in.GRNID, orgID).Scan(&product, &poLine, &qty, &already, &unit, &lot); err != nil {
			return VendorReturn{}, invalid("a line is not on that goods receipt")
		}
		if round3(already+l.Qty) > round3(qty) {
			return VendorReturn{}, invalid("only %v of this line is left to return", round3(qty-already))
		}
		var tracking string
		if err := tx.QueryRow(ctx, `SELECT tracking FROM adrient.inv_products WHERE id=$1`, product).Scan(&tracking); err != nil {
			return VendorReturn{}, err
		}
		a := AdjustInput{LocationID: loc, Qty: l.Qty, Reason: "remove", HistoryReason: "vendor_return", Ref: num, Party: supName,
			Note: "Returned to " + supName + ": " + in.Reason, PreferLayerRef: l.GRNLineID, IgnoreReservations: true}
		switch tracking {
		case "lot":
			ln := strings.TrimSpace(l.LotNo)
			if ln == "" {
				ln = lot
			}
			a.Lots = []LotPick{{LotNo: ln, Qty: l.Qty}}
		case "serial":
			a.Serials = l.Serials
		}
		out := &AdjustOut{}
		a.Out = out
		if _, err := r.applyAdjust(ctx, tx, orgID, userID, product, a); err != nil {
			return VendorReturn{}, err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO adrient.inv_vendor_return_lines (return_id, org_id, grn_line_id, product_id, qty, unit_cost, lot_no, serials) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			id, orgID, l.GRNLineID, product, round3(l.Qty), unit, lot, nz(out.Serials)); err != nil {
			return VendorReturn{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.inv_grn_lines SET qty_returned = qty_returned + $2 WHERE id=$1`, l.GRNLineID, l.Qty); err != nil {
			return VendorReturn{}, err
		}
		if poLine != "" {
			var recv, ret, billed, cost, taxPct float64
			if err := tx.QueryRow(ctx, `SELECT qty_received::float8, qty_returned::float8, qty_billed::float8, unit_cost::float8, tax_pct::float8
				FROM adrient.inv_purchase_order_lines WHERE id=$1 FOR UPDATE`, poLine).Scan(&recv, &ret, &billed, &cost, &taxPct); err != nil {
				return VendorReturn{}, err
			}
			overBefore := maxf(0, billed-(recv-ret))
			overAfter := maxf(0, billed-(recv-ret-l.Qty))
			if d := round3(overAfter - overBefore); d > 0 {
				reverseNet += round2(d * cost)
				reverseTax += round2(d * cost * taxPct / 100)
			}
			if _, err := tx.Exec(ctx, `UPDATE adrient.inv_purchase_order_lines SET qty_returned = qty_returned + $2 WHERE id=$1`, poLine, l.Qty); err != nil {
				return VendorReturn{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return VendorReturn{}, err
	}
	if reverseNet > 0 {
		// Best effort: a failure leaves the return booked; Accounting can be corrected by hand.
		if p := currentPoster(); p != nil {
			lines := []JournalLine{{AccountKey: "ap", Debit: round2(reverseNet + reverseTax), Memo: "Return " + num}, {AccountKey: "inventory", Credit: round2(reverseNet), Memo: "Return " + num}}
			lines = append(lines, taxLines(st.TaxSplit, reverseTax, true, "Return "+num)...)
			_ = p.PostEntry(ctx, orgID, userID, JournalEntry{Date: time.Now().Format("2006-01-02"), Memo: "Return to vendor " + num, Ref: num, Source: "purchase_return",
				SourceID: id, ContactName: supName, Lines: lines})
		}
	}
	return r.GetReturn(ctx, orgID, id)
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// taxLines splits input tax over the GST accounts. credit=true reverses it.
func taxLines(split string, tax float64, credit bool, memo string) []JournalLine {
	if tax <= 0 {
		return nil
	}
	mk := func(key string, v float64) JournalLine {
		if credit {
			return JournalLine{AccountKey: key, Credit: v, Memo: memo}
		}
		return JournalLine{AccountKey: key, Debit: v, Memo: memo}
	}
	if split == "igst" {
		return []JournalLine{mk("gst_in_igst", tax)}
	}
	half := round2(tax / 2)
	return []JournalLine{mk("gst_in_cgst", half), mk("gst_in_sgst", round2(tax-half))}
}

// GetReturn loads one return.
func (r *Repository) GetReturn(ctx context.Context, orgID, id string) (VendorReturn, error) {
	rs, err := r.ListReturns(ctx, orgID, id)
	if err != nil {
		return VendorReturn{}, err
	}
	if len(rs) == 0 {
		return VendorReturn{}, ErrNotFound
	}
	return rs[0], nil
}

// ListReturns lists returns to vendor.
func (r *Repository) ListReturns(ctx context.Context, orgID, onlyID string) ([]VendorReturn, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT v.id::text, v.number, v.supplier_id::text, s.name, COALESCE(v.grn_id::text,''), COALESCE(v.po_id::text,''), v.location_id::text, v.reason, v.credit_note, v.returned_at
		 FROM adrient.inv_vendor_returns v JOIN adrient.inv_suppliers s ON s.id=v.supplier_id
		 WHERE v.org_id=$1 AND ($2='' OR v.id::text=$2) ORDER BY v.returned_at DESC LIMIT 300`, orgID, onlyID)
	if err != nil {
		return nil, err
	}
	out := []VendorReturn{}
	idx := map[string]int{}
	var ids []string
	for rows.Next() {
		var v VendorReturn
		if err := rows.Scan(&v.ID, &v.Number, &v.SupplierID, &v.SupplierName, &v.GRNID, &v.POID, &v.LocationID, &v.Reason, &v.CreditNote, &v.ReturnedAt); err != nil {
			rows.Close()
			return nil, err
		}
		v.Lines = []VendorReturnLine{}
		idx[v.ID] = len(out)
		out = append(out, v)
		ids = append(ids, v.ID)
	}
	rows.Close()
	lr, err := r.pool.Query(ctx,
		`SELECT l.return_id::text, COALESCE(l.grn_line_id::text,''), l.product_id::text, p.name, l.qty::float8, l.unit_cost::float8, l.lot_no, l.serials
		 FROM adrient.inv_vendor_return_lines l JOIN adrient.inv_products p ON p.id=l.product_id WHERE l.org_id=$1 AND l.return_id = ANY($2::uuid[]) ORDER BY l.id`, orgID, ids)
	if err != nil {
		return nil, err
	}
	defer lr.Close()
	for lr.Next() {
		var rid string
		var l VendorReturnLine
		if err := lr.Scan(&rid, &l.GRNLineID, &l.ProductID, &l.ProductName, &l.Qty, &l.UnitCost, &l.LotNo, &l.Serials); err != nil {
			return nil, err
		}
		if l.Serials == nil {
			l.Serials = []string{}
		}
		out[idx[rid]].Lines = append(out[idx[rid]].Lines, l)
		out[idx[rid]].Total = round2(out[idx[rid]].Total + l.Qty*l.UnitCost)
	}
	return out, lr.Err()
}
