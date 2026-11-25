// Package inventory is the data-access + service layer for products,
// variants, stock per location, suppliers and the stock history.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned when no record matches within the org.
	ErrNotFound = errors.New("not found")
	// ErrInvalid wraps validation failures.
	ErrInvalid = errors.New("invalid")
	// ErrConflict is returned for duplicate SKU / barcode.
	ErrConflict = errors.New("conflict")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Reasons accepted by AdjustStock.
var validReasons = map[string]bool{
	"sale": true, "refund": true, "receive": true, "count": true,
	"remove": true, "correction": true,
}

type Location struct {
	ID        string
	Name      string
	Code      string
	IsDefault bool
	CreatedAt time.Time
}

type Supplier struct {
	ID           string
	Name         string
	ContactID    string
	Email        string
	Phone        string
	Notes        string
	ProductCount int32
	CreatedAt    time.Time
}

type ProductSupplier struct {
	SupplierID   string
	SupplierName string
	SupplierSKU  string
	Cost         float64
	LeadDays     int32
	PackSize     float64
	Preferred    bool
}

type StockLevel struct {
	LocationID   string
	LocationName string
	Qty          float64
	ShelfCode    string
}

type Product struct {
	ID           string
	ParentID     string
	VariantLabel string
	Name         string
	SKU          string
	Barcode      string
	Description  string
	Category     string
	Unit         string
	Price        float64
	Cost         float64
	MinStock     float64
	ReorderQty   float64
	PhotoFileID  string
	Archived     bool
	TotalStock   float64
	Stock        []StockLevel
	Variants     []Product
	Suppliers    []ProductSupplier
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ProductFields bundles editable attributes.
type ProductFields struct {
	ParentID          string
	VariantLabel      string
	Name              string
	SKU               string
	Barcode           string
	Description       string
	Category          string
	Unit              string
	Price             float64
	Cost              float64
	MinStock          float64
	ReorderQty        float64
	PhotoFileID       string
	OpeningQty        float64
	OpeningLocationID string
}

type ProductFilter struct {
	Query           string
	Barcode         string
	Category        string
	LocationID      string
	IncludeArchived bool
	ArchivedOnly    bool
	LowStockOnly    bool
	Flat            bool
	// Paging (flat lists only, see page.go): Limit rows after the keyset
	// (AfterName, AfterLabel, AfterID) in name order. Limit 0 = no paging.
	Limit      int
	AfterName  string
	AfterLabel string
	AfterID    string
}

type AdjustInput struct {
	LocationID string
	Qty        float64
	Absolute   bool
	Reason     string
	Note       string
	Ref        string
	ShelfCode  string
	UnitCost   float64
	// HistoryReason overrides Reason in the history row (used by transfers).
	HistoryReason string
	// LossReason tags a removal as damaged, expired, lost or internal (write-offs).
	LossReason string

	// Lots to receive into or take from (lot-tracked products). Taking without
	// naming lots picks first-expiry-first-out.
	Lots []LotPick
	// Serial numbers to receive or issue (serial-tracked products).
	Serials []string
	// Party is the customer/supplier shown in lot traceability.
	Party string
	// ReserveRef consumes active reservations carrying this reference.
	ReserveRef string
	// IgnoreReservations lets a removal dip into reserved stock.
	IgnoreReservations bool
	// NoValuation skips cost layers (stock that is not new value, e.g. a count correction handled elsewhere).
	NoValuation bool
	// LayerRef tags the cost layer a receipt creates; PreferLayerRef takes stock
	// out of the layers carrying that tag first (returns to the original receipt).
	LayerRef       string
	PreferLayerRef string
	// Out, when set, is filled with what the movement did.
	Out *AdjustOut
}

type HistoryEntry struct {
	ID           string
	ProductID    string
	ProductName  string
	SKU          string
	LocationID   string
	LocationName string
	Delta        float64
	QtyAfter     float64
	Reason       string
	Note         string
	Ref          string
	UserID       string
	CreatedAt    time.Time
}

type ReorderLine struct {
	Product      Product
	Stock        float64
	SuggestedQty float64
	SupplierID   string
	SupplierName string
	SupplierSKU  string
	UnitCost     float64
}

type ReportRow struct {
	Label       string
	Units       float64
	CostValue   float64
	RetailValue float64
}

type Report struct {
	ProductCount    int32
	Units           float64
	CostValue       float64
	RetailValue     float64
	LowStockCount   int32
	OutOfStockCount int32
	ByCategory      []ReportRow
	ByLocation      []ReportRow
	Movement        []ReportRow
	TopMovers       []ReportRow
}

// Repository reads and writes inventory data.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }

// ---------------------------------------------------------------- locations

// ensureDefaultLocation returns the default location id, creating "Main" on first use.
func (r *Repository) ensureDefaultLocation(ctx context.Context, tx pgx.Tx, orgID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx,
		`SELECT id::text FROM adrient.inv_locations WHERE org_id=$1
		 ORDER BY is_default DESC, created_at LIMIT 1`, orgID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_locations (org_id, name, code, is_default)
		 VALUES ($1,'Main','MAIN',true) RETURNING id::text`, orgID).Scan(&id)
	return id, err
}

func (r *Repository) ListLocations(ctx context.Context, orgID string) ([]Location, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := r.ensureDefaultLocation(ctx, tx, orgID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, name, code, is_default, created_at FROM adrient.inv_locations
		 WHERE org_id=$1 ORDER BY is_default DESC, name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Location{}
	for rows.Next() {
		var l Location
		if err := rows.Scan(&l.ID, &l.Name, &l.Code, &l.IsDefault, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Repository) SaveLocation(ctx context.Context, orgID, id string, l Location) (Location, error) {
	l.Name = strings.TrimSpace(l.Name)
	if l.Name == "" {
		return Location{}, invalid("location name required")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Location{}, err
	}
	defer tx.Rollback(ctx)
	if l.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE adrient.inv_locations SET is_default=false WHERE org_id=$1`, orgID); err != nil {
			return Location{}, err
		}
	}
	if id == "" {
		err = tx.QueryRow(ctx,
			`INSERT INTO adrient.inv_locations (org_id,name,code,is_default) VALUES ($1,$2,$3,$4)
			 RETURNING id::text, name, code, is_default, created_at`,
			orgID, l.Name, l.Code, l.IsDefault).Scan(&l.ID, &l.Name, &l.Code, &l.IsDefault, &l.CreatedAt)
	} else {
		err = tx.QueryRow(ctx,
			`UPDATE adrient.inv_locations SET name=$3, code=$4, is_default=$5 WHERE org_id=$1 AND id=$2
			 RETURNING id::text, name, code, is_default, created_at`,
			orgID, id, l.Name, l.Code, l.IsDefault).Scan(&l.ID, &l.Name, &l.Code, &l.IsDefault, &l.CreatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Location{}, ErrNotFound
	}
	if err != nil {
		return Location{}, err
	}
	return l, tx.Commit(ctx)
}

