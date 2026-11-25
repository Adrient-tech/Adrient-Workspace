# Shifts

Route `/shifts`, module id `shifts`, accent `#126C5A`, migration `0305`, Go package `internal/shifts`, web `web/app/src/pages/shifts`, API `/api/v1/shifts`.
Weekly rota planning for the people in HR: shifts and open shifts, availability, coverage rules, swap requests, attendance and a labour-cost view. Hours export to HR payroll.

## Who can do what (enforced on the server)
- **Scheduler** = org admin, HR admin, or an employee with direct reports in HR (a manager). Schedulers create, move, assign, publish and delete shifts, edit staff profiles and pay rates, set rules and coverage, decide swaps, record attendance and see costs.
- **Everyone else** sees only **published** shifts, requests a swap for their own shifts, and clocks in and out on their own published shift of today (or last night's). Drafts, pay rates and costs are hidden from them.
- Payroll export: HR admins only (HR enforces the same rule again).

## Data and links to other apps
- **HR**: the staff list is the HR employee directory. Approved HR leave appears as time off (`hr.Repository.ApprovedLeave`), so a shift on leave gets a warning and the person is excluded from suggestions. The caller's roles come from `hr.Repository.ActorFor`.
- **HR payroll**: `POST /payroll-export {period}` adds one earning line per employee to the draft payroll run of the month (paid hours x hourly rate, `internal/payrollhours`). Paid hours are the clocked time minus the unpaid break when attendance exists, the scheduled net time otherwise, nothing when absent. Exporting again replaces the earlier line; other adjustments on the payslip are untouched. A locked run is refused.
- Tables: `shf_shifts`, `shf_staff` (role, colour, rate, target and max weekly hours, availability), `shf_swaps`, `shf_coverage`, `shf_attendance`, `shf_settings`.

## Rules and checks (non-blocking unless stated)
- Overlap with the person's own shifts (also across midnight) **blocks** the assignment. Time off, unavailable or partial availability, weekly maximum hours, minimum rest, daily hours and consecutive days **warn**.
- Optional limits are off at 0. Overtime: paid minutes past a weekly threshold cost the multiplier, in the order shifts are worked. A weekly budget shows used and over.
- Suggestions exclude inactive people, time off, overlaps, unavailable and over-maximum, and rank by rule warnings, availability, hours below target, role match. "Fill open shifts" proposes one person per open shift of the week, each aware of the ones above it, and nothing changes until you apply.
- Swaps: pending, approved, declined or cancelled. Approving re-checks that the shift is still with the requester and not overlapping for the receiver, then reassigns it.

## AI assistant
Tools in `pages/shifts/ai/tools.ts` run as the user (docs/ai.md): `shifts_my_schedule`, `shifts_week_overview`, `shifts_open_shifts`, `shifts_suggest_assignee`, and the confirmed writes `shifts_assign_shift`, `shifts_create_shift` (draft) and `shifts_request_swap`, all undoable.

## Tests
Go: `internal/shifts/logic_test.go` (overnight maths, conflicts, availability, rules, ranking, fill, coverage, overtime cost, paid minutes) and `repo_test.go` (database flow: leave as time off, roles, drafts hidden, swap, attendance, timesheet, payroll lines, copy week). Web: `pages/shifts/logic.test.ts`, `pages/shifts/ai/tools.test.ts`, `pages/ops/dates.test.ts`.
