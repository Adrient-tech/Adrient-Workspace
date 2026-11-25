-- 0204: price book and products on deals. inv_product_id links an Inventory product.

CREATE TABLE IF NOT EXISTS adrient.crm_price_book (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    inv_product_id UUID REFERENCES adrient.inv_products(id) ON DELETE SET NULL,
    name           TEXT NOT NULL,
    sku            TEXT NOT NULL DEFAULT '',
    price          NUMERIC(14,2) NOT NULL DEFAULT 0,
    currency       TEXT NOT NULL DEFAULT 'INR',
    active         BOOLEAN NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS crm_price_book_inv_uq ON adrient.crm_price_book (org_id, inv_product_id) WHERE inv_product_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS adrient.crm_deal_items (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deal_id    UUID NOT NULL REFERENCES adrient.crm_deals(id) ON DELETE CASCADE,
    product_id UUID REFERENCES adrient.crm_price_book(id) ON DELETE SET NULL,
    name       TEXT NOT NULL,
    qty        NUMERIC(14,3) NOT NULL DEFAULT 1,
    unit_price NUMERIC(14,2) NOT NULL DEFAULT 0,
    discount_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    position   INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS crm_deal_items_deal_idx ON adrient.crm_deal_items (deal_id);
