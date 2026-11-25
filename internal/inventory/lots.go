package inventory

// Lot and serial queries: lists, expiry, traceability and recall reports.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Lot is a batch with its on-hand quantity.
type Lot struct {
	ID          string  `json:"id"`
	ProductID   string  `json:"product_id"`
	ProductName string  `json:"product_name"`
	SKU         string  `json:"sku"`
	LotNo       string  `json:"lot_no"`
	Expiry      string  `json:"expiry"`
	MfgDate     string  `json:"mfg_date"`
	Note        string  `json:"note"`
	Recalled    bool    `json:"recalled"`
	OnHand      float64 `json:"on_hand"`
	DaysLeft    *int    `json:"days_left"` // nil without expiry; negative when expired
	Status      string  `json:"status"`    // ok | expiring | expired | recalled
	Locations   []LotAt `json:"locations"`
}

// LotAt is quantity of a lot at a location.
type LotAt struct {
	LocationID string  `json:"location_id"`
	Path       string  `json:"path"`
	Qty        float64 `json:"qty"`
}

func lotStatus(recalled bool, expiry *time.Time, soonDays int) (string, *int) {
	if expiry == nil {
		if recalled {
			return "recalled", nil
		}
		return "ok", nil
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	d := int(expiry.Sub(today).Hours() / 24)
	switch {
	case recalled:
		return "recalled", &d
	case d < 0:
		return "expired", &d
	case d <= soonDays:
		return "expiring", &d
	}
	return "ok", &d
}

// ListLots lists lots by product, with on-hand totals. onlyStock hides empty lots;
// expiringDays>0 keeps lots that expire within that many days (or already did).
func (r *Repository) ListLots(ctx context.Context, orgID, productID string, onlyStock bool, expiringDays int, query string) ([]Lot, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT l.id::text, l.product_id::text, p.name, p.sku, l.lot_no, l.expiry, l.mfg_date, l.note, l.recalled,
		        COALESCE((SELECT sum(qty) FROM adrient.inv_lot_stock s WHERE s.lot_id=l.id),0)::float8
		 FROM adrient.inv_lots l JOIN adrient.inv_products p ON p.id=l.product_id
		 WHERE l.org_id=$1 AND ($2='' OR l.product_id::text=$2) AND ($5='' OR l.lot_no ILIKE '%'||$5||'%' OR p.name ILIKE '%'||$5||'%' OR p.sku ILIKE '%'||$5||'%')
		   AND ($3 = false OR EXISTS (SELECT 1 FROM adrient.inv_lot_stock s WHERE s.lot_id=l.id AND s.qty>0))
		   AND ($4 <= 0 OR (l.expiry IS NOT NULL AND l.expiry <= CURRENT_DATE + $4::int))
		 ORDER BY l.expiry NULLS LAST, l.lot_no LIMIT 1000`, orgID, productID, onlyStock, expiringDays, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lot{}
	for rows.Next() {
		var l Lot
		var exp, mfg *time.Time
		if err := rows.Scan(&l.ID, &l.ProductID, &l.ProductName, &l.SKU, &l.LotNo, &exp, &mfg, &l.Note, &l.Recalled, &l.OnHand); err != nil {
			return nil, err
		}
		if exp != nil {
			l.Expiry = exp.Format("2006-01-02")
		}
		if mfg != nil {
			l.MfgDate = mfg.Format("2006-01-02")
		}
		l.OnHand = round3(l.OnHand)
		l.Status, l.DaysLeft = lotStatus(l.Recalled, exp, 30)
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// LotLocations fills the location split of one lot.
func (r *Repository) LotLocations(ctx context.Context, orgID, lotID string) ([]LotAt, error) {
	tree, err := r.ListLocationTree(ctx, orgID)
	if err != nil {
		return nil, err
	}
	path := map[string]string{}
	for _, l := range tree {
		path[l.ID] = l.Path
	}
	rows, err := r.pool.Query(ctx, `SELECT location_id::text, qty::float8 FROM adrient.inv_lot_stock WHERE org_id=$1 AND lot_id=$2 AND qty<>0`, orgID, lotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LotAt{}
	for rows.Next() {
		var a LotAt
		if err := rows.Scan(&a.LocationID, &a.Qty); err != nil {
			return nil, err
		}
		a.Path = path[a.LocationID]
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateLot edits expiry, manufacture date, note and the recall flag.
func (r *Repository) UpdateLot(ctx context.Context, orgID, id string, expiry, mfg, note string, recalled bool) (Lot, error) {
	e, err := parseDate(expiry)
	if err != nil {
		return Lot{}, err
	}
	m, err := parseDate(mfg)
	if err != nil {
		return Lot{}, err
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.inv_lots SET expiry=$3, mfg_date=$4, note=$5, recalled=$6 WHERE org_id=$1 AND id=$2`, orgID, id, e, m, note, recalled)
	if err != nil {
		return Lot{}, err
	}
	if tag.RowsAffected() == 0 {
		return Lot{}, ErrNotFound
	}
	return r.GetLot(ctx, orgID, id)
}