// DeleteLocation refuses to delete the last location or one that holds stock.
func (r *Repository) DeleteLocation(ctx context.Context, orgID, id string) error {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.inv_locations WHERE org_id=$1`, orgID).Scan(&n); err != nil {
		return err
	}
	if n <= 1 {
		return invalid("cannot delete the only location")
	}
	var held float64
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(sum(abs(qty)),0)::float8 FROM adrient.inv_stock WHERE org_id=$1 AND location_id=$2`,
		orgID, id).Scan(&held); err != nil {
		return err
	}
	if held != 0 {
		return invalid("location still holds stock; transfer it first")
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.inv_locations WHERE org_id=$1 AND id=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	// Keep one default.
	_, err = r.pool.Exec(ctx,
		`UPDATE adrient.inv_locations SET is_default=true WHERE id=(
		   SELECT id FROM adrient.inv_locations WHERE org_id=$1 ORDER BY is_default DESC, created_at LIMIT 1)
		 AND NOT EXISTS (SELECT 1 FROM adrient.inv_locations WHERE org_id=$1 AND is_default)`, orgID)
	return err
}

// ---------------------------------------------------------------- suppliers

func (r *Repository) ListSuppliers(ctx context.Context, orgID string) ([]Supplier, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT s.id::text, s.name, s.contact_id, s.email, s.phone, s.notes, s.created_at,
		        (SELECT count(*) FROM adrient.inv_product_suppliers ps WHERE ps.supplier_id=s.id)
		 FROM adrient.inv_suppliers s WHERE s.org_id=$1 ORDER BY lower(s.name)`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Supplier{}
	for rows.Next() {
		var s Supplier
		if err := rows.Scan(&s.ID, &s.Name, &s.ContactID, &s.Email, &s.Phone, &s.Notes, &s.CreatedAt, &s.ProductCount); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) SaveSupplier(ctx context.Context, orgID, id string, s Supplier) (Supplier, error) {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return Supplier{}, invalid("supplier name required")
	}
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx,
			`INSERT INTO adrient.inv_suppliers (org_id,name,contact_id,email,phone,notes)
			 VALUES ($1,$2,$3,$4,$5,$6)
			 RETURNING id::text, name, contact_id, email, phone, notes, created_at`,
			orgID, s.Name, s.ContactID, s.Email, s.Phone, s.Notes).
			Scan(&s.ID, &s.Name, &s.ContactID, &s.Email, &s.Phone, &s.Notes, &s.CreatedAt)
	} else {
		err = r.pool.QueryRow(ctx,
			`UPDATE adrient.inv_suppliers SET name=$3, contact_id=$4, email=$5, phone=$6, notes=$7
			 WHERE org_id=$1 AND id=$2
			 RETURNING id::text, name, contact_id, email, phone, notes, created_at`,
			orgID, id, s.Name, s.ContactID, s.Email, s.Phone, s.Notes).
			Scan(&s.ID, &s.Name, &s.ContactID, &s.Email, &s.Phone, &s.Notes, &s.CreatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Supplier{}, ErrNotFound
	}
	return s, err
}

func (r *Repository) DeleteSupplier(ctx context.Context, orgID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.inv_suppliers WHERE org_id=$1 AND id=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ----------------------------------------------------------------- products

const productCols = `p.id::text, COALESCE(p.parent_id::text,''), p.variant_label, p.name, p.sku, p.barcode,
  p.description, p.category, p.unit, p.price::float8, p.cost::float8, p.min_stock::float8,
  p.reorder_qty::float8, p.photo_file_id, p.archived, p.created_at, p.updated_at`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.ParentID, &p.VariantLabel, &p.Name, &p.SKU, &p.Barcode,
		&p.Description, &p.Category, &p.Unit, &p.Price, &p.Cost, &p.MinStock,
		&p.ReorderQty, &p.PhotoFileID, &p.Archived, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// hydrate fills Stock, TotalStock and Suppliers for the given products in place.
func (r *Repository) hydrate(ctx context.Context, orgID string, ps []Product, locationID string) error {
	if len(ps) == 0 {
		return nil
	}
	ids := make([]string, len(ps))
	idx := map[string]int{}
	for i, p := range ps {
		ids[i] = p.ID
		idx[p.ID] = i
	}
	rows, err := r.pool.Query(ctx,
		`SELECT s.product_id::text, s.location_id::text, l.name, s.qty::float8, s.shelf_code
		 FROM adrient.inv_stock s JOIN adrient.inv_locations l ON l.id=s.location_id
		 WHERE s.org_id=$1 AND s.product_id = ANY($2::uuid[]) AND ($3='' OR s.location_id::text=$3)
		 ORDER BY l.is_default DESC, l.name`, orgID, ids, locationID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var pid string
		var sl StockLevel
		if err := rows.Scan(&pid, &sl.LocationID, &sl.LocationName, &sl.Qty, &sl.ShelfCode); err != nil {
			rows.Close()
			return err
		}
		i := idx[pid]
		ps[i].Stock = append(ps[i].Stock, sl)
		ps[i].TotalStock = round3(ps[i].TotalStock + sl.Qty)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	srows, err := r.pool.Query(ctx,
		`SELECT ps.product_id::text, ps.supplier_id::text, s.name, ps.supplier_sku, ps.cost::float8,
		        ps.lead_days, ps.pack_size::float8, ps.preferred
		 FROM adrient.inv_product_suppliers ps JOIN adrient.inv_suppliers s ON s.id=ps.supplier_id
		 WHERE ps.org_id=$1 AND ps.product_id = ANY($2::uuid[])
		 ORDER BY ps.preferred DESC, ps.cost`, orgID, ids)
	if err != nil {
		return err
	}
	defer srows.Close()
	for srows.Next() {
		var pid string
		var s ProductSupplier
		if err := srows.Scan(&pid, &s.SupplierID, &s.SupplierName, &s.SupplierSKU, &s.Cost, &s.LeadDays, &s.PackSize, &s.Preferred); err != nil {
			return err
		}
		i := idx[pid]
		ps[i].Suppliers = append(ps[i].Suppliers, s)
	}
	return srows.Err()
}

// ListProducts returns products. Unless Flat, variants nest under their family
// and the family's TotalStock includes its variants.
func (r *Repository) ListProducts(ctx context.Context, orgID string, f ProductFilter) ([]Product, error) {
	where := []string{"p.org_id=$1"}
	args := []any{orgID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	switch {
	case f.ArchivedOnly:
		where = append(where, "p.archived")
	case !f.IncludeArchived:
		where = append(where, "NOT p.archived")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		add(`(p.name ILIKE ? OR p.sku ILIKE ? OR p.barcode ILIKE ? OR p.category ILIKE ?
		      OR p.variant_label ILIKE ?
		      OR EXISTS (SELECT 1 FROM adrient.inv_products v WHERE v.parent_id=p.id AND (v.sku ILIKE ? OR v.barcode ILIKE ? OR v.variant_label ILIKE ?)))`,
			"%"+escapeLike(q)+"%")
	}
	if f.Barcode != "" {
		add(`(p.barcode=? OR lower(p.sku)=lower(?))`, f.Barcode)
	}
	if f.Category != "" {
		add(`p.category=?`, f.Category)
	}
	limitSQL := ""
	if f.Limit > 0 {
		// Nested paging pages families: a family is listed when it or one of
		// its variants matches (the query clause above checks variants too).
		if !f.Flat {
			where = append(where, "p.parent_id IS NULL")
		}
		if f.AfterID != "" {
			args = append(args, f.AfterName, f.AfterLabel, f.AfterID)
			where = append(where, fmt.Sprintf("(lower(p.name), p.variant_label, p.id) > ($%d, $%d, $%d::uuid)", len(args)-2, len(args)-1, len(args)))
		}
		limitSQL = fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	sql := `SELECT ` + productCols + ` FROM adrient.inv_products p WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY lower(p.name), p.variant_label, p.id` + limitSQL
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	var all []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// A variant matched by the query may have its family outside the result; pull
	// the families in so the nested view is complete.
	if !f.Flat {
		have := map[string]bool{}
		for _, p := range all {
			have[p.ID] = true
		}
		var missing []string
		for _, p := range all {
			if p.ParentID != "" && !have[p.ParentID] {
				missing = append(missing, p.ParentID)
				have[p.ParentID] = true
			}
		}
		if len(missing) > 0 {
			mrows, err := r.pool.Query(ctx, `SELECT `+productCols+` FROM adrient.inv_products p WHERE p.org_id=$1 AND p.id=ANY($2::uuid[])`, orgID, missing)
			if err != nil {
				return nil, err
			}
			for mrows.Next() {
				p, err := scanProduct(mrows)
				if err != nil {
					mrows.Close()
					return nil, err
				}
				all = append(all, p)
			}
			mrows.Close()
		}
		// Also every variant of every family in the list.
		var famIDs []string
		for _, p := range all {
			if p.ParentID == "" {
				famIDs = append(famIDs, p.ID)
			}
		}
		if len(famIDs) > 0 {
			vrows, err := r.pool.Query(ctx, `SELECT `+productCols+` FROM adrient.inv_products p
				WHERE p.org_id=$1 AND p.parent_id=ANY($2::uuid[]) AND ($3 OR NOT p.archived)`, orgID, famIDs, f.IncludeArchived || f.ArchivedOnly)
			if err != nil {
				return nil, err
			}
			known := map[string]bool{}
			for _, p := range all {
				known[p.ID] = true
			}
			for vrows.Next() {
				p, err := scanProduct(vrows)
				if err != nil {
					vrows.Close()
					return nil, err
				}
				if !known[p.ID] {
					all = append(all, p)
				}
			}
			vrows.Close()
		}
	}
	if err := r.hydrate(ctx, orgID, all, f.LocationID); err != nil {
		return nil, err
	}
	if f.Flat {
		out := all[:0]
		for _, p := range all {
			if f.LowStockOnly && !(p.MinStock > 0 && p.TotalStock <= p.MinStock) {
				continue
			}
			out = append(out, p)
		}
		return out, nil
	}
	return nestVariants(all, f.LowStockOnly), nil
}

// nestVariants groups variants under families, totals the family stock and
// optionally keeps only families (or variants) that are low.
func nestVariants(all []Product, lowOnly bool) []Product {
	fams := []Product{}
	pos := map[string]int{}
	for _, p := range all {
		if p.ParentID == "" {
			pos[p.ID] = len(fams)
			fams = append(fams, p)
		}
	}
	for _, p := range all {
		if p.ParentID == "" {
			continue
		}
		if i, ok := pos[p.ParentID]; ok {
			fams[i].Variants = append(fams[i].Variants, p)
			fams[i].TotalStock = round3(fams[i].TotalStock + p.TotalStock)
		}
	}
	if !lowOnly {
		return fams
	}
	out := []Product{}
	for _, p := range fams {
		low := p.MinStock > 0 && p.TotalStock <= p.MinStock
		for _, v := range p.Variants {
			if v.MinStock > 0 && v.TotalStock <= v.MinStock {
				low = true
			}
		}
		if low {
			out = append(out, p)
		}
	}
	return out
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}

func (r *Repository) GetProduct(ctx context.Context, orgID, id string) (Product, error) {
	p, err := scanProduct(r.pool.QueryRow(ctx,
		`SELECT `+productCols+` FROM adrient.inv_products p WHERE p.org_id=$1 AND p.id=$2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, err
	}
	list := []Product{p}
	if p.ParentID == "" {
		rows, err := r.pool.Query(ctx, `SELECT `+productCols+` FROM adrient.inv_products p
			WHERE p.org_id=$1 AND p.parent_id=$2 ORDER BY p.variant_label`, orgID, id)
		if err != nil {
			return Product{}, err
		}
		for rows.Next() {
			v, err := scanProduct(rows)
			if err != nil {
				rows.Close()
				return Product{}, err
			}
			list = append(list, v)
		}
		rows.Close()
	}
	if err := r.hydrate(ctx, orgID, list, ""); err != nil {
		return Product{}, err
	}
	p = list[0]
	p.Variants = list[1:]
	for _, v := range p.Variants {
		p.TotalStock = round3(p.TotalStock + v.TotalStock)
	}
	return p, nil
}

