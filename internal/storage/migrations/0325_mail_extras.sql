-- 0325: Mail saved searches and signatures (per user).
CREATE TABLE IF NOT EXISTS adrient.mail_saved_searches (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE RESTRICT,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    query      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, name)
);
CREATE INDEX IF NOT EXISTS mail_saved_searches_user_idx ON adrient.mail_saved_searches (user_id);

CREATE TABLE IF NOT EXISTS adrient.mail_signatures (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE RESTRICT,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    body       TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, name)
);
CREATE INDEX IF NOT EXISTS mail_signatures_user_idx ON adrient.mail_signatures (user_id);
