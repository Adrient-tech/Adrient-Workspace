import type { Win } from "./extrasApi";

export const DAYS: { key: string; label: string }[] = [
  { key: "1", label: "Monday" },
  { key: "2", label: "Tuesday" },
  { key: "3", label: "Wednesday" },
  { key: "4", label: "Thursday" },
  { key: "5", label: "Friday" },
  { key: "6", label: "Saturday" },
  { key: "0", label: "Sunday" },
];

function mins(s: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(s.trim());
  if (!m) return null;
  const h = Number(m[1]);
  const mi = Number(m[2]);
  return h > 24 || mi > 59 || (h === 24 && mi !== 0) ? null : h * 60 + mi;
}

/** Error text for one day's windows, "" when valid (mirrors the server). */
export function windowsError(ws: Win[]): string {
  const iv: [number, number][] = [];
  for (const w of ws) {
    const a = mins(w.start);
    const b = mins(w.end);
    if (a === null || b === null) return "Use times like 09:00";
    if (b <= a) return "A window must end after it starts";
    iv.push([a, b]);
  }
  iv.sort((x, y) => x[0] - y[0]);
  for (let i = 1; i < iv.length; i++) if (iv[i][0] < iv[i - 1][1]) return "Windows overlap";
  return "";
}

/** Splits free text of emails separated by comma, space or newline. */
export function parseEmails(text: string): string[] {
  const seen = new Set<string>();
  for (const p of text.split(/[\s,;]+/)) {
    const e = p.trim().toLowerCase();
    if (/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(e)) seen.add(e);
  }
  return [...seen];
}

/** One-line summary of a day for the settings list. */
export function summarize(ws: Win[]): string {
  return ws.length === 0 ? "Not working" : ws.map((w) => `${w.start}–${w.end}`).join(", ");
}

/** Absolute subscribe URL for the feed path. */
export function feedUrl(origin: string, path: string): string {
  return origin.replace(/\/$/, "") + path;
}
