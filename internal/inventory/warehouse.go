package inventory

// Warehouses and locations hierarchy, transfers with in-transit state,
// reservations and availability (on hand vs reserved vs incoming).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func nz[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// nextNumber returns the next document number such as PO-0007.
func nextNumber(ctx context.Context, tx pgx.Tx, orgID, kind, prefix string) (string, error) {
	var n int64
	err := tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_counters (org_id, kind, last_no) VALUES ($1,$2,1)
		 ON CONFLICT (org_id, kind) DO UPDATE SET last_no = adrient.inv_counters.last_no + 1 RETURNING last_no`,
		orgID, kind).Scan(&n)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%04d", prefix, n), nil
}

// ---------------------------------------------------------------- locations

// Loc is a node in the warehouse tree: warehouse > zone > shelf > bin.
type Loc struct {
	ID           string  `json:"id"`
	ParentID     string  `json:"parent_id"`
	Name         string  `json:"name"`
	Code         string  `json:"code"`
	Kind         string  `json:"kind"`
	Address      string  `json:"address"`
	IsDefault    bool    `json:"is_default"`
	Active       bool    `json:"active"`
	Path         string  `json:"path"`
	Depth        int     `json:"depth"`
	WarehouseID  string  `json:"warehouse_id"`
	Units        float64 `json:"units"`         // stock held directly here
	SubtreeUnits float64 `json:"subtree_units"` // here and everything below
	Items        int     `json:"items"`         // distinct products held directly here
}

var locKinds = map[string]bool{"warehouse": true, "zone": true, "shelf": true, "bin": true}

// ListLocationTree returns every location with its path and stock totals,
// parents before children.
func (r *Repository) ListLocationTree(ctx context.Context, orgID string) ([]Loc, error) {
	if _, err := r.ListLocations(ctx, orgID); err != nil { // makes sure a default exists
		return nil, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT l.id::text, COALESCE(l.parent_id::text,''), l.name, l.code, l.kind, l.address, l.is_default, l.active,
		        COALESCE(s.units,0)::float8, COALESCE(s.items,0)
		 FROM adrient.inv_locations l
		 LEFT JOIN (SELECT location_id, sum(qty) units, count(*) FILTER (WHERE qty<>0) items FROM adrient.inv_stock WHERE org_id=$1 GROUP BY location_id) s
		   ON s.location_id=l.id
		 WHERE l.org_id=$1 ORDER BY l.is_default DESC, l.name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all := map[string]*Loc{}
	var order []string
	for rows.Next() {
		var l Loc
		if err := rows.Scan(&l.ID, &l.ParentID, &l.Name, &l.Code, &l.Kind, &l.Address, &l.IsDefault, &l.Active, &l.Units, &l.Items); err != nil {
			return nil, err
		}
		all[l.ID] = &l
		order = append(order, l.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	kids := map[string][]string{}
	var roots []string
	for _, id := range order {
		l := all[id]
		if l.ParentID != "" && all[l.ParentID] != nil {
			kids[l.ParentID] = append(kids[l.ParentID], id)
		} else {
			roots = append(roots, id)
		}
	}
	out := []Loc{}
	var walk func(id, path, wh string, depth int) float64
	walk = func(id, path, wh string, depth int) float64 {
		l := all[id]
		if path == "" {
			l.Path = l.Name
		} else {
			l.Path = path + " / " + l.Name
		}
		if l.Kind == "warehouse" || wh == "" {
			wh = l.ID
		}
		l.WarehouseID = wh
		l.Depth = depth
		idx := len(out)
		out = append(out, *l)
		sub := l.Units
		for _, k := range kids[id] {
			sub += walk(k, l.Path, wh, depth+1)
		}
		out[idx].SubtreeUnits = round3(sub)
		return sub
	}
	for _, id := range roots {
		walk(id, "", "", 0)
	}
	return out, nil
}

// SaveLocationNode creates or updates a location with parent and kind.
func (r *Repository) SaveLocationNode(ctx context.Context, orgID, id string, in Loc) (Loc, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return Loc{}, invalid("name required")
	}
	if in.Kind == "" {
		in.Kind = "warehouse"
		if in.ParentID != "" {
			in.Kind = "bin"
		}
	}
	if !locKinds[in.Kind] {
		return Loc{}, invalid("kind must be warehouse, zone, shelf or bin")
	}
	if in.Kind == "warehouse" && in.ParentID != "" {
		return Loc{}, invalid("a warehouse cannot sit inside another location")
	}
	if in.Kind != "warehouse" && in.ParentID == "" {
		return Loc{}, invalid("a %s must be placed inside a warehouse or zone", in.Kind)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Loc{}, err
	}
	defer tx.Rollback(ctx)
	if in.ParentID != "" {
		if in.ParentID == id {
			return Loc{}, invalid("a location cannot be its own parent")
		}
		var pk string
		if err := tx.QueryRow(ctx, `SELECT kind FROM adrient.inv_locations WHERE org_id=$1 AND id=$2`, orgID, in.ParentID).Scan(&pk); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Loc{}, invalid("parent not found")
			}
			return Loc{}, err
		}
		if id != "" { // no cycles: the parent must not be below this node
			var cyc bool
			if err := tx.QueryRow(ctx,
				`WITH RECURSIVE sub AS (SELECT id FROM adrient.inv_locations WHERE org_id=$1 AND id=$2
				   UNION ALL SELECT c.id FROM adrient.inv_locations c JOIN sub ON c.parent_id=sub.id)
				 SELECT EXISTS(SELECT 1 FROM sub WHERE id=$3)`, orgID, id, in.ParentID).Scan(&cyc); err != nil {
				return Loc{}, err
			}
			if cyc {
				return Loc{}, invalid("a location cannot be moved below itself")
			}
		}
	}
	if in.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE adrient.inv_locations SET is_default=false WHERE org_id=$1`, orgID); err != nil {
			return Loc{}, err
		}
	}
	if id == "" {
		err = tx.QueryRow(ctx,
			`INSERT INTO adrient.inv_locations (org_id,name,code,is_default,parent_id,kind,address,active)
			 VALUES ($1,$2,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8) RETURNING id::text`,
			orgID, in.Name, strings.TrimSpace(in.Code), in.IsDefault, in.ParentID, in.Kind, in.Address, true).Scan(&id)
	} else {
		var n int
		err = tx.QueryRow(ctx,
			`UPDATE adrient.inv_locations SET name=$3, code=$4, is_default=$5, parent_id=NULLIF($6,'')::uuid, kind=$7, address=$8, active=$9
			 WHERE org_id=$1 AND id=$2 RETURNING 1`,
			orgID, id, in.Name, strings.TrimSpace(in.Code), in.IsDefault, in.ParentID, in.Kind, in.Address, in.Active).Scan(&n)
		if errors.Is(err, pgx.ErrNoRows) {
			return Loc{}, ErrNotFound
		}
	}
	if err != nil {
		return Loc{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Loc{}, err
	}
	tree, err := r.ListLocationTree(ctx, orgID)
	if err != nil {
		return Loc{}, err
	}
	for _, l := range tree {
		if l.ID == id {
			return l, nil
		}
	}
	return Loc{}, ErrNotFound
}

