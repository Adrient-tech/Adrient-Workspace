// Public holidays as a computed, read-only overlay. Holidays are never stored:
// they come from a small rule set, so they work for any year, offline, and can
// never leak into an export or fire a reminder. National holidays only: no
// states or provinces. Lunar and lunisolar holidays (Diwali, Eid, Holi) need an
// ephemeris and are not derived, so they are not listed.
import type { CalendarEvent } from "./types";

const MS_PER_DAY = 86_400_000;
/** Whole days since 1970-01-01 for a civil date (m is 1-12). */
const dayNum = (y: number, m: number, d: number) => Date.UTC(y, m - 1, d) / MS_PER_DAY;
function civil(n: number) {
  const dt = new Date(n * MS_PER_DAY);
  return { y: dt.getUTCFullYear(), m: dt.getUTCMonth() + 1, d: dt.getUTCDate() };
}
/** Day of week, 0 = Sunday. The epoch was a Thursday. */
const dowOf = (n: number) => (((n % 7) + 11) % 7);

/** The nth weekday of a month as a day number; nth is 1-5 or -1 for the last. */
function nthWeekday(y: number, m: number, nth: number, dow: number): number {
  if (nth > 0) {
    const first = dayNum(y, m, 1);
    return first + ((dow - dowOf(first) + 7) % 7) + (nth - 1) * 7;
  }
  const last = dayNum(y, m + 1, 1) - 1;
  return last - ((dowOf(last) - dow + 7) % 7);
}

/** Western Easter Sunday (anonymous Gregorian algorithm). */
export function easterSunday(year: number): { month: number; day: number } {
  const a = year % 19, b = Math.floor(year / 100), c = year % 100;
  const d = Math.floor(b / 4), e = b % 4, f = Math.floor((b + 8) / 25);
  const g = Math.floor((b - f + 1) / 3);
  const h = (19 * a + b - d - g + 15) % 30;
  const i = Math.floor(c / 4), k = c % 4;
  const l = (32 + 2 * e + 2 * i - h - k) % 7;
  const m = Math.floor((a + 11 * h + 22 * l) / 451);
  return { month: Math.floor((h + l - 7 * m + 114) / 31), day: ((h + l - 7 * m + 114) % 31) + 1 };
}
const easterDay = (y: number) => {
  const { month, day } = easterSunday(y);
  return dayNum(y, month, day);
};
const vernalEquinox = (y: number) => dayNum(y, 3, Math.floor(20.8431 + 0.242194 * (y - 1980) - Math.floor((y - 1980) / 4)));
const autumnalEquinox = (y: number) => dayNum(y, 9, Math.floor(23.2488 + 0.242194 * (y - 1980) - Math.floor((y - 1980) / 4)));

interface Rule {
  name: string;
  month?: number;
  day?: number;
  nth?: number;
  dow?: number;
  easter?: number;
  on?: (year: number) => number;
  from?: number;
  to?: number;
  observe?: boolean;
}
type Strategy = "us" | "next-weekday" | "sunday";
interface Country {
  id: string;
  name: string;
  observe?: Strategy;
  bridge?: boolean;
  bridgeName?: string;
  rules: Rule[];
}

const one = (name: string, y: number, m: number, d: number): Rule => ({ name, on: () => dayNum(y, m, d), from: y, to: y });

