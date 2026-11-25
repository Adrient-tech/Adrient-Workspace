package crm

import (
	"context"
	"strings"
)

// PriceItem is one sellable product in the CRM price book.
type PriceItem struct {
	ID           string  `json:"id"`
	InvProductID string  `json:"inv_product_id"`
	Name         string  `json:"name"`
	SKU          string  `json:"sku"`
	Price        float64 `json:"price"`
	Currency     string  `json:"currency"`
	Active       bool    `json:"active"`
}

func (r *Repository) ListPriceBook(ctx context.Context, org string) ([]PriceItem, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, coalesce(inv_product_id::text,''), name, sku, price::float8, currency, active
		FROM adrient.crm_price_book WHERE org_id=$1 ORDER BY lower(name)`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PriceItem{}
	for rows.Next() {
		var p PriceItem
		if err := rows.Scan(&p.ID, &p.InvProductID, &p.Name, &p.SKU, &p.Price, &p.Currency, &p.Active); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) SavePriceItem(ctx context.Context, org, id string, p PriceItem) (PriceItem, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || p.Price < 0 {
		return p, invalid("a product needs a name and a non-negative price")
	}
	if p.Currency == "" {
		p.Currency = "INR"
	}
	p.Currency = strings.ToUpper(p.Currency)
	if p.InvProductID != "" {
		var ok bool
		if !uuidRe.MatchString(p.InvProductID) || r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.inv_products WHERE id=$1 AND org_id=$2)`, p.InvProductID, org).Scan(&ok) != nil || !ok {
			return p, invalid("inventory product not found")
		}
	}
	if id == "" {
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_price_book (org_id,inv_product_id,name,sku,price,currency,active) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`,
			org, nz(p.InvProductID), p.Name, p.SKU, p.Price, p.Currency, p.Active).Scan(&p.ID)
		return p, err
	}
	if err := r.del(ctx, `UPDATE adrient.crm_price_book SET inv_product_id=$3,name=$4,sku=$5,price=$6,currency=$7,active=$8 WHERE org_id=$1 AND id=$2`,
		org, id, nz(p.InvProductID), p.Name, p.SKU, p.Price, p.Currency, p.Active); err != nil {
		return p, err
	}
	p.ID = id
	return p, nil
}

func (r *Repository) DeletePriceItem(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_price_book WHERE org_id=$1 AND id=$2`, org, id)
}

// SyncInventoryProducts copies active Inventory products into the price book
// (matched by product id, so re-running updates name, SKU and price in place).
// It returns how many were added and how many updated.
func (r *Repository) SyncInventoryProducts(ctx context.Context, org string) (added, updated int, err error) {
	cur := "INR"
	if s, e := r.GetSettings(ctx, org); e == nil && s.Currency != "" {
		cur = s.Currency
	}
	rows, err := r.pool.Query(ctx, `WITH up AS (
		INSERT INTO adrient.crm_price_book (org_id,inv_product_id,name,sku,price,currency)
		SELECT p.org_id, p.id, CASE WHEN p.variant_label<>'' THEN p.name || ' - ' || p.variant_label ELSE p.name END, p.sku, p.price, $2
		FROM adrient.inv_products p WHERE p.org_id=$1 AND NOT p.archived
		ON CONFLICT (org_id, inv_product_id) WHERE inv_product_id IS NOT NULL
		DO UPDATE SET name=EXCLUDED.name, sku=EXCLUDED.sku, price=EXCLUDED.price
		RETURNING (xmax = 0) AS inserted) SELECT inserted FROM up`, org, cur)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var ins bool
		_ = rows.Scan(&ins)
		if ins {
			added++
		} else {
			updated++
		}
	}
	return added, updated, rows.Err()
}
