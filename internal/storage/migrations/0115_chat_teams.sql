-- 0115: Chat -> Teams-class upgrade.
--
-- Adds public/private channels (kind), topic/description/archive, per-user
-- sidebar prefs (star/mute/section), custom sections, message edit + mentions,
-- pins, bookmarks, saved ("Later"), reminders, drafts, scheduled sends, user
-- profiles (title/status) and a per-channel huddle code (Meet room).
-- One Adrient org == one workspace, so there is no workspace table.

ALTER TABLE adrient.chat_channels DROP CONSTRAINT IF EXISTS chat_channels_kind_check;
ALTER TABLE adrient.chat_channels
    ADD CONSTRAINT chat_channels_kind_check
    CHECK (kind IN ('dm', 'group', 'public', 'private'));

ALTER TABLE adrient.chat_channels
    ADD COLUMN IF NOT EXISTS topic       TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS archived    BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS created_by  UUID,
    ADD COLUMN IF NOT EXISTS huddle_code TEXT NOT NULL DEFAULT '';

ALTER TABLE adrient.chat_messages
    ADD COLUMN IF NOT EXISTS edited_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS mention_ids JSONB NOT NULL DEFAULT '[]';

CREATE INDEX IF NOT EXISTS chat_messages_mentions_idx
    ON adrient.chat_messages USING gin (mention_ids);
CREATE INDEX IF NOT EXISTS chat_messages_org_sent_idx
    ON adrient.chat_messages (org_id, sent_at DESC);

CREATE TABLE IF NOT EXISTS adrient.chat_sections (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id    UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id   UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    position  INT NOT NULL DEFAULT 0,
    collapsed BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS chat_sections_user_idx ON adrient.chat_sections (user_id);

-- Per-user, per-channel sidebar state.
CREATE TABLE IF NOT EXISTS adrient.chat_channel_prefs (
    channel_id UUID NOT NULL REFERENCES adrient.chat_channels(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    starred    BOOLEAN NOT NULL DEFAULT false,
    muted      BOOLEAN NOT NULL DEFAULT false,
    section_id UUID REFERENCES adrient.chat_sections(id) ON DELETE SET NULL,
    PRIMARY KEY (channel_id, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.chat_profiles (
    user_id       UUID PRIMARY KEY REFERENCES adrient.users(id) ON DELETE CASCADE,
    title         TEXT NOT NULL DEFAULT '',
    status_text   TEXT NOT NULL DEFAULT '',
    status_emoji  TEXT NOT NULL DEFAULT '',
    activity_seen_at TIMESTAMPTZ NOT NULL DEFAULT '-infinity'
);

CREATE TABLE IF NOT EXISTS adrient.chat_pins (
    message_id UUID PRIMARY KEY REFERENCES adrient.chat_messages(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES adrient.chat_channels(id) ON DELETE CASCADE,
    pinned_by  UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS chat_pins_channel_idx ON adrient.chat_pins (channel_id);

CREATE TABLE IF NOT EXISTS adrient.chat_bookmarks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID NOT NULL REFERENCES adrient.chat_channels(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    url        TEXT NOT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS chat_bookmarks_channel_idx ON adrient.chat_bookmarks (channel_id);

CREATE TABLE IF NOT EXISTS adrient.chat_saved (
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES adrient.chat_messages(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, message_id)
);

CREATE TABLE IF NOT EXISTS adrient.chat_reminders (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    message_id UUID REFERENCES adrient.chat_messages(id) ON DELETE CASCADE,
    note       TEXT NOT NULL DEFAULT '',
    remind_at  TIMESTAMPTZ NOT NULL,
    fired      BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS chat_reminders_due_idx ON adrient.chat_reminders (remind_at) WHERE NOT fired;

CREATE TABLE IF NOT EXISTS adrient.chat_drafts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES adrient.chat_channels(id) ON DELETE CASCADE,
    parent_id  TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, channel_id, parent_id)
);

CREATE TABLE IF NOT EXISTS adrient.chat_scheduled (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES adrient.chat_channels(id) ON DELETE CASCADE,
    parent_id  TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL,
    send_at    TIMESTAMPTZ NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'cancelled', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS chat_scheduled_due_idx ON adrient.chat_scheduled (send_at) WHERE status = 'pending';
