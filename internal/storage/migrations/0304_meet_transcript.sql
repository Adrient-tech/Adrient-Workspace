-- Wave 6 (prod): meeting transcript segments (written only while the host has transcription on). Idempotent.
CREATE TABLE IF NOT EXISTS adrient.meet_transcript_segments (
    id       BIGSERIAL PRIMARY KEY,
    org_id   UUID NOT NULL,
    room_id  UUID NOT NULL,
    user_id  UUID NOT NULL,
    speaker  TEXT NOT NULL,
    body     TEXT NOT NULL,
    at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS meet_transcript_room_idx ON adrient.meet_transcript_segments (room_id, id);
