-- Shifts: weekly rota (shifts, swaps, coverage, attendance) on top of HR employees.
CREATE TABLE IF NOT EXISTS adrient.shf_settings (
    org_id     UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    data       JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.shf_staff (
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    employee_id  UUID NOT NULL REFERENCES adrient.hr_employees(id) ON DELETE CASCADE,
    role         TEXT NOT NULL DEFAULT '',
    color        TEXT NOT NULL DEFAULT '',
    hourly_rate  NUMERIC(12,2),
    target_hours NUMERIC(6,2),
    max_hours    NUMERIC(6,2),
    availability JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (org_id, employee_id)
);

CREATE TABLE IF NOT EXISTS adrient.shf_shifts (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    shift_date     DATE NOT NULL,
    start_time     TEXT NOT NULL,
    end_time       TEXT NOT NULL,
    break_minutes  INT NOT NULL DEFAULT 0 CHECK (break_minutes >= 0),
    role           TEXT NOT NULL,
    location       TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    employee_id    UUID REFERENCES adrient.hr_employees(id) ON DELETE SET NULL,
    published      BOOLEAN NOT NULL DEFAULT false,
    created_by     UUID,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS shf_shifts_date_idx ON adrient.shf_shifts (org_id, shift_date);
CREATE INDEX IF NOT EXISTS shf_shifts_emp_idx ON adrient.shf_shifts (org_id, employee_id, shift_date);

CREATE TABLE IF NOT EXISTS adrient.shf_swaps (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    shift_id         UUID REFERENCES adrient.shf_shifts(id) ON DELETE SET NULL,
    from_employee_id UUID NOT NULL,
    to_employee_id   UUID NOT NULL,
    reason           TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','declined','cancelled')),
    snapshot         JSONB NOT NULL DEFAULT '{}'::jsonb,
    requested_by     UUID,
    resolved_by      UUID,
    resolved_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS shf_swaps_org_idx ON adrient.shf_swaps (org_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS adrient.shf_coverage (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    days       JSONB NOT NULL DEFAULT '[]'::jsonb,
    min_count  INT NOT NULL CHECK (min_count > 0),
    start_time TEXT NOT NULL DEFAULT '',
    end_time   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS adrient.shf_attendance (
    shift_id   UUID PRIMARY KEY REFERENCES adrient.shf_shifts(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'present' CHECK (status IN ('present','late','absent')),
    clock_in   TEXT NOT NULL DEFAULT '',
    clock_out  TEXT NOT NULL DEFAULT '',
    note       TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