// DeleteLocationNode deletes a leaf location that holds no stock.
func (r *Repository) DeleteLocationNode(ctx context.Context, orgID, id string) error {
	var kids int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.inv_locations WHERE org_id=$1 AND parent_id=$2`, orgID, id).Scan(&kids); err != nil {
		return err
	}
	if kids > 0 {
		return invalid("move or delete the locations inside it first")
	}
	return r.DeleteLocation(ctx, orgID, id)
}

// SubtreeIDs returns a location and everything below it.
func (r *Repository) SubtreeIDs(ctx context.Context, orgID, id string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`WITH RECURSIVE sub AS (SELECT id FROM adrient.inv_locations WHERE org_id=$1 AND id=$2
		   UNION ALL SELECT c.id FROM adrient.inv_locations c JOIN sub ON c.parent_id=sub.id)
		 SELECT id::text FROM sub`, orgID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetShelfCode sets the shelf/bin label of a product at a location.
func (r *Repository) SetShelfCode(ctx context.Context, orgID, productID, locationID, code string) error {
	code = strings.TrimSpace(code)
	if len(code) > 40 {
		return invalid("shelf code is too long")
	}
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO adrient.inv_stock (product_id, location_id, org_id, qty, shelf_code)
		 SELECT p.id, l.id, $1, 0, $4 FROM adrient.inv_products p, adrient.inv_locations l
		 WHERE p.org_id=$1 AND p.id=$2 AND l.org_id=$1 AND l.id=$3
		 ON CONFLICT (product_id, location_id) DO UPDATE SET shelf_code=EXCLUDED.shelf_code`,
		orgID, productID, locationID, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------- availability

// Avail is stock position of one product.
type Avail struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	SKU       string  `json:"sku"`
	Unit      string  `json:"unit"`
	Category  string  `json:"category"`
	Tracking  string  `json:"tracking"`
	IsKit     bool    `json:"is_kit"`
	OnHand    float64 `json:"on_hand"`
	Reserved  float64 `json:"reserved"`
	Available float64 `json:"available"`
	Incoming  float64 `json:"incoming"`   // open purchase orders
	InTransit float64 `json:"in_transit"` // shipped transfers not yet received
	Forecast  float64 `json:"forecast"`   // on hand + incoming + in transit - reserved
	MinStock  float64 `json:"min_stock"`
}

