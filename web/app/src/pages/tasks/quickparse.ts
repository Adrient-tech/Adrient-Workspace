// Natural-language quick add: "pay rent friday 9am every month !high #home"
// becomes a task draft. Matched fragments come back as tokens (chips); text
// that is not understood stays in the title. Dismissed token types (`ignore`)
// return their text to the title. English keywords, conservative dates.

export interface QuickToken {
  type: "due" | "time" | "repeat" | "priority" | "list" | "tag";
  key: string;
  label: string;
  text: string;
}

export interface QuickDraft {
  title: string;
  /** YYYY-MM-DD */
  due: string | null;
  /** HH:MM */
  dueTime: string | null;
  /** 0 none, 1 low, 2 medium, 3 high */
  priority: number;
  /** Server repeat rule, see internal/tasks/rrule.go */
  repeat: string | null;
  listId: string | null;
  tags: string[];
  tokens: QuickToken[];
}

const WD: Record<string, number> = {
  sun: 0, sunday: 0, mon: 1, monday: 1, tue: 2, tues: 2, tuesday: 2, wed: 3, weds: 3, wednesday: 3,
  thu: 4, thur: 4, thurs: 4, thursday: 4, fri: 5, friday: 5, sat: 6, saturday: 6,
};
const WD_ALT = Object.keys(WD).sort((a, b) => b.length - a.length).join("|");
const MONTHS: Record<string, number> = {
  jan: 1, january: 1, feb: 2, february: 2, mar: 3, march: 3, apr: 4, april: 4, may: 5, jun: 6, june: 6,
  jul: 7, july: 7, aug: 8, august: 8, sep: 9, sept: 9, september: 9, oct: 10, october: 10,
  nov: 11, november: 11, dec: 12, december: 12,
};
const MONTH_ALT = Object.keys(MONTHS).sort((a, b) => b.length - a.length).join("|");
const PRIORITY: Record<string, number> = { high: 3, medium: 2, med: 2, low: 1, p1: 3, p2: 2, p3: 1, "1": 3, "2": 2, "3": 1 };
const B = String.raw`(?=[\s,.!?;]|$)`;

const pad2 = (n: number) => String(n).padStart(2, "0");
const daysInMonth = (y: number, m: number) => new Date(y, m, 0).getDate();

export function isoOf(d: Date): string {
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}
function parseISO(s: string): Date {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d);
}
export function addDays(iso: string, n: number): string {
  const d = parseISO(iso);
  d.setDate(d.getDate() + n);
  return isoOf(d);
}
const weekdayOf = (iso: string) => parseISO(iso).getDay();
/** The next date strictly after `iso` that falls on weekday `wd`. */
export function nextWeekday(iso: string, wd: number): string {
  const diff = ((wd - weekdayOf(iso) + 6) % 7) + 1;
  return addDays(iso, diff);
}

function monthDayISO(month: number, day: number, year: number | null, today: string): string {
  const y = year || Number(today.slice(0, 4));
  let iso = `${y}-${pad2(month)}-${pad2(Math.min(day, daysInMonth(y, month)))}`;
  if (!year && iso < today) iso = `${y + 1}-${pad2(month)}-${pad2(Math.min(day, daysInMonth(y + 1, month)))}`;
  return iso;
}

/** Build the stored rule: every:<n>:<freq>[:weekdays] */
export function buildRule(freq: "daily" | "weekly" | "monthly" | "yearly", interval = 1, weekdays: number[] = []): string {
  const simple = interval === 1 && weekdays.length === 0;
  if (simple) return freq;
  if (freq === "weekly" && interval === 1 && weekdays.join(",") === "1,2,3,4,5") return "weekdays";
  const wd = freq === "weekly" && weekdays.length ? `:${[...new Set(weekdays)].sort().join(",")}` : "";
  return `every:${interval}:${freq}${wd}`;
}

/** Where a new weekday rule starts: today if it qualifies, else the nearest chosen day. */
export function initialDue(rule: string, today: string): string {
  const m = /^every:\d+:weekly:([\d,]+)/.exec(rule);
  const days = rule === "weekdays" ? [1, 2, 3, 4, 5] : m ? m[1].split(",").map(Number) : [];
  if (!days.length) return today;
  if (days.includes(weekdayOf(today))) return today;
  const dist = (d: number) => ((d - weekdayOf(today) + 7) % 7) || 7;
  return addDays(today, Math.min(...days.map(dist)));
}

export interface QuickOptions {
  lists?: { id: string; name: string }[];
  today?: string;
  ignore?: Set<string>;
  startMonday?: boolean;
}

