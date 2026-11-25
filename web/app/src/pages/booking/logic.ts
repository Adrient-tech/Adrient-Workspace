export type Window = { start: string; end: string };
export type Availability = Record<string, Window[]>;
export type Question = { id: string; label: string; type: "text" | "longtext"; required: boolean };

export interface BookingPage {
  id: string;
  slug: string;
  title: string;
  description: string;
  location: string;
  duration_min: number;
  buffer_before_min: number;
  buffer_after_min: number;
  min_notice_min: number;
  max_days_ahead: number;
  slot_step_min: number;
  max_per_day: number;
  timezone: string;
  availability: Availability;
  questions: Question[];
  create_meet: boolean;
  active: boolean;
}

/** Weekday order shown in the editor: Monday first. Keys match the backend (0 = Sunday). */
export const WEEKDAYS: { key: string; label: string }[] = [
  { key: "1", label: "Monday" },
  { key: "2", label: "Tuesday" },
  { key: "3", label: "Wednesday" },
  { key: "4", label: "Thursday" },
  { key: "5", label: "Friday" },
  { key: "6", label: "Saturday" },
  { key: "0", label: "Sunday" },
];

export function defaultAvailability(): Availability {
  const a: Availability = {};
  for (const d of ["1", "2", "3", "4", "5"]) a[d] = [{ start: "09:00", end: "17:00" }];
  return a;
}

export function blankPage(tz: string): Omit<BookingPage, "id" | "slug"> {
  return {
    title: "",
    description: "",
    location: "",
    duration_min: 30,
    buffer_before_min: 0,
    buffer_after_min: 0,
    min_notice_min: 240,
    max_days_ahead: 60,
    slot_step_min: 30,
    max_per_day: 0,
    timezone: tz,
    availability: defaultAvailability(),
    questions: [],
    create_meet: true,
    active: true,
  };
}

function hm(s: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(s.trim());
  if (!m) return null;
  const h = Number(m[1]);
  const mi = Number(m[2]);
  if (h > 24 || mi > 59 || (h === 24 && mi !== 0)) return null;
  return h * 60 + mi;
}

/** Returns an error message for a day's windows, or "" when valid. Mirrors the server rules. */
export function validateWindows(ws: Window[]): string {
  const iv: [number, number][] = [];
  for (const w of ws) {
    const s = hm(w.start);
    const e = hm(w.end);
    if (s === null || e === null) return "Use times like 09:00";
    if (e <= s) return "A window must end after it starts";
    iv.push([s, e]);
  }
  iv.sort((a, b) => a[0] - b[0]);
  for (let i = 1; i < iv.length; i++) if (iv[i][0] < iv[i - 1][1]) return "Windows overlap";
  return "";
}

/** Removes days with no windows so the payload stays small. */
export function cleanAvailability(a: Availability): Availability {
  const out: Availability = {};
  for (const [k, v] of Object.entries(a)) if (v.length > 0) out[k] = v;
  return out;
}

export function publicLink(origin: string, slug: string): string {
  return `${origin.replace(/\/$/, "")}/book/${slug}`;
}

/** The local day (YYYY-MM-DD) of an instant in a time zone. */
export function dayKey(iso: string, tz: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", { timeZone: tz, year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(new Date(iso));
  const g = (t: string) => parts.find((p) => p.type === t)?.value ?? "";
  return `${g("year")}-${g("month")}-${g("day")}`;
}

export function timeLabel(iso: string, tz: string): string {
  return new Intl.DateTimeFormat("en-GB", { timeZone: tz, hour: "2-digit", minute: "2-digit", hour12: false }).format(new Date(iso));
}

/** Groups slot instants by local day in `tz`, keeping order. */
export function groupSlotsByDay(slots: string[], tz: string): Map<string, { iso: string; label: string }[]> {
  const out = new Map<string, { iso: string; label: string }[]>();
  for (const iso of [...slots].sort()) {
    const k = dayKey(iso, tz);
    const list = out.get(k) ?? [];
    list.push({ iso, label: timeLabel(iso, tz) });
    out.set(k, list);
  }
  return out;
}

export function formatDuration(min: number): string {
  if (min < 60) return `${min} min`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  return m ? `${h} h ${m} min` : `${h} h`;
}

export function fullWhen(iso: string, tz: string): string {
  return new Intl.DateTimeFormat("en-GB", {
    timeZone: tz,
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZoneName: "short",
  }).format(new Date(iso));
}

export function detectTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

export function isValidEmail(s: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s.trim());
}
