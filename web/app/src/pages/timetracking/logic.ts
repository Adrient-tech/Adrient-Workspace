// Pure helpers of Time tracking: the duration parser and formatters.

/** "1:30", "90m", "1.5h", "1h 20m", "45" (minutes above 24), "2" (hours) -> seconds, NaN when not a duration. */
export function parseDuration(input: string): number {
  let s = String(input ?? "").trim().toLowerCase().replace(",", ".");
  if (!s) return NaN;
  if (s.startsWith(".")) s = `0${s}`;
  let m = s.match(/^(\d+):([0-5]?\d)(?::([0-5]?\d))?$/);
  if (m) return +m[1] * 3600 + +m[2] * 60 + +(m[3] || 0);
  m = s.match(/^:([0-5]?\d)$/);
  if (m) return +m[1] * 60;
  if (/^\d+(?:\.\d+)?$/.test(s)) {
    const n = parseFloat(s);
    return Number.isInteger(n) && n > 24 ? n * 60 : Math.round(n * 3600);
  }
  m = s.match(/^(\d+(?:\.\d+)?)\s*h(?:ours?|rs?)?(?:\s*(\d+(?:\.\d+)?)\s*m(?:in(?:ute)?s?)?|\s+(\d+(?:\.\d+)?)|(\d+(?:\.\d+)?))?$/);
  if (m) return Math.round(parseFloat(m[1]) * 3600 + parseFloat(m[2] ?? m[3] ?? m[4] ?? "0") * 60);
  m = s.match(/^(\d+(?:\.\d+)?)\s*m(?:in(?:ute)?s?)?$/);
  if (m) return Math.round(parseFloat(m[1]) * 60);
  m = s.match(/^(\d+)\s*s(?:ec(?:ond)?s?)?$/);
  if (m) return +m[1];
  return NaN;
}

const p2 = (n: number) => String(n).padStart(2, "0");
/** 5400 -> "1:30". */
export function fmtClock(seconds: number): string {
  const t = Math.max(0, Math.round(seconds));
  return `${Math.floor(t / 3600)}:${p2(Math.floor((t % 3600) / 60))}`;
}
/** 5400 -> "1:30:00" for the running timer. */
export function fmtTimer(seconds: number): string {
  const t = Math.max(0, Math.floor(seconds));
  return `${Math.floor(t / 3600)}:${p2(Math.floor((t % 3600) / 60))}:${p2(t % 60)}`;
}
/** 5400 -> "1h 30m". */
export function fmtDuration(seconds: number): string {
  const min = Math.round(Math.max(0, seconds) / 60);
  const h = Math.floor(min / 60);
  const m = min % 60;
  return h && m ? `${h}h ${m}m` : h ? `${h}h` : `${m}m`;
}
export const toHours = (seconds: number) => Math.round((seconds / 3600) * 100) / 100;

/** Rounds up to the increment in minutes (0 = off), like billing rounding. */
export function roundSeconds(seconds: number, minutes: number): number {
  if (!minutes || seconds <= 0) return Math.max(0, seconds);
  const step = minutes * 60;
  return Math.ceil(seconds / step) * step;
}

/** Sums seconds per ISO date. */
export function secondsByDay(entries: { date: string; seconds: number }[]): Map<string, number> {
  const m = new Map<string, number>();
  for (const e of entries) m.set(e.date, (m.get(e.date) ?? 0) + e.seconds);
  return m;
}