export function parseQuickAdd(text: string, opts: QuickOptions = {}): QuickDraft {
  const lists = opts.lists ?? [];
  const today = opts.today ?? isoOf(new Date());
  const ignore = opts.ignore ?? new Set<string>();
  const startMonday = opts.startMonday ?? true;
  let work = String(text || "");
  const tokens: QuickToken[] = [];
  const out: QuickDraft = { title: "", due: null, dueTime: null, priority: 0, repeat: null, listId: null, tags: [], tokens };

  const grab = (re: RegExp, type: QuickToken["type"], apply: (m: RegExpExecArray) => string | false): boolean => {
    if (ignore.has(type)) return false;
    const m = re.exec(work);
    if (!m) return false;
    const label = apply(m);
    if (label === false) return false;
    tokens.push({ type, key: type, label, text: m[0].trim() });
    work = work.slice(0, m.index) + " ".repeat(m[0].length) + work.slice(m.index + m[0].length);
    return true;
  };
  const days = (s: string) => s.toLowerCase().split(/\s*,\s*/).map((w) => WD[w]);

  // Recurrence first: "every monday" is a rule, not a date.
  (grab(new RegExp(String.raw`(^|\s)every\s+(weekday|(?:${WD_ALT})(?:\s*,\s*(?:${WD_ALT}))*)${B}`, "i"), "repeat", (m) => {
    const d = m[2].toLowerCase() === "weekday" ? [1, 2, 3, 4, 5] : days(m[2]);
    out.repeat = buildRule("weekly", 1, d);
    return "repeats";
  }) ||
    grab(new RegExp(String.raw`(^|\s)every\s+(?:(\d+)\s+)?(day|week|month|year)s?(?:\s+on\s+((?:${WD_ALT})(?:\s*,\s*(?:${WD_ALT}))*))?${B}`, "i"), "repeat", (m) => {
      const n = Math.min(Number(m[2]) || 1, 999);
      if (n < 1) return false;
      let freq: "daily" | "weekly" | "monthly" | "yearly" = ({ day: "daily", week: "weekly", month: "monthly", year: "yearly" } as const)[m[3].toLowerCase() as "day"];
      let wd: number[] = [];
      if (m[4]) {
        freq = "weekly";
        wd = days(m[4]);
      }
      out.repeat = buildRule(freq, n, wd);
      return "repeats";
    }) ||
    grab(new RegExp(String.raw`(^|\s)(daily|weekly|monthly|yearly|annually)${B}`, "i"), "repeat", (m) => {
      const k = m[2].toLowerCase();
      out.repeat = buildRule(k === "annually" ? "yearly" : (k as "daily"), 1);
      return "repeats";
    }));

  // Dates. Years below 1000 and impossible dates are declined so the text stays.
  (grab(new RegExp(String.raw`(^|\s)(\d{4})-(\d{2})-(\d{2})${B}`), "due", (m) => {
    const y = Number(m[2]), mo = Number(m[3]), d = Number(m[4]);
    if (y < 1000 || mo < 1 || mo > 12 || d < 1 || d > daysInMonth(y, mo)) return false;
    out.due = `${m[2]}-${m[3]}-${m[4]}`;
    return out.due;
  }) ||
    grab(new RegExp(String.raw`(^|\s)(?:on\s+)?(${MONTH_ALT})\s+(\d{1,2})(?:st|nd|rd|th)?(?:,?\s+(\d{4}))?${B}`, "i"), "due", (m) => {
      if (Number(m[3]) < 1 || (m[4] && Number(m[4]) < 1000)) return false;
      out.due = monthDayISO(MONTHS[m[2].toLowerCase()], Number(m[3]), m[4] ? Number(m[4]) : null, today);
      return out.due;
    }) ||
    grab(new RegExp(String.raw`(^|\s)(?:on\s+)?(\d{1,2})(?:st|nd|rd|th)?\s+(${MONTH_ALT})(?:,?\s+(\d{4}))?${B}`, "i"), "due", (m) => {
      if (Number(m[2]) < 1 || (m[4] && Number(m[4]) < 1000)) return false;
      out.due = monthDayISO(MONTHS[m[3].toLowerCase()], Number(m[2]), m[4] ? Number(m[4]) : null, today);
      return out.due;
    }) ||
    grab(new RegExp(String.raw`(^|\s)(today|tod|tonight)${B}`, "i"), "due", () => ((out.due = today), "today")) ||
    grab(new RegExp(String.raw`(^|\s)(tomorrow|tmrw|tmr|tom)${B}`, "i"), "due", () => ((out.due = addDays(today, 1)), "tomorrow")) ||
    grab(new RegExp(String.raw`(^|\s)next\s+week${B}`, "i"), "due", () => ((out.due = nextWeekday(today, startMonday ? 1 : 0)), "next week")) ||
    grab(new RegExp(String.raw`(^|\s)next\s+(${WD_ALT})${B}`, "i"), "due", (m) => (out.due = nextWeekday(today, WD[m[2].toLowerCase()]))) ||
    grab(new RegExp(String.raw`(^|\s)(?:on\s+)?(${WD_ALT})${B}`, "i"), "due", (m) => {
      const wd = WD[m[2].toLowerCase()];
      return (out.due = weekdayOf(today) === wd ? today : nextWeekday(today, wd));
    }));

  // Time.
  (grab(new RegExp(String.raw`(^|\s)(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)${B}`, "i"), "time", (m) => {
    if (Number(m[2]) > 12 || Number(m[2]) < 1 || (m[3] && Number(m[3]) > 59)) return false;
    let h = Number(m[2]) % 12;
    if (m[4].toLowerCase() === "pm") h += 12;
    return (out.dueTime = `${pad2(h)}:${m[3] || "00"}`);
  }) ||
    grab(new RegExp(String.raw`(^|\s)(?:at\s+)?([01]?\d|2[0-3]):([0-5]\d)${B}`), "time", (m) => (out.dueTime = `${pad2(Number(m[2]))}:${m[3]}`)) ||
    grab(new RegExp(String.raw`(^|\s)at\s+([01]?\d|2[0-3])${B}`, "i"), "time", (m) => (out.dueTime = `${pad2(Number(m[2]))}:00`)));

  // Priority.
  grab(new RegExp(String.raw`(^|\s)!(high|medium|med|low|p[123]|[123])${B}`, "i"), "priority", (m) => {
    out.priority = PRIORITY[m[2].toLowerCase()] || 0;
    return ["", "Low", "Medium", "High"][out.priority];
  });

  // #words: a list name wins, anything else is a tag (max 5, extras stay in the title).
  {
    const re = new RegExp(String.raw`(^|\s)#([\p{L}\p{N}_-]+)${B}`, "giu");
    const blanks: [number, number][] = [];
    let m: RegExpExecArray | null;
    while ((m = re.exec(work))) {
      const word = m[2].toLowerCase();
      const list =
        !out.listId && !ignore.has("list")
          ? lists.find((l) => l.name.toLowerCase() === word) ?? lists.find((l) => l.name.toLowerCase().startsWith(word))
          : undefined;
      if (list) {
        out.listId = list.id;
        tokens.push({ type: "list", key: "list", label: list.name, text: m[0].trim() });
        blanks.push([m.index, m[0].length]);
      } else if (!ignore.has("tag") && !ignore.has(`tag:${word}`) && word.length <= 32) {
        if (out.tags.includes(word)) blanks.push([m.index, m[0].length]);
        else if (out.tags.length < 5) {
          out.tags.push(word);
          tokens.push({ type: "tag", key: `tag:${word}`, label: `#${word}`, text: m[0].trim() });
          blanks.push([m.index, m[0].length]);
        }
      }
    }
    for (const [i, len] of blanks) work = work.slice(0, i) + " ".repeat(len) + work.slice(i + len);
  }

  if (out.repeat && !out.due) out.due = initialDue(out.repeat, today);
  if (out.dueTime && !out.due) out.due = today;
  out.title = work.replace(/\s+/g, " ").trim();
  return out;
}

const WD_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

/** Human text for a stored rule ("Every 2 weeks on Mon, Wed"). */
export function describeRule(rule: string): string {
  const simple: Record<string, string> = { daily: "Every day", weekdays: "Every weekday", weekly: "Every week", monthly: "Every month", yearly: "Every year" };
  if (simple[rule]) return simple[rule];
  const m = /^every:(\d+):(daily|weekly|monthly|yearly)(?::([\d,]+))?(:done)?$/.exec(rule) ?? /^every:(\d+):(daily|weekly|monthly|yearly)()(:done)$/.exec(rule);
  if (!m) return "";
  const unit = { daily: "day", weekly: "week", monthly: "month", yearly: "year" }[m[2]] as string;
  let s = Number(m[1]) === 1 ? `Every ${unit}` : `Every ${m[1]} ${unit}s`;
  if (m[3]) s += ` on ${m[3].split(",").map((d) => WD_NAMES[Number(d)]).join(", ")}`;
  if (m[4]) s += " after completion";
  return s;
}
