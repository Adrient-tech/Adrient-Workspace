-- 0202: lead capture and communications: public lead forms, Forms-app links,
-- email templates, logged mail.

CREATE TABLE IF NOT EXISTS adrient.crm_lead_forms (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    token      TEXT NOT NULL UNIQUE,
    enabled    BOOLEAN NOT NULL DEFAULT true,
    fields     JSONB NOT NULL DEFAULT '[]',     -- [{key,label,required}] key: name|email|phone|company|message|custom:<k>
    source     TEXT NOT NULL DEFAULT 'web form',
    owner_ids  JSONB NOT NULL DEFAULT '[]',     -- round-robin pool (user ids); empty = creator
    thanks     TEXT NOT NULL DEFAULT 'Thanks, we will be in touch.',
    created_by UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.crm_form_links (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    form_id    TEXT NOT NULL,                   -- Forms app form id
    mapping    JSONB NOT NULL DEFAULT '{}',     -- {question_id: name|email|phone|company|message|custom:<k>}
    source     TEXT NOT NULL DEFAULT 'forms',
    owner_ids  JSONB NOT NULL DEFAULT '[]',
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_by UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    UNIQUE (org_id, form_id)
);

CREATE TABLE IF NOT EXISTS adrient.crm_email_templates (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    subject    TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Mail messages matched to a CRM person by address (metadata only, never the body).
CREATE TABLE IF NOT EXISTS adrient.crm_mail_log (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    message_id UUID NOT NULL,
    thread_id  UUID NOT NULL,
    person_id  UUID NOT NULL REFERENCES adrient.crm_people(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    direction  TEXT NOT NULL DEFAULT 'in',      -- in | out
    subject    TEXT NOT NULL DEFAULT '',
    snippet    TEXT NOT NULL DEFAULT '',
    sent_at    TIMESTAMPTZ NOT NULL,
    UNIQUE (message_id, person_id)
);
CREATE INDEX IF NOT EXISTS crm_mail_log_person_idx ON adrient.crm_mail_log (person_id, sent_at DESC);
