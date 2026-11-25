-- 0139: Per-app controls engine (plan D). Typed control values with overrides by
-- subject (everyone, role, user_type, department, group, user) and optional
-- conditions ({"ip_cidrs":[...],"user_types":[...],"departments":[...]}).
-- The control catalog (keys, types, defaults) lives in code (authz/controls.go).

CREATE TABLE IF NOT EXISTS adrient.app_control_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    app_id       TEXT NOT NULL,
    control      TEXT NOT NULL,
    value        JSONB NOT NULL,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('everyone','user','group','department','role','user_type')),
    subject_id   TEXT NOT NULL DEFAULT '',
    condition    JSONB,
    priority     INT NOT NULL DEFAULT 100,
    effect_mode  TEXT NOT NULL DEFAULT 'set' CHECK (effect_mode IN ('set','restrict_only')),
    note         TEXT NOT NULL DEFAULT '',
    created_by   UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS app_control_rules_org_idx ON adrient.app_control_rules (org_id, control);

-- App access policies (0118) may now target a user type and carry a condition.
DO $$
DECLARE c TEXT;
BEGIN
    FOR c IN SELECT conname FROM pg_constraint
             WHERE conrelid = 'adrient.app_access_policies'::regclass AND contype = 'c'
               AND pg_get_constraintdef(oid) LIKE '%subject_type%' LOOP
        EXECUTE format('ALTER TABLE adrient.app_access_policies DROP CONSTRAINT %I', c);
    END LOOP;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'app_access_policies_subject_chk') THEN
        ALTER TABLE adrient.app_access_policies ADD CONSTRAINT app_access_policies_subject_chk
            CHECK (subject_type IN ('everyone','user','group','department','role','user_type'));
    END IF;
END $$;
ALTER TABLE adrient.app_access_policies ADD COLUMN IF NOT EXISTS condition JSONB;