func normalize(f *ProductFields) error {
	f.Name = strings.TrimSpace(f.Name)
	f.SKU = strings.TrimSpace(f.SKU)
	f.Barcode = strings.TrimSpace(f.Barcode)
	if f.Name == "" {
		return invalid("product name required")
	}
	if strings.TrimSpace(f.Unit) == "" {
		f.Unit = "pcs"
	}
	if f.Price < 0 || f.Cost < 0 || f.MinStock < 0 || f.ReorderQty < 0 {
		return invalid("price, cost and minimums cannot be negative")
	}
	return nil
}

func conflictErr(err error) error {
	if err != nil && strings.Contains(err.Error(), "23505") {
		if strings.Contains(err.Error(), "barcode") {
			return fmt.Errorf("%w: barcode already used by another product", ErrConflict)
		}
		return fmt.Errorf("%w: SKU already used by another product", ErrConflict)
	}
	return err
}

func (r *Repository) CreateProduct(ctx context.Context, orgID, userID string, f ProductFields) (Product, error) {
	if err := normalize(&f); err != nil {
		return Product{}, err
	}
	if f.OpeningQty < 0 {
		return Product{}, invalid("opening quantity cannot be negative")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Product{}, err
	}
	defer tx.Rollback(ctx)
	id, err := r.insertProduct(ctx, tx, orgID, userID, f)
	if err != nil {
		return Product{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Product{}, err
	}
	return r.GetProduct(ctx, orgID, id)
}

func (r *Repository) insertProduct(ctx context.Context, tx pgx.Tx, orgID, userID string, f ProductFields) (string, error) {
	if f.ParentID != "" {
		var pp *string
		if err := tx.QueryRow(ctx, `SELECT parent_id::text FROM adrient.inv_products WHERE org_id=$1 AND id=$2`, orgID, f.ParentID).Scan(&pp); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", invalid("family product not found")
			}
			return "", err
		}
		if pp != nil {
			return "", invalid("a variant cannot have variants")
		}
	}
	var id string
	err := tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_products (org_id,parent_id,variant_label,name,sku,barcode,description,category,unit,price,cost,min_stock,reorder_qty,photo_file_id)
		 VALUES ($1,NULLIF($2,'')::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id::text`,
		orgID, f.ParentID, f.VariantLabel, f.Name, f.SKU, f.Barcode, f.Description, f.Category, f.Unit,
		f.Price, f.Cost, f.MinStock, f.ReorderQty, f.PhotoFileID).Scan(&id)
	if err != nil {
		return "", conflictErr(err)
	}
	if f.OpeningQty > 0 {
		loc := f.OpeningLocationID
		if loc == "" {
			if loc, err = r.ensureDefaultLocation(ctx, tx, orgID); err != nil {
				return "", err
			}
		}
		if _, err := r.applyAdjust(ctx, tx, orgID, userID, id, AdjustInput{
			LocationID: loc, Qty: f.OpeningQty, Reason: "receive", Note: "Opening stock",
		}); err != nil {
			return "", err
		}
	}
	return id, nil
}

func (r *Repository) UpdateProduct(ctx context.Context, orgID, id string, f ProductFields) (Product, error) {
	if err := normalize(&f); err != nil {
		return Product{}, err
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE adrient.inv_products SET variant_label=$3, name=$4, sku=$5, barcode=$6, description=$7,
		   category=$8, unit=$9, price=$10, cost=$11, min_stock=$12, reorder_qty=$13, photo_file_id=$14, updated_at=now()
		 WHERE org_id=$1 AND id=$2`,
		orgID, id, f.VariantLabel, f.Name, f.SKU, f.Barcode, f.Description, f.Category, f.Unit,
		f.Price, f.Cost, f.MinStock, f.ReorderQty, f.PhotoFileID)
	if err != nil {
		return Product{}, conflictErr(err)
	}
	if tag.RowsAffected() == 0 {
		return Product{}, ErrNotFound
	}
	return r.GetProduct(ctx, orgID, id)
}

