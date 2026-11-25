package invoicing

import (
	"context"
	"log/slog"
)

// StockReserver holds and frees stock for a sales order. Inventory implements
// it; nil means reservations are off and orders still work.
type StockReserver interface {
	ReserveForOrder(ctx context.Context, orgID, userID, ref, productID string, qty float64) error
	ReleaseOrder(ctx context.Context, orgID, ref string) error
}

// SetStockReserver wires Inventory reservations to confirmed orders.
func (r *Repository) SetStockReserver(s StockReserver) { r.reserver = s }

// SyncReservations makes the stock reservations of an order match its state:
// a confirmed order holds what is still to be delivered, everything else holds
// nothing. It is safe to call repeatedly. Failures are logged, never returned,
// because a stock hiccup must not block order handling.
func (r *Repository) SyncReservations(ctx context.Context, orgID, userID, orderID string) {
	if r.reserver == nil {
		return
	}
	o, err := r.GetOrder(ctx, orgID, orderID)
	if err != nil {
		return
	}
	if err := r.reserver.ReleaseOrder(ctx, orgID, o.Number); err != nil {
		slog.Warn("sales order reservation release", "order", o.Number, "err", err)
		return
	}
	if o.Status != "confirmed" {
		return
	}
	for i, l := range o.Lines {
		if l.ProductID == "" || l.Qty <= 0 || i >= len(o.Progress) {
			continue
		}
		left := o.Progress[i].Ordered - o.Progress[i].Delivered
		if left <= 0 {
			continue
		}
		if err := r.reserver.ReserveForOrder(ctx, orgID, userID, o.Number, l.ProductID, left); err != nil {
			slog.Warn("sales order reservation", "order", o.Number, "product", l.ProductID, "err", err)
		}
	}
}

// OrderIDForDelivery returns the order a delivery note belongs to.
func (r *Repository) OrderIDForDelivery(ctx context.Context, orgID, deliveryID string) string {
	var id string
	_ = r.pool.QueryRow(ctx, `SELECT order_id::text FROM adrient.sales_delivery_notes WHERE org_id=$1 AND id=$2::uuid`, orgID, deliveryID).Scan(&id)
	return id
}
