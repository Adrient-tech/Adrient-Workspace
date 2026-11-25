-- Wave 6 (prod): archive issues, soft work-in-progress limits per team and status. Idempotent.
ALTER TABLE adrient.project_issue_extras ADD COLUMN IF NOT EXISTS archived BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS adrient.project_wip_limits (
    org_id  UUID NOT NULL,
    team_id UUID NOT NULL,
    status  TEXT NOT NULL,
    max_issues INT NOT NULL CHECK (max_issues BETWEEN 1 AND 999),
    PRIMARY KEY (team_id, status)
);