const COUNTRIES: Country[] = [
  {
    id: "us", name: "United States", observe: "us",
    rules: [
      { name: "New Year's Day", month: 1, day: 1, observe: true },
      { name: "Martin Luther King Jr. Day", month: 1, nth: 3, dow: 1, from: 1986 },
      { name: "Presidents' Day", month: 2, nth: 3, dow: 1 },
      { name: "Memorial Day", month: 5, nth: -1, dow: 1 },
      { name: "Juneteenth", month: 6, day: 19, from: 2021, observe: true },
      { name: "Independence Day", month: 7, day: 4, observe: true },
      { name: "Labor Day", month: 9, nth: 1, dow: 1 },
      { name: "Columbus Day", month: 10, nth: 2, dow: 1 },
      { name: "Veterans Day", month: 11, day: 11, observe: true },
      { name: "Thanksgiving Day", month: 11, nth: 4, dow: 4 },
      { name: "Christmas Day", month: 12, day: 25, observe: true },
    ],
  },
  {
    id: "uk", name: "United Kingdom", observe: "next-weekday",
    rules: [
      { name: "New Year's Day", month: 1, day: 1, observe: true },
      { name: "Good Friday", easter: -2 },
      { name: "Easter Monday", easter: 1 },
      { name: "Early May Bank Holiday", month: 5, nth: 1, dow: 1, from: 1978, to: 1994 },
      { name: "Early May Bank Holiday", on: () => dayNum(1995, 5, 8), from: 1995, to: 1995 },
      { name: "Early May Bank Holiday", month: 5, nth: 1, dow: 1, from: 1996, to: 2019 },
      { name: "Early May Bank Holiday", on: () => dayNum(2020, 5, 8), from: 2020, to: 2020 },
      { name: "Early May Bank Holiday", month: 5, nth: 1, dow: 1, from: 2021 },
      { name: "Spring Bank Holiday", month: 5, nth: -1, dow: 1, to: 1976 },
      one("Spring Bank Holiday", 1977, 6, 6),
      one("Silver Jubilee Bank Holiday", 1977, 6, 7),
      { name: "Spring Bank Holiday", month: 5, nth: -1, dow: 1, from: 1978, to: 2001 },
      one("Golden Jubilee Bank Holiday", 2002, 6, 3),
      one("Spring Bank Holiday", 2002, 6, 4),
      { name: "Spring Bank Holiday", month: 5, nth: -1, dow: 1, from: 2003, to: 2011 },
      one("Spring Bank Holiday", 2012, 6, 4),
      one("Diamond Jubilee Bank Holiday", 2012, 6, 5),
      { name: "Spring Bank Holiday", month: 5, nth: -1, dow: 1, from: 2013, to: 2021 },
      one("Spring Bank Holiday", 2022, 6, 2),
      one("Platinum Jubilee Bank Holiday", 2022, 6, 3),
      { name: "Spring Bank Holiday", month: 5, nth: -1, dow: 1, from: 2023 },
      one("Royal Wedding Bank Holiday", 1981, 7, 29),
      one("Millennium Bank Holiday", 1999, 12, 31),
      one("Royal Wedding Bank Holiday", 2011, 4, 29),
      one("Bank Holiday for the State Funeral of Queen Elizabeth II", 2022, 9, 19),
      one("Bank Holiday for the Coronation of King Charles III", 2023, 5, 8),
      { name: "Summer Bank Holiday", month: 8, nth: -1, dow: 1 },
      { name: "Christmas Day", month: 12, day: 25, observe: true },
      { name: "Boxing Day", month: 12, day: 26, observe: true },
    ],
  },
  {
    id: "ca", name: "Canada", observe: "next-weekday",
    rules: [
      { name: "New Year's Day", month: 1, day: 1, observe: true },
      { name: "Good Friday", easter: -2 },
      { name: "Victoria Day", on: (y) => { const n = dayNum(y, 5, 24); return n - ((dowOf(n) - 1 + 7) % 7); } },
      { name: "Canada Day", month: 7, day: 1, observe: true },
      { name: "Labour Day", month: 9, nth: 1, dow: 1 },
      { name: "National Day for Truth and Reconciliation", month: 9, day: 30, from: 2021, observe: true },
      { name: "Thanksgiving Day", month: 10, nth: 2, dow: 1 },
      { name: "Remembrance Day", month: 11, day: 11, observe: true },
      { name: "Christmas Day", month: 12, day: 25, observe: true },
      { name: "Boxing Day", month: 12, day: 26, observe: true },
    ],
  },
  {
    id: "au", name: "Australia", observe: "next-weekday",
    rules: [
      { name: "New Year's Day", month: 1, day: 1, observe: true },
      { name: "Australia Day", month: 1, day: 26, observe: true },
      { name: "Good Friday", easter: -2 },
      { name: "Easter Monday", easter: 1 },
      { name: "Anzac Day", month: 4, day: 25 },
      { name: "Queen's Birthday", month: 6, nth: 2, dow: 1, to: 2022 },
      { name: "King's Birthday", month: 6, nth: 2, dow: 1, from: 2023 },
      { name: "Christmas Day", month: 12, day: 25, observe: true },
      { name: "Boxing Day", month: 12, day: 26, observe: true },
    ],
  },
  {
    id: "de", name: "Germany",
    rules: [
      { name: "New Year's Day", month: 1, day: 1 },
      { name: "Good Friday", easter: -2 },
      { name: "Easter Monday", easter: 1 },
      { name: "Labour Day", month: 5, day: 1 },
      { name: "Ascension Day", easter: 39 },
      { name: "Whit Monday", easter: 50 },
      { name: "German Unity Day", month: 6, day: 17, to: 1990 },
      { name: "German Unity Day", month: 10, day: 3, from: 1990 },
      { name: "Day of Repentance and Prayer", on: (y) => { const n = dayNum(y, 11, 22); return n - ((dowOf(n) - 3 + 7) % 7); }, to: 1994 },
      { name: "Christmas Day", month: 12, day: 25 },
      { name: "Second Day of Christmas", month: 12, day: 26 },
    ],
  },
  {
    id: "fr", name: "France",
    rules: [
      { name: "New Year's Day", month: 1, day: 1 },
      { name: "Easter Monday", easter: 1 },
      { name: "Labour Day", month: 5, day: 1 },
      { name: "Victory in Europe Day", month: 5, day: 8 },
      { name: "Ascension Day", easter: 39 },
      { name: "Whit Monday", easter: 50 },
      { name: "Bastille Day", month: 7, day: 14 },
      { name: "Assumption of Mary", month: 8, day: 15 },
      { name: "All Saints' Day", month: 11, day: 1 },
      { name: "Armistice Day", month: 11, day: 11 },
      { name: "Christmas Day", month: 12, day: 25 },
    ],
  },
  {
    id: "nl", name: "Netherlands",
    rules: [
      { name: "New Year's Day", month: 1, day: 1 },
      { name: "Good Friday", easter: -2 },
      { name: "Easter Sunday", easter: 0 },
      { name: "Easter Monday", easter: 1 },
      { name: "Queen's Day", on: (y) => { const n = dayNum(y, 4, 30); return dowOf(n) === 0 ? n - 1 : n; }, to: 2013 },
      { name: "King's Day", on: (y) => { const n = dayNum(y, 4, 27); return dowOf(n) === 0 ? n - 1 : n; }, from: 2014 },
      { name: "Liberation Day", month: 5, day: 5 },
      { name: "Ascension Day", easter: 39 },
      { name: "Whit Sunday", easter: 49 },
      { name: "Whit Monday", easter: 50 },
      { name: "Christmas Day", month: 12, day: 25 },
      { name: "Second Day of Christmas", month: 12, day: 26 },
    ],
  },
  {
    id: "es", name: "Spain",
    rules: [
      { name: "New Year's Day", month: 1, day: 1 },
      { name: "Epiphany", month: 1, day: 6 },
      { name: "Good Friday", easter: -2 },
      { name: "Labour Day", month: 5, day: 1 },
      { name: "Assumption of Mary", month: 8, day: 15 },
      { name: "National Day", month: 10, day: 12 },
      { name: "All Saints' Day", month: 11, day: 1 },
      { name: "Constitution Day", month: 12, day: 6 },
      { name: "Immaculate Conception", month: 12, day: 8 },
      { name: "Christmas Day", month: 12, day: 25 },
    ],
  },
  {
    id: "in", name: "India",
    rules: [
      { name: "Republic Day", month: 1, day: 26 },
      { name: "Independence Day", month: 8, day: 15 },
      { name: "Gandhi Jayanti", month: 10, day: 2 },
    ],
  },
  {
    id: "jp", name: "Japan", observe: "sunday", bridge: true, bridgeName: "Citizens' Holiday",
    rules: [
      { name: "New Year's Day", month: 1, day: 1, observe: true },
      { name: "Coming of Age Day", month: 1, day: 15, to: 1999, observe: true },
      { name: "Coming of Age Day", month: 1, nth: 2, dow: 1, from: 2000 },
      { name: "National Foundation Day", month: 2, day: 11, observe: true },
      { name: "Emperor's Birthday", month: 12, day: 23, to: 2018, observe: true },
      { name: "Emperor's Birthday", month: 2, day: 23, from: 2020, observe: true },
      { name: "Vernal Equinox Day", on: vernalEquinox, observe: true },
      { name: "Greenery Day", month: 4, day: 29, to: 2006, observe: true },
      { name: "Showa Day", month: 4, day: 29, from: 2007, observe: true },
      one("Emperor's Enthronement Day", 2019, 5, 1),
      { name: "Constitution Memorial Day", month: 5, day: 3, observe: true },
      { name: "Greenery Day", month: 5, day: 4, from: 2007, observe: true },
      { name: "Children's Day", month: 5, day: 5, observe: true },
      { name: "Marine Day", month: 7, day: 20, from: 1996, to: 2002, observe: true },
      { name: "Marine Day", month: 7, nth: 3, dow: 1, from: 2003, to: 2019 },
      one("Marine Day", 2020, 7, 23),
      one("Marine Day", 2021, 7, 22),
      { name: "Marine Day", month: 7, nth: 3, dow: 1, from: 2022 },
      { name: "Mountain Day", month: 8, day: 11, from: 2016, to: 2019, observe: true },
      one("Mountain Day", 2020, 8, 10),
      { name: "Mountain Day", on: () => dayNum(2021, 8, 8), from: 2021, to: 2021, observe: true },
      { name: "Mountain Day", month: 8, day: 11, from: 2022, observe: true },
      { name: "Respect for the Aged Day", month: 9, day: 15, to: 2002, observe: true },
      { name: "Respect for the Aged Day", month: 9, nth: 3, dow: 1, from: 2003 },
      { name: "Autumnal Equinox Day", on: autumnalEquinox, observe: true },
      { name: "Health and Sports Day", month: 10, nth: 2, dow: 1, to: 2019 },
      one("Sports Day", 2020, 7, 24),
      one("Sports Day", 2021, 7, 23),
      { name: "Sports Day", month: 10, nth: 2, dow: 1, from: 2022 },
      one("Enthronement Ceremony Day", 2019, 10, 22),
      { name: "Culture Day", month: 11, day: 3, observe: true },
      { name: "Labour Thanksgiving Day", month: 11, day: 23, observe: true },
    ],
  },
];
const BY_ID = new Map(COUNTRIES.map((c) => [c.id, c]));

