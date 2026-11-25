-- Scale seeding for a SCRATCH database only (never the shared `adrient` DB).
--   psql -v org=default -f scripts/scale/seed_scale.sql <scratch-db>
-- Creates fictional data: 20,000 users, 60 departments, 2,000 groups,
-- 500,000 audit rows, 300,000 sign-in events, 100,000 journals (transactions),
-- 100,000 CRM deals and 20,000 HR employees. Re-running skips what exists.
-- It refuses to run against a database named adrient or zitadel.
\set ON_ERROR_STOP on
SELECT current_database() IN ('adrient', 'zitadel') AS refuse \gset
\if :refuse
  \echo 'refusing to seed the shared database'
  \quit
\endif

SELECT id AS org_id FROM adrient.orgs WHERE slug = :'org' \gset

CREATE TEMP TABLE firsts(n int, v text);
INSERT INTO firsts(n, v) SELECT i, v FROM unnest(ARRAY['Asha','Bram','Chen','Dara','Elio','Fatima','Goran','Hana','Ivo','Jaya','Kofi','Lena','Mika','Nadia','Omar','Priya','Quinn','Rafa','Sana','Tomas','Uma','Vik','Wen','Xavi','Yara','Zane','Anil','Bea','Cyrus','Dina']) WITH ORDINALITY AS t(v, i);
CREATE TEMP TABLE lasts(n int, v text);
INSERT INTO lasts(n, v) SELECT i, v FROM unnest(ARRAY['Rao','Okafor','Lindqvist','Haddad','Moreau','Tanaka','Silva','Kowalski','Mehta','Novak','Quispe','Farouk','Ivanov','Nguyen','Bianchi','Adeyemi','Kaur','Larsen','Duarte','Petrov','Hughes','Sato','Costa','Mwangi','Reyes','Fischer','Iyer','Zhou','Banerjee','Dubois']) WITH ORDINALITY AS t(v, i);

INSERT INTO adrient.departments (org_id, name)
SELECT :'org_id'::uuid, 'Dept ' || g FROM generate_series(1, 60) g
ON CONFLICT DO NOTHING;

INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, job_title, employee_id,
                           department_id, upn, status, created_at, last_sign_in_at)
SELECT :'org_id'::uuid, 'scale-seed', 'scale-' || g,
       lower(f.v || '.' || l.v || g) || '@scale.example',
       f.v || ' ' || l.v,
       (ARRAY['Engineer','Analyst','Manager','Designer','Accountant','Support lead','Recruiter'])[1 + g % 7],
       'E' || lpad(g::text, 6, '0'),
       (SELECT id FROM adrient.departments WHERE org_id = :'org_id'::uuid AND name = 'Dept ' || (1 + g % 60)),
       lower(f.v || '.' || l.v || g) || '@scale.example',
       CASE WHEN g % 40 = 0 THEN 'blocked' ELSE 'active' END,
       now() - (g || ' minutes')::interval,
       now() - ((g * 7) % 100000 || ' seconds')::interval
FROM generate_series(1, 20000) g
JOIN firsts f ON f.n = 1 + g % 30
JOIN lasts l ON l.n = 1 + (g / 30) % 30
ON CONFLICT DO NOTHING;

INSERT INTO adrient.groups (org_id, owner_id, name, email, description)
SELECT :'org_id'::uuid, (SELECT id FROM adrient.users WHERE org_id = :'org_id'::uuid AND oidc_subject = 'scale-1'),
       'Team ' || g, 'team' || g || '@scale.example', 'Fictional group ' || g
FROM generate_series(1, 2000) g
WHERE NOT EXISTS (SELECT 1 FROM adrient.groups WHERE org_id = :'org_id'::uuid AND name = 'Team ' || g);

CREATE TEMP TABLE seeded AS
SELECT (row_number() OVER (ORDER BY oidc_subject))::int AS n, id, email
FROM adrient.users WHERE org_id = :'org_id'::uuid AND oidc_subject LIKE 'scale-%';
CREATE UNIQUE INDEX ON seeded(n);
ANALYZE seeded;

