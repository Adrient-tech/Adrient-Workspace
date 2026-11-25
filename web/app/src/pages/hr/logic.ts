// Pure helpers for the HR pages (unit-tested in logic.test.ts).
import type { OrgNode } from "./api";

export const ACCENT = "#126C5A";

/** formatINR renders an amount with Indian digit grouping (12,34,567.50). */
export function formatINR(n: number): string {
  const neg = n < 0;
  const [int, dec] = Math.abs(n).toFixed(2).split(".");
  let head = int;
  if (int.length > 3) {
    const last3 = int.slice(-3);
    const rest = int.slice(0, -3).replace(/\B(?=(\d{2})+(?!\d))/g, ",");
    head = `${rest},${last3}`;
  }
  return `${neg ? "-" : ""}₹${head}.${dec}`;
}

export function periodLabel(period: string): string {
  const m = /^(\d{4})-(\d{2})$/.exec(period);
  if (!m) return period;
  return new Date(Date.UTC(+m[1], +m[2] - 1, 1)).toLocaleDateString("en-IN", { month: "long", year: "numeric", timeZone: "UTC" });
}

export function currentPeriod(d = new Date()): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

export function shiftPeriod(period: string, delta: number): string {
  const m = /^(\d{4})-(\d{2})$/.exec(period);
  if (!m) return period;
  const d = new Date(Date.UTC(+m[1], +m[2] - 1 + delta, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

/** Days a leave request will probably consume (weekly offs skipped); the server is authoritative. */
export function previewLeaveDays(from: string, to: string, half: string, weeklyOff: number[], holidays: string[]): number {
  if (!from) return 0;
  const a = new Date(`${from}T00:00:00Z`);
  const b = new Date(`${to || from}T00:00:00Z`);
  if (b < a) return 0;
  let n = 0;
  for (let d = a; d <= b; d = new Date(d.getTime() + 86400000)) {
    if (weeklyOff.includes(d.getUTCDay())) continue;
    if (holidays.includes(d.toISOString().slice(0, 10))) continue;
    n++;
  }
  if (half && from === (to || from)) return n > 0 ? 0.5 : 0;
  return half ? 0 : n;
}

export const STATUS_COLOR: Record<string, "success" | "warning" | "danger" | "neutral" | "primary"> = {
  approved: "success", present: "success", active: "success", locked: "success", reimbursed: "success",
  pending: "warning", half_day: "warning", in_progress: "warning", notice: "warning", reviewed: "primary", draft: "neutral",
  rejected: "danger", absent: "danger", missing_punch: "danger", exited: "neutral", cancelled: "neutral",
  leave: "primary", holiday: "neutral", weekly_off: "neutral", upcoming: "neutral", not_employed: "neutral",
};

export function statusLabel(s: string): string {
  return s.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}

export function initials(name: string): string {
  const p = name.trim().split(/\s+/).filter(Boolean);
  return ((p[0]?.[0] ?? "") + (p.length > 1 ? p[p.length - 1][0] : "")).toUpperCase();
}

export function minutesToHM(m: number): string {
  if (!m) return "0h";
  return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, "0")}m`;
}

export interface Tree { node: OrgNode; reports: Tree[] }

/** buildTree builds the reporting forest. Managers that are missing or loop become roots. */
export function buildTree(nodes: OrgNode[]): Tree[] {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const kids = new Map<string, OrgNode[]>();
  const roots: OrgNode[] = [];
  for (const n of nodes) {
    if (n.manager_id && byId.has(n.manager_id) && n.manager_id !== n.id) kids.set(n.manager_id, [...(kids.get(n.manager_id) ?? []), n]);
    else roots.push(n);
  }
  const seen = new Set<string>();
  const make = (n: OrgNode): Tree => {
    seen.add(n.id);
    return { node: n, reports: (kids.get(n.id) ?? []).filter((c) => !seen.has(c.id)).map(make) };
  };
  const out = roots.map(make);
  for (const n of nodes) if (!seen.has(n.id)) out.push(make(n));
  return out;
}

export const WEEKDAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

export const EMPLOYMENT_TYPES = [
  { v: "full_time", l: "Full time" }, { v: "part_time", l: "Part time" }, { v: "contract", l: "Contract" },
  { v: "intern", l: "Intern" }, { v: "consultant", l: "Consultant" },
];

/** monthDays lists every YYYY-MM-DD of a YYYY-MM period. */
export function monthDays(period: string): string[] {
  const m = /^(\d{4})-(\d{2})$/.exec(period);
  if (!m) return [];
  const out: string[] = [];
  const last = new Date(Date.UTC(+m[1], +m[2], 0)).getUTCDate();
  for (let d = 1; d <= last; d++) out.push(`${period}-${String(d).padStart(2, "0")}`);
  return out;
}
