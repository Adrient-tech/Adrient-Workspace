-- Service desk phase 2: inbound mail ledger, custom field definitions, scheduled replies,
-- reply attachments, AI results, skill-required routing. Idempotent.

CREATE TABLE IF NOT EXISTS adrient.ticket_inbound (
    org_id      UUID NOT NULL,
    message_key TEXT NOT NULL,
    ticket_id   UUID REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    outcome     TEXT NOT NULL DEFAULT 'ticket',   -- ticket | comment | ignored
    sender      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, message_key)
);
CREATE INDEX IF NOT EXISTS ticket_inbound_sender_idx ON adrient.ticket_inbound (org_id, sender, created_at DESC);

CREATE TABLE IF NOT EXISTS adrient.ticket_field_defs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    project_id  UUID NOT NULL REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,
    key         TEXT NOT NULL,
    label       TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'text',      -- text | longtext | number | select | date | checkbox
    options     TEXT[] NOT NULL DEFAULT '{}',
    required_on TEXT[] NOT NULL DEFAULT '{}',      -- statuses that need this field filled
    ticket_type TEXT NOT NULL DEFAULT '',          -- '' = every type
    position    INT NOT NULL DEFAULT 0,
    UNIQUE (project_id, key)
);

CREATE TABLE IF NOT EXISTS adrient.ticket_scheduled (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id    UUID NOT NULL,
    ticket_id UUID NOT NULL REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    user_id   UUID NOT NULL,
    user_name TEXT NOT NULL DEFAULT '',
    body      TEXT NOT NULL,
    internal  BOOLEAN NOT NULL DEFAULT false,
    solve     BOOLEAN NOT NULL DEFAULT false,
    send_at   TIMESTAMPTZ NOT NULL,
    state     TEXT NOT NULL DEFAULT 'pending',     -- pending | sent | cancelled | failed
    error     TEXT NOT NULL DEFAULT '',
    attachments JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_scheduled_due_idx ON adrient.ticket_scheduled (send_at) WHERE state = 'pending';

CREATE TABLE IF NOT EXISTS adrient.ticket_attachments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    ticket_id  UUID NOT NULL REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    comment_id UUID,
    file_id    TEXT NOT NULL,                      -- Drive file id
    name       TEXT NOT NULL,
    size       BIGINT NOT NULL DEFAULT 0,
    added_by   UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_attachments_ticket_idx ON adrient.ticket_attachments (ticket_id);

ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS sentiment   TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS ai_summary  TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS need_skills TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE adrient.ticket_forms ADD COLUMN IF NOT EXISTS skills TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE adrient.ticket_groups ADD COLUMN IF NOT EXISTS ooo_message TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS ticket_groups_alias_idx ON adrient.ticket_groups (lower(email_alias)) WHERE email_alias <> '';
