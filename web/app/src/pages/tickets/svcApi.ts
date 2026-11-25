// Service desk API: paginated list, groups, calendars, SLA rules, automations, forms,
// reports, approvals, presence, drafts, time. All under /api/v1/ts/.
const TS = "/api/v1/ts";

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(path, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  if (r.status === 204) return undefined as T;
  return (await r.json()) as T;
}
const body = (v: unknown) => JSON.stringify(v);

export type TicketType = "question" | "incident" | "problem" | "task" | "request" | "change";
export const TICKET_TYPES: TicketType[] = ["question", "incident", "problem", "request", "change", "task"];
export type Priority = "low" | "normal" | "high" | "urgent";
export const PRIORITIES: Priority[] = ["urgent", "high", "normal", "low"];

export interface Row {
  id: string;
  project_id: string;
  ref: string;
  number: number;
  title: string;
  body?: string;
  status: string;
  priority: Priority;
  type: TicketType;
  tags: string[];
  requester_name: string;
  requester_email: string;
  assignee_user_id?: string;
  source: string;
  created_at: string;
  updated_at: string;
  resolved_at?: string;
  merged_into?: string;
  problem_id?: string;
  group_id?: string;
  urgency: string;
  impact: string;
  category: string;
  subcategory: string;
  parent_id?: string;
  major: boolean;
  snoozed_until?: string;
  first_response_at?: string;
  form_id?: string;
  custom: Record<string, string>;
  sentiment: string;
  ai_summary: string;
  need_skills: string[];
}
export interface ViewCount {
  id: string;
  title: string;
  count: number;
}
export interface Clock {
  state: string;
  due?: string;
  paused?: boolean;
  left_min?: number;
}
export interface Sla {
  first_response: Clock;
  resolution: Clock;
}
export interface StatusDef {
  key: string;
  label: string;
  category: "open" | "pending" | "hold" | "solved" | "closed";
  pauses_sla: boolean;
  position?: number;
  requires: string[];
}
export interface Member {
  user_id: string;
  name?: string;
  skills: string[];
  capacity: number;
  ooo_until?: string | null;
  open?: number;
}
export interface Group {
  id: string;
  project_id?: string;
  name: string;
  description: string;
  strategy: "manual" | "round_robin" | "load";
  calendar_id: string;
  email_alias: string;
  members: Member[];
  open_count?: number;
}
export interface Holiday {
  id?: string;
  day: string;
  name: string;
}
export interface Calendar {
  id: string;
  name: string;
  tz: string;
  hours: Record<string, [string, string][]>;
  is_default: boolean;
  holidays: Holiday[];
}
export interface SlaRule {
  id: string;
  name: string;
  position: number;
  enabled: boolean;
  match_priority: string;
  match_type: string;
  match_customer: string;
  first_response_min: number;
  resolve_min: number;
  business_hours: boolean;
  calendar_id: string;
}
export interface Condition {
  field: string;
  op: string;
  value: string;
}
export interface Action {
  type: string;
  value: string;
}
export interface Automation {
  id: string;
  name: string;
  kind: "trigger" | "time" | "dispatch";
  event: "create" | "update" | "any";
  match_mode: "all" | "any";
  conditions: Condition[];
  actions: Action[];
  position: number;
  enabled: boolean;
  run_count: number;
  last_run_at?: string;
}
export interface AutoTest {
  matched: boolean;
  conditions: { field: string; op: string; value: string; actual: string; pass: boolean }[];
  changes: { field: string; from: string; to: string }[];
  effects: { type: string; value: string }[];
  description: string;
}
export interface FormField {
  key: string;
  label: string;
  type: "text" | "longtext" | "select" | "date" | "checkbox";
  required: boolean;
  options?: string[];
}
export interface RequestForm {
  id: string;
  name: string;
  description: string;
  ticket_type: TicketType;
  group_id: string;
  category: string;
  fields: FormField[];
  enabled: boolean;
  position?: number;
  skills?: string[];
}
export interface Bucket {
  label: string;
  count: number;
}
export interface DayCount {
  day: string;
  count: number;
}
export interface ReportV2 {
  created: number;
  resolved: number;
  open_now: number;
  avg_first_response_min: number;
  avg_resolution_min: number;
  sla_met_pct: number;
  sla_checked: number;
  csat_avg: number;
  csat_count: number;
  by_priority: Record<string, number>;
  created_per_day: DayCount[];
  resolved_per_day: DayCount[];
  from: string;
  to: string;
  by_type: Bucket[];
  by_category: Bucket[];
  by_group: Bucket[];
  backlog: Bucket[];
  workload: { agent: string; open: number; resolved: number }[];
  sla_breached_by_priority: Bucket[];
  time_logged_min: number;
}
export interface ReportOpts {
  days?: number;
  from?: string;
  to?: string;
  group?: string;
  type?: string;
  priority?: string;
}
export interface Presence {
  user_id: string;
  name: string;
  mode: "viewing" | "replying";
}
export interface TimeEntry {
  id: string;
  user_name: string;
  minutes: number;
  note: string;
  billable: boolean;
  date: string;
}
export interface Approval {
  id: string;
  step: number;
  approver_id: string;
  approver: string;
  decision: "pending" | "approved" | "rejected";
  comment: string;
}
export interface Article {
  id: string;
  slug: string;
  title: string;
  body: string;
  published: boolean;
  category: string;
  views: number;
  helpful_yes: number;
  helpful_no: number;
}

