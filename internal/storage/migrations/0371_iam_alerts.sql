-- 0371: risk alerts, triage queue, comments.
CREATE TABLE IF NOT EXISTS adrient.iam_alerts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id     UUID,
    email       TEXT NOT NULL DEFAULT '',
    ip          TEXT NOT NULL DEFAULT '',
    rule        TEXT NOT NULL,
    severity    TEXT NOT NULL CHECK (severity IN ('low','medium','high','critical')),
    status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','investigating','false_positive','compromised','policy')),
    title       TEXT NOT NULL,
    evidence    JSONB NOT NULL DEFAULT '{}',
    recommended TEXT NOT NULL DEFAULT '',
    dedupe_key  TEXT NOT NULL DEFAULT '',
    assignee_id UUID,
    event_id    BIGINT,
    sla_due_at  TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    resolved_by UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS iam_alerts_queue_idx ON adrient.iam_alerts (org_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS iam_alerts_user_idx  ON adrient.iam_alerts (org_id, user_id);
-- One OPEN alert per rule and subject: a burst of events updates it rather than flooding the queue.
CREATE UNIQUE INDEX IF NOT EXISTS iam_alerts_open_dedupe ON adrient.iam_alerts (org_id, dedupe_key)
    WHERE dedupe_key <> '' AND status IN ('open','investigating');

CREATE TABLE IF NOT EXISTS adrient.iam_alert_comments (
    id         BIGSERIAL PRIMARY KEY,
    alert_id   UUID NOT NULL REFERENCES adrient.iam_alerts(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL,
    author_id  UUID,
    kind       TEXT NOT NULL DEFAULT 'comment',  -- comment | status | assign | action
    body       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS iam_alert_comments_idx ON adrient.iam_alert_comments (alert_id, id);
