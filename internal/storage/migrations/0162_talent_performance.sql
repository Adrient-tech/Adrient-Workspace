-- 0162: Talent performance (goals/OKRs, review cycles, 1:1s, feedback).

CREATE TABLE IF NOT EXISTS adrient.talent_goals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    owner_id    UUID NOT NULL,
    parent_id   UUID REFERENCES adrient.talent_goals(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL DEFAULT 'objective',
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    unit        TEXT NOT NULL DEFAULT '%',
    start_value NUMERIC(14,2) NOT NULL DEFAULT 0,
    target      NUMERIC(14,2) NOT NULL DEFAULT 100,
    current     NUMERIC(14,2) NOT NULL DEFAULT 0,
    due_date    DATE,
    visibility  TEXT NOT NULL DEFAULT 'private',
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT talent_goal_kind_chk CHECK (kind IN ('objective','key_result')),
    CONSTRAINT talent_goal_vis_chk CHECK (visibility IN ('private','team','company'))
);
CREATE INDEX IF NOT EXISTS talent_goals_owner_idx ON adrient.talent_goals (org_id, owner_id);

CREATE TABLE IF NOT EXISTS adrient.talent_review_cycles (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    start_date DATE NOT NULL,
    end_date   DATE NOT NULL,
    status     TEXT NOT NULL DEFAULT 'draft',
    kinds      TEXT[] NOT NULL DEFAULT '{self,manager}',
    scale      JSONB NOT NULL DEFAULT '[{"v":1,"label":"Needs improvement"},{"v":2,"label":"Partially meets"},{"v":3,"label":"Meets expectations"},{"v":4,"label":"Exceeds"},{"v":5,"label":"Outstanding"}]',
    questions  JSONB NOT NULL DEFAULT '["What went well this period?","What could be improved?"]',
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT talent_cycle_status_chk CHECK (status IN ('draft','active','closed','published'))
);

CREATE TABLE IF NOT EXISTS adrient.talent_reviews (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    cycle_id     UUID NOT NULL REFERENCES adrient.talent_review_cycles(id) ON DELETE CASCADE,
    subject_id   UUID NOT NULL,
    reviewer_id  UUID NOT NULL,
    kind         TEXT NOT NULL,
    rating       INT,
    answers      JSONB NOT NULL DEFAULT '[]',
    comment      TEXT NOT NULL DEFAULT '',
    submitted_at TIMESTAMPTZ,
    calibrated_rating INT,
    CONSTRAINT talent_review_kind_chk CHECK (kind IN ('self','manager','peer')),
    UNIQUE (cycle_id, subject_id, reviewer_id, kind)
);
CREATE INDEX IF NOT EXISTS talent_reviews_subject_idx ON adrient.talent_reviews (org_id, cycle_id, subject_id);

CREATE TABLE IF NOT EXISTS adrient.talent_one_on_ones (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    manager_id  UUID NOT NULL,
    report_id   UUID NOT NULL,
    meeting_date DATE NOT NULL,
    agenda      TEXT NOT NULL DEFAULT '',
    notes       TEXT NOT NULL DEFAULT '',
    actions     JSONB NOT NULL DEFAULT '[]',
    created_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.talent_feedback (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    about_id     UUID NOT NULL,
    from_id      UUID NOT NULL,
    requested_by UUID,
    prompt       TEXT NOT NULL DEFAULT '',
    response     TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'requested',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at  TIMESTAMPTZ,
    CONSTRAINT talent_fb_status_chk CHECK (status IN ('requested','answered','declined'))
);
