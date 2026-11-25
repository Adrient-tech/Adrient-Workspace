-- 0211: SCIM 2.0 provisioning (Entra ID, Okta, Google). Tokens are stored as
-- SHA-256 hashes; the plaintext is shown once when created.
CREATE TABLE IF NOT EXISTS adrient.scim_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL,
    token_hash   TEXT NOT NULL,
    rate_per_min INT  NOT NULL DEFAULT 600 CHECK (rate_per_min BETWEEN 10 AND 100000),
    created_by   UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS scim_tokens_hash_uq ON adrient.scim_tokens (token_hash);
CREATE INDEX IF NOT EXISTS scim_tokens_org_idx ON adrient.scim_tokens (org_id) WHERE revoked_at IS NULL;

ALTER TABLE adrient.users  ADD COLUMN IF NOT EXISTS scim_external_id TEXT;
ALTER TABLE adrient.users  ADD COLUMN IF NOT EXISTS scim_managed BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE adrient.groups ADD COLUMN IF NOT EXISTS scim_external_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS users_scim_ext_uq  ON adrient.users  (org_id, scim_external_id) WHERE scim_external_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS groups_scim_ext_uq ON adrient.groups (org_id, scim_external_id) WHERE scim_external_id IS NOT NULL;

-- Request log for the admin console (kept short by a periodic trim).
CREATE TABLE IF NOT EXISTS adrient.scim_events (
    id         BIGSERIAL PRIMARY KEY,
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    token_id   UUID,
    method     TEXT NOT NULL,
    path       TEXT NOT NULL,
    status     INT  NOT NULL,
    detail     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS scim_events_org_idx ON adrient.scim_events (org_id, id DESC);
