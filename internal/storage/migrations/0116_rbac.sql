-- 0116: RBAC core (roles, role_permissions, role_assignments).
--
-- System roles are global rows (org_id NULL): owner, admin, app_admin, manager,
-- member, guest. A permission is "app:resource:action"; "*" and "app:*" are
-- wildcards. Assignments are org-scoped and target a user or a group
-- (adrient.groups.member_ids). An app_admin assignment carries scope_app and
-- grants "<app>:*". Everyone in an org is implicitly a Member unless they hold
-- the Guest role. The legacy org_admins table still counts as Admin at runtime,
-- so the existing grant/revoke admin UI keeps working.

CREATE TABLE IF NOT EXISTS adrient.roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    key         TEXT NOT NULL,
    name        TEXT NOT NULL,
    system      BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS roles_system_key_uq ON adrient.roles (key) WHERE org_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS roles_org_key_uq ON adrient.roles (org_id, key) WHERE org_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS adrient.role_permissions (
    role_id     UUID NOT NULL REFERENCES adrient.roles(id) ON DELETE CASCADE,
    permission  TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE IF NOT EXISTS adrient.role_assignments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    subject_type  TEXT NOT NULL CHECK (subject_type IN ('user', 'group')),
    subject_id    UUID NOT NULL,
    role_id       UUID NOT NULL REFERENCES adrient.roles(id) ON DELETE CASCADE,
    scope_app     TEXT NOT NULL DEFAULT '',
    created_by    UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS role_assignments_uq
    ON adrient.role_assignments (org_id, subject_type, subject_id, role_id, scope_app);
CREATE INDEX IF NOT EXISTS role_assignments_subject_idx
    ON adrient.role_assignments (org_id, subject_type, subject_id);

INSERT INTO adrient.roles (org_id, key, name, system) VALUES
    (NULL, 'owner',     'Owner',     true),
    (NULL, 'admin',     'Admin',     true),
    (NULL, 'app_admin', 'App admin', true),
    (NULL, 'manager',   'Manager',   true),
    (NULL, 'member',    'Member',    true),
    (NULL, 'guest',     'Guest',     true)
ON CONFLICT DO NOTHING;

INSERT INTO adrient.role_permissions (role_id, permission)
SELECT r.id, p.permission
FROM adrient.roles r
JOIN (VALUES
    ('owner',   '*'),
    ('owner',   'org:owner'),
    ('admin',   '*'),
    ('manager', 'org:manager'),
    ('manager', 'pos:*'),
    ('member',  'org:member'),
    ('member',  'crm:*'),
    ('member',  'invoicing:*'),
    ('member',  'inventory:*'),
    ('member',  'pos:read'),
    ('member',  'pos:sell'),
    ('member',  'pos:shift:open')
) AS p(role_key, permission) ON p.role_key = r.key
WHERE r.org_id IS NULL
ON CONFLICT DO NOTHING;

-- Backfill: every org_admins row becomes Admin; the earliest admin of each org
-- also becomes Owner.
INSERT INTO adrient.role_assignments (org_id, subject_type, subject_id, role_id, created_by)
SELECT oa.org_id, 'user', oa.user_id, r.id, oa.granted_by
FROM adrient.org_admins oa
JOIN adrient.roles r ON r.key = 'admin' AND r.org_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO adrient.role_assignments (org_id, subject_type, subject_id, role_id)
SELECT first_admin.org_id, 'user', first_admin.user_id, r.id
FROM (
    SELECT DISTINCT ON (org_id) org_id, user_id
    FROM adrient.org_admins
    ORDER BY org_id, granted_at, user_id
) first_admin
JOIN adrient.roles r ON r.key = 'owner' AND r.org_id IS NULL
ON CONFLICT DO NOTHING;
