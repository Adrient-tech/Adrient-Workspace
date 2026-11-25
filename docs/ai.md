# Adrient AI assistant: contract for apps

The web contract below is the public interface for apps. The server is `/api/v1/ai/*`.
Answer quality depends on the model you connect.

## Mount the panel (Sheets / Docs / Slides / any app)

```tsx
import { lazy, Suspense, useMemo } from "react";
const AssistantPanel = lazy(() => import("@/lib/ai/AssistantPanel")); // default export
import { useAssistantTools, type AssistantTool } from "@/lib/ai";

function SheetEditor() {
  const tools = useMemo<AssistantTool[]>(() => sheetTools(apiRef), []);   // see pages/sheets/ai/tools.ts
  useAssistantTools("sheets", tools);                                      // register once
  return (
    <Suspense fallback={null}>
      <AssistantPanel app="sheets" documentId={sheetId} documentTitle={title}
        getContext={() => ({ label: "Sheet1!A1:C4", text: "A1\tB1\n..." })}
        subscribeContext={(cb) => api.onSelectionChange(cb)}   // optional, updates the chip live
        onClose={() => setOpen(false)} />
    </Suspense>
  );
}
```

`@/` is an alias for `web/app/src` (added to vite.config.ts and tsconfig.json). Imports from `@/lib/ai` are the
only public surface: `AssistantPanel` (default export of `@/lib/ai/AssistantPanel`), `useAssistantTools`,
`registerAssistantTools`, and the types in `types.ts`.

### Props (`AssistantPanelProps`)
| prop | meaning |
| --- | --- |
| `app` | `"sheets"`, `"docs"`, `"slides"`, `"global"`, ... |
| `documentId` | stable id; chat history is stored per app+document in localStorage |
| `documentTitle` | shown in the header |
| `tools?` | explicit tools; default = tools registered with `useAssistantTools(app, ...)` |
| `getContext?` | returns `{label, text}` of the selection: label shows in the chip, text is sent as DATA (cut at 8000 chars) |
| `subscribeContext?` | `(cb) => unsubscribe`, call `cb` when the selection changes |
| `systemHint?` | extra app guidance (cannot override safety rules) |
| `suggestions?` | chips shown on an empty chat; defaults in `lib/ai/defaults.ts` |
| `onClose?` | shows a close button |
| `onInsertText?` | shows an **Insert** button on answers; receives the Markdown text |

### Tools (`AssistantTool`)
```ts
{ name: "sheet_write_cells", description: "...", risk: "write",
  parameters: { type: "object", properties: {...}, required: [...] },
  label: (a) => `Write ${...} cells`,
  handler: async (args) => ({ result: {...}, summary: "Wrote 12 cells in B2:D5", undo: () => restore() }) }
```
* `risk: "read"` runs at once. `"write"` runs at once and the panel shows an **Undo** button (needs `undo`).
  `"destructive"` asks the user to Allow/Decline in the panel first; a decline is reported to the model.
* Handlers run in the browser against the open document and must validate their own arguments (the loop also
  validates against `parameters`). Return small results (truncated at 6000 chars). Return `{error}` to let the model retry.
* `undo` must restore exactly the state before this call. "Undo all AI changes" runs undos newest first.
* Tool results and document text are DATA: the server wraps them as untrusted and tells the model never to follow
  instructions found in them. Mark anything hard to reverse as `destructive`.
* Keep tool definitions in `pages/<app>/ai/tools.ts`, importing only from `@/lib/ai`.

## Wire protocol
`GET /api/v1/ai/status?app=sheets` returns `{enabled, configured, reason?, model?, local_only?, notice?, is_admin?}`.
`reason` is one of `not_configured`, `kill_switch`, `disabled_for_app`, `disabled_for_user`, `quota`.

