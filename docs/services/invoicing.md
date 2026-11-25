# Invoicing

Route `/invoicing`, module id `invoicing`, accent `#C2410C`, migration `0112_invoicing.sql`.

## Data sharing
- **Clients** are Contacts. A document stores `contact_id` plus a name/email/address snapshot so old invoices never change. A contact can be created from inside the invoice editor.
- **Line items** are free text, a saved price-list item, or an Inventory product (`GET /api/v1/inventory/products`, price copied, `product_id` kept).
- **Taxes** reuse POS tax rates (`GET /api/v1/pos/taxes`); the rate and name are copied onto the line. Without POS taxes you type a rate.
- **Stock** is not touched unless Settings > "Deduct stock when an invoice is first sent" is on. Then each line with a `product_id` calls Inventory `AdjustStock` (reason `sale`, ref = invoice number), once per invoice.
- **Email** uses the server `email.Sender` (same as invites). If it is not configured, Send falls back to the user's mail app (`mailto:`) and marks the document sent.
- **Read API for Accounting:** `GET /api/v1/invoicing/documents?kind=invoice` and `GET /api/v1/invoicing/payments?from=&to=`.

## API (JSON, `/api/v1/invoicing`, auth-wrapped)
settings, items, documents (list/get/create/update/trash/restore/purge, duplicate, convert, status, send, remind, payments), payments, recurring (+run), clients, report, capabilities. Public: `GET /api/v1/public/invoices/{token}` (read-only, unguessable 192-bit token, drafts and trashed documents return 404). SPA page: `/invoice/{token}`.

| Feature | Status |
|---|---|
| Estimates / quotes (accept, decline, convert to invoice) | done |
| Invoices with line items, per-line discount and tax, document discount (% or amount) | done |
| Send by email, public share link, "viewed" tracking | done |
| Payment reminders (manual, counted per invoice) | done; scheduled reminders added in (see below) |
| Recurring invoices (weekly/monthly/quarterly/yearly, catch-up, end date, auto-send) | done; generated when the module opens or "Run now" (no background scheduler) |
| Receipts and refunds (printable receipt per payment, refund up to amount paid) | done |
| Reports (invoiced, collected, outstanding, overdue, aging, by month/client/status) | done |
| One total across currencies (manual exchange rates in Settings) | done |
| Payment schedules (instalments printed on the document) | done (informational) |
| Units and a price list | done |
| Write-offs | done |
| Tax notes | done |
| Trash, not void | done (restore, delete forever only from trash) |
| Clients | done (per-client totals, from Contacts) |
| Branding (name, address, tax id, logo URL, payment details) | done |
| PDF and print | done via browser print / Save as PDF with print CSS |
| CSV import (invoices, price list) and CSV export | done |
| Payment-gateway links | done (Razorpay, off until keys exist) |

## the sales side

Migration `0190_sales_core.sql` (additive, idempotent). This module now covers Zoho Invoice/Books-style and Odoo Sales-style flows end to end: **estimate -> sales order -> delivery note -> invoice -> payment -> credit note**.

### Estimates and quotes
- Each estimate has `valid_until` (default: issue date + "Estimate valid for" days from Settings). A sent estimate past that date shows **Expired** and cannot be accepted.
- **Versions:** "Revise (new version)" copies the estimate to `-v2`, `-v3`... and marks the old one **Replaced**. The old public link says it was replaced and points to the latest version. `GET /documents/{id}/versions` lists the family.
- **Customer acceptance on the public link** (`/invoice/{token}`): the customer types their full name, ticks "I accept...", and presses Accept. The server stores name, time, IP, user agent and a SHA-256 fingerprint over number, version, total, lines, name and time. This is an e-sign style acceptance, not a qualified digital signature. Accepting twice keeps the first acceptance. Declining stores an optional reason. Public writes are rate limited (30 per minute per address).
- Convert: "Create sales order" (idempotent, one order per estimate) or "Convert to invoice" (idempotent, returns the same invoice if clicked twice).
- **CRM:** `POST /api/v1/invoicing/estimates/from-deal` with `{deal_id, title, contact_id, client_name, client_email, client_address, client_gstin, currency, value, tax_rate, notes, lines?}`, or in Go `invoicing.Repository.CreateEstimateFromDeal(ctx, orgID, userID, invoicing.DealEstimate{...})`. It is idempotent per `deal_id` (the estimate carries `source_ref = "crm:deal:<id>"`); the second call returns the same estimate with `created:false`. Without `lines` one line with the deal title and value is created.

### Sales orders, deliveries, partial invoicing
- Orders keep stable line keys (`l1`, `l2`...). **Nothing is stored as a counter:** packed, delivered and invoiced quantities are always derived from the delivery notes and invoices that reference a line (`internal/invoicing/orders.go: Progress`).
- Delivery notes claim quantity while draft (so two notes cannot promise the same units) and count as delivered once **dispatched**. The sum over notes never exceeds the order. The packing slip is a print page with no prices.
- **Stock:** dispatching a delivery note moves stock once through Inventory `AdjustStock` (reason `sale`, ref = delivery number). Invoices raised from an order never deduct stock again, even when "Deduct stock on first send" is on. Orders that are only invoiced and never dispatched do not move stock.
- Invoice from an order: "delivered and not yet billed", "everything left", or explicit quantities. Billed quantity is limited to ordered minus already invoiced. A fixed amount discount on the order is billed pro rata.
- Order and estimate lines can pick Inventory products; reservations stay with Inventory (existing `/api/v1/inventory` contracts untouched).

