-- 0217: e-mail notifications to admins, and the org notification hook.
CREATE TABLE IF NOT EXISTS adrient.admin_alert_settings (
    org_id     UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    recipients TEXT[] NOT NULL DEFAULT '{}',
    events     TEXT[] NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Hook sources: other apps (or outside systems) that post notifications into
-- people's feeds. The token is stored hashed.
CREATE TABLE IF NOT EXISTS adrient.notify_sources (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    app          TEXT NOT NULL,
    prefix       TEXT NOT NULL,
    token_hash   TEXT NOT NULL,
    created_by   UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS notify_sources_hash_uq ON adrient.notify_sources (token_hash);

-- Which app an existing notification came from, so the center can group them.
ALTER TABLE adrient.notifications ADD COLUMN IF NOT EXISTS app TEXT NOT NULL DEFAULT '';
