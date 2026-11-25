-- 0355: Stories (the Podcasts app, rebuilt as an inside-the-company audio stories platform).
--
-- A "show" is a story or a series; it holds ordered episodes. The text of an
-- episode is the source of truth: audio is generated from it by text-to-speech
-- (see 0356) or read aloud by the listener's browser. Cover art is either an
-- uploaded image (a blob key) or a generated gradient (cover_style).
-- The old RSS subscriptions table (0090) is unchanged: it backs "Shows from the web".

CREATE TABLE IF NOT EXISTS adrient.story_shows (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    author_id      UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    title          TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    genre          TEXT NOT NULL DEFAULT 'General',
    language       TEXT NOT NULL DEFAULT 'en',
    cover_blob     TEXT NOT NULL DEFAULT '',
    cover_mime     TEXT NOT NULL DEFAULT '',
    cover_style    TEXT NOT NULL DEFAULT '',
    audience       TEXT NOT NULL DEFAULT 'everyone' CHECK (audience IN ('everyone','group','department')),
    audience_id    UUID,
    comments_on    BOOLEAN NOT NULL DEFAULT true,
    allow_download BOOLEAN NOT NULL DEFAULT false,
    voice_provider UUID,
    voice          TEXT NOT NULL DEFAULT '',
    voice_map      JSONB NOT NULL DEFAULT '{}',
    speed          REAL NOT NULL DEFAULT 1.0,
    pitch          REAL NOT NULL DEFAULT 1.0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS story_shows_org_idx ON adrient.story_shows (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS story_shows_author_idx ON adrient.story_shows (org_id, author_id);

CREATE TABLE IF NOT EXISTS adrient.story_episodes (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    show_id        UUID NOT NULL REFERENCES adrient.story_shows(id) ON DELETE CASCADE,
    position       INT NOT NULL DEFAULT 1,
    title          TEXT NOT NULL,
    summary        TEXT NOT NULL DEFAULT '',
    body_html      TEXT NOT NULL DEFAULT '',
    body_text      TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','scheduled','published','unpublished')),
    publish_at     TIMESTAMPTZ,
    published_at   TIMESTAMPTZ,
    -- Per-episode voice settings; empty means "use the show's".
    voice_provider UUID,
    voice          TEXT NOT NULL DEFAULT '',
    voice_map      JSONB,
    speed          REAL,
    pitch          REAL,
    -- Generated audio.
    audio_status   TEXT NOT NULL DEFAULT 'none' CHECK (audio_status IN ('none','queued','generating','ready','failed')),
    audio_hash     TEXT NOT NULL DEFAULT '',
    audio_blob     TEXT NOT NULL DEFAULT '',
    audio_mime     TEXT NOT NULL DEFAULT '',
    audio_bytes    BIGINT NOT NULL DEFAULT 0,
    audio_ms       INT NOT NULL DEFAULT 0,
    audio_error    TEXT NOT NULL DEFAULT '',
    audio_timings  JSONB NOT NULL DEFAULT '[]',
    audio_at       TIMESTAMPTZ,
    -- Duration estimate for the listener when there is no audio yet (browser voice).
    est_ms         INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS story_episodes_show_idx ON adrient.story_episodes (show_id, position);
CREATE INDEX IF NOT EXISTS story_episodes_org_status_idx ON adrient.story_episodes (org_id, status, published_at DESC);
CREATE INDEX IF NOT EXISTS story_episodes_due_idx ON adrient.story_episodes (publish_at) WHERE status = 'scheduled';
CREATE INDEX IF NOT EXISTS story_episodes_hash_idx ON adrient.story_episodes (org_id, audio_hash) WHERE audio_status = 'ready';

-- Listening progress per user and episode.
CREATE TABLE IF NOT EXISTS adrient.story_progress (
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    episode_id  UUID NOT NULL REFERENCES adrient.story_episodes(id) ON DELETE CASCADE,
    show_id     UUID NOT NULL REFERENCES adrient.story_shows(id) ON DELETE CASCADE,
    position_ms INT NOT NULL DEFAULT 0,
    duration_ms INT NOT NULL DEFAULT 0,
    completed   BOOLEAN NOT NULL DEFAULT false,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, episode_id)
);
CREATE INDEX IF NOT EXISTS story_progress_user_idx ON adrient.story_progress (org_id, user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS story_progress_episode_idx ON adrient.story_progress (episode_id);

-- One row per "started listening" (for the author's play counts).
CREATE TABLE IF NOT EXISTS adrient.story_plays (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    episode_id UUID NOT NULL REFERENCES adrient.story_episodes(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS story_plays_episode_idx ON adrient.story_plays (episode_id, created_at DESC);
