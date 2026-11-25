-- 0200: CRM custom fields, server-side saved views, activity metadata.
-- Custom values live in a JSONB "custom" column on each record (keyed by field key).
-- Entities: lead, contact (both stored on crm_people), company, deal.

CREATE TABLE IF NOT EXISTS adrient.crm_field_defs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    entity     TEXT NOT NULL,                       -- lead | contact | company | deal
    key        TEXT NOT NULL,
    label      TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT 'text',        -- text | number | date | select | multiselect | url | user
    options    JSONB NOT NULL DEFAULT '[]',
    required   BOOLEAN NOT NULL DEFAULT false,
    position   INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, entity, key)
);

ALTER TABLE adrient.crm_people    ADD COLUMN IF NOT EXISTS custom JSONB NOT NULL DEFAULT '{}';
ALTER TABLE adrient.crm_companies ADD COLUMN IF NOT EXISTS custom JSONB NOT NULL DEFAULT '{}';
ALTER TABLE adrient.crm_deals     ADD COLUMN IF NOT EXISTS custom JSONB NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS adrient.crm_views (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    owner_id   UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    entity     TEXT NOT NULL,                        -- lead | company | deal | activity
    name       TEXT NOT NULL,
    config     JSONB NOT NULL DEFAULT '{}',          -- {filters:{}, sort:"", columns:[]}
    shared     BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS crm_views_org_idx ON adrient.crm_views (org_id, entity);

ALTER TABLE adrient.crm_activities ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES adrient.users(id) ON DELETE SET NULL;
ALTER TABLE adrient.crm_activities ADD COLUMN IF NOT EXISTS direction TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.crm_activities ADD COLUMN IF NOT EXISTS duration_sec INT NOT NULL DEFAULT 0;
ALTER TABLE adrient.crm_activities ADD COLUMN IF NOT EXISTS meet_url TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.crm_followups  ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES adrient.users(id) ON DELETE SET NULL;
