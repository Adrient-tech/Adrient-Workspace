package inventory

// The movement engine: every stock change that goes through applyAdjust also
// updates cost layers + the value ledger, lot/serial stock and reservations,
// inside the same transaction.

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// AdjustOut reports what a movement did so a caller can mirror it (a transfer
// receives the same lots and serials it shipped) or use its cost (COGS).
type AdjustOut struct {
	Picks     []LotPick
	Serials   []string
	UnitCost  float64
	Value     float64 // always positive: the money value moved
	QtyAfter  float64
	HistoryID string
}

type prodInfo struct {
	Tracking string
	Method   string
	Cost     float64
	Name     string
}

func isInternalMove(histReason string) bool {
	return histReason == "transfer" || histReason == "transfer_out" || histReason == "transfer_in"
}

func (r *Repository) loadProd(ctx context.Context, tx pgx.Tx, orgID, productID string) (prodInfo, error) {
	var p prodInfo
	err := tx.QueryRow(ctx,
		`SELECT tracking, valuation_method, cost::float8, name FROM adrient.inv_products WHERE org_id=$1 AND id=$2`,
		orgID, productID).Scan(&p.Tracking, &p.Method, &p.Cost, &p.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// afterMove runs the side effects of a stock movement that was just applied.
func (r *Repository) afterMove(ctx context.Context, tx pgx.Tx, orgID, userID, productID, loc string, delta, next float64, a AdjustInput, histReason, historyID string) error {
	p, err := r.loadProd(ctx, tx, orgID, productID)
	if err != nil {
		return err
	}
	if a.Out != nil {
		a.Out.QtyAfter = next
		a.Out.HistoryID = historyID
	}
	if p.Tracking == "lot" {
		if err := r.moveLots(ctx, tx, orgID, productID, loc, delta, a, histReason); err != nil {
			return err
		}
	} else if p.Tracking == "serial" {
		if err := r.moveSerials(ctx, tx, orgID, productID, loc, delta, a, histReason); err != nil {
			return err
		}
	}
	if !a.NoValuation && !isInternalMove(histReason) {
		if err := r.valueMove(ctx, tx, orgID, productID, loc, delta, p, a, histReason); err != nil {
			return err
		}
	}
	if delta < 0 && a.ReserveRef != "" {
		if err := fulfilReservations(ctx, tx, orgID, productID, a.ReserveRef, -delta); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- valuation

func journalKind(reason, histReason string) string {
	switch histReason {
	case "transfer", "transfer_out", "transfer_in", "grn", "assembly_in", "assembly_out", "vendor_return", "opening":
		return ""
	}
	switch reason {
	case "sale", "refund":
		return "stock_cogs"
	case "count", "correction", "remove":
		return "stock_adjustment"
	}
	return ""
}

func (r *Repository) openLayers(ctx context.Context, tx pgx.Tx, productID string, lock bool) ([]Layer, error) {
	q := `SELECT id::text, qty_remaining::float8, unit_cost::float8 FROM adrient.inv_cost_layers
	      WHERE product_id=$1 AND qty_remaining > 0 ORDER BY received_at, id`
	if lock {
		q += ` FOR UPDATE`
	}
	rows, err := tx.Query(ctx, q, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Layer
	for rows.Next() {
		var l Layer
		if err := rows.Scan(&l.ID, &l.Qty, &l.Cost); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Repository) valueMove(ctx context.Context, tx pgx.Tx, orgID, productID, loc string, delta float64, p prodInfo, a AdjustInput, histReason string) error {
	kind := journalKind(a.Reason, histReason)
	state := "none"
	if kind != "" {
		state = "pending"
	}
	reason := histReason
	if reason == "" {
		reason = a.Reason
	}
	if delta > 0 {
		layers, err := r.openLayers(ctx, tx, productID, true)
		if err != nil {
			return err
		}
		unit := a.UnitCost
		if unit <= 0 {
			unit = AverageCost(layers, p.Cost)
		}
		var total float64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(qty),0)::float8 FROM adrient.inv_stock WHERE product_id=$1`, productID).Scan(&total); err != nil {
			return err
		}
		prev := total - delta
		covered := math.Min(delta, math.Max(0, -prev)) // units that settle earlier overselling
		layerQty := round3(delta - covered)
		layerRef := a.LayerRef
		if layerRef == "" {
			layerRef = a.Ref
		}
		if layerQty > 0 {
			if _, err := tx.Exec(ctx,
				`INSERT INTO adrient.inv_cost_layers (org_id, product_id, qty_in, qty_remaining, unit_cost, source, ref)
				 VALUES ($1,$2,$3,$3,$4,$5,$6)`, orgID, productID, layerQty, round4(unit), reason, layerRef); err != nil {
				return err
			}
		}
		value := round2(layerQty*unit + covered*p.Cost)
		if a.Out != nil {
			a.Out.UnitCost, a.Out.Value = round4(unit), value
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO adrient.inv_value_ledger (org_id, product_id, location_id, qty, unit_cost, value, reason, ref, journal_kind, journal_state)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			orgID, productID, loc, delta, round4(unit), value, reason, a.Ref, kind, state)
		return err
	}
	// Outbound.
	layers, err := r.openLayers(ctx, tx, productID, true)
	if err != nil {
		return err
	}
	prefer := map[string]bool{}
	if a.PreferLayerRef != "" {
		rows, err := tx.Query(ctx, `SELECT id::text FROM adrient.inv_cost_layers WHERE product_id=$1 AND ref=$2 AND qty_remaining>0`, productID, a.PreferLayerRef)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			prefer[id] = true
		}
		rows.Close()
	}
	fallback := p.Cost
	if fallback <= 0 {
		fallback = AverageCost(layers, 0)
	}
	cons := ConsumeLayers(p.Method, layers, -delta, prefer, fallback)
	for _, c := range cons {
		if c.LayerID == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.inv_cost_layers SET qty_remaining = qty_remaining - $2 WHERE id=$1`, c.LayerID, c.Qty); err != nil {
			return err
		}
	}
	value := ConsumedValue(cons)
	unit := 0.0
	if q := -delta; q > 0 {
		unit = round4(value / q)
	}
	if a.Out != nil {
		a.Out.UnitCost, a.Out.Value = unit, value
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO adrient.inv_value_ledger (org_id, product_id, location_id, qty, unit_cost, value, reason, ref, journal_kind, journal_state)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		orgID, productID, loc, delta, unit, -value, reason, a.Ref, kind, state)
	return err
}

// --------------------------------------------------------------- lots

func parseDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, invalid("dates must be YYYY-MM-DD")
	}
	return &t, nil
}

// ensureLot returns the lot id, creating the lot when new.
func ensureLot(ctx context.Context, tx pgx.Tx, orgID, productID, lotNo, expiry string) (string, error) {
	lotNo = strings.TrimSpace(lotNo)
	if lotNo == "" {
		lotNo = "UNASSIGNED"
	}
	exp, err := parseDate(expiry)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_lots (org_id, product_id, lot_no, expiry) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (org_id, product_id, lower(lot_no)) DO UPDATE SET expiry = COALESCE(adrient.inv_lots.expiry, EXCLUDED.expiry)
		 RETURNING id::text`, orgID, productID, lotNo, exp).Scan(&id)
	return id, err
}

func (r *Repository) lotStockAt(ctx context.Context, tx pgx.Tx, orgID, productID, loc string, skipRecalled bool) ([]LotStock, error) {
	rows, err := tx.Query(ctx,
		`SELECT l.id::text, l.lot_no, l.expiry, s.qty::float8
		 FROM adrient.inv_lot_stock s JOIN adrient.inv_lots l ON l.id=s.lot_id
		 WHERE s.org_id=$1 AND s.product_id=$2 AND s.location_id=$3 AND s.qty > 0 AND (NOT $4 OR NOT l.recalled)
		 FOR UPDATE OF s`, orgID, productID, loc, skipRecalled)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LotStock
	for rows.Next() {
		var s LotStock
		if err := rows.Scan(&s.LotID, &s.LotNo, &s.Expiry, &s.Qty); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func addTrace(ctx context.Context, tx pgx.Tx, orgID, productID, lotID, serialID, loc string, qty float64, reason, ref, party string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO adrient.inv_trace (org_id, product_id, lot_id, serial_id, location_id, qty, reason, ref, party)
		 VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9)`,
		orgID, productID, lotID, serialID, loc, qty, reason, ref, party)
	return err
}

func (r *Repository) moveLots(ctx context.Context, tx pgx.Tx, orgID, productID, loc string, delta float64, a AdjustInput, histReason string) error {
	reason := histReason
	if reason == "" {
		reason = a.Reason
	}
	if delta > 0 {
		picks := a.Lots
		if len(picks) == 0 {
			picks = []LotPick{{LotNo: "UNASSIGNED", Qty: delta}}
		}
		var sum float64
		for _, p := range picks {
			if p.Qty <= 0 {
				return invalid("lot quantity must be positive")
			}
			sum += p.Qty
		}
		if math.Abs(sum-delta) > 0.0005 {
			return invalid("lot quantities add up to %v but %v is being received", round3(sum), round3(delta))
		}
		for i, p := range picks {
			id, err := ensureLot(ctx, tx, orgID, productID, p.LotNo, p.Expiry)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO adrient.inv_lot_stock (lot_id, location_id, org_id, product_id, qty) VALUES ($1,$2,$3,$4,$5)
				 ON CONFLICT (lot_id, location_id) DO UPDATE SET qty = adrient.inv_lot_stock.qty + EXCLUDED.qty`,
				id, loc, orgID, productID, p.Qty); err != nil {
				return err
			}
			if err := addTrace(ctx, tx, orgID, productID, id, "", loc, p.Qty, reason, a.Ref, a.Party); err != nil {
				return err
			}
			picks[i].LotID = id
		}
		if a.Out != nil {
			a.Out.Picks = picks
		}
		return nil
	}
	need := round3(-delta)
	stock, err := r.lotStockAt(ctx, tx, orgID, productID, loc, a.Reason == "sale")
	if err != nil {
		return err
	}
	var picks []LotPick
	if len(a.Lots) > 0 {
		have := map[string]LotStock{}
		for _, s := range stock {
			have[strings.ToLower(s.LotNo)] = s
			have["id:"+s.LotID] = s
		}
		var sum float64
		for _, p := range a.Lots {
			s, ok := have[strings.ToLower(strings.TrimSpace(p.LotNo))]
			if !ok && p.LotID != "" {
				s, ok = have["id:"+p.LotID]
			}
			if !ok || s.Qty+0.0005 < p.Qty {
				return invalid("lot %s does not have %v here", p.LotNo, p.Qty)
			}
			sum += p.Qty
			lp := LotPick{LotID: s.LotID, LotNo: s.LotNo, Qty: p.Qty}
			if s.Expiry != nil {
				lp.Expiry = s.Expiry.Format("2006-01-02")
			}
			picks = append(picks, lp)
		}
		if math.Abs(sum-need) > 0.0005 {
			return invalid("lot quantities add up to %v but %v is being removed", round3(sum), need)
		}
	} else {
		allowExpired := a.Reason != "sale" && !isInternalMove(histReason)
		var short float64
		picks, short = PickFEFO(stock, need, time.Now(), allowExpired)
		if short > 0.0005 {
			return invalid("not enough in-date lot stock: %v short", short)
		}
	}
	for _, p := range picks {
		if _, err := tx.Exec(ctx, `UPDATE adrient.inv_lot_stock SET qty = qty - $3 WHERE lot_id=$1 AND location_id=$2`, p.LotID, loc, p.Qty); err != nil {
			return err
		}
		if err := addTrace(ctx, tx, orgID, productID, p.LotID, "", loc, -p.Qty, reason, a.Ref, a.Party); err != nil {
			return err
		}
	}
	if a.Out != nil {
		a.Out.Picks = picks
	}
	return nil
}

// ------------------------------------------------------------- serials

func (r *Repository) moveSerials(ctx context.Context, tx pgx.Tx, orgID, productID, loc string, delta float64, a AdjustInput, histReason string) error {
	reason := histReason
	if reason == "" {
		reason = a.Reason
	}
	if math.Abs(delta-math.Round(delta)) > 0.0005 {
		return invalid("serial-tracked items move in whole units")
	}
	n := int(math.Round(math.Abs(delta)))
	if delta > 0 {
		if len(a.Serials) != n {
			return invalid("enter %d serial number(s) for this receipt", n)
		}
		seen := map[string]bool{}
		for _, s := range a.Serials {
			s = strings.TrimSpace(s)
			if s == "" || seen[strings.ToLower(s)] {
				return invalid("serial numbers must be filled in and unique")
			}
			seen[strings.ToLower(s)] = true
			var id, state string
			err := tx.QueryRow(ctx, `SELECT id::text, state FROM adrient.inv_serials WHERE org_id=$1 AND product_id=$2 AND lower(serial_no)=lower($3) FOR UPDATE`,
				orgID, productID, s).Scan(&id, &state)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				err = tx.QueryRow(ctx,
					`INSERT INTO adrient.inv_serials (org_id, product_id, serial_no, location_id) VALUES ($1,$2,$3,$4) RETURNING id::text`,
					orgID, productID, s, loc).Scan(&id)
				if err != nil {
					return err
				}
			case err != nil:
				return err
			case state == "in_stock":
				return invalid("serial %s is already in stock", s)
			default:
				if _, err := tx.Exec(ctx, `UPDATE adrient.inv_serials SET state='in_stock', location_id=$2, out_ref='', out_at=NULL WHERE id=$1`, id, loc); err != nil {
					return err
				}
			}
			if err := addTrace(ctx, tx, orgID, productID, "", id, loc, 1, reason, a.Ref, a.Party); err != nil {
				return err
			}
		}
		if a.Out != nil {
			a.Out.Serials = a.Serials
		}
		return nil
	}
	var chosen []string
	if len(a.Serials) > 0 {
		if len(a.Serials) != n {
			return invalid("enter %d serial number(s)", n)
		}
		chosen = a.Serials
	} else {
		rows, err := tx.Query(ctx,
			`SELECT serial_no FROM adrient.inv_serials WHERE org_id=$1 AND product_id=$2 AND state='in_stock' AND location_id=$3
			 ORDER BY received_at, serial_no LIMIT $4 FOR UPDATE`, orgID, productID, loc, n)
		if err != nil {
			return err
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				rows.Close()
				return err
			}
			chosen = append(chosen, s)
		}
		rows.Close()
		if len(chosen) < n {
			return invalid("only %d serial number(s) in stock here", len(chosen))
		}
	}
	for _, s := range chosen {
		var id string
		err := tx.QueryRow(ctx,
			`UPDATE adrient.inv_serials SET state='out', location_id=NULL, out_ref=$5, out_at=now()
			 WHERE org_id=$1 AND product_id=$2 AND lower(serial_no)=lower($3) AND state='in_stock' AND location_id=$4
			 RETURNING id::text`, orgID, productID, strings.TrimSpace(s), loc, a.Ref).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("serial %s is not in stock here", s)
		}
		if err != nil {
			return err
		}
		if err := addTrace(ctx, tx, orgID, productID, "", id, loc, -1, reason, a.Ref, a.Party); err != nil {
			return err
		}
	}
	if a.Out != nil {
		a.Out.Serials = chosen
	}
	return nil
}

// -------------------------------------------------------- reservations

func fulfilReservations(ctx context.Context, tx pgx.Tx, orgID, productID, ref string, qty float64) error {
	rows, err := tx.Query(ctx,
		`SELECT id::text, qty::float8 FROM adrient.inv_reservations
		 WHERE org_id=$1 AND product_id=$2 AND ref=$3 AND state='active' ORDER BY created_at FOR UPDATE`, orgID, productID, ref)
	if err != nil {
		return err
	}
	type rv struct {
		id  string
		qty float64
	}
	var list []rv
	for rows.Next() {
		var x rv
		if err := rows.Scan(&x.id, &x.qty); err != nil {
			rows.Close()
			return err
		}
		list = append(list, x)
	}
	rows.Close()
	left := qty
	for _, x := range list {
		if left <= 0 {
			break
		}
		if x.qty <= left+0.0005 {
			left = round3(left - x.qty)
			if _, err := tx.Exec(ctx, `UPDATE adrient.inv_reservations SET state='fulfilled', closed_at=now() WHERE id=$1`, x.id); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `UPDATE adrient.inv_reservations SET qty=qty-$2 WHERE id=$1`, x.id, left); err != nil {
				return err
			}
			left = 0
		}
	}
	return nil
}

// reservedElsewhere is the active reserved quantity of a product that does not
// belong to ref.
func reservedElsewhere(ctx context.Context, tx pgx.Tx, orgID, productID, ref string) (float64, error) {
	var q float64
	err := tx.QueryRow(ctx,
		`SELECT COALESCE(sum(qty),0)::float8 FROM adrient.inv_reservations
		 WHERE org_id=$1 AND product_id=$2 AND state='active' AND ($3='' OR ref<>$3) AND (expires_at IS NULL OR expires_at > now())`,
		orgID, productID, ref).Scan(&q)
	return q, err
}

// ------------------------------------------------------------------ kits

type kitPart struct {
	ProductID string
	Qty       float64
}

// kitParts returns the components of a kit product (nil when it is not a kit).
func kitParts(ctx context.Context, tx pgx.Tx, orgID, productID string) ([]kitPart, float64, error) {
	var bomID string
	var per float64
	err := tx.QueryRow(ctx, `SELECT id::text, qty::float8 FROM adrient.inv_boms WHERE org_id=$1 AND product_id=$2 AND kind='kit'`, orgID, productID).Scan(&bomID, &per)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT component_id::text, qty::float8 FROM adrient.inv_bom_lines WHERE bom_id=$1 ORDER BY id`, bomID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var parts []kitPart
	for rows.Next() {
		var k kitPart
		if err := rows.Scan(&k.ProductID, &k.Qty); err != nil {
			return nil, 0, err
		}
		parts = append(parts, k)
	}
	return parts, per, rows.Err()
}

// explodeKit applies a sale/refund/remove of a kit to its components.
func (r *Repository) explodeKit(ctx context.Context, tx pgx.Tx, orgID, userID, productID string, a AdjustInput, parts []kitPart, per float64) (float64, error) {
	var cost float64
	qty := math.Abs(a.Qty)
	for _, k := range parts {
		sub := a
		sub.Qty = round3(qty * k.Qty / per)
		sub.Out = &AdjustOut{}
		sub.Lots, sub.Serials = nil, nil
		sub.Note = strings.TrimSpace("Kit component. " + a.Note)
		if _, err := r.applyAdjust(ctx, tx, orgID, userID, k.ProductID, sub); err != nil {
			return 0, err
		}
		cost += sub.Out.Value
	}
	delta := signedDelta(a)
	loc := a.LocationID
	if loc == "" {
		var err error
		if loc, err = r.ensureDefaultLocation(ctx, tx, orgID); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO adrient.inv_history (org_id, product_id, location_id, delta, qty_after, reason, note, ref, user_id)
		 VALUES ($1,$2,$3,$4,0,$5,$6,$7,NULLIF($8,'')::uuid)`,
		orgID, productID, loc, delta, a.Reason, "Kit: components moved. "+a.Note, a.Ref, userID); err != nil {
		return 0, err
	}
	if a.Out != nil {
		a.Out.Value = round2(cost)
		if qty > 0 {
			a.Out.UnitCost = round4(cost / qty)
		}
	}
	return 0, nil
}
