import { call, qs } from "../ops/http";

export interface Vehicle {
  id: string; name: string; vin: string; plate: string; make: string; model: string; year: number; type: string; fuel_type: string; status: string; group: string;
  meter: number; assignee_id: string; assignee_name?: string; purchase_date: string; purchase_price: number; notes: string; archived: boolean;
  open_issues?: number; overdue?: number; due_soon?: number;
}
export interface Interval { every: number; unit: string }
export interface HistoryItem { date: string; meter: number; on_time: boolean }
export interface Reminder {
  id: string; vehicle_id: string; title: string; interval: Interval; meter_interval: number; start_date: string; start_meter: number; last_date: string; last_meter: number;
  due_soon_days: number; due_soon_meter: number; snoozed_until: string; history: HistoryItem[]; event_id: string;
  status?: string; next_date?: string; next_meter?: number; days_left?: number; meter_left?: number;
}
export interface Renewal {
  id: string; vehicle_id: string; kind: string; title: string; due_date: string; repeat: Interval; cost: number; due_soon_days: number; snoozed_until: string; done: boolean;
  history: { date: string; due_date: string; cost: number }[]; event_id: string; status?: string; days_left?: number;
}
export interface Fuel { id: string; vehicle_id: string; date: string; quantity: number; total: number; meter: number; vendor: string; full_tank: boolean; economy?: number; expense_id: string }
export interface Service { id: string; vehicle_id: string; date: string; meter: number; vendor: string; description: string; cost: number; reminder_ids: string[]; expense_id: string }
export interface MeterReading { id: string; vehicle_id: string; date: string; value: number; source: string; note: string }
export interface Issue { id: string; vehicle_id: string; title: string; description: string; priority: string; status: string; reported_by: string; assignee_id: string; due_date: string; resolution: string; source: string; created_at?: string }
export interface InspectionItem { label: string; result: "pass" | "fail" | "na"; note: string }
export interface Inspection { id: string; vehicle_id: string; date: string; meter: number; by_name?: string; items: InspectionItem[]; failed: number; issue_ids: string[] }
export interface Settings { due_soon_days: number; due_soon_meter: number; meter_unit: string; currency: string; inspection_items: string[]; expense_category_id: string }
export interface Dashboard {
  vehicles: number; by_status: Record<string, number>; open_issues: number; overdue_reminders: Reminder[] | null; due_soon_reminders: Reminder[] | null; renewals_due: Renewal[] | null;
  months: { month: string; fuel: number; service: number }[]; cost_per_distance: number; failed_inspections_30d: number;
}
export interface Costs { fuel: number; service: number; total: number; distance: number; per_distance: number }
export interface Category { id: string; code: string; name: string }
export interface Me { user_id: string; admin: boolean }

const B = "/api/v1/fleet";
export const fleet = {
  me: () => call<Me>(`${B}/me`),
  settings: () => call<Settings>(`${B}/settings`),
  saveSettings: (s: Settings) => call<Settings>(`${B}/settings`, "PUT", s),
  dashboard: () => call<Dashboard>(`${B}/dashboard`),
  vin: (vin: string) => call<{ vin: string; valid: boolean; check_digit_ok: boolean; year: number }>(`${B}/vin/check${qs({ vin })}`),
  vehicles: (f: { status?: string; q?: string } = {}) => call<{ vehicles: Vehicle[] }>(`${B}/vehicles${qs(f)}`).then((r) => r.vehicles),
  vehicle: (id: string) => call<{ vehicle: Vehicle; costs: Costs }>(`${B}/vehicles/${id}`),
  saveVehicle: (id: string, v: Partial<Vehicle>) => call<{ vehicle: Vehicle; warnings: string[] }>(id ? `${B}/vehicles/${id}` : `${B}/vehicles`, id ? "PUT" : "POST", v),
  meter: (vehicle: string) => call<{ readings: MeterReading[] }>(`${B}/meter${qs({ vehicle })}`).then((r) => r.readings),
  addMeter: (vehicle_id: string, value: number, date: string, force = false) => call<{ vehicle: Vehicle; warnings: string[] }>(`${B}/meter`, "POST", { vehicle_id, value, date, force }),
  fuel: (vehicle: string) => call<{ fuel: Fuel[] }>(`${B}/fuel${qs({ vehicle })}`).then((r) => r.fuel),
  addFuel: (f: Partial<Fuel>) => call<{ fuel: Fuel; warnings: string[] }>(`${B}/fuel`, "POST", f),
  service: (vehicle: string) => call<{ service: Service[] }>(`${B}/service${qs({ vehicle })}`).then((r) => r.service),
  addService: (s: Partial<Service>) => call<Service>(`${B}/service`, "POST", s),
  reminders: (vehicle = "") => call<{ reminders: Reminder[] }>(`${B}/reminders${qs({ vehicle })}`).then((r) => r.reminders),
  saveReminder: (id: string, r: Partial<Reminder>) => call<Reminder>(id ? `${B}/reminders/${id}` : `${B}/reminders`, id ? "PUT" : "POST", r),
  completeReminder: (id: string, date: string, meter: number) => call<Reminder>(`${B}/reminders/${id}/complete`, "POST", { date, meter }),
  snoozeReminder: (id: string, until: string) => call<Reminder>(`${B}/reminders/${id}/snooze`, "POST", { until }),
  deleteReminder: (id: string) => call(`${B}/reminders/${id}`, "DELETE"),
  renewals: (vehicle = "") => call<{ renewals: Renewal[] }>(`${B}/renewals${qs({ vehicle })}`).then((r) => r.renewals),
  saveRenewal: (id: string, r: Partial<Renewal>) => call<Renewal>(id ? `${B}/renewals/${id}` : `${B}/renewals`, id ? "PUT" : "POST", r),
  completeRenewal: (id: string, date: string, cost: number) => call<Renewal>(`${B}/renewals/${id}/complete`, "POST", { date, cost }),
  issues: (vehicle = "", status = "") => call<{ issues: Issue[] }>(`${B}/issues${qs({ vehicle, status })}`).then((r) => r.issues),
  saveIssue: (id: string, i: Partial<Issue>) => call<Issue>(id ? `${B}/issues/${id}` : `${B}/issues`, id ? "PUT" : "POST", i),
  checklist: () => call<{ items: string[] }>(`${B}/checklist`).then((r) => r.items),
  inspections: (vehicle = "") => call<{ inspections: Inspection[] }>(`${B}/inspections${qs({ vehicle })}`).then((r) => r.inspections),
  submitInspection: (i: Partial<Inspection>) => call<Inspection>(`${B}/inspections`, "POST", i),
  categories: () => call<{ categories: Category[] }>(`${B}/expenses/categories`).then((r) => r.categories),
  postExpense: (kind: "fuel" | "service", id: string, category_id: string) => call<{ expense_id: string }>(`${B}/expenses`, "POST", { kind, id, category_id }),
};
