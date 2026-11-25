-- Booking pages (appointment schedules): a public, Calendly-style page where
-- anyone can book a slot in the owner's free time. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.booking_pages (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            UUID NOT NULL REFERENCES adrient.orgs(id)  ON DELETE CASCADE,
    owner_id          UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    slug              TEXT NOT NULL UNIQUE,
    title             TEXT NOT NULL DEFAULT '',
    description       TEXT NOT NULL DEFAULT '',
    location          TEXT NOT NULL DEFAULT '',
    duration_min      INT  NOT NULL DEFAULT 30,
    buffer_before_min INT  NOT NULL DEFAULT 0,
    buffer_after_min  INT  NOT NULL DEFAULT 0,
    min_notice_min    INT  NOT NULL DEFAULT 240,
    max_days_ahead    INT  NOT NULL DEFAULT 60,
    slot_step_min     INT  NOT NULL DEFAULT 30,
    max_per_day       INT  NOT NULL DEFAULT 0,
    timezone          TEXT NOT NULL DEFAULT 'UTC',
    availability      JSONB NOT NULL DEFAULT '{}'::jsonb,
    questions         JSONB NOT NULL DEFAULT '[]'::jsonb,
    create_meet       BOOLEAN NOT NULL DEFAULT true,
    active            BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS booking_pages_owner_idx ON adrient.booking_pages (org_id, owner_id);

CREATE TABLE IF NOT EXISTS adrient.booking_appointments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    page_id      UUID NOT NULL REFERENCES adrient.booking_pages(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL,
    owner_id     UUID NOT NULL,
    event_id     UUID,
    start_at     TIMESTAMPTZ NOT NULL,
    end_at       TIMESTAMPTZ NOT NULL,
    guest_name   TEXT NOT NULL,
    guest_email  TEXT NOT NULL,
    guest_tz     TEXT NOT NULL DEFAULT 'UTC',
    answers      JSONB NOT NULL DEFAULT '{}'::jsonb,
    meet_code    TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'confirmed',  -- confirmed | cancelled
    cancel_token TEXT NOT NULL UNIQUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- A start time on one page can be confirmed only once.
CREATE UNIQUE INDEX IF NOT EXISTS booking_appt_slot_uq
    ON adrient.booking_appointments (page_id, start_at) WHERE status = 'confirmed';
CREATE INDEX IF NOT EXISTS booking_appt_owner_idx
    ON adrient.booking_appointments (org_id, owner_id, start_at);