const qs = (o: Record<string, string | number | undefined>) => {
  const p = new URLSearchParams();
  Object.entries(o).forEach(([k, v]) => v !== undefined && v !== "" && p.set(k, String(v)));
  const s = p.toString();
  return s ? `?${s}` : "";
};

export const svc = {
  list: (pid: string, o: { view?: string; q?: string; group?: string; assignee?: string; limit?: number; cursor?: string }) =>
    call<{ tickets: Row[]; next: string; views?: ViewCount[] }>(`${TS}/projects/${pid}/list${qs(o)}`),
  statuses: (pid: string) => call<{ statuses: StatusDef[]; categories: string[] }>(`${TS}/projects/${pid}/statuses`),
  saveStatuses: (pid: string, statuses: StatusDef[]) => call<{ statuses: StatusDef[] }>(`${TS}/projects/${pid}/statuses`, { method: "PUT", body: body({ statuses }) }),
  groups: (pid: string) => call<{ groups: Group[]; strategies: string[] }>(`${TS}/projects/${pid}/groups`).then((r) => r.groups),
  saveGroup: (pid: string, g: Partial<Group>) => call<Group>(`${TS}/projects/${pid}/groups`, { method: "POST", body: body(g) }),
  deleteGroup: (id: string) => call<void>(`${TS}/groups/${id}`, { method: "DELETE" }),
  setOoo: (until: string | null) => call<unknown>(`${TS}/me/ooo`, { method: "PUT", body: body({ until }) }),
  calendars: () => call<{ calendars: Calendar[] }>(`${TS}/calendars`).then((r) => r.calendars),
  saveCalendar: (c: Partial<Calendar>) => call<Calendar>(`${TS}/calendars`, { method: "POST", body: body(c) }),
  deleteCalendar: (id: string) => call<void>(`${TS}/calendars/${id}`, { method: "DELETE" }),
  slaRules: (pid: string) => call<{ rules: SlaRule[] }>(`${TS}/projects/${pid}/sla-rules`).then((r) => r.rules),
  saveSlaRule: (pid: string, r: Partial<SlaRule>) => call<SlaRule>(`${TS}/projects/${pid}/sla-rules`, { method: "POST", body: body(r) }),
  deleteSlaRule: (id: string) => call<void>(`${TS}/sla-rules/${id}`, { method: "DELETE" }),
  automations: (pid: string) =>
    call<{ automations: { rule: Automation; description: string }[]; condition_fields: Record<string, string>; action_types: string[] }>(`${TS}/projects/${pid}/automations`),
  saveAutomation: (pid: string, a: Partial<Automation>) => call<Automation>(`${TS}/projects/${pid}/automations`, { method: "POST", body: body(a) }),
  deleteAutomation: (id: string) => call<void>(`${TS}/automations/${id}`, { method: "DELETE" }),
  testAutomation: (pid: string, a: Partial<Automation>, ticketId: string) =>
    call<AutoTest>(`${TS}/projects/${pid}/automations/test`, { method: "POST", body: body({ automation: a, ticket_id: ticketId }) }),
  forms: (pid: string) => call<{ forms: RequestForm[] }>(`${TS}/projects/${pid}/forms`).then((r) => r.forms),
  saveForm: (pid: string, f: Partial<RequestForm>) => call<RequestForm>(`${TS}/projects/${pid}/forms`, { method: "POST", body: body(f) }),
  deleteForm: (id: string) => call<void>(`${TS}/forms/${id}`, { method: "DELETE" }),
  report: (pid: string, o: ReportOpts) => call<ReportV2>(`${TS}/projects/${pid}/report2${qs({ ...o })}`),
  csvUrl: (pid: string, o: ReportOpts) => `${TS}/projects/${pid}/report.csv${qs({ ...o })}`,
  savedReports: (pid: string) => call<{ reports: { id: string; name: string; params: ReportOpts }[] }>(`${TS}/projects/${pid}/saved-reports`).then((r) => r.reports),
  saveReport: (pid: string, name: string, params: ReportOpts) => call<unknown>(`${TS}/projects/${pid}/saved-reports`, { method: "POST", body: body({ name, params }) }),
  deleteReport: (id: string) => call<void>(`${TS}/saved-reports/${id}`, { method: "DELETE" }),
  articles: (pid: string) => call<{ articles: Article[] }>(`${TS}/projects/${pid}/kb`).then((r) => r.articles),
  saveArticle: (pid: string, a: Partial<Article>) => call<Article>(`${TS}/projects/${pid}/kb`, { method: a.id ? "PUT" : "POST", body: body(a) }),
  deleteArticle: (id: string) => call<void>(`${TS}/kb/${id}`, { method: "DELETE" }),
  suggest: (pid: string, q: string) => call<{ articles: Article[] }>(`${TS}/projects/${pid}/kb/suggest?q=${encodeURIComponent(q)}`).then((r) => r.articles),
  kbStats: (pid: string) => call<{ articles: number; published: number; views: number; helpful_yes: number; helpful_no: number; deflected: number }>(`${TS}/projects/${pid}/kb/stats`),
  major: () => call<{ incidents: Row[] }>(`${TS}/major`).then((r) => r.incidents),
  myApprovals: () => call<{ tickets: Row[] }>(`${TS}/approvals/mine`).then((r) => r.tickets),
  sla: (id: string) => call<Sla>(`${TS}/tickets/${id}/sla`),
  slaStates: (pid: string) => call<{ states: Record<string, Sla> }>(`${TS}/projects/${pid}/sla-state`).then((r) => r.states),
  presence: (id: string, mode: string) => call<{ others: Presence[] }>(`${TS}/tickets/${id}/presence`, { method: "POST", body: body({ mode }) }).then((r) => r.others),
  draft: (id: string) => call<{ body: string; internal: boolean }>(`${TS}/tickets/${id}/draft`),
  saveDraft: (id: string, d: { body: string; internal: boolean }) => call<unknown>(`${TS}/tickets/${id}/draft`, { method: "PUT", body: body(d) }),
  time: (id: string) => call<{ entries: TimeEntry[]; total_min: number }>(`${TS}/tickets/${id}/time`),
  addTime: (id: string, e: { minutes: number; note: string; billable: boolean }) => call<TimeEntry>(`${TS}/tickets/${id}/time`, { method: "POST", body: body(e) }),
  snooze: (id: string, until: string | null) => call<unknown>(`${TS}/tickets/${id}/snooze`, { method: "PUT", body: body({ until }) }),
  setParent: (id: string, parent: string) => call<unknown>(`${TS}/tickets/${id}/parent`, { method: "PUT", body: body({ parent_id: parent }) }),
  setMajor: (id: string, on: boolean) => call<unknown>(`${TS}/tickets/${id}/major`, { method: "PUT", body: body({ on }) }),
  setFields: (id: string, f: Record<string, unknown>) => call<Row>(`${TS}/tickets/${id}/fields`, { method: "PUT", body: body(f) }),
  detail2: (id: string) =>
    call<{ children: Row[]; approvals: Approval[]; approval_state: string; time: TimeEntry[]; time_total_min: number; sla: Sla }>(`${TS}/tickets/${id}/detail2`),
  requestApprovals: (id: string, steps: string[][]) => call<unknown>(`${TS}/tickets/${id}/approvals`, { method: "POST", body: body({ steps }) }),
  decide: (id: string, decision: "approved" | "rejected", comment: string) =>
    call<unknown>(`${TS}/tickets/${id}/approvals/decide`, { method: "POST", body: body({ decision, comment }) }),
  sweep: () => call<{ woken: number; rule_hits: number; evaluated: number; mail_tickets: number; mail_comments: number; scheduled_sent: number }>(`${TS}/sweep`, { method: "POST", body: "{}" }),
  ai: (id: string, kind: "suggest_reply" | "summary" | "sentiment" | "classify") => call<AiResult>(`${TS}/tickets/${id}/ai`, { method: "POST", body: body({ kind }) }),
  applyClassification: (id: string, c: Classification) => call<Row>(`${TS}/tickets/${id}/ai/apply`, { method: "POST", body: body(c) }),
  scheduled: (id: string) => call<{ scheduled: Scheduled[] }>(`${TS}/tickets/${id}/scheduled`).then((r) => r.scheduled),
  schedule: (id: string, s: { body: string; internal: boolean; solve: boolean; send_at: string }) => call<Scheduled>(`${TS}/tickets/${id}/scheduled`, { method: "POST", body: body(s) }),
  cancelScheduled: (id: string, sid: string) => call<void>(`${TS}/tickets/${id}/scheduled/${sid}`, { method: "DELETE" }),
  attachments: (id: string) => call<{ attachments: Attachment[] }>(`${TS}/tickets/${id}/attachments`).then((r) => r.attachments),
  attach: (id: string, fileId: string) => call<Attachment>(`${TS}/tickets/${id}/attachments`, { method: "POST", body: body({ file_id: fileId }) }),
  detach: (id: string, aid: string) => call<void>(`${TS}/tickets/${id}/attachments/${aid}`, { method: "DELETE" }),
  fieldDefs: (pid: string) => call<{ fields: FieldDef[] }>(`${TS}/projects/${pid}/fields`).then((r) => r.fields),
  saveFieldDef: (pid: string, d: Partial<FieldDef>) => call<FieldDef>(`${TS}/projects/${pid}/fields`, { method: "POST", body: body(d) }),
  deleteFieldDef: (id: string) => call<void>(`${TS}/fields/${id}`, { method: "DELETE" }),
  ooo: () => call<{ until: string | null }>(`${TS}/me/ooo`).then((r) => r.until),
  inboundMail: (m: { key: string; from: string; to: string[]; subject: string; body: string }) => call<{ outcome: string; ref?: string; reason?: string }>(`${TS}/inbound/mail`, { method: "POST", body: body(m) }),
};

