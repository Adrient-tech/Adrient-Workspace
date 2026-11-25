-- 0201: CRM automation: workflow rules (idempotent runs), playbooks, lead scoring, SLA, merge log.

CREATE TABLE IF NOT EXISTS adrient.crm_rules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT true,
    trigger    TEXT NOT NULL,        -- stage_changed | deal_idle | form_submitted | email_received | lead_created
    conditions JSONB NOT NULL DEFAULT '{}',
    actions    JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS crm_rules_org_idx ON adrient.crm_rules (org_id, trigger) WHERE enabled;

-- One row per (rule, event key): a conflict means the rule already ran for that event.
CREATE TABLE IF NOT EXISTS adrient.crm_rule_runs (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id    UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    rule_id   UUID NOT NULL REFERENCES adrient.crm_rules(id) ON DELETE CASCADE,
    event_key TEXT NOT NULL,
    result    JSONB NOT NULL DEFAULT '[]',
    ran_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (rule_id, event_key)
);

CREATE TABLE IF NOT EXISTS adrient.crm_rr_state (
    org_id UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    key    TEXT NOT NULL,
    idx    BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (org_id, key)
);

CREATE TABLE IF NOT EXISTS adrient.crm_playbooks (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id   UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    stage    TEXT NOT NULL,
    name     TEXT NOT NULL DEFAULT '',
    items    JSONB NOT NULL DEFAULT '[]',
    required BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (org_id, stage)
);

CREATE TABLE IF NOT EXISTS adrient.crm_deal_checklist (
    deal_id  UUID NOT NULL REFERENCES adrient.crm_deals(id) ON DELETE CASCADE,
    stage    TEXT NOT NULL,
    position INT NOT NULL,
    text     TEXT NOT NULL,
    done     BOOLEAN NOT NULL DEFAULT false,
    done_at  TIMESTAMPTZ,
    PRIMARY KEY (deal_id, stage, position)
);

CREATE TABLE IF NOT EXISTS adrient.crm_score_rules (
    id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name   TEXT NOT NULL DEFAULT '',
    field  TEXT NOT NULL,            -- status | source | tag | has_email | has_phone | has_company | activities_30d | custom:<key>
    op     TEXT NOT NULL DEFAULT 'eq',   -- eq | contains | gt | lt | exists
    value  TEXT NOT NULL DEFAULT '',
    points INT NOT NULL DEFAULT 0
);
ALTER TABLE adrient.crm_people ADD COLUMN IF NOT EXISTS score INT NOT NULL DEFAULT 0;

ALTER TABLE adrient.crm_settings ADD COLUMN IF NOT EXISTS sla_lead_hours INT NOT NULL DEFAULT 0;
ALTER TABLE adrient.crm_settings ADD COLUMN IF NOT EXISTS sla_stage_days JSONB NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS adrient.crm_merge_log (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id    UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind      TEXT NOT NULL,         -- person | company
    winner_id UUID NOT NULL,
    loser_id  UUID NOT NULL,
    snapshot  JSONB NOT NULL DEFAULT '{}',
    user_id   UUID,
    at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
