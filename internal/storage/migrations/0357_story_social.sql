-- 0357: Stories social layer: likes, library (bookmarks), follows, comments, reports.

CREATE TABLE IF NOT EXISTS adrient.story_likes (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    episode_id UUID NOT NULL REFERENCES adrient.story_episodes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, episode_id)
);
CREATE INDEX IF NOT EXISTS story_likes_episode_idx ON adrient.story_likes (episode_id);

-- Library: saved episodes and saved shows.
CREATE TABLE IF NOT EXISTS adrient.story_library (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('show','episode')),
    target_id  UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind, target_id)
);
CREATE INDEX IF NOT EXISTS story_library_user_idx ON adrient.story_library (org_id, user_id, created_at DESC);

-- Follows: a series or an author (target_id is a show id or a user id).
CREATE TABLE IF NOT EXISTS adrient.story_follows (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('show','author')),
    target_id  UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind, target_id)
);
CREATE INDEX IF NOT EXISTS story_follows_target_idx ON adrient.story_follows (kind, target_id);

CREATE TABLE IF NOT EXISTS adrient.story_comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    episode_id UUID NOT NULL REFERENCES adrient.story_episodes(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    body       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'visible' CHECK (status IN ('visible','hidden')),
    hidden_by  UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS story_comments_episode_idx ON adrient.story_comments (episode_id, created_at);

CREATE TABLE IF NOT EXISTS adrient.story_reports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    comment_id  UUID NOT NULL REFERENCES adrient.story_comments(id) ON DELETE CASCADE,
    reporter_id UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    reason      TEXT NOT NULL DEFAULT '',
    resolved    BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (comment_id, reporter_id)
);
