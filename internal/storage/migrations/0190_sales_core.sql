-- 0190: Sales side of Invoicing. Estimate versions + public acceptance, sales
-- orders, delivery notes, price lists, discount rules, GST place of supply,
-- credit notes, reminders log, payment links, customer portal, subscriptions.
-- Everything is additive and idempotent.

ALTER TABLE adrient.invc_settings ADD COLUMN IF NOT EXISTS sales JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Per-org counters for the new document numbers (order, delivery, credit, ...).
CREATE TABLE IF NOT EXISTS adrient.invc_sequences (
    org_id   UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name     TEXT NOT NULL,
    next_val INT  NOT NULL DEFAULT 1,
    PRIMARY KEY (org_id, name)
);

-- Payment gateway keys (Razorpay). Env vars override; the secret is never returned by the API.
CREATE TABLE IF NOT EXISTS adrient.invc_gateway (
    org_id                  UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    razorpay_key_id         TEXT NOT NULL DEFAULT '',
    razorpay_key_secret     TEXT NOT NULL DEFAULT '',
    razorpay_webhook_secret TEXT NOT NULL DEFAULT '',
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS valid_until      DATE;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS version          INT NOT NULL DEFAULT 1;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS version_group    UUID;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS accepted_at      TIMESTAMPTZ;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS accepted_name    TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS accepted_ip      TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS accepted_ua      TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS accepted_hash    TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS declined_reason  TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS sales_order_id   UUID;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS credit_for       UUID;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS place_of_supply  TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS supplier_state   TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS cgst_total       NUMERIC(14,2) NOT NULL DEFAULT 0;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS sgst_total       NUMERIC(14,2) NOT NULL DEFAULT 0;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS igst_total       NUMERIC(14,2) NOT NULL DEFAULT 0;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS client_gstin     TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS price_list_id    UUID;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS source_ref       TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS subscription_id  UUID;
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS payment_link_id   TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS payment_link_url  TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.invc_documents ADD COLUMN IF NOT EXISTS recurring_period DATE;

CREATE INDEX IF NOT EXISTS invc_documents_group_idx ON adrient.invc_documents (org_id, version_group);
CREATE INDEX IF NOT EXISTS invc_documents_so_idx ON adrient.invc_documents (org_id, sales_order_id) WHERE sales_order_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS invc_documents_credit_idx ON adrient.invc_documents (org_id, credit_for) WHERE credit_for IS NOT NULL;
-- A recurring template can create at most one invoice per period, however often it runs.
CREATE UNIQUE INDEX IF NOT EXISTS invc_documents_recurring_period_uq
    ON adrient.invc_documents (recurring_id, recurring_period) WHERE recurring_id IS NOT NULL AND recurring_period IS NOT NULL;
-- A payment gateway reference can be recorded once.
CREATE UNIQUE INDEX IF NOT EXISTS invc_payments_ref_uq
    ON adrient.invc_payments (document_id, reference) WHERE method = 'razorpay' AND reference <> '';

-- Reminder log: one row per (document, rule) so scheduled reminders never repeat.
CREATE TABLE IF NOT EXISTS adrient.invc_reminder_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    document_id UUID NOT NULL REFERENCES adrient.invc_documents(id) ON DELETE CASCADE,
    rule_key    TEXT NOT NULL,
    channel     TEXT NOT NULL DEFAULT 'email',
    status      TEXT NOT NULL DEFAULT 'sent',   -- sent | skipped | failed
    detail      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_id, rule_key)
);

-- Customer portal: one unguessable token per customer (contact or client name).
CREATE TABLE IF NOT EXISTS adrient.invc_portal (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    customer_key TEXT NOT NULL,          -- contact id, or name:<lowercase name>
    token      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, customer_key)
);
CREATE UNIQUE INDEX IF NOT EXISTS invc_portal_token_uq ON adrient.invc_portal (token);

