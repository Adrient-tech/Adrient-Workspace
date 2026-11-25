import { jf } from "../know/http";

export interface Settings { number_format: string; increment_min: number; default_rate: number; currency: string; terms_days: number; tax_rate_bp: number; tax_name: string; practice_areas: string[] }
export interface Client { id: string; contact_id?: string; kind: "person" | "company"; name: string; email: string; phone: string; address: string; rate: number; notes: string; archived: boolean; matters: number; open_matters: number }
export type ClientInput = Partial<Omit<Client, "id" | "matters" | "open_matters">>;
export interface Party { id?: string; name: string; role: string; contact_id?: string; email?: string; phone?: string }
export interface Deadline { id: string; matter_id: string; matter_title?: string; matter_number?: string; title: string; kind: string; due_date: string; assignee?: string; assignee_id?: string; done: boolean; notes: string; urgency: string; days_left: number; reminders: boolean }
export interface DocLink { id: string; kind: "drive" | "signature" | "contract"; ref_id: string; name: string; category: string; status?: string }
export interface StatusLog { from: string; to: string; note: string; by: string; at: string }
export interface Matter {
  id: string; seq: number; number: string; title: string; description: string; client_id: string; client_name: string; practice_area: string; stage: string; status: string; next_statuses: string[];
  responsible_id: string; responsible: string; originating_id: string; open_date: string; pending_date: string; closed_date: string; sol_date: string; sol_days?: number;
  billing_method: "hourly" | "flat" | "contingency"; rate: number; flat_fee: number; flat_fee_billed: boolean; contingency_bp: number; budget: number; tags: string[]; access: "org" | "restricted"; shared_with: string[];
  conflict_checked_at: string; conflict_waiver: string; minutes: number; unbilled_minutes: number; unbilled: number; billed: number; can_edit: boolean;
  parties?: Party[]; deadlines?: Deadline[]; documents?: DocLink[]; status_log?: StatusLog[];
}
export type MatterInput = Partial<Pick<Matter, "title" | "description" | "client_id" | "practice_area" | "stage" | "responsible_id" | "sol_date" | "billing_method" | "rate" | "flat_fee" | "contingency_bp" | "budget" | "tags" | "access" | "shared_with">> & { parties?: Party[] };
export interface Entry { id: string; matter_id: string; matter_number?: string; matter_title?: string; kind: "time" | "expense" | "flat"; user_id: string; user: string; date: string; minutes: number; rate: number; qty?: string; amount: number; description: string; billable: boolean; bill_id?: string; source?: string }
export interface EntryInput { kind: string; date?: string; duration?: string; minutes?: number; price?: number; qty?: string; amount?: number; description?: string; billable?: boolean; user_id?: string }
export interface Line { entry_id?: string; kind: string; date: string; description: string; minutes?: number; rate?: number; qty?: string; amount: number }
export interface Bill { id: string; matter_id: string; matter_number?: string; matter_title?: string; client_name?: string; number: string; status: "draft" | "issued" | "void"; issue_date: string; due_date: string; period_from: string; period_to: string; fees: number; expenses: number; discount: number; tax: number; total: number; invoice_id?: string; invoice_number?: string; lines?: Line[]; note?: string }
export interface Note { id: string; kind: string; body: string; author: string; created_at: string }
export interface ConflictHit { source: string; id: string; name: string; role?: string; score: number; matters: { id?: string; number?: string; title?: string; status?: string; role: string; restricted?: boolean }[] }
export interface ConflictResult { query: string; hits: ConflictHit[]; clear: boolean }
export interface Overview {
  by_status: Record<string, number>; open_matters: number; unbilled: number; unbilled_minutes: number; billed_30_days: number; minutes_this_month: number; overdue_deadlines: number; deadlines_next_7_days: number;
  deadlines: Deadline[]; limitation_watch: Matter[]; recent_matters: Matter[]; currency: string;
}
export interface Meta { settings: Settings; statuses: string[]; today: string; admin: boolean; invoicing: boolean; calendar: boolean; tasks: boolean; drive: boolean; time_tracking: boolean; contacts: boolean }
export interface TimeCand { id: string; user_id: string; date: string; seconds: number; note: string; project: string; billable: boolean }
export interface UserHours { user_id: string; user: string; minutes: number; billable_minutes: number; value: number }

const B = "/api/v1/legal";
const qs = (o: Record<string, string | undefined>) => {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(o)) if (v) p.set(k, v);
  const s = p.toString();
  return s ? `?${s}` : "";
};

