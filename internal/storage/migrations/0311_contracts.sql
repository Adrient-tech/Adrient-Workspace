-- 0311: Contracts (renewals, obligations, documents, deadline links). Idempotent.

CREATE TABLE IF NOT EXISTS adrient.contracts_contracts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'other',
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('draft','active','terminated')),
    party_name  TEXT NOT NULL DEFAULT '',
    contact_id  UUID,
    value       NUMERIC(16,2) NOT NULL DEFAULT 0,
    currency    TEXT NOT NULL DEFAULT 'INR',
    billing     TEXT NOT NULL DEFAULT 'one-time',
    start_date  DATE,
    end_date    DATE,
    auto_renew  BOOLEAN NOT NULL DEFAULT false,
    term_months INT NOT NULL DEFAULT 12,
    notice_days INT NOT NULL DEFAULT 0,
    owner_id    UUID NOT NULL,
    access      TEXT NOT NULL DEFAULT 'org' CHECK (access IN ('org','restricted')),
    shared_with UUID[] NOT NULL DEFAULT '{}',
    remind      BOOLEAN NOT NULL DEFAULT true,
    tags        TEXT[] NOT NULL DEFAULT '{}',
    notes       TEXT NOT NULL DEFAULT '',
    decision    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS contracts_contracts_org_idx ON adrient.contracts_contracts (org_id, status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS contracts_contracts_end_idx ON adrient.contracts_contracts (org_id, end_date) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS adrient.contracts_obligations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    contract_id UUID NOT NULL REFERENCES adrient.contracts_contracts(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    owner_side  TEXT NOT NULL DEFAULT 'us' CHECK (owner_side IN ('us','them','mutual')),
    due         DATE,
    done        BOOLEAN NOT NULL DEFAULT false,
    repeat      JSONB,
    history     JSONB NOT NULL DEFAULT '[]',
    assignee_id UUID,
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS contracts_obligations_idx ON adrient.contracts_obligations (contract_id);
CREATE INDEX IF NOT EXISTS contracts_obligations_due_idx ON adrient.contracts_obligations (org_id, due) WHERE done = false;

CREATE TABLE IF NOT EXISTS adrient.contracts_documents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    contract_id UUID NOT NULL REFERENCES adrient.contracts_contracts(id) ON DELETE CASCADE,
    file_id     UUID NOT NULL,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'agreement',
    added_by    UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS contracts_documents_idx ON adrient.contracts_documents (contract_id);

-- One row per Calendar event or Task a contract created, so reminders are never duplicated.
CREATE TABLE IF NOT EXISTS adrient.contracts_links (
    org_id      UUID NOT NULL,
    contract_id UUID NOT NULL REFERENCES adrient.contracts_contracts(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,   -- term_end | notice | obligation
    ref         TEXT NOT NULL,   -- the date for term_end/notice, the obligation id for obligation
    app         TEXT NOT NULL CHECK (app IN ('calendar','tasks')),
    external_id UUID NOT NULL,
    due         DATE,
    PRIMARY KEY (contract_id, kind, ref, app)
);
