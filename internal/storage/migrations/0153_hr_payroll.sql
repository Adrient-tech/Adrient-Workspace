-- 0153: HR payroll (India). Salary structures, per-employee salary, professional
-- tax slabs per state, monthly payroll runs and payslips.

CREATE TABLE IF NOT EXISTS adrient.hr_salary_structures (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    lines      JSONB NOT NULL DEFAULT '[]'::jsonb,        -- [{code,name,kind,calc,value,pf_wage,esi_wage,taxable}]
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS hr_salary_struct_name_uq ON adrient.hr_salary_structures (org_id, lower(name));

-- One row per employee per effective date; the latest row on or before the
-- payroll month is used.
CREATE TABLE IF NOT EXISTS adrient.hr_employee_salary (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    employee_id    UUID NOT NULL REFERENCES adrient.hr_employees(id) ON DELETE CASCADE,
    effective_from DATE NOT NULL,
    structure_id   UUID REFERENCES adrient.hr_salary_structures(id) ON DELETE SET NULL,
    annual_gross   NUMERIC(14,2) NOT NULL DEFAULT 0,
    lines          JSONB NOT NULL DEFAULT '[]'::jsonb,    -- monthly amounts, see SalaryLine
    pf_enabled     BOOLEAN NOT NULL DEFAULT true,
    esi_enabled    BOOLEAN NOT NULL DEFAULT true,
    pt_enabled     BOOLEAN NOT NULL DEFAULT true,
    tds_enabled    BOOLEAN NOT NULL DEFAULT false,
    created_by     UUID,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (employee_id, effective_from)
);

CREATE TABLE IF NOT EXISTS adrient.hr_pt_slabs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    state          TEXT NOT NULL,
    min_salary     NUMERIC(12,2) NOT NULL DEFAULT 0,
    max_salary     NUMERIC(12,2),                         -- null = no upper bound
    amount         NUMERIC(10,2) NOT NULL DEFAULT 0,
    month          INT NOT NULL DEFAULT 0,                -- 0 = every month, 1..12 = only that month (overrides)
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS hr_pt_slabs_idx ON adrient.hr_pt_slabs (org_id, state);

CREATE TABLE IF NOT EXISTS adrient.hr_payroll_runs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    period        TEXT NOT NULL,                          -- YYYY-MM
    status        TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','reviewed','locked')),
    created_by    UUID,
    reviewed_by   UUID,
    locked_by     UUID,
    locked_at     TIMESTAMPTZ,
    journal_id    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS hr_payroll_runs_period_uq ON adrient.hr_payroll_runs (org_id, period);

CREATE TABLE IF NOT EXISTS adrient.hr_payslips (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    run_id         UUID NOT NULL REFERENCES adrient.hr_payroll_runs(id) ON DELETE CASCADE,
    employee_id    UUID NOT NULL REFERENCES adrient.hr_employees(id) ON DELETE CASCADE,
    emp_code       TEXT NOT NULL DEFAULT '',
    emp_name       TEXT NOT NULL DEFAULT '',
    designation    TEXT NOT NULL DEFAULT '',
    department     TEXT NOT NULL DEFAULT '',
    pan            TEXT NOT NULL DEFAULT '',
    uan            TEXT NOT NULL DEFAULT '',
    bank_name      TEXT NOT NULL DEFAULT '',
    bank_account   TEXT NOT NULL DEFAULT '',
    bank_ifsc      TEXT NOT NULL DEFAULT '',
    days_in_month  INT NOT NULL,
    lop_days       NUMERIC(6,2) NOT NULL DEFAULT 0,
    paid_days      NUMERIC(6,2) NOT NULL,
    earnings       JSONB NOT NULL DEFAULT '[]'::jsonb,    -- [{code,name,amount}]
    deductions     JSONB NOT NULL DEFAULT '[]'::jsonb,
    adjustments    JSONB NOT NULL DEFAULT '[]'::jsonb,    -- HR one-offs: [{kind,name,amount,expense_report_id}]
    gross          NUMERIC(14,2) NOT NULL DEFAULT 0,
    pf_employee    NUMERIC(12,2) NOT NULL DEFAULT 0,
    pf_employer    NUMERIC(12,2) NOT NULL DEFAULT 0,
    esi_employee   NUMERIC(12,2) NOT NULL DEFAULT 0,
    esi_employer   NUMERIC(12,2) NOT NULL DEFAULT 0,
    prof_tax       NUMERIC(12,2) NOT NULL DEFAULT 0,
    tds            NUMERIC(12,2) NOT NULL DEFAULT 0,
    other_deductions NUMERIC(12,2) NOT NULL DEFAULT 0,
    reimbursements NUMERIC(12,2) NOT NULL DEFAULT 0,
    net_pay        NUMERIC(14,2) NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (run_id, employee_id)
);
CREATE INDEX IF NOT EXISTS hr_payslips_emp_idx ON adrient.hr_payslips (employee_id);
