-- Help desk wave 6: tags, ticket type, merge and problem/incident links, event trail,
-- macros and followers.
-- Idempotent.
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS tags        TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS ticket_type TEXT   NOT NULL DEFAULT 'question';
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS merged_into UUID;
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS problem_id  UUID;
CREATE INDEX IF NOT EXISTS tickets_problem_idx ON adrient.tickets (problem_id) WHERE problem_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS tickets_tags_idx ON adrient.tickets USING GIN (tags);

CREATE TABLE IF NOT EXISTS adrient.ticket_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    ticket_id  UUID NOT NULL REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    actor_id   UUID,
    actor_name TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL,                       -- status | priority | assignee | tags | type | merged | merged_into | problem | macro | comment
    detail     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_events_ticket_idx ON adrient.ticket_events (ticket_id, created_at);

CREATE TABLE IF NOT EXISTS adrient.ticket_macros (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    project_id UUID REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,  -- NULL = every project
    name       TEXT NOT NULL,
    actions    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_macros_org_idx ON adrient.ticket_macros (org_id);

CREATE TABLE IF NOT EXISTS adrient.ticket_followers (
    ticket_id  UUID NOT NULL REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    org_id     UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (ticket_id, user_id)
);
