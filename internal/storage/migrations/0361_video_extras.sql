-- 0361: Video extras: visibility, processing state, views and watch time,
-- likes, threaded comments, subscriptions, watch later, Drive-sourced videos.
-- Idempotent; existing videos stay visible to the whole org as before.

ALTER TABLE adrient.videos ADD COLUMN IF NOT EXISTS visibility     TEXT NOT NULL DEFAULT 'org';
ALTER TABLE adrient.videos ADD COLUMN IF NOT EXISTS group_id       UUID;
ALTER TABLE adrient.videos ADD COLUMN IF NOT EXISTS status         TEXT NOT NULL DEFAULT 'ready';
ALTER TABLE adrient.videos ADD COLUMN IF NOT EXISTS source_file_id  UUID;
ALTER TABLE adrient.videos ADD COLUMN IF NOT EXISTS source_kind    TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS adrient.video_views (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id        UUID NOT NULL REFERENCES adrient.videos(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES adrient.users(id)  ON DELETE CASCADE,
    watched_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS video_views_video_idx ON adrient.video_views (video_id);
CREATE INDEX IF NOT EXISTS video_views_user_idx  ON adrient.video_views (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS adrient.video_likes (
    video_id   UUID NOT NULL REFERENCES adrient.videos(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id)  ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (video_id, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.video_comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id)   ON DELETE CASCADE,
    video_id   UUID NOT NULL REFERENCES adrient.videos(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id)  ON DELETE CASCADE,
    parent_id  UUID REFERENCES adrient.video_comments(id)  ON DELETE CASCADE,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS video_comments_video_idx ON adrient.video_comments (video_id, created_at);

CREATE TABLE IF NOT EXISTS adrient.video_comment_likes (
    comment_id UUID NOT NULL REFERENCES adrient.video_comments(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id)          ON DELETE CASCADE,
    PRIMARY KEY (comment_id, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.video_subscriptions (
    user_id         UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    channel_user_id UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, channel_user_id)
);
CREATE INDEX IF NOT EXISTS video_subscriptions_channel_idx ON adrient.video_subscriptions (channel_user_id);

CREATE TABLE IF NOT EXISTS adrient.video_watch_later (
    user_id  UUID NOT NULL REFERENCES adrient.users(id)  ON DELETE CASCADE,
    video_id UUID NOT NULL REFERENCES adrient.videos(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, video_id)
);
