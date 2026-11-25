-- Time tracking: clients, projects, entries and settings (JSON records per kind) plus one running timer per person.
CREATE TABLE IF NOT EXISTS adrient.tt_docs (
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
CREATE INDEX IF NOT EXISTS tt_docs_kind_day_idx ON adrient.tt_docs (org_id, kind, day);
CREATE INDEX IF NOT EXISTS tt_docs_kind_ref_idx ON adrient.tt_docs (org_id, kind, ref);

CREATE TABLE IF NOT EXISTS adrient.tt_timers (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL DEFAULT '',
    task       TEXT NOT NULL DEFAULT '',
    billable   BOOLEAN NOT NULL DEFAULT true,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, user_id)
);
