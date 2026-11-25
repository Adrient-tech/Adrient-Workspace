# Fleet

Route `/fleet`, module id `fleet`, accent `#475569`, migration `0308`, Go package `internal/fleet`, web `web/app/src/pages/fleet`, API `/api/v1/fleet`.
Vehicles and equipment: meter readings, fuel, service history, service reminders, renewals, inspections that raise issues, and costs that post to Accounting.

## Who can do what
- **Members** see the fleet, add meter readings, fuel and inspections, and report issues (drivers do this on a phone).
- **Admins** (org admins) add and edit vehicles, log service, manage reminders and renewals, resolve issues, change the checklist and post costs to Accounting.

## Rules
- VIN: 17 characters without I, O, Q is required. The North American check digit is a warning only (other markets do not use it). The model year is read from the VIN when empty. Plate and VIN must be unique.
- Meter: a reading lower than the last is refused unless confirmed; a jump of more than 5,000 warns. The vehicle's meter only goes up through the log.
- Reminders repeat by time, by meter, or both (whichever comes first) with due-soon thresholds; status overdue, due soon, snoozed or upcoming. Completing one, directly or by logging a service entry, recomputes the next due date and meter and records whether it was on time (compliance).
- Renewals (registration, insurance, emission, licence) repeat from the old due date, so a late renewal does not shift the cycle; a one-off renewal is marked done.
- Inspections use an editable checklist. Each failed item opens a high-priority issue and the reading updates the meter.
- Fuel economy is computed between consecutive full-tank fill-ups. The overview shows 6 months of fuel and service cost and the cost per km or mile.

## Links to other apps
- **Calendar**: each reminder and renewal keeps one all-day Calendar item on its next due date for the vehicle's driver (else the person who set it). It moves when the date changes and is removed when deleted or done (`opshttp.CalendarWriter`).
- **Accounting**: "Post to Accounting" records a fuel or service cost as an unpaid bill under the chosen expense category (`accounting.Repository.SaveExpense`, via `opsports.Expenses`). Posting twice is refused and the entry shows "In Accounting".
- **People**: drivers are organisation users.

## AI assistant
`fleet_overview`, `fleet_find_vehicles`, `fleet_reminders`, `fleet_open_issues`, and the confirmed writes `fleet_report_issue` and `fleet_record_meter`. Nothing posts to Accounting.

## Tests
Go: `logic_test.go` (VIN, intervals, reminder state and compliance, renewal rolling, costs), `repo_test.go` (roles, duplicates, meter rules, fuel economy, expense posting once, reminder Calendar item following the due date, service completing a reminder, renewals, inspection issues, dashboard, costs). Web: `logic.test.ts`, `ai/tools.test.ts`.
