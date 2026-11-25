-- Wave 6 (prod): task tags. Idempotent.
ALTER TABLE adrient.task_extras ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX IF NOT EXISTS task_extras_tags_idx ON adrient.task_extras USING GIN (tags);