export const HOLIDAY_COUNTRIES = COUNTRIES.map(({ id, name }) => ({ id, name }));
export const isSupportedCountry = (id: string) => BY_ID.has(id);

function resolveRule(rule: Rule, year: number): number {
  if (rule.on) return rule.on(year);
  if (rule.easter != null) return easterDay(year) + rule.easter;
  if (rule.nth != null) return nthWeekday(year, rule.month!, rule.nth, rule.dow!);
  return dayNum(year, rule.month!, rule.day!);
}

function observedDay(strategy: Strategy, n: number, taken: Set<number>): number | null {
  const dow = dowOf(n);
  if (strategy === "us") return dow === 6 ? n - 1 : dow === 0 ? n + 1 : null;
  if (strategy === "next-weekday") {
    if (dow !== 0 && dow !== 6) return null;
    let m = n + (dow === 6 ? 2 : 1);
    while (dowOf(m) === 0 || dowOf(m) === 6 || taken.has(m)) m += 1;
    return m;
  }
  if (dow !== 0) return null;
  let m = n + 1;
  while (taken.has(m)) m += 1;
  return m;
}

interface Raw { n: number; name: string; observed: boolean; bridged?: boolean }

function generate(country: Country, year: number): Raw[] {
  const base: { n: number; name: string; observe: boolean }[] = [];
  for (const rule of country.rules) {
    if (rule.from != null && year < rule.from) continue;
    if (rule.to != null && year > rule.to) continue;
    const n = resolveRule(rule, year);
    if (Number.isFinite(n)) base.push({ n, name: rule.name, observe: !!rule.observe });
  }
  base.sort((a, b) => a.n - b.n);
  const taken = new Set<number>();
  const seen = new Set<string>();
  const out: Raw[] = [];
  for (const h of base) {
    const key = `${h.n}|${h.name}`;
    if (seen.has(key)) continue;
    seen.add(key);
    taken.add(h.n);
    out.push({ n: h.n, name: h.name, observed: false });
  }
  if (country.observe) {
    for (const h of base) {
      if (!h.observe) continue;
      const moved = observedDay(country.observe, h.n, taken);
      if (moved == null || moved === h.n || taken.has(moved)) continue;
      taken.add(moved);
      out.push({ n: moved, name: `${h.name} (observed)`, observed: true });
    }
  }
  if (country.bridge) {
    // A working day squeezed between two holidays becomes one (never a Sunday).
    const bridged: number[] = [];
    for (const n of [...taken].sort((a, b) => a - b)) {
      const gap = n + 1;
      if (taken.has(gap) || !taken.has(gap + 1) || dowOf(gap) === 0) continue;
      bridged.push(gap);
    }
    for (const n of bridged) {
      if (taken.has(n)) continue;
      taken.add(n);
      out.push({ n, name: country.bridgeName || "Public Holiday", observed: false, bridged: true });
    }
  }
  return out.sort((a, b) => a.n - b.n || a.name.localeCompare(b.name));
}

