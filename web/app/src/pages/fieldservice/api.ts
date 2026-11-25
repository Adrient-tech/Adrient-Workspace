import { call, qs } from "../ops/http";

export interface Item { name: string; description?: string; qty: number; unit_price: number; unit_cost: number; taxable: boolean; optional?: boolean; selected?: boolean }
export interface Discount { kind: "pct" | "amount"; value: number }
export interface Totals { subtotal: number; discount: number; tax: number; total: number; cost: number; profit: number }
export interface Schedule {
  start: string; repeat: string; interval: number; weekdays: number[]; monthly_by: string; end_type: string; end_count: number; end_date: string; start_time: string; end_time: string;
}
export interface Customer { id: string; name: string; email: string; phone: string; company: string }
export interface Property { id: string; contact_id: string; name: string; address: string; notes: string }
export interface Service { id: string; name: string; description: string; unit_price: number; unit_cost: number; taxable: boolean; archived: boolean }
export interface Request {
  id: string; contact_id: string; client_name: string; property_id: string; title: string; details: string; preferred_date: string; status: string;
  items: Item[]; tax_rate: number; quote_id: string; quote_number: string; job_id: string; created_at?: string;
}
export interface Visit {
  id: string; job_id: string; date: string; start: string; end: string; assignees: string[]; status: string; notes: string; completed_at: string; invoice_id: string;
  job_title?: string; client_name?: string; address?: string;
}
export interface Job {
  id: string; contact_id: string; client_name: string; property_id: string; title: string; instructions: string; status: string; display_status: string;
  schedule: Schedule; assignees: string[]; items: Item[]; discount: Discount | null; tax_rate: number; billing: string; quote_id: string; request_id: string;
  invoice_ids: string[]; totals: Totals; visits?: Visit[];
}
export interface DocRef { id: string; kind: string; number: string; status: string; total: number; paid: number; balance: number }
export interface Team { id: string; name: string; email: string }
export interface Settings { tax_rate: number; currency: string; terms_days: number; default_billing: string }
export interface Dashboard { new_requests: number; quotes_awaiting: number; active_jobs: number; late_jobs: number; requires_invoicing: number; visits_today: Visit[]; visits_this_week: number; outstanding_amount: number }
export interface Me { user_id: string; admin: boolean }

const B = "/api/v1/fieldservice";
export const fs = {
  me: () => call<Me>(`${B}/me`),
  team: () => call<{ team: Team[] }>(`${B}/team`).then((r) => r.team),
  settings: () => call<Settings>(`${B}/settings`),
  saveSettings: (s: Settings) => call<Settings>(`${B}/settings`, "PUT", s),
  dashboard: () => call<Dashboard>(`${B}/dashboard`),
  customers: (q: string) => call<{ customers: Customer[] }>(`${B}/customers${qs({ q })}`).then((r) => r.customers),
  createCustomer: (c: Partial<Customer>) => call<Customer>(`${B}/customers`, "POST", c),
  services: () => call<{ services: Service[] }>(`${B}/services`).then((r) => r.services),
  saveService: (id: string, s: Partial<Service>) => call<Service>(id ? `${B}/services/${id}` : `${B}/services`, id ? "PUT" : "POST", s),
  properties: (contact: string) => call<{ properties: Property[] }>(`${B}/properties${qs({ contact })}`).then((r) => r.properties),
  saveProperty: (id: string, p: Partial<Property>) => call<Property>(id ? `${B}/properties/${id}` : `${B}/properties`, id ? "PUT" : "POST", p),
  requests: (status = "") => call<{ requests: Request[] }>(`${B}/requests${qs({ status })}`).then((r) => r.requests),
  saveRequest: (id: string, r: Partial<Request>) => call<Request>(id ? `${B}/requests/${id}` : `${B}/requests`, id ? "PUT" : "POST", r),
  moveRequest: (id: string, status: string) => call<Request>(`${B}/requests/${id}/status`, "POST", { status }),
  quote: (id: string) => call<Request>(`${B}/requests/${id}/quote`, "POST"),
  quoteStatus: (id: string) => call<{ quote_id: string; quote_number: string; status: string }>(`${B}/requests/${id}/quote`),
  requestToJob: (id: string, schedule: Partial<Schedule>, assignees: string[]) => call<Job>(`${B}/requests/${id}/job`, "POST", { schedule, assignees }),
  jobs: (f: { status?: string; assignee?: string; contact?: string } = {}) => call<{ jobs: Job[] }>(`${B}/jobs${qs(f)}`).then((r) => r.jobs),
  job: (id: string) => call<{ job: Job; invoices: DocRef[]; property: string }>(`${B}/jobs/${id}`),
  saveJob: (id: string, j: Partial<Job>) => call<Job>(id ? `${B}/jobs/${id}` : `${B}/jobs`, id ? "PUT" : "POST", j),
  cancelJob: (id: string) => call<Job>(`${B}/jobs/${id}/cancel`, "POST"),
  invoiceJob: (id: string) => call<{ document: DocRef; visits: number }>(`${B}/jobs/${id}/invoice`, "POST"),
  visits: (from: string, to: string, assignee = "") => call<{ visits: Visit[] }>(`${B}/visits${qs({ from, to, assignee })}`).then((r) => r.visits),
  updateVisit: (id: string, u: Partial<{ date: string; start: string; end: string; assignees: string[]; notes: string }>) => call<Visit>(`${B}/visits/${id}`, "PUT", u),
  completeVisit: (id: string) => call<{ visit: Visit; invoice: DocRef | null }>(`${B}/visits/${id}/complete`, "POST"),
  reopenVisit: (id: string) => call<Visit>(`${B}/visits/${id}/reopen`, "POST"),
  cancelVisit: (id: string) => call<Visit>(`${B}/visits/${id}/cancel`, "POST"),
};