// ListAvailability gives on hand / reserved / available / incoming per product.
// A kit's on-hand is how many complete kits its components allow.
func (r *Repository) ListAvailability(ctx context.Context, orgID, query string, locationID string) ([]Avail, error) {
	var locs []string
	if locationID != "" {
		ids, err := r.SubtreeIDs(ctx, orgID, locationID)
		if err != nil {
			return nil, err
		}
		locs = ids
	}
	rows, err := r.pool.Query(ctx,
		`SELECT p.id::text, p.name || CASE WHEN p.variant_label<>'' THEN ' / '||p.variant_label ELSE '' END, p.sku, p.unit, p.category, p.tracking,
		        EXISTS(SELECT 1 FROM adrient.inv_boms b WHERE b.product_id=p.id AND b.kind='kit'),
		        COALESCE((SELECT sum(qty) FROM adrient.inv_stock s WHERE s.product_id=p.id AND ($3::uuid[] IS NULL OR s.location_id = ANY($3))),0)::float8,
		        COALESCE((SELECT sum(qty) FROM adrient.inv_reservations v WHERE v.product_id=p.id AND v.state='active'
		                  AND (v.expires_at IS NULL OR v.expires_at > now()) AND ($3::uuid[] IS NULL OR v.location_id IS NULL OR v.location_id = ANY($3))),0)::float8,
		        COALESCE((SELECT sum(l.qty - l.qty_received) FROM adrient.inv_purchase_order_lines l JOIN adrient.inv_purchase_orders o ON o.id=l.po_id
		                  WHERE l.product_id=p.id AND o.state IN ('approved','sent','acknowledged','partial') AND l.qty > l.qty_received
		                  AND ($3::uuid[] IS NULL OR o.ship_to_loc_id = ANY($3))),0)::float8,
		        COALESCE((SELECT sum(l.qty_shipped - l.qty_received) FROM adrient.inv_transfer_lines l JOIN adrient.inv_transfers t ON t.id=l.transfer_id
		                  WHERE l.product_id=p.id AND t.state='in_transit' AND ($3::uuid[] IS NULL OR t.to_loc_id = ANY($3))),0)::float8,
		        p.min_stock::float8
		 FROM adrient.inv_products p
		 WHERE p.org_id=$1 AND NOT p.archived
		   AND NOT EXISTS (SELECT 1 FROM adrient.inv_products c WHERE c.parent_id=p.id AND NOT c.archived)
		   AND ($2='' OR p.name ILIKE '%'||$2||'%' OR p.sku ILIKE '%'||$2||'%' OR p.barcode=$2)
		 ORDER BY p.name LIMIT 2000`, orgID, query, locs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Avail{}
	for rows.Next() {
		var a Avail
		if err := rows.Scan(&a.ProductID, &a.Name, &a.SKU, &a.Unit, &a.Category, &a.Tracking, &a.IsKit, &a.OnHand, &a.Reserved, &a.Incoming, &a.InTransit, &a.MinStock); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].IsKit {
			if n, err := r.kitCapacity(ctx, orgID, out[i].ProductID, locs); err == nil {
				out[i].OnHand = n
			}
		}
		out[i].OnHand = round3(out[i].OnHand)
		out[i].Available = round3(out[i].OnHand - out[i].Reserved)
		out[i].Forecast = round3(out[i].OnHand + out[i].Incoming + out[i].InTransit - out[i].Reserved)
	}
	return out, nil
}

