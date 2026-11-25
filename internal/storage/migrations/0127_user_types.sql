-- 0127: User types and lifecycle state (identity plan section B).
-- user_type: member (default), external (contractor), guest (B2B), service.
-- status: active, blocked, expired is derived (expires_at passed) and then
-- stored as blocked by the daily expiry job with blocked_reason = 'expired'.

ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS user_type TEXT NOT NULL DEFAULT 'member';
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS sponsor_id UUID REFERENCES adrient.users(id) ON DELETE SET NULL;
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS blocked_at TIMESTAMPTZ;
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS blocked_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS last_sign_in_at TIMESTAMPTZ;
-- Apps a guest may open (everything else is closed to guests).
ALTER TABLE adrient.users ADD COLUMN IF NOT EXISTS guest_apps TEXT[] NOT NULL DEFAULT '{}';

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_user_type_chk') THEN
        ALTER TABLE adrient.users ADD CONSTRAINT users_user_type_chk
            CHECK (user_type IN ('member', 'external', 'guest', 'service'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_status_chk') THEN
        ALTER TABLE adrient.users ADD CONSTRAINT users_status_chk
            CHECK (status IN ('active', 'blocked', 'expired'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS users_org_type_idx ON adrient.users (org_id, user_type, status);
CREATE INDEX IF NOT EXISTS users_expiry_idx ON adrient.users (expires_at) WHERE expires_at IS NOT NULL;
