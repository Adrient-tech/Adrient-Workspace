-- 0205: migration into CRM: import jobs and external-id map (makes re-runs update, not duplicate).

CREATE TABLE IF NOT EXISTS adrient.crm_external_ids (
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    source      TEXT NOT NULL,                   -- zoho | hubspot | salesforce | csv
    entity      TEXT NOT NULL,                   -- lead | company | deal
    external_id TEXT NOT NULL,
    record_id   UUID NOT NULL,
    PRIMARY KEY (org_id, source, entity, external_id)
);

CREATE TABLE IF NOT EXISTS adrient.crm_import_jobs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID,
    source     TEXT NOT NULL,
    entity     TEXT NOT NULL,
    dry_run    BOOLEAN NOT NULL DEFAULT false,
    mapping    JSONB NOT NULL DEFAULT '{}',
    report     JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
