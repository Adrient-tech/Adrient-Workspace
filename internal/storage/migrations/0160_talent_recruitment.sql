-- 0160: Talent / recruitment (ATS). People are referenced by org user id only.

CREATE TABLE IF NOT EXISTS adrient.talent_settings (
    org_id            UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    stages            JSONB NOT NULL DEFAULT '[{"id":"applied","name":"Applied"},{"id":"screening","name":"Screening"},{"id":"interview","name":"Interview"},{"id":"offer","name":"Offer"}]',
    hr_user_ids       UUID[] NOT NULL DEFAULT '{}',
    career_enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    career_title      TEXT NOT NULL DEFAULT 'Careers',
    career_blurb      TEXT NOT NULL DEFAULT '',
    privacy_text      TEXT NOT NULL DEFAULT 'I agree that my details are stored and used to assess my application and to contact me about this and future roles. I can ask for them to be deleted at any time.',
    retention_days    INT NOT NULL DEFAULT 365,
    resume_folder_id  UUID,
    currency          TEXT NOT NULL DEFAULT 'INR',
    offer_template    TEXT NOT NULL DEFAULT '',
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.talent_jobs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    title             TEXT NOT NULL,
    department        TEXT NOT NULL DEFAULT '',
    location          TEXT NOT NULL DEFAULT '',
    employment_type   TEXT NOT NULL DEFAULT 'full_time',
    skills            TEXT[] NOT NULL DEFAULT '{}',
    description       TEXT NOT NULL DEFAULT '',
    salary_min        NUMERIC(14,2),
    salary_max        NUMERIC(14,2),
    currency          TEXT NOT NULL DEFAULT 'INR',
    openings          INT NOT NULL DEFAULT 1,
    status            TEXT NOT NULL DEFAULT 'draft',
    approval_status   TEXT NOT NULL DEFAULT 'none',
    approver_id       UUID,
    recruiter_id      UUID,
    hiring_manager_id UUID,
    team_ids          UUID[] NOT NULL DEFAULT '{}',
    created_by        UUID,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at      TIMESTAMPTZ,
    closed_at         TIMESTAMPTZ,
    CONSTRAINT talent_jobs_status_chk CHECK (status IN ('draft','open','closed')),
    CONSTRAINT talent_jobs_approval_chk CHECK (approval_status IN ('none','pending','approved','rejected'))
);
CREATE INDEX IF NOT EXISTS talent_jobs_org_idx ON adrient.talent_jobs (org_id, status);

CREATE TABLE IF NOT EXISTS adrient.talent_candidates (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    email         TEXT NOT NULL,
    phone         TEXT NOT NULL DEFAULT '',
    location      TEXT NOT NULL DEFAULT '',
    source        TEXT NOT NULL DEFAULT 'career_page',
    source_detail TEXT NOT NULL DEFAULT '',
    resume_file_id UUID,
    resume_key    TEXT NOT NULL DEFAULT '',
    resume_name   TEXT NOT NULL DEFAULT '',
    resume_mime   TEXT NOT NULL DEFAULT '',
    consent_at    TIMESTAMPTZ,
    consent_text  TEXT NOT NULL DEFAULT '',
    talent_pool   BOOLEAN NOT NULL DEFAULT FALSE,
    tags          TEXT[] NOT NULL DEFAULT '{}',
    notes         TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS talent_candidates_email_uq ON adrient.talent_candidates (org_id, lower(email));

CREATE TABLE IF NOT EXISTS adrient.talent_applications (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    job_id           UUID NOT NULL REFERENCES adrient.talent_jobs(id) ON DELETE CASCADE,
    candidate_id     UUID NOT NULL REFERENCES adrient.talent_candidates(id) ON DELETE CASCADE,
    stage            TEXT NOT NULL DEFAULT 'applied',
    status           TEXT NOT NULL DEFAULT 'active',
    reject_reason    TEXT NOT NULL DEFAULT '',
    cover_note       TEXT NOT NULL DEFAULT '',
    applied_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    stage_entered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at        TIMESTAMPTZ,
    CONSTRAINT talent_app_status_chk CHECK (status IN ('active','hired','rejected','withdrawn')),
    UNIQUE (job_id, candidate_id)
);
CREATE INDEX IF NOT EXISTS talent_apps_org_idx ON adrient.talent_applications (org_id, job_id);

CREATE TABLE IF NOT EXISTS adrient.talent_stage_history (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES adrient.talent_applications(id) ON DELETE CASCADE,
    org_id         UUID NOT NULL,
    from_stage     TEXT NOT NULL DEFAULT '',
    to_stage       TEXT NOT NULL,
    note           TEXT NOT NULL DEFAULT '',
    by_user        UUID,
    at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS talent_hist_app_idx ON adrient.talent_stage_history (application_id, at);

CREATE TABLE IF NOT EXISTS adrient.talent_interviews (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    application_id    UUID NOT NULL REFERENCES adrient.talent_applications(id) ON DELETE CASCADE,
    title             TEXT NOT NULL DEFAULT 'Interview',
    starts_at         TIMESTAMPTZ NOT NULL,
    ends_at           TIMESTAMPTZ NOT NULL,
    interviewer_ids   UUID[] NOT NULL DEFAULT '{}',
    calendar_event_id UUID,
    meet_code         TEXT NOT NULL DEFAULT '',
    mail_sent         BOOLEAN NOT NULL DEFAULT FALSE,
    status            TEXT NOT NULL DEFAULT 'scheduled',
    created_by        UUID,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.talent_scorecards (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL,
    application_id UUID NOT NULL REFERENCES adrient.talent_applications(id) ON DELETE CASCADE,
    interview_id   UUID,
    reviewer_id    UUID NOT NULL,
    ratings        JSONB NOT NULL DEFAULT '{}',
    overall        INT NOT NULL DEFAULT 0,
    recommendation TEXT NOT NULL DEFAULT '',
    comment        TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT talent_score_overall_chk CHECK (overall BETWEEN 0 AND 5)
);
CREATE UNIQUE INDEX IF NOT EXISTS talent_score_uq ON adrient.talent_scorecards (application_id, reviewer_id, COALESCE(interview_id, '00000000-0000-0000-0000-000000000000'::uuid));

CREATE TABLE IF NOT EXISTS adrient.talent_offers (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL,
    application_id UUID NOT NULL REFERENCES adrient.talent_applications(id) ON DELETE CASCADE,
    job_title      TEXT NOT NULL,
    department     TEXT NOT NULL DEFAULT '',
    salary         NUMERIC(14,2) NOT NULL DEFAULT 0,
    currency       TEXT NOT NULL DEFAULT 'INR',
    start_date     DATE,
    body           TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'draft',
    accept_token   TEXT NOT NULL,
    pdf_key        TEXT NOT NULL DEFAULT '',
    sent_at        TIMESTAMPTZ,
    responded_at   TIMESTAMPTZ,
    responded_name TEXT NOT NULL DEFAULT '',
    expires_at     TIMESTAMPTZ,
    created_by     UUID,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT talent_offer_status_chk CHECK (status IN ('draft','sent','accepted','declined','withdrawn'))
);
CREATE UNIQUE INDEX IF NOT EXISTS talent_offer_token_uq ON adrient.talent_offers (accept_token);

-- Persistent public-form abuse counters (per hashed client key and hour).
CREATE TABLE IF NOT EXISTS adrient.talent_public_hits (
    key    TEXT NOT NULL,
    hour   TIMESTAMPTZ NOT NULL,
    hits   INT NOT NULL DEFAULT 0,
    PRIMARY KEY (key, hour)
);