-- Audit rows, many sharing a timestamp so keyset ties are exercised.
INSERT INTO adrient.audit_events (org_id, actor_id, actor_email, service, action, resource_type, resource_id,
                                  method, status, ip, user_agent, created_at)
SELECT :'org_id'::uuid, u.id, u.email,
       (ARRAY['drive','docs','mail','calendar','admin','hr','crm','chat'])[1 + g % 8],
       (ARRAY['create','update','delete','share','view','denied'])[1 + g % 6],
       'item', 'res-' || (g % 5000), 'POST /api/v1/x', CASE WHEN g % 13 = 0 THEN 'error' ELSE 'ok' END,
       '10.0.' || (g % 250) || '.' || (g % 200), 'seed-agent',
       now() - ((g / 3) || ' seconds')::interval
FROM generate_series(1, 500000) g
JOIN seeded u ON u.n = 1 + g % (SELECT count(*) FROM seeded)
WHERE (SELECT count(*) FROM adrient.audit_events WHERE org_id = :'org_id'::uuid AND ip LIKE '10.0.%' AND user_agent = 'seed-agent') < 1000;

INSERT INTO adrient.login_events (org_id, user_id, email, ip, user_agent, method, success, reason, suspicious, created_at)
SELECT :'org_id'::uuid, u.id, u.email, '10.1.' || (g % 250) || '.' || (g % 200), 'seed-agent', 'password',
       g % 11 <> 0, CASE WHEN g % 11 = 0 THEN 'bad password' ELSE '' END, g % 97 = 0,
       now() - (g || ' seconds')::interval
FROM generate_series(1, 300000) g
JOIN seeded u ON u.n = 1 + g % (SELECT count(*) FROM seeded)
WHERE (SELECT count(*) FROM adrient.login_events WHERE org_id = :'org_id'::uuid AND user_agent = 'seed-agent') < 1000;

INSERT INTO adrient.acct_journals (org_id, number, entry_date, memo, ref, kind, source, contact_name)
SELECT :'org_id'::uuid, 900000 + g, current_date - (g % 900), 'Sale ' || g, 'INV-' || g,
       (ARRAY['invoice','sale','payment','expense','bill'])[1 + g % 5], 'manual', 'Customer ' || (g % 3000)
FROM generate_series(1, 100000) g
WHERE (SELECT count(*) FROM adrient.acct_journals WHERE org_id = :'org_id'::uuid AND number >= 900000) < 1000;

INSERT INTO adrient.crm_deals (org_id, title, stage, value, source, created_at)
SELECT :'org_id'::uuid, 'Deal ' || g, (ARRAY['new','qualified','proposal','won','lost'])[1 + g % 5],
       (g % 1000) * 10, 'seed', now() - (g || ' minutes')::interval
FROM generate_series(1, 100000) g
WHERE (SELECT count(*) FROM adrient.crm_deals WHERE org_id = :'org_id'::uuid AND source = 'seed') < 1000;

INSERT INTO adrient.hr_employees (org_id, user_id, emp_code, first_name, last_name, work_email, join_date, department_id)
SELECT u.org_id, u.id, 'S' || u.employee_id, split_part(u.display_name, ' ', 1), split_part(u.display_name, ' ', 2),
       u.email, current_date - (row_number() OVER () % 3000)::int, u.department_id
FROM adrient.users u WHERE u.org_id = :'org_id'::uuid AND u.oidc_subject LIKE 'scale-%'
ON CONFLICT DO NOTHING;

ANALYZE adrient.users; ANALYZE adrient.audit_events; ANALYZE adrient.login_events;
ANALYZE adrient.acct_journals; ANALYZE adrient.crm_deals; ANALYZE adrient.hr_employees; ANALYZE adrient.groups;
SELECT (SELECT count(*) FROM adrient.users) users, (SELECT count(*) FROM adrient.audit_events) audit,
       (SELECT count(*) FROM adrient.login_events) logins, (SELECT count(*) FROM adrient.acct_journals) journals,
       (SELECT count(*) FROM adrient.crm_deals) deals, (SELECT count(*) FROM adrient.hr_employees) employees;
