-- 0125: remember which Drive file a mail attachment was saved to (dedupe for
-- "Open in app" / "Save to Drive" on mail attachments).
CREATE TABLE IF NOT EXISTS adrient.mail_attachment_saves (
    attachment_id UUID NOT NULL REFERENCES adrient.mail_attachments(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    drive_file_id UUID NOT NULL REFERENCES adrient.drive_files(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (attachment_id, user_id)
);
