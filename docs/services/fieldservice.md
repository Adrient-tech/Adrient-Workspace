# Field service

Route `/fieldservice`, module id `fieldservice`, accent `#C2410C`, migration `0307`, Go package `internal/fieldservice`, web `web/app/src/pages/fieldservice`, API `/api/v1/fieldservice`.
For service businesses (lawn care, cleaning, HVAC, plumbing, handyman): requests, quotes, jobs with recurring visits, a schedule, and invoices. It reuses the other apps instead of keeping its own customers and money.

## Flow
Request (new, assessment, quoted, job created, closed) -> **quote** (a draft estimate in Invoicing) -> **job** (lines, schedule, assigned people) -> **visits** (generated from the schedule) -> complete -> **invoice** (in Invoicing).
- Recurrence: daily, weekly on chosen weekdays, monthly by date (clamped to month end) or by weekday (2nd Tuesday), yearly (Feb 29 clamps to Feb 28), ending after N visits or on a date, at most 400 visits.
- Editing a job keeps completed and invoiced visits and rebuilds the open ones. Cancelling a job cancels its open visits.
- Totals: optional add-ons only when selected, percent or fixed discount shared over the taxable lines, tax on the taxable remainder, estimated cost and profit. The same maths runs in Go and in the web editor for the live total.
- Job status is derived: active, late (an open visit before today), requires invoicing (all visits done, not invoiced), completed, cancelled.
- Billing: "when visits are done" invoices all completed, uninvoiced visits in one invoice (lines x number of visits); "after every visit" invoices each visit on completion.

## Links to other apps
- **Contacts**: customers are Contacts. The picker searches Contacts and can add a person there (`opshttp.ContactDirectory`). Properties (addresses) belong to a contact.
- **Invoicing**: quotes are estimates and invoices are invoices (`opshttp.InvoiceWriter`); status and balances are read live. Documents open in Invoicing (`/invoicing?doc=`).
- **Calendar**: every scheduled visit creates a Calendar item for each assigned person (title, address, instructions, all-day when no time). Rescheduling or reassigning moves or removes the items; cancelling a visit or job removes them.
- **People**: assignees are organisation users.

## Access
Any member with app access can handle requests, jobs and visits (office and crew work in one place). Only org admins edit the services catalogue and the defaults (tax, currency, payment terms, billing mode).

## AI assistant
`fs_dashboard`, `fs_visits`, `fs_find_jobs`, and the confirmed writes `fs_create_request` (undo closes it), `fs_reschedule_visit` (undo moves back), `fs_complete_visit`. No tool sends a quote or invoice.

## Tests
Go: `logic_test.go` (recurrence cases, totals, request transitions, job status), `repo_test.go` (request to quote to job, Calendar items per visit, reschedule, completion, invoice multiplied by visits, edit keeps done visits, cancel clears Calendar). Web: `logic.test.ts` (totals match the server, schedule text), `ai/tools.test.ts`.
