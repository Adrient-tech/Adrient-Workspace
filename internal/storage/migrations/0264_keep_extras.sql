-- Keep extras: notebooks (sections), image and drawing attachments, reminder
-- delivery log. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.keep_notebooks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    owner_id   UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    position   INT  NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS keep_notebooks_owner_idx ON adrient.keep_notebooks (org_id, owner_id);

CREATE TABLE IF NOT EXISTS adrient.keep_note_notebook (
    note_id     UUID PRIMARY KEY REFERENCES adrient.keep_notes(id) ON DELETE CASCADE,
    notebook_id UUID NOT NULL REFERENCES adrient.keep_notebooks(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS keep_note_notebook_nb_idx ON adrient.keep_note_notebook (notebook_id);

CREATE TABLE IF NOT EXISTS adrient.keep_attachments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    note_id    UUID NOT NULL REFERENCES adrient.keep_notes(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL,
    kind       TEXT NOT NULL,            -- image | drawing
    mime       TEXT NOT NULL,
    data       BYTEA NOT NULL,
    size_bytes INT  NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS keep_attachments_note_idx ON adrient.keep_attachments (note_id);

CREATE TABLE IF NOT EXISTS adrient.keep_reminders_sent (
    note_id   UUID NOT NULL REFERENCES adrient.keep_notes(id) ON DELETE CASCADE,
    remind_at TIMESTAMPTZ NOT NULL,
    sent_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (note_id, remind_at)
);
