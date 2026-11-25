package inventory

// Kits (sold as a bundle, components leave stock) and simple bills of material
// with assembly orders (components are consumed, the finished product is produced).

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
)

// BOMLineInput is one component.
type BOMLineInput struct {
	ComponentID string  `json:"component_id"`
	Qty         float64 `json:"qty"`
}

// BOMInput creates or replaces the bill of material of a product.
type BOMInput struct {
	ProductID string         `json:"product_id"`
	Kind      string         `json:"kind"` // kit | assembly
	Qty       float64        `json:"qty"`  // finished units the lines make
	Note      string         `json:"note"`
	Lines     []BOMLineInput `json:"lines"`
}

// BOMLine is a component with live stock and cost.
type BOMLine struct {
	ComponentID string  `json:"component_id"`
	Name        string  `json:"name"`
	SKU         string  `json:"sku"`
	Unit        string  `json:"unit"`
	Qty         float64 `json:"qty"`
	Cost        float64 `json:"cost"`
	OnHand      float64 `json:"on_hand"`
}

// BOM is a bill of material.
type BOM struct {
	ID          string    `json:"id"`
	ProductID   string    `json:"product_id"`
	ProductName string    `json:"product_name"`
	SKU         string    `json:"sku"`
	Kind        string    `json:"kind"`
	Qty         float64   `json:"qty"`
	Note        string    `json:"note"`
	Lines       []BOMLine `json:"lines"`
	UnitCost    float64   `json:"unit_cost"` // component cost per finished unit (current product costs)
	Buildable   float64   `json:"buildable"` // finished units the current stock allows
}

