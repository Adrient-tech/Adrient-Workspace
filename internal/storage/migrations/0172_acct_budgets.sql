-- Accounting wave 3, part 3: budgets by account and month. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.acct_budgets (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES adrient.acct_accounts(id) ON DELETE CASCADE,
    month      DATE NOT NULL,            -- first day of the month
    amount     NUMERIC(16,2) NOT NULL DEFAULT 0,
    PRIMARY KEY (org_id, account_id, month)
);