export interface Holiday {
  id: string;
  /** YYYY-MM-DD */
  date: string;
  name: string;
  observed: boolean;
}

/** Holidays whose date is in [from, to] (inclusive, YYYY-MM-DD), sorted by date. */
export function holidaysInRange(country: string, from: Date, to: Date): Holiday[] {
  const c = BY_ID.get(country);
  if (!c || to < from) return [];
  const out: Holiday[] = [];
  const lo = dayNum(from.getFullYear(), from.getMonth() + 1, from.getDate());
  const hi = dayNum(to.getFullYear(), to.getMonth() + 1, to.getDate());
  for (let year = from.getFullYear() - 1; year <= to.getFullYear() + 1; year++) {
    const perDay = new Map<string, number>();
    for (const h of generate(c, year)) {
      if (h.n < lo || h.n > hi) continue;
      const { y, m, d } = civil(h.n);
      const date = `${y}-${String(m).padStart(2, "0")}-${String(d).padStart(2, "0")}`;
      const k = perDay.get(date) ?? 0;
      perDay.set(date, k + 1);
      if (out.some((o) => o.date === date && o.name === h.name)) continue; // padded years overlap
      out.push({ id: `hol:${c.id}:${date}${k ? `#${k + 1}` : ""}`, date, name: h.name, observed: h.observed });
    }
  }
  return out.sort((a, b) => a.date.localeCompare(b.date) || a.name.localeCompare(b.name));
}

const ZONE_TO_COUNTRY: Record<string, string> = {
  "Europe/London": "uk", "Europe/Berlin": "de", "Europe/Paris": "fr", "Europe/Amsterdam": "nl", "Europe/Madrid": "es",
  "Asia/Kolkata": "in", "Asia/Calcutta": "in", "Asia/Tokyo": "jp",
  "America/New_York": "us", "America/Chicago": "us", "America/Denver": "us", "America/Phoenix": "us", "America/Los_Angeles": "us",
  "America/Anchorage": "us", "Pacific/Honolulu": "us", "America/Detroit": "us",
  "America/Toronto": "ca", "America/Vancouver": "ca", "America/Edmonton": "ca", "America/Winnipeg": "ca", "America/Halifax": "ca",
};
const REGION_TO_COUNTRY: Record<string, string> = { US: "us", GB: "uk", CA: "ca", AU: "au", DE: "de", FR: "fr", NL: "nl", ES: "es", IN: "in", JP: "jp" };

/** The country for the "Automatic" setting: the time zone first, then the browser language. */
export function detectCountry(zone?: string, languages?: readonly string[]): string {
  let z = zone;
  if (z === undefined) {
    try { z = Intl.DateTimeFormat().resolvedOptions().timeZone || ""; } catch { z = ""; }
  }
  if (z && ZONE_TO_COUNTRY[z]) return ZONE_TO_COUNTRY[z];
  if (z?.startsWith("Australia/")) return "au";
  if (z?.startsWith("US/")) return "us";
  const tags = languages ?? (typeof navigator !== "undefined" ? [...(navigator.languages ?? []), navigator.language] : []);
  for (const tag of tags) {
    let region = "";
    try { region = new Intl.Locale(tag).maximize().region ?? ""; } catch { region = tag.split("-")[1]?.toUpperCase() ?? ""; }
    if (REGION_TO_COUNTRY[region]) return REGION_TO_COUNTRY[region];
  }
  return "";
}

/** "auto" resolves by detection, "" means off, an id is used if supported. */
export function resolveHolidayCountry(setting: string | null | undefined): string {
  if (setting == null || setting === "auto") return detectCountry();
  return isSupportedCountry(setting) ? setting : "";
}

export const HOLIDAY_COUNTRY_KEY = "adrient.calendar.holidayCountry";
export const HOLIDAY_PREFIX = "hol:";
export const isHoliday = (e: Pick<CalendarEvent, "id">) => e.id.startsWith(HOLIDAY_PREFIX);

/** Read-only all-day entries for the calendar views. */
export function holidayEvents(country: string, from: Date, to: Date): CalendarEvent[] {
  return holidaysInRange(country, from, to).map((h) => {
    const [y, m, d] = h.date.split("-").map(Number);
    return {
      id: h.id, org_id: "", owner_id: "", title: h.name, description: "", location: "",
      start_at: new Date(y, m - 1, d).toISOString(), end_at: new Date(y, m - 1, d + 1).toISOString(),
      all_day: true, color: "#8e24aa", recurrence: "", created_at: "", updated_at: "", item_type: "event", status: "free",
    };
  });
}
