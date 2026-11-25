-- 0134: server-side Drive file -> editor document link. Every user opening the
-- same Drive .docx/.xlsx/.pptx lands on the SAME Docs/Sheets/Slides document.
-- Access follows the Drive file (org membership or an object grant on it).
CREATE TABLE IF NOT EXISTS adrient.drive_doc_links (
    drive_file_id UUID NOT NULL,
    doc_kind      TEXT NOT NULL CHECK (doc_kind IN ('docs', 'sheets', 'slides')),
    doc_id        UUID NOT NULL,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    created_by    UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (drive_file_id, doc_kind)
);
CREATE INDEX IF NOT EXISTS drive_doc_links_doc_idx ON adrient.drive_doc_links (doc_kind, doc_id);
