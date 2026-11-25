-- 0150: HR core (Adrient HR). Employees, designations, work locations, emergency
-- contacts, HR documents (the file itself lives in Drive blob storage), HR admin
-- grants and per-org HR settings. Departments reuse adrient.departments so the
-- directory, org chart and app policies keep one department tree.

CREATE TABLE IF NOT EXISTS adrient.hr_settings (
    org_id               UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    timezone             TEXT NOT NULL DEFAULT 'Asia/Kolkata',
    weekly_off           JSONB NOT NULL DEFAULT '[0]'::jsonb,        -- weekday numbers, 0 = Sunday
    leave_year_start     INT  NOT NULL DEFAULT 1 CHECK (leave_year_start BETWEEN 1 AND 12),
    geofence_enabled     BOOLEAN NOT NULL DEFAULT false,
    geofence_lat         DOUBLE PRECISION NOT NULL DEFAULT 0,
    geofence_lng         DOUBLE PRECISION NOT NULL DEFAULT 0,
    geofence_radius_m    INT NOT NULL DEFAULT 200,
    lop_mode             TEXT NOT NULL DEFAULT 'leave_only' CHECK (lop_mode IN ('leave_only','attendance_and_leave')),
    ot_enabled           BOOLEAN NOT NULL DEFAULT false,
    pf_wage_ceiling      NUMERIC(12,2) NOT NULL DEFAULT 15000,
    pf_capped            BOOLEAN NOT NULL DEFAULT true,
    esi_wage_limit       NUMERIC(12,2) NOT NULL DEFAULT 21000,
    company_name         TEXT NOT NULL DEFAULT '',
    company_address      TEXT NOT NULL DEFAULT '',
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Users who hold the HR admin role besides org admins.
CREATE TABLE IF NOT EXISTS adrient.hr_admins (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES adrient.users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.hr_locations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    address    TEXT NOT NULL DEFAULT '',
    state      TEXT NOT NULL DEFAULT '',     -- Indian state, drives professional tax
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS hr_locations_name_uq ON adrient.hr_locations (org_id, lower(name));

CREATE TABLE IF NOT EXISTS adrient.hr_designations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS hr_designations_name_uq ON adrient.hr_designations (org_id, lower(name));

CREATE TABLE IF NOT EXISTS adrient.hr_employees (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    user_id          UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    contact_id       UUID,
    emp_code         TEXT NOT NULL,
    first_name       TEXT NOT NULL,
    last_name        TEXT NOT NULL DEFAULT '',
    work_email       TEXT NOT NULL DEFAULT '',
    personal_email   TEXT NOT NULL DEFAULT '',
    phone            TEXT NOT NULL DEFAULT '',
    dob              DATE,
    gender           TEXT NOT NULL DEFAULT '',
    marital_status   TEXT NOT NULL DEFAULT '',
    blood_group      TEXT NOT NULL DEFAULT '',
    address          TEXT NOT NULL DEFAULT '',
    join_date        DATE NOT NULL,
    probation_end    DATE,
    exit_date        DATE,
    exit_reason      TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','notice','exited')),
    employment_type  TEXT NOT NULL DEFAULT 'full_time' CHECK (employment_type IN ('full_time','part_time','contract','intern','consultant')),
    department_id    UUID REFERENCES adrient.departments(id) ON DELETE SET NULL,
    designation_id   UUID REFERENCES adrient.hr_designations(id) ON DELETE SET NULL,
    location_id      UUID REFERENCES adrient.hr_locations(id) ON DELETE SET NULL,
    manager_id       UUID REFERENCES adrient.hr_employees(id) ON DELETE SET NULL,
    shift_id         UUID,
    pan              TEXT NOT NULL DEFAULT '',
    aadhaar_last4    TEXT NOT NULL DEFAULT '',
    uan              TEXT NOT NULL DEFAULT '',
    esi_number       TEXT NOT NULL DEFAULT '',
    bank_name        TEXT NOT NULL DEFAULT '',
    bank_account     TEXT NOT NULL DEFAULT '',
    bank_ifsc        TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (manager_id IS NULL OR manager_id <> id)
);
CREATE UNIQUE INDEX IF NOT EXISTS hr_employees_code_uq ON adrient.hr_employees (org_id, lower(emp_code));
CREATE UNIQUE INDEX IF NOT EXISTS hr_employees_user_uq ON adrient.hr_employees (org_id, user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS hr_employees_manager_idx ON adrient.hr_employees (manager_id) WHERE manager_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS hr_employees_status_idx ON adrient.hr_employees (org_id, status);

CREATE TABLE IF NOT EXISTS adrient.hr_emergency_contacts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    employee_id  UUID NOT NULL REFERENCES adrient.hr_employees(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    relationship TEXT NOT NULL DEFAULT '',
    phone        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS hr_emergency_emp_idx ON adrient.hr_emergency_contacts (employee_id);

-- A Drive file row is created for each document (owned by the uploader) so it
-- shows in storage accounting, but access is only ever through the HR API:
-- HR admins and the employee themselves.
CREATE TABLE IF NOT EXISTS adrient.hr_documents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    employee_id  UUID NOT NULL REFERENCES adrient.hr_employees(id) ON DELETE CASCADE,
    category     TEXT NOT NULL DEFAULT 'other',      -- offer_letter | id_proof | address_proof | contract | other
    name         TEXT NOT NULL,
    drive_file_id UUID,
    storage_key  TEXT NOT NULL,
    mime_type    TEXT NOT NULL DEFAULT 'application/octet-stream',
    size_bytes   BIGINT NOT NULL DEFAULT 0,
    uploaded_by  UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS hr_documents_emp_idx ON adrient.hr_documents (employee_id);

CREATE TABLE IF NOT EXISTS adrient.hr_audit (
    id         BIGSERIAL PRIMARY KEY,
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    actor_id   UUID,
    action     TEXT NOT NULL,
    entity     TEXT NOT NULL,
    entity_id  TEXT NOT NULL DEFAULT '',
    detail     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS hr_audit_org_idx ON adrient.hr_audit (org_id, created_at DESC);
