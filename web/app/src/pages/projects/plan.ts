export interface IssuePlan {
  start_date: string;
  due_date: string;
  milestone_id: string;
  estimate_hours: number | null;
  logged_hours: number;
  depends_on: string[];
  archived?: boolean;
}
/** How many issues a status holds beyond its soft limit (0 when within it or unlimited). */
export function wipOver(count: number, max: number | undefined): number {
  return !max || count <= max ? 0 : count - max;
}

export interface Milestone {
  id: string;
  project_id: string;
  name: string;
  due_date: string;
  done: boolean;
}

export const emptyPlan = (): IssuePlan => ({ start_date: "", due_date: "", milestone_id: "", estimate_hours: null, logged_hours: 0, depends_on: [] });

const DAY = 86400000;

/** Whole days since the Unix epoch for a YYYY-MM-DD date (UTC, no DST effects). */
export function dayNumber(date: string): number {
  const [y, m, d] = date.split("-").map(Number);
  if (!y || !m || !d) return NaN;
  return Math.floor(Date.UTC(y, m - 1, d) / DAY);
}

export function dateOfDay(n: number): string {
  const d = new Date(n * DAY);
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}-${String(d.getUTCDate()).padStart(2, "0")}`;
}

export interface Bar {
  id: string;
  left: number; // day offset from the chart start
  width: number; // days, at least 1
  scheduled: boolean;
}

export interface Chart {
  startDay: number;
  days: number;
  bars: Record<string, Bar>;
  unscheduled: string[];
  milestones: { id: string; name: string; left: number; done: boolean }[];
  todayLeft: number | null;
}

/**
 * Lays issues out on a day axis. An issue with a start and a due date spans
 * both; with only a due date it is a one-day bar on that day; with only a
 * start it is a one-day bar there; with neither it is unscheduled.
 */
export function layoutChart(ids: string[], plans: Record<string, IssuePlan | undefined>, milestones: Milestone[], today: Date, padDays = 3): Chart {
  const spans: Record<string, [number, number]> = {};
  const unscheduled: string[] = [];
  let min = Infinity;
  let max = -Infinity;
  for (const id of ids) {
    const p = plans[id];
    const s = p?.start_date ? dayNumber(p.start_date) : NaN;
    const e = p?.due_date ? dayNumber(p.due_date) : NaN;
    let a = NaN;
    let b = NaN;
    if (!isNaN(s) && !isNaN(e)) [a, b] = [s, Math.max(s, e)];
    else if (!isNaN(e)) [a, b] = [e, e];
    else if (!isNaN(s)) [a, b] = [s, s];
    if (isNaN(a)) {
      unscheduled.push(id);
      continue;
    }
    spans[id] = [a, b];
    min = Math.min(min, a);
    max = Math.max(max, b);
  }
  const todayN = Math.floor(Date.UTC(today.getFullYear(), today.getMonth(), today.getDate()) / DAY);
  const ms = milestones.filter((m) => m.due_date && !isNaN(dayNumber(m.due_date)));
  for (const m of ms) {
    const n = dayNumber(m.due_date);
    min = Math.min(min, n);
    max = Math.max(max, n);
  }
  if (!isFinite(min)) {
    min = todayN;
    max = todayN + 14;
  }
  min = Math.min(min, todayN);
  max = Math.max(max, todayN);
  const startDay = min - padDays;
  const days = max - min + 1 + padDays * 2;
  const bars: Record<string, Bar> = {};
  for (const [id, [a, b]] of Object.entries(spans)) bars[id] = { id, left: a - startDay, width: b - a + 1, scheduled: true };
  return {
    startDay,
    days,
    bars,
    unscheduled,
    milestones: ms.map((m) => ({ id: m.id, name: m.name, left: dayNumber(m.due_date) - startDay, done: m.done })),
    todayLeft: todayN - startDay,
  };
}

/** Prerequisites that end after the dependent issue starts. */
export interface DepConflict {
  issue: string;
  prerequisite: string;
}
export function depConflicts(plans: Record<string, IssuePlan | undefined>): DepConflict[] {
  const out: DepConflict[] = [];
  for (const [id, p] of Object.entries(plans)) {
    if (!p?.start_date) continue;
    const start = dayNumber(p.start_date);
    for (const d of p.depends_on) {
      const due = plans[d]?.due_date ? dayNumber(plans[d]!.due_date) : NaN;
      if (!isNaN(due) && due >= start) out.push({ issue: id, prerequisite: d });
    }
  }
  return out;
}

/** Would adding these prerequisites to `issue` create a loop? Mirrors the server check. */
export function wouldCycle(plans: Record<string, IssuePlan | undefined>, issue: string, newDeps: string[]): boolean {
  const seen = new Set<string>();
  const visit = (n: string): boolean => {
    if (n === issue) return true;
    if (seen.has(n)) return false;
    seen.add(n);
    return (plans[n]?.depends_on ?? []).some(visit);
  };
  return newDeps.some(visit);
}

/** "12 / 40 h" style summary and whether time is over the estimate. */
export function timeSummary(p?: IssuePlan): { text: string; over: boolean } {
  if (!p || (p.estimate_hours == null && !p.logged_hours)) return { text: "", over: false };
  const logged = Math.round(p.logged_hours * 10) / 10;
  if (p.estimate_hours == null) return { text: `${logged} h logged`, over: false };
  return { text: `${logged} / ${p.estimate_hours} h`, over: p.logged_hours > p.estimate_hours };
}

/** Share of issues done in a milestone, 0..1. */
export function milestoneProgress(milestoneId: string, issues: { id: string; status: string }[], plans: Record<string, IssuePlan | undefined>): { done: number; total: number } {
  let done = 0;
  let total = 0;
  for (const i of issues) {
    if (plans[i.id]?.milestone_id !== milestoneId) continue;
    total++;
    if (i.status === "done" || i.status === "canceled") done++;
  }
  return { done, total };
}

/** Month header cells (label and span in days) for the chart axis. */
export function monthSpans(startDay: number, days: number): { label: string; days: number }[] {
  const out: { label: string; days: number }[] = [];
  for (let i = 0; i < days; i++) {
    const d = new Date((startDay + i) * DAY);
    const label = d.toLocaleString("en", { month: "short", year: "numeric", timeZone: "UTC" });
    const last = out[out.length - 1];
    if (last && last.label === label) last.days++;
    else out.push({ label, days: 1 });
  }
  return out;
}
