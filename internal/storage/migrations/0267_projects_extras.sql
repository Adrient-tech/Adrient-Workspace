-- Projects extras: dates, dependencies, time estimates, milestones, project
-- brief, saved views and links to CRM and HR. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.project_issue_extras (
    issue_id        UUID PRIMARY KEY REFERENCES adrient.project_issues(id) ON DELETE CASCADE,
    org_id          UUID NOT NULL,
    start_date      DATE,
    due_date        DATE,
    milestone_id    UUID,
    estimate_hours  NUMERIC(8,2),
    logged_hours    NUMERIC(8,2) NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS project_issue_extras_org_idx ON adrient.project_issue_extras (org_id);

CREATE TABLE IF NOT EXISTS adrient.project_issue_deps (
    issue_id      UUID NOT NULL REFERENCES adrient.project_issues(id) ON DELETE CASCADE,
    depends_on_id UUID NOT NULL REFERENCES adrient.project_issues(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL,
    PRIMARY KEY (issue_id, depends_on_id),
    CHECK (issue_id <> depends_on_id)
);
CREATE INDEX IF NOT EXISTS project_issue_deps_dep_idx ON adrient.project_issue_deps (depends_on_id);

CREATE TABLE IF NOT EXISTS adrient.project_milestones (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES adrient.project_projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    due_date   DATE,
    done       BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS project_milestones_project_idx ON adrient.project_milestones (project_id);

CREATE TABLE IF NOT EXISTS adrient.project_briefs (
    project_id UUID PRIMARY KEY REFERENCES adrient.project_projects(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    updated_by UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.project_saved_views (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    owner_id   UUID NOT NULL,
    name       TEXT NOT NULL,
    mode       TEXT NOT NULL DEFAULT 'list',
    filters    JSONB NOT NULL DEFAULT '{}'::jsonb,
    shared     BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS project_saved_views_owner_idx ON adrient.project_saved_views (org_id, owner_id);

CREATE TABLE IF NOT EXISTS adrient.project_links (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES adrient.project_projects(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,                 -- crm_deal | hr_employee
    ref_id     UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, kind, ref_id)
);
