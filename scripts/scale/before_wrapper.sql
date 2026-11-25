-- Drops the wave-9 indexes inside a transaction, runs the old-query timings, then rolls back.
BEGIN;
DROP INDEX IF EXISTS adrient.users_name_trgm_idx, adrient.users_email_trgm_idx, adrient.users_employee_trgm_idx,
  adrient.users_org_name_key_idx, adrient.users_org_lower_email_idx, adrient.users_org_lower_name_idx,
  adrient.audit_events_org_ts_id_idx, adrient.acct_journals_org_date_id_idx, adrient.acct_journals_org_created_idx;
\i scripts/scale/before_after.sql
ROLLBACK;
