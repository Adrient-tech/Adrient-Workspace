-- 0163: Talent learning, policies, announcements and recognition.

CREATE TABLE IF NOT EXISTS adrient.talent_courses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL DEFAULT '',
    link        TEXT NOT NULL DEFAULT '',
    duration_min INT NOT NULL DEFAULT 0,
    mandatory   BOOLEAN NOT NULL DEFAULT FALSE,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.talent_enrollments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    course_id   UUID NOT NULL REFERENCES adrient.talent_courses(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL,
    status      TEXT NOT NULL DEFAULT 'enrolled',
    due_date    DATE,
    enrolled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    enrolled_by UUID,
    CONSTRAINT talent_enr_status_chk CHECK (status IN ('enrolled','in_progress','completed')),
    UNIQUE (course_id, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.talent_policies (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    summary    TEXT NOT NULL DEFAULT '',
    link       TEXT NOT NULL DEFAULT '',
    version    INT NOT NULL DEFAULT 1,
    mandatory  BOOLEAN NOT NULL DEFAULT TRUE,
    audience   UUID[] NOT NULL DEFAULT '{}',
    active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.talent_policy_acks (
    policy_id  UUID NOT NULL REFERENCES adrient.talent_policies(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL,
    user_id    UUID NOT NULL,
    version    INT NOT NULL,
    acked_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (policy_id, user_id, version)
);

CREATE TABLE IF NOT EXISTS adrient.talent_announcements (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    pinned     BOOLEAN NOT NULL DEFAULT FALSE,
    author_id  UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.talent_kudos (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    from_id    UUID NOT NULL,
    to_id      UUID NOT NULL,
    badge      TEXT NOT NULL DEFAULT 'thanks',
    message    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS talent_kudos_org_idx ON adrient.talent_kudos (org_id, created_at DESC);

-- Birthdays have no source elsewhere: each person (or HR) enters month/day.
CREATE TABLE IF NOT EXISTS adrient.talent_birthdays (
    org_id  UUID NOT NULL,
    user_id UUID NOT NULL,
    month   INT NOT NULL CHECK (month BETWEEN 1 AND 12),
    day     INT NOT NULL CHECK (day BETWEEN 1 AND 31),
    PRIMARY KEY (org_id, user_id)
);
