-- 0151: HR leave. Types carry their accrual policy; balances are an append-only
-- ledger (accrual, carry forward, used, adjustments) so every number is explainable.

CREATE TABLE IF NOT EXISTS adrient.hr_leave_types (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    code             TEXT NOT NULL,
    name             TEXT NOT NULL,
    color            TEXT NOT NULL DEFAULT '#126C5A',
    paid             BOOLEAN NOT NULL DEFAULT true,
    tracked          BOOLEAN NOT NULL DEFAULT true,        -- false = unlimited (e.g. unpaid leave)
    annual_quota     NUMERIC(6,2) NOT NULL DEFAULT 0,
    accrual          TEXT NOT NULL DEFAULT 'monthly' CHECK (accrual IN ('monthly','annual','none')),
    carry_forward    BOOLEAN NOT NULL DEFAULT false,
    carry_cap        NUMERIC(6,2) NOT NULL DEFAULT 0,      -- max days carried into the next year
    max_balance      NUMERIC(6,2),                         -- cap on the running balance (null = none)
    allow_half_day   BOOLEAN NOT NULL DEFAULT true,
    employment_types JSONB NOT NULL DEFAULT '[]'::jsonb,   -- empty = all types
    active           BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS hr_leave_types_code_uq ON adrient.hr_leave_types (org_id, lower(code));

CREATE TABLE IF NOT EXISTS adrient.hr_holiday_calendars (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    location_id UUID REFERENCES adrient.hr_locations(id) ON DELETE SET NULL,   -- null = default for all locations
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS hr_holiday_cal_name_uq ON adrient.hr_holiday_calendars (org_id, lower(name));

CREATE TABLE IF NOT EXISTS adrient.hr_holidays (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    calendar_id UUID NOT NULL REFERENCES adrient.hr_holiday_calendars(id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    name        TEXT NOT NULL,
    optional    BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (calendar_id, day)
);

-- Ledger of leave days. days > 0 adds balance, days < 0 consumes it.
CREATE TABLE IF NOT EXISTS adrient.hr_leave_ledger (
    id          BIGSERIAL PRIMARY KEY,
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    employee_id UUID NOT NULL REFERENCES adrient.hr_employees(id) ON DELETE CASCADE,
    type_id     UUID NOT NULL REFERENCES adrient.hr_leave_types(id) ON DELETE CASCADE,
    leave_year  INT NOT NULL,                              -- year the leave year starts in
    kind        TEXT NOT NULL CHECK (kind IN ('accrual','carry_forward','lapse','adjustment','used','reversal')),
    period      TEXT NOT NULL DEFAULT '',                  -- 'YYYY-MM' for accrual, makes accrual idempotent
    days        NUMERIC(6,2) NOT NULL,
    request_id  UUID,
    note        TEXT NOT NULL DEFAULT '',
    created_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS hr_leave_ledger_period_uq
    ON adrient.hr_leave_ledger (employee_id, type_id, leave_year, kind, period) WHERE period <> '';
CREATE INDEX IF NOT EXISTS hr_leave_ledger_emp_idx ON adrient.hr_leave_ledger (employee_id, type_id, leave_year);

CREATE TABLE IF NOT EXISTS adrient.hr_leave_requests (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    employee_id      UUID NOT NULL REFERENCES adrient.hr_employees(id) ON DELETE CASCADE,
    type_id          UUID NOT NULL REFERENCES adrient.hr_leave_types(id),
    from_date        DATE NOT NULL,
    to_date          DATE NOT NULL,
    half_day         TEXT NOT NULL DEFAULT '' CHECK (half_day IN ('','first','second')),
    days             NUMERIC(6,2) NOT NULL,
    reason           TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','cancelled')),
    decided_by       UUID,
    decided_at       TIMESTAMPTZ,
    decision_note    TEXT NOT NULL DEFAULT '',
    calendar_event_id TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (to_date >= from_date)
);
CREATE INDEX IF NOT EXISTS hr_leave_req_emp_idx ON adrient.hr_leave_requests (employee_id, from_date);
CREATE INDEX IF NOT EXISTS hr_leave_req_status_idx ON adrient.hr_leave_requests (org_id, status, from_date);