`POST /api/v1/ai/chat` takes a JSON `ChatRequest` and answers `text/event-stream`. Each SSE `data:` line is a `StreamEvent`:
`delta {text}`, `tool_call {call:{id,name,arguments}}`, `source {title,url?,app?}`, `usage`, `error {code,message}`,
`done {finish}`. Tool definitions carry their `risk`; for a user who can only view the document the server
drops every non-`read` tool. `attachments` (`[{name,text}]`, Drive text files) are wrapped as untrusted data.
The browser executes `tool_call`s and calls `/chat` again with the `tool` messages appended
(`runAssistantTurn` in `toolLoop.ts`, max 8 steps). The server only accepts tool names declared in the same request.

## Reference tool sets

| App | File | Adapter | Mounted |
| --- | --- | --- | --- |
| Sheets | `pages/sheets/ai/tools.ts` (`createSheetTools`, `sheetContext`) | `pages/sheets/ai/fortuneAdapter.ts` | yes: **Assistant** button in the sheet header (`pages/sheets/ai/SheetsAssistant.tsx`) |
| Docs | `pages/docs/ai/tools.ts` (`createDocTools`, `docContext`) | `pages/docs/ai/tiptapAdapter.ts` (`tiptapAdapter(() => editor)`) | no, the Docs helper mounts it |
| Slides | `pages/slides/ai/tools.ts` (`createSlideTools`, `slideContext`) | `pages/slides/ai/deckAdapter.ts` (`deckAdapter({getSlides,setSlides,getCurrent,layouts?,applyLayout?})`) | no, the Slides helper mounts it |

Tools whose adapter method is missing are not offered (no fake tools). Mounting Docs, for example:

```tsx
const ad = useMemo(() => tiptapAdapter(() => editorRef.current), []);
const tools = useMemo(() => createDocTools(ad), [ad]);
<AssistantPanel app="docs" documentId={docId} tools={tools} getContext={() => docContext(ad, title)} />
```

## Ask Adrient
Typing in the global search shows **Ask Adrient: <query>** as the first row (Enter with nothing selected also
asks). It calls `POST /api/v1/ai/ask`, which searches the caller's own data with the global search and answers
with numbered sources, shown as links. See docs/services/ai.md for the server side.

## Productivity apps

`AssistantSlot` (`components/AssistantSlot.tsx`) is the single header button plus right-hand panel; each app passes its tools.
Tool sets live in `pages/<app>/ai/tools.ts` (adapter + tools, unit-tested) with a `<App>Assistant.tsx` that wires the real API.

