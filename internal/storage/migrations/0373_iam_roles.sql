-- 0373: IAM operation roles (help desk, security analyst, identity admin, auditor).
CREATE TABLE IF NOT EXISTS adrient.iam_role_assignments (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('help_desk','security_analyst','identity_admin','auditor')),
    granted_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id, role)
);
