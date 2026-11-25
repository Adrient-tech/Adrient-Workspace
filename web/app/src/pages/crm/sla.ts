import type { Deal, Settings } from "./types";

export type SlaState = "none" | "ok" | "due" | "breached";

/** Lead response SLA: ok once answered in time, breached when the window passed unanswered (or was answered late). */
export function slaState(dueAt: string | null | undefined, answeredAt: string | null | undefined, now = Date.now()): SlaState {
  if (!dueAt) return "none";
  const due = new Date(dueAt).getTime();
  if (answeredAt) return new Date(answeredAt).getTime() <= due ? "ok" : "breached";
  if (now > due) return "breached";
  return due - now < 3600_000 * 4 ? "due" : "none";
}

/** Deal SLA: days in the current stage above the stage limit. Returns the limit when breached. */
export function stageSlaBreach(d: Pick<Deal, "stage" | "stage_entered_at">, s: Settings, now = Date.now()): number {
  const limit = s.sla_stage_days?.[d.stage] ?? 0;
  if (!limit) return 0;
  const days = Math.floor((now - new Date(d.stage_entered_at).getTime()) / 86400000);
  return days > limit ? limit : 0;
}