| App | Tools | Confirmation / undo |
| --- | --- | --- |
| Mail | find, read thread, draft reply (opens compose, never sends), extract tasks, label, move (trash/spam/inbox), mark read | all changes except the draft confirm; all undoable. No scheduled send and no Archive folder exist in Mail, so neither is offered. |
| Calendar | list events, find free slot (org free/busy when allowed), create event, move/edit one-off event, meeting context from search | create/update confirm; undo. Recurring events and deletes are not offered. |
| Drive | list folder, search, read text file, create folder, move files | create/move confirm; undo. No delete. |
| Chat | list channels, read channel/thread, post message, create tasks | post/tasks confirm; undo deletes. |
| Contacts | find, duplicate report, open contact, update notes | notes confirm; undo. No merge/delete. |
| Tasks | list, plan context (with today's events), create (with subtasks), update, set priorities | confirm; undo. No delete. |
| Keep | find, create note/checklist, rewrite note | confirm; undo. |
| Forms | get, summarise responses, add/update questions, set details, create form | confirm; undo. |

Mail bodies, chat messages, form responses, file text and contact notes are untrusted data (system prompt per app in `internal/ai/prompt.go`).
The admin AI page lists every app with its own switch (`ai.Apps`); app access and the `ai.assistant` control apply as for Sheets.
`go run ./cmd/aifake` understands `@@tool NAME {json}` lines in the user message to drive tool calls without a model.

## Business apps (CRM, HR, Talent, Accounting, Inventory, Invoicing, POS, Projects, Tickets, Admin)

Each of these apps shows one **Assistant** button in its header (`components/BizAssistant.tsx`, built on
`AssistantPanel`) and has its tools in `pages/<app>/ai/tools.ts`, built with `lib/ai/biz.ts` (`bizTool`).

**Retrieval runs as the user.** Tools run in the browser and call the module REST APIs with the signed-in
user's own session. There is no service account and no server-side data access in the gateway, so every
module's own checks (org scoping, HR roles, `accounting:ledger:*`, record sharing) apply to every tool call. A
403 comes back to the model as "no permission".

**Second line: capability-gated tool lists.** `internal/ai/policy.go` maps tool names to a capability
(`ToolCaps`). `Gateway.prepare` drops every declared tool the caller lacks the capability for, so the model is never
shown, for example, `hr_payroll_summary` to an employee or `acct_report` to a sales rep, and a call to a filtered tool
is rejected like any undeclared tool. Capabilities come from `Caller.Can` (wired in `internal/server/ai_wiring.go`:
platform permissions through authz, `hr.admin` and `talent.recruiter` through the HR and Talent role tables).

**Writes.** Every tool that changes data has risk `destructive`: the panel shows what will change and waits for
Allow/Decline. Each write returns an `undo`. Reads of personal or financial data and every write are audited through
`POST /api/v1/ai/audit` (tool name, status, one-line summary, record id; never arguments or results). The endpoint only
accepts tools the caller could have been offered and respects the kill switch and per-app switches.

**Untrusted text.** CRM notes, ticket bodies, resumes, memos and bank descriptions are returned as
`{untrusted_text}`; the server additionally wraps all tool results as untrusted data and redacts secrets.

**What each app can do** (see the `ToolCaps` table for the permissions):

| App | Reads | Writes (confirmed, audited) |
| --- | --- | --- |
| CRM | find/get deal, company summary, forecast numbers, stalled deals, follow-up e-mail context | log activity, create follow-up, move deal, bulk update (max 30, no bulk close) |
| HR | own leave balance and requests, leave policy and holidays, team availability; HR admins: headcount, attrition, payroll totals | request leave (for yourself only). No tool returns anyone's pay, bank or ID data |
| Talent (recruiters) | jobs, candidates, candidate summary, keyword screening with evidence, interview-question facts, offer facts | draft job (status draft), offer record (not sent) |
| Accounting | accounts, reports, spend by topic, rule-based anomaly review, pending bank lines, journal draft check | set bank-line category (does not post), post a reviewed journal |
| Inventory | low stock, product lookup, reorder suggestions by supplier | draft purchase order |
| Invoicing | documents, overdue, aging, customers | draft invoice or estimate |
| POS | day summary, product lookup | none |
| Projects | teams, issues, status summary, sprint proposal | create issues |
| Tickets | list, ticket, reply context from the knowledge base, triage context | priority/status/assignee change, internal note. Never a customer-visible reply |
| Admin | users, effective access, audit digest, policy facts | none (read only by design) |

**Admin page.** `/admin/ai` lists every app in `ai.Apps`; each has an enable switch (`disabled_apps`).

## Operations apps

Same model as the business apps: `BizAssistant` in the header, tools in `pages/<app>/ai/tools.ts` built with `bizTool`, run in the browser as the signed-in user, writes ask for confirmation and are undoable where the app allows it, the module API is the permission check.

| App | Reads | Writes (confirmed) |
| --- | --- | --- |
| Shifts | own schedule, week overview with open shifts and understaffed days, open shifts, ranked assignee suggestions (managers) | assign shift, create draft shift, request swap |
| Time tracking | own hours, timer status; admins: unbilled by client, report (amounts) | log time, start timer |
| Field service | dashboard, visits, jobs by status | create request, reschedule visit, complete visit |
| Fleet | overview, vehicles, reminders and renewals, open issues | report issue, record meter reading |

Notes, task names, request details, job instructions, issue text and inspection notes are untrusted data (prompts in `internal/ai/prompt.go`). No tool sends a quote or an invoice, posts to Accounting, or shows other people's pay. Members never get billing amounts from Time tracking.
