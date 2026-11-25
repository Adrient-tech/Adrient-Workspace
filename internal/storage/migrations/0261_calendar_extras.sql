-- Calendar extras: per-user settings (time zone, working hours), reminder
-- delivery log, private feed tokens, ICS import de-duplication. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.calendar_settings (
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id)  ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    timezone      TEXT NOT NULL DEFAULT 'UTC',
    working_hours JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.calendar_reminders_sent (
    event_id    UUID NOT NULL REFERENCES adrient.calendar_events(id) ON DELETE CASCADE,
    occurrence  TIMESTAMPTZ NOT NULL,
    minutes     INT  NOT NULL,
    user_id     UUID NOT NULL,
    sent_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, occurrence, minutes, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.calendar_feed_tokens (
    user_id    UUID PRIMARY KEY REFERENCES adrient.users(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL,
    token      TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE adrient.calendar_events ADD COLUMN IF NOT EXISTS ics_uid TEXT;
CREATE INDEX IF NOT EXISTS calendar_events_ics_uid_idx
    ON adrient.calendar_events (org_id, owner_id, ics_uid) WHERE ics_uid IS NOT NULL;
