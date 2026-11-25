-- 0114: Accounting + Expenses (double-entry ledger, chart of accounts, bank
-- statements/rules/reconciliation, expenses and expense reports, audit log).
-- Customers/vendors are Contacts (contact_id plus a name snapshot). Journals can be
-- auto-posted from POS and Invoicing, keyed by (source, source_id) so a sync never
-- double-posts.

CREATE TABLE IF NOT EXISTS adrient.acct_settings (
    org_id           UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    business_name    TEXT NOT NULL DEFAULT '',
    gstin            TEXT NOT NULL DEFAULT '',
    currency         TEXT NOT NULL DEFAULT 'INR',
    fy_start_month   INT  NOT NULL DEFAULT 4,
    intrastate       BOOLEAN NOT NULL DEFAULT true,   -- split GST CGST/SGST (true) or IGST (false) on auto-posts
    closing_date     DATE,                            -- books are locked through this date
    pos_synced_at    TIMESTAMPTZ,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.acct_accounts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    code        TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,       -- asset | liability | equity | income | expense
    subtype     TEXT NOT NULL DEFAULT '',  -- bank | cash | receivable | payable | gst_input_cgst ... | fixed_asset | loan | other
    system_key  TEXT NOT NULL DEFAULT '',  -- stable key used by auto-posting
    parent_id   UUID REFERENCES adrient.acct_accounts(id) ON DELETE SET NULL,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS acct_accounts_org_idx ON adrient.acct_accounts (org_id);
CREATE UNIQUE INDEX IF NOT EXISTS acct_accounts_key_idx ON adrient.acct_accounts (org_id, system_key) WHERE system_key <> '';

CREATE TABLE IF NOT EXISTS adrient.acct_journals (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number        BIGINT NOT NULL DEFAULT 0,
    entry_date    DATE NOT NULL,
    due_date      DATE,
    memo          TEXT NOT NULL DEFAULT '',
    ref           TEXT NOT NULL DEFAULT '',
    kind          TEXT NOT NULL DEFAULT 'journal',   -- journal | sale | invoice | payment | expense | bill | bank | reimbursement | transfer
    source        TEXT NOT NULL DEFAULT 'manual',    -- manual | pos_sale | pos_refund | invoice | invoice_payment | expense | expense_payment | bank_line | report
    source_id     TEXT NOT NULL DEFAULT '',
    source_hash   TEXT NOT NULL DEFAULT '',
    contact_id    TEXT NOT NULL DEFAULT '',
    contact_name  TEXT NOT NULL DEFAULT '',
    voided        BOOLEAN NOT NULL DEFAULT false,
    created_by    UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS acct_journals_org_date_idx ON adrient.acct_journals (org_id, entry_date);
CREATE UNIQUE INDEX IF NOT EXISTS acct_journals_source_idx ON adrient.acct_journals (org_id, source, source_id) WHERE source_id <> '';

CREATE TABLE IF NOT EXISTS adrient.acct_lines (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_id  UUID NOT NULL REFERENCES adrient.acct_journals(id) ON DELETE CASCADE,
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    account_id  UUID NOT NULL REFERENCES adrient.acct_accounts(id),
    debit       NUMERIC(16,2) NOT NULL DEFAULT 0,
    credit      NUMERIC(16,2) NOT NULL DEFAULT 0,
    memo        TEXT NOT NULL DEFAULT '',
    position    INT NOT NULL DEFAULT 0,
    reconciliation_id UUID,
    CHECK (debit >= 0 AND credit >= 0)
);
CREATE INDEX IF NOT EXISTS acct_lines_journal_idx ON adrient.acct_lines (journal_id);
CREATE INDEX IF NOT EXISTS acct_lines_account_idx ON adrient.acct_lines (org_id, account_id);

CREATE TABLE IF NOT EXISTS adrient.acct_bank_lines (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    account_id   UUID NOT NULL REFERENCES adrient.acct_accounts(id) ON DELETE CASCADE,
    line_date    DATE NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    amount       NUMERIC(16,2) NOT NULL,           -- + money in, - money out
    hash         TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',  -- pending | added | excluded
    category_id  UUID REFERENCES adrient.acct_accounts(id) ON DELETE SET NULL,
    journal_id   UUID REFERENCES adrient.acct_journals(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS acct_bank_lines_hash_idx ON adrient.acct_bank_lines (org_id, account_id, hash);

CREATE TABLE IF NOT EXISTS adrient.acct_bank_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL DEFAULT '',
    account_id   UUID REFERENCES adrient.acct_accounts(id) ON DELETE CASCADE,  -- NULL = any bank account
    direction    TEXT NOT NULL DEFAULT 'any',      -- in | out | any
    contains     TEXT NOT NULL,
    category_id  UUID NOT NULL REFERENCES adrient.acct_accounts(id) ON DELETE CASCADE,
    priority     INT NOT NULL DEFAULT 100,
    auto_add     BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.acct_reconciliations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    account_id     UUID NOT NULL REFERENCES adrient.acct_accounts(id) ON DELETE CASCADE,
    statement_date DATE NOT NULL,
    ending_balance NUMERIC(16,2) NOT NULL,
    created_by     UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.acct_expense_reports (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    title          TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'draft',  -- draft | submitted | approved | rejected | reimbursed
    submitter_id   UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    submitter_name TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    decision_note  TEXT NOT NULL DEFAULT '',
    reimbursed_at  DATE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.acct_expenses (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    expense_date   DATE NOT NULL,
    vendor_id      TEXT NOT NULL DEFAULT '',
    vendor_name    TEXT NOT NULL DEFAULT '',
    category_id    UUID NOT NULL REFERENCES adrient.acct_accounts(id),
    amount         NUMERIC(16,2) NOT NULL,         -- before GST
    gst_rate       NUMERIC(6,2) NOT NULL DEFAULT 0,
    gst_amount     NUMERIC(16,2) NOT NULL DEFAULT 0,
    total          NUMERIC(16,2) NOT NULL,
    paid_from_id   UUID REFERENCES adrient.acct_accounts(id) ON DELETE SET NULL,  -- NULL = bill (payable) unless out_of_pocket
    out_of_pocket  BOOLEAN NOT NULL DEFAULT false, -- paid by an employee, reimbursed through a report
    paid           BOOLEAN NOT NULL DEFAULT true,
    notes          TEXT NOT NULL DEFAULT '',
    receipts       JSONB NOT NULL DEFAULT '[]'::jsonb,  -- [{id,name}] Drive file ids
    report_id      UUID REFERENCES adrient.acct_expense_reports(id) ON DELETE SET NULL,
    journal_id     UUID REFERENCES adrient.acct_journals(id) ON DELETE SET NULL,
    created_by     UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS acct_expenses_org_idx ON adrient.acct_expenses (org_id, expense_date);

CREATE TABLE IF NOT EXISTS adrient.acct_audit (
    id         BIGSERIAL PRIMARY KEY,
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL DEFAULT '',
    action     TEXT NOT NULL,
    entity     TEXT NOT NULL DEFAULT '',
    entity_id  TEXT NOT NULL DEFAULT '',
    detail     TEXT NOT NULL DEFAULT '',
    at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS acct_audit_org_idx ON adrient.acct_audit (org_id, id DESC);
