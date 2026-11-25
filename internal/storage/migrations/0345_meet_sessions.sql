-- 0345: a meeting room has a current "session": it starts when the first
-- participant joins and ends when the last one leaves, the host ends it for
-- everyone, or the room is closed. The Chat meeting card and the /meet/<code>
-- page read the end state from these columns.
ALTER TABLE adrient.meet_rooms ADD COLUMN IF NOT EXISTS session_started_at TIMESTAMPTZ;
ALTER TABLE adrient.meet_rooms ADD COLUMN IF NOT EXISTS ended_at TIMESTAMPTZ;
ALTER TABLE adrient.meet_rooms ADD COLUMN IF NOT EXISTS ended_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.meet_rooms ADD COLUMN IF NOT EXISTS participant_ids TEXT[] NOT NULL DEFAULT '{}';
