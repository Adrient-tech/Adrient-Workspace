# CRM

Route `/crm`, module id `crm`, accent `#3B5BDB`, migration `0113_crm.sql`. Replaces the external Twenty tile (the Twenty docker services in `deploy/local` are left untouched, the tile now opens the native app).

## Data sharing
- **People are Contacts.** `crm_people` is a CRM profile (status, source, tags, company) keyed by `contact_id`. Contacts only has a free-text `company`, so CRM adds `crm_companies` (accounts) that people and deals link to. New leads can create a Contact in place.
- **Calendar:** "Schedule follow-up" creates an event through `POST /api/v1/calendar/events` and stores its id on the follow-up (`calendar_event_id`). Deleting the follow-up deletes the event.
- **Mail:** activities carry an optional `mail_thread_id` (entered manually; no Mail picker yet).
- **Invoicing:** deal page lists the primary contact's estimates/invoices (`GET /api/v1/invoicing/documents?contact_id=`) and "Create estimate" posts a draft estimate (one line = deal value) then opens Invoicing.

## API (JSON, `/api/v1/crm`, auth-wrapped, org-scoped)
`settings`, `companies`, `people`, `deals` (+`POST deals/{id}/stage {stage,reason}`), `history`, `followups`, `activities`. Lists return `{<name>: [...]}`.

## Features
| Feature | Status |
|---|---|
| Pipeline board (drag between stages, per-card move menu) and list, filters, sort | done |
| Custom funnel stages (1-8 open, rename/reorder/add/remove; Won/Lost fixed; deals in removed stage move to first) | done |
| Stage history + "in stage for N days", stalled marker | done |
| Leads and companies with detail dialogs (deals, follow-ups, activity) | done |
| Going cold (leads filter, dashboard, card chip) | done |
| Meeting history / activity log (call, meeting, email, note; filters, search, weekly target) | done |
| Win/loss reasons asked on close, reported in Forecast | done |
| Buying committee (several contacts per deal with roles, last touch each) | done |
| Subscriptions priced properly (one-time/monthly/quarterly/yearly, MRR won and in open deals) | done |
| Where deals leak (funnel conversion), forecast by month (stage-weighted), win rate by source | done |
| Monthly revenue goal progress, dashboard | done |
| CSV: export deals, leads, companies; import deals | done |
| Default currency INR | done |

## wiring
- Deal won creates the draft estimate in Invoicing automatically (idempotent); `POST /api/v1/bridge/crm/deal-won {deal_id, create_project}` re-runs it and can create a "Delivery: <deal>" project with three starter issues. The deal dialog has Open estimate / Create project buttons and shows stored links. `/crm?deal=<id>` opens the deal.
- Customer 360 (`/api/v1/crm/360/{company|person}/{id}`) also lists projects created from the customer's deals and tickets linked by hand to the company or person (besides e-mail and name matching).
- Tickets: the ticket page has a CRM panel (match by requester e-mail, or link a company by hand; `GET /api/v1/bridge/ticket-crm`, `POST /api/v1/bridge/link|unlink`).
- Not integrated: CRM leads do not become HR or Talent records; Talent candidates are not Contacts until hired.
