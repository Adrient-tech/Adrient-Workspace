-- 0240: AI assistant gateway (internal/ai). Org settings, model providers with
-- encrypted keys, and a usage log for quotas and cost reporting. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.ai_settings (
    org_id     UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    settings   JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_by UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.ai_providers (
    id         UUID PRIMARY KEY,
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('openai','azure','anthropic','gemini')),
    base_url   TEXT NOT NULL DEFAULT '',
    models     JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- Name of an environment variable holding the key (must start with ADRIENT_AI_).
    key_env    TEXT NOT NULL DEFAULT '',
    -- AES-256-GCM sealed API key (ADRIENT_SECRETS_KEY), bound to org + provider id.
    key_sealed BYTEA,
    config     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ai_providers_org_idx ON adrient.ai_providers (org_id);

CREATE TABLE IF NOT EXISTS adrient.ai_usage (
    id            BIGSERIAL PRIMARY KEY,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id       UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    app           TEXT NOT NULL,
    provider_id   UUID,
    model         TEXT NOT NULL DEFAULT '',
    input_tokens  INT NOT NULL DEFAULT 0,
    output_tokens INT NOT NULL DEFAULT 0,
    cost_usd      NUMERIC(14,6) NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'ok',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ai_usage_org_time_idx ON adrient.ai_usage (org_id, created_at);
CREATE INDEX IF NOT EXISTS ai_usage_user_time_idx ON adrient.ai_usage (org_id, user_id, created_at);
