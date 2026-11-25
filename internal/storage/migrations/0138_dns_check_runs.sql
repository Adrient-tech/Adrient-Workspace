-- 0138: history of DNS checks, used for the DMARC ramp hint (14 clean days).

CREATE TABLE IF NOT EXISTS adrient.dns_check_runs (
    id        BIGSERIAL PRIMARY KEY,
    domain_id UUID NOT NULL REFERENCES adrient.org_domains(id) ON DELETE CASCADE,
    ran_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    pass      INT NOT NULL DEFAULT 0,
    warn      INT NOT NULL DEFAULT 0,
    fail      INT NOT NULL DEFAULT 0,
    clean     BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS dns_check_runs_domain_idx ON adrient.dns_check_runs (domain_id, ran_at DESC);
