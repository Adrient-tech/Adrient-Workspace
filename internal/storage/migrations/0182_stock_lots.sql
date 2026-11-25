-- 0182: Lots/batches, serial numbers, traceability.

CREATE TABLE IF NOT EXISTS adrient.inv_lots (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id   UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    lot_no       TEXT NOT NULL,
    expiry       DATE,
    mfg_date     DATE,
    supplier_id  UUID REFERENCES adrient.inv_suppliers(id) ON DELETE SET NULL,
    note         TEXT NOT NULL DEFAULT '',
    recalled     BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS inv_lots_no_uq ON adrient.inv_lots (org_id, product_id, lower(lot_no));
CREATE INDEX IF NOT EXISTS inv_lots_expiry_idx ON adrient.inv_lots (org_id, expiry);

CREATE TABLE IF NOT EXISTS adrient.inv_lot_stock (
    lot_id       UUID NOT NULL REFERENCES adrient.inv_lots(id) ON DELETE CASCADE,
    location_id  UUID NOT NULL REFERENCES adrient.inv_locations(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id   UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    qty          NUMERIC(14,3) NOT NULL DEFAULT 0,
    PRIMARY KEY (lot_id, location_id)
);
CREATE INDEX IF NOT EXISTS inv_lot_stock_prod_idx ON adrient.inv_lot_stock (org_id, product_id);

CREATE TABLE IF NOT EXISTS adrient.inv_serials (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id   UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    serial_no    TEXT NOT NULL,
    lot_id       UUID REFERENCES adrient.inv_lots(id) ON DELETE SET NULL,
    state        TEXT NOT NULL DEFAULT 'in_stock' CHECK (state IN ('in_stock','out')),
    location_id  UUID REFERENCES adrient.inv_locations(id) ON DELETE SET NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    out_ref      TEXT NOT NULL DEFAULT '',
    out_at       TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS inv_serials_no_uq ON adrient.inv_serials (org_id, product_id, lower(serial_no));
CREATE INDEX IF NOT EXISTS inv_serials_stock_idx ON adrient.inv_serials (org_id, product_id, state);

-- Every lot/serial movement, signed, with the document and counterparty.
CREATE TABLE IF NOT EXISTS adrient.inv_trace (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id   UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    lot_id       UUID REFERENCES adrient.inv_lots(id) ON DELETE CASCADE,
    serial_id    UUID REFERENCES adrient.inv_serials(id) ON DELETE CASCADE,
    location_id  UUID REFERENCES adrient.inv_locations(id) ON DELETE SET NULL,
    qty          NUMERIC(14,3) NOT NULL,
    reason       TEXT NOT NULL,
    ref          TEXT NOT NULL DEFAULT '',
    party        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS inv_trace_lot_idx ON adrient.inv_trace (org_id, lot_id, created_at);
CREATE INDEX IF NOT EXISTS inv_trace_serial_idx ON adrient.inv_trace (org_id, serial_id, created_at);
