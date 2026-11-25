-- Field service: services catalogue, properties, requests, jobs and visits (JSON records per kind).
CREATE TABLE IF NOT EXISTS adrient.fs_docs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    ref        TEXT NOT NULL DEFAULT '',
    day        DATE,
    data       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS fs_docs_kind_day_idx ON adrient.fs_docs (org_id, kind, day);
CREATE INDEX IF NOT EXISTS fs_docs_kind_ref_idx ON adrient.fs_docs (org_id, kind, ref);
