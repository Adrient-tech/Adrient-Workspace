-- 0136: DKIM keys (private half encrypted with the server key) and per-domain
-- mail settings (DNS/email plan section F).

CREATE TABLE IF NOT EXISTS adrient.dkim_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id       UUID NOT NULL REFERENCES adrient.org_domains(id) ON DELETE CASCADE,
    selector        TEXT NOT NULL,
    algo            TEXT NOT NULL DEFAULT 'rsa-2048' CHECK (algo IN ('rsa-2048', 'ed25519')),
    public_key      TEXT NOT NULL,
    private_key_enc BYTEA NOT NULL,
    state           TEXT NOT NULL DEFAULT 'next' CHECK (state IN ('active', 'next', 'retired')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at    TIMESTAMPTZ,
    rotated_at      TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS dkim_keys_selector_uq ON adrient.dkim_keys (domain_id, selector);
CREATE UNIQUE INDEX IF NOT EXISTS dkim_keys_active_uq ON adrient.dkim_keys (domain_id) WHERE state = 'active';
CREATE UNIQUE INDEX IF NOT EXISTS dkim_keys_next_uq ON adrient.dkim_keys (domain_id) WHERE state = 'next';

CREATE TABLE IF NOT EXISTS adrient.mail_domain_settings (
    domain_id     UUID PRIMARY KEY REFERENCES adrient.org_domains(id) ON DELETE CASCADE,
    dmarc_policy  TEXT NOT NULL DEFAULT 'none' CHECK (dmarc_policy IN ('none', 'quarantine', 'reject')),
    dmarc_rua     TEXT NOT NULL DEFAULT '',
    mta_sts_mode  TEXT NOT NULL DEFAULT 'testing' CHECK (mta_sts_mode IN ('none', 'testing', 'enforce')),
    mta_sts_id    TEXT NOT NULL DEFAULT '',
    tls_rpt_rua   TEXT NOT NULL DEFAULT '',
    mail_host     TEXT NOT NULL DEFAULT '',
    spf_ips       TEXT[] NOT NULL DEFAULT '{}',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
