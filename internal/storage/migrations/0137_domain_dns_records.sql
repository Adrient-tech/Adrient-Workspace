-- 0137: last observed state of every planned DNS record per domain.

CREATE TABLE IF NOT EXISTS adrient.domain_dns_records (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id       UUID NOT NULL REFERENCES adrient.org_domains(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,
    name            TEXT NOT NULL,
    rtype           TEXT NOT NULL,
    expected        TEXT NOT NULL DEFAULT '',
    required        BOOLEAN NOT NULL DEFAULT true,
    status          TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('pass', 'warn', 'fail', 'unknown')),
    observed        JSONB NOT NULL DEFAULT '[]',
    note            TEXT NOT NULL DEFAULT '',
    last_checked_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS domain_dns_records_uq ON adrient.domain_dns_records (domain_id, kind, name, rtype);
