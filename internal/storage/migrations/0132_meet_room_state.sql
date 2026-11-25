-- 0132: durable Meet room state (lock, waiting room, host, bans, admissions)
-- so a backend restart does not unlock a locked room or forget a ban.
CREATE TABLE IF NOT EXISTS adrient.meet_room_state (
  room_id       UUID PRIMARY KEY REFERENCES adrient.meet_rooms(id) ON DELETE CASCADE,
  host_id       UUID,
  locked        BOOLEAN NOT NULL DEFAULT FALSE,
  waiting_room  BOOLEAN NOT NULL DEFAULT FALSE,
  egress_id     TEXT NOT NULL DEFAULT '',
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- kind: 'ban' (user id), 'admit' (user id), 'deny' (peer identity)
CREATE TABLE IF NOT EXISTS adrient.meet_room_members (
  room_id  UUID NOT NULL REFERENCES adrient.meet_rooms(id) ON DELETE CASCADE,
  kind     TEXT NOT NULL CHECK (kind IN ('ban','admit','deny')),
  subject  TEXT NOT NULL,
  PRIMARY KEY (room_id, kind, subject)
);
