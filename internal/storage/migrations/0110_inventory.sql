-- 0110: Inventory (products, variants, stock per location, suppliers, history).

CREATE TABLE IF NOT EXISTS adrient.inv_locations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    code        TEXT NOT NULL DEFAULT '',
    is_default  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS inv_locations_org_idx ON adrient.inv_locations (org_id);

CREATE TABLE IF NOT EXISTS adrient.inv_suppliers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    contact_id  TEXT NOT NULL DEFAULT '',
    email       TEXT NOT NULL DEFAULT '',
    phone       TEXT NOT NULL DEFAULT '',
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS inv_suppliers_org_idx ON adrient.inv_suppliers (org_id);

-- A variant is a product row whose parent_id points at its family.
CREATE TABLE IF NOT EXISTS adrient.inv_products (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    parent_id      UUID REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    variant_label  TEXT NOT NULL DEFAULT '',
    name           TEXT NOT NULL,
    sku            TEXT NOT NULL DEFAULT '',
    barcode        TEXT NOT NULL DEFAULT '',
    description    TEXT NOT NULL DEFAULT '',
    category       TEXT NOT NULL DEFAULT '',
    unit           TEXT NOT NULL DEFAULT 'pcs',
    price          NUMERIC(14,2) NOT NULL DEFAULT 0,
    cost           NUMERIC(14,2) NOT NULL DEFAULT 0,
    min_stock      NUMERIC(14,3) NOT NULL DEFAULT 0,
    reorder_qty    NUMERIC(14,3) NOT NULL DEFAULT 0,
    photo_file_id  TEXT NOT NULL DEFAULT '',
    archived       BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS inv_products_org_idx ON adrient.inv_products (org_id, archived);
CREATE INDEX IF NOT EXISTS inv_products_parent_idx ON adrient.inv_products (parent_id);
CREATE UNIQUE INDEX IF NOT EXISTS inv_products_sku_uq ON adrient.inv_products (org_id, lower(sku)) WHERE sku <> '';
CREATE UNIQUE INDEX IF NOT EXISTS inv_products_barcode_uq ON adrient.inv_products (org_id, barcode) WHERE barcode <> '';

CREATE TABLE IF NOT EXISTS adrient.inv_stock (
    product_id   UUID NOT NULL REFERENCES adrient.inv_products(id)  ON DELETE CASCADE,
    location_id  UUID NOT NULL REFERENCES adrient.inv_locations(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    qty          NUMERIC(14,3) NOT NULL DEFAULT 0,
    shelf_code   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (product_id, location_id)
);
CREATE INDEX IF NOT EXISTS inv_stock_org_idx ON adrient.inv_stock (org_id);

CREATE TABLE IF NOT EXISTS adrient.inv_product_suppliers (
    product_id    UUID NOT NULL REFERENCES adrient.inv_products(id)  ON DELETE CASCADE,
    supplier_id   UUID NOT NULL REFERENCES adrient.inv_suppliers(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    supplier_sku  TEXT NOT NULL DEFAULT '',
    cost          NUMERIC(14,2) NOT NULL DEFAULT 0,
    lead_days     INT NOT NULL DEFAULT 0,
    pack_size     NUMERIC(14,3) NOT NULL DEFAULT 1,
    preferred     BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (product_id, supplier_id)
);

CREATE TABLE IF NOT EXISTS adrient.inv_history (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id   UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    location_id  UUID REFERENCES adrient.inv_locations(id) ON DELETE SET NULL,
    delta        NUMERIC(14,3) NOT NULL,
    qty_after    NUMERIC(14,3) NOT NULL,
    reason       TEXT NOT NULL,
    note         TEXT NOT NULL DEFAULT '',
    ref          TEXT NOT NULL DEFAULT '',
    user_id      UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS inv_history_org_time_idx ON adrient.inv_history (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS inv_history_product_idx ON adrient.inv_history (product_id, created_at DESC);
