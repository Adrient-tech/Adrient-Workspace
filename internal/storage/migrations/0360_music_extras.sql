-- 0360: Music extras: tag fields, lyrics, play history, playlist visibility,
-- Drive-sourced tracks. Every statement is idempotent and keeps existing rows
-- working (new columns have defaults; existing playlists stay org-visible).

ALTER TABLE adrient.music_tracks ADD COLUMN IF NOT EXISTS genre          TEXT    NOT NULL DEFAULT '';
ALTER TABLE adrient.music_tracks ADD COLUMN IF NOT EXISTS track_no       INTEGER NOT NULL DEFAULT 0;
ALTER TABLE adrient.music_tracks ADD COLUMN IF NOT EXISTS year           INTEGER NOT NULL DEFAULT 0;
ALTER TABLE adrient.music_tracks ADD COLUMN IF NOT EXISTS lyrics         TEXT    NOT NULL DEFAULT '';
ALTER TABLE adrient.music_tracks ADD COLUMN IF NOT EXISTS source_file_id UUID;

ALTER TABLE adrient.music_playlists ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'org';
ALTER TABLE adrient.music_playlists ADD COLUMN IF NOT EXISTS group_id   UUID;

CREATE TABLE IF NOT EXISTS adrient.music_plays (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id)         ON DELETE CASCADE,
    user_id          UUID NOT NULL REFERENCES adrient.users(id)        ON DELETE CASCADE,
    track_id         UUID NOT NULL REFERENCES adrient.music_tracks(id) ON DELETE CASCADE,
    seconds_listened DOUBLE PRECISION NOT NULL DEFAULT 0,
    played_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS music_plays_user_idx  ON adrient.music_plays (user_id, played_at DESC);
CREATE INDEX IF NOT EXISTS music_plays_track_idx ON adrient.music_plays (track_id);
