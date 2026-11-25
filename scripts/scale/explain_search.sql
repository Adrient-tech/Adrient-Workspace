EXPLAIN ANALYZE
SELECT u.id FROM adrient.users u
WHERE u.org_id = (SELECT id FROM adrient.orgs WHERE slug = 'default')
  AND (u.display_name ILIKE '%E00123%' OR u.email ILIKE '%E00123%' OR u.employee_id ILIKE '%E00123%' OR u.upn ILIKE '%E00123%'
       OR u.department_id = ANY (ARRAY[]::uuid[]))
ORDER BY lower(COALESCE(NULLIF(u.display_name,''), u.email)), u.id LIMIT 31;
