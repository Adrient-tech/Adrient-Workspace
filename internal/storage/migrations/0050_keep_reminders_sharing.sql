-- 0050: Keep reminders + note-level sharing.
--
-- remind_at: an optional timestamp stored on the note. No push delivery is
-- implemented — this is purely storage + display. The frontend shows a
-- "Reminders" view filtered to notes where remind_at IS NOT NULL.
--
-- Note sharing reuses the existing adrient.object_grants ACL table
-- (object_type = 'keep_note') introduced in migration 0042.

ALTER TABLE adrient.keep_notes
    ADD COLUMN IF NOT EXISTS remind_at TIMESTAMPTZ;

-- Index so the "Reminders" view query is fast.
CREATE INDEX IF NOT EXISTS keep_notes_remind_at_idx
    ON adrient.keep_notes (org_id, remind_at)
    WHERE remind_at IS NOT NULL AND trashed_at IS NULL;
