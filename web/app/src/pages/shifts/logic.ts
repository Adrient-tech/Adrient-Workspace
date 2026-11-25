// Pure helpers of the Shifts page (ranking and labels), unit-tested in logic.test.ts.
import { fmtMinutes } from "../ops/dates";
import type { Gap, Shift, Staff } from "./api";

export const ACCENT = "#126C5A";

/** Net paid minutes per employee for the shifts given (open shifts excluded). */
export function minutesByEmployee(shifts: Shift[], net: (s: Shift) => number): Map<string, number> {
  const m = new Map<string, number>();
  for (const s of shifts) if (s.employee_id) m.set(s.employee_id, (m.get(s.employee_id) ?? 0) + net(s));
  return m;
}

/** "under" / "over" / "ok" against the optional weekly target and maximum. */
export function hoursStatus(minutes: number, st: Pick<Staff, "target_hours" | "max_hours">): "under" | "over" | "ok" {
  if (st.max_hours != null && minutes > st.max_hours * 60) return "over";
  if (st.target_hours != null && minutes < st.target_hours * 60) return "under";
  return "ok";
}

export function describeGap(g: Gap): string {
  return `${g.date} ${g.role}: ${g.scheduled} of ${g.required} (${g.missing} short)`;
}

export const weekLabel = (minutes: number) => fmtMinutes(minutes);
