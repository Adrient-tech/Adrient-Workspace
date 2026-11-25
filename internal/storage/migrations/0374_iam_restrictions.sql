-- 0374: quarantine mode, "this wasn't me" reports.
CREATE TABLE IF NOT EXISTS adrient.iam_restrictions (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('quarantine')),
    reason     TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id, kind)
);

CREATE TABLE IF NOT EXISTS adrient.iam_user_reports (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    event_id   BIGINT,
    verdict    TEXT NOT NULL CHECK (verdict IN ('not_me','was_me','suspicious')),
    note       TEXT NOT NULL DEFAULT '',
    alert_id   UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS iam_user_reports_idx ON adrient.iam_user_reports (org_id, user_id, created_at DESC);
