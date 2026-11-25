# HR

Route `/hr`, module id `hr`, accent `#126C5A`, migrations `0150_hr_core.sql` to `0153_hr_payroll.sql`, Go package `internal/hr`, web `web/app/src/pages/hr`. An HRMS for an Indian SME: employees, leave, attendance, payroll, self-service. Built from scratch for Adrient.

## Roles (enforced on the server, in `internal/hr`)
Every repository method takes an `Actor` resolved from the database, so the rules hold for any caller, not only the HTTP layer. Tests: `internal/hr/roles_test.go`, `http_test.go`.

| Role | How you get it | Can |
|---|---|---|
| HR admin | org admin/owner, or an explicit grant (HR dashboard, "HR admins") | everything in HR |
| Manager | has at least one direct report (employee record with `manager_id`) | approve leave and regularisations of direct reports, see their timesheets, leave balances, roster and team report. **Never salary, payslips, bank, PAN, documents.** |
| Employee | has an employee record linked to the account | own profile (phone, address, emergency contacts), own leave, attendance, payslips (after lock), documents |
| Other member | any logged-in member | directory fields of active employees (name, title, department, email, phone, location, manager), org chart, holidays, team birthdays |

Rules worth knowing: nobody approves their own request (HR may only for an employee with no manager); another employee's payslip, document or salary answers `403`/`404`, never data; draft payslips are invisible to employees until the run is locked.

## Data sharing with other apps
- **Directory / org chart:** departments are the directory's `adrient.departments`. Saving an employee linked to an account mirrors job title, department, manager, employee id and start date onto `adrient.users`, so PersonCard and the org chart agree. The org chart page reads `/api/v1/hr/orgchart` (HR managers, including people without accounts) and falls back to the directory when HR has no employees.
- **Calendar:** an approved leave creates an out-of-office item (all day, or a timed half day) on the employee's calendar through the Calendar repository. Cancelling deletes it.
- **Notifications:** apply, approve, reject, cancel, regularisation and payslip-published notifications go through `internal/notifications`. Chat messages are not sent (no stable internal API); notifications are the channel.
- **Drive:** HR documents are stored in the Drive blob store and registered as Drive files in an `HR Documents` folder owned by the uploader. They have no Drive grants: bytes are served only by `GET /api/v1/hr/documents/{id}` to HR and the employee.
- **Accounting:** locking a payroll run posts one journal (source `payroll`, source id = run id, so re-posting never duplicates). Accounts used or created by system key: `salaries` (6010), `hr_employer_contrib` (6015), `hr_pf_payable` (2410), `hr_esi_payable` (2420), `hr_pt_payable` (2430), `tds_payable` (2400), `hr_other_deductions` (2440), `hr_salary_payable` (2450), `emp_payable` (reimbursements).
  Debit: salaries (gross), employer PF/ESI, reimbursements settled in payroll. Credit: PF, ESI, PT, TDS, other deductions, salary payable (net). If Accounting refuses (for example a closed period) the lock is undone so you can fix the books and retry.
- **Expense reports:** HR does not copy them. In a draft payslip, "Adjust" lists the employee's approved Accounting expense reports with out-of-pocket amounts; linking one adds it to the net pay, and on lock the report is marked reimbursed (the payroll journal clears Employee Reimbursements Payable, so it must not be paid again).
- **App controls:** `hr` is a gated app (`internal/authz/apps.go`), so admins can restrict it per user, group or department.

## API (`/api/v1/hr`, JSON, org-scoped)
`me`, `dashboard`, `dashboard/manager`, `settings`, `hr-admins`, `available-users`, `orgchart`, `celebrations`, `departments|designations|locations`, `employees` (+`export.csv`, `import`, `{id}`, `{id}/emergency`, `{id}/documents`, `{id}/salary`, `{id}/balances`, `{id}/balances/adjust`, `{id}/timesheet`), `documents/{id}`, `leave-types`, `holiday-calendars`, `holidays/{id}`, `leave/requests` (+`/{id}/approve|reject|cancel`), `leave/team-calendar`, `leave/accrual/run`, `attendance/today|punch|timesheet|report|import`, `shifts`, `roster`, `regularisations`, `payroll/structures`, `payroll/pt-slabs`, `payroll/runs` (+`/{id}/recompute|review|reopen|lock|payslips|bank-advice.csv|statutory.csv`), `payroll/payslips/mine|{id}|{id}/pdf|{id}/adjustments|{id}/reimbursements`.

