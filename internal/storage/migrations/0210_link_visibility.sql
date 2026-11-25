-- 0210: link visibility for Drive and Docs share links (control
-- drive.link_sharing_default). Existing links keep working as before ('anyone').
ALTER TABLE adrient.drive_shares ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'anyone';
ALTER TABLE adrient.docs_shares  ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'anyone';
