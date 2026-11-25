import { call, qs } from "../ops/http";

export interface Shift { id: string; date: string; start: string; end: string; break_minutes: number; role: string; location: string; notes: string; employee_id: string; published: boolean }
export interface DayWindow { off: boolean; start: string; end: string }
export interface TimeOff { from: string; to: string; note: string; source?: string }
export interface Availability { days: DayWindow[]; exceptions: TimeOff[] }
export interface Staff {
  employee_id: string; name: string; emp_code: string; department: string; role: string; color: string;
  hourly_rate: number | null; target_hours: number | null; max_hours: number | null; availability: Availability; active: boolean;
}
export interface Rules { min_rest_hours: number; max_daily_hours: number; max_consecutive_days: number }
export interface Settings { rules: Rules; overtime_threshold_hours: number; overtime_multiplier: number; weekly_budget: number; currency: string; grace_minutes: number; roles: string[] }
export interface Coverage { id: string; role: string; days: number[]; count: number; start: string; end: string }
export interface Gap { date: string; role: string; required: number; scheduled: number; missing: number }
export interface Candidate { employee_id: string; name: string; status: string; deficit_minutes: number; role_match: boolean; projected_minutes: number; warnings: string[]; reasons: string[] }
export interface FillProposal { shift: Shift; candidates: Candidate[]; proposal: Candidate | null }
export interface Swap {
  id: string; shift_id: string; from_employee_id: string; from_name: string; to_employee_id: string; to_name: string; reason: string;
  status: string; snapshot: { date?: string; start?: string; end?: string; role?: string }; created_at: string; can_resolve: boolean; can_cancel: boolean;
}
export interface Attendance { shift_id: string; status: string; clock_in: string; clock_out: string; note: string }
export interface TimesheetRow {
  shift_id: string; date: string; employee_id: string; employee: string; role: string; start: string; end: string; break_minutes: number; scheduled_hours: number;
  status: string; clock_in: string; clock_out: string; worked_hours: number | null; minutes_late: number; paid_hours: number; hourly_rate: number | null; pay: number | null;
}
export interface TimesheetSummary { employee_id: string; employee: string; shifts: number; scheduled_hours: number; worked_hours: number; paid_hours: number; present: number; late: number; absent: number; not_recorded: number; pay: number | null }
export interface Bucket { key: string; minutes: number; cost: number }
export interface Cost { cost: number; base_cost: number; overtime_cost: number; minutes: number; overtime_minutes: number; rated_count: number; employee_count: number; by_day: Bucket[]; by_role: Bucket[]; by_employee: Bucket[] }
export interface Me { employee_id: string; scheduler: boolean; hr_admin: boolean; manager: boolean }
export interface PayLine { employee_id: string; hours: number; amount: number }
export interface PayResult { run_id: string; period: string; created_run: boolean; applied: { employee_id: string; name: string; hours: number; amount: number }[]; skipped: { employee_id: string; reason: string }[] }

const B = "/api/v1/shifts";
export const shiftsApi = {
  me: () => call<Me>(`${B}/me`),
  settings: () => call<Settings>(`${B}/settings`),
  saveSettings: (s: Settings) => call<Settings>(`${B}/settings`, "PUT", s),
  staff: (from: string, to: string) => call<{ staff: Staff[] }>(`${B}/staff${qs({ from, to })}`).then((r) => r.staff),
  saveStaff: (id: string, s: Partial<Staff>) => call(`${B}/staff/${id}`, "PUT", s),
  shifts: (from: string, to: string) => call<{ shifts: Shift[] }>(`${B}/shifts${qs({ from, to })}`).then((r) => r.shifts),
  shift: (id: string) => call<Shift>(`${B}/shifts/${id}`),
  saveShift: (id: string, s: Partial<Shift>) => call<{ shift: Shift; warnings: string[] }>(id ? `${B}/shifts/${id}` : `${B}/shifts`, id ? "PUT" : "POST", s),
  deleteShift: (id: string) => call(`${B}/shifts/${id}`, "DELETE"),
  suggest: (id: string) => call<{ candidates: Candidate[] }>(`${B}/shifts/${id}/suggest`).then((r) => r.candidates),
  copyWeek: (from: string, to: string) => call<{ copied: number }>(`${B}/copy-week`, "POST", { from, to }),
  publish: (from: string, to: string, published: boolean) => call<{ changed: number }>(`${B}/publish`, "POST", { from, to, published }),
  fillPlan: (from: string, to: string) => call<{ plan: FillProposal[] }>(`${B}/fill${qs({ from, to })}`).then((r) => r.plan),
  fillApply: (assignments: { shift_id: string; employee_id: string }[]) => call<{ assigned: number; problems: string[] }>(`${B}/fill`, "POST", { assignments }),
  coverage: () => call<{ coverage: Coverage[] }>(`${B}/coverage`).then((r) => r.coverage),
  saveCoverage: (id: string, c: Partial<Coverage>) => call<Coverage>(id ? `${B}/coverage/${id}` : `${B}/coverage`, id ? "PUT" : "POST", c),
  deleteCoverage: (id: string) => call(`${B}/coverage/${id}`, "DELETE"),
  gaps: (from: string, to: string) => call<{ gaps: Gap[] }>(`${B}/gaps${qs({ from, to })}`).then((r) => r.gaps),
  swaps: (status = "") => call<{ swaps: Swap[] }>(`${B}/swaps${qs({ status })}`).then((r) => r.swaps),
  requestSwap: (shift_id: string, to_employee_id: string, reason: string) => call<Swap>(`${B}/swaps`, "POST", { shift_id, to_employee_id, reason }),
  swapWarnings: (id: string) => call<{ warnings: string[] }>(`${B}/swaps/${id}/warnings`).then((r) => r.warnings),
  resolveSwap: (id: string, action: "approve" | "decline" | "cancel") => call<Swap>(`${B}/swaps/${id}/${action}`, "POST"),
  attendance: (from: string, to: string) => call<{ attendance: Attendance[] }>(`${B}/attendance${qs({ from, to })}`).then((r) => r.attendance),
  setAttendance: (shiftId: string, a: Partial<Attendance>) => call<Attendance>(`${B}/attendance/${shiftId}`, "PUT", a),
  clock: (shiftId: string, dir: "in" | "out") => call<Attendance>(`${B}/clock/${shiftId}/${dir}`, "POST"),
  timesheet: (from: string, to: string) => call<{ rows: TimesheetRow[]; summary: TimesheetSummary[] }>(`${B}/timesheet${qs({ from, to })}`),
  csvUrl: (from: string, to: string) => `${B}/timesheet.csv${qs({ from, to })}`,
  cost: (from: string, to: string) => call<{ cost: Cost; weekly_budget: number; currency: string }>(`${B}/cost${qs({ from, to })}`),
  payPreview: (period: string) => call<{ lines: PayLine[] }>(`${B}/payroll-preview${qs({ period })}`).then((r) => r.lines),
  payExport: (period: string) => call<PayResult>(`${B}/payroll-export`, "POST", { period }),
};
