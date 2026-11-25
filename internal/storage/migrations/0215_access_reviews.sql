-- 0215: access review campaigns (group membership, external guests, admin roles).
CREATE TABLE IF NOT EXISTS adrient.access_review_campaigns (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    kind           TEXT NOT NULL CHECK (kind IN ('group_membership','external_guests','admin_roles')),
    scope          JSONB NOT NULL DEFAULT '{}'::jsonb,
    due_at         TIMESTAMPTZ NOT NULL,
    on_no_response TEXT NOT NULL DEFAULT 'keep' CHECK (on_no_response IN ('keep','remove')),
    recurrence_days INT CHECK (recurrence_days IS NULL OR recurrence_days >= 7),
    status         TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
    created_by     UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS access_review_campaigns_org_idx ON adrient.access_review_campaigns (org_id, status);

CREATE TABLE IF NOT EXISTS adrient.access_review_items (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id   UUID NOT NULL REFERENCES adrient.access_review_campaigns(id) ON DELETE CASCADE,
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    subject_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL CHECK (resource_type IN ('group','guest','admin_role')),
    resource_id   TEXT NOT NULL DEFAULT '',
    resource_name TEXT NOT NULL DEFAULT '',
    reviewer_id   UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    decision      TEXT NOT NULL DEFAULT 'pending' CHECK (decision IN ('pending','keep','remove')),
    decided_by    UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    decided_at    TIMESTAMPTZ,
    comment       TEXT NOT NULL DEFAULT '',
    applied_at    TIMESTAMPTZ,
    apply_result  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS access_review_items_campaign_idx ON adrient.access_review_items (campaign_id);
CREATE INDEX IF NOT EXISTS access_review_items_reviewer_idx ON adrient.access_review_items (reviewer_id) WHERE decision = 'pending';
