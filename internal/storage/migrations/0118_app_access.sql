-- 0118: Per-app access policies (A1).
--
-- Decides who may OPEN an app (launcher tile + API). Rules per (org, app):
--   * no rows                           -> everyone may open the app (default)
--   * a 'deny' row matching the caller  -> blocked (deny wins over allow)
--   * any 'allow' row exists            -> only callers matching an allow row
--                                          (an 'everyone' allow row = all)
-- subject_type: everyone | user | group | department | role
--   user       subject_id = adrient.users.id
--   group      subject_id = adrient.groups.id
--   department subject_id = adrient.departments.id; applies to that
--              department and every descendant department
--   role       subject_id = role key (member, manager, guest, app_admin, ...)
-- Org Owner/Admin and the app's own App admin are never locked out, and the
-- core apps (admin, settings, profile, search, notifications) ignore policies.
-- org_service_settings (org-wide on/off) stays the first filter in the UI.

CREATE TABLE IF NOT EXISTS adrient.app_access_policies (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    service_id    TEXT NOT NULL,
    subject_type  TEXT NOT NULL CHECK (subject_type IN ('everyone', 'user', 'group', 'department', 'role')),
    subject_id    TEXT NOT NULL DEFAULT '',
    effect        TEXT NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow', 'deny')),
    created_by    UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS app_access_policies_uq
    ON adrient.app_access_policies (org_id, service_id, subject_type, subject_id, effect);
CREATE INDEX IF NOT EXISTS app_access_policies_org_idx
    ON adrient.app_access_policies (org_id, service_id);
