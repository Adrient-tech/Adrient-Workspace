-- 0366: composite audit indexes for the common two-filter views
-- (service + action, service + status), keyset ordered.
CREATE INDEX IF NOT EXISTS audit_events_org_service_action_ts_idx
    ON adrient.audit_events (org_id, service, action, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS audit_events_org_service_status_ts_idx
    ON adrient.audit_events (org_id, service, status, created_at DESC, id DESC);
