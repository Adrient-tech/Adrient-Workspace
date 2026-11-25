-- 0184: Stock counts, kits/BOM and assembly orders.

CREATE TABLE IF NOT EXISTS adrient.inv_counts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number       TEXT NOT NULL,
    kind         TEXT NOT NULL DEFAULT 'cycle' CHECK (kind IN ('cycle','full')),
    location_id  UUID NOT NULL REFERENCES adrient.inv_locations(id),
    category     TEXT NOT NULL DEFAULT '',
    state        TEXT NOT NULL DEFAULT 'counting' CHECK (state IN ('counting','review','approved','cancelled')),
    reason       TEXT NOT NULL DEFAULT '',
    note         TEXT NOT NULL DEFAULT '',
    created_by   UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    approved_at  TIMESTAMPTZ,
    UNIQUE (org_id, number)
);
CREATE TABLE IF NOT EXISTS adrient.inv_count_lines (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    count_id      UUID NOT NULL REFERENCES adrient.inv_counts(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id    UUID NOT NULL REFERENCES adrient.inv_products(id),
    expected_qty  NUMERIC(14,3) NOT NULL DEFAULT 0,
    counted_qty   NUMERIC(14,3),
    unit_cost     NUMERIC(16,4) NOT NULL DEFAULT 0,
    accepted      BOOLEAN NOT NULL DEFAULT true,
    note          TEXT NOT NULL DEFAULT '',
    UNIQUE (count_id, product_id)
);

CREATE TABLE IF NOT EXISTS adrient.inv_boms (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id  UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL DEFAULT 'assembly' CHECK (kind IN ('kit','assembly')),
    qty         NUMERIC(14,3) NOT NULL DEFAULT 1 CHECK (qty > 0),
    note        TEXT NOT NULL DEFAULT '',
    UNIQUE (org_id, product_id)
);
CREATE TABLE IF NOT EXISTS adrient.inv_bom_lines (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bom_id        UUID NOT NULL REFERENCES adrient.inv_boms(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    component_id  UUID NOT NULL REFERENCES adrient.inv_products(id),
    qty           NUMERIC(14,3) NOT NULL CHECK (qty > 0)
);
CREATE TABLE IF NOT EXISTS adrient.inv_assembly_orders (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number       TEXT NOT NULL,
    bom_id       UUID NOT NULL REFERENCES adrient.inv_boms(id),
    product_id   UUID NOT NULL REFERENCES adrient.inv_products(id),
    qty          NUMERIC(14,3) NOT NULL CHECK (qty > 0),
    location_id  UUID NOT NULL REFERENCES adrient.inv_locations(id),
    labor_cost   NUMERIC(16,2) NOT NULL DEFAULT 0,
    unit_cost    NUMERIC(16,4) NOT NULL DEFAULT 0,
    lot_no       TEXT NOT NULL DEFAULT '',
    expiry       DATE,
    state        TEXT NOT NULL DEFAULT 'done' CHECK (state IN ('done')),
    note         TEXT NOT NULL DEFAULT '',
    created_by   UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, number)
);
