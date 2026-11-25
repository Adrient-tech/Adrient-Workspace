-- 0135: server-side call history (Chat > Calls). One row per call, one party
-- row per person rung. A party still 'ringing' after the ring window is shown
-- as missed.
CREATE TABLE IF NOT EXISTS adrient.chat_calls (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    code        TEXT NOT NULL,
    caller_id   UUID NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    video       BOOLEAN NOT NULL DEFAULT false,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at TIMESTAMPTZ,
    ended_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS chat_calls_code_idx ON adrient.chat_calls (org_id, code, started_at DESC);
CREATE INDEX IF NOT EXISTS chat_calls_caller_idx ON adrient.chat_calls (caller_id, started_at DESC);
CREATE TABLE IF NOT EXISTS adrient.chat_call_parties (
    call_id      UUID NOT NULL REFERENCES adrient.chat_calls(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL,
    status       TEXT NOT NULL DEFAULT 'ringing' CHECK (status IN ('ringing', 'accepted', 'declined', 'missed')),
    responded_at TIMESTAMPTZ,
    PRIMARY KEY (call_id, user_id)
);
CREATE INDEX IF NOT EXISTS chat_call_parties_user_idx ON adrient.chat_call_parties (user_id);
