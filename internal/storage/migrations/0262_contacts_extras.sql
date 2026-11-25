-- Contacts extras: birthday/anniversary/address/website per contact, and a
-- change journal that powers "Undo changes". Idempotent.
CREATE TABLE IF NOT EXISTS adrient.contact_extras (
    contact_id  UUID PRIMARY KEY REFERENCES adrient.contacts(id) ON DELETE CASCADE,
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    birthday    DATE,
    anniversary DATE,
    website     TEXT NOT NULL DEFAULT '',
    address     TEXT NOT NULL DEFAULT '',
    nickname    TEXT NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS contact_extras_org_idx ON adrient.contact_extras (org_id);

CREATE TABLE IF NOT EXISTS adrient.contact_history (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    contact_id UUID NOT NULL,
    owner_id   UUID NOT NULL,
    action     TEXT NOT NULL,                 -- update | trash
    snapshot   JSONB NOT NULL,                -- the contact row before the change
    undone_at  TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS contact_history_owner_idx ON adrient.contact_history (org_id, owner_id, created_at DESC);
