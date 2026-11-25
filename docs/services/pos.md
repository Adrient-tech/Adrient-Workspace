# POS (Point of Sale)

Register, cash drawer, sales, refunds, taxes and customer balances. Route `/pos`, module id `pos`, tile "POS" (Material `PointOfSale`, accent `#7B3FB5`).

## Data sharing

- **Products and stock are the Inventory module.** The register reads `GET /api/v1/inventory/products?flat=true`. Completing a sale calls `inventory.Repository.AdjustInTx` (reason `sale`, `ref` = POS sale id) inside the same Postgres transaction as the sale, so stock cannot drift: if a stock move fails the sale is rolled back. Restocking refunds use reason `refund` the same way. Sales may take stock below zero (Inventory rule).
- **Customers come from Contacts.** A sale stores `customer_id` (Contacts id, no foreign key so deleting a contact keeps the receipt) and a name snapshot.
- POS owns: registers, shifts, cash moves, sales, sale lines, payments, refunds, tax rates, discount presets, settings, loyalty and store-credit balances.

## Feature map

|---|---|---|
| Register: tap product grid, search, categories | Done | Search also takes a scanned barcode/SKU (Enter) |
| Custom one-off items | Done | Name + price, no stock move |
| Quantity, per-line discount (% or amount), line note | Done | |
| Order discount + reusable presets | Done | Presets managed in Settings |
| Taxes + tax summary | Done | Named rates, exclusive or inclusive, default rate, per-line choice; tax summary in Reports and on receipts |
| Cash with change, quick-tender buttons | Done | |
| Card / other payments recorded manually | Done | No processor integration |
| Split payments | Done | Several tenders on one sale, remaining balance shown, change only on cash |
| Hold and resume | Done | Held sales are saved server-side per register |
| Refunds (full or by line, restock option) | Done | Own refund records and receipts, original never rewritten; refund to cash/card/other/store credit |
| Loyalty card | Done (points, tiers, stamps) | Points earned per currency unit, redeemed as a payment at a set value; reversed pro rata on refund. Tiers and stamps: see |
| Store credit | Done | Top up manually, issue by refund, pay with it |
| Cash drawer open/close + reconciliation | Done | Opening float, paid in/out, expected vs counted, variance |
| Receipts (print/email-friendly) | Done | On-screen receipt, browser print (thermal-width CSS), `mailto:` plain-text copy |
| Thermal printer via browser print | Done | 80mm `@media print` layout |
| Sales reports | Done | Totals, net of refunds, by day, by payment method, top products, tax summary |
| Catalog import | Via Inventory | Inventory's CSV import is the catalog; POS has no second catalog |
| Arrange grid / favorites, product options, offline outbox, manager PIN, void, stamps/tiers, hardware drawer kick, email sending from server, customer-facing display, gift cards, Z report, stores | Done in (below) | |

## API (gateway, org-scoped, `/api/v1/pos`)

- Registers `GET/POST /registers`, `PATCH/DELETE /registers/{id}`; shifts `POST /registers/{id}/shift` (open), `POST /shifts/{id}/close`, `POST /shifts/{id}/cash`, `GET /shifts[/{id}]`.
- Sales `POST /sales/hold`, `POST /sales/complete`, `GET /sales/{id}`, `DELETE /sales/{id}` (held only), `POST /sales/{id}/refund`.
- **`GET /sales?status=&since=&until=&customer_id=&register_id=&query=&limit=&detail=true`** is the list-sales read API for Accounting/Invoicing: totals, tax, paid, refunded, status, and with `detail=true` lines and payments. Go callers can use `internal/pos.Repository.ListSales`.
- `GET /report?since=&until=`, `/taxes`, `/discounts`, `/settings`, `/balances`, `/balances/{contact_id}/adjust`.

## Rules

- A sale needs a register with an open shift. One open shift per register (unique index).
- Totals are computed server-side (`internal/pos/pricing.go`, mirrored in `web/app/src/pages/pos/logic.ts`): line discount, then order discount spread over lines, then tax per line. Money is 2-decimal.
- Non-cash tenders cannot overpay; cash overpay becomes change. Store credit and points cannot go negative.
- Receipt numbers are a per-org sequence assigned at completion (advisory lock), held sales have number 0.

## More features

Migration `0191_pos_wave3.sql`. The generated gRPC gateway could not be extended without regenerating protobuf, so the new features are plain JSON under **`/api/v1/pos/x/`** (same session, same org scoping, same audit log). Permissions: selling, voiding, refunding with a PIN and selling gift cards need `pos:sell`; setting your own PIN and reading reports follow the existing manager permissions; settings need `pos:settings:write`.

