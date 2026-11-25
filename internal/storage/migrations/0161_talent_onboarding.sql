-- 0161: Talent onboarding / offboarding checklists.

CREATE TABLE IF NOT EXISTS adrient.talent_checklist_templates (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL DEFAULT 'onboarding',
    name       TEXT NOT NULL,
    tasks      JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT talent_tpl_kind_chk CHECK (kind IN ('onboarding','offboarding'))
);

CREATE TABLE IF NOT EXISTS adrient.talent_checklists (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    template_id   UUID,
    kind          TEXT NOT NULL DEFAULT 'onboarding',
    subject_user_id UUID,
    subject_name  TEXT NOT NULL,
    subject_email TEXT NOT NULL DEFAULT '',
    manager_id    UUID,
    hr_id         UUID,
    it_id         UUID,
    anchor_date   DATE NOT NULL,
    status        TEXT NOT NULL DEFAULT 'active',
    created_by    UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT talent_cl_status_chk CHECK (status IN ('active','done','cancelled'))
);

CREATE TABLE IF NOT EXISTS adrient.talent_checklist_tasks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL,
    checklist_id  UUID NOT NULL REFERENCES adrient.talent_checklists(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    assignee_role TEXT NOT NULL DEFAULT 'hr',
    assignee_id   UUID,
    due_date      DATE,
    link          TEXT NOT NULL DEFAULT '',
    done          BOOLEAN NOT NULL DEFAULT FALSE,
    done_at       TIMESTAMPTZ,
    done_by       UUID,
    tasks_item_id UUID,
    reminded_at   TIMESTAMPTZ,
    position      INT NOT NULL DEFAULT 0,
    CONSTRAINT talent_clt_role_chk CHECK (assignee_role IN ('hr','it','manager','employee'))
);
CREATE INDEX IF NOT EXISTS talent_clt_cl_idx ON adrient.talent_checklist_tasks (checklist_id);
CREATE INDEX IF NOT EXISTS talent_clt_assignee_idx ON adrient.talent_checklist_tasks (org_id, assignee_id) WHERE NOT done;

CREATE TABLE IF NOT EXISTS adrient.talent_assets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    checklist_id UUID NOT NULL REFERENCES adrient.talent_checklists(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    serial       TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'assigned',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT talent_asset_status_chk CHECK (status IN ('assigned','returned','lost'))
);

CREATE TABLE IF NOT EXISTS adrient.talent_exit_interviews (
    checklist_id UUID PRIMARY KEY REFERENCES adrient.talent_checklists(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL,
    reason       TEXT NOT NULL DEFAULT '',
    would_return BOOLEAN,
    answers      JSONB NOT NULL DEFAULT '{}',
    notes        TEXT NOT NULL DEFAULT '',
    by_user      UUID,
    at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
