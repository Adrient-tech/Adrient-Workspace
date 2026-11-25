-- Accounting wave 3, part 5: fixed assets and depreciation. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.acct_assets (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    tag            TEXT NOT NULL DEFAULT '',
    category       TEXT NOT NULL DEFAULT '',
    purchase_date  DATE NOT NULL,
    dep_start      DATE NOT NULL,               -- first month depreciated
    cost           NUMERIC(16,2) NOT NULL,
    salvage        NUMERIC(16,2) NOT NULL DEFAULT 0,
    life_months    INT NOT NULL DEFAULT 60,
    method         TEXT NOT NULL DEFAULT 'slm', -- slm | wdv
    wdv_rate       NUMERIC(6,3) NOT NULL DEFAULT 0,   -- annual % for wdv
    asset_account_id   UUID NOT NULL REFERENCES adrient.acct_accounts(id),
    accum_account_id   UUID NOT NULL REFERENCES adrient.acct_accounts(id),
    expense_account_id UUID NOT NULL REFERENCES adrient.acct_accounts(id),
    class          TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'active',    -- active | disposed
    disposed_on    DATE,
    proceeds       NUMERIC(16,2) NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS acct_assets_org_idx ON adrient.acct_assets (org_id, status);

CREATE TABLE IF NOT EXISTS adrient.acct_asset_dep (
    asset_id  UUID NOT NULL REFERENCES adrient.acct_assets(id) ON DELETE CASCADE,
    org_id    UUID NOT NULL,
    period    TEXT NOT NULL,                  -- YYYY-MM
    amount    NUMERIC(16,2) NOT NULL,
    PRIMARY KEY (asset_id, period)
);