### Offline mode and idempotency
- **Every new sale** (online or not) is sent to `POST /x/sales/complete` with a client-made **idempotency key**. The server claims the key first (`pos_sync_log`), so a retry after a lost response, a double click or ten parallel retries create exactly one sale; the replay returns the original sale. A sale the server refuses (for example payments that do not cover the total) is remembered as **failed** under that key and answered `422` with the reason, so a client never retries it forever.
- When the server cannot be reached (offline, timeout, 5xx) the register saves the sale in an **IndexedDB outbox** on the device, prints the receipt marked "OFFLINE, number assigned when synced", and syncs automatically when the browser comes back online (and every 20 seconds while sales wait). The "Offline" bar shows waiting and rejected sales; rejected sales can be retried or discarded by a person.
- Receipt numbers are assigned by the server at sync time (per-org sequence under an advisory lock), so two offline devices never collide. Stock moves at sync time and can go negative (Inventory rule). The sale belongs to the register's shift that is open **at sync time**; if the shift was closed meanwhile the sale is rejected with "open a shift..." and waits for a manager.
- The catalogue (products, option groups, favourites, settings) is cached in the browser, so a register tab that is already open keeps selling when the connection drops. **Limit:** the app shell is not cached by a service worker, so reloading the page while offline does not work.
- Held sales are an online feature (a held sale completes through the classic path).

### Manager PIN, void, approvals
- A manager sets a 4-8 digit PIN (bcrypt; trivial PINs like 1234 or 1111 and a PIN another manager uses are refused) and chooses what it may approve (void, refund, discount). Five wrong PINs lock **the person typing** for five minutes, so a cashier guessing cannot lock the managers out.
- **Void** (`POST /x/sales/{id}/void`): reason required (3+ characters), manager PIN if enabled. Allowed only for a completed sale with no refunds; it refunds every line with restock to the original method and records reason and approver (`pos_voids`).
- **Refund** from the Sales page goes through `POST /x/sales/{id}/refund` with the PIN when "Refunds need a PIN" is on.
- **Discount approval:** discounts above a configured percent need a PIN at the register (checked again on the server for sales sent through `/x/sales/complete`).
- Honest limit: roles still decide who can call the old gRPC endpoints directly; the PIN rules apply to everything done through the register screens and `/x/` endpoints.

### Product options, favourites
Option groups (required / pick several, extra price) are assigned to products; the register asks before adding. The line name carries the choices ("Latte (Large, Oat milk)") and the price includes the extras. The Favourites tab shows the grid managers arrange in Settings (reorder, add, remove).

### Gift cards
Issue a card with a value (code like `K7QD-M2XP-9WJA`), top up, disable. At payment choose **Gift card**, enter the code, check the balance. Redemption happens **inside the sale transaction** with the card row locked: two tills cannot spend the same balance, an insufficient balance rejects the whole sale, and a retried sale redeems once (unique ledger entry per card and sale).

### Loyalty tiers and stamps
Tiers raise the points earned (`floor(total x earn rate x tier multiplier)`) once lifetime spend (net of refunds) reaches the tier's level. A stamp card adds a stamp for each sale of at least a set amount and pays **store credit** every N stamps, in the same transaction as the sale.

### Z report, stores, receipts, display, hardware
- **Z report / X report** per shift (`GET /x/zreport?shift_id=`): sales, tax, discounts, refunds, voids, takings and refunds by method, expected cash (opening + cash sales - cash refunds + paid in/out), counted cash and variance for a closed shift. Print layout is 80 mm.
- **Stores:** group registers into stores; the store's name, address and footer appear on emailed receipts and Z reports.
- **Email receipts from the server:** `POST /x/sales/{id}/email`. Without server email the button opens your mail app instead.
- **Customer-facing display:** "Customer display" opens `/pos/display` in a second window; the register publishes the cart and totals to it over a `BroadcastChannel`, so it works on a second screen of the **same device and browser**. It does not use the network.
- **Hardware:** receipt printing is the browser print dialog (80 mm layout). With **WebUSB** (Chrome/Edge on a computer, https or localhost) the register can also connect a USB ESC/POS printer, print the receipt and **kick the cash drawer** (ESC p), optionally on every cash sale. Where WebUSB is unavailable the buttons say why and fall back to the print dialog (many printers can open the drawer on print).

## audit
Verified by API: an idempotent sale (`/api/v1/pos/x/sales/complete`, replay returns the same sale) reduced stock and, after the sync, produced cash, sales and COGS journal entries with the right amounts. POS access is restricted to the Cashier / Store manager presets when an admin applies them (admin.md).

## More features
