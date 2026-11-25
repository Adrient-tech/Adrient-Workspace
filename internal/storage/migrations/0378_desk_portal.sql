-- Service desk: requester portal tracking token. Idempotent.
ALTER TABLE adrient.tickets ADD COLUMN IF NOT EXISTS portal_token TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS tickets_portal_token_idx ON adrient.tickets (portal_token) WHERE portal_token IS NOT NULL;
