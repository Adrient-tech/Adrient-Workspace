# Time tracking

Route `/timetracking`, module id `timetracking`, accent `#0E7490`, migration `0306`, Go package `internal/timetracking`, web `web/app/src/pages/timetracking`, API `/api/v1/timetracking`.
Timers and timesheets by client and project, billing rates with rounding, invoices created in Invoicing, and tracked hours exported to HR payroll.

## Who can do what
- **Members** log their own time (timer, manual entry, weekly grid), see their own entries and reports, and never see rates or amounts.
- **Admins** (org admin or HR admin) manage clients, projects and defaults, see everyone's time with amounts, log time for others, create invoices and read unbilled totals.
- **HR admins** export hours to payroll.

## Billing rules
- Rate cascade: entry override, project, client, organisation default. Rounding (5, 6, 10, 15, 30, 60 minutes, rounded up) and a minimum billable time come from the client, else the default. Non-billable projects and entries never bill.
- Creating an invoice freezes rate, rounding and minimum on each entry and links it to the document; invoiced time can no longer be edited or deleted, and rate changes never rewrite history.
- Entry checks: 1 second to 24 hours, no future days beyond tomorrow, project required and active. Warnings (not blocks): the person is on approved HR leave that day, or more than the configured hours are logged in a day.

## Links to other apps
- **Invoicing**: "Invoice" on a client creates one invoice (`internal/opshttp` `InvoiceWriter`, implemented in `internal/opsports`), one line per project and rate, tax from the client or default, due date from the payment terms. The invoice list shows the live payment status from Invoicing and opens the document.
- **Contacts**: a client can be linked to a Contact (search in the client dialog); the invoice then uses that contact as the customer.
- **HR**: leave warnings (`hr.Repository.ApprovedLeave`) and payroll. `POST /payroll-export` adds one earning line per employee (tracked hours x the person's payroll rate) to the draft HR payroll run (`internal/payrollhours`). The rate is a default or per HR employee in the settings. People without an HR record or rate are listed as skipped. Re-exporting replaces the earlier line.

## Data
`tt_docs` (clients, projects, entries, settings as JSON records per kind, org scoped) and `tt_timers` (one running timer per person). A timer left running is capped at 24 hours when stopped.

## AI assistant
`tt_my_hours`, `tt_timer_status`, confirmed and undoable `tt_log_time` and `tt_start_timer`; admins also get `tt_unbilled_by_client` and `tt_report`. Members never get amounts.

## Tests
Go: `logic_test.go` (duration parser, rounding, rate cascade, minimums, locked values), `repo_test.go` (roles, rate hiding, weekly cell, timer, invoice with locking, payroll lines). Web: `logic.test.ts`, `ai/tools.test.ts`.
