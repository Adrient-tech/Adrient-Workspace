-- 0111: Point of sale (registers, shifts, sales, payments, refunds, taxes,
-- discount presets, loyalty / store-credit balances). The product catalog and
-- stock are the Inventory module (inv_products / inv_stock).

CREATE TABLE IF NOT EXISTS adrient.pos_registers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    location_id  UUID REFERENCES adrient.inv_locations(id) ON DELETE SET NULL,
    archived     BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pos_registers_org_idx ON adrient.pos_registers (org_id);

CREATE TABLE IF NOT EXISTS adrient.pos_shifts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    register_id   UUID NOT NULL REFERENCES adrient.pos_registers(id) ON DELETE CASCADE,
    opened_by     UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    opened_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    opening_cash  NUMERIC(14,2) NOT NULL DEFAULT 0,
    closed_at     TIMESTAMPTZ,
    closed_by     UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    counted_cash  NUMERIC(14,2),
    expected_cash NUMERIC(14,2),
    note          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS pos_shifts_reg_idx ON adrient.pos_shifts (register_id, opened_at DESC);
-- At most one open shift per register.
CREATE UNIQUE INDEX IF NOT EXISTS pos_shifts_one_open_uq ON adrient.pos_shifts (register_id) WHERE closed_at IS NULL;

