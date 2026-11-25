-- +goose Up
-- Saved hand-drawn, typed or uploaded signatures and initials, one store per
-- user (keyed by the verified email the workspace session carries).
CREATE TABLE IF NOT EXISTS user_signatures (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_email  TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('signature', 'initials')),
    label       TEXT NOT NULL DEFAULT '',
    image_png   TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS user_signatures_owner_idx ON user_signatures (lower(user_email), kind, created_at DESC);

-- +goose Down
-- Kept on purpose: rolling back the app must not delete people's signatures.
SELECT 1;