// ArchiveProduct archives/restores a product; archiving a family also archives its variants.
func (r *Repository) ArchiveProduct(ctx context.Context, orgID, id string, archived bool) (Product, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE adrient.inv_products SET archived=$3, updated_at=now()
		 WHERE org_id=$1 AND (id=$2 OR parent_id=$2)`, orgID, id, archived)
	if err != nil {
		return Product{}, err
	}
	if tag.RowsAffected() == 0 {
		return Product{}, ErrNotFound
	}
	return r.GetProduct(ctx, orgID, id)
}

func (r *Repository) DeleteProduct(ctx context.Context, orgID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.inv_products WHERE org_id=$1 AND id=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetProductSuppliers replaces the supplier sources of a product.
func (r *Repository) SetProductSuppliers(ctx context.Context, orgID, id string, list []ProductSupplier) (Product, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Product{}, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT true FROM adrient.inv_products WHERE org_id=$1 AND id=$2`, orgID, id).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Product{}, ErrNotFound
		}
		return Product{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.inv_product_suppliers WHERE org_id=$1 AND product_id=$2`, orgID, id); err != nil {
		return Product{}, err
	}
	seen := map[string]bool{}
	hasPreferred := false
	for _, s := range list {
		if s.SupplierID == "" || seen[s.SupplierID] {
			continue
		}
		seen[s.SupplierID] = true
		if s.PackSize <= 0 {
			s.PackSize = 1
		}
		pref := s.Preferred && !hasPreferred
		hasPreferred = hasPreferred || pref
		tag, err := tx.Exec(ctx,
			`INSERT INTO adrient.inv_product_suppliers (product_id,supplier_id,org_id,supplier_sku,cost,lead_days,pack_size,preferred)
			 SELECT $1, s.id, $3, $4, $5, $6, $7, $8 FROM adrient.inv_suppliers s WHERE s.id=$2 AND s.org_id=$3`,
			id, s.SupplierID, orgID, s.SupplierSKU, s.Cost, s.LeadDays, s.PackSize, pref)
		if err != nil {
			return Product{}, err
		}
		if tag.RowsAffected() == 0 {
			return Product{}, invalid("supplier %s not found", s.SupplierID)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Product{}, err
	}
	return r.GetProduct(ctx, orgID, id)
}

// -------------------------------------------------------------------- stock

func signedDelta(a AdjustInput) float64 {
	switch a.Reason {
	case "sale", "remove":
		return -math.Abs(a.Qty)
	case "refund", "receive":
		return math.Abs(a.Qty)
	}
	return a.Qty
}

// applyAdjust moves stock for one product/location inside tx and writes history.
func (r *Repository) applyAdjust(ctx context.Context, tx pgx.Tx, orgID, userID, productID string, a AdjustInput) (float64, error) {
	if !validReasons[a.Reason] {
		return 0, invalid("reason must be one of sale, refund, receive, count, remove, correction")
	}
	if a.Absolute && a.Reason != "count" {
		return 0, invalid("absolute quantities are only allowed for count")
	}
	if a.Qty < 0 && a.Absolute {
		return 0, invalid("counted quantity cannot be negative")
	}
	var one bool
	if err := tx.QueryRow(ctx, `SELECT true FROM adrient.inv_products WHERE org_id=$1 AND id=$2`, orgID, productID).Scan(&one); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if a.Reason == "sale" || a.Reason == "refund" || a.Reason == "remove" {
		if parts, per, err := kitParts(ctx, tx, orgID, productID); err != nil {
			return 0, err
		} else if parts != nil {
			return r.explodeKit(ctx, tx, orgID, userID, productID, a, parts, per)
		}
	}
	loc := a.LocationID
	var err error
	if loc == "" {
		if loc, err = r.ensureDefaultLocation(ctx, tx, orgID); err != nil {
			return 0, err
		}
	} else {
		if err := tx.QueryRow(ctx, `SELECT true FROM adrient.inv_locations WHERE org_id=$1 AND id=$2`, orgID, loc).Scan(&one); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, invalid("location not found")
			}
			return 0, err
		}
	}
	// Lock (or create) the stock row.
	if _, err := tx.Exec(ctx,
		`INSERT INTO adrient.inv_stock (product_id, location_id, org_id, qty) VALUES ($1,$2,$3,0)
		 ON CONFLICT (product_id, location_id) DO NOTHING`, productID, loc, orgID); err != nil {
		return 0, err
	}
	var cur float64
	if err := tx.QueryRow(ctx,
		`SELECT qty::float8 FROM adrient.inv_stock WHERE product_id=$1 AND location_id=$2 FOR UPDATE`,
		productID, loc).Scan(&cur); err != nil {
		return 0, err
	}
	var delta, next float64
	if a.Absolute {
		next = round3(a.Qty)
		delta = round3(next - cur)
	} else {
		delta = round3(signedDelta(a))
		next = round3(cur + delta)
	}
	if next < 0 && a.Reason != "sale" && a.Reason != "refund" {
		return 0, invalid("not enough stock (have %v)", cur)
	}
	if delta < 0 && a.Reason == "remove" && !a.IgnoreReservations {
		var total float64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(qty),0)::float8 FROM adrient.inv_stock WHERE product_id=$1`, productID).Scan(&total); err != nil {
			return 0, err
		}
		reserved, err := reservedElsewhere(ctx, tx, orgID, productID, a.ReserveRef)
		if err != nil {
			return 0, err
		}
		if round3(total-cur+next) < round3(reserved) {
			return 0, invalid("%v unit(s) are reserved for orders; only %v can be taken out", round3(reserved), round3(math.Max(0, total-reserved)))
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE adrient.inv_stock SET qty=$3, shelf_code=CASE WHEN $4<>'' THEN $4 ELSE shelf_code END
		 WHERE product_id=$1 AND location_id=$2`, productID, loc, next, a.ShelfCode); err != nil {
		return 0, err
	}
	if a.Reason == "receive" && a.UnitCost > 0 {
		if _, err := tx.Exec(ctx, `UPDATE adrient.inv_products SET cost=$3, updated_at=now() WHERE org_id=$1 AND id=$2`,
			orgID, productID, a.UnitCost); err != nil {
			return 0, err
		}
	}
	histReason := a.Reason
	if a.HistoryReason != "" {
		histReason = a.HistoryReason
	}
	var histID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_history (org_id, product_id, location_id, delta, qty_after, reason, note, ref, user_id, loss_reason)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid,$10) RETURNING id::text`,
		orgID, productID, loc, delta, next, histReason, a.Note, a.Ref, userID, a.LossReason).Scan(&histID); err != nil {
		return 0, err
	}
	if delta != 0 {
		if err := r.afterMove(ctx, tx, orgID, userID, productID, loc, delta, next, a, histReason, histID); err != nil {
			return 0, err
		}
	}
	return next, nil
}