CREATE TABLE IF NOT EXISTS adrient.pos_cash_moves (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    shift_id    UUID NOT NULL REFERENCES adrient.pos_shifts(id) ON DELETE CASCADE,
    amount      NUMERIC(14,2) NOT NULL,
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.pos_tax_rates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    rate        NUMERIC(7,3) NOT NULL DEFAULT 0,
    inclusive   BOOLEAN NOT NULL DEFAULT false,
    is_default  BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS pos_tax_rates_org_idx ON adrient.pos_tax_rates (org_id);

CREATE TABLE IF NOT EXISTS adrient.pos_discount_presets (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id  UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name    TEXT NOT NULL,
    kind    TEXT NOT NULL DEFAULT 'percent',
    value   NUMERIC(14,2) NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS pos_discount_presets_org_idx ON adrient.pos_discount_presets (org_id);

CREATE TABLE IF NOT EXISTS adrient.pos_settings (
    org_id                 UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    store_name             TEXT NOT NULL DEFAULT '',
    currency               TEXT NOT NULL DEFAULT '',
    receipt_header         TEXT NOT NULL DEFAULT '',
    receipt_footer         TEXT NOT NULL DEFAULT 'Thank you!',
    loyalty_earn_per_unit  NUMERIC(10,4) NOT NULL DEFAULT 0,
    loyalty_point_value    NUMERIC(10,4) NOT NULL DEFAULT 0.01
);

CREATE TABLE IF NOT EXISTS adrient.pos_sales (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number          BIGINT NOT NULL DEFAULT 0,
    register_id     UUID REFERENCES adrient.pos_registers(id) ON DELETE SET NULL,
    shift_id        UUID REFERENCES adrient.pos_shifts(id) ON DELETE SET NULL,
    customer_id     TEXT NOT NULL DEFAULT '',
    customer_name   TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'held',
    subtotal        NUMERIC(14,2) NOT NULL DEFAULT 0,
    discount_total  NUMERIC(14,2) NOT NULL DEFAULT 0,
    tax_total       NUMERIC(14,2) NOT NULL DEFAULT 0,
    total           NUMERIC(14,2) NOT NULL DEFAULT 0,
    paid            NUMERIC(14,2) NOT NULL DEFAULT 0,
    change_due      NUMERIC(14,2) NOT NULL DEFAULT 0,
    discount_kind   TEXT NOT NULL DEFAULT '',
    discount_value  NUMERIC(14,2) NOT NULL DEFAULT 0,
    note            TEXT NOT NULL DEFAULT '',
    hold_label      TEXT NOT NULL DEFAULT '',
    refunded_total  NUMERIC(14,2) NOT NULL DEFAULT 0,
    loyalty_earned  NUMERIC(14,2) NOT NULL DEFAULT 0,
    created_by      UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS pos_sales_org_time_idx ON adrient.pos_sales (org_id, completed_at DESC);
CREATE INDEX IF NOT EXISTS pos_sales_shift_idx ON adrient.pos_sales (shift_id);
CREATE UNIQUE INDEX IF NOT EXISTS pos_sales_number_uq ON adrient.pos_sales (org_id, number) WHERE number > 0;

CREATE TABLE IF NOT EXISTS adrient.pos_sale_lines (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    sale_id          UUID NOT NULL REFERENCES adrient.pos_sales(id) ON DELETE CASCADE,
    position         INT NOT NULL DEFAULT 0,
    product_id       UUID REFERENCES adrient.inv_products(id) ON DELETE SET NULL,
    name             TEXT NOT NULL,
    sku              TEXT NOT NULL DEFAULT '',
    qty              NUMERIC(14,3) NOT NULL,
    unit_price       NUMERIC(14,2) NOT NULL,
    discount_kind    TEXT NOT NULL DEFAULT '',
    discount_value   NUMERIC(14,2) NOT NULL DEFAULT 0,
    tax_rate_id      TEXT NOT NULL DEFAULT '',
    tax_name         TEXT NOT NULL DEFAULT '',
    tax_rate         NUMERIC(7,3) NOT NULL DEFAULT 0,
    tax_inclusive    BOOLEAN NOT NULL DEFAULT false,
    discount_amount  NUMERIC(14,2) NOT NULL DEFAULT 0,
    tax_amount       NUMERIC(14,2) NOT NULL DEFAULT 0,
    line_total       NUMERIC(14,2) NOT NULL DEFAULT 0,
    refunded_qty     NUMERIC(14,3) NOT NULL DEFAULT 0,
    refunded_amount  NUMERIC(14,2) NOT NULL DEFAULT 0,
    note             TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS pos_sale_lines_sale_idx ON adrient.pos_sale_lines (sale_id, position);
CREATE INDEX IF NOT EXISTS pos_sale_lines_product_idx ON adrient.pos_sale_lines (org_id, product_id);

CREATE TABLE IF NOT EXISTS adrient.pos_payments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    sale_id     UUID NOT NULL REFERENCES adrient.pos_sales(id) ON DELETE CASCADE,
    method      TEXT NOT NULL,
    amount      NUMERIC(14,2) NOT NULL,
    tendered    NUMERIC(14,2) NOT NULL,
    ref         TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pos_payments_sale_idx ON adrient.pos_payments (sale_id);

CREATE TABLE IF NOT EXISTS adrient.pos_refunds (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    sale_id     UUID NOT NULL REFERENCES adrient.pos_sales(id) ON DELETE CASCADE,
    shift_id    UUID REFERENCES adrient.pos_shifts(id) ON DELETE SET NULL,
    total       NUMERIC(14,2) NOT NULL,
    method      TEXT NOT NULL,
    restock     BOOLEAN NOT NULL DEFAULT false,
    note        TEXT NOT NULL DEFAULT '',
    created_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pos_refunds_sale_idx ON adrient.pos_refunds (sale_id);
CREATE INDEX IF NOT EXISTS pos_refunds_shift_idx ON adrient.pos_refunds (shift_id);
CREATE INDEX IF NOT EXISTS pos_refunds_org_time_idx ON adrient.pos_refunds (org_id, created_at DESC);

CREATE TABLE IF NOT EXISTS adrient.pos_refund_lines (
    refund_id  UUID NOT NULL REFERENCES adrient.pos_refunds(id) ON DELETE CASCADE,
    line_id    UUID NOT NULL REFERENCES adrient.pos_sale_lines(id) ON DELETE CASCADE,
    qty        NUMERIC(14,3) NOT NULL,
    amount     NUMERIC(14,2) NOT NULL,
    PRIMARY KEY (refund_id, line_id)
);

-- Loyalty points and store credit per Contacts record.
CREATE TABLE IF NOT EXISTS adrient.pos_balances (
    org_id          UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    contact_id      TEXT NOT NULL,
    loyalty_points  NUMERIC(14,2) NOT NULL DEFAULT 0,
    store_credit    NUMERIC(14,2) NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, contact_id)
);

CREATE TABLE IF NOT EXISTS adrient.pos_balance_entries (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    contact_id  TEXT NOT NULL,
    kind        TEXT NOT NULL,
    delta       NUMERIC(14,2) NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    sale_id     TEXT NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pos_balance_entries_idx ON adrient.pos_balance_entries (org_id, contact_id, created_at DESC);
