-- 0183: Purchasing (requests, orders, goods receipts, supplier bills, returns, price lists, reorder rules).

CREATE TABLE IF NOT EXISTS adrient.inv_purchase_settings (
    org_id               UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    company_name         TEXT NOT NULL DEFAULT '',
    company_address      TEXT NOT NULL DEFAULT '',
    company_gstin        TEXT NOT NULL DEFAULT '',
    currency             TEXT NOT NULL DEFAULT 'INR',
    default_terms        TEXT NOT NULL DEFAULT '',
    require_approval     BOOLEAN NOT NULL DEFAULT true,
    approval_threshold   NUMERIC(16,2) NOT NULL DEFAULT 0,
    over_receipt_pct     NUMERIC(6,2) NOT NULL DEFAULT 0,
    under_receipt_policy TEXT NOT NULL DEFAULT 'backorder' CHECK (under_receipt_policy IN ('backorder','close')),
    qty_tolerance_pct    NUMERIC(6,2) NOT NULL DEFAULT 0,
    price_tolerance_pct  NUMERIC(6,2) NOT NULL DEFAULT 2,
    tax_split            TEXT NOT NULL DEFAULT 'cgst_sgst' CHECK (tax_split IN ('cgst_sgst','igst'))
);

CREATE TABLE IF NOT EXISTS adrient.inv_supplier_prices (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    supplier_id  UUID NOT NULL REFERENCES adrient.inv_suppliers(id) ON DELETE CASCADE,
    product_id   UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    min_qty      NUMERIC(14,3) NOT NULL DEFAULT 1,
    unit_cost    NUMERIC(16,4) NOT NULL,
    lead_days    INT NOT NULL DEFAULT 0,
    valid_from   DATE,
    valid_to     DATE,
    supplier_sku TEXT NOT NULL DEFAULT '',
    note         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS inv_supplier_prices_idx ON adrient.inv_supplier_prices (org_id, product_id, supplier_id);

CREATE TABLE IF NOT EXISTS adrient.inv_purchase_requests (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number        TEXT NOT NULL,
    state         TEXT NOT NULL DEFAULT 'draft' CHECK (state IN ('draft','submitted','approved','rejected','converted')),
    note          TEXT NOT NULL DEFAULT '',
    needed_by     DATE,
    requested_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    decided_by    UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    po_id         UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, number)
);
CREATE TABLE IF NOT EXISTS adrient.inv_purchase_request_lines (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id  UUID NOT NULL REFERENCES adrient.inv_purchase_requests(id) ON DELETE CASCADE,
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id  UUID NOT NULL REFERENCES adrient.inv_products(id),
    qty         NUMERIC(14,3) NOT NULL CHECK (qty > 0),
    note        TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS adrient.inv_purchase_orders (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number           TEXT NOT NULL,
    supplier_id      UUID NOT NULL REFERENCES adrient.inv_suppliers(id),
    state            TEXT NOT NULL DEFAULT 'draft'
        CHECK (state IN ('draft','approved','sent','acknowledged','partial','received','closed','cancelled')),
    order_date       DATE NOT NULL DEFAULT CURRENT_DATE,
    expected_date    DATE,
    ship_to_loc_id   UUID REFERENCES adrient.inv_locations(id) ON DELETE SET NULL,
    currency         TEXT NOT NULL DEFAULT 'INR',
    supplier_ref     TEXT NOT NULL DEFAULT '',
    notes            TEXT NOT NULL DEFAULT '',
    terms            TEXT NOT NULL DEFAULT '',
    subtotal         NUMERIC(16,2) NOT NULL DEFAULT 0,
    tax_total        NUMERIC(16,2) NOT NULL DEFAULT 0,
    total            NUMERIC(16,2) NOT NULL DEFAULT 0,
    request_id       UUID REFERENCES adrient.inv_purchase_requests(id) ON DELETE SET NULL,
    approved_by      UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    approved_at      TIMESTAMPTZ,
    sent_at          TIMESTAMPTZ,
    sent_to          TEXT NOT NULL DEFAULT '',
    ack_at           TIMESTAMPTZ,
    ack_ref          TEXT NOT NULL DEFAULT '',
    ack_note         TEXT NOT NULL DEFAULT '',
    promised_date    DATE,
    closed_at        TIMESTAMPTZ,
    created_by       UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, number)
);
CREATE INDEX IF NOT EXISTS inv_po_org_state_idx ON adrient.inv_purchase_orders (org_id, state);
CREATE INDEX IF NOT EXISTS inv_po_supplier_idx ON adrient.inv_purchase_orders (org_id, supplier_id);