### Pricing
- **Price lists** (`/price-lists`): named lists, optional default list, assigned customers, quantity tiers (`min_qty`), and customer-specific rows. Resolution order for a line with a product: customer-specific price, a list assigned to the customer or chosen on the document, the default list, the item's own price. Inside a level the highest reached tier wins.
- **Discount rules** (`/discount-rules`): line rules (percent, optional product, minimum quantity) and order rules (percent or amount above a minimum amount), with customer, date range and priority. Rules do not stack: best priority wins. A larger discount typed by hand is kept.
- `POST /pricing/resolve` returns repriced lines plus the rules that fired. The editor button "Apply prices and discounts" calls it.

### GST
- Set your state in Settings > GST. The document then splits tax into CGST + SGST (UTGST in the five union territories without a legislature) when the **place of supply** equals your state, and IGST otherwise (export, code 96, is always IGST). An empty place of supply is your own state. A customer GSTIN fills the place of supply from its first two digits.
- Lines carry **HSN/SAC**. Tax is rounded per line and the CGST/SGST split is exact to the paisa, so totals match what a GST return needs. With no state configured the old single "Tax" behaviour is unchanged.
- **E-invoice JSON** (`GET /documents/{id}/einvoice`): IRP schema 1.1 for invoices and credit notes, with a list of what is missing (GSTINs, HSN, PIN codes). Adrient prepares the file; it does not call the government portal.

### Invoicing
- **Recurring invoices** are now created by a scheduler (every 15 minutes) as well as "Run now". One invoice per template and period (unique index on `recurring_id, recurring_period`), so repeated or parallel runs never double bill.
- **Credit notes:** `POST /documents/{invoice}/credit-note` with the lines to credit (or none for the whole invoice). The server refuses totals above what is still creditable. The credit is applied to the invoice balance (a `credit` row on the invoice, an `applied` row on the note); any excess stays on the note and can be **refunded** (`/refund`) or applied to another invoice of the same customer (`/apply`). Credit notes are documents with `kind = credit_note`, `credit_for = <invoice id>`, numbered with the credit prefix.
  - **For Accounting:** read them with `GET /documents?kind=credit_note` (each has `total`, `tax_total`, `cgst_total`, `sgst_total`, `igst_total`, `credit_for`). Payment rows with `kind` `credit`/`applied` (method `credit_note`) are **not cash** and are left out of `GET /payments`. A credit note refund appears in `/payments` as `kind = refund` on the note's number. The invoice's `credited` and `balance` fields already net credits.
- **Partial payments** were already supported; the balance, status "Partly paid" and aging follow.
- **Reminders:** rules ("3 days before due", "7 days after") and templates in Settings; the scheduler sends each (invoice, rule) pair once, and an invoice first seen very late gets one reminder, not four. Needs server email and a public URL (saved automatically from your browser). **WhatsApp** is a click-to-chat link (`wa.me`) with the message filled in; nothing is sent by WhatsApp servers.
- **Payment links (Razorpay):** keys come from the server environment (`RAZORPAY_KEY_ID`, `RAZORPAY_KEY_SECRET`, `RAZORPAY_WEBHOOK_SECRET`) or Settings > Online payments. Nothing is hardcoded. With no keys the pay button says "Online payment not available" and the API answers 409. A paid link posts to `/api/v1/public/invoices/webhook/razorpay` (HMAC-SHA256 signature checked; replays are harmless because the payment id is unique per invoice). Payments are recorded with method `razorpay`.
- **Customer portal:** "Copy customer portal link" gives `/portal/{token}`: all invoices and open estimates of one customer, each linking to the view/pay page. Tokens are 192-bit and per customer.
- **UPI QR** on the invoice sheet (from your UPI id; amount is the balance), bank details, accent colour, footer and logo: "PDF" is the browser's Save as PDF of the printable sheet.
- **Statements of account** and **aging per customer** ("Who owes" page), printable.
- **Bulk actions** on the document list: email, reminders, mark sent, trash, with a result per document.

### Subscriptions
Plans (weekly/monthly/quarterly/yearly, price, tax, HSN/SAC) and subscriptions. Renewals invoice automatically with one invoice per subscription period (unique index) and catch up at most 12 missed periods. **Dunning:** an overdue unpaid invoice marks the subscription **past due**; after "pause after N days" it is **paused** and stops renewing; resume it after payment. Cancel now, or at the end of the current period.

### API additions (all under `/api/v1/invoicing`, auth-wrapped)
`documents/{id}/version|versions|order|credit-note|apply|refund|payment-link|einvoice|whatsapp`, `documents/bulk`, `estimates/from-deal`, `orders` (+ `/{id}`, `/status`, `/deliveries`, `/invoice`), `deliveries` (+ `/dispatch`, `/cancel`), `price-lists`, `discount-rules`, `pricing/resolve`, `gateway`, `customers/aging`, `statement?customer=`, `portal-link`, `reminders/log|run`, `plans`, `subscriptions` (+ `/run`, `/{id}/action`). Public: `POST /api/v1/public/invoices/{token}/accept|decline|pay`, `GET /api/v1/public/portal/{token}`, `POST /api/v1/public/invoices/webhook/razorpay`.

## wiring
- Confirmed sales orders reserve stock; see inventory.md. Order lines need a product to reserve.
- A CRM deal entering **Won** creates one draft estimate (`CreateEstimateFromDeal`, idempotent per deal, lines from the deal items) through the CRM hook; the deal dialog shows "Open estimate" and `/invoicing?doc=<id>` opens it.
- Verified by API: estimate, order (reserves 4), partial delivery of 3 (reservation becomes 1, stock -3 because overselling is allowed), invoice, payment; ledger shows Sales, COGS and zero receivable.

## More features
