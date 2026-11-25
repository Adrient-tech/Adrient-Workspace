-- 0331: Cap table (stakeholders, share classes, rounds, securities, events, saved scenarios, named access). Idempotent.
-- Share counts are BIGINT. Money is BIGINT minor units (paise/cents). A price per share is NUMERIC(24,8) in
-- currency units and is read back as text so no float ever touches it.

CREATE TABLE IF NOT EXISTS adrient.captable_company (
    org_id        UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name          TEXT NOT NULL DEFAULT '',
    currency      TEXT NOT NULL DEFAULT 'INR',
    incorporated  DATE,
    notes         TEXT NOT NULL DEFAULT ''
);

-- Who may open the cap table. Org admins always may. Everyone else needs a row here.
CREATE TABLE IF NOT EXISTS adrient.captable_access (
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL,
    level       TEXT NOT NULL DEFAULT 'view' CHECK (level IN ('view','edit')),
    created_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.captable_stakeholders (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'employee' CHECK (kind IN ('founder','employee','investor','advisor','entity')),
    email       TEXT NOT NULL DEFAULT '',
    contact_id  UUID,
    user_id     UUID,
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS captable_stakeholders_org_idx ON adrient.captable_stakeholders (org_id, lower(name));

CREATE TABLE IF NOT EXISTS adrient.captable_classes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    kind                TEXT NOT NULL DEFAULT 'common' CHECK (kind IN ('common','preferred')),
    seniority           INT NOT NULL DEFAULT 0,
    pref_multiple_milli INT NOT NULL DEFAULT 1000,      -- 1000 = 1.0x
    participating       BOOLEAN NOT NULL DEFAULT false,
    convert_ratio_milli INT NOT NULL DEFAULT 1000,      -- 1000 = 1:1
    issue_price         NUMERIC(24,8) NOT NULL DEFAULT 0,
    authorized          BIGINT NOT NULL DEFAULT 0,      -- 0 = not limited
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, name)
);

CREATE TABLE IF NOT EXISTS adrient.captable_pools (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    size        BIGINT NOT NULL CHECK (size >= 0),
    adopted     DATE NOT NULL,
    notes       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS adrient.captable_rounds (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    kind              TEXT NOT NULL DEFAULT 'priced' CHECK (kind IN ('founding','priced','bridge','other')),
    round_date        DATE NOT NULL,
    pre_money         BIGINT NOT NULL DEFAULT 0,
    investment        BIGINT NOT NULL DEFAULT 0,
    price_per_share   NUMERIC(24,8) NOT NULL DEFAULT 0,
    class_id          UUID REFERENCES adrient.captable_classes(id) ON DELETE SET NULL,
    notes             TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.captable_securities (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    label          TEXT NOT NULL,
    kind           TEXT NOT NULL CHECK (kind IN ('stock','option','warrant','safe','note')),
    stakeholder_id UUID NOT NULL REFERENCES adrient.captable_stakeholders(id) ON DELETE RESTRICT,
    class_id       UUID REFERENCES adrient.captable_classes(id) ON DELETE RESTRICT,
    pool_id        UUID REFERENCES adrient.captable_pools(id) ON DELETE SET NULL,
    round_id       UUID REFERENCES adrient.captable_rounds(id) ON DELETE SET NULL,
    parent_id      UUID,
    quantity       BIGINT NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    price          NUMERIC(24,8) NOT NULL DEFAULT 0,     -- issue price (stock) or strike (option, warrant)
    issue_date     DATE NOT NULL,
    vest_months    INT NOT NULL DEFAULT 0,
    cliff_months   INT NOT NULL DEFAULT 0,
    vest_freq      TEXT NOT NULL DEFAULT 'monthly' CHECK (vest_freq IN ('monthly','quarterly','annual')),
    vest_start     DATE,
    expires        DATE,
    principal      BIGINT NOT NULL DEFAULT 0,           -- SAFE / note money in minor units
    cap            BIGINT NOT NULL DEFAULT 0,
    discount_bp    INT NOT NULL DEFAULT 0,
    post_money     BOOLEAN NOT NULL DEFAULT false,
    interest_bp    INT NOT NULL DEFAULT 0,
    notes          TEXT NOT NULL DEFAULT '',
    created_by     UUID,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, label)
);
CREATE INDEX IF NOT EXISTS captable_securities_holder_idx ON adrient.captable_securities (org_id, stakeholder_id);

-- Exercises, cancellations, transfers and conversions. The cap table on any date is derived from securities and these.
CREATE TABLE IF NOT EXISTS adrient.captable_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('exercise','cancel','transfer','convert')),
    security_id     UUID NOT NULL REFERENCES adrient.captable_securities(id) ON DELETE CASCADE,
    event_date      DATE NOT NULL,
    quantity        BIGINT NOT NULL DEFAULT 0,
    to_stakeholder  UUID,
    new_security_id UUID,
    price           NUMERIC(24,8) NOT NULL DEFAULT 0,
    note            TEXT NOT NULL DEFAULT '',
    by_user         UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS captable_events_sec_idx ON adrient.captable_events (security_id);

CREATE TABLE IF NOT EXISTS adrient.captable_scenarios (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('round','exit')),
    name        TEXT NOT NULL,
    params      JSONB NOT NULL DEFAULT '{}',
    created_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS captable_scenarios_org_idx ON adrient.captable_scenarios (org_id, kind);
