// Client for /api/v1/hr (internal/hr). Same-origin fetch; error bodies surface as Error messages.

export interface Employee {
  id: string;
  user_id: string;
  contact_id: string;
  emp_code: string;
  first_name: string;
  last_name: string;
  name: string;
  work_email: string;
  personal_email: string;
  phone: string;
  dob: string;
  gender: string;
  marital_status: string;
  blood_group: string;
  address: string;
  join_date: string;
  probation_end: string;
  exit_date: string;
  exit_reason: string;
  status: string;
  employment_type: string;
  department_id: string;
  department_name: string;
  designation_id: string;
  designation_name: string;
  location_id: string;
  location_name: string;
  manager_id: string;
  manager_name: string;
  shift_id: string;
  pan: string;
  aadhaar_last4: string;
  uan: string;
  esi_number: string;
  bank_name: string;
  bank_account: string;
  bank_ifsc: string;
  full: boolean;
}

export interface Lookup { id: string; name: string; state?: string; address?: string; parent_id?: string; count: number }
export interface LeaveType {
  id: string; code: string; name: string; color: string; paid: boolean; tracked: boolean; annual_quota: number;
  accrual: string; carry_forward: boolean; carry_cap: number; max_balance: number | null; allow_half_day: boolean;
  employment_types: string[]; active: boolean;
}
export interface Balance { type_id: string; code: string; name: string; color: string; paid: boolean; tracked: boolean; credited: number; used: number; pending: number; balance: number; year: number }
export interface LeaveRequest {
  id: string; employee_id: string; employee_name: string; emp_code: string; type_id: string; type_name: string; type_color: string;
  from_date: string; to_date: string; half_day: string; days: number; reason: string; status: string; decided_by: string;
  decision_note: string; created_at: string; can_decide: boolean; can_cancel: boolean;
}
export interface TeamLeave { employee_id: string; name: string; type_name: string; color: string; from_date: string; to_date: string; half_day: string; status: string }
export interface Holiday { id: string; day: string; name: string; optional: boolean }
export interface HolidayCalendar { id: string; name: string; location_id: string; holidays: Holiday[] }
export interface DayResult {
  date: string; status: string; first_in: string; last_out: string; worked_min: number; late: boolean; late_min: number;
  early_out: boolean; ot_min: number; leave: string; holiday: string; lop: number; punches: number;
}
export interface Today { date: string; punches: { at: string; source: string }[]; checked_in: boolean; day: DayResult; geofence: boolean }
export interface Timesheet { employee_id: string; name: string; period: string; days: DayResult[]; summary: Record<string, number> }
export interface MonthlyRow {
  employee_id: string; emp_code: string; name: string; present: number; half_days: number; absent: number; leave: number;
  holidays: number; weekly_offs: number; late: number; early_out: number; worked_hours: number; ot_hours: number; lop: number;
}
export interface Shift { id: string; name: string; is_default: boolean; start_time: string; end_time: string; grace_minutes: number; early_minutes: number; full_day_mins: number; half_day_mins: number; ot_after_mins: number }
export interface Regularisation { id: string; employee_id: string; employee_name: string; day: string; in_time: string; out_time: string; reason: string; status: string; decision_note: string; can_decide: boolean; can_cancel: boolean }
export interface SalaryLine { code: string; name: string; kind: string; calc: string; value: number; amount: number; pf_wage: boolean; esi_wage: boolean; taxable: boolean }
export interface Structure { id: string; name: string; lines: SalaryLine[] }
export interface EmployeeSalary { id: string; employee_id: string; effective_from: string; structure_id: string; annual_gross: number; monthly_gross: number; lines: SalaryLine[]; pf_enabled: boolean; esi_enabled: boolean; pt_enabled: boolean; tds_enabled: boolean }
export interface PTSlab { state: string; min_salary: number; max_salary: number | null; amount: number; month: number }
export interface Run { id: string; period: string; status: string; journal_id: string; locked_at: string; employees: number; gross: number; net_pay: number; pf: number; esi: number; prof_tax: number; tds: number; skipped?: string[] }
export interface Amt { code: string; name: string; amount: number }
export interface Adjustment { kind: string; name: string; amount: number; expense_report_id?: string }
export interface Payslip {
  id: string; run_id: string; period: string; run_status: string; employee_id: string; emp_code: string; emp_name: string;
  days_in_month: number; lop_days: number; paid_days: number; earnings: Amt[]; deductions: Amt[]; adjustments: Adjustment[];
  gross: number; pf_employee: number; pf_employer: number; esi_employee: number; esi_employer: number; prof_tax: number; tds: number;
  other_deductions: number; reimbursements: number; net_pay: number;
}
export interface Settings {
  timezone: string; weekly_off: number[]; leave_year_start: number; geofence_enabled: boolean; geofence_lat: number; geofence_lng: number;
  geofence_radius_m: number; lop_mode: string; ot_enabled: boolean; pf_wage_ceiling: number; pf_capped: boolean; esi_wage_limit: number;
  company_name: string; company_address: string;
}
export interface Celebration { employee_id: string; user_id: string; name: string; kind: string; date: string; years?: number }
export interface Document { id: string; employee_id: string; category: string; name: string; mime_type: string; size_bytes: number; created_at: string }
export interface Count { label: string; count: number }
export interface HRDashboard {
  headcount: number; on_notice: number; by_department: Count[]; by_type: Count[]; by_location: Count[]; joiners: Employee[]; leavers: Employee[];
  attrition_pct: number; leavers_12m: number; on_leave_today: TeamLeave[]; pending_leave: number; pending_regularisations: number;
  celebrations: Celebration[]; payroll_period: string; payroll_status: string; employees_without_salary: number;
}
export interface ManagerDashboard { team_size: number; team: Employee[]; on_leave_today: TeamLeave[]; pending_leave: number; pending_regularisations: number; present_today: number; not_in_yet: number }
export interface MyHR {
  roles: { hr: boolean; manager: boolean; employee: boolean };
  employee: Employee | null; balances: Balance[]; today: Today; my_leave: LeaveRequest[]; payslips: Payslip[]; documents: Document[];
  manager: Employee | null; team: Employee[]; reports: Employee[]; holidays: Holiday[]; celebrations: Celebration[];
  on_leave_today: TeamLeave[]; pending_approvals: number;
}
export interface OrgNode { id: string; user_id: string; name: string; email: string; designation: string; department: string; department_id: string; manager_id: string; location: string }
export interface ImportResult { created?: number; updated?: number; imported?: number; duplicates?: number; errors: { row: number; error: string }[] }

