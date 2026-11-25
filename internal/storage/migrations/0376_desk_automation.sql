-- Service desk: SLA rules by priority/type/customer, automations, request forms,
-- change approvals, status definitions, KB categories. Idempotent.

CREATE TABLE IF NOT EXISTS adrient.ticket_sla_rules (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             UUID NOT NULL,
    project_id         UUID NOT NULL REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    position           INT NOT NULL DEFAULT 0,
    enabled            BOOLEAN NOT NULL DEFAULT true,
    match_priority     TEXT NOT NULL DEFAULT '',
    match_type         TEXT NOT NULL DEFAULT '',
    match_customer     TEXT NOT NULL DEFAULT '',     -- requester email domain, e.g. "acme.example"
    first_response_min INT NOT NULL DEFAULT 0,
    resolve_min        INT NOT NULL DEFAULT 0,
    business_hours     BOOLEAN NOT NULL DEFAULT true, -- count only calendar hours
    calendar_id        UUID REFERENCES adrient.ticket_calendars(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_sla_rules_idx ON adrient.ticket_sla_rules (project_id, position);

CREATE TABLE IF NOT EXISTS adrient.ticket_status_defs (
    project_id UUID NOT NULL REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    label      TEXT NOT NULL,
    category   TEXT NOT NULL DEFAULT 'open',          -- open | pending | hold | solved | closed
    pauses_sla BOOLEAN NOT NULL DEFAULT false,
    position   INT NOT NULL DEFAULT 0,
    -- required ticket fields before moving to this status, e.g. {"category","assignee"}
    requires   TEXT[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (project_id, key)
);

CREATE TABLE IF NOT EXISTS adrient.ticket_automations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL DEFAULT 'trigger',       -- trigger | time | dispatch
    event      TEXT NOT NULL DEFAULT 'create',        -- create | update | any (trigger kind)
    match_mode TEXT NOT NULL DEFAULT 'all',           -- all | any
    conditions JSONB NOT NULL DEFAULT '[]'::jsonb,    -- [{"field","op","value"}]
    actions    JSONB NOT NULL DEFAULT '[]'::jsonb,    -- [{"type","value"}]
    position   INT NOT NULL DEFAULT 0,
    enabled    BOOLEAN NOT NULL DEFAULT true,
    run_count  INT NOT NULL DEFAULT 0,
    last_run_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_automations_idx ON adrient.ticket_automations (project_id, position);

CREATE TABLE IF NOT EXISTS adrient.ticket_forms (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    project_id  UUID NOT NULL REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    ticket_type TEXT NOT NULL DEFAULT 'question',
    group_id    UUID,
    category    TEXT NOT NULL DEFAULT '',
    -- [{"key","label","type":"text|longtext|select|date|checkbox","required":bool,"options":[...]}]
    fields      JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    position    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_forms_idx ON adrient.ticket_forms (project_id, position);

CREATE TABLE IF NOT EXISTS adrient.ticket_approvals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    ticket_id   UUID NOT NULL REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    step        INT NOT NULL DEFAULT 1,
    approver_id UUID NOT NULL,
    decision    TEXT NOT NULL DEFAULT 'pending',      -- pending | approved | rejected
    comment     TEXT NOT NULL DEFAULT '',
    decided_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ticket_id, step, approver_id)
);
CREATE INDEX IF NOT EXISTS ticket_approvals_approver_idx ON adrient.ticket_approvals (approver_id, decision);

ALTER TABLE adrient.kb_articles ADD COLUMN IF NOT EXISTS category    TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.kb_articles ADD COLUMN IF NOT EXISTS views       INT NOT NULL DEFAULT 0;
ALTER TABLE adrient.kb_articles ADD COLUMN IF NOT EXISTS helpful_yes INT NOT NULL DEFAULT 0;
ALTER TABLE adrient.kb_articles ADD COLUMN IF NOT EXISTS helpful_no  INT NOT NULL DEFAULT 0;
ALTER TABLE adrient.kb_articles ADD COLUMN IF NOT EXISTS deflected   INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS adrient.ticket_saved_reports (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    params     JSONB NOT NULL DEFAULT '{}'::jsonb,   -- {"days":30,"group":"","type":"","priority":""}
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
