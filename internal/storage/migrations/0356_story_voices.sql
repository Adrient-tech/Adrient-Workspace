-- 0356: Stories text-to-speech: voice providers (configured by an admin) and the
-- audio generation queue. Secrets (API keys) are sealed with ADRIENT_SECRETS_KEY or
-- read from an environment variable named in key_env; they are never returned.

CREATE TABLE IF NOT EXISTS adrient.story_voice_providers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('openai','command')),
    base_url     TEXT NOT NULL DEFAULT '',
    key_sealed   BYTEA,
    key_env      TEXT NOT NULL DEFAULT '',
    model        TEXT NOT NULL DEFAULT '',
    format       TEXT NOT NULL DEFAULT 'wav',
    command      TEXT NOT NULL DEFAULT '',
    args         JSONB NOT NULL DEFAULT '[]',
    voices_dir   TEXT NOT NULL DEFAULT '',
    voices       JSONB NOT NULL DEFAULT '[]',
    default_voice TEXT NOT NULL DEFAULT '',
    enabled      BOOLEAN NOT NULL DEFAULT true,
    is_default   BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS story_voice_providers_org_idx ON adrient.story_voice_providers (org_id);

-- Org-wide voice settings (one row per org).
CREATE TABLE IF NOT EXISTS adrient.story_settings (
    org_id          UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    daily_chars     INT NOT NULL DEFAULT 200000,
    per_minute_jobs INT NOT NULL DEFAULT 6,
    max_chars       INT NOT NULL DEFAULT 60000,
    auto_generate   BOOLEAN NOT NULL DEFAULT true,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Audio generation jobs. A worker claims queued rows with FOR UPDATE SKIP LOCKED.
CREATE TABLE IF NOT EXISTS adrient.story_audio_jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    episode_id  UUID NOT NULL REFERENCES adrient.story_episodes(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','done','failed','cancelled')),
    chars       INT NOT NULL DEFAULT 0,
    progress    INT NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS story_audio_jobs_queue_idx ON adrient.story_audio_jobs (created_at) WHERE status = 'queued';
CREATE INDEX IF NOT EXISTS story_audio_jobs_user_idx ON adrient.story_audio_jobs (org_id, user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS story_audio_jobs_episode_idx ON adrient.story_audio_jobs (episode_id, created_at DESC);
