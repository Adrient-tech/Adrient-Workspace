-- 0014: Whiteboards (Excalidraw drawing surfaces).
--
-- The scene contents are an opaque JSON blob (the Excalidraw scene: elements +
-- appState + files), autosaved by the editor.

CREATE TABLE IF NOT EXISTS adrient.whiteboards (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id)  ON DELETE RESTRICT,
    owner_id      UUID NOT NULL REFERENCES adrient.users(id) ON DELETE RESTRICT,
    title         TEXT NOT NULL DEFAULT 'Untitled whiteboard',
    data          TEXT,
    trashed_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS whiteboards_org_idx
  ON adrient.whiteboards (org_id, owner_id) WHERE trashed_at IS NULL;
