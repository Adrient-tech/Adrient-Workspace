-- 0068: Whiteboard sharing support.
--
-- Adds thumbnail_blob_key to adrient.whiteboards for gallery preview images.
-- Sharing reuses the existing adrient.object_grants table with
-- object_type = 'whiteboard'; no new table is needed.

ALTER TABLE adrient.whiteboards
    ADD COLUMN IF NOT EXISTS thumbnail_blob_key TEXT;