const BASE = "/api/v1/hr";

async function jf<T>(url: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, {
    credentials: "same-origin",
    headers: { Accept: "application/json", ...(init?.body && !(init.body instanceof FormData) ? { "Content-Type": "application/json" } : {}) },
    ...init,
  });
  if (!resp.ok) {
    const text = (await resp.text().catch(() => "")).trim();
    throw new Error(text || `HTTP ${resp.status}`);
  }
  return (await resp.json()) as T;
}
const body = (method: string, b?: unknown): RequestInit => ({ method, body: b === undefined ? undefined : JSON.stringify(b) });
const qs = (o: Record<string, string | undefined>) => {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(o)) if (v) p.set(k, v);
  const s = p.toString();
  return s ? `?${s}` : "";
};

export const hr = {
  me: () => jf<MyHR>(`${BASE}/me`),
  dashboard: () => jf<HRDashboard>(`${BASE}/dashboard`),
  managerDashboard: () => jf<ManagerDashboard>(`${BASE}/dashboard/manager`),
  settings: () => jf<Settings>(`${BASE}/settings`),
  saveSettings: (s: Settings) => jf<Settings>(`${BASE}/settings`, body("PUT", s)),
  orgChart: async () => (await jf<{ people: OrgNode[] }>(`${BASE}/orgchart`)).people ?? [],
  hrAdmins: async () => (await jf<{ admins: { user_id: string; name: string; email: string }[] }>(`${BASE}/hr-admins`)).admins ?? [],
  setHrAdmin: (userId: string, on: boolean) => jf<unknown>(`${BASE}/hr-admins/${userId}`, body(on ? "PUT" : "DELETE")),
  availableUsers: async () => (await jf<{ users: { id: string; name: string; email: string }[] }>(`${BASE}/available-users`)).users ?? [],
  lookups: async (kind: "departments" | "designations" | "locations") => (await jf<Record<string, Lookup[]>>(`${BASE}/${kind}`))[kind] ?? [],
  saveLookup: (kind: string, id: string, l: Partial<Lookup>) => jf<Lookup>(id ? `${BASE}/${kind}/${id}` : `${BASE}/${kind}`, body(id ? "PUT" : "POST", l)),
  deleteLookup: (kind: string, id: string) => jf<unknown>(`${BASE}/${kind}/${id}`, body("DELETE")),
  employees: async (f: { q?: string; status?: string; department_id?: string; location_id?: string; manager_id?: string } = {}) =>
    (await jf<{ employees: Employee[] }>(`${BASE}/employees${qs(f)}`)).employees ?? [],
  employee: (id: string) => jf<Employee>(`${BASE}/employees/${id}`),
  saveEmployee: (id: string, e: Partial<Employee>) => jf<Employee>(id ? `${BASE}/employees/${id}` : `${BASE}/employees`, body(id ? "PUT" : "POST", e)),
  saveOwnProfile: (e: Partial<Employee>) => jf<Employee>(`${BASE}/employees/me`, body("PUT", e)),
  deleteEmployee: (id: string) => jf<unknown>(`${BASE}/employees/${id}`, body("DELETE")),
  importEmployees: (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return jf<ImportResult>(`${BASE}/employees/import`, { method: "POST", body: fd });
  },
  emergency: async (id: string) => (await jf<{ contacts: { id: string; name: string; relationship: string; phone: string }[] }>(`${BASE}/employees/${id}/emergency`)).contacts ?? [],
  addEmergency: (id: string, c: { name: string; relationship: string; phone: string }) => jf<unknown>(`${BASE}/employees/${id}/emergency`, body("POST", c)),
  deleteEmergency: (id: string, cid: string) => jf<unknown>(`${BASE}/employees/${id}/emergency/${cid}`, body("DELETE")),
  documents: async (id: string) => (await jf<{ documents: Document[] }>(`${BASE}/employees/${id}/documents`)).documents ?? [],
  uploadDocument: (id: string, category: string, file: File) => {
    const fd = new FormData();
    fd.append("category", category);
    fd.append("file", file);
    return jf<Document>(`${BASE}/employees/${id}/documents`, { method: "POST", body: fd });
  },
  deleteDocument: (id: string) => jf<unknown>(`${BASE}/documents/${id}`, body("DELETE")),
  documentUrl: (id: string) => `${BASE}/documents/${id}`,
  salaries: async (id: string) => (await jf<{ salaries: EmployeeSalary[] }>(`${BASE}/employees/${id}/salary`)).salaries ?? [],
  saveSalary: (id: string, s: Partial<EmployeeSalary>) => jf<EmployeeSalary>(`${BASE}/employees/${id}/salary`, body("POST", s)),
  balances: async (id: string) => (await jf<{ balances: Balance[] }>(`${BASE}/employees/${id}/balances`)).balances ?? [],
  adjustBalance: (id: string, type_id: string, days: number, note: string) => jf<unknown>(`${BASE}/employees/${id}/balances/adjust`, body("POST", { type_id, days, note })),
  leaveTypes: async (all = false) => (await jf<{ leave_types: LeaveType[] }>(`${BASE}/leave-types${all ? "?all=1" : ""}`)).leave_types ?? [],
  saveLeaveType: (id: string, t: Partial<LeaveType>) => jf<LeaveType>(id ? `${BASE}/leave-types/${id}` : `${BASE}/leave-types`, body(id ? "PUT" : "POST", t)),
  holidayCalendars: async () => (await jf<{ calendars: HolidayCalendar[] }>(`${BASE}/holiday-calendars`)).calendars ?? [],
  saveCalendar: (id: string, c: Partial<HolidayCalendar>) => jf<HolidayCalendar>(id ? `${BASE}/holiday-calendars/${id}` : `${BASE}/holiday-calendars`, body(id ? "PUT" : "POST", c)),
  deleteCalendar: (id: string) => jf<unknown>(`${BASE}/holiday-calendars/${id}`, body("DELETE")),
  addHoliday: (calId: string, h: Partial<Holiday>) => jf<Holiday>(`${BASE}/holiday-calendars/${calId}/holidays`, body("POST", h)),
  deleteHoliday: (id: string) => jf<unknown>(`${BASE}/holidays/${id}`, body("DELETE")),
  leaveRequests: async (scope: string, status = "") => (await jf<{ requests: LeaveRequest[] }>(`${BASE}/leave/requests${qs({ scope, status })}`)).requests ?? [],
  applyLeave: (i: { employee_id?: string; type_id: string; from_date: string; to_date: string; half_day: string; reason: string }) => jf<LeaveRequest>(`${BASE}/leave/requests`, body("POST", i)),
  decideLeave: (id: string, action: "approve" | "reject" | "cancel", note = "") => jf<LeaveRequest>(`${BASE}/leave/requests/${id}/${action}`, body("POST", { note })),
  teamCalendar: (from: string, to: string) => jf<{ leaves: TeamLeave[]; holidays: Holiday[] }>(`${BASE}/leave/team-calendar${qs({ from, to })}`),
  runAccrual: () => jf<{ employees: number }>(`${BASE}/leave/accrual/run`, body("POST")),
  today: () => jf<Today>(`${BASE}/attendance/today`),
  punch: (pos?: { lat: number; lng: number }) => jf<Today>(`${BASE}/attendance/punch`, body("POST", pos ?? {})),
  timesheet: (employeeId: string, month: string) => jf<Timesheet>(`${BASE}/attendance/timesheet${qs({ employee_id: employeeId, month })}`),
  attendanceReport: async (month: string) => (await jf<{ rows: MonthlyRow[] }>(`${BASE}/attendance/report${qs({ month })}`)).rows ?? [],
  /** One page (100 employees) of the monthly attendance report. */
  attendanceReportPage: async (month: string, page = 1, q = "") => {
    const r = await jf<{ rows: MonthlyRow[]; next_cursor?: string }>(`${BASE}/attendance/report${qs({ month, page: String(page), q })}`);
    return { items: r.rows ?? [], next: r.next_cursor ?? "" };
  },
  attendanceReportUrl: (month: string) => `${BASE}/attendance/report?month=${month}&format=csv`,
  importPunches: (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return jf<ImportResult>(`${BASE}/attendance/import`, { method: "POST", body: fd });
  },
  shifts: async () => (await jf<{ shifts: Shift[] }>(`${BASE}/shifts`)).shifts ?? [],
  saveShift: (id: string, s: Partial<Shift>) => jf<Shift>(id ? `${BASE}/shifts/${id}` : `${BASE}/shifts`, body(id ? "PUT" : "POST", s)),
  deleteShift: (id: string) => jf<unknown>(`${BASE}/shifts/${id}`, body("DELETE")),
  setRoster: (employee_ids: string[], from: string, to: string, shift_id: string) => jf<{ days: number }>(`${BASE}/roster`, body("POST", { employee_ids, from, to, shift_id })),
  regularisations: async (scope: string, status = "") => (await jf<{ regularisations: Regularisation[] }>(`${BASE}/regularisations${qs({ scope, status })}`)).regularisations ?? [],
  requestRegularisation: (day: string, in_time: string, out_time: string, reason: string) => jf<Regularisation>(`${BASE}/regularisations`, body("POST", { day, in_time, out_time, reason })),
  decideRegularisation: (id: string, action: "approve" | "reject" | "cancel", note = "") => jf<unknown>(`${BASE}/regularisations/${id}/${action}`, body("POST", { note })),
  structures: async () => (await jf<{ structures: Structure[] }>(`${BASE}/payroll/structures`)).structures ?? [],
  saveStructure: (id: string, s: Partial<Structure>) => jf<Structure>(id ? `${BASE}/payroll/structures/${id}` : `${BASE}/payroll/structures`, body(id ? "PUT" : "POST", s)),
  deleteStructure: (id: string) => jf<unknown>(`${BASE}/payroll/structures/${id}`, body("DELETE")),
  previewStructure: async (annual_gross: number, lines: SalaryLine[]) => (await jf<{ lines: SalaryLine[] }>(`${BASE}/payroll/structures/preview`, body("POST", { annual_gross, lines }))).lines,
  ptSlabs: async () => (await jf<{ slabs: PTSlab[] }>(`${BASE}/payroll/pt-slabs`)).slabs ?? [],
  savePtSlabs: (slabs: PTSlab[]) => jf<unknown>(`${BASE}/payroll/pt-slabs`, body("PUT", { slabs })),
  runs: async () => (await jf<{ runs: Run[] }>(`${BASE}/payroll/runs`)).runs ?? [],
  createRun: (period: string) => jf<Run>(`${BASE}/payroll/runs`, body("POST", { period })),
  runAction: (id: string, action: "recompute" | "review" | "reopen" | "lock") => jf<Run>(`${BASE}/payroll/runs/${id}/${action}`, body("POST")),
  deleteRun: (id: string) => jf<unknown>(`${BASE}/payroll/runs/${id}`, body("DELETE")),
  /** One page (100) of a run's payslips; `next` is the next page number or "". */
  runPayslipsPage: async (id: string, page = 1, q = "") => {
    const r = await jf<{ payslips: Payslip[]; next_cursor?: string }>(`${BASE}/payroll/runs/${id}/payslips${qs({ page: String(page), q })}`);
    return { items: r.payslips ?? [], next: r.next_cursor ?? "" };
  },
  /** One page (100) of leave requests; `next` is the next page number or "". */
  leaveRequestsPage: async (scope: string, status = "", page = 1) => {
    const r = await jf<{ requests: LeaveRequest[]; next_cursor?: string }>(`${BASE}/leave/requests${qs({ scope, status, page: String(page) })}`);
    return { items: r.requests ?? [], next: r.next_cursor ?? "" };
  },
  runPayslips: async (id: string) => (await jf<{ payslips: Payslip[] }>(`${BASE}/payroll/runs/${id}/payslips`)).payslips ?? [],
  bankAdviceUrl: (id: string) => `${BASE}/payroll/runs/${id}/bank-advice.csv`,
  statutoryUrl: (id: string) => `${BASE}/payroll/runs/${id}/statutory.csv`,
  myPayslips: async () => (await jf<{ payslips: Payslip[] }>(`${BASE}/payroll/payslips/mine`)).payslips ?? [],
  payslipPdfUrl: (id: string) => `${BASE}/payroll/payslips/${id}/pdf`,
  setAdjustments: (id: string, adjustments: Adjustment[]) => jf<Payslip>(`${BASE}/payroll/payslips/${id}/adjustments`, body("PUT", { adjustments })),
  reimbursements: async (id: string) => (await jf<{ reports: { id: string; title: string; amount: number }[] }>(`${BASE}/payroll/payslips/${id}/reimbursements`)).reports ?? [],
};
