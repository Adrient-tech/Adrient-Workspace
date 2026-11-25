-- Service desk core: groups and routing, business-hours calendars, richer ticket fields,
-- time entries, reply drafts, presence (collision detection). Idempotent.

CREATE TABLE IF NOT EXISTS adrient.ticket_calendars (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    name       TEXT NOT NULL,
    tz         TEXT NOT NULL DEFAULT 'Asia/Kolkata',
    -- {"mon":[["09:00","18:00"]],"tue":[...],...}; a missing or empty day is closed
    hours      JSONB NOT NULL DEFAULT '{"mon":[["09:00","18:00"]],"tue":[["09:00","18:00"]],"wed":[["09:00","18:00"]],"thu":[["09:00","18:00"]],"fri":[["09:00","18:00"]]}'::jsonb,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_calendars_org_idx ON adrient.ticket_calendars (org_id);

CREATE TABLE IF NOT EXISTS adrient.ticket_holidays (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    calendar_id UUID NOT NULL REFERENCES adrient.ticket_calendars(id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    UNIQUE (calendar_id, day)
);

CREATE TABLE IF NOT EXISTS adrient.ticket_groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    project_id  UUID NOT NULL REFERENCES adrient.ticket_projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    strategy    TEXT NOT NULL DEFAULT 'manual',   -- manual | round_robin | load
    calendar_id UUID REFERENCES adrient.ticket_calendars(id) ON DELETE SET NULL,
    email_alias TEXT NOT NULL DEFAULT '',          -- inbound address for this group (needs mail setup)
    rr_cursor   INT  NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS ticket_groups_name_idx ON adrient.ticket_groups (project_id, lower(name));

CREATE TABLE IF NOT EXISTS adrient.ticket_group_members (
    group_id  UUID NOT NULL REFERENCES adrient.ticket_groups(id) ON DELETE CASCADE,
    org_id    UUID NOT NULL,
    user_id   UUID NOT NULL,
    skills    TEXT[] NOT NULL DEFAULT '{}',
    capacity  INT NOT NULL DEFAULT 20,
    ooo_until TIMESTAMPTZ,                          -- out of office: skipped by routing
    position  INT NOT NULL DEFAULT 0,
    PRIMARY KEY (group_id, user_id)
);

ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS group_id        UUID;
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS urgency         TEXT NOT NULL DEFAULT 'medium';
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS impact          TEXT NOT NULL DEFAULT 'medium';
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS category        TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS subcategory     TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS custom          JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS parent_id       UUID;
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS major           BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS snoozed_until   TIMESTAMPTZ;
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS first_response_at TIMESTAMPTZ;
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS sla_paused_at   TIMESTAMPTZ;
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS sla_paused_min  INT NOT NULL DEFAULT 0;
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS form_id         UUID;
CREATE INDEX IF NOT EXISTS tickets_group_idx    ON adrient.tickets (group_id) WHERE group_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS tickets_parent_idx   ON adrient.tickets (parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS tickets_assignee_idx ON adrient.tickets (org_id, assignee_user_id, status);
CREATE INDEX IF NOT EXISTS tickets_list_idx     ON adrient.tickets (project_id, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS tickets_org_created_idx ON adrient.tickets (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS tickets_major_idx    ON adrient.tickets (org_id) WHERE major;

CREATE TABLE IF NOT EXISTS adrient.ticket_time_entries (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    ticket_id  UUID NOT NULL REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    user_name  TEXT NOT NULL DEFAULT '',
    minutes    INT NOT NULL CHECK (minutes > 0 AND minutes <= 1440),
    note       TEXT NOT NULL DEFAULT '',
    billable   BOOLEAN NOT NULL DEFAULT false,
    work_date  DATE NOT NULL DEFAULT CURRENT_DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ticket_time_ticket_idx ON adrient.ticket_time_entries (ticket_id);

CREATE TABLE IF NOT EXISTS adrient.ticket_drafts (
    ticket_id  UUID NOT NULL REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    org_id     UUID NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    internal   BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (ticket_id, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.ticket_presence (
    ticket_id UUID NOT NULL REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    user_id   UUID NOT NULL,
    org_id    UUID NOT NULL,
    name      TEXT NOT NULL DEFAULT '',
    mode      TEXT NOT NULL DEFAULT 'viewing',      -- viewing | replying
    seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (ticket_id, user_id)
);
