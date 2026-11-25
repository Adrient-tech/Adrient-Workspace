-- Accounting wave 3, part 2: recurring transactions. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.acct_recurring (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,                  -- journal | bill | expense
    name        TEXT NOT NULL,
    frequency   TEXT NOT NULL DEFAULT 'monthly', -- daily | weekly | monthly | quarterly | yearly
    every       INT  NOT NULL DEFAULT 1,
    start_date  DATE NOT NULL,
    end_date    DATE,
    max_runs    INT  NOT NULL DEFAULT 0,        -- 0 = unlimited
    active      BOOLEAN NOT NULL DEFAULT true,
    template    JSONB NOT NULL DEFAULT '{}'::jsonb,
    auto_post   BOOLEAN NOT NULL DEFAULT true,  -- bills: false creates drafts
    last_run    DATE,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS acct_recurring_org_idx ON adrient.acct_recurring (org_id, active);

CREATE TABLE IF NOT EXISTS adrient.acct_recurring_runs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recurring_id  UUID NOT NULL REFERENCES adrient.acct_recurring(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL,
    run_date      DATE NOT NULL,
    status        TEXT NOT NULL DEFAULT 'running',  -- running | done | failed
    result_id     TEXT NOT NULL DEFAULT '',
    error         TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (recurring_id, run_date)
);
