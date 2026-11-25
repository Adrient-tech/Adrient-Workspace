-- 0152: HR attendance. Raw punches (web, biometric import, approved
-- regularisations) are the source of truth; daily status is derived from them,
-- the shift, holidays and approved leave.

CREATE TABLE IF NOT EXISTS adrient.hr_shifts (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    start_time     TEXT NOT NULL DEFAULT '09:30',        -- HH:MM in the org timezone
    end_time       TEXT NOT NULL DEFAULT '18:30',
    grace_minutes  INT NOT NULL DEFAULT 10,
    early_minutes  INT NOT NULL DEFAULT 10,              -- leaving earlier than end - this counts as early exit
    full_day_mins  INT NOT NULL DEFAULT 480,             -- worked minutes for a full day
    half_day_mins  INT NOT NULL DEFAULT 240,             -- worked minutes for a half day
    ot_after_mins  INT NOT NULL DEFAULT 540,             -- overtime accrues past this many worked minutes
    is_default     BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS hr_shifts_name_uq ON adrient.hr_shifts (org_id, lower(name));

-- Per-day roster override. shift_id NULL means a day off for that employee.
CREATE TABLE IF NOT EXISTS adrient.hr_roster (
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    employee_id UUID NOT NULL REFERENCES adrient.hr_employees(id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    shift_id    UUID REFERENCES adrient.hr_shifts(id) ON DELETE CASCADE,
    PRIMARY KEY (employee_id, day)
);

CREATE TABLE IF NOT EXISTS adrient.hr_punches (
    id          BIGSERIAL PRIMARY KEY,
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    employee_id UUID NOT NULL REFERENCES adrient.hr_employees(id) ON DELETE CASCADE,
    punched_at  TIMESTAMPTZ NOT NULL,
    day         DATE NOT NULL,                           -- local date in the org timezone
    source      TEXT NOT NULL DEFAULT 'web' CHECK (source IN ('web','biometric','regularised','manual')),
    lat         DOUBLE PRECISION,
    lng         DOUBLE PRECISION,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS hr_punches_dedupe_uq ON adrient.hr_punches (employee_id, punched_at, source);
CREATE INDEX IF NOT EXISTS hr_punches_emp_day_idx ON adrient.hr_punches (employee_id, day);

CREATE TABLE IF NOT EXISTS adrient.hr_regularisations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    employee_id   UUID NOT NULL REFERENCES adrient.hr_employees(id) ON DELETE CASCADE,
    day           DATE NOT NULL,
    in_time       TEXT NOT NULL,                         -- HH:MM
    out_time      TEXT NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','cancelled')),
    decided_by    UUID,
    decided_at    TIMESTAMPTZ,
    decision_note TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS hr_reg_emp_idx ON adrient.hr_regularisations (employee_id, day);
CREATE INDEX IF NOT EXISTS hr_reg_status_idx ON adrient.hr_regularisations (org_id, status);
