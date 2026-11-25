// Pure CRM helpers (forecast, funnel, going-cold, agenda, CSV).
import type { Activity, Deal, Followup, HistoryRow, Settings, Stage } from "./types";

export const ACCENT = "#3B5BDB";
export const DAY = 86400000;

export function money(n: number, currency = "INR"): string {
  const v = Number.isFinite(n) ? n : 0;
  try {
    return new Intl.NumberFormat(undefined, { style: "currency", currency: currency.toUpperCase(), maximumFractionDigits: 0 }).format(v);
  } catch {
    return `${currency} ${Math.round(v)}`;
  }
}

export const isOpen = (d: Deal) => d.stage !== "won" && d.stage !== "lost";
export const todayStr = (now = new Date()) =>
  `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;

export function addDaysStr(date: string, n: number): string {
  const d = new Date(`${date}T12:00:00`);
  d.setDate(d.getDate() + n);
  return todayStr(d);
}

/** Monthly recurring revenue a deal adds once won (one-time deals add none). */
export function mrr(d: Pick<Deal, "value" | "billing">): number {
  const div = d.billing === "monthly" ? 1 : d.billing === "quarterly" ? 3 : d.billing === "yearly" ? 12 : 0;
  return div ? Math.round((d.value / div) * 100) / 100 : 0;
}

export const BILLING_LABEL: Record<string, string> = {
  one_time: "One-time",
  monthly: "Monthly",
  quarterly: "Quarterly",
  yearly: "Yearly",
};

export function allStages(s: Settings): Stage[] {
  return [...s.stages, { id: "won", name: "Won" }, { id: "lost", name: "Lost" }];
}
export const stageName = (s: Settings, id: string) => allStages(s).find((x) => x.id === id)?.name ?? id;

export function daysInStage(d: Deal, now = Date.now()): number {
  return Math.max(0, Math.floor((now - new Date(d.stage_entered_at).getTime()) / DAY));
}
export const isStalled = (d: Deal, staleDays: number, now = Date.now()) =>
  isOpen(d) && staleDays > 0 && daysInStage(d, now) >= staleDays;

/** Probability weight of an open stage: later stages are likelier (linear). */
export function stageWeight(s: Settings, stage: string): number {
  if (stage === "won") return 1;
  if (stage === "lost") return 0;
  const i = s.stages.findIndex((x) => x.id === stage);
  return i < 0 ? 0 : (i + 1) / (s.stages.length + 1);
}

export interface Forecast {
  openCount: number;
  openValue: number;
  weighted: number;
  openMrr: number;
  wonMrr: number;
  byMonth: { month: string; value: number; weighted: number }[];
  noDate: number;
}

export function forecast(deals: Deal[], s: Settings): Forecast {
  const open = deals.filter(isOpen);
  const byM = new Map<string, { value: number; weighted: number }>();
  let noDate = 0;
  for (const d of open) {
    const w = d.value * stageWeight(s, d.stage);
    if (!d.expected_close) {
      noDate += d.value;
      continue;
    }
    const m = d.expected_close.slice(0, 7);
    const cur = byM.get(m) ?? { value: 0, weighted: 0 };
    cur.value += d.value;
    cur.weighted += w;
    byM.set(m, cur);
  }
  return {
    openCount: open.length,
    openValue: open.reduce((a, d) => a + d.value, 0),
    weighted: open.reduce((a, d) => a + d.value * stageWeight(s, d.stage), 0),
    openMrr: open.reduce((a, d) => a + mrr(d), 0),
    wonMrr: deals.filter((d) => d.stage === "won").reduce((a, d) => a + mrr(d), 0),
    byMonth: [...byM.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([month, v]) => ({ month, ...v })),
    noDate,
  };
}

/** Revenue won in the calendar month of `now` (by closed_at) against the goal. */
export function wonThisMonth(deals: Deal[], now = new Date()): number {
  const key = todayStr(now).slice(0, 7);
  return deals.filter((d) => d.stage === "won" && d.closed_at && d.closed_at.slice(0, 7) === key).reduce((a, d) => a + d.value, 0);
}

export interface FunnelRow {
  stage: Stage;
  reached: number;
  toNext: number | null;
}

/** Where deals leak: how many deals ever reached each open stage (or later) and the step conversion. */
export function funnel(deals: Deal[], history: HistoryRow[], s: Settings): FunnelRow[] {
  const byDeal = new Map<string, Set<string>>();
  for (const h of history) {
    const set = byDeal.get(h.deal_id) ?? new Set();
    set.add(h.stage);
    byDeal.set(h.deal_id, set);
  }
  const idx = new Map(s.stages.map((st, i) => [st.id, i]));
  const reached = s.stages.map(() => 0);
  for (const d of deals) {
    // A won deal has passed every stage; others reached the furthest open stage they visited.
    let max = d.stage === "won" ? s.stages.length : -1;
    if (max < 0) for (const st of byDeal.get(d.id) ?? [d.stage]) max = Math.max(max, idx.get(st) ?? -1);
    for (let i = 0; i < s.stages.length; i++) if (i <= max) reached[i]++;
  }
  return s.stages.map((stage, i) => ({
    stage,
    reached: reached[i],
    toNext: i < s.stages.length - 1 && reached[i] > 0 ? Math.round((reached[i + 1] / reached[i]) * 100) : null,
  }));
}

export function winLoss(deals: Deal[]) {
  const won = deals.filter((d) => d.stage === "won");
  const lost = deals.filter((d) => d.stage === "lost");
  const closed = won.length + lost.length;
  const avg = (l: Deal[]) => (l.length ? l.reduce((a, d) => a + d.value, 0) / l.length : 0);
  const reasons = (l: Deal[]) => {
    const m = new Map<string, number>();
    for (const d of l) if (d.close_reason.trim()) m.set(d.close_reason.trim(), (m.get(d.close_reason.trim()) ?? 0) + 1);
    return [...m.entries()].sort((a, b) => b[1] - a[1]);
  };
  return {
    won: won.length,
    lost: lost.length,
    winRate: closed ? Math.round((won.length / closed) * 100) : 0,
    avgWon: avg(won),
    avgLost: avg(lost),
    wonReasons: reasons(won),
    lostReasons: reasons(lost),
  };
}

/** Win rate by a deal attribute, e.g. lead source. */
export function winRateBy(deals: Deal[], key: (d: Deal) => string): { key: string; won: number; closed: number; rate: number }[] {
  const m = new Map<string, { won: number; closed: number }>();
  for (const d of deals) {
    if (isOpen(d)) continue;
    const k = key(d) || "Unknown";
    const cur = m.get(k) ?? { won: 0, closed: 0 };
    cur.closed++;
    if (d.stage === "won") cur.won++;
    m.set(k, cur);
  }
  return [...m.entries()].map(([k, v]) => ({ key: k, ...v, rate: Math.round((v.won / v.closed) * 100) })).sort((a, b) => b.closed - a.closed);
}

/** Last logged touch (ms) across the given contact ids and optional deal id; 0 if none. */
export function lastTouch(acts: Activity[], contactIds: string[], dealId?: string): number {
  const ids = new Set(contactIds.filter(Boolean));
  let last = 0;
  for (const a of acts) {
    if ((a.contact_id && ids.has(a.contact_id)) || (dealId && a.deal_id === dealId)) last = Math.max(last, new Date(a.occurred_at).getTime());
  }
  return last;
}

export const dealContacts = (d: Deal): string[] => [d.contact_id ?? "", ...d.committee.map((m) => m.contact_id)];

export function isCold(last: number, coldDays: number, now = Date.now()): boolean {
  return coldDays > 0 && (last === 0 || now - last >= coldDays * DAY);
}

export interface Agenda {
  overdue: Followup[];
  today: Followup[];
  upcoming: Followup[];
  done: Followup[];
}
export function groupFollowups(list: Followup[], today = todayStr()): Agenda {
  const open = list.filter((f) => !f.done);
  const key = (f: Followup) => f.due_date.slice(0, 10) + (f.due_time || "");
  const sorted = [...open].sort((a, b) => key(a).localeCompare(key(b)));
  return {
    overdue: sorted.filter((f) => f.due_date.slice(0, 10) < today),
    today: sorted.filter((f) => f.due_date.slice(0, 10) === today),
    upcoming: sorted.filter((f) => f.due_date.slice(0, 10) > today),
    done: list.filter((f) => f.done),
  };
}

/** Seven consecutive dates starting at the Monday of the week containing `date`. */
export function weekDays(date: string): string[] {
  const d = new Date(`${date}T12:00:00`);
  const back = (d.getDay() + 6) % 7;
  const mon = addDaysStr(date, -back);
  return Array.from({ length: 7 }, (_, i) => addDaysStr(mon, i));
}

/** Touches logged in the 7 days ending `now` against the weekly target. */
export function weeklyProgress(acts: Activity[], target: number, now = Date.now()) {
  const n = acts.filter((a) => now - new Date(a.occurred_at).getTime() < 7 * DAY && now >= new Date(a.occurred_at).getTime()).length;
  return { done: n, target, pct: target > 0 ? Math.min(100, Math.round((n / target) * 100)) : 0 };
}

// ---- CSV ----

export function toCSV(rows: (string | number | null | undefined)[][]): string {
  const esc = (v: string | number | null | undefined) => {
    const s = v == null ? "" : String(v);
    return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
  };
  return rows.map((r) => r.map(esc).join(",")).join("\n") + "\n";
}

export function parseCSV(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let q = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (q) {
      if (c === '"' && text[i + 1] === '"') {
        cell += '"';
        i++;
      } else if (c === '"') q = false;
      else cell += c;
    } else if (c === '"') q = true;
    else if (c === ",") {
      row.push(cell);
      cell = "";
    } else if (c === "\n" || c === "\r") {
      if (c === "\r" && text[i + 1] === "\n") i++;
      row.push(cell);
      cell = "";
      if (row.some((x) => x.trim())) rows.push(row);
      row = [];
    } else cell += c;
  }
  row.push(cell);
  if (row.some((x) => x.trim())) rows.push(row);
  return rows;
}

export interface DealCsvRow {
  title: string;
  company: string;
  value: number;
  stage: string;
  expected_close: string;
  source: string;
}

/** Parse deals CSV with headers title, company, value, stage, expected_close, source (any order). */
export function parseDealsCSV(text: string): DealCsvRow[] {
  const rows = parseCSV(text);
  if (rows.length < 2) return [];
  const h = rows[0].map((x) => x.trim().toLowerCase());
  const col = (n: string) => h.indexOf(n);
  const get = (r: string[], n: string) => (col(n) >= 0 ? (r[col(n)] ?? "").trim() : "");
  return rows
    .slice(1)
    .map((r) => ({
      title: get(r, "title") || get(r, "deal"),
      company: get(r, "company"),
      value: Number(get(r, "value").replace(/[^0-9.\-]/g, "")) || 0,
      stage: get(r, "stage").toLowerCase(),
      expected_close: /^\d{4}-\d{2}-\d{2}$/.test(get(r, "expected_close")) ? get(r, "expected_close") : "",
      source: get(r, "source"),
    }))
    .filter((r) => r.title);
}

/** Open deals with no open follow-up and no future close date to act on: nobody has booked the next step. */
export function dealsWithoutNextStep(deals: Deal[], followups: Followup[]): Deal[] {
  const withStep = new Set(followups.filter((f) => !f.done && f.deal_id).map((f) => f.deal_id as string));
  return deals.filter((d) => isOpen(d) && !withStep.has(d.id));
}
