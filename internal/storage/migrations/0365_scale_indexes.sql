-- 0365: indexes for server-side search, keyset pagination and sorting at scale
-- (10,000+ users, millions of audit rows). Every statement is idempotent.
--
-- Trigram indexes need the pg_trgm extension. When the database role may not
-- create it, the DO block below leaves the extension out and the lower()
-- prefix indexes further down still serve prefix searches.

DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_trgm not available (%): directory search falls back to prefix indexes', SQLERRM;
END
$$;

-- People: ordered keyset scan (name, id) inside an org, and prefix lookups.
CREATE INDEX IF NOT EXISTS users_org_name_key_idx
    ON adrient.users (org_id, (lower(COALESCE(NULLIF(display_name,''), email))), id);
CREATE INDEX IF NOT EXISTS users_org_lower_email_idx
    ON adrient.users (org_id, (lower(email)) text_pattern_ops);
CREATE INDEX IF NOT EXISTS users_org_lower_name_idx
    ON adrient.users (org_id, (lower(display_name)) text_pattern_ops);
CREATE INDEX IF NOT EXISTS users_org_employee_idx
    ON adrient.users (org_id, (lower(employee_id)) text_pattern_ops) WHERE employee_id <> '';
CREATE INDEX IF NOT EXISTS users_org_upn_idx
    ON adrient.users (org_id, (lower(upn)) text_pattern_ops) WHERE upn IS NOT NULL;
CREATE INDEX IF NOT EXISTS users_org_dept_idx
    ON adrient.users (org_id, department_id) WHERE department_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS users_org_status_created_idx
    ON adrient.users (org_id, status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS users_org_last_signin_idx
    ON adrient.users (org_id, last_sign_in_at DESC NULLS LAST, id DESC);

-- Groups and departments.
CREATE INDEX IF NOT EXISTS groups_org_name_key_idx
    ON adrient.groups (org_id, (lower(name)), id);
CREATE INDEX IF NOT EXISTS departments_org_name_key_idx
    ON adrient.departments (org_id, (lower(name)), id);

-- Trigram (substring) search, only when the extension exists.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        EXECUTE 'CREATE INDEX IF NOT EXISTS users_name_trgm_idx ON adrient.users USING gin (display_name gin_trgm_ops)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS users_email_trgm_idx ON adrient.users USING gin (email gin_trgm_ops)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS users_employee_trgm_idx ON adrient.users USING gin (employee_id gin_trgm_ops)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS groups_name_trgm_idx ON adrient.groups USING gin (name gin_trgm_ops)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS departments_name_trgm_idx ON adrient.departments USING gin (name gin_trgm_ops)';
    END IF;
END
$$;

-- Audit log: stable keyset (created_at, id) for the unfiltered feed and for
-- each filter the viewer offers.
CREATE INDEX IF NOT EXISTS audit_events_org_ts_id_idx
    ON adrient.audit_events (org_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS audit_events_org_actor_ts_idx
    ON adrient.audit_events (org_id, actor_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS audit_events_org_service_ts_idx
    ON adrient.audit_events (org_id, service, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS audit_events_org_action_ts_idx
    ON adrient.audit_events (org_id, action, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS audit_events_org_status_ts_idx
    ON adrient.audit_events (org_id, status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS audit_events_org_resource_idx
    ON adrient.audit_events (org_id, resource_id, created_at DESC) WHERE resource_id <> '';

-- Sign-in history: per org feed and per user feed, plus the failure filter.
CREATE INDEX IF NOT EXISTS login_events_org_ts_idx
    ON adrient.login_events (org_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS login_events_org_success_idx
    ON adrient.login_events (org_id, success, id DESC);
CREATE INDEX IF NOT EXISTS login_events_user_ts_idx
    ON adrient.login_events (user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS login_events_org_email_idx
    ON adrient.login_events (org_id, (lower(email)) text_pattern_ops, id DESC);

-- Accounting.
CREATE INDEX IF NOT EXISTS acct_journals_org_date_id_idx
    ON adrient.acct_journals (org_id, entry_date DESC, id DESC);
CREATE INDEX IF NOT EXISTS acct_journals_org_created_idx
    ON adrient.acct_journals (org_id, created_at DESC, id DESC);