// SaveBOM creates or replaces a bill of material.
func (r *Repository) SaveBOM(ctx context.Context, orgID string, in BOMInput) (BOM, error) {
	if in.Kind != "kit" && in.Kind != "assembly" {
		return BOM{}, invalid("kind must be kit or assembly")
	}
	if in.Qty <= 0 {
		in.Qty = 1
	}
	if len(in.Lines) == 0 {
		return BOM{}, invalid("add at least one component")
	}
	seen := map[string]bool{}
	for _, l := range in.Lines {
		if l.Qty <= 0 {
			return BOM{}, invalid("component quantities must be positive")
		}
		if l.ComponentID == in.ProductID {
			return BOM{}, invalid("a product cannot be its own component")
		}
		if seen[l.ComponentID] {
			return BOM{}, invalid("each component can only appear once")
		}
		seen[l.ComponentID] = true
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BOM{}, err
	}
	defer tx.Rollback(ctx)
	var one bool
	if err := tx.QueryRow(ctx, `SELECT true FROM adrient.inv_products WHERE org_id=$1 AND id=$2`, orgID, in.ProductID).Scan(&one); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BOM{}, invalid("product not found")
		}
		return BOM{}, err
	}
	// A component may not (directly or through other BOMs) contain this product.
	for _, l := range in.Lines {
		var cyc bool
		if err := tx.QueryRow(ctx,
			`WITH RECURSIVE down AS (
			   SELECT $3::uuid AS pid
			   UNION
			   SELECT bl.component_id FROM down d JOIN adrient.inv_boms b ON b.product_id=d.pid JOIN adrient.inv_bom_lines bl ON bl.bom_id=b.id WHERE b.org_id=$1)
			 SELECT EXISTS(SELECT 1 FROM down WHERE pid=$2)`, orgID, in.ProductID, l.ComponentID).Scan(&cyc); err != nil {
			return BOM{}, err
		}
		if cyc {
			return BOM{}, invalid("that would make the product part of itself")
		}
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT true FROM adrient.inv_products WHERE org_id=$1 AND id=$2`, orgID, l.ComponentID).Scan(&ok); err != nil {
			return BOM{}, invalid("component not found")
		}
	}
	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_boms (org_id, product_id, kind, qty, note) VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (org_id, product_id) DO UPDATE SET kind=EXCLUDED.kind, qty=EXCLUDED.qty, note=EXCLUDED.note RETURNING id::text`,
		orgID, in.ProductID, in.Kind, in.Qty, in.Note).Scan(&id)
	if err != nil {
		return BOM{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.inv_bom_lines WHERE bom_id=$1`, id); err != nil {
		return BOM{}, err
	}
	for _, l := range in.Lines {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.inv_bom_lines (bom_id, org_id, component_id, qty) VALUES ($1,$2,$3,$4)`, id, orgID, l.ComponentID, l.Qty); err != nil {
			return BOM{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return BOM{}, err
	}
	return r.GetBOM(ctx, orgID, id)
}

// ListBOMs lists every bill of material.
func (r *Repository) ListBOMs(ctx context.Context, orgID string) ([]BOM, error) {
	ids, err := r.bomIDs(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := []BOM{}
	for _, id := range ids {
		b, err := r.GetBOM(ctx, orgID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *Repository) bomIDs(ctx context.Context, orgID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT b.id::text FROM adrient.inv_boms b JOIN adrient.inv_products p ON p.id=b.product_id WHERE b.org_id=$1 ORDER BY p.name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetBOM loads one bill of material with component stock and cost.
func (r *Repository) GetBOM(ctx context.Context, orgID, id string) (BOM, error) {
	var b BOM
	err := r.pool.QueryRow(ctx,
		`SELECT b.id::text, b.product_id::text, p.name, p.sku, b.kind, b.qty::float8, b.note
		 FROM adrient.inv_boms b JOIN adrient.inv_products p ON p.id=b.product_id WHERE b.org_id=$1 AND b.id=$2`, orgID, id).
		Scan(&b.ID, &b.ProductID, &b.ProductName, &b.SKU, &b.Kind, &b.Qty, &b.Note)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, ErrNotFound
	}
	if err != nil {
		return b, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT c.id::text, c.name || CASE WHEN c.variant_label<>'' THEN ' / '||c.variant_label ELSE '' END, c.sku, c.unit, l.qty::float8, c.cost::float8,
		        COALESCE((SELECT sum(qty) FROM adrient.inv_stock s WHERE s.product_id=c.id),0)::float8
		 FROM adrient.inv_bom_lines l JOIN adrient.inv_products c ON c.id=l.component_id WHERE l.bom_id=$1 ORDER BY l.id`, id)
	if err != nil {
		return b, err
	}
	defer rows.Close()
	b.Lines = []BOMLine{}
	build := math.Inf(1)
	var cost float64
	for rows.Next() {
		var l BOMLine
		if err := rows.Scan(&l.ComponentID, &l.Name, &l.SKU, &l.Unit, &l.Qty, &l.Cost, &l.OnHand); err != nil {
			return b, err
		}
		b.Lines = append(b.Lines, l)
		cost += l.Qty * l.Cost
		if n := math.Floor(l.OnHand / l.Qty * b.Qty); n < build {
			build = n
		}
	}
	if math.IsInf(build, 1) || build < 0 {
		build = 0
	}
	b.Buildable = build
	b.UnitCost = round4(cost / b.Qty)
	return b, rows.Err()
}

// DeleteBOM removes a bill of material (a kit becomes an ordinary product again).
func (r *Repository) DeleteBOM(ctx context.Context, orgID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.inv_boms WHERE org_id=$1 AND id=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AssembleInput builds finished units from components.
type AssembleInput struct {
	BomID      string   `json:"bom_id"`
	Qty        float64  `json:"qty"`
	LocationID string   `json:"location_id"`
	LaborCost  float64  `json:"labor_cost"`
	LotNo      string   `json:"lot_no"`
	Expiry     string   `json:"expiry"`
	Serials    []string `json:"serials"`
	Note       string   `json:"note"`
}

// AssemblyOrder is a completed build.
type AssemblyOrder struct {
	ID          string    `json:"id"`
	Number      string    `json:"number"`
	ProductID   string    `json:"product_id"`
	ProductName string    `json:"product_name"`
	Qty         float64   `json:"qty"`
	LocationID  string    `json:"location_id"`
	LaborCost   float64   `json:"labor_cost"`
	UnitCost    float64   `json:"unit_cost"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"created_at"`
}

// Assemble consumes the components at their valued cost and receives the
// finished product at (components + labour) / quantity, in one transaction.
func (r *Repository) Assemble(ctx context.Context, orgID, userID string, in AssembleInput) (AssemblyOrder, error) {
	if in.Qty <= 0 {
		return AssemblyOrder{}, invalid("quantity must be positive")
	}
	if in.LaborCost < 0 {
		return AssemblyOrder{}, invalid("labour cost cannot be negative")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AssemblyOrder{}, err
	}
	defer tx.Rollback(ctx)
	var product, kind string
	var per float64
	if err := tx.QueryRow(ctx, `SELECT product_id::text, kind, qty::float8 FROM adrient.inv_boms WHERE org_id=$1 AND id=$2`, orgID, in.BomID).Scan(&product, &kind, &per); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AssemblyOrder{}, ErrNotFound
		}
		return AssemblyOrder{}, err
	}
	if kind != "assembly" {
		return AssemblyOrder{}, invalid("a kit is not built; it is sold as a bundle. Change it to an assembly to build it")
	}
	loc := in.LocationID
	if loc == "" {
		if loc, err = r.ensureDefaultLocation(ctx, tx, orgID); err != nil {
			return AssemblyOrder{}, err
		}
	}
	num, err := nextNumber(ctx, tx, orgID, "assembly", "ASM")
	if err != nil {
		return AssemblyOrder{}, err
	}
	rows, err := tx.Query(ctx, `SELECT component_id::text, qty::float8 FROM adrient.inv_bom_lines WHERE bom_id=$1 ORDER BY id`, in.BomID)
	if err != nil {
		return AssemblyOrder{}, err
	}
	var comps []kitPart
	for rows.Next() {
		var k kitPart
		if err := rows.Scan(&k.ProductID, &k.Qty); err != nil {
			rows.Close()
			return AssemblyOrder{}, err
		}
		comps = append(comps, k)
	}
	rows.Close()
	var cost float64
	for _, c := range comps {
		out := &AdjustOut{}
		need := round3(c.Qty * in.Qty / per)
		if _, err := r.applyAdjust(ctx, tx, orgID, userID, c.ProductID, AdjustInput{LocationID: loc, Qty: need, Reason: "remove", HistoryReason: "assembly_out",
			Ref: num, Note: "Used in " + num, Out: out}); err != nil {
			return AssemblyOrder{}, err
		}
		cost += out.Value
	}
	unit := round4((cost + in.LaborCost) / in.Qty)
	ai := AdjustInput{LocationID: loc, Qty: in.Qty, Reason: "receive", HistoryReason: "assembly_in", UnitCost: unit, Ref: num, Note: "Built " + num, LayerRef: num}
	if in.LotNo != "" {
		ai.Lots = []LotPick{{LotNo: in.LotNo, Expiry: in.Expiry, Qty: in.Qty}}
	}
	ai.Serials = in.Serials
	if _, err := r.applyAdjust(ctx, tx, orgID, userID, product, ai); err != nil {
		return AssemblyOrder{}, err
	}
	var o AssemblyOrder
	err = tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_assembly_orders (org_id, number, bom_id, product_id, qty, location_id, labor_cost, unit_cost, lot_no, expiry, note, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::date,$11,NULLIF($12,'')::uuid)
		 RETURNING id::text, number, product_id::text, qty::float8, location_id::text, labor_cost::float8, unit_cost::float8, note, created_at`,
		orgID, num, in.BomID, product, in.Qty, loc, in.LaborCost, unit, in.LotNo, in.Expiry, in.Note, userID).
		Scan(&o.ID, &o.Number, &o.ProductID, &o.Qty, &o.LocationID, &o.LaborCost, &o.UnitCost, &o.Note, &o.CreatedAt)
	if err != nil {
		return AssemblyOrder{}, err
	}
	return o, tx.Commit(ctx)
}

// ListAssemblies lists completed builds.
func (r *Repository) ListAssemblies(ctx context.Context, orgID string) ([]AssemblyOrder, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT a.id::text, a.number, a.product_id::text, p.name, a.qty::float8, a.location_id::text, a.labor_cost::float8, a.unit_cost::float8, a.note, a.created_at
		 FROM adrient.inv_assembly_orders a JOIN adrient.inv_products p ON p.id=a.product_id WHERE a.org_id=$1 ORDER BY a.created_at DESC LIMIT 200`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AssemblyOrder{}
	for rows.Next() {
		var o AssemblyOrder
		if err := rows.Scan(&o.ID, &o.Number, &o.ProductID, &o.ProductName, &o.Qty, &o.LocationID, &o.LaborCost, &o.UnitCost, &o.Note, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
