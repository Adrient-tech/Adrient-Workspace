-- 0126: Verified org domains, org sign-up settings and user principal names
-- (identity plan section A). One org owns many domains; a domain is claimed by
-- at most one org at a time (pending or verified).

CREATE TABLE IF NOT EXISTS adrient.org_domains (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    domain          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'verified', 'failed', 'removed')),
    verify_txt      TEXT NOT NULL,
    verified_at     TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ,
    is_primary      BOOLEAN NOT NULL DEFAULT false,
    purpose         TEXT[] NOT NULL DEFAULT '{signin,mail}',
    allow_signup    BOOLEAN NOT NULL DEFAULT false,
    auto_join       BOOLEAN NOT NULL DEFAULT false,
    require_approval BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS org_domains_claim_uq
    ON adrient.org_domains (domain) WHERE status IN ('pending', 'verified');
CREATE UNIQUE INDEX IF NOT EXISTS org_domains_primary_uq
    ON adrient.org_domains (org_id) WHERE is_primary;
CREATE INDEX IF NOT EXISTS org_domains_org_idx ON adrient.org_domains (org_id);

CREATE TABLE IF NOT EXISTS adrient.org_settings (
    org_id                 UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    signup_mode            TEXT NOT NULL DEFAULT 'invite_only'
                           CHECK (signup_mode IN ('invite_only', 'domain_auto_join', 'domain_approval', 'open')),
    allowed_signin_domains TEXT[] NOT NULL DEFAULT '{}',
    personal_orgs_allowed  BOOLEAN NOT NULL DEFAULT true,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS upn TEXT;
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS home_domain_id UUID
    REFERENCES adrient.org_domains(id) ON DELETE SET NULL;

-- Backfill: UPN = lower(email); the oldest row wins when an email repeats in an org.
UPDATE adrient.users u SET upn = lower(u.email)
FROM (
    SELECT id, row_number() OVER (PARTITION BY org_id, lower(email) ORDER BY created_at, id) AS rn
    FROM adrient.users WHERE email <> ''
) d
WHERE u.id = d.id AND d.rn = 1 AND u.upn IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS users_org_upn_uq ON adrient.users (org_id, upn) WHERE upn IS NOT NULL;
