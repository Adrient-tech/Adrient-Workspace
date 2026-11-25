-- 0372: action log (rate limits, who did what), two-person approvals, step-up, recovery codes.
CREATE TABLE IF NOT EXISTS adrient.iam_actions (
    id         BIGSERIAL PRIMARY KEY,
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    actor_id   UUID,
    actor_role TEXT NOT NULL DEFAULT '',
    target_id  UUID,
    target_email TEXT NOT NULL DEFAULT '',
    action     TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    ticket     TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'done',   -- done | failed | pending_approval | dry_run
    detail     JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS iam_actions_org_idx   ON adrient.iam_actions (org_id, id DESC);
CREATE INDEX IF NOT EXISTS iam_actions_actor_idx ON adrient.iam_actions (org_id, actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS iam_actions_target_idx ON adrient.iam_actions (org_id, target_id, id DESC);

CREATE TABLE IF NOT EXISTS adrient.iam_approvals (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    action       TEXT NOT NULL,
    target_id    UUID NOT NULL,
    requester_id UUID NOT NULL,
    approver_id  UUID,
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','expired','executed')),
    reason       TEXT NOT NULL DEFAULT '',
    ticket       TEXT NOT NULL DEFAULT '',
    payload      JSONB NOT NULL DEFAULT '{}',
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT now() + interval '24 hours',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS iam_approvals_org_idx ON adrient.iam_approvals (org_id, status, created_at DESC);

-- Fresh re-authentication of an admin session (password or one-time code).
CREATE TABLE IF NOT EXISTS adrient.iam_stepups (
    session_key TEXT PRIMARY KEY,
    user_id     UUID NOT NULL,
    method      TEXT NOT NULL,
    verified_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Time-limited MFA bypass or recovery codes. Only a hash is stored.
CREATE TABLE IF NOT EXISTS adrient.iam_recovery_codes (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    code_hash  TEXT NOT NULL,
    kind       TEXT NOT NULL DEFAULT 'bypass',
    created_by UUID,
    reason     TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS iam_recovery_user_idx ON adrient.iam_recovery_codes (org_id, user_id);
