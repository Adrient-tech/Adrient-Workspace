// Pure date and time helpers for the ops apps. Dates are ISO "YYYY-MM-DD", times "HH:MM".

const pad = (n: number) => String(n).padStart(2, "0");
export const isoOf = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
export const parseISO = (s: string) => {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d);
};
export const todayISO = () => isoOf(new Date());
export function addDays(s: string, n: number): string {
  const d = parseISO(s);
  d.setDate(d.getDate() + n);
  return isoOf(d);
}
/** Monday of the week containing s. */
export function startOfWeek(s: string): string {
  const dow = parseISO(s).getDay();
  return addDays(s, dow === 0 ? -6 : 1 - dow);
}
export const weekDates = (weekStart: string) => Array.from({ length: 7 }, (_, i) => addDays(weekStart, i));
export const monthStart = (s: string) => `${s.slice(0, 7)}-01`;
export function monthEnd(s: string): string {
  const d = parseISO(monthStart(s));
  return isoOf(new Date(d.getFullYear(), d.getMonth() + 1, 0));
}
export function addMonths(s: string, n: number): string {
  const d = parseISO(monthStart(s));
  return isoOf(new Date(d.getFullYear(), d.getMonth() + n, 1));
}
export const DAY_SHORT = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
export const dayShort = (s: string) => DAY_SHORT[parseISO(s).getDay()];
export function fmtDay(s: string): string {
  return parseISO(s).toLocaleDateString(undefined, { day: "numeric", month: "short" });
}
export function fmtMonth(s: string): string {
  return parseISO(s).toLocaleDateString(undefined, { month: "long", year: "numeric" });
}

export function toMinutes(hhmm: string): number | null {
  const m = /^([01]\d|2[0-3]):([0-5]\d)$/.exec(hhmm);
  return m ? Number(m[1]) * 60 + Number(m[2]) : null;
}
/** Length of start..end in minutes; the end may be past midnight. null when invalid or equal. */
export function durationMinutes(start: string, end: string): number | null {
  const s = toMinutes(start);
  const e = toMinutes(end);
  if (s === null || e === null || s === e) return null;
  return e > s ? e - s : e + 1440 - s;
}
export const isOvernight = (start: string, end: string) => {
  const s = toMinutes(start);
  const e = toMinutes(end);
  return s !== null && e !== null && e < s;
};
/** Paid minutes: length minus the unpaid break. */
export function netMinutes(start: string, end: string, breakMin = 0): number {
  return Math.max(0, (durationMinutes(start, end) ?? 0) - Math.max(0, breakMin));
}
/** 450 -> "7h 30m", 120 -> "2h", 0 -> "0h". */
export function fmtMinutes(min: number): string {
  const h = Math.floor(min / 60);
  const m = Math.round(min % 60);
  return m ? `${h}h ${m}m` : `${h}h`;
}
/** "9:00 AM – 5:00 PM", with +1 when the shift ends the next day. */
export function fmtRange(start: string, end: string): string {
  const f = (t: string) => {
    const m = toMinutes(t);
    if (m === null) return t;
    const h = Math.floor(m / 60);
    const mm = m % 60;
    const h12 = h % 12 === 0 ? 12 : h % 12;
    return `${h12}${mm ? ":" + pad(mm) : ""} ${h < 12 ? "AM" : "PM"}`;
  };
  return `${f(start)} – ${f(end)}${isOvernight(start, end) ? " +1" : ""}`;
}
export const money = (n: number, currency = "INR") => {
  try {
    return new Intl.NumberFormat("en-IN", { style: "currency", currency, maximumFractionDigits: 2 }).format(n);
  } catch {
    return `${currency} ${n.toFixed(2)}`;
  }
};
