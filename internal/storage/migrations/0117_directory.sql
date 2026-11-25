-- 0117: Directory (D1). User attributes, a department tree and a normalized
-- group_members table.
--
-- Departments form a per-org tree (parent_id) with an optional head user.
-- User attributes live on adrient.users so every app can read them through
-- internal/people. group_members mirrors adrient.groups.member_ids: the JSONB
-- column stays the write path used by internal/groups and the authz loader,
-- and a trigger keeps group_members in sync, so both views agree.

CREATE TABLE IF NOT EXISTS adrient.departments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    parent_id     UUID REFERENCES adrient.departments(id) ON DELETE SET NULL,
    name          TEXT NOT NULL,
    head_user_id  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS departments_org_parent_name_uq
    ON adrient.departments (org_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name));
CREATE INDEX IF NOT EXISTS departments_org_idx ON adrient.departments (org_id);

ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS job_title     TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS department_id UUID REFERENCES adrient.departments(id) ON DELETE SET NULL;
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS manager_id    UUID REFERENCES adrient.users(id) ON DELETE SET NULL;
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS phone         TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS location      TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS employee_id   TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS start_date    DATE;
CREATE INDEX IF NOT EXISTS users_manager_idx ON adrient.users (manager_id) WHERE manager_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS users_department_idx ON adrient.users (department_id) WHERE department_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS adrient.group_members (
    group_id   UUID NOT NULL REFERENCES adrient.groups(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id)  ON DELETE CASCADE,
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id)   ON DELETE CASCADE,
    added_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX IF NOT EXISTS group_members_user_idx ON adrient.group_members (user_id);

-- Replace a group's rows with the ids in groups.member_ids (ignores ids that are
-- not valid uuids or not users of the org).
CREATE OR REPLACE FUNCTION adrient.sync_group_members() RETURNS trigger AS $$
BEGIN
    DELETE FROM adrient.group_members WHERE group_id = NEW.id;
    INSERT INTO adrient.group_members (group_id, user_id, org_id)
    SELECT NEW.id, u.id, NEW.org_id
    FROM (SELECT DISTINCT m AS mid FROM jsonb_array_elements_text(NEW.member_ids) AS m
          WHERE m ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$') ids
    JOIN adrient.users u ON u.id = ids.mid::uuid AND u.org_id = NEW.org_id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS groups_sync_members ON adrient.groups;
CREATE TRIGGER groups_sync_members
    AFTER INSERT OR UPDATE OF member_ids ON adrient.groups
    FOR EACH ROW EXECUTE FUNCTION adrient.sync_group_members();

-- Backfill from existing JSONB arrays.
INSERT INTO adrient.group_members (group_id, user_id, org_id)
SELECT g.id, u.id, g.org_id
FROM adrient.groups g
CROSS JOIN LATERAL (SELECT DISTINCT m FROM jsonb_array_elements_text(g.member_ids) AS m
    WHERE m ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$') ids
JOIN adrient.users u ON u.id = ids.m::uuid AND u.org_id = g.org_id
ON CONFLICT DO NOTHING;
