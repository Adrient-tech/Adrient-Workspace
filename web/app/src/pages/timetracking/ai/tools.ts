/**
 * Time tracking assistant tools. They call the Time tracking API as the signed-in
 * user: members only see their own time and no rates, admins also get amounts and
 * unbilled totals. Logging time and starting a timer need confirmation and are undoable.
 */
import type { AssistantTool } from "@/lib/ai/types";
import { bizTool, capList, clip, int, obj, str, today } from "@/lib/ai/biz";
import { addDays } from "../../ops/dates";
import { tt } from "../api";
import { parseDuration, toHours } from "../logic";

export const TT_SUGGESTIONS = ["How many hours did I log this week?", "What is running on my timer?", "Log 1h30 on the website project for yesterday", "How much unbilled time is there per client?"];
export const TT_HINT = "Dates are YYYY-MM-DD. Hours come from tool results. Rates and amounts are only available to admins; never guess them. Logging time needs the user's confirmation.";
const DATE = /^\d{4}-\d{2}-\d{2}$/;

export function createTimeTools(opts: { admin: boolean }): AssistantTool[] {
  const A = "timetracking";
  const tools: AssistantTool[] = [
    bizTool(A, {
      name: "tt_my_hours",
      description: "The signed-in user's own logged time for a date range (default: this week), by project and day.",
      parameters: obj({ from: str("YYYY-MM-DD"), to: str("YYYY-MM-DD") }),
      run: async (a) => {
        const t = today();
        const to = DATE.test(String(a.to ?? "")) ? String(a.to) : t;
        const from = DATE.test(String(a.from ?? "")) ? String(a.from) : addDays(to, -6);
        const [e, p] = await Promise.all([tt.entries({ from, to, user: "me" }), tt.projects()]);
        const name = (id: string) => p.find((x) => x.id === id)?.name ?? "project";
        const byProject: Record<string, number> = {};
        for (const x of e) byProject[name(x.project_id)] = (byProject[name(x.project_id)] ?? 0) + x.seconds;
        return { result: { from, to, total_hours: toHours(e.reduce((n, x) => n + x.seconds, 0)), by_project: Object.entries(byProject).map(([project, s]) => ({ project, hours: toHours(s) })), entries: capList(e.map((x) => ({ id: x.id, date: x.date, project: name(x.project_id), hours: toHours(x.seconds), task: clip(x.task, 80), invoiced: !!x.invoice_id })), 30) } };
      },
    }),
    bizTool(A, {
      name: "tt_timer_status",
      description: "Whether a timer is running for the user and for how long.",
      run: async () => {
        const t = await tt.timer();
        if (!t) return { result: { running: false } };
        const p = (await tt.projects()).find((x) => x.id === t.project_id);
        return { result: { running: true, project: p?.name, task: clip(t.task, 80), started_at: t.started_at, elapsed_hours: toHours(Math.max(0, (Date.now() - new Date(t.started_at).getTime()) / 1000)) } };
      },
    }),
    bizTool(A, {
      name: "tt_log_time",
      description: "Log time for the user on a project. Duration accepts 1:30, 90m or 1.5h.",
      write: true,
      parameters: obj({ project: str("project name (or part of it)"), date: str("YYYY-MM-DD, default today"), duration: str("e.g. 1:30, 90m, 1.5h"), task: str("short task name"), billable: { type: "boolean", description: "default true" } }, ["project", "duration"]),
      label: (a) => `Log ${String(a.duration)} on ${String(a.project)}${a.date ? ` for ${String(a.date)}` : ""}`,
      run: async (a) => {
        const sec = parseDuration(String(a.duration));
        if (Number.isNaN(sec)) return { error: "That is not a duration. Use 1:30, 90m or 1.5h." };
        const q = String(a.project).toLowerCase();
        const ps = (await tt.projects()).filter((p) => p.name.toLowerCase().includes(q));
        if (ps.length !== 1) return { error: ps.length ? `Several projects match: ${ps.map((p) => p.name).join(", ")}.` : "No project with that name." };
        const date = DATE.test(String(a.date ?? "")) ? String(a.date) : today();
        const r = await tt.saveEntry("", { project_id: ps[0].id, date, seconds: sec, billable: a.billable !== false, task: clip(a.task, 100) });
        return { target: r.entry.id, result: { id: r.entry.id, project: ps[0].name, date, hours: toHours(sec), warnings: r.warnings }, summary: `Logged ${toHours(sec)} h on ${ps[0].name} (${date})`, undo: async () => { await tt.deleteEntry(r.entry.id); } };
      },
    }),
    bizTool(A, {
      name: "tt_start_timer",
      description: "Start the timer on a project (a running timer is saved first).",
      write: true,
      parameters: obj({ project: str("project name"), task: str("task name") }, ["project"]),
      label: (a) => `Start the timer on ${String(a.project)}`,
      run: async (a) => {
        const q = String(a.project).toLowerCase();
        const ps = (await tt.projects()).filter((p) => p.name.toLowerCase().includes(q));
        if (ps.length !== 1) return { error: ps.length ? `Several projects match: ${ps.map((p) => p.name).join(", ")}.` : "No project with that name." };
        await tt.start(ps[0].id, clip(a.task, 100), true);
        return { result: { running: true, project: ps[0].name }, summary: `Timer started on ${ps[0].name}`, undo: async () => { await tt.discard(); } };
      },
    }),
  ];
  if (!opts.admin) return tools;
  return [
    ...tools,
    bizTool(A, {
      name: "tt_unbilled_by_client",
      description: "Admins: unbilled billable time and amount per client.",
      sensitive: true,
      run: async () => {
        const c = await tt.clients();
        return { result: c.filter((x) => x.unbilled_seconds).map((x) => ({ client: x.name, hours: toHours(x.unbilled_seconds ?? 0), amount: x.unbilled_amount })) };
      },
    }),
    bizTool(A, {
      name: "tt_report",
      description: "Admins: hours and amounts for a date range grouped by client, project, user, task or day.",
      sensitive: true,
      parameters: obj({ from: str("YYYY-MM-DD"), to: str("YYYY-MM-DD"), group: str("client | project | user | task | day", ["client", "project", "user", "task", "day"]), limit: int("max rows") }),
      run: async (a) => {
        const to = DATE.test(String(a.to ?? "")) ? String(a.to) : today();
        const from = DATE.test(String(a.from ?? "")) ? String(a.from) : addDays(to, -29);
        const rows = await tt.report({ from, to }, String(a.group ?? "project"));
        return { result: capList(rows.map((r) => ({ label: clip(r.label, 60), hours: toHours(r.seconds), billable_hours: toHours(r.billed_seconds), amount: r.amount })), Math.min(Number(a.limit) || 15, 30)) };
      },
    }),
  ];
}
