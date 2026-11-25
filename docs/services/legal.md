# Legal

Route `/legal`, module id `legal`, accent `#126C5A`, migration `0330`, Go package `internal/legal`, web `web/app/src/pages/legal`, API `/api/v1/legal`.
Practice management for a law firm or an in-house legal team: clients, matters with a status workflow, deadlines, time and expenses, bills that become Invoicing documents, links to Drive, Signatures and Contracts, conflict checks and privileged notes.

## Who can do what
- **Everyone with access to the Legal app** can see matters that are not restricted, record their own time, add deadlines, notes and documents, draft and issue bills, and run conflict checks.
- **Restricted matters** (an ethical wall) are visible only to org admins, the creator, the responsible and originating lawyers and the people it is shared with. They are hidden from lists, deadlines, time, bills and search. A conflict check still finds a party on a restricted matter, so conflicts are never missed, but it shows only "a restricted matter as <role>".
- **Org admins** change billing settings, delete matters (those without issued bills) and clients, see everyone's hours, and record time for other people.
- Use the **Legal counsel** role preset (Admin › App access) to give a group the Legal, Contracts, Signatures, Contacts, Calendar, Tasks and Invoicing apps and the Legal app-admin role.

## Rules
- **Numbering**: `2026-00012`, `00012` or `00012-Lopez` (Settings). The counter is per organization and never reused.
- **Status workflow**: intake → open ⇄ pending ⇄ on hold → closed → (reopen) open. Opening needs a recorded conflict check or a written waiver; closing with unbilled work needs "close anyway"; no time can be added to a closed matter. Every move is kept in the status history.
- **Time**: input accepts `1.5`, `1:30`, `90m`, `1h 30m`, `.3h`. It is rounded **up** to the billing increment (6 minutes by default). Rate precedence: matter rate (hourly matters), then client rate, then firm default.
- **Money** is whole paise in the database (BIGINT) and in the API. Time value = rate × minutes / 60 rounded half up; expense = price × quantity (3 decimals) rounded half up. Tax is in basis points on fees after discount; expenses are not taxed.
- **Bills**: "Prepare a bill" previews the unbilled work, "Create draft bill" locks those entries to the bill (two people cannot bill the same work), "Issue as invoice" creates a draft invoice in Invoicing (tax rate on fee lines, expenses untaxed) and the invoice total becomes the bill total. Voiding returns the work to unbilled; an issued bill's invoice must be cancelled in Invoicing (the reply says so). Flat-fee matters add one line for the agreed fee, once, and again only after the bill is voided.
- **Deadlines** create a Calendar event and a Task for the assignee, move them when the date changes, and remove them when done, deleted or the matter closes. The limitation date on a matter is kept as one "Limitation period ends" deadline.

## Links to other apps
- **Invoicing**: issued bills create draft invoices (`opshttp.InvoiceWriter`); the bill shows the invoice number and links to it.
- **Time tracking**: "Import from Time tracking" copies the caller's entries (once each, `source_ref tt:<id>`).
- **Drive / Signatures / Contracts**: a matter links Drive files (own files only), signature requests and contracts of the organization (restricted contracts only if you can see them); the status of signatures and contracts is shown live.
- **Contacts**: a client can start from a contact; conflict checks also search Contacts.
- **Calendar / Tasks**: deadline reminders (above).
- **Audit**: every write is audited under app `legal` (matter, client, entry, deadline, bill, document, note, settings, conflict check).

## AI assistant
`pages/legal/ai/tools.ts`, wired with `BizAssistant` like Contracts. Read tools: matters, one matter, deadlines, unbilled work, conflict check. Write tools (confirmed in the panel, undoable where possible, audited): log time, add a deadline, add a note. The assistant quotes amounts and hours from the server and gives no legal advice; matter text is marked untrusted.
