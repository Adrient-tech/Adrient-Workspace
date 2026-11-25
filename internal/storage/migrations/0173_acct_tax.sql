-- Accounting wave 3, part 4: tax rates/codes, GST parties, HSN map, TDS tracking, GSTR-2B uploads. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.acct_tax_rates (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id    UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind      TEXT NOT NULL DEFAULT 'gst',      -- gst | tds
    code      TEXT NOT NULL,                    -- e.g. GST18, 194C
    name      TEXT NOT NULL DEFAULT '',
    rate      NUMERIC(6,3) NOT NULL DEFAULT 0,
    section   TEXT NOT NULL DEFAULT '',         -- TDS section
    active    BOOLEAN NOT NULL DEFAULT true,
    UNIQUE (org_id, kind, code)
);
CREATE TABLE IF NOT EXISTS adrient.acct_gst_parties (
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    contact_id  TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    gstin       TEXT NOT NULL DEFAULT '',
    state_code  TEXT NOT NULL DEFAULT '',
    pan         TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (org_id, contact_id)
);
CREATE TABLE IF NOT EXISTS adrient.acct_hsn_map (
    org_id   UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    match    TEXT NOT NULL,                     -- lower-cased item description
    hsn      TEXT NOT NULL,
    PRIMARY KEY (org_id, match)
);
CREATE TABLE IF NOT EXISTS adrient.acct_tds_entries (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    direction   TEXT NOT NULL,                  -- payable (we deducted) | receivable (customer deducted)
    section     TEXT NOT NULL DEFAULT '',
    party_id    TEXT NOT NULL DEFAULT '',
    party_name  TEXT NOT NULL DEFAULT '',
    pan         TEXT NOT NULL DEFAULT '',
    entry_date  DATE NOT NULL,
    base_amount NUMERIC(16,2) NOT NULL,
    rate        NUMERIC(6,3) NOT NULL DEFAULT 0,
    tds_amount  NUMERIC(16,2) NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending', -- pending | deposited (payable) | claimed (receivable)
    challan     TEXT NOT NULL DEFAULT '',
    settled_on  DATE,
    settle_account_id UUID REFERENCES adrient.acct_accounts(id),
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS acct_tds_org_idx ON adrient.acct_tds_entries (org_id, entry_date);
CREATE TABLE IF NOT EXISTS adrient.acct_gstr2b (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    period     TEXT NOT NULL,                   -- YYYY-MM
    supplier_gstin TEXT NOT NULL DEFAULT '',
    supplier_name  TEXT NOT NULL DEFAULT '',
    invoice_no TEXT NOT NULL DEFAULT '',
    invoice_date DATE,
    taxable    NUMERIC(16,2) NOT NULL DEFAULT 0,
    igst       NUMERIC(16,2) NOT NULL DEFAULT 0,
    cgst       NUMERIC(16,2) NOT NULL DEFAULT 0,
    sgst       NUMERIC(16,2) NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS acct_gstr2b_idx ON adrient.acct_gstr2b (org_id, period);
