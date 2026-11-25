-- 0370: identity operations. Richer sign-in events, org thresholds and IP/country rules.
ALTER TABLE adrient.login_events ADD COLUMN IF NOT EXISTS country    TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.login_events ADD COLUMN IF NOT EXISTS city       TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.login_events ADD COLUMN IF NOT EXISTS device_fp  TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.login_events ADD COLUMN IF NOT EXISTS mfa_result TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.login_events ADD COLUMN IF NOT EXISTS source     TEXT NOT NULL DEFAULT 'app';
CREATE INDEX IF NOT EXISTS login_events_ip_idx    ON adrient.login_events (ip, id DESC);
CREATE INDEX IF NOT EXISTS login_events_email_idx ON adrient.login_events (lower(email), id DESC);
CREATE INDEX IF NOT EXISTS login_events_time_idx  ON adrient.login_events (org_id, created_at DESC);

CREATE TABLE IF NOT EXISTS adrient.iam_settings (
    org_id           UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    thresholds       JSONB NOT NULL DEFAULT '{}',
    anon_networks    TEXT[] NOT NULL DEFAULT '{}',
    security_mailbox TEXT NOT NULL DEFAULT '',
    webhook_url      TEXT NOT NULL DEFAULT '',
    notify_manager   BOOLEAN NOT NULL DEFAULT false,
    mfa_policy       JSONB NOT NULL DEFAULT '{}',
    require_approval TEXT[] NOT NULL DEFAULT '{}',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Allow or block lists, with optional expiry. kind: allow|block, scope: ip|cidr|country.
CREATE TABLE IF NOT EXISTS adrient.iam_ip_rules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('allow','block')),
    scope      TEXT NOT NULL CHECK (scope IN ('ip','cidr','country')),
    value      TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS iam_ip_rules_org_idx ON adrient.iam_ip_rules (org_id, kind);