// ---- small helpers shared by the desk screens ----

export function ago(iso?: string): string {
  if (!iso) return "";
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  if (s < 86400 * 30) return `${Math.floor(s / 86400)}d ago`;
  return new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short" });
}

export function fmtMin(m: number): string {
  if (!m) return "0m";
  const a = Math.abs(m);
  const d = Math.floor(a / 1440), h = Math.floor((a % 1440) / 60), mm = Math.round(a % 60);
  const parts = d ? [`${d}d`, h ? `${h}h` : ""] : h ? [`${h}h`, mm ? `${mm}m` : ""] : [`${mm}m`];
  return (m < 0 ? "-" : "") + parts.filter(Boolean).join(" ");
}

/** Label for the SLA clock chip: "2h left", "Breached 1h ago", "Paused". */
export function slaLabel(c?: Clock): { text: string; tone: "success" | "warning" | "danger" | "neutral" } {
  if (!c || !c.state) return { text: "", tone: "neutral" };
  if (c.paused) return { text: "SLA paused", tone: "neutral" };
  if (c.state === "met") return { text: "Met", tone: "success" };
  if (c.state === "breached") return { text: c.left_min != null && c.left_min < 0 ? `Late ${fmtMin(-c.left_min)}` : "Breached", tone: "danger" };
  if (c.state === "at_risk") return { text: `${fmtMin(c.left_min ?? 0)} left`, tone: "warning" };
  return { text: `${fmtMin(c.left_min ?? 0)} left`, tone: "success" };
}

export interface Classification {
  type: string;
  priority: string;
  category: string;
  group: string;
  group_id: string;
  reason: string;
}
export interface AiResult {
  kind: string;
  text?: string;
  sentiment?: { sentiment: string; reason: string };
  classification?: Classification;
  sources?: { id: string; title: string }[];
}
export interface Scheduled {
  id: string;
  body: string;
  internal: boolean;
  solve: boolean;
  send_at: string;
  state: string;
  error?: string;
  user_id: string;
  user_name: string;
}
export interface Attachment {
  id: string;
  file_id: string;
  name: string;
  size: number;
  added_by: string;
}
export interface FieldDef {
  id: string;
  key: string;
  label: string;
  kind: "text" | "longtext" | "number" | "select" | "date" | "checkbox";
  options: string[];
  required_on: string[];
  ticket_type: string;
  position: number;
}

export const SENTIMENT_COLOR: Record<string, "success" | "neutral" | "warning" | "danger"> = { positive: "success", neutral: "neutral", negative: "warning", angry: "danger" };

export function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}
