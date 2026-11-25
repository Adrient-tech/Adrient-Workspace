// Duration input and hours display. Money here is whole minor units (paise); nothing is a float.

/** "1,234.50" -> 123450 (paise). Returns null for anything that is not a plain amount with at most 2 decimals. */
export function parseMoney(input: string): number | null {
  const s = input.replace(/[₹,\s]/g, "");
  const m = /^(-?)(\d{1,13})(?:\.(\d{1,2}))?$/.exec(s);
  if (!m) return null;
  const v = Number(m[2]) * 100 + Number((m[3] ?? "").padEnd(2, "0") || 0);
  return m[1] ? -v : v;
}

/** 123450 -> "₹1,234.50" (Indian digit grouping). */
export function fmtMoney(minor: number | null | undefined, currency = "INR"): string {
  if (minor === null || minor === undefined) return "";
  const neg = minor < 0;
  const abs = Math.abs(minor);
  const whole = Math.trunc(abs / 100);
  const frac = String(abs % 100).padStart(2, "0");
  let text: string;
  try {
    text = new Intl.NumberFormat("en-IN", { style: "currency", currency, minimumFractionDigits: 0, maximumFractionDigits: 0 }).format(whole);
  } catch {
    text = String(whole);
  }
  // Intl formats the whole part only; append the paise from integers so no rounding creeps in.
  return `${neg ? "-" : ""}${text}.${frac}`;
}

/** What a person typed into a duration field, as whole minutes (null when unreadable). */
export function parseDuration(input: string): number | null {
  const s = input.trim().toLowerCase().replace(",", ".");
  if (!s) return null;
  let m = /^(\d{1,3}):(\d{1,2})(?::(\d{1,2}))?$/.exec(s);
  if (m) {
    if (Number(m[2]) > 59) return null;
    return Number(m[1]) * 60 + Number(m[2]) + (Number(m[3] ?? 0) >= 30 ? 1 : 0);
  }
  m = /^(?:(\d*\.?\d+)\s*h(?:ours?|rs?)?)?\s*(?:(\d+)\s*m(?:in(?:utes?|s)?)?)?$/.exec(s);
  if (m && (m[1] || m[2])) return Math.round(Number(m[1] || 0) * 60) + Number(m[2] || 0);
  m = /^(\d+)\s*h\s*(\d+)$/.exec(s);
  if (m) return Number(m[1]) * 60 + Number(m[2]);
  m = /^(\d*\.?\d+)$/.exec(s);
  if (m) return Math.round(Number(m[1]) * 60);
  return null;
}

/** Round up to the billing increment. */
export function roundUp(minutes: number, increment: number): number {
  if (minutes <= 0) return 0;
  const inc = Math.max(1, increment);
  return Math.ceil(minutes / inc) * inc;
}

/** 72 -> "1h 12m", 120 -> "2h", 45 -> "45m". */
export function fmtHM(minutes: number): string {
  const h = Math.floor(minutes / 60);
  const r = minutes % 60;
  if (!h) return `${r}m`;
  return r ? `${h}h ${r}m` : `${h}h`;
}

/** 72 -> "1.20". */
export const fmtHours = (minutes: number) => (minutes / 60).toFixed(2);

export const STATUS_LABEL: Record<string, string> = { intake: "Intake", open: "Open", pending: "Pending", on_hold: "On hold", closed: "Closed" };
export const STATUS_COLOR: Record<string, "neutral" | "primary" | "success" | "warning" | "danger"> = { intake: "neutral", open: "success", pending: "warning", on_hold: "neutral", closed: "primary" };
export const KIND_LABEL: Record<string, string> = { deadline: "Deadline", court_date: "Court date", filing: "Filing", meeting: "Meeting", limitation: "Limitation date" };
export const URGENCY_COLOR: Record<string, "danger" | "warning" | "neutral" | "success"> = { overdue: "danger", soon: "warning", later: "neutral", done: "success" };

/** "in 3 days", "today", "2 days ago". */
export function relDays(days: number): string {
  if (days === 0) return "today";
  if (days === 1) return "tomorrow";
  if (days === -1) return "yesterday";
  return days > 0 ? `in ${days} days` : `${-days} days ago`;
}

export const BILL_COLOR: Record<string, "neutral" | "success" | "danger"> = { draft: "neutral", issued: "success", void: "danger" };

/** A CSV with a spreadsheet-safe first cell. */
export function csv(rows: string[][]): string {
  const cell = (v: string) => {
    const safe = /^[=+\-@\t\r]/.test(v) ? `'${v}` : v;
    return /[",\n]/.test(safe) ? `"${safe.replace(/"/g, '""')}"` : safe;
  };
  return rows.map((r) => r.map(cell).join(",")).join("\n");
}
