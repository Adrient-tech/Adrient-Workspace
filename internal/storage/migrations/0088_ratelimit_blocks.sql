-- 0088_ratelimit_blocks.sql
-- Observability for the per-IP API rate limiter (internal/ratelimit). When the
-- limiter rejects a request (429), it records a best-effort block event here so
-- the admin console can surface recent throttling + the top offending IPs.
--
-- Instance-global by design: the limiter keys on client IP and fires on requests
-- that may be unauthenticated (it sits OUTERMOST, before the auth wall), so there
-- is no org to scope to. This mirrors adrient.honeypot_alerts (0087) and
-- adrient.geo_access (0085), which are likewise instance-level side-tables.
CREATE TABLE IF NOT EXISTS adrient.ratelimit_blocks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip          TEXT NOT NULL DEFAULT '',   -- best-effort client IP (XFF first hop)
    path        TEXT NOT NULL DEFAULT '',   -- the throttled request path
    bucket      TEXT NOT NULL DEFAULT '',   -- which bucket rejected: 'general' | 'auth'
    country     TEXT NOT NULL DEFAULT '',   -- CF-IPCountry, when present
    user_agent  TEXT NOT NULL DEFAULT '',   -- request User-Agent
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ratelimit_blocks_created_idx ON adrient.ratelimit_blocks (created_at DESC);
CREATE INDEX IF NOT EXISTS ratelimit_blocks_ip_idx      ON adrient.ratelimit_blocks (ip);