export const legal = {
  meta: () => jf<Meta>(`${B}/meta`),
  overview: () => jf<Overview>(`${B}/overview`),
  people: () => jf<{ people: { id: string; name: string }[]; me: string }>(`${B}/people`),
  saveSettings: (s: Settings) => jf<Settings>(`${B}/settings`, { method: "PUT", body: s }),
  contacts: (q: string) => jf<{ contacts: { id: string; name: string; company: string; email: string; phone: string }[] }>(`${B}/contacts${qs({ q })}`).then((r) => r.contacts),
  clients: (q = "", archived = false) => jf<{ clients: Client[] }>(`${B}/clients${qs({ q, archived: archived ? "1" : "" })}`).then((r) => r.clients),
  client: (id: string) => jf<Client>(`${B}/clients/${id}`),
  createClient: (c: ClientInput) => jf<Client>(`${B}/clients`, { method: "POST", body: c }),
  updateClient: (id: string, c: ClientInput) => jf<Client>(`${B}/clients/${id}`, { method: "PUT", body: c }),
  clientFromContact: (id: string) => jf<Client>(`${B}/clients/${id}/from-contact`, { method: "POST" }),
  matters: (f: { q?: string; status?: string; client?: string; area?: string; mine?: boolean } = {}) =>
    jf<{ matters: Matter[] }>(`${B}/matters${qs({ q: f.q, status: f.status, client: f.client, area: f.area, mine: f.mine ? "1" : "" })}`).then((r) => r.matters),
  matter: (id: string) => jf<Matter>(`${B}/matters/${id}`),
  createMatter: (m: MatterInput) => jf<Matter>(`${B}/matters`, { method: "POST", body: m }),
  updateMatter: (id: string, m: MatterInput) => jf<Matter>(`${B}/matters/${id}`, { method: "PUT", body: m }),
  deleteMatter: (id: string) => jf<{ ok: boolean }>(`${B}/matters/${id}`, { method: "DELETE" }),
  move: (id: string, b: { status: string; note?: string; force?: boolean; waive_conflict?: string }) => jf<Matter>(`${B}/matters/${id}/status`, { method: "POST", body: b }),
  conflictCheck: (q: string) => jf<ConflictResult>(`${B}/conflicts${qs({ q })}`),
  recordConflict: (id: string, query: string) => jf<ConflictResult>(`${B}/matters/${id}/conflict`, { method: "POST", body: { query } }),
  deadlines: (done = false) => jf<{ deadlines: Deadline[] }>(`${B}/deadlines${qs({ done: done ? "1" : "" })}`).then((r) => r.deadlines),
  addDeadline: (matter: string, d: { title: string; kind?: string; due_date: string; assignee_id?: string; notes?: string }) => jf<Deadline>(`${B}/matters/${matter}/deadlines`, { method: "POST", body: d }),
  updateDeadline: (id: string, d: { title: string; kind?: string; due_date: string; assignee_id?: string; notes?: string }) => jf<Deadline>(`${B}/deadlines/${id}`, { method: "PUT", body: d }),
  deadlineDone: (id: string, done: boolean) => jf<Deadline>(`${B}/deadlines/${id}/${done ? "done" : "reopen"}`, { method: "POST" }),
  deleteDeadline: (id: string) => jf<{ ok: boolean }>(`${B}/deadlines/${id}`, { method: "DELETE" }),
  entries: (f: { matter?: string; billed?: string; from?: string; to?: string; kind?: string } = {}) => jf<{ entries: Entry[] }>(`${B}/entries${qs(f)}`).then((r) => r.entries),
  addEntry: (matter: string, e: EntryInput) => jf<Entry>(`${B}/matters/${matter}/entries`, { method: "POST", body: e }),
  updateEntry: (id: string, e: EntryInput) => jf<Entry>(`${B}/entries/${id}`, { method: "PUT", body: e }),
  deleteEntry: (id: string) => jf<{ ok: boolean }>(`${B}/entries/${id}`, { method: "DELETE" }),
  timeCandidates: () => jf<{ entries: TimeCand[] }>(`${B}/time-candidates`).then((r) => r.entries),
  importTime: (matter: string, ids: string[]) => jf<{ entries: Entry[] }>(`${B}/matters/${matter}/import-time`, { method: "POST", body: { ids } }),
  bills: (matter = "") => jf<{ bills: Bill[] }>(`${B}/bills`).then((r) => (matter ? r.bills.filter((b) => b.matter_id === matter) : r.bills)),
  bill: (id: string) => jf<Bill>(`${B}/bills/${id}`),
  draftBill: (matter: string, b: { from?: string; to?: string; discount_bp?: number; preview?: boolean }) => jf<Bill>(`${B}/matters/${matter}/bills`, { method: "POST", body: b }),
  issueBill: (id: string) => jf<Bill>(`${B}/bills/${id}/issue`, { method: "POST" }),
  voidBill: (id: string) => jf<Bill>(`${B}/bills/${id}/void`, { method: "POST" }),
  notes: (matter: string) => jf<{ notes: Note[] }>(`${B}/matters/${matter}/notes`).then((r) => r.notes),
  addNote: (matter: string, kind: string, body: string) => jf<Note>(`${B}/matters/${matter}/notes`, { method: "POST", body: { kind, body } }),
  linkable: (kind: string, q: string) => jf<{ items: { id: string; name: string; status?: string }[] }>(`${B}/linkable${qs({ kind, q })}`).then((r) => r.items),
  addDocument: (matter: string, d: { kind: string; ref_id: string; category?: string }) => jf<DocLink>(`${B}/matters/${matter}/documents`, { method: "POST", body: d }),
  removeDocument: (matter: string, doc: string) => jf<{ ok: boolean }>(`${B}/matters/${matter}/documents/${doc}`, { method: "DELETE" }),
  hours: (from = "", to = "") => jf<{ rows: UserHours[] }>(`${B}/reports/hours${qs({ from, to })}`).then((r) => r.rows),
};
