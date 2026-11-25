-- 0216: delegated admin roles (user admin, billing admin, help desk, read-only)
-- and department scope. Super admin is the existing Admin/Owner role.
INSERT INTO adrient.roles (org_id, key, name, system) VALUES
    (NULL, 'user_admin',     'User admin',     true),
    (NULL, 'billing_admin',  'Billing admin',  true),
    (NULL, 'help_desk',      'Help desk',      true),
    (NULL, 'readonly_admin', 'Read-only admin', true)
ON CONFLICT DO NOTHING;

INSERT INTO adrient.role_permissions (role_id, permission)
SELECT r.id, p.perm FROM adrient.roles r
JOIN (VALUES
    ('user_admin', 'admin:users:read'), ('user_admin', 'admin:users:write'), ('user_admin', 'admin:groups:read'),
    ('user_admin', 'admin:groups:write'), ('user_admin', 'admin:sessions:read'), ('user_admin', 'admin:sessions:write'),
    ('user_admin', 'admin:identity:read'), ('user_admin', 'admin:identity:write'), ('user_admin', 'admin:audit:read'),
    ('user_admin', 'admin:lifecycle:read'), ('user_admin', 'admin:lifecycle:write'), ('user_admin', 'admin:reviews:read'),
    ('user_admin', 'admin:reviews:write'), ('user_admin', 'identity:user:write'), ('user_admin', 'identity:invite:write'),
    ('billing_admin', 'admin:billing:read'), ('billing_admin', 'admin:billing:write'), ('billing_admin', 'admin:users:read'),
    ('billing_admin', 'admin:audit:read'),
    ('help_desk', 'admin:users:read'), ('help_desk', 'admin:users:reset'), ('help_desk', 'admin:sessions:read'),
    ('help_desk', 'admin:sessions:write'), ('help_desk', 'admin:groups:read'), ('help_desk', 'admin:identity:read'),
    ('readonly_admin', 'admin:users:read'), ('readonly_admin', 'admin:groups:read'), ('readonly_admin', 'admin:sessions:read'),
    ('readonly_admin', 'admin:identity:read'), ('readonly_admin', 'admin:audit:read'), ('readonly_admin', 'admin:billing:read'),
    ('readonly_admin', 'admin:lifecycle:read'), ('readonly_admin', 'admin:reviews:read'), ('readonly_admin', 'admin:security:read'),
    ('readonly_admin', 'admin:apps:read'), ('readonly_admin', 'admin:external:read'), ('readonly_admin', 'admin:settings:read')
) AS p(role_key, perm) ON p.role_key = r.key
WHERE r.org_id IS NULL
ON CONFLICT DO NOTHING;

-- A delegated admin can be limited to some departments (including their
-- sub-departments). No row means the whole organization.
CREATE TABLE IF NOT EXISTS adrient.admin_scopes (
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    department_id UUID NOT NULL REFERENCES adrient.departments(id) ON DELETE CASCADE,
    PRIMARY KEY (org_id, user_id, department_id)
);

-- Admin API tokens: a service user + role + personal token with the "admin" scope.
CREATE TABLE IF NOT EXISTS adrient.admin_api_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    role_key        TEXT NOT NULL,
    service_user_id UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    api_token_id    UUID REFERENCES adrient.api_tokens(id) ON DELETE SET NULL,
    prefix          TEXT NOT NULL DEFAULT '',
    created_by      UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ
);
