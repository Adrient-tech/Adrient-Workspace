# Tickets (service desk)

A service desk for IT, facilities and customer support: agent workspace, requester portal, knowledge base, SLA clocks, routing, automations, reports and change approvals.

Routes: `/tickets` (agents), `/support/{token}` and `/support/{token}/track/{trackToken}` (requesters, no sign-in), `/help/{token}` (help centre), `/csat/{token}`.

## Agent workspace
- Three panes: views with live counts and teams, a virtualised ticket list (cursor paging, 60 per page, search with `status:` `assignee:` `priority:` `type:` `tag:` `requester:` `via:` and `#number`), and the ticket with its properties panel. Density (compact, comfortable, spacious) and split or full view are remembered per browser.
- Several tickets can be open at once as tabs (up to 8).
- Shortcuts (press `h` for the list; `?` opens the app-wide list): `j`/`k` move, `Enter` open, `r` reply, `n` note, `a` assign to me, `e` solve, `z` snooze, `f` follow, `/` search, `g` then `m u a h i s z` go to a view, `x` close tab, `Ctrl+Shift+K` desk command bar, `Ctrl+Enter` send. Shortcuts are off while typing.
- Conversation: public reply and internal note tabs, bold/italic/list/link, `@mention` of teammates (notified), saved replies with variables, macros, suggested knowledge base articles while writing, draft autosave per person, "Send and mark solved".
- Collision warning: while someone has text in a reply box on the same ticket, others see "X is replying". Presence expires after 30 seconds.
- Snooze (hides from open views until the time, then wakes and notifies the assignee), follow, merge, parent and child tickets, problem and incident links, tags, time entries (also sent to Time tracking when connected), full activity trail.
- Major incident: flag a ticket and a red banner shows on every desk; the team is notified.

## Fields and workflow
- Types: question, incident, problem, request, change, task. Urgency and impact set the priority (3 by 3 matrix: high+high is urgent). Category and subcategory, custom fields from request forms.
- Statuses are configurable per project with a category (open, pending, hold, solved, closed), an option to pause the SLA clock and required fields before entering the status. A requester reply on a solved ticket reopens it.

## Routing, hours and SLA
- Teams with manual, round-robin or load-based assignment, capacity per person, skills and out-of-office. Full or away people are skipped.
- Business-hours calendars with a time zone and holidays; each team can use its own.
- SLA policies match on priority, type and customer email domain (first match wins), with first-response and resolution targets, counted in business hours or round the clock. The clock pauses in pausing statuses and the paused time is not counted.

## Automations
Trigger (create or update), dispatch (new tickets) and time-based rules with all/any conditions and actions: set status, priority, type, team, assignee, category, add or remove tag, mark major, notify, webhook, create task, add internal note. Time rules run every 5 minutes and at most once per ticket. Webhooks only reach public addresses. The builder can test a rule against a real ticket and shows the changes without saving.

## Requester portal and knowledge base
Request forms (type, team, category and custom questions) at `/support/{token}`, with article suggestions while typing. After submitting the requester gets a private tracking link to read replies, reply (reopens) and close. Articles have categories, views and helpful votes; the stats show deflection. Email-to-ticket addresses can be stored per team but receiving mail needs mail set up for the domain; nothing is received without it.

## Reports
Volume, backlog by age, first response and resolution, SLA met, CSAT, by type, category and team, SLA breaches, agent workload, saved views and CSV export (formulas neutralised).

## Changes and approvals
A ticket of type change can ask people for approval in steps. Step 2 only opens after step 1; any rejection rejects the change. People are notified through the notification centre.

## AI
The assistant panel (top bar) works on the project's tickets with confirmation, as before. Article suggestions use keyword scoring, not a model.

## Permissions
Everyone signed in can work tickets. Changing statuses, teams, calendars, SLA policies, automations, forms or running a sweep needs an organisation admin.

## API
Under `/api/v1/ts/`: `projects/{id}/list`, `statuses`, `groups`, `sla-rules`, `automations` (+`/test`), `forms`, `report2`, `report.csv`, `saved-reports`, `kb/suggest`, `kb/stats`; `tickets/{id}/` `sla`, `presence`, `draft`, `time`, `snooze`, `parent`, `major`, `fields`, `approvals`, `detail2`; `calendars`, `major`, `approvals/mine`, `me/ooo`, `sweep`. Public: `/api/v1/public/help/{token}/forms`, `.../forms/{id}/submit`, `.../suggest`, `/api/v1/public/track/{token}`.
