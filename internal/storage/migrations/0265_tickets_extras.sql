-- Help desk extras: SLA policies, canned replies, assignment rules, CSAT
-- survey, knowledge base articles, resolved_at. Idempotent.
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS adrient.ticket_sla_policies (
    project_id         UUID NOT NULL REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,
    priority           TEXT NOT NULL,                 -- low | normal | high | urgent
    first_response_min INT  NOT NULL,
    resolve_min        INT  NOT NULL,
    PRIMARY KEY (project_id, priority)
);

CREATE TABLE IF NOT EXISTS adrient.ticket_canned (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    project_id UUID REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,  -- NULL = all projects
    title      TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_canned_org_idx ON adrient.ticket_canned (org_id);

CREATE TABLE IF NOT EXISTS adrient.ticket_rules (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL,
    project_id     UUID NOT NULL REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    position       INT  NOT NULL DEFAULT 0,
    enabled        BOOLEAN NOT NULL DEFAULT true,
    match_keyword  TEXT NOT NULL DEFAULT '',
    match_priority TEXT NOT NULL DEFAULT '',
    match_source   TEXT NOT NULL DEFAULT '',
    set_assignee   UUID,
    set_priority   TEXT NOT NULL DEFAULT '',
    set_status     TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_rules_project_idx ON adrient.ticket_rules (project_id, position);

CREATE TABLE IF NOT EXISTS adrient.ticket_csat (
    ticket_id    UUID PRIMARY KEY REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL,
    token        TEXT NOT NULL UNIQUE,
    rating       INT,
    comment      TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at  TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS adrient.kb_articles (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,
    slug       TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    published  BOOLEAN NOT NULL DEFAULT false,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug)
);
