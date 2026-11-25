-- 0330: Legal (clients, matters, deadlines, time and expenses, bills, document links, conflict checks). Idempotent.
-- Money is stored as whole minor units (paise/cents) in BIGINT columns so sums are exact.

CREATE TABLE IF NOT EXISTS adrient.legal_settings (
    org_id          UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    number_format   TEXT NOT NULL DEFAULT 'year-number' CHECK (number_format IN ('number','number-client','year-number')),
    next_matter_no  INT  NOT NULL DEFAULT 1,
    increment_min   INT  NOT NULL DEFAULT 6,
    default_rate    BIGINT NOT NULL DEFAULT 0,
    currency        TEXT NOT NULL DEFAULT 'INR',
    terms_days      INT  NOT NULL DEFAULT 30,
    tax_rate_bp     INT  NOT NULL DEFAULT 0,   -- tax in basis points: 1800 = 18%
    tax_name        TEXT NOT NULL DEFAULT 'GST',
    practice_areas  TEXT[] NOT NULL DEFAULT '{Corporate,Litigation,Intellectual property,Employment,Real estate,Tax,Family}'
);

CREATE TABLE IF NOT EXISTS adrient.legal_clients (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    contact_id  UUID,
    kind        TEXT NOT NULL DEFAULT 'person' CHECK (kind IN ('person','company')),
    name        TEXT NOT NULL,
    email       TEXT NOT NULL DEFAULT '',
    phone       TEXT NOT NULL DEFAULT '',
    address     TEXT NOT NULL DEFAULT '',
    rate        BIGINT NOT NULL DEFAULT 0,
    notes       TEXT NOT NULL DEFAULT '',
    archived    BOOLEAN NOT NULL DEFAULT false,
    created_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS legal_clients_org_idx ON adrient.legal_clients (org_id, lower(name));

CREATE TABLE IF NOT EXISTS adrient.legal_matters (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    seq              INT  NOT NULL,
    number           TEXT NOT NULL,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    client_id        UUID REFERENCES adrient.legal_clients(id) ON DELETE RESTRICT,
    practice_area    TEXT NOT NULL DEFAULT '',
    stage            TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'intake' CHECK (status IN ('intake','open','pending','on_hold','closed')),
    responsible_id   UUID,
    originating_id   UUID,
    open_date        DATE,
    pending_date     DATE,
    closed_date      DATE,
    sol_date         DATE,
    billing_method   TEXT NOT NULL DEFAULT 'hourly' CHECK (billing_method IN ('hourly','flat','contingency')),
    rate             BIGINT NOT NULL DEFAULT 0,
    flat_fee         BIGINT NOT NULL DEFAULT 0,
    flat_fee_billed  BOOLEAN NOT NULL DEFAULT false,
    contingency_bp   INT NOT NULL DEFAULT 0,
    budget           BIGINT NOT NULL DEFAULT 0,
    tags             TEXT[] NOT NULL DEFAULT '{}',
    access           TEXT NOT NULL DEFAULT 'org' CHECK (access IN ('org','restricted')),
    shared_with      UUID[] NOT NULL DEFAULT '{}',
    conflict_checked_at TIMESTAMPTZ,
    conflict_checked_by UUID,
    conflict_waiver  TEXT NOT NULL DEFAULT '',
    created_by       UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at       TIMESTAMPTZ,
    UNIQUE (org_id, seq)
);
CREATE INDEX IF NOT EXISTS legal_matters_org_idx ON adrient.legal_matters (org_id, status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS legal_matters_client_idx ON adrient.legal_matters (client_id);

CREATE TABLE IF NOT EXISTS adrient.legal_parties (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    matter_id   UUID NOT NULL REFERENCES adrient.legal_matters(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'Related',
    contact_id  UUID,
    email       TEXT NOT NULL DEFAULT '',
    phone       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS legal_parties_matter_idx ON adrient.legal_parties (matter_id);
CREATE INDEX IF NOT EXISTS legal_parties_org_idx ON adrient.legal_parties (org_id, lower(name));

CREATE TABLE IF NOT EXISTS adrient.legal_status_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    matter_id   UUID NOT NULL REFERENCES adrient.legal_matters(id) ON DELETE CASCADE,
    from_status TEXT NOT NULL,
    to_status   TEXT NOT NULL,
    note        TEXT NOT NULL DEFAULT '',
    by_user     UUID,
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS legal_status_log_idx ON adrient.legal_status_log (matter_id, at);

CREATE TABLE IF NOT EXISTS adrient.legal_deadlines (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL,
    matter_id     UUID NOT NULL REFERENCES adrient.legal_matters(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    kind          TEXT NOT NULL DEFAULT 'deadline' CHECK (kind IN ('deadline','court_date','filing','meeting','limitation')),
    due_date      DATE NOT NULL,
    assignee_id   UUID,
    done          BOOLEAN NOT NULL DEFAULT false,
    notes         TEXT NOT NULL DEFAULT '',
    event_id      UUID,
    task_id       UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS legal_deadlines_matter_idx ON adrient.legal_deadlines (matter_id);
CREATE INDEX IF NOT EXISTS legal_deadlines_due_idx ON adrient.legal_deadlines (org_id, due_date) WHERE done = false;

CREATE TABLE IF NOT EXISTS adrient.legal_bills (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL,
    matter_id      UUID NOT NULL REFERENCES adrient.legal_matters(id) ON DELETE CASCADE,
    number         TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','issued','void')),
    issue_date     DATE NOT NULL,
    due_date       DATE NOT NULL,
    period_from    DATE,
    period_to      DATE,
    fees           BIGINT NOT NULL DEFAULT 0,
    expenses       BIGINT NOT NULL DEFAULT 0,
    discount       BIGINT NOT NULL DEFAULT 0,
    tax            BIGINT NOT NULL DEFAULT 0,
    total          BIGINT NOT NULL DEFAULT 0,
    invoice_id     UUID,
    invoice_number TEXT NOT NULL DEFAULT '',
    created_by     UUID,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS legal_bills_matter_idx ON adrient.legal_bills (matter_id);

CREATE TABLE IF NOT EXISTS adrient.legal_entries (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    matter_id    UUID NOT NULL REFERENCES adrient.legal_matters(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL DEFAULT 'time' CHECK (kind IN ('time','expense','flat')),
    user_id      UUID NOT NULL,
    entry_date   DATE NOT NULL,
    minutes      INT NOT NULL DEFAULT 0,
    rate         BIGINT NOT NULL DEFAULT 0,
    qty_milli    BIGINT NOT NULL DEFAULT 1000,     -- expense quantity x 1000
    amount       BIGINT NOT NULL DEFAULT 0,        -- computed value in minor units
    description  TEXT NOT NULL DEFAULT '',
    billable     BOOLEAN NOT NULL DEFAULT true,
    bill_id      UUID REFERENCES adrient.legal_bills(id) ON DELETE SET NULL,
    source_ref   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS legal_entries_matter_idx ON adrient.legal_entries (matter_id, entry_date);
CREATE INDEX IF NOT EXISTS legal_entries_unbilled_idx ON adrient.legal_entries (org_id, matter_id) WHERE bill_id IS NULL AND billable;
CREATE UNIQUE INDEX IF NOT EXISTS legal_entries_source_idx ON adrient.legal_entries (org_id, source_ref) WHERE source_ref <> '';

CREATE TABLE IF NOT EXISTS adrient.legal_documents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    matter_id   UUID NOT NULL REFERENCES adrient.legal_matters(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('drive','signature','contract')),
    ref_id      UUID NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL DEFAULT '',
    added_by    UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (matter_id, kind, ref_id)
);

CREATE TABLE IF NOT EXISTS adrient.legal_notes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    matter_id   UUID NOT NULL REFERENCES adrient.legal_matters(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL DEFAULT 'note' CHECK (kind IN ('note','call','email','meeting','conflict')),
    body        TEXT NOT NULL,
    author_id   UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS legal_notes_matter_idx ON adrient.legal_notes (matter_id, created_at);
