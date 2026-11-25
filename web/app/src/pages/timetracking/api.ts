import { call, qs } from "../ops/http";

export interface Client { id: string; name: string; contact_id: string; email: string; color: string; currency: string; rate: number | null; rounding: number | null; minimum_minutes: number | null; tax_rate: number | null; terms_days: number | null; archived: boolean; unbilled_amount?: number; unbilled_seconds?: number }
export interface Project { id: string; name: string; client_id: string; color: string; rate: number | null; billable: boolean; budget_hours: number; budget_amount: number; archived: boolean }
export interface Settings { default_rate: number; currency: string; rounding: number; minimum_minutes: number; tax_rate: number; terms_days: number; payroll_rate: number; payroll_rates: Record<string, number>; long_day_hours: number }
export interface Entry {
  id: string; user_id: string; user_name?: string; project_id: string; date: string; seconds: number; billable: boolean; task: string; note: string; start: string; end: string;
  rate: number | null; invoice_id: string; billed_seconds: number; amount?: number;
}
export interface Timer { project_id: string; task: string; billable: boolean; started_at: string; elapsed_seconds: number }
export interface ReportRow { key: string; label: string; entries: number; seconds: number; billed_seconds: number; amount?: number }
export interface DocRef { id: string; kind: string; number: string; status: string; total: number; paid: number; balance: number }
export interface InvoiceRow extends DocRef { client: string; entries: number; hours: number }
export interface Me { user_id: string; admin: boolean; hr_admin: boolean; employee_id: string }
export interface PayLine { employee_id: string; hours: number; amount: number }
export interface Contact { id: string; name: string; email: string; phone: string; company: string }

export interface EntryFilter { from?: string; to?: string; user?: string; project?: string; client?: string; unbilled?: boolean }
const B = "/api/v1/timetracking";
const f = (x: EntryFilter) => qs({ from: x.from, to: x.to, user: x.user, project: x.project, client: x.client, unbilled: x.unbilled ? "1" : "" });

export const tt = {
  me: () => call<Me>(`${B}/me`),
  settings: () => call<Settings>(`${B}/settings`),
  saveSettings: (s: Settings) => call<Settings>(`${B}/settings`, "PUT", s),
  clients: (archived = false) => call<{ clients: Client[] }>(`${B}/clients${qs({ archived: archived ? "1" : "" })}`).then((r) => r.clients),
  saveClient: (id: string, c: Partial<Client>) => call<Client>(id ? `${B}/clients/${id}` : `${B}/clients`, id ? "PUT" : "POST", c),
  projects: (archived = false) => call<{ projects: Project[] }>(`${B}/projects${qs({ archived: archived ? "1" : "" })}`).then((r) => r.projects),
  saveProject: (id: string, p: Partial<Project>) => call<Project>(id ? `${B}/projects/${id}` : `${B}/projects`, id ? "PUT" : "POST", p),
  contacts: (q: string) => call<{ contacts: Contact[] }>(`${B}/contacts${qs({ q })}`).then((r) => r.contacts),
  entries: (x: EntryFilter) => call<{ entries: Entry[] }>(`${B}/entries${f(x)}`).then((r) => r.entries),
  saveEntry: (id: string, e: Partial<Entry>) => call<{ entry: Entry; warnings: string[] }>(id ? `${B}/entries/${id}` : `${B}/entries`, id ? "PUT" : "POST", e),
  deleteEntry: (id: string) => call(`${B}/entries/${id}`, "DELETE"),
  setCell: (project_id: string, date: string, seconds: number) => call<{ seconds: number }>(`${B}/entries/cell`, "POST", { project_id, date, seconds }),
  timer: () => call<{ timer: Timer | null }>(`${B}/timer`).then((r) => r.timer),
  start: (project_id: string, task: string, billable: boolean) => call<{ timer: Timer; saved: { entry: Entry } | null }>(`${B}/timer/start`, "POST", { project_id, task, billable }),
  stop: () => call<{ entry: Entry; warnings: string[] }>(`${B}/timer/stop`, "POST"),
  discard: () => call(`${B}/timer`, "DELETE"),
  report: (x: EntryFilter, group: string) => call<{ rows: ReportRow[] }>(`${B}/report${f(x)}${f(x) ? "&" : "?"}group=${group}`).then((r) => r.rows),
  reportCsv: (x: EntryFilter, group: string) => `${B}/report.csv${f(x)}${f(x) ? "&" : "?"}group=${group}`,
  entriesCsv: (x: EntryFilter) => `${B}/entries.csv${f(x)}`,
  invoices: () => call<{ invoices: InvoiceRow[] }>(`${B}/invoices`).then((r) => r.invoices),
  invoice: (client_id: string, from: string, to: string) => call<{ document: DocRef; entries: number; hours: number }>(`${B}/invoices`, "POST", { client_id, from, to }),
  payPreview: (period: string) => call<{ lines: PayLine[]; skipped: string[] }>(`${B}/payroll-preview${qs({ period })}`),
  payExport: (period: string) => call<{ result: { applied: unknown[]; skipped: { reason: string }[]; created_run: boolean; period: string }; skipped_people: string[] }>(`${B}/payroll-export`, "POST", { period }),
};
