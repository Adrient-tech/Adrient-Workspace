-- 0018: Mail attachments.
--
-- Attachment bytes live in the blob store (shared with Drive); metadata lives
-- here. Messages carry a denormalized JSONB list of their attachments for
-- display without a join.

ALTER TABLE adrient.mail_messages ADD COLUMN IF NOT EXISTS attachments JSONB NOT NULL DEFAULT '[]';

CREATE TABLE IF NOT EXISTS adrient.mail_attachments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id)  ON DELETE RESTRICT,
    owner_id     UUID NOT NULL REFERENCES adrient.users(id) ON DELETE RESTRICT,
    filename     TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    size         BIGINT NOT NULL DEFAULT 0,
    blob_key     TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
