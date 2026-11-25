-- 0133: durable waiting-room queue and Meet recordings.
CREATE TABLE IF NOT EXISTS adrient.meet_waiters (
  room_id   UUID NOT NULL REFERENCES adrient.meet_rooms(id) ON DELETE CASCADE,
  identity  TEXT NOT NULL,
  name      TEXT NOT NULL,
  since     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (room_id, identity)
);

CREATE TABLE IF NOT EXISTS adrient.meet_recordings (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  room_id       UUID NOT NULL REFERENCES adrient.meet_rooms(id) ON DELETE CASCADE,
  org_id        UUID NOT NULL,
  started_by    UUID NOT NULL,
  egress_id     TEXT NOT NULL UNIQUE,
  object_key    TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'recording' CHECK (status IN ('recording','saved','failed')),
  drive_file_id UUID,
  started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS meet_recordings_room_idx ON adrient.meet_recordings (room_id);
