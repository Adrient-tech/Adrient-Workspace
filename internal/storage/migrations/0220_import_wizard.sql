-- 0220: Migration wizard (Google Workspace, Microsoft 365, Zoho, Nextcloud/ownCloud,
-- generic IMAP/CalDAV/CardDAV). Builds on 0078 (archive import jobs).
--
--   import_provider_settings  admin-entered OAuth app credentials per provider
--   import_connections        one source account/tenant an admin or user connected
--   import_runs               one execution (dry-run or real) with progress counters
--   import_user_map           remote user -> Adrient user, edited in the mapping UI
--   import_map                remote object -> local object (makes re-runs idempotent)
--   import_errors             per-item error report
--
-- Secrets (client secrets, tokens, passwords) are stored AES-GCM sealed by the
-- application (BYTEA), never as plain text.

CREATE TABLE IF NOT EXISTS adrient.import_provider_settings (
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    provider      TEXT NOT NULL,
    client_id     TEXT NOT NULL DEFAULT '',
    client_secret_enc BYTEA,
    extra         JSONB NOT NULL DEFAULT '{}'::jsonb,   -- e.g. tenant, redirect override, region
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, provider)
);

CREATE TABLE IF NOT EXISTS adrient.import_connections (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    created_by  UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    provider    TEXT NOT NULL,                       -- google | microsoft | zoho | nextcloud | imap | caldav ...
    label       TEXT NOT NULL DEFAULT '',
    scope       TEXT NOT NULL DEFAULT 'me',          -- me (own account) | org (admin, all users)
    config      JSONB NOT NULL DEFAULT '{}'::jsonb,  -- non-secret: server URLs, domain, username
    secret_enc  BYTEA,                               -- sealed JSON: password / oauth tokens
    status      TEXT NOT NULL DEFAULT 'ready',       -- ready | needs_auth | error
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS import_connections_org ON adrient.import_connections (org_id, created_at DESC);

CREATE TABLE IF NOT EXISTS adrient.import_runs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    connection_id UUID NOT NULL REFERENCES adrient.import_connections(id) ON DELETE CASCADE,
    started_by    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    kinds         TEXT[] NOT NULL DEFAULT '{}',
    dry_run       BOOLEAN NOT NULL DEFAULT FALSE,
    status        TEXT NOT NULL DEFAULT 'queued',    -- queued | running | done | failed | cancelled | interrupted
    counters      JSONB NOT NULL DEFAULT '{}'::jsonb, -- {"files":{"seen":3,"imported":2,"skipped":1,"failed":0}}
    message       TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS import_runs_conn ON adrient.import_runs (connection_id, created_at DESC);

CREATE TABLE IF NOT EXISTS adrient.import_user_map (
    connection_id  UUID NOT NULL REFERENCES adrient.import_connections(id) ON DELETE CASCADE,
    remote_id      TEXT NOT NULL,
    remote_email   TEXT NOT NULL DEFAULT '',
    remote_name    TEXT NOT NULL DEFAULT '',
    local_user_id  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    skip           BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (connection_id, remote_id)
);

CREATE TABLE IF NOT EXISTS adrient.import_map (
    connection_id UUID NOT NULL REFERENCES adrient.import_connections(id) ON DELETE CASCADE,
    kind          TEXT NOT NULL,
    remote_id     TEXT NOT NULL,
    local_id      TEXT NOT NULL DEFAULT '',
    fingerprint   TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (connection_id, kind, remote_id)
);

CREATE TABLE IF NOT EXISTS adrient.import_errors (
    id         BIGSERIAL PRIMARY KEY,
    run_id     UUID NOT NULL REFERENCES adrient.import_runs(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    remote_id  TEXT NOT NULL DEFAULT '',
    label      TEXT NOT NULL DEFAULT '',
    message    TEXT NOT NULL,
    at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS import_errors_run ON adrient.import_errors (run_id, id);