// kitCapacity is the number of complete kits the components can supply.
func (r *Repository) kitCapacity(ctx context.Context, orgID, productID string, locs []string) (float64, error) {
	var n *float64
	err := r.pool.QueryRow(ctx,
		`SELECT min(floor(
		    (COALESCE((SELECT sum(qty) FROM adrient.inv_stock s WHERE s.product_id=bl.component_id AND ($3::uuid[] IS NULL OR s.location_id = ANY($3))),0)
		     - COALESCE((SELECT sum(qty) FROM adrient.inv_reservations v WHERE v.product_id=bl.component_id AND v.state='active'),0))
		    / bl.qty * b.qty))::float8
		 FROM adrient.inv_boms b JOIN adrient.inv_bom_lines bl ON bl.bom_id=b.id
		 WHERE b.org_id=$1 AND b.product_id=$2 AND b.kind='kit'`, orgID, productID, locs).Scan(&n)
	if err != nil || n == nil || *n < 0 {
		return 0, err
	}
	return *n, nil
}

// LocAvail is stock of a product at one location.
type LocAvail struct {
	LocationID string  `json:"location_id"`
	Path       string  `json:"path"`
	Qty        float64 `json:"qty"`
	Reserved   float64 `json:"reserved"`
	ShelfCode  string  `json:"shelf_code"`
}

// ProductLocations lists stock of a product by location (with paths).
func (r *Repository) ProductLocations(ctx context.Context, orgID, productID string) ([]LocAvail, error) {
	tree, err := r.ListLocationTree(ctx, orgID)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT s.location_id::text, s.qty::float8, s.shelf_code,
		        COALESCE((SELECT sum(v.qty) FROM adrient.inv_reservations v WHERE v.product_id=s.product_id AND v.location_id=s.location_id AND v.state='active'),0)::float8
		 FROM adrient.inv_stock s WHERE s.org_id=$1 AND s.product_id=$2`, orgID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	have := map[string]LocAvail{}
	for rows.Next() {
		var a LocAvail
		if err := rows.Scan(&a.LocationID, &a.Qty, &a.ShelfCode, &a.Reserved); err != nil {
			return nil, err
		}
		have[a.LocationID] = a
	}
	out := []LocAvail{}
	for _, l := range tree {
		a := have[l.ID]
		a.LocationID, a.Path = l.ID, l.Path
		out = append(out, a)
	}
	return out, rows.Err()
}

// ------------------------------------------------------------- reservations

// Reservation holds stock for an order.
type Reservation struct {
	ID          string     `json:"id"`
	ProductID   string     `json:"product_id"`
	ProductName string     `json:"product_name"`
	SKU         string     `json:"sku"`
	LocationID  string     `json:"location_id"`
	Qty         float64    `json:"qty"`
	RefType     string     `json:"ref_type"`
	Ref         string     `json:"ref"`
	State       string     `json:"state"`
	Note        string     `json:"note"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// ReserveInput asks to hold qty of a product.
type ReserveInput struct {
	ProductID  string  `json:"product_id"`
	LocationID string  `json:"location_id"`
	Qty        float64 `json:"qty"`
	RefType    string  `json:"ref_type"`
	Ref        string  `json:"ref"`
	Note       string  `json:"note"`
	ExpiresAt  string  `json:"expires_at"`
	// Backorder lets the reservation exceed what is available (it then waits for incoming stock).
	Backorder bool `json:"backorder"`
}

// Reserve holds stock for a sales order (or any document). Kits reserve their components.
func (r *Repository) Reserve(ctx context.Context, orgID, userID string, in ReserveInput) ([]Reservation, error) {
	in.Ref = strings.TrimSpace(in.Ref)
	if in.Ref == "" {
		return nil, invalid("a reference (for example the sales order number) is required")
	}
	if in.Qty <= 0 {
		return nil, invalid("quantity must be positive")
	}
	if in.RefType == "" {
		in.RefType = "sales_order"
	}
	var exp *time.Time
	if in.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, in.ExpiresAt)
		if err != nil {
			d, err2 := time.Parse("2006-01-02", in.ExpiresAt)
			if err2 != nil {
				return nil, invalid("expires_at must be a date or RFC3339 time")
			}
			t = d
		}
		exp = &t
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	targets := []kitPart{{ProductID: in.ProductID, Qty: in.Qty}}
	parts, per, err := kitParts(ctx, tx, orgID, in.ProductID)
	if err != nil {
		return nil, err
	}
	if parts != nil {
		targets = nil
		for _, k := range parts {
			targets = append(targets, kitPart{ProductID: k.ProductID, Qty: round3(in.Qty * k.Qty / per)})
		}
	}
	var out []Reservation
	for _, t := range targets {
		// Serialise reservations per product so two orders cannot both take the last unit.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "inv-res:"+t.ProductID); err != nil {
			return nil, err
		}
		var name string
		if err := tx.QueryRow(ctx, `SELECT name FROM adrient.inv_products WHERE org_id=$1 AND id=$2`, orgID, t.ProductID).Scan(&name); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if in.LocationID != "" {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT true FROM adrient.inv_locations WHERE org_id=$1 AND id=$2`, orgID, in.LocationID).Scan(&ok); err != nil {
				return nil, invalid("location not found")
			}
		}
		if !in.Backorder {
			var onHand, reserved float64
			if err := tx.QueryRow(ctx,
				`SELECT COALESCE(sum(qty) FILTER (WHERE $3='' OR location_id::text=$3),0)::float8 FROM adrient.inv_stock WHERE product_id=$1 AND org_id=$2`,
				t.ProductID, orgID, in.LocationID).Scan(&onHand); err != nil {
				return nil, err
			}
			if err := tx.QueryRow(ctx,
				`SELECT COALESCE(sum(qty),0)::float8 FROM adrient.inv_reservations
				 WHERE product_id=$1 AND org_id=$2 AND state='active' AND (expires_at IS NULL OR expires_at > now())
				   AND ($3='' OR location_id::text=$3)`, t.ProductID, orgID, in.LocationID).Scan(&reserved); err != nil {
				return nil, err
			}
			if round3(onHand-reserved) < t.Qty {
				return nil, invalid("only %v of %s available (on hand %v, reserved %v)", round3(onHand-reserved), name, round3(onHand), round3(reserved))
			}
		}
		var res Reservation
		err := tx.QueryRow(ctx,
			`INSERT INTO adrient.inv_reservations (org_id, product_id, location_id, qty, ref_type, ref, note, created_by, expires_at)
			 VALUES ($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9)
			 RETURNING id::text, product_id::text, COALESCE(location_id::text,''), qty::float8, ref_type, ref, state, note, created_at, expires_at`,
			orgID, t.ProductID, in.LocationID, t.Qty, in.RefType, in.Ref, in.Note, userID, exp).
			Scan(&res.ID, &res.ProductID, &res.LocationID, &res.Qty, &res.RefType, &res.Ref, &res.State, &res.Note, &res.CreatedAt, &res.ExpiresAt)
		if err != nil {
			return nil, err
		}
		res.ProductName = name
		out = append(out, res)
	}
	return out, tx.Commit(ctx)
}

// ReleaseReservations frees all active reservations for a reference.
func (r *Repository) ReleaseReservations(ctx context.Context, orgID, ref string) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE adrient.inv_reservations SET state='released', closed_at=now() WHERE org_id=$1 AND ref=$2 AND state='active'`, orgID, ref)
	return tag.RowsAffected(), err
}

