-- 0203: sales teams, territories, ownership and visibility, quotas.

CREATE TABLE IF NOT EXISTS adrient.crm_teams (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    manager_id UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, name)
);
CREATE TABLE IF NOT EXISTS adrient.crm_team_members (
    team_id UUID NOT NULL REFERENCES adrient.crm_teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    PRIMARY KEY (team_id, user_id)
);
CREATE INDEX IF NOT EXISTS crm_team_members_user_idx ON adrient.crm_team_members (user_id);

CREATE TABLE IF NOT EXISTS adrient.crm_territories (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id    UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    team_id   UUID REFERENCES adrient.crm_teams(id) ON DELETE SET NULL,
    match     JSONB NOT NULL DEFAULT '{}',      -- {industries:[], keywords:[]} matched against company
    UNIQUE (org_id, name)
);

ALTER TABLE adrient.crm_people    ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES adrient.users(id) ON DELETE SET NULL;
ALTER TABLE adrient.crm_people    ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES adrient.users(id) ON DELETE SET NULL;
ALTER TABLE adrient.crm_companies ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES adrient.users(id) ON DELETE SET NULL;
ALTER TABLE adrient.crm_companies ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES adrient.users(id) ON DELETE SET NULL;
ALTER TABLE adrient.crm_companies ADD COLUMN IF NOT EXISTS territory_id UUID REFERENCES adrient.crm_territories(id) ON DELETE SET NULL;
ALTER TABLE adrient.crm_deals     ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES adrient.users(id) ON DELETE SET NULL;
ALTER TABLE adrient.crm_deals     ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES adrient.users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS crm_deals_owner_idx ON adrient.crm_deals (org_id, owner_id);
CREATE INDEX IF NOT EXISTS crm_people_owner_idx ON adrient.crm_people (org_id, owner_id);

-- Per-user CRM settings. scope: own | team | all (admins always see all).
CREATE TABLE IF NOT EXISTS adrient.crm_user_scope (
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    scope         TEXT NOT NULL DEFAULT 'all',
    auto_log_mail BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (org_id, user_id)
);
ALTER TABLE adrient.crm_settings ADD COLUMN IF NOT EXISTS default_scope TEXT NOT NULL DEFAULT 'all';

CREATE TABLE IF NOT EXISTS adrient.crm_quotas (
    org_id  UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    period  DATE NOT NULL,                       -- first day of month
    target  NUMERIC(14,2) NOT NULL DEFAULT 0,
    PRIMARY KEY (org_id, user_id, period)
);
