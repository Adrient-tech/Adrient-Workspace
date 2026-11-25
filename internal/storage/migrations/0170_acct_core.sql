-- Accounting wave 3, part 1: multi-currency, classes, bills, credit memos, deposits.
-- Idempotent.

ALTER TABLE adrient.acct_journals ADD COLUMN IF NOT EXISTS currency TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.acct_journals ADD COLUMN IF NOT EXISTS fx_rate NUMERIC(18,8) NOT NULL DEFAULT 1;
ALTER TABLE adrient.acct_lines ADD COLUMN IF NOT EXISTS class TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.acct_lines ADD COLUMN IF NOT EXISTS fcy_debit NUMERIC(16,2) NOT NULL DEFAULT 0;
ALTER TABLE adrient.acct_lines ADD COLUMN IF NOT EXISTS fcy_credit NUMERIC(16,2) NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS acct_lines_class_idx ON adrient.acct_lines (org_id, class) WHERE class <> '';

ALTER TABLE adrient.acct_settings ADD COLUMN IF NOT EXISTS use_undeposited BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE adrient.acct_settings ADD COLUMN IF NOT EXISTS approval_limit NUMERIC(16,2) NOT NULL DEFAULT 0; -- bills above this need approval; 0 = off
ALTER TABLE adrient.acct_settings ADD COLUMN IF NOT EXISTS fx_auto_fetch BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS adrient.acct_fx_rates (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    currency   TEXT NOT NULL,                 -- the foreign currency
    rate_date  DATE NOT NULL,
    rate       NUMERIC(18,8) NOT NULL CHECK (rate > 0),  -- base currency per 1 unit of foreign currency
    source     TEXT NOT NULL DEFAULT 'manual',           -- manual | fetched
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, currency, rate_date)
);

-- Vendor bills and vendor credit notes (kind = bill | vendor_credit). Debit notes are bills of kind vendor_credit.
CREATE TABLE IF NOT EXISTS adrient.acct_bills (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind          TEXT NOT NULL DEFAULT 'bill',
    number        TEXT NOT NULL DEFAULT '',
    vendor_id     TEXT NOT NULL DEFAULT '',
    vendor_name   TEXT NOT NULL DEFAULT '',
    bill_date     DATE NOT NULL,
    due_date      DATE,
    currency      TEXT NOT NULL DEFAULT '',   -- '' = base
    fx_rate       NUMERIC(18,8) NOT NULL DEFAULT 1,
    subtotal      NUMERIC(16,2) NOT NULL DEFAULT 0,   -- document currency
    tax_total     NUMERIC(16,2) NOT NULL DEFAULT 0,
    total         NUMERIC(16,2) NOT NULL DEFAULT 0,
    paid          NUMERIC(16,2) NOT NULL DEFAULT 0,   -- payments + credits applied, document currency
    status        TEXT NOT NULL DEFAULT 'open',       -- draft | pending_approval | open | paid | void | rejected
    approved_by   TEXT NOT NULL DEFAULT '',
    approval_note TEXT NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT '',
    attachments   JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS acct_bills_org_idx ON adrient.acct_bills (org_id, bill_date);

CREATE TABLE IF NOT EXISTS adrient.acct_bill_lines (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bill_id     UUID NOT NULL REFERENCES adrient.acct_bills(id) ON DELETE CASCADE,
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    account_id  UUID NOT NULL REFERENCES adrient.acct_accounts(id),
    description TEXT NOT NULL DEFAULT '',
    hsn         TEXT NOT NULL DEFAULT '',
    amount      NUMERIC(16,2) NOT NULL,         -- before tax, document currency
    tax_rate    NUMERIC(6,2) NOT NULL DEFAULT 0,
    tax_amount  NUMERIC(16,2) NOT NULL DEFAULT 0,
    class       TEXT NOT NULL DEFAULT '',
    position    INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS acct_bill_lines_bill_idx ON adrient.acct_bill_lines (bill_id);

-- Payments against a bill, and credits applied to a bill (credit_id set).
CREATE TABLE IF NOT EXISTS adrient.acct_bill_payments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    bill_id     UUID NOT NULL REFERENCES adrient.acct_bills(id) ON DELETE CASCADE,
    credit_id   UUID REFERENCES adrient.acct_bills(id) ON DELETE CASCADE,   -- vendor credit applied instead of cash
    pay_date    DATE NOT NULL,
    amount      NUMERIC(16,2) NOT NULL CHECK (amount > 0),   -- document currency
    account_id  UUID REFERENCES adrient.acct_accounts(id),   -- bank/cash paid from (NULL for a credit application)
    fx_rate     NUMERIC(18,8) NOT NULL DEFAULT 1,
    reference   TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS acct_bill_payments_bill_idx ON adrient.acct_bill_payments (bill_id);

-- Customer credit memos.
CREATE TABLE IF NOT EXISTS adrient.acct_credit_memos (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number        TEXT NOT NULL DEFAULT '',
    customer_id   TEXT NOT NULL DEFAULT '',
    customer_name TEXT NOT NULL DEFAULT '',
    memo_date     DATE NOT NULL,
    currency      TEXT NOT NULL DEFAULT '',
    fx_rate       NUMERIC(18,8) NOT NULL DEFAULT 1,
    account_id    UUID NOT NULL REFERENCES adrient.acct_accounts(id),   -- revenue (usually Sales Returns)
    amount        NUMERIC(16,2) NOT NULL,       -- before GST
    gst_rate      NUMERIC(6,2) NOT NULL DEFAULT 0,
    tax_total     NUMERIC(16,2) NOT NULL DEFAULT 0,
    total         NUMERIC(16,2) NOT NULL,
    refunded      NUMERIC(16,2) NOT NULL DEFAULT 0,
    reason        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'open',  -- open | void
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Deposits move receipts from Undeposited Funds to a bank account.
CREATE TABLE IF NOT EXISTS adrient.acct_deposits (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    deposit_date DATE NOT NULL,
    account_id   UUID NOT NULL REFERENCES adrient.acct_accounts(id),
    total        NUMERIC(16,2) NOT NULL,
    reference    TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS adrient.acct_deposit_items (
    deposit_id UUID NOT NULL REFERENCES adrient.acct_deposits(id) ON DELETE CASCADE,
    line_id    UUID NOT NULL REFERENCES adrient.acct_lines(id) ON DELETE CASCADE,
    PRIMARY KEY (deposit_id, line_id),
    UNIQUE (line_id)
);
