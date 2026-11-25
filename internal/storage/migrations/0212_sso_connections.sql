-- 0212: Single sign-on connections. The provider itself lives in Zitadel
-- (zitadel_idp_id); secrets are sent to Zitadel and never stored here.
CREATE TABLE IF NOT EXISTS adrient.sso_connections (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL CHECK (kind IN ('entra','google','oidc','saml')),
    name           TEXT NOT NULL,
    zitadel_idp_id TEXT NOT NULL,
    jit            BOOLEAN NOT NULL DEFAULT true,
    enforce        BOOLEAN NOT NULL DEFAULT false,
    enabled        BOOLEAN NOT NULL DEFAULT true,
    issuer         TEXT NOT NULL DEFAULT '',
    tenant_id      TEXT NOT NULL DEFAULT '',
    client_id      TEXT NOT NULL DEFAULT '',
    created_by     UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sso_connections_org_idx ON adrient.sso_connections (org_id);

-- Domain routing: a domain belongs to exactly one connection.
CREATE TABLE IF NOT EXISTS adrient.sso_domains (
    domain        TEXT PRIMARY KEY,
    connection_id UUID NOT NULL REFERENCES adrient.sso_connections(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE
);