// ListReservations returns reservations, newest first.
func (r *Repository) ListReservations(ctx context.Context, orgID, state, ref, productID string) ([]Reservation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT v.id::text, v.product_id::text, p.name, p.sku, COALESCE(v.location_id::text,''), v.qty::float8, v.ref_type, v.ref, v.state, v.note, v.created_at, v.expires_at
		 FROM adrient.inv_reservations v JOIN adrient.inv_products p ON p.id=v.product_id
		 WHERE v.org_id=$1 AND ($2='' OR v.state=$2) AND ($3='' OR v.ref=$3) AND ($4='' OR v.product_id::text=$4)
		 ORDER BY v.created_at DESC LIMIT 500`, orgID, state, ref, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reservation{}
	for rows.Next() {
		var v Reservation
		if err := rows.Scan(&v.ID, &v.ProductID, &v.ProductName, &v.SKU, &v.LocationID, &v.Qty, &v.RefType, &v.Ref, &v.State, &v.Note, &v.CreatedAt, &v.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- transfers

// TransferLine is one product on a stock transfer.
type TransferLine struct {
	ID          string    `json:"id"`
	ProductID   string    `json:"product_id"`
	ProductName string    `json:"product_name"`
	SKU         string    `json:"sku"`
	Qty         float64   `json:"qty"`
	QtyShipped  float64   `json:"qty_shipped"`
	QtyReceived float64   `json:"qty_received"`
	Lots        []LotPick `json:"lots"`
	Serials     []string  `json:"serials"`
}

// Transfer moves stock between locations through an in-transit state.
type Transfer struct {
	ID         string         `json:"id"`
	Number     string         `json:"number"`
	FromID     string         `json:"from_loc_id"`
	FromName   string         `json:"from_name"`
	ToID       string         `json:"to_loc_id"`
	ToName     string         `json:"to_name"`
	State      string         `json:"state"`
	Note       string         `json:"note"`
	CreatedAt  time.Time      `json:"created_at"`
	ShippedAt  *time.Time     `json:"shipped_at"`
	ReceivedAt *time.Time     `json:"received_at"`
	Lines      []TransferLine `json:"lines"`
}

func (r *Repository) loadTransferLines(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, orgID string, ids []string) (map[string][]TransferLine, error) {
	rows, err := q.Query(ctx,
		`SELECT l.transfer_id::text, l.id::text, l.product_id::text, p.name || CASE WHEN p.variant_label<>'' THEN ' / '||p.variant_label ELSE '' END, p.sku,
		        l.qty::float8, l.qty_shipped::float8, l.qty_received::float8, l.lots, l.serials
		 FROM adrient.inv_transfer_lines l JOIN adrient.inv_products p ON p.id=l.product_id
		 WHERE l.org_id=$1 AND l.transfer_id = ANY($2::uuid[]) ORDER BY l.id`, orgID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]TransferLine{}
	for rows.Next() {
		var tid string
		var l TransferLine
		if err := rows.Scan(&tid, &l.ID, &l.ProductID, &l.ProductName, &l.SKU, &l.Qty, &l.QtyShipped, &l.QtyReceived, &l.Lots, &l.Serials); err != nil {
			return nil, err
		}
		out[tid] = append(out[tid], l)
	}
	return out, rows.Err()
}

// ListTransfers lists transfers, optionally by state.
func (r *Repository) ListTransfers(ctx context.Context, orgID, state string) ([]Transfer, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT t.id::text, t.number, t.from_loc_id::text, f.name, t.to_loc_id::text, d.name, t.state, t.note, t.created_at, t.shipped_at, t.received_at
		 FROM adrient.inv_transfers t JOIN adrient.inv_locations f ON f.id=t.from_loc_id JOIN adrient.inv_locations d ON d.id=t.to_loc_id
		 WHERE t.org_id=$1 AND ($2='' OR t.state=$2) ORDER BY t.created_at DESC LIMIT 300`, orgID, state)
	if err != nil {
		return nil, err
	}
	out := []Transfer{}
	var ids []string
	for rows.Next() {
		var t Transfer
		if err := rows.Scan(&t.ID, &t.Number, &t.FromID, &t.FromName, &t.ToID, &t.ToName, &t.State, &t.Note, &t.CreatedAt, &t.ShippedAt, &t.ReceivedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, t)
		ids = append(ids, t.ID)
	}
	rows.Close()
	lines, err := r.loadTransferLines(ctx, r.pool, orgID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Lines = lines[out[i].ID]
		if out[i].Lines == nil {
			out[i].Lines = []TransferLine{}
		}
	}
	return out, nil
}

