/**
 * Shifts assistant tools. They run in the browser with the signed-in user's own
 * session, so the Shifts API decides what is allowed (an employee gets only the
 * published rota; only managers and HR admins can change it). Writes ask for
 * confirmation and are undoable. Notes and shift text are untrusted data.
 */
import type { AssistantTool } from "@/lib/ai/types";
import { bizTool, capList, clip, obj, str, today } from "@/lib/ai/biz";
import { addDays } from "../../ops/dates";
import { shiftsApi } from "../api";
import type { Shift } from "../api";

export const SHIFTS_SUGGESTIONS = [
  "What are my shifts this week?",
  "Which shifts are still open next week?",
  "Where are we understaffed?",
  "Who could take the open shift on Friday?",
];
export const SHIFTS_HINT =
  "Dates are YYYY-MM-DD and times HH:MM (24h). Answer rota questions only from tool results. Assigning or creating shifts needs the user's confirmation; suggestions are advice, a person decides.";

const DATE = /^\d{4}-\d{2}-\d{2}$/;
const brief = (s: Shift, name?: string) => ({ id: s.id, date: s.date, start: s.start, end: s.end, role: clip(s.role, 60), assigned_to: name ?? (s.employee_id ? s.employee_id : "open"), published: s.published });

export function createShiftsTools(): AssistantTool[] {
  const A = "shifts";
  const names = async (from: string, to: string) => new Map((await shiftsApi.staff(from, to)).map((s) => [s.employee_id, s.name]));
  return [
    bizTool(A, {
      name: "shifts_my_schedule",
      description: "The signed-in person's own upcoming shifts (default next 14 days), published ones only.",
      parameters: obj({ from: str("YYYY-MM-DD, default today"), days: { type: "integer", description: "number of days, default 14, max 60" } }),
      run: async (a) => {
        const me = await shiftsApi.me();
        const from = DATE.test(String(a.from ?? "")) ? String(a.from) : today();
        const to = addDays(from, Math.min(Number(a.days) || 14, 60) - 1);
        if (!me.employee_id) return { error: "Your account is not linked to an employee record; ask HR." };
        const sh = (await shiftsApi.shifts(from, to)).filter((s) => s.employee_id === me.employee_id);
        return { result: capList(sh.map((s) => brief(s, "you")), 40) };
      },
    }),
    bizTool(A, {
      name: "shifts_week_overview",
      description: "The rota for a date range (default the next 7 days): shifts per day with who is assigned, the open shifts and the understaffed days.",
      parameters: obj({ from: str("YYYY-MM-DD, default today"), days: { type: "integer", description: "default 7, max 31" } }),
      run: async (a) => {
        const from = DATE.test(String(a.from ?? "")) ? String(a.from) : today();
        const to = addDays(from, Math.min(Number(a.days) || 7, 31) - 1);
        const [sh, gaps, nm] = await Promise.all([shiftsApi.shifts(from, to), shiftsApi.gaps(from, to), names(from, to)]);
        const open = sh.filter((s) => !s.employee_id);
        return { result: { from, to, shifts: capList(sh.map((s) => brief(s, nm.get(s.employee_id))), 60), open_shifts: open.length, understaffed: gaps.slice(0, 20) } };
      },
    }),
    bizTool(A, {
      name: "shifts_open_shifts",
      description: "Shifts nobody is assigned to yet in a date range.",
      parameters: obj({ from: str("YYYY-MM-DD, default today"), days: { type: "integer", description: "default 14" } }),
      run: async (a) => {
        const from = DATE.test(String(a.from ?? "")) ? String(a.from) : today();
        const to = addDays(from, Math.min(Number(a.days) || 14, 60) - 1);
        const sh = (await shiftsApi.shifts(from, to)).filter((s) => !s.employee_id);
        return { result: capList(sh.map((s) => brief(s)), 40) };
      },
    }),
    bizTool(A, {
      name: "shifts_suggest_assignee",
      description: "Managers: ranked people who could take one shift, with the reasons (availability, weekly hours, role match, working-time warnings). People on leave or already busy are excluded.",
      parameters: obj({ shift_id: str("shift id") }, ["shift_id"]),
      run: async (a) => {
        const c = await shiftsApi.suggest(String(a.shift_id));
        return { result: c.map((x) => ({ employee_id: x.employee_id, name: x.name, reasons: x.reasons, warnings: x.warnings })) };
      },
    }),
    bizTool(A, {
      name: "shifts_assign_shift",
      description: "Managers: assign a shift to a person (leave employee_id empty to make it open). Overlaps are refused; other warnings are returned.",
      write: true,
      parameters: obj({ shift_id: str("shift id"), employee_id: str("employee id, empty for open") }, ["shift_id"]),
      label: (a) => `Assign shift ${String(a.shift_id).slice(0, 8)} to ${a.employee_id ? String(a.employee_id).slice(0, 8) : "nobody (open)"}`,
      run: async (a) => {
        const id = String(a.shift_id);
        const cur = await shiftsApi.shift(id);
        const r = await shiftsApi.saveShift(id, { ...cur, employee_id: String(a.employee_id ?? "") });
        return {
          target: id,
          result: { shift: brief(r.shift), warnings: r.warnings },
          summary: `Shift on ${cur.date} ${cur.start}-${cur.end} now ${r.shift.employee_id ? "assigned" : "open"}`,
          undo: async () => { await shiftsApi.saveShift(id, { ...cur }); },
        };
      },
    }),
    bizTool(A, {
      name: "shifts_create_shift",
      description: "Managers: create a draft (unpublished) shift, optionally assigned.",
      write: true,
      parameters: obj({ date: str("YYYY-MM-DD"), start: str("HH:MM"), end: str("HH:MM"), role: str("role or label"), break_minutes: { type: "integer", description: "unpaid break" }, employee_id: str("optional employee id") }, ["date", "start", "end", "role"]),
      label: (a) => `Create shift ${String(a.date)} ${String(a.start)}-${String(a.end)} (${clip(a.role, 40)})`,
      run: async (a) => {
        const r = await shiftsApi.saveShift("", { date: String(a.date), start: String(a.start), end: String(a.end), role: String(a.role), break_minutes: Number(a.break_minutes) || 0, employee_id: String(a.employee_id ?? ""), published: false });
        return { target: r.shift.id, result: { shift: brief(r.shift), warnings: r.warnings }, summary: `Created draft shift ${r.shift.date} ${r.shift.start}-${r.shift.end}`, undo: async () => { await shiftsApi.deleteShift(r.shift.id); } };
      },
    }),
    bizTool(A, {
      name: "shifts_request_swap",
      description: "Ask a colleague to take one of your own shifts. A manager still has to approve it.",
      write: true,
      parameters: obj({ shift_id: str("your shift id"), to_employee_id: str("colleague's employee id"), reason: str("short reason") }, ["shift_id", "to_employee_id"]),
      label: (a) => `Request a swap for shift ${String(a.shift_id).slice(0, 8)}`,
      run: async (a) => {
        const s = await shiftsApi.requestSwap(String(a.shift_id), String(a.to_employee_id), clip(a.reason, 300));
        return { target: s.id, result: { id: s.id, status: s.status, to: s.to_name }, summary: `Swap requested with ${s.to_name}`, undo: async () => { await shiftsApi.resolveSwap(s.id, "cancel"); } };
      },
    }),
  ];
}
