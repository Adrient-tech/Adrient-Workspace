-- 0112: Invoicing (estimates, invoices, payments, recurring templates, price
-- list, per-org settings and branding). Clients are Contacts (contact_id plus a
-- snapshot of name/email/address); products come from Inventory (product_id on
-- a line is optional).

CREATE TABLE IF NOT EXISTS adrient.invc_settings (
    org_id            UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    business_name     TEXT NOT NULL DEFAULT '',
    address           TEXT NOT NULL DEFAULT '',
    tax_id            TEXT NOT NULL DEFAULT '',
    email             TEXT NOT NULL DEFAULT '',
    phone             TEXT NOT NULL DEFAULT '',
    logo_url          TEXT NOT NULL DEFAULT '',
    invoice_prefix    TEXT NOT NULL DEFAULT 'INV-',
    estimate_prefix   TEXT NOT NULL DEFAULT 'EST-',
    next_invoice      INT  NOT NULL DEFAULT 1,
    next_estimate     INT  NOT NULL DEFAULT 1,
    default_currency  TEXT NOT NULL DEFAULT 'INR',
    due_days          INT  NOT NULL DEFAULT 14,
    default_notes     TEXT NOT NULL DEFAULT '',
    default_terms     TEXT NOT NULL DEFAULT '',
    tax_note          TEXT NOT NULL DEFAULT '',
    payment_info      TEXT NOT NULL DEFAULT '',
    units             TEXT[] NOT NULL DEFAULT ARRAY['pcs','hrs','days','kg','month'],
    rates             JSONB NOT NULL DEFAULT '{}'::jsonb,
    deduct_stock      BOOLEAN NOT NULL DEFAULT false,
    reminder_days     INT NOT NULL DEFAULT 3,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.invc_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    unit        TEXT NOT NULL DEFAULT '',
    price       NUMERIC(14,2) NOT NULL DEFAULT 0,
    tax_rate    NUMERIC(7,3) NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS invc_items_org_idx ON adrient.invc_items (org_id);

CREATE TABLE IF NOT EXISTS adrient.invc_documents (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL DEFAULT 'invoice',   -- invoice | estimate
    number         TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'draft',     -- draft | sent | accepted | declined | converted
    contact_id     UUID,
    client_name    TEXT NOT NULL DEFAULT '',
    client_email   TEXT NOT NULL DEFAULT '',
    client_address TEXT NOT NULL DEFAULT '',
    currency       TEXT NOT NULL DEFAULT 'INR',
    issue_date     DATE NOT NULL DEFAULT CURRENT_DATE,
    due_date       DATE,
    lines          JSONB NOT NULL DEFAULT '[]'::jsonb,
    discount_kind  TEXT NOT NULL DEFAULT 'percent',   -- percent | amount
    discount_value NUMERIC(14,2) NOT NULL DEFAULT 0,
    subtotal       NUMERIC(14,2) NOT NULL DEFAULT 0,
    discount_total NUMERIC(14,2) NOT NULL DEFAULT 0,
    tax_total      NUMERIC(14,2) NOT NULL DEFAULT 0,
    total          NUMERIC(14,2) NOT NULL DEFAULT 0,
    notes          TEXT NOT NULL DEFAULT '',
    terms          TEXT NOT NULL DEFAULT '',
    tax_note       TEXT NOT NULL DEFAULT '',
    schedule       JSONB NOT NULL DEFAULT '[]'::jsonb,
    attachments    JSONB NOT NULL DEFAULT '[]'::jsonb,
    share_token    TEXT NOT NULL,
    recurring_id   UUID,
    converted_from UUID,
    stock_deducted BOOLEAN NOT NULL DEFAULT false,
    sent_at        TIMESTAMPTZ,
    viewed_at      TIMESTAMPTZ,
    last_reminder_at TIMESTAMPTZ,
    reminders_sent INT NOT NULL DEFAULT 0,
    deleted_at     TIMESTAMPTZ,
    created_by     UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS invc_documents_org_idx ON adrient.invc_documents (org_id, kind, issue_date DESC);
CREATE INDEX IF NOT EXISTS invc_documents_contact_idx ON adrient.invc_documents (org_id, contact_id);
CREATE UNIQUE INDEX IF NOT EXISTS invc_documents_token_uq ON adrient.invc_documents (share_token);

-- Payments, refunds and write-offs against an invoice.
CREATE TABLE IF NOT EXISTS adrient.invc_payments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    document_id UUID NOT NULL REFERENCES adrient.invc_documents(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL DEFAULT 'payment',      -- payment | refund | writeoff
    amount      NUMERIC(14,2) NOT NULL,
    method      TEXT NOT NULL DEFAULT '',
    reference   TEXT NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    paid_at     DATE NOT NULL DEFAULT CURRENT_DATE,
    created_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS invc_payments_doc_idx ON adrient.invc_payments (document_id);
CREATE INDEX IF NOT EXISTS invc_payments_org_idx ON adrient.invc_payments (org_id, paid_at DESC);

-- Recurring invoice templates; the template body is a document payload.
CREATE TABLE IF NOT EXISTS adrient.invc_recurring (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL DEFAULT '',
    frequency   TEXT NOT NULL DEFAULT 'monthly',      -- weekly | monthly | quarterly | yearly
    next_run    DATE NOT NULL DEFAULT CURRENT_DATE,
    end_date    DATE,
    active      BOOLEAN NOT NULL DEFAULT true,
    auto_send   BOOLEAN NOT NULL DEFAULT false,
    template    JSONB NOT NULL DEFAULT '{}'::jsonb,
    generated   INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS invc_recurring_org_idx ON adrient.invc_recurring (org_id);
