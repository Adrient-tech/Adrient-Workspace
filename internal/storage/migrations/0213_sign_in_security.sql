-- 0213: login history, session policy, seats and plans.
CREATE TABLE IF NOT EXISTS adrient.login_events (
    id         BIGSERIAL PRIMARY KEY,
    org_id     UUID,
    user_id    UUID,
    email      TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    method     TEXT NOT NULL DEFAULT '',
    success    BOOLEAN NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    suspicious BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS login_events_org_idx  ON adrient.login_events (org_id, id DESC);
CREATE INDEX IF NOT EXISTS login_events_user_idx ON adrient.login_events (user_id, id DESC);

-- One row per org; NULL means "not limited". Every value is enforced by the
-- server (session lookup and sign-in), none is advisory.
CREATE TABLE IF NOT EXISTS adrient.org_security_settings (
    org_id                  UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    session_max_hours       INT CHECK (session_max_hours BETWEEN 1 AND 8760),
    session_idle_minutes    INT CHECK (session_idle_minutes BETWEEN 5 AND 525600),
    max_concurrent_sessions INT CHECK (max_concurrent_sessions BETWEEN 1 AND 100),
    alert_new_device        BOOLEAN NOT NULL DEFAULT true,
    alert_failed_threshold  INT NOT NULL DEFAULT 5 CHECK (alert_failed_threshold BETWEEN 2 AND 100),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seats: NULL seats = unlimited. A seat is an active member (guests, external
-- members and service accounts do not use one).
CREATE TABLE IF NOT EXISTS adrient.org_plans (
    org_id     UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    plan       TEXT NOT NULL DEFAULT 'standard',
    seats      INT CHECK (seats IS NULL OR seats >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Per-app seat caps: how many people may be given an app.
CREATE TABLE IF NOT EXISTS adrient.app_seat_caps (
    org_id UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    app_id TEXT NOT NULL,
    seats  INT NOT NULL CHECK (seats >= 0),
    PRIMARY KEY (org_id, app_id)
);