// GetTransfer returns one transfer.
func (r *Repository) GetTransfer(ctx context.Context, orgID, id string) (Transfer, error) {
	var t Transfer
	err := r.pool.QueryRow(ctx,
		`SELECT t.id::text, t.number, t.from_loc_id::text, f.name, t.to_loc_id::text, d.name, t.state, t.note, t.created_at, t.shipped_at, t.received_at
		 FROM adrient.inv_transfers t JOIN adrient.inv_locations f ON f.id=t.from_loc_id JOIN adrient.inv_locations d ON d.id=t.to_loc_id
		 WHERE t.org_id=$1 AND t.id=$2`, orgID, id).
		Scan(&t.ID, &t.Number, &t.FromID, &t.FromName, &t.ToID, &t.ToName, &t.State, &t.Note, &t.CreatedAt, &t.ShippedAt, &t.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	lines, err := r.loadTransferLines(ctx, r.pool, orgID, []string{id})
	if err != nil {
		return t, err
	}
	t.Lines = lines[id]
	if t.Lines == nil {
		t.Lines = []TransferLine{}
	}
	return t, nil
}

// TransferInput creates a draft transfer.
type TransferInput struct {
	FromID string `json:"from_loc_id"`
	ToID   string `json:"to_loc_id"`
	Note   string `json:"note"`
	Lines  []struct {
		ProductID string  `json:"product_id"`
		Qty       float64 `json:"qty"`
	} `json:"lines"`
	// Ship moves the stock out immediately (in transit).
	Ship bool `json:"ship"`
}

// CreateTransfer saves a draft transfer (and ships it when asked).
func (r *Repository) CreateTransfer(ctx context.Context, orgID, userID string, in TransferInput) (Transfer, error) {
	if in.FromID == "" || in.ToID == "" || in.FromID == in.ToID {
		return Transfer{}, invalid("pick two different locations")
	}
	if len(in.Lines) == 0 {
		return Transfer{}, invalid("add at least one product")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Transfer{}, err
	}
	defer tx.Rollback(ctx)
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.inv_locations WHERE org_id=$1 AND id IN ($2,$3)`, orgID, in.FromID, in.ToID).Scan(&n); err != nil {
		return Transfer{}, err
	}
	if n != 2 {
		return Transfer{}, invalid("location not found")
	}
	num, err := nextNumber(ctx, tx, orgID, "transfer", "TR")
	if err != nil {
		return Transfer{}, err
	}
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_transfers (org_id, number, from_loc_id, to_loc_id, note, created_by) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid) RETURNING id::text`,
		orgID, num, in.FromID, in.ToID, in.Note, userID).Scan(&id); err != nil {
		return Transfer{}, err
	}
	seen := map[string]bool{}
	for _, l := range in.Lines {
		if l.Qty <= 0 {
			return Transfer{}, invalid("quantities must be positive")
		}
		if seen[l.ProductID] {
			return Transfer{}, invalid("a product can only appear once per transfer")
		}
		seen[l.ProductID] = true
		if _, err := tx.Exec(ctx,
			`INSERT INTO adrient.inv_transfer_lines (transfer_id, org_id, product_id, qty)
			 SELECT $1,$2,p.id,$4 FROM adrient.inv_products p WHERE p.org_id=$2 AND p.id=$3`, id, orgID, l.ProductID, round3(l.Qty)); err != nil {
			return Transfer{}, err
		}
	}
	var cnt int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.inv_transfer_lines WHERE transfer_id=$1`, id).Scan(&cnt); err != nil {
		return Transfer{}, err
	}
	if cnt != len(in.Lines) {
		return Transfer{}, invalid("product not found")
	}
	if in.Ship {
		if err := r.shipTx(ctx, tx, orgID, userID, id); err != nil {
			return Transfer{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, err
	}
	return r.GetTransfer(ctx, orgID, id)
}

// ShipTransfer takes the stock out of the source location and puts it in transit.
func (r *Repository) ShipTransfer(ctx context.Context, orgID, userID, id string) (Transfer, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Transfer{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.shipTx(ctx, tx, orgID, userID, id); err != nil {
		return Transfer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, err
	}
	return r.GetTransfer(ctx, orgID, id)
}

func (r *Repository) shipTx(ctx context.Context, tx pgx.Tx, orgID, userID, id string) error {
	var state, from, number string
	if err := tx.QueryRow(ctx, `SELECT state, from_loc_id::text, number FROM adrient.inv_transfers WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&state, &from, &number); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if state != "draft" {
		return invalid("only a draft transfer can be shipped")
	}
	rows, err := tx.Query(ctx, `SELECT id::text, product_id::text, qty::float8 FROM adrient.inv_transfer_lines WHERE transfer_id=$1 ORDER BY id`, id)
	if err != nil {
		return err
	}
	type ln struct {
		id, product string
		qty         float64
	}
	var lines []ln
	for rows.Next() {
		var l ln
		if err := rows.Scan(&l.id, &l.product, &l.qty); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, l)
	}
	rows.Close()
	for _, l := range lines {
		out := &AdjustOut{}
		if _, err := r.applyAdjust(ctx, tx, orgID, userID, l.product, AdjustInput{LocationID: from, Qty: l.qty, Reason: "remove", HistoryReason: "transfer_out",
			Ref: number, Note: "Shipped on " + number, Out: out}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.inv_transfer_lines SET qty_shipped=qty, lots=$2, serials=$3 WHERE id=$1`,
			l.id, nz(out.Picks), nz(out.Serials)); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE adrient.inv_transfers SET state='in_transit', shipped_at=now() WHERE id=$1`, id)
	return err
}

