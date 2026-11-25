-- 0214: offboarding and lifecycle: archive, scheduled deletion with a restore
-- window, legal holds, retention.
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS purge_after TIMESTAMPTZ;
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS offboard_note TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS adrient.legal_holds (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    org_wide    BOOLEAN NOT NULL DEFAULT false,
    user_ids    UUID[] NOT NULL DEFAULT '{}',
    created_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    released_at TIMESTAMPTZ,
    released_by UUID REFERENCES adrient.users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS legal_holds_org_idx ON adrient.legal_holds (org_id) WHERE released_at IS NULL;

CREATE TABLE IF NOT EXISTS adrient.retention_settings (
    org_id               UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    restore_window_days  INT NOT NULL DEFAULT 30 CHECK (restore_window_days BETWEEN 0 AND 3650),
    audit_retention_days INT CHECK (audit_retention_days IS NULL OR audit_retention_days >= 30),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.offboard_jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('suspend','unsuspend','archive','delete','restore','purge')),
    transfer_to UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    options     JSONB NOT NULL DEFAULT '{}'::jsonb,
    result      JSONB NOT NULL DEFAULT '{}'::jsonb,
    status      TEXT NOT NULL DEFAULT 'done' CHECK (status IN ('done','failed','refused')),
    note        TEXT NOT NULL DEFAULT '',
    started_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS offboard_jobs_user_idx ON adrient.offboard_jobs (org_id, user_id, created_at DESC);