// AdjustInTx moves stock inside the caller's transaction so another module
// (POS) can commit a sale and its stock movement atomically.
func (r *Repository) AdjustInTx(ctx context.Context, tx pgx.Tx, orgID, userID, productID string, a AdjustInput) (float64, error) {
	return r.applyAdjust(ctx, tx, orgID, userID, productID, a)
}

// ProductBasics returns the name, SKU and price of a product inside tx.
func (r *Repository) ProductBasics(ctx context.Context, tx pgx.Tx, orgID, productID string) (name, sku string, price float64, err error) {
	var label string
	err = tx.QueryRow(ctx,
		`SELECT p.name, p.sku, p.price::float8, p.variant_label FROM adrient.inv_products p
		 WHERE p.org_id=$1 AND p.id=$2`, orgID, productID).Scan(&name, &sku, &price, &label)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", 0, ErrNotFound
	}
	if label != "" {
		name += " (" + label + ")"
	}
	return name, sku, price, err
}

// AdjustStock changes stock at one location and records it in the history.
func (r *Repository) AdjustStock(ctx context.Context, orgID, userID, productID string, a AdjustInput) (Product, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Product{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := r.applyAdjust(ctx, tx, orgID, userID, productID, a); err != nil {
		return Product{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Product{}, err
	}
	return r.GetProduct(ctx, orgID, productID)
}

// TransferStock moves qty between two locations (two history rows, reason "transfer").
func (r *Repository) TransferStock(ctx context.Context, orgID, userID, productID, from, to string, qty float64, note string) (Product, error) {
	if qty <= 0 {
		return Product{}, invalid("transfer quantity must be positive")
	}
	if from == "" || to == "" || from == to {
		return Product{}, invalid("pick two different locations")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Product{}, err
	}
	defer tx.Rollback(ctx)
	out := &AdjustOut{}
	if _, err := r.applyAdjust(ctx, tx, orgID, userID, productID, AdjustInput{LocationID: from, Qty: qty, Reason: "remove", HistoryReason: "transfer", Note: "Transfer out. " + note, Out: out}); err != nil {
		return Product{}, err
	}
	if _, err := r.applyAdjust(ctx, tx, orgID, userID, productID, AdjustInput{LocationID: to, Qty: qty, Reason: "receive", HistoryReason: "transfer", Note: "Transfer in. " + note, Lots: out.Picks, Serials: out.Serials}); err != nil {
		return Product{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Product{}, err
	}
	return r.GetProduct(ctx, orgID, productID)
}

func (r *Repository) ListHistory(ctx context.Context, orgID, productID, reason string, since *time.Time, limit int) ([]HistoryEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx,
		`SELECT h.id::text, h.product_id::text, p.name || CASE WHEN p.variant_label<>'' THEN ' / '||p.variant_label ELSE '' END,
		        p.sku, COALESCE(h.location_id::text,''), COALESCE(l.name,''), h.delta::float8, h.qty_after::float8,
		        h.reason, h.note, h.ref, COALESCE(h.user_id::text,''), h.created_at
		 FROM adrient.inv_history h
		 JOIN adrient.inv_products p ON p.id=h.product_id
		 LEFT JOIN adrient.inv_locations l ON l.id=h.location_id
		 WHERE h.org_id=$1 AND ($2='' OR h.product_id::text=$2 OR p.parent_id::text=$2) AND ($3='' OR h.reason=$3)
		   AND ($4::timestamptz IS NULL OR h.created_at >= $4)
		 ORDER BY h.created_at DESC, h.id LIMIT $5`, orgID, productID, reason, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.ProductID, &e.ProductName, &e.SKU, &e.LocationID, &e.LocationName,
			&e.Delta, &e.QtyAfter, &e.Reason, &e.Note, &e.Ref, &e.UserID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------- reorder / report

// stockable returns the products that hold their own stock (families that
// have variants are totals only and are skipped).
func (r *Repository) stockable(ctx context.Context, orgID string) ([]Product, error) {
	prods, err := r.ListProducts(ctx, orgID, ProductFilter{Flat: true})
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT parent_id::text FROM adrient.inv_products WHERE org_id=$1 AND parent_id IS NOT NULL AND NOT archived`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fam := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		fam[id] = true
	}
	out := prods[:0]
	for _, p := range prods {
		if !fam[p.ID] {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

// ReorderList returns every stockable product at or below its minimum with a
// suggested order quantity and the preferred (else cheapest) supplier.
func (r *Repository) ReorderList(ctx context.Context, orgID string) ([]ReorderLine, error) {
	prods, err := r.stockable(ctx, orgID)
	if err != nil {
		return nil, err
	}
	lines := []ReorderLine{}
	for _, p := range prods {
		if p.MinStock <= 0 || p.TotalStock > p.MinStock {
			continue
		}
		q := p.ReorderQty
		if q <= 0 {
			q = p.MinStock*2 - p.TotalStock
		}
		if q <= 0 {
			q = 1
		}
		ln := ReorderLine{Product: p, Stock: p.TotalStock, SuggestedQty: round3(q), UnitCost: p.Cost}
		if len(p.Suppliers) > 0 { // hydrate orders preferred first, then cheapest
			s := p.Suppliers[0]
			ln.SupplierID, ln.SupplierName, ln.SupplierSKU = s.SupplierID, s.SupplierName, s.SupplierSKU
			if s.Cost > 0 {
				ln.UnitCost = s.Cost
			}
			if s.PackSize > 1 {
				ln.SuggestedQty = math.Ceil(ln.SuggestedQty/s.PackSize) * s.PackSize
			}
		}
		lines = append(lines, ln)
	}
	return lines, nil
}

func (r *Repository) Report(ctx context.Context, orgID string) (Report, error) {
	var rep Report
	prods, err := r.stockable(ctx, orgID)
	if err != nil {
		return rep, err
	}
	cat := map[string]*ReportRow{}
	loc := map[string]*ReportRow{}
	for _, p := range prods {
		rep.ProductCount++
		rep.Units += p.TotalStock
		rep.CostValue += p.TotalStock * p.Cost
		rep.RetailValue += p.TotalStock * p.Price
		if p.TotalStock <= 0 {
			rep.OutOfStockCount++
		}
		if p.MinStock > 0 && p.TotalStock <= p.MinStock {
			rep.LowStockCount++
		}
		c := p.Category
		if c == "" {
			c = "Uncategorised"
		}
		if cat[c] == nil {
			cat[c] = &ReportRow{Label: c}
		}
		cat[c].Units += p.TotalStock
		cat[c].CostValue += p.TotalStock * p.Cost
		cat[c].RetailValue += p.TotalStock * p.Price
		for _, s := range p.Stock {
			if loc[s.LocationName] == nil {
				loc[s.LocationName] = &ReportRow{Label: s.LocationName}
			}
			loc[s.LocationName].Units += s.Qty
			loc[s.LocationName].CostValue += s.Qty * p.Cost
			loc[s.LocationName].RetailValue += s.Qty * p.Price
		}
	}
	rep.ByCategory = sortedRows(cat)
	rep.ByLocation = sortedRows(loc)
	mrows, err := r.pool.Query(ctx,
		`SELECT reason, sum(delta)::float8 FROM adrient.inv_history
		 WHERE org_id=$1 AND created_at >= now() - interval '30 days' GROUP BY reason ORDER BY reason`, orgID)
	if err != nil {
		return rep, err
	}
	for mrows.Next() {
		var row ReportRow
		if err := mrows.Scan(&row.Label, &row.Units); err != nil {
			mrows.Close()
			return rep, err
		}
		rep.Movement = append(rep.Movement, row)
	}
	mrows.Close()
	trows, err := r.pool.Query(ctx,
		`SELECT p.name || CASE WHEN p.variant_label<>'' THEN ' / '||p.variant_label ELSE '' END, sum(-h.delta)::float8
		 FROM adrient.inv_history h JOIN adrient.inv_products p ON p.id=h.product_id
		 WHERE h.org_id=$1 AND h.reason='sale' AND h.created_at >= now() - interval '30 days'
		 GROUP BY 1 HAVING sum(-h.delta) > 0 ORDER BY 2 DESC LIMIT 10`, orgID)
	if err != nil {
		return rep, err
	}
	defer trows.Close()
	for trows.Next() {
		var row ReportRow
		if err := trows.Scan(&row.Label, &row.Units); err != nil {
			return rep, err
		}
		rep.TopMovers = append(rep.TopMovers, row)
	}
	rep.Units, rep.CostValue, rep.RetailValue = round3(rep.Units), math.Round(rep.CostValue*100)/100, math.Round(rep.RetailValue*100)/100
	return rep, trows.Err()
}

func sortedRows(m map[string]*ReportRow) []ReportRow {
	out := make([]ReportRow, 0, len(m))
	for _, v := range m {
		v.Units = round3(v.Units)
		v.CostValue = math.Round(v.CostValue*100) / 100
		v.RetailValue = math.Round(v.RetailValue*100) / 100
		out = append(out, *v)
	}
	// insertion sort by cost value desc then label
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].CostValue > out[j-1].CostValue ||
			(out[j].CostValue == out[j-1].CostValue && out[j].Label < out[j-1].Label)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ------------------------------------------------------------------- import

type ImportResult struct {
	Created, Updated, Skipped int32
	Errors                    []string
}

// ImportProducts upserts products by SKU. mode: update | add | skip.
// Each row may carry OpeningQty placed at the named location (created on demand).
func (r *Repository) ImportProducts(ctx context.Context, orgID, userID, mode string, rows []ProductFields, locNames []string) (ImportResult, error) {
	var res ImportResult
	if mode == "" {
		mode = "update"
	}
	if mode != "update" && mode != "add" && mode != "skip" {
		return res, invalid("mode must be update, add or skip")
	}
	for i, f := range rows {
		line := i + 1
		if err := normalize(&f); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("row %d: %v", line, err))
			continue
		}
		if err := r.importOne(ctx, orgID, userID, mode, f, locName(locNames, i), &res); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("row %d (%s): %v", line, f.Name, err))
		}
	}
	return res, nil
}

func locName(names []string, i int) string {
	if i < len(names) {
		return strings.TrimSpace(names[i])
	}
	return ""
}

func (r *Repository) importOne(ctx context.Context, orgID, userID, mode string, f ProductFields, locationName string, res *ImportResult) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	loc := ""
	if locationName != "" {
		err := tx.QueryRow(ctx, `SELECT id::text FROM adrient.inv_locations WHERE org_id=$1 AND lower(name)=lower($2)`, orgID, locationName).Scan(&loc)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `INSERT INTO adrient.inv_locations (org_id,name) VALUES ($1,$2) RETURNING id::text`, orgID, locationName).Scan(&loc)
		}
		if err != nil {
			return err
		}
	}
	f.OpeningLocationID = loc
	existing := ""
	if f.SKU != "" {
		err := tx.QueryRow(ctx, `SELECT id::text FROM adrient.inv_products WHERE org_id=$1 AND lower(sku)=lower($2)`, orgID, f.SKU).Scan(&existing)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	switch {
	case existing == "":
		if _, err := r.insertProduct(ctx, tx, orgID, userID, f); err != nil {
			return err
		}
		res.Created++
	case mode == "skip":
		res.Skipped++
		return nil
	default:
		if _, err := tx.Exec(ctx,
			`UPDATE adrient.inv_products SET name=$3, barcode=$4, description=$5, category=$6, unit=$7,
			   price=$8, cost=$9, min_stock=$10, reorder_qty=$11, updated_at=now() WHERE org_id=$1 AND id=$2`,
			orgID, existing, f.Name, f.Barcode, f.Description, f.Category, f.Unit, f.Price, f.Cost, f.MinStock, f.ReorderQty); err != nil {
			return conflictErr(err)
		}
		if f.OpeningQty > 0 {
			a := AdjustInput{LocationID: loc, Qty: f.OpeningQty, Reason: "receive", Note: "CSV import (added)"}
			if mode == "update" {
				a = AdjustInput{LocationID: loc, Qty: f.OpeningQty, Absolute: true, Reason: "count", Note: "CSV import (set)"}
			}
			if _, err := r.applyAdjust(ctx, tx, orgID, userID, existing, a); err != nil {
				return err
			}
		}
		res.Updated++
	}
	return tx.Commit(ctx)
}
