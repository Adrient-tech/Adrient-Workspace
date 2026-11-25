-- 0141: Cross-organisation trust (plan C5). Both orgs must opt in; either side
-- may revoke. Enforcement for cross-org sharing/chat/meet reads this table.

CREATE TABLE IF NOT EXISTS adrient.cross_org_trusts (
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    partner_org_id UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','active','revoked')),
    inbound        JSONB NOT NULL DEFAULT '{}'::jsonb,
    outbound       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, partner_org_id),
    CHECK (org_id <> partner_org_id)
);
