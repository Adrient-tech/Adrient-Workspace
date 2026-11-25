-- Accounting wave 3, part 6: audit before/after, journal attachments. Idempotent.
ALTER TABLE adrient.acct_audit ADD COLUMN IF NOT EXISTS before JSONB;
ALTER TABLE adrient.acct_audit ADD COLUMN IF NOT EXISTS after  JSONB;

CREATE TABLE IF NOT EXISTS adrient.acct_journal_files (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    journal_id  UUID NOT NULL REFERENCES adrient.acct_journals(id) ON DELETE CASCADE,
    file_id     TEXT NOT NULL,                 -- Drive file id
    name        TEXT NOT NULL DEFAULT '',
    added_by    TEXT NOT NULL DEFAULT '',
    added_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (journal_id, file_id)
);
