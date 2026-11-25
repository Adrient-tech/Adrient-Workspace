-- 0076: short meeting codes for Meet.
--
-- Adds a human-readable code (e.g. abc-defg-hij) to each meet room so
-- participants can share a stable, typed-friendly link. The code is unique
-- across the table (globally); room isolation is still org-scoped in queries.

ALTER TABLE adrient.meet_rooms ADD COLUMN IF NOT EXISTS code TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS meet_rooms_code_idx ON adrient.meet_rooms (code);
