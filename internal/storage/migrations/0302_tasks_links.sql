-- Wave 6 (prod): Tasks subscribe feed (per user) and public read-only list links. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.task_feed_tokens (
    user_id    UUID PRIMARY KEY REFERENCES adrient.users(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    token      TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.task_public_links (
    list_id    UUID PRIMARY KEY REFERENCES adrient.task_lists(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    token      TEXT NOT NULL UNIQUE,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
