/**
 * HR assistant tools.
 *
 * Privacy rule: an employee's assistant only ever reads that employee's own
 * leave and payslip data. There is no tool that returns another person's pay,
 * bank or ID details, not even for HR admins (HR admins get aggregates:
 * headcount, attrition and payroll totals). The HR API enforces the same
 * scoping again; the server also hides the admin tools from non-admins.
 */
import type { AssistantTool } from "@/lib/ai/types";
import { api, bizTool, capList, clip, int, obj, round2, str, today } from "@/lib/ai/biz";
import { hr } from "../api";

export const HR_EMPLOYEE_SUGGESTIONS = [
  "How many leave days do I have left?",
  "What is the leave policy?",
  "Who in my team is away next week?",
  "Apply for casual leave next Friday",
];
export const HR_ADMIN_SUGGESTIONS = [
  "Give me the headcount by department",
  "What is our attrition over the last 12 months?",
  "Summarise the latest payroll run",
  "Who is on leave this week?",
];
export const HR_HINT =
  "Answer leave and policy questions only from tool results. You can only act for the signed-in person. Never try to look up, estimate or compare anyone's salary or bank details. Dates are YYYY-MM-DD.";

interface PolicyRow { title?: string; summary?: string; mandatory?: boolean; link?: string }

export function createHrTools(opts: { isHR: boolean }): AssistantTool[] {
  const A = "hr";
  const tools: AssistantTool[] = [
    bizTool(A, {
      name: "hr_my_leave_balance",
      description: "The signed-in employee's own leave balances for this leave year (credited, used, pending, available) per leave type.",
      sensitive: true,
      run: async () => {
        const me = await hr.me();
        if (!me.employee) return { error: "Your account is not linked to an employee record; ask HR." };
        return {
          result: {
            employee: me.employee.name, manager: me.manager?.name ?? null, year: me.balances[0]?.year,
            balances: me.balances.filter((b) => b.tracked).map((b) => ({ type: b.name, code: b.code, credited: b.credited, used: b.used, pending: b.pending, available: b.balance, paid: b.paid })),
            pending_approvals_for_you: me.roles.manager || me.roles.hr ? me.pending_approvals : undefined,
          },
        };
      },
    }),
    bizTool(A, {
      name: "hr_my_leave_requests",
      description: "The signed-in employee's own leave requests with status. Optional status filter: pending, approved, rejected, cancelled.",
      parameters: obj({ status: str("pending | approved | rejected | cancelled") }),
      sensitive: true,
      run: async (a) => {
        const rows = await hr.leaveRequests("mine", typeof a.status === "string" ? a.status : "");
        return { result: capList(rows.map((r) => ({ id: r.id, type: r.type_name, from: r.from_date, to: r.to_date, days: r.days, half_day: r.half_day, status: r.status, reason: clip(r.reason, 120), decision_note: clip(r.decision_note, 120) })), 20) };
      },
    }),
    bizTool(A, {
      name: "hr_leave_policy",
      description: "The organisation's leave types (quota, accrual, carry-forward, half days), upcoming holidays and published HR policies. Use it to answer policy questions.",
      run: async () => {
        const [types, me, pol] = await Promise.all([
          hr.leaveTypes(),
          hr.me(),
          api<{ policies?: PolicyRow[] }>("/api/v1/talent/policies").catch(() => ({ policies: [] as PolicyRow[] })),
        ]);
        return {
          result: {
            leave_types: types.filter((t) => t.active).map((t) => ({ name: t.name, code: t.code, paid: t.paid, annual_quota: t.annual_quota, accrual: t.accrual, carry_forward: t.carry_forward, carry_cap: t.carry_cap, half_day_allowed: t.allow_half_day })),
            upcoming_holidays: me.holidays.filter((h) => h.day >= today()).slice(0, 12).map((h) => ({ day: h.day, name: h.name, optional: h.optional })),
            policies: (pol.policies ?? []).slice(0, 15).map((p) => ({ title: clip(p.title, 100), summary: clip(p.summary, 400), mandatory: !!p.mandatory })),
          },
        };
      },
    }),
    bizTool(A, {
      name: "hr_team_availability",
      description: "Who is on leave between two dates, and the holidays in that range, as the signed-in user may see them (their team or, for HR, the organisation).",
      parameters: obj({ from: str("YYYY-MM-DD"), to: str("YYYY-MM-DD") }, ["from", "to"]),
      run: async (a) => {
        const f = String(a.from), t = String(a.to);
        if (!/^\d{4}-\d{2}-\d{2}$/.test(f) || !/^\d{4}-\d{2}-\d{2}$/.test(t) || t < f) return { error: "from and to must be YYYY-MM-DD with to >= from." };
        const r = await hr.teamCalendar(f, t);
        const byPerson = new Map<string, { name: string; periods: string[] }>();
        for (const l of r.leaves.filter((x) => x.status !== "rejected" && x.status !== "cancelled")) {
          const e = byPerson.get(l.employee_id) ?? { name: l.name, periods: [] };
          e.periods.push(`${l.from_date}${l.to_date !== l.from_date ? ` to ${l.to_date}` : ""} ${l.type_name}${l.status === "pending" ? " (pending)" : ""}`);
          byPerson.set(l.employee_id, e);
        }
        return { result: { from: f, to: t, people_away: [...byPerson.values()].slice(0, 40), holidays: r.holidays.map((h) => ({ day: h.day, name: h.name })) } };
      },
    }),
    bizTool(A, {
      name: "hr_submit_leave",
      description: "Apply for leave for the signed-in employee (never for anyone else). Check hr_my_leave_balance first. The user confirms the request before it is sent to their manager.",
      write: true,
      parameters: obj({
        leave_type: str("leave type name or code, e.g. Casual Leave or CL"),
        from_date: str("YYYY-MM-DD"), to_date: str("YYYY-MM-DD"),
        half_day: str("none | first | second", ["none", "first", "second"]),
        reason: str("short reason"),
      }, ["leave_type", "from_date", "to_date"]),
      label: (a) => `Request ${String(a.leave_type)} from ${String(a.from_date)} to ${String(a.to_date)}${a.half_day && a.half_day !== "none" ? ` (${String(a.half_day)} half)` : ""}${a.reason ? `: ${clip(a.reason, 60)}` : ""}`,
      run: async (a) => {
        const f = String(a.from_date), t = String(a.to_date);
        if (!/^\d{4}-\d{2}-\d{2}$/.test(f) || !/^\d{4}-\d{2}-\d{2}$/.test(t) || t < f) return { error: "Dates must be YYYY-MM-DD and to_date must not be before from_date." };
        const types = await hr.leaveTypes();
        const q = String(a.leave_type).trim().toLowerCase();
        const lt = types.find((x) => x.code.toLowerCase() === q || x.name.toLowerCase() === q) ?? types.find((x) => x.name.toLowerCase().includes(q));
        if (!lt) return { error: `Unknown leave type. Available: ${types.map((x) => x.name).join(", ")}.` };
        const req = await hr.applyLeave({ type_id: lt.id, from_date: f, to_date: t, half_day: a.half_day && a.half_day !== "none" ? String(a.half_day) : "", reason: clip(a.reason, 300) });
        return {
          target: req.id,
          result: { id: req.id, status: req.status, days: req.days, type: req.type_name },
          summary: `Requested ${req.days} day(s) of ${req.type_name} (${f} to ${t}), status ${req.status}`,
          undo: async () => { await hr.decideLeave(req.id, "cancel", "cancelled from the assistant"); },
        };
      },
    }),
  ];
  if (!opts.isHR) return tools;
  return [
    ...tools,
    bizTool(A, {
      name: "hr_headcount",
      description: "HR admins only: current headcount, people on notice, and headcount by department, employment type and location, plus joiners in the last 30 days (counts and names).",
      sensitive: true,
      run: async () => {
        const d = await hr.dashboard();
        return { result: { headcount: d.headcount, on_notice: d.on_notice, by_department: d.by_department, by_type: d.by_type, by_location: d.by_location, joiners_30d: d.joiners.map((e) => ({ name: e.name, department: e.department_name, join_date: e.join_date })).slice(0, 20), employees_without_salary: d.employees_without_salary } };
      },
    }),
    bizTool(A, {
      name: "hr_attrition",
      description: "HR admins only: attrition rate, leavers in the last 12 months with exit reason and tenure (no pay data), and leavers by department.",
      parameters: obj({ limit: int("max leavers listed (default 20)") }),
      sensitive: true,
      run: async (a) => {
        const [d, ex] = await Promise.all([hr.dashboard(), hr.employees({ status: "exited" })]);
        const since = new Date(Date.now() - 365 * 86400000).toISOString().slice(0, 10);
        const left = ex.filter((e) => e.exit_date && e.exit_date >= since);
        const byDept: Record<string, number> = {};
        for (const e of left) byDept[e.department_name || "No department"] = (byDept[e.department_name || "No department"] ?? 0) + 1;
        return {
          result: {
            headcount: d.headcount, attrition_pct_12m: round2(d.attrition_pct), leavers_12m: d.leavers_12m,
            by_department: Object.entries(byDept).map(([label, count]) => ({ label, count })).sort((x, y) => y.count - x.count),
            leavers: left.slice(0, Math.min(Number(a.limit) || 20, 40)).map((e) => ({ name: e.name, department: e.department_name, exit_date: e.exit_date, reason: clip(e.exit_reason, 120), tenure_months: Math.round((new Date(e.exit_date).getTime() - new Date(e.join_date).getTime()) / (30.4 * 86400000)) })),
          },
        };
      },
    }),
    bizTool(A, {
      name: "hr_payroll_summary",
      description: "HR admins only: payroll run totals (employees, gross, net pay, PF, ESI, tax) per period for the latest runs. Totals only; individual salaries are not available.",
      parameters: obj({ limit: int("number of runs (default 6)") }),
      sensitive: true,
      run: async (a) => {
        const runs = await hr.runs();
        return { result: runs.slice(0, Math.min(Number(a.limit) || 6, 12)).map((r) => ({ period: r.period, status: r.status, employees: r.employees, gross: r.gross, net_pay: r.net_pay, pf: r.pf, esi: r.esi, professional_tax: r.prof_tax, tds: r.tds, skipped: r.skipped?.length ?? 0 })) };
      },
    }),
  ];
}
