-- Wave 6 (prod): announcement channels (only the creator posts top-level messages).
ALTER TABLE adrient.chat_channels ADD COLUMN IF NOT EXISTS announce BOOLEAN NOT NULL DEFAULT false;
