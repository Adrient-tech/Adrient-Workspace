-- Fleet: vehicles, meter readings, fuel, service, reminders, renewals, issues and inspections (JSON records per kind).
CREATE TABLE IF NOT EXISTS adrient.flt_docs (
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
CREATE INDEX IF NOT EXISTS flt_docs_kind_day_idx ON adrient.flt_docs (org_id, kind, day);
CREATE INDEX IF NOT EXISTS flt_docs_kind_ref_idx ON adrient.flt_docs (org_id, kind, ref);
