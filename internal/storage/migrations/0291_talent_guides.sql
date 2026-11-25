-- Talent wave 6: reusable interview guides (question sets) and guide answers on scorecards.
-- Idempotent.
CREATE TABLE IF NOT EXISTS adrient.talent_guides (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    name       TEXT NOT NULL,
    stage      TEXT NOT NULL DEFAULT '',          -- pipeline stage id this guide suits ('' = any)
    format     TEXT NOT NULL DEFAULT '',          -- '', phone, video, onsite
    questions  JSONB NOT NULL DEFAULT '[]'::jsonb, -- [{"text": "...", "dimension": "..."}]
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS talent_guides_org_idx ON adrient.talent_guides (org_id);

ALTER TABLE adrient.talent_scorecards ADD COLUMN IF NOT EXISTS guide_id UUID;
ALTER TABLE adrient.talent_scorecards ADD COLUMN IF NOT EXISTS answers  JSONB NOT NULL DEFAULT '[]'::jsonb; -- snapshot [{"question","answer"}]
