-- 0119: Teams-style channel tabs. Each channel/DM can pin a Doc, Sheet,
-- Slides deck, Whiteboard, Tasks list, Project board or a link as a tab.
CREATE TABLE IF NOT EXISTS adrient.chat_tabs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID NOT NULL REFERENCES adrient.chat_channels(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL DEFAULT 'link',
    title      TEXT NOT NULL,
    url        TEXT NOT NULL,
    position   INT NOT NULL DEFAULT 0,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS chat_tabs_channel_idx ON adrient.chat_tabs (channel_id, position, created_at);
