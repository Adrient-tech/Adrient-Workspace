-- 0020: Meet (video-call rooms).
--
-- Persists Meet room metadata. Signaling state (who is currently connected)
-- is purely in-memory in the Hub; only room identity survives restarts.

CREATE TABLE IF NOT EXISTS adrient.meet_rooms (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id)  ON DELETE RESTRICT,
    owner_id   UUID NOT NULL REFERENCES adrient.users(id) ON DELETE RESTRICT,
    name       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS meet_rooms_org_idx
  ON adrient.meet_rooms (org_id);
