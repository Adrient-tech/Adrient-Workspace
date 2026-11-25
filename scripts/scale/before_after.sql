-- Times the queries behind the old and new list endpoints on the scale data.
-- Run: psql -d <scratch-db> -f scripts/scale/before_after.sql   (scratch databases only)
\set ON_ERROR_STOP on
\timing on
SELECT id AS org FROM adrient.orgs WHERE slug = 'default' \gset
\echo '--- picker typing "ana": old query (ILIKE, order by lower name, limit 50)'
SELECT count(*) FROM (SELECT id FROM adrient.users WHERE org_id = :'org' AND (display_name ILIKE '%ana%' OR email ILIKE '%ana%') ORDER BY lower(COALESCE(NULLIF(display_name,''), email)) LIMIT 50) x;
\echo '--- audit first page (old): ORDER BY created_at DESC LIMIT 100'
SELECT count(*) FROM (SELECT * FROM adrient.audit_events WHERE org_id = :'org' ORDER BY created_at DESC LIMIT 100) x;
\echo '--- audit filtered service+status (old): order by created_at'
SELECT count(*) FROM (SELECT * FROM adrient.audit_events WHERE org_id = :'org' AND service = 'hr' ORDER BY created_at DESC LIMIT 100) x;
\echo '--- audit deep page (old, offset-free keyset on created_at only)'
SELECT count(*) FROM (SELECT * FROM adrient.audit_events WHERE org_id = :'org' AND created_at < now() - interval '40 hours' ORDER BY created_at DESC LIMIT 100) x;
\echo '--- HR/People list (old): everyone, ordered by name (orgchart/All)'
SELECT count(*) FROM (SELECT u.id, u.display_name, d.name FROM adrient.users u LEFT JOIN adrient.departments d ON d.id = u.department_id WHERE u.org_id = :'org' ORDER BY lower(COALESCE(NULLIF(u.display_name,''), u.email))) x;
\echo '--- accounting transactions (old): newest 100 by entry date'
SELECT count(*) FROM (SELECT * FROM adrient.acct_journals WHERE org_id = :'org' ORDER BY entry_date DESC, number DESC LIMIT 100) x;