CREATE TABLE IF NOT EXISTS adrient.inv_purchase_order_lines (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    po_id         UUID NOT NULL REFERENCES adrient.inv_purchase_orders(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id    UUID NOT NULL REFERENCES adrient.inv_products(id),
    description   TEXT NOT NULL DEFAULT '',
    qty           NUMERIC(14,3) NOT NULL CHECK (qty > 0),
    unit_cost     NUMERIC(16,4) NOT NULL DEFAULT 0,
    tax_pct       NUMERIC(6,2) NOT NULL DEFAULT 0,
    qty_received  NUMERIC(14,3) NOT NULL DEFAULT 0,
    qty_returned  NUMERIC(14,3) NOT NULL DEFAULT 0,
    qty_billed    NUMERIC(14,3) NOT NULL DEFAULT 0,
    sort          INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS inv_po_lines_po_idx ON adrient.inv_purchase_order_lines (po_id);
CREATE INDEX IF NOT EXISTS inv_po_lines_prod_idx ON adrient.inv_purchase_order_lines (org_id, product_id);

CREATE TABLE IF NOT EXISTS adrient.inv_grns (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number           TEXT NOT NULL,
    po_id            UUID REFERENCES adrient.inv_purchase_orders(id) ON DELETE SET NULL,
    supplier_id      UUID NOT NULL REFERENCES adrient.inv_suppliers(id),
    location_id      UUID NOT NULL REFERENCES adrient.inv_locations(id),
    received_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivery_ref     TEXT NOT NULL DEFAULT '',
    note             TEXT NOT NULL DEFAULT '',
    landed_cost      NUMERIC(16,2) NOT NULL DEFAULT 0,
    landed_method    TEXT NOT NULL DEFAULT 'value' CHECK (landed_method IN ('value','qty')),
    landed_note      TEXT NOT NULL DEFAULT '',
    created_by       UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    UNIQUE (org_id, number)
);
CREATE INDEX IF NOT EXISTS inv_grns_po_idx ON adrient.inv_grns (org_id, po_id);

CREATE TABLE IF NOT EXISTS adrient.inv_grn_lines (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    grn_id        UUID NOT NULL REFERENCES adrient.inv_grns(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    po_line_id    UUID REFERENCES adrient.inv_purchase_order_lines(id) ON DELETE SET NULL,
    product_id    UUID NOT NULL REFERENCES adrient.inv_products(id),
    qty           NUMERIC(14,3) NOT NULL CHECK (qty > 0),
    unit_cost     NUMERIC(16,4) NOT NULL DEFAULT 0,
    landed_alloc  NUMERIC(16,4) NOT NULL DEFAULT 0,
    lot_no        TEXT NOT NULL DEFAULT '',
    expiry        DATE,
    serials       JSONB NOT NULL DEFAULT '[]',
    qty_returned  NUMERIC(14,3) NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS inv_grn_lines_grn_idx ON adrient.inv_grn_lines (grn_id);

CREATE TABLE IF NOT EXISTS adrient.inv_supplier_bills (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number           TEXT NOT NULL,
    supplier_id      UUID NOT NULL REFERENCES adrient.inv_suppliers(id),
    po_id            UUID REFERENCES adrient.inv_purchase_orders(id) ON DELETE SET NULL,
    bill_ref         TEXT NOT NULL DEFAULT '',
    bill_date        DATE NOT NULL DEFAULT CURRENT_DATE,
    due_date         DATE,
    state            TEXT NOT NULL DEFAULT 'draft' CHECK (state IN ('draft','posted','void')),
    match_status     TEXT NOT NULL DEFAULT 'unchecked',
    match_detail     JSONB NOT NULL DEFAULT '[]',
    override_reason  TEXT NOT NULL DEFAULT '',
    subtotal         NUMERIC(16,2) NOT NULL DEFAULT 0,
    tax_total        NUMERIC(16,2) NOT NULL DEFAULT 0,
    total            NUMERIC(16,2) NOT NULL DEFAULT 0,
    posted_at        TIMESTAMPTZ,
    accounting_state TEXT NOT NULL DEFAULT 'none',
    accounting_error TEXT NOT NULL DEFAULT '',
    created_by       UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, number)
);
CREATE INDEX IF NOT EXISTS inv_bills_po_idx ON adrient.inv_supplier_bills (org_id, po_id);

CREATE TABLE IF NOT EXISTS adrient.inv_supplier_bill_lines (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bill_id      UUID NOT NULL REFERENCES adrient.inv_supplier_bills(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    po_line_id   UUID REFERENCES adrient.inv_purchase_order_lines(id) ON DELETE SET NULL,
    product_id   UUID REFERENCES adrient.inv_products(id),
    description  TEXT NOT NULL DEFAULT '',
    qty          NUMERIC(14,3) NOT NULL CHECK (qty > 0),
    unit_cost    NUMERIC(16,4) NOT NULL DEFAULT 0,
    tax_pct      NUMERIC(6,2) NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS inv_bill_lines_bill_idx ON adrient.inv_supplier_bill_lines (bill_id);

CREATE TABLE IF NOT EXISTS adrient.inv_vendor_returns (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number        TEXT NOT NULL,
    supplier_id   UUID NOT NULL REFERENCES adrient.inv_suppliers(id),
    grn_id        UUID REFERENCES adrient.inv_grns(id) ON DELETE SET NULL,
    po_id         UUID REFERENCES adrient.inv_purchase_orders(id) ON DELETE SET NULL,
    location_id   UUID NOT NULL REFERENCES adrient.inv_locations(id),
    reason        TEXT NOT NULL DEFAULT '',
    credit_note   TEXT NOT NULL DEFAULT '',
    returned_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by    UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    UNIQUE (org_id, number)
);
CREATE TABLE IF NOT EXISTS adrient.inv_vendor_return_lines (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    return_id     UUID NOT NULL REFERENCES adrient.inv_vendor_returns(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    grn_line_id   UUID REFERENCES adrient.inv_grn_lines(id) ON DELETE SET NULL,
    product_id    UUID NOT NULL REFERENCES adrient.inv_products(id),
    qty           NUMERIC(14,3) NOT NULL CHECK (qty > 0),
    unit_cost     NUMERIC(16,4) NOT NULL DEFAULT 0,
    lot_no        TEXT NOT NULL DEFAULT '',
    serials       JSONB NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS adrient.inv_reorder_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id   UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    min_qty      NUMERIC(14,3) NOT NULL DEFAULT 0,
    max_qty      NUMERIC(14,3) NOT NULL DEFAULT 0,
    multiple     NUMERIC(14,3) NOT NULL DEFAULT 1,
    supplier_id  UUID REFERENCES adrient.inv_suppliers(id) ON DELETE SET NULL,
    active       BOOLEAN NOT NULL DEFAULT true,
    UNIQUE (org_id, product_id)
);