## Employees
Profile (personal, job, manager, department, designation, location, employment type, join/probation/exit), account link, PAN/UAN/ESI/bank, emergency contacts, documents. Employee codes auto-number (`EMP0001`). Manager loops are refused. CSV import upserts by `emp_code`, creates missing departments, designations and locations, resolves `manager_code` after all rows load and links accounts by work email; export defuses spreadsheet formulas.

## Leave
Types carry the policy: annual quota, accrual `monthly` / `annual` / `none`, carry forward with a cap, max balance, half days, employment types, paid or unpaid, tracked or unlimited. Balances are an append-only ledger (accrual, carry forward, lapse, adjustment, used, reversal). Accrual is idempotent per type, year and month and runs when balances are read, or from "Run accrual now". Joining after the 15th starts accrual next month. The leave year can start in any month. Holiday calendars apply per location (a location calendar replaces the default one); weekly offs are a setting. Requests support half days, overlap and balance checks, manager approval with HR override, cancellation (restores the balance, removes the Calendar item) and a team calendar.
Starter data (created on first use, all editable): General shift, Casual 12/yr monthly, Sick 6/yr annual, Earned 15/yr monthly with carry forward up to 30, Loss of Pay (unpaid, unlimited), one holiday calendar, one salary structure, professional tax slabs for a few states.

## Attendance
Raw punches are the source of truth: web check-in/out (alternating), biometric CSV import, approved regularisations. A day is classified from punches, shift, holidays, weekly off, roster and approved leave: present, half day (worked at least the half-day minutes), absent, leave, holiday, weekly off, missing punch. Late (past start plus grace), early exit, overtime (optional, after a threshold). Shifts support a per-day roster including days off. Geofence is **off by default**; when on, the browser sends its position and the server rejects check-ins outside the radius (haversine). Overnight shifts are not supported.

## Payroll (India)
- Salary structures: components with `fixed`, `% of gross`, `% of basic` or one balancing line; BASIC required. Per-employee salary is a dated snapshot you can edit after applying a structure. Annual gross, not CTC: employer PF/ESI are shown separately.
- PF: 12% employee and 12% employer on PF-wage components, capped at the wage ceiling (setting, default 15,000). Employer EDLI/admin charges are not computed.
- ESI: applies while contracted ESI wages are within the limit (default 21,000); 0.75% employee, 3.25% employer, rounded up to the rupee.
- Professional tax: editable per-state slab table with optional month overrides (for example February). The employee's work location carries the state. The seeded values are a starting point and must be verified.
- TDS: an even monthly estimate under the new regime (standard deduction 75,000, 87A rebate with marginal relief, 4% cess). It is an estimate for salary only: no old regime, no declarations, no other income, no form 16.
- Loss of pay: approved unpaid leave, plus (setting `lop_mode = attendance_and_leave`) absent days from attendance. Pay is prorated by calendar days, also for joiners and leavers mid-month.
- Run lifecycle: draft (recompute, adjustments: extra earning, deduction, reimbursement) → reviewed (can reopen) → locked (final, journal posted, payslips published). Bank advice CSV and PF/ESI/PT register CSV for reviewed and locked runs. Payslip PDFs are generated without third-party libraries and validated with pdfcpu in tests.

## Self-service and dashboards
**My HR** home: check-in, leave balances, apply, recent leave, payslips (PDF), documents, team, upcoming holidays, birthdays and work anniversaries, my details and emergency contacts. **Manager dashboard:** team size, in today, on leave, pending approvals. **HR dashboard:** headcount, by department/type/location, joiners and leavers (30 days), attrition over 12 months (leavers over average headcount), on leave today, pending approvals, payroll status, employees without salary, HR admin grants.

## Tests
`go test ./internal/hr` (pure rules always; database tests need `ADRIENT_TEST_DSN` pointing at a scratch database, they drop the `adrient` schema) and `npx vitest run src/pages/hr`.

## Hire into HR and deep links
- `hr.Repository.CreateFromNewHire` creates an employee from a hired Talent candidate (HR admins only, idempotent by `source_ref` or work email). Used by `POST /api/v1/bridge/talent/hire`, see talent.md. Migration `0250_integ_links.sql` adds `hr_employees.source_ref`.
- Deep links: `/hr?view=people&employee=<id>` opens the employee dialog; `view` accepts home, dashboard, people, leave, attendance, approvals, payroll. The employee dialog shows "Open in" buttons (Contacts, directory, Talent, onboarding checklist, project).
- Verified in a browser on the dev stack: hire in Talent, "Open in HR" lands on the new employee with its links. Verified by API: payroll lock posts one journal (6010, 6015, 2410, 2450 for the test run) that appears in the trial balance as of the month end (journal date is the last day of the period, so a trial balance dated earlier does not show it), a second lock is refused and changes nothing.
