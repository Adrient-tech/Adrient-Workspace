// Pure helpers of Fleet: VIN clean-up and service reminders.
export const ACCENT = "#475569";

/** Upper-cases and strips spaces and dashes from a VIN. */
export const cleanVin = (s: string) => s.replace(/[\s-]/g, "").toUpperCase();

/** 17 characters, no I, O or Q. */
export const vinShapeOk = (vin: string) => /^[A-HJ-NPR-Z0-9]{17}$/.test(vin);

const TRANSLIT: Record<string, number> = { A: 1, B: 2, C: 3, D: 4, E: 5, F: 6, G: 7, H: 8, J: 1, K: 2, L: 3, M: 4, N: 5, P: 7, R: 9, S: 2, T: 3, U: 4, V: 5, W: 6, X: 7, Y: 8, Z: 9 };
const WEIGHTS = [8, 7, 6, 5, 4, 3, 2, 10, 0, 9, 8, 7, 6, 5, 4, 3, 2];
/** North American check digit (position 9). Other markets do not use it, so a mismatch is only a hint. */
export function vinCheckOk(vin: string): boolean {
  if (!vinShapeOk(vin)) return false;
  let sum = 0;
  for (let i = 0; i < 17; i++) sum += (TRANSLIT[vin[i]] ?? Number(vin[i])) * WEIGHTS[i];
  const r = sum % 11;
  return vin[8] === (r === 10 ? "X" : String(r));
}

export const STATUS_LABEL: Record<string, string> = {
  overdue: "Overdue", due_soon: "Due soon", upcoming: "Upcoming", snoozed: "Snoozed", active: "Active", maintenance: "In maintenance", inactive: "Inactive", sold: "Sold",
  open: "Open", resolved: "Resolved", closed: "Closed",
};
export const STATUS_COLOR: Record<string, "danger" | "warning" | "success" | "neutral" | "primary"> = {
  overdue: "danger", due_soon: "warning", upcoming: "neutral", snoozed: "neutral", active: "success", maintenance: "warning", inactive: "neutral", sold: "neutral",
  open: "danger", resolved: "success", closed: "neutral", high: "danger", medium: "warning", low: "neutral",
};

/** "Every 6 months or 10,000 km". */
export function describeInterval(iv: { every: number; unit: string }, meterInterval: number, unit = "km"): string {
  const parts: string[] = [];
  if (iv.every > 0) parts.push(`${iv.every} ${iv.every === 1 ? iv.unit.replace(/s$/, "") : iv.unit}`);
  if (meterInterval > 0) parts.push(`${meterInterval.toLocaleString("en-IN")} ${unit}`);
  return parts.length ? `Every ${parts.join(" or ")}` : "One time";
}

/** What is left until a reminder is due: "12 days" / "-300 km" style text. */
export function dueText(daysLeft?: number, meterLeft?: number, unit = "km"): string {
  const t: string[] = [];
  if (daysLeft !== undefined) t.push(daysLeft < 0 ? `${-daysLeft} days overdue` : daysLeft === 0 ? "due today" : `in ${daysLeft} days`);
  if (meterLeft !== undefined) t.push(meterLeft <= 0 ? `${Math.round(-meterLeft).toLocaleString("en-IN")} ${unit} over` : `${Math.round(meterLeft).toLocaleString("en-IN")} ${unit} left`);
  return t.join(" · ") || "no due date";
}

/** Share of reminder completions that were on time, as a percentage (null when none). */
export function compliancePct(h: { on_time: boolean }[]): number | null {
  return h.length ? Math.round((h.filter((x) => x.on_time).length / h.length) * 100) : null;
}