// ReceiveLine states how much arrived for a transfer line.
type ReceiveLine struct {
	LineID string  `json:"line_id"`
	Qty    float64 `json:"qty"`
}

// ReceiveTransfer books received quantities at the destination. With no lines
// it receives everything still in transit. Partial receipts keep the transfer open.
func (r *Repository) ReceiveTransfer(ctx context.Context, orgID, userID, id string, recv []ReceiveLine) (Transfer, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Transfer{}, err
	}
	defer tx.Rollback(ctx)
	var state, to, number string
	if err := tx.QueryRow(ctx, `SELECT state, to_loc_id::text, number FROM adrient.inv_transfers WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&state, &to, &number); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Transfer{}, ErrNotFound
		}
		return Transfer{}, err
	}
	if state != "in_transit" {
		return Transfer{}, invalid("only a transfer in transit can be received")
	}
	lines, err := r.loadTransferLines(ctx, tx, orgID, []string{id})
	if err != nil {
		return Transfer{}, err
	}
	want := map[string]float64{}
	if len(recv) == 0 {
		for _, l := range lines[id] {
			want[l.ID] = round3(l.QtyShipped - l.QtyReceived)
		}
	} else {
		for _, x := range recv {
			want[x.LineID] += x.Qty
		}
	}
	for _, l := range lines[id] {
		q, ok := want[l.ID]
		if !ok || q <= 0 {
			continue
		}
		open := round3(l.QtyShipped - l.QtyReceived)
		if q > open+0.0005 {
			return Transfer{}, invalid("%s: only %v left in transit", l.ProductName, open)
		}
		// Hand the shipped lots/serials over in the order they left.
		lots, serials := takeFromQueue(l.Lots, l.Serials, l.QtyReceived, q)
		if _, err := r.applyAdjust(ctx, tx, orgID, userID, l.ProductID, AdjustInput{LocationID: to, Qty: q, Reason: "receive", HistoryReason: "transfer_in",
			Ref: number, Note: "Received on " + number, Lots: lots, Serials: serials}); err != nil {
			return Transfer{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.inv_transfer_lines SET qty_received = qty_received + $2 WHERE id=$1`, l.ID, q); err != nil {
			return Transfer{}, err
		}
	}
	var open float64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(qty_shipped - qty_received),0)::float8 FROM adrient.inv_transfer_lines WHERE transfer_id=$1`, id).Scan(&open); err != nil {
		return Transfer{}, err
	}
	if open <= 0.0005 {
		if _, err := tx.Exec(ctx, `UPDATE adrient.inv_transfers SET state='received', received_at=now() WHERE id=$1`, id); err != nil {
			return Transfer{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, err
	}
	return r.GetTransfer(ctx, orgID, id)
}

// takeFromQueue picks the next qty units (after skipping `skip`) of the lot and
// serial lists in shipping order.
func takeFromQueue(lots []LotPick, serials []string, skip, qty float64) ([]LotPick, []string) {
	var outL []LotPick
	if len(lots) > 0 {
		s, need := skip, qty
		for _, l := range lots {
			avail := l.Qty
			if s >= avail {
				s -= avail
				continue
			}
			avail -= s
			s = 0
			take := avail
			if take > need {
				take = need
			}
			if take > 0 {
				x := l
				x.Qty = round3(take)
				outL = append(outL, x)
				need = round3(need - take)
			}
			if need <= 0 {
				break
			}
		}
	}
	var outS []string
	if len(serials) > 0 {
		a, b := int(skip+0.5), int(skip+qty+0.5)
		if a > len(serials) {
			a = len(serials)
		}
		if b > len(serials) {
			b = len(serials)
		}
		outS = serials[a:b]
	}
	return outL, outS
}

// CancelTransfer cancels a draft, or returns the goods still in transit to the source.
func (r *Repository) CancelTransfer(ctx context.Context, orgID, userID, id string) (Transfer, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Transfer{}, err
	}
	defer tx.Rollback(ctx)
	var state, from, number string
	if err := tx.QueryRow(ctx, `SELECT state, from_loc_id::text, number FROM adrient.inv_transfers WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&state, &from, &number); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Transfer{}, ErrNotFound
		}
		return Transfer{}, err
	}
	switch state {
	case "draft":
	case "in_transit":
		lines, err := r.loadTransferLines(ctx, tx, orgID, []string{id})
		if err != nil {
			return Transfer{}, err
		}
		for _, l := range lines[id] {
			open := round3(l.QtyShipped - l.QtyReceived)
			if open <= 0 {
				continue
			}
			lots, serials := takeFromQueue(l.Lots, l.Serials, l.QtyReceived, open)
			if _, err := r.applyAdjust(ctx, tx, orgID, userID, l.ProductID, AdjustInput{LocationID: from, Qty: open, Reason: "receive", HistoryReason: "transfer_in",
				Ref: number, Note: "Returned to source: " + number + " cancelled", Lots: lots, Serials: serials}); err != nil {
				return Transfer{}, err
			}
		}
	default:
		return Transfer{}, invalid("a %s transfer cannot be cancelled", state)
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.inv_transfers SET state='cancelled' WHERE id=$1`, id); err != nil {
		return Transfer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, err
	}
	return r.GetTransfer(ctx, orgID, id)
}
