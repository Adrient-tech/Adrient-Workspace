-- 0128: Invitations (identity plan section B3) and expiry reminder log.

CREATE TABLE IF NOT EXISTS adrient.invitations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    user_type   TEXT NOT NULL DEFAULT 'guest'
                CHECK (user_type IN ('member', 'external', 'guest')),
    invited_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    sponsor_id  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    app_ids     TEXT[] NOT NULL DEFAULT '{}',
    group_ids   UUID[] NOT NULL DEFAULT '{}',
    user_expires_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    accepted_by UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    revoked_at  TIMESTAMPTZ,
    secret_hash BYTEA NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS invitations_secret_uq ON adrient.invitations (secret_hash);
CREATE INDEX IF NOT EXISTS invitations_org_idx ON adrient.invitations (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS invitations_email_idx ON adrient.invitations (lower(email))
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS adrient.lifecycle_notices (
    user_id UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    kind    TEXT NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind)
);