-- Price lists: a named list, optionally assigned to customers; rows may be
-- quantity tiers or customer-specific overrides.
CREATE TABLE IF NOT EXISTS adrient.sales_price_lists (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    currency   TEXT NOT NULL DEFAULT 'INR',
    is_default BOOLEAN NOT NULL DEFAULT false,
    active     BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sales_price_lists_org_idx ON adrient.sales_price_lists (org_id);

CREATE TABLE IF NOT EXISTS adrient.sales_price_list_items (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    price_list_id UUID NOT NULL REFERENCES adrient.sales_price_lists(id) ON DELETE CASCADE,
    product_id    TEXT NOT NULL,                         -- Inventory product id or price-list item id
    contact_id    UUID,                                  -- set = customer-specific price
    min_qty       NUMERIC(14,3) NOT NULL DEFAULT 1,
    price         NUMERIC(14,2) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sales_pli_idx ON adrient.sales_price_list_items (org_id, price_list_id, product_id);

CREATE TABLE IF NOT EXISTS adrient.sales_price_list_customers (
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    price_list_id UUID NOT NULL REFERENCES adrient.sales_price_lists(id) ON DELETE CASCADE,
    contact_id    UUID NOT NULL,
    PRIMARY KEY (price_list_id, contact_id)
);

CREATE TABLE IF NOT EXISTS adrient.sales_discount_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    scope       TEXT NOT NULL DEFAULT 'line',          -- line | order
    kind        TEXT NOT NULL DEFAULT 'percent',       -- percent | amount (amount only on order scope)
    value       NUMERIC(14,2) NOT NULL DEFAULT 0,
    min_qty     NUMERIC(14,3) NOT NULL DEFAULT 0,
    min_amount  NUMERIC(14,2) NOT NULL DEFAULT 0,
    product_id  TEXT NOT NULL DEFAULT '',
    contact_id  UUID,
    valid_from  DATE,
    valid_to    DATE,
    priority    INT NOT NULL DEFAULT 0,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sales_discount_rules_org_idx ON adrient.sales_discount_rules (org_id);

-- Sales orders. Delivered and invoiced quantities are derived from the
-- delivery notes and invoices that reference a line, never stored as counters.
CREATE TABLE IF NOT EXISTS adrient.sales_orders (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number         TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'draft',      -- draft | confirmed | cancelled
    contact_id     UUID,
    client_name    TEXT NOT NULL DEFAULT '',
    client_email   TEXT NOT NULL DEFAULT '',
    client_address TEXT NOT NULL DEFAULT '',
    client_gstin   TEXT NOT NULL DEFAULT '',
    ship_to        TEXT NOT NULL DEFAULT '',
    currency       TEXT NOT NULL DEFAULT 'INR',
    order_date     DATE NOT NULL DEFAULT CURRENT_DATE,
    expected_date  DATE,
    place_of_supply TEXT NOT NULL DEFAULT '',
    lines          JSONB NOT NULL DEFAULT '[]'::jsonb,
    discount_kind  TEXT NOT NULL DEFAULT 'percent',
    discount_value NUMERIC(14,2) NOT NULL DEFAULT 0,
    subtotal       NUMERIC(14,2) NOT NULL DEFAULT 0,
    discount_total NUMERIC(14,2) NOT NULL DEFAULT 0,
    tax_total      NUMERIC(14,2) NOT NULL DEFAULT 0,
    total          NUMERIC(14,2) NOT NULL DEFAULT 0,
    notes          TEXT NOT NULL DEFAULT '',
    terms          TEXT NOT NULL DEFAULT '',
    estimate_id    UUID,
    source_ref     TEXT NOT NULL DEFAULT '',
    price_list_id  UUID,
    created_by     UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS sales_orders_org_idx ON adrient.sales_orders (org_id, order_date DESC);
CREATE UNIQUE INDEX IF NOT EXISTS sales_orders_number_uq ON adrient.sales_orders (org_id, number);
CREATE UNIQUE INDEX IF NOT EXISTS sales_orders_estimate_uq ON adrient.sales_orders (org_id, estimate_id) WHERE estimate_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS adrient.sales_delivery_notes (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    order_id     UUID NOT NULL REFERENCES adrient.sales_orders(id) ON DELETE CASCADE,
    number       TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'draft',        -- draft | dispatched | cancelled
    lines        JSONB NOT NULL DEFAULT '[]'::jsonb,   -- [{line_ref, qty, ...}]
    ship_to      TEXT NOT NULL DEFAULT '',
    carrier      TEXT NOT NULL DEFAULT '',
    tracking     TEXT NOT NULL DEFAULT '',
    notes        TEXT NOT NULL DEFAULT '',
    stock_moved  BOOLEAN NOT NULL DEFAULT false,
    delivery_date DATE,
    created_by   UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sales_dn_order_idx ON adrient.sales_delivery_notes (org_id, order_id);
CREATE UNIQUE INDEX IF NOT EXISTS sales_dn_number_uq ON adrient.sales_delivery_notes (org_id, number);

-- Subscriptions / memberships billed from plans.
CREATE TABLE IF NOT EXISTS adrient.sales_plans (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    interval   TEXT NOT NULL DEFAULT 'monthly',        -- weekly | monthly | quarterly | yearly
    price      NUMERIC(14,2) NOT NULL DEFAULT 0,
    tax_rate   NUMERIC(7,3) NOT NULL DEFAULT 0,
    hsn_sac    TEXT NOT NULL DEFAULT '',
    currency   TEXT NOT NULL DEFAULT 'INR',
    active     BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sales_plans_org_idx ON adrient.sales_plans (org_id);

CREATE TABLE IF NOT EXISTS adrient.sales_subscriptions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    plan_id        UUID NOT NULL REFERENCES adrient.sales_plans(id) ON DELETE RESTRICT,
    contact_id     UUID,
    client_name    TEXT NOT NULL DEFAULT '',
    client_email   TEXT NOT NULL DEFAULT '',
    place_of_supply TEXT NOT NULL DEFAULT '',
    quantity       NUMERIC(14,3) NOT NULL DEFAULT 1,
    status         TEXT NOT NULL DEFAULT 'active',     -- active | past_due | paused | cancelled
    start_date     DATE NOT NULL DEFAULT CURRENT_DATE,
    next_renewal   DATE NOT NULL DEFAULT CURRENT_DATE,
    end_date       DATE,
    cancel_at_period_end BOOLEAN NOT NULL DEFAULT false,
    dunning_days   INT NOT NULL DEFAULT 7,             -- past due this long => paused
    renewals       INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sales_subs_org_idx ON adrient.sales_subscriptions (org_id, status);
-- One invoice per subscription period.
CREATE UNIQUE INDEX IF NOT EXISTS invc_documents_subscription_period_uq
    ON adrient.invc_documents (subscription_id, recurring_period) WHERE subscription_id IS NOT NULL AND recurring_period IS NOT NULL;
