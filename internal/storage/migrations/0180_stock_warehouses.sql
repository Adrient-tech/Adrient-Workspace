-- 0180: Warehouses/locations hierarchy, stock transfers with in-transit, reservations.

ALTER TABLE adrient.inv_locations ADD COLUMN IF NOT EXISTS parent_id UUID REFERENCES adrient.inv_locations(id) ON DELETE RESTRICT;
ALTER TABLE adrient.inv_locations ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'warehouse';
ALTER TABLE adrient.inv_locations ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE adrient.inv_locations ADD COLUMN IF NOT EXISTS address TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS inv_locations_parent_idx ON adrient.inv_locations (parent_id);

-- Per-org document counters (PO-0001, GRN-0001 ...).
CREATE TABLE IF NOT EXISTS adrient.inv_counters (
    org_id   UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind     TEXT NOT NULL,
    last_no  BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (org_id, kind)
);

CREATE TABLE IF NOT EXISTS adrient.inv_transfers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number       TEXT NOT NULL,
    from_loc_id  UUID NOT NULL REFERENCES adrient.inv_locations(id),
    to_loc_id    UUID NOT NULL REFERENCES adrient.inv_locations(id),
    state        TEXT NOT NULL DEFAULT 'draft' CHECK (state IN ('draft','in_transit','received','cancelled')),
    note         TEXT NOT NULL DEFAULT '',
    created_by   UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    shipped_at   TIMESTAMPTZ,
    received_at  TIMESTAMPTZ,
    UNIQUE (org_id, number)
);
CREATE INDEX IF NOT EXISTS inv_transfers_org_idx ON adrient.inv_transfers (org_id, state);

CREATE TABLE IF NOT EXISTS adrient.inv_transfer_lines (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id   UUID NOT NULL REFERENCES adrient.inv_transfers(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id    UUID NOT NULL REFERENCES adrient.inv_products(id),
    qty           NUMERIC(14,3) NOT NULL CHECK (qty > 0),
    qty_shipped   NUMERIC(14,3) NOT NULL DEFAULT 0,
    qty_received  NUMERIC(14,3) NOT NULL DEFAULT 0,
    -- lots/serials chosen at shipping: [{"lot_id":..,"lot_no":..,"qty":..}] and ["SN1",...]
    lots          JSONB NOT NULL DEFAULT '[]',
    serials       JSONB NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS inv_transfer_lines_tr_idx ON adrient.inv_transfer_lines (transfer_id);
CREATE INDEX IF NOT EXISTS inv_transfer_lines_prod_idx ON adrient.inv_transfer_lines (product_id);

CREATE TABLE IF NOT EXISTS adrient.inv_reservations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id  UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    location_id UUID REFERENCES adrient.inv_locations(id) ON DELETE CASCADE,
    qty         NUMERIC(14,3) NOT NULL CHECK (qty > 0),
    ref_type    TEXT NOT NULL DEFAULT 'sales_order',
    ref         TEXT NOT NULL,
    state       TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active','fulfilled','released')),
    note        TEXT NOT NULL DEFAULT '',
    created_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ,
    closed_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS inv_reservations_prod_idx ON adrient.inv_reservations (org_id, product_id) WHERE state='active';
CREATE INDEX IF NOT EXISTS inv_reservations_ref_idx ON adrient.inv_reservations (org_id, ref);
