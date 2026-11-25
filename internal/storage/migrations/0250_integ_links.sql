-- Wave 4 cross-app links. Idempotent.
ALTER TABLE adrient.hr_employees ADD COLUMN IF NOT EXISTS source_ref TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS hr_employees_source_ref_uq ON adrient.hr_employees (org_id, source_ref) WHERE source_ref <> '';

-- Generic record link table: "this record in app A relates to that record in app B".
CREATE TABLE IF NOT EXISTS adrient.record_links (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    from_app   TEXT NOT NULL,
    from_type  TEXT NOT NULL,
    from_id    TEXT NOT NULL,
    to_app     TEXT NOT NULL,
    to_type    TEXT NOT NULL,
    to_id      TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT '',
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS record_links_uq ON adrient.record_links (org_id, from_app, from_type, from_id, to_app, to_type, to_id);
CREATE INDEX IF NOT EXISTS record_links_from_idx ON adrient.record_links (org_id, from_app, from_type, from_id);
CREATE INDEX IF NOT EXISTS record_links_to_idx ON adrient.record_links (org_id, to_app, to_type, to_id);
