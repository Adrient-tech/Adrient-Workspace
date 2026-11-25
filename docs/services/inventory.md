# Inventory

Products, stock per location, suppliers, reorder list and stock history. Route `/inventory`, module id `inventory`.

## Feature map

|---|---|---|
| Products (name, SKU, barcode, category, unit, cost, price, photo, notes) | Done | Photos upload to Drive (`/api/v1/drive/files/upload`), id stored on the product |
| Variants | Done | A variant is a product row with `parent_id` and a label (e.g. "Red / L"); each has its own SKU, barcode, price, cost and stock. The list shows one row per family with the total |
| Page per product | Done (dialog) | Edit form, stock by location, variants, suppliers, recent history in one dialog. No bookmarkable URL yet |
| Stock per location | Done | Locations with a default; receive, remove, count (set to), sale, refund, correction, move between locations |
| Units | Done (basic) | Free-text unit, decimals allowed (3 places). No pack conversions |
| Shelf codes | Partial | Stored per product per location (set via API `shelf_code`); shown in the stock table. No UI field to edit yet |
| Minimums / low-stock alerts | Done | Low-stock level per product, badge on the Reorder entry, "Low stock" filter |
| Barcodes | Done | Generate internal EAN-13, preview, print label, type-in/USB scanner, camera scan where the browser has `BarcodeDetector` |
| Suppliers, several sources per product | Done | Supplier, their code, price, preferred flag per product. Suppliers can link to a Contacts record (`contact_id`) and work without one |
| Reorder | Done | Lines at/below minimum, suggested qty (reorder qty or 2x minimum minus stock, rounded to the supplier pack size), grouped plain-text order sheets |
| History | Done | Every change with reason, location, change, quantity after, reference, user |
| Reports | Done (basic) | Stock value (cost/retail), by category, by location, 30-day movement by reason, top sellers |
| Bulk actions | Partial | Multi-select archive/restore |
| Archive | Done | Archiving a family archives its variants |
| CSV import/export | Done | Column auto-detection, SKU matching (update sets stock, add adds to stock, skip), per-location quantities, formula-injection safe export |

## API for POS and Invoicing

Org-scoped REST (gateway) under `/api/v1/inventory`:

- `GET /products?query=&barcode=&location_id=&flat=true` returns products with `price`, `cost`, `sku`, `barcode`, `total_stock` and `stock[]` per location. `barcode` matches barcode or SKU exactly. `flat=true` lists variants as rows (use this for sale lookups).
- `GET /products/{id}`.
- `POST /products/{id}/adjust` `{location_id?, qty, reason, ref?, note?, absolute?, unit_cost?}`. Reasons: `sale` (qty is subtracted whatever its sign), `refund` and `receive` (added), `remove` (subtracted), `count` (with `absolute: true` sets the counted quantity), `correction` (signed). Sales and refunds may take stock below zero (overselling); other removals are rejected when stock is short. Every call writes a history row with `ref` (e.g. an invoice number).
- `POST /products/{id}/transfer`, `GET /history`, `GET /reorder`, `GET /report`.
- Locations: `/locations`. Suppliers: `/suppliers`.

Go callers can use `internal/inventory.Repository` directly (`ListProducts`, `GetProduct`, `AdjustStock`).

## Data

Migration `0110_inventory.sql`: `inv_locations`, `inv_suppliers`, `inv_products`, `inv_stock`, `inv_product_suppliers`, `inv_history`. Every table carries `org_id`. SKU is unique per org (case-insensitive) and barcode is unique per org when set.

## Purchasing, reservations and accounting
- Routes: stock extensions under `/api/v1/stock/*` (valuation, reservations, counts, lots ...) and purchasing under `/api/v1/purchasing/*` (orders, receipts, bills, returns, requests). The older `/api/v1/inventory/*` contract above still serves products, locations, suppliers and adjustments.
- Verified end to end by API: purchase order, approval, goods receipt, supplier bill, post. The bill posts once (a second post is refused); Inventory rises by the net amount and Accounts Payable by the gross.
- A confirmed sales order now reserves its product lines in Inventory (backorder allowed, so it never blocks the order). Dispatching a delivery releases and re-reserves what is still to deliver; cancelling releases everything. Reservations are visible under `/api/v1/stock/reservations`.
- POS and invoice deliveries move stock and queue COGS journals (flushed within 30 s).

## More features
