-- 0080: User-imported HTML games.
--
-- Self-contained HTML games uploaded at runtime. The HTML bytes live in the
-- blob store (shared with Drive); metadata lives here. Org-scoped: only the
-- owning org can list/play its imported games. Untrusted content is only ever
-- served into a sandboxed iframe (no allow-same-origin).

CREATE TABLE IF NOT EXISTS adrient.games (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id)  ON DELETE RESTRICT,
    owner_id     UUID NOT NULL REFERENCES adrient.users(id) ON DELETE RESTRICT,
    name         TEXT NOT NULL DEFAULT '',
    blob_key     TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'text/html',
    size         BIGINT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS games_org_idx ON adrient.games (org_id);
