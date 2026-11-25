-- 0191: POS wave 3. Manager PIN, voids, idempotent sync log (offline mode),
-- product options, favourites grid, gift cards, loyalty stamps, stores.
-- Additive and idempotent.

CREATE TABLE IF NOT EXISTS adrient.pos_ext_settings (
    org_id     UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    data       JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Manager PINs. Only a bcrypt hash is stored; five wrong tries lock the person typing for five minutes.
CREATE TABLE IF NOT EXISTS adrient.pos_pins (
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    pin_hash     TEXT NOT NULL,
    can_void     BOOLEAN NOT NULL DEFAULT true,
    can_refund   BOOLEAN NOT NULL DEFAULT true,
    can_discount BOOLEAN NOT NULL DEFAULT true,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);

-- Wrong-PIN counter per terminal user (the person typing), so one cashier
-- guessing cannot be told apart from a manager and cannot lock the managers out.
CREATE TABLE IF NOT EXISTS adrient.pos_pin_attempts (
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL,
    failed       INT NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ,
    PRIMARY KEY (org_id, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.pos_voids (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    sale_id     UUID NOT NULL REFERENCES adrient.pos_sales(id) ON DELETE CASCADE,
    reason      TEXT NOT NULL,
    approved_by UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (sale_id)
);

-- One row per client-generated idempotency key. A sale made offline is sent
-- again and again until acknowledged; the key makes sure it exists once.
CREATE TABLE IF NOT EXISTS adrient.pos_sync_log (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    idem_key   TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending',   -- pending | done | failed
    sale_id    UUID,
    error      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, idem_key)
);

CREATE TABLE IF NOT EXISTS adrient.pos_modifier_groups (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    required   BOOLEAN NOT NULL DEFAULT false,
    multi      BOOLEAN NOT NULL DEFAULT false,
    options    JSONB NOT NULL DEFAULT '[]'::jsonb,   -- [{"id","name","price"}]
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pos_modifier_groups_org_idx ON adrient.pos_modifier_groups (org_id);

CREATE TABLE IF NOT EXISTS adrient.pos_product_modifiers (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id UUID NOT NULL,
    group_id   UUID NOT NULL REFERENCES adrient.pos_modifier_groups(id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, group_id)
);

CREATE TABLE IF NOT EXISTS adrient.pos_favourites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    register_id UUID,                      -- NULL = every register
    product_id  UUID NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    color       TEXT NOT NULL DEFAULT '',
    position    INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS pos_favourites_org_idx ON adrient.pos_favourites (org_id, position);

CREATE TABLE IF NOT EXISTS adrient.pos_gift_cards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    code        TEXT NOT NULL,
    initial     NUMERIC(14,2) NOT NULL,
    balance     NUMERIC(14,2) NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',   -- active | disabled
    expires_on  DATE,
    holder_name TEXT NOT NULL DEFAULT '',
    holder_email TEXT NOT NULL DEFAULT '',
    created_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (balance >= 0)
);
CREATE UNIQUE INDEX IF NOT EXISTS pos_gift_cards_code_uq ON adrient.pos_gift_cards (org_id, upper(code));

CREATE TABLE IF NOT EXISTS adrient.pos_gift_card_log (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    card_id    UUID NOT NULL REFERENCES adrient.pos_gift_cards(id) ON DELETE CASCADE,
    delta      NUMERIC(14,2) NOT NULL,
    reason     TEXT NOT NULL,                      -- issue | redeem | topup | adjust
    sale_id    UUID,
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- A sale redeems a card once.
CREATE UNIQUE INDEX IF NOT EXISTS pos_gift_card_log_sale_uq ON adrient.pos_gift_card_log (card_id, sale_id) WHERE reason = 'redeem';

CREATE TABLE IF NOT EXISTS adrient.pos_loyalty_ext (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL,
    stamps     INT NOT NULL DEFAULT 0,
    rewards    INT NOT NULL DEFAULT 0,
    PRIMARY KEY (org_id, contact_id)
);

CREATE TABLE IF NOT EXISTS adrient.pos_stores (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    address        TEXT NOT NULL DEFAULT '',
    phone          TEXT NOT NULL DEFAULT '',
    receipt_header TEXT NOT NULL DEFAULT '',
    receipt_footer TEXT NOT NULL DEFAULT '',
    active         BOOLEAN NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pos_stores_org_idx ON adrient.pos_stores (org_id);

CREATE TABLE IF NOT EXISTS adrient.pos_register_store (
    register_id UUID PRIMARY KEY REFERENCES adrient.pos_registers(id) ON DELETE CASCADE,
    store_id    UUID NOT NULL REFERENCES adrient.pos_stores(id) ON DELETE CASCADE
);
