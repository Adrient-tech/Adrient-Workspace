import type { Application, Job, Stage } from "./api";

export const ACCENT = "#126C5A";

export const JOB_TYPES: Record<string, string> = {
  full_time: "Full time", part_time: "Part time", contract: "Contract", intern: "Intern", temporary: "Temporary",
};
export const SOURCES = ["career_page", "referral", "job_board", "linkedin", "agency", "walk_in", "campus", "social", "other"];
export const sourceLabel = (s: string) => s.replace(/_/g, " ").replace(/^./, (c) => c.toUpperCase());

/** Group active applications by stage; unknown stages fall into the first column. */
export function groupByStage(apps: Application[], stages: Stage[]): Record<string, Application[]> {
  const out: Record<string, Application[]> = {};
  for (const s of stages) out[s.id] = [];
  for (const a of apps) {
    if (a.status !== "active") continue;
    (out[a.stage] ?? out[stages[0]?.id ?? ""] ?? []).push(a);
  }
  return out;
}

/** Whole days an application has been in its stage. */
export function daysInStage(a: Pick<Application, "stage_entered_at">, now = new Date()): number {
  const t = new Date(a.stage_entered_at).getTime();
  if (Number.isNaN(t)) return 0;
  return Math.max(0, Math.floor((now.getTime() - t) / 86400000));
}

export function salaryRange(j: Pick<Job, "salary_min" | "salary_max" | "currency">): string {
  const f = (n: number) => new Intl.NumberFormat("en-IN", { maximumFractionDigits: 0 }).format(n);
  if (j.salary_min != null && j.salary_max != null) return `${j.currency} ${f(j.salary_min)} to ${f(j.salary_max)}`;
  if (j.salary_min != null) return `from ${j.currency} ${f(j.salary_min)}`;
  if (j.salary_max != null) return `up to ${j.currency} ${f(j.salary_max)}`;
  return "";
}

export function jobStatusLabel(j: Pick<Job, "status" | "approval_status">): string {
  if (j.status === "draft") {
    if (j.approval_status === "pending") return "Awaiting approval";
    if (j.approval_status === "rejected") return "Approval rejected";
    if (j.approval_status === "approved") return "Approved, not published";
    return "Draft";
  }
  return j.status === "open" ? "Open" : "Closed";
}

/** What the current user may do with a job. */
export function jobActions(j: Job, isHR: boolean): { publish: boolean; requestApproval: boolean; approve: boolean; close: boolean; reopen: boolean } {
  return {
    publish: j.status === "draft" && j.approval_status !== "pending" && j.approval_status !== "rejected" && (isHR || j.approval_status === "approved"),
    requestApproval: j.status === "draft" && !isHR && (j.approval_status === "none" || j.approval_status === "rejected"),
    approve: isHR && j.approval_status === "pending",
    close: j.status === "open",
    reopen: j.status === "closed",
  };
}

export const funnelPercent = (count: number, first: number) => (first > 0 ? Math.round((count / first) * 100) : 0);

export function progressOf(start: number, target: number, current: number): number {
  if (target === start) return current >= target ? 100 : 0;
  const p = ((current - start) / (target - start)) * 100;
  return Math.max(0, Math.min(100, Math.round(p * 10) / 10));
}

export function dueLabel(due: string | null | undefined, now = new Date()): { text: string; overdue: boolean } {
  if (!due) return { text: "No date", overdue: false };
  const d = new Date(due.slice(0, 10) + "T00:00:00");
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const diff = Math.round((d.getTime() - today.getTime()) / 86400000);
  if (diff < 0) return { text: `${-diff} day${diff === -1 ? "" : "s"} overdue`, overdue: true };
  if (diff === 0) return { text: "Due today", overdue: false };
  return { text: `Due in ${diff} day${diff === 1 ? "" : "s"}`, overdue: false };
}

export const fmtDate = (s?: string | null) => (s ? new Date(s).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" }) : "");
export const fmtDateTime = (s?: string | null) => (s ? new Date(s).toLocaleString(undefined, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }) : "");

/** Convert a datetime-local value to an ISO string (local time). */
export const localToISO = (v: string) => (v ? new Date(v).toISOString() : "");