// GetLot returns a single lot with its locations.
func (r *Repository) GetLot(ctx context.Context, orgID, id string) (Lot, error) {
	var l Lot
	var exp, mfg *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT l.id::text, l.product_id::text, p.name, p.sku, l.lot_no, l.expiry, l.mfg_date, l.note, l.recalled,
		        COALESCE((SELECT sum(qty) FROM adrient.inv_lot_stock s WHERE s.lot_id=l.id),0)::float8
		 FROM adrient.inv_lots l JOIN adrient.inv_products p ON p.id=l.product_id WHERE l.org_id=$1 AND l.id=$2`, orgID, id).
		Scan(&l.ID, &l.ProductID, &l.ProductName, &l.SKU, &l.LotNo, &exp, &mfg, &l.Note, &l.Recalled, &l.OnHand)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	if exp != nil {
		l.Expiry = exp.Format("2006-01-02")
	}
	if mfg != nil {
		l.MfgDate = mfg.Format("2006-01-02")
	}
	l.OnHand = round3(l.OnHand)
	l.Status, l.DaysLeft = lotStatus(l.Recalled, exp, 30)
	l.Locations, err = r.LotLocations(ctx, orgID, id)
	return l, err
}

// TraceRow is one movement of a lot or serial.
type TraceRow struct {
	At         time.Time `json:"at"`
	LocationID string    `json:"location_id"`
	Location   string    `json:"location"`
	Qty        float64   `json:"qty"`
	Reason     string    `json:"reason"`
	Ref        string    `json:"ref"`
	Party      string    `json:"party"`
	Serial     string    `json:"serial"`
}

// Recall is the traceability answer for a lot: where it came from, where it went
// and where what is left sits.
type Recall struct {
	Lot      Lot        `json:"lot"`
	Received float64    `json:"received"`
	Shipped  float64    `json:"shipped"`  // left the business (sales and other issues)
	Moves    []TraceRow `json:"moves"`    // every movement, oldest first
	Outbound []TraceRow `json:"outbound"` // customers/documents it went to
	Origins  []TraceRow `json:"origins"`  // goods receipts and other ways in
}

var inboundOrigins = map[string]bool{"grn": true, "receive": true, "assembly_in": true, "refund": true, "opening": true}
var internalReasons = map[string]bool{"transfer": true, "transfer_out": true, "transfer_in": true}

// LotRecall builds the "where did lot X go" report.
func (r *Repository) LotRecall(ctx context.Context, orgID, lotID string) (Recall, error) {
	var rc Recall
	lot, err := r.GetLot(ctx, orgID, lotID)
	if err != nil {
		return rc, err
	}
	rc.Lot = lot
	rows, err := r.pool.Query(ctx,
		`SELECT t.created_at, COALESCE(t.location_id::text,''), COALESCE(l.name,''), t.qty::float8, t.reason, t.ref, t.party
		 FROM adrient.inv_trace t LEFT JOIN adrient.inv_locations l ON l.id=t.location_id
		 WHERE t.org_id=$1 AND t.lot_id=$2 ORDER BY t.created_at, t.id`, orgID, lotID)
	if err != nil {
		return rc, err
	}
	defer rows.Close()
	rc.Moves, rc.Outbound, rc.Origins = []TraceRow{}, []TraceRow{}, []TraceRow{}
	for rows.Next() {
		var x TraceRow
		if err := rows.Scan(&x.At, &x.LocationID, &x.Location, &x.Qty, &x.Reason, &x.Ref, &x.Party); err != nil {
			return rc, err
		}
		rc.Moves = append(rc.Moves, x)
		switch {
		case internalReasons[x.Reason]:
		case x.Qty > 0 && inboundOrigins[x.Reason]:
			rc.Origins = append(rc.Origins, x)
			rc.Received += x.Qty
		case x.Qty > 0:
			rc.Received += x.Qty
		default:
			rc.Outbound = append(rc.Outbound, x)
			rc.Shipped += -x.Qty
		}
	}
	rc.Received, rc.Shipped = round3(rc.Received), round3(rc.Shipped)
	return rc, rows.Err()
}

// Serial is one serial-numbered unit.
type Serial struct {
	ID         string     `json:"id"`
	ProductID  string     `json:"product_id"`
	Product    string     `json:"product_name"`
	SerialNo   string     `json:"serial_no"`
	State      string     `json:"state"`
	Location   string     `json:"location"`
	ReceivedAt time.Time  `json:"received_at"`
	OutRef     string     `json:"out_ref"`
	OutAt      *time.Time `json:"out_at"`
}

// ListSerials lists serial numbers of a product (or finds one by text).
func (r *Repository) ListSerials(ctx context.Context, orgID, productID, state, query string) ([]Serial, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT s.id::text, s.product_id::text, p.name, s.serial_no, s.state, COALESCE(l.name,''), s.received_at, s.out_ref, s.out_at
		 FROM adrient.inv_serials s JOIN adrient.inv_products p ON p.id=s.product_id LEFT JOIN adrient.inv_locations l ON l.id=s.location_id
		 WHERE s.org_id=$1 AND ($2='' OR s.product_id::text=$2) AND ($3='' OR s.state=$3) AND ($4='' OR s.serial_no ILIKE '%'||$4||'%')
		 ORDER BY s.received_at DESC, s.serial_no LIMIT 500`, orgID, productID, state, strings.TrimSpace(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Serial{}
	for rows.Next() {
		var s Serial
		if err := rows.Scan(&s.ID, &s.ProductID, &s.Product, &s.SerialNo, &s.State, &s.Location, &s.ReceivedAt, &s.OutRef, &s.OutAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SerialHistory is every movement of one serial number.
func (r *Repository) SerialHistory(ctx context.Context, orgID, serialID string) ([]TraceRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT t.created_at, COALESCE(t.location_id::text,''), COALESCE(l.name,''), t.qty::float8, t.reason, t.ref, t.party
		 FROM adrient.inv_trace t LEFT JOIN adrient.inv_locations l ON l.id=t.location_id
		 WHERE t.org_id=$1 AND t.serial_id=$2 ORDER BY t.created_at, t.id`, orgID, serialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TraceRow{}
	for rows.Next() {
		var x TraceRow
		if err := rows.Scan(&x.At, &x.LocationID, &x.Location, &x.Qty, &x.Reason, &x.Ref, &x.Party); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// LotsAt lists lots in stock at a location for a product (for pickers).
func (r *Repository) LotsAt(ctx context.Context, orgID, productID, locationID string) ([]LotStock, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT l.id::text, l.lot_no, l.expiry, s.qty::float8 FROM adrient.inv_lot_stock s JOIN adrient.inv_lots l ON l.id=s.lot_id
		 WHERE s.org_id=$1 AND s.product_id=$2 AND ($3='' OR s.location_id::text=$3) AND s.qty>0 ORDER BY l.expiry NULLS LAST, l.lot_no`, orgID, productID, locationID)
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
