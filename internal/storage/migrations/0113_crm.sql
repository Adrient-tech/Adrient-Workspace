-- 0113: CRM (leads, companies, deals pipeline, follow-ups, activity log,
-- buying committee, stage history, MRR subscriptions, forecast settings).
-- People ARE Contacts: crm_people is a CRM profile keyed by contact_id.

CREATE TABLE IF NOT EXISTS adrient.crm_settings (
    org_id         UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    stages         JSONB NOT NULL DEFAULT '[{"id":"new","name":"New"},{"id":"qualified","name":"Qualified"},{"id":"proposal","name":"Proposal"},{"id":"negotiation","name":"Negotiation"}]',
    currency       TEXT NOT NULL DEFAULT 'INR',
    monthly_goal   NUMERIC(14,2) NOT NULL DEFAULT 0,
    stale_days     INT NOT NULL DEFAULT 14,
    cold_days      INT NOT NULL DEFAULT 14,
    weekly_target  INT NOT NULL DEFAULT 0,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.crm_companies (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    industry   TEXT NOT NULL DEFAULT '',
    website    TEXT NOT NULL DEFAULT '',
    phone      TEXT NOT NULL DEFAULT '',
    address    TEXT NOT NULL DEFAULT '',
    notes      TEXT NOT NULL DEFAULT '',
    tags       TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS crm_companies_org_idx ON adrient.crm_companies (org_id);

CREATE TABLE IF NOT EXISTS adrient.crm_people (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES adrient.contacts(id) ON DELETE CASCADE,
    company_id UUID REFERENCES adrient.crm_companies(id) ON DELETE SET NULL,
    status     TEXT NOT NULL DEFAULT 'lead',
    source     TEXT NOT NULL DEFAULT '',
    tags       TEXT[] NOT NULL DEFAULT '{}',
    notes      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, contact_id)
);
CREATE INDEX IF NOT EXISTS crm_people_org_idx ON adrient.crm_people (org_id);

CREATE TABLE IF NOT EXISTS adrient.crm_deals (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    title          TEXT NOT NULL,
    company_id     UUID REFERENCES adrient.crm_companies(id) ON DELETE SET NULL,
    contact_id     UUID REFERENCES adrient.contacts(id) ON DELETE SET NULL,
    stage          TEXT NOT NULL DEFAULT 'new',
    value          NUMERIC(14,2) NOT NULL DEFAULT 0,
    currency       TEXT NOT NULL DEFAULT 'INR',
    billing        TEXT NOT NULL DEFAULT 'one_time',   -- one_time | monthly | quarterly | yearly
    expected_close DATE,
    source         TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    close_reason   TEXT NOT NULL DEFAULT '',
    closed_at      TIMESTAMPTZ,
    stage_entered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS crm_deals_org_idx ON adrient.crm_deals (org_id, stage);

CREATE TABLE IF NOT EXISTS adrient.crm_deal_people (
    deal_id    UUID NOT NULL REFERENCES adrient.crm_deals(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES adrient.contacts(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (deal_id, contact_id)
);

CREATE TABLE IF NOT EXISTS adrient.crm_stage_history (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    deal_id    UUID NOT NULL REFERENCES adrient.crm_deals(id) ON DELETE CASCADE,
    stage      TEXT NOT NULL,
    entered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS crm_stage_history_deal_idx ON adrient.crm_stage_history (deal_id, entered_at);

CREATE TABLE IF NOT EXISTS adrient.crm_followups (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    title             TEXT NOT NULL,
    due_date          DATE NOT NULL,
    due_time          TEXT NOT NULL DEFAULT '',
    deal_id           UUID REFERENCES adrient.crm_deals(id) ON DELETE CASCADE,
    contact_id        UUID REFERENCES adrient.contacts(id) ON DELETE SET NULL,
    done              BOOLEAN NOT NULL DEFAULT false,
    calendar_event_id TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS crm_followups_org_idx ON adrient.crm_followups (org_id, due_date);

CREATE TABLE IF NOT EXISTS adrient.crm_activities (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind              TEXT NOT NULL DEFAULT 'note',   -- call | meeting | email | note
    summary           TEXT NOT NULL DEFAULT '',
    occurred_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    deal_id           UUID REFERENCES adrient.crm_deals(id) ON DELETE CASCADE,
    contact_id        UUID REFERENCES adrient.contacts(id) ON DELETE SET NULL,
    company_id        UUID REFERENCES adrient.crm_companies(id) ON DELETE SET NULL,
    calendar_event_id TEXT NOT NULL DEFAULT '',
    mail_thread_id    TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS crm_activities_org_idx ON adrient.crm_activities (org_id, occurred_at DESC);
