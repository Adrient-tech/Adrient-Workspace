// iCalendar (.ics) export of open follow-ups and deal close dates. Timed events use floating local time.
import type { Deal, Followup } from "./types";

const CRLF = "\r\n";
// eslint-disable-next-line no-control-regex
const CONTROL_CHARS = /[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g;

export interface IcsEvent {
  uid: string;
  summary: string;
  description?: string;
  date: string;
  time?: string;
  minutes?: number;
  alarm?: boolean;
}

/** RFC 5545 text escaping. Line breaks never survive, so imported notes cannot inject properties. */
export function escapeText(value: unknown): string {
  return String(value ?? "")
    .replace(/\\/g, "\\\\")
    .replace(/;/g, "\\;")
    .replace(/,/g, "\\,")
    .replace(/\r\n|[\r\n]/g, "\\n")
    .replace(CONTROL_CHARS, "");
}

// eslint-disable-next-line no-control-regex
const safeId = (v: unknown) => String(v ?? "").replace(/[\x00-\x1f\x7f]/g, "");

/** Folds a content line at 75 octets without splitting a code point. */
export function foldLine(line: string): string {
  const octets = (cp: number) => (cp < 0x80 ? 1 : cp < 0x800 ? 2 : cp < 0x10000 ? 3 : 4);
  const parts: string[] = [];
  let part = "";
  let size = 0;
  for (const ch of line) {
    const n = octets(ch.codePointAt(0)!);
    if (size + n > 75) {
      parts.push(part);
      part = " ";
      size = 1;
    }
    part += ch;
    size += n;
  }
  parts.push(part);
  return parts.join(CRLF);
}

const compactDate = (iso: string) => iso.replace(/-/g, "");
const compactTime = (hhmm: string) => `${hhmm.replace(":", "")}00`;
const pad = (n: number) => String(n).padStart(2, "0");

function shiftDate(iso: string, days: number): string {
  const [y, m, d] = iso.split("-").map(Number);
  const next = new Date(y, m - 1, d + days);
  return `${next.getFullYear()}-${pad(next.getMonth() + 1)}-${pad(next.getDate())}`;
}

function stamp(now: Date): string {
  return `${now.getUTCFullYear()}${pad(now.getUTCMonth() + 1)}${pad(now.getUTCDate())}T${pad(now.getUTCHours())}${pad(now.getUTCMinutes())}${pad(now.getUTCSeconds())}Z`;
}

export function buildEvent(e: IcsEvent, now = new Date()): string[] {
  const lines = ["BEGIN:VEVENT", `UID:${safeId(e.uid)}`, `DTSTAMP:${stamp(now)}`];
  if (e.time) {
    const [h, m] = e.time.split(":").map(Number);
    const endMin = h * 60 + m + (e.minutes || 30);
    const end = `${pad(Math.floor(endMin / 60) % 24)}:${pad(endMin % 60)}`;
    const endDate = endMin >= 1440 ? shiftDate(e.date, 1) : e.date;
    lines.push(`DTSTART:${compactDate(e.date)}T${compactTime(e.time)}`, `DTEND:${compactDate(endDate)}T${compactTime(end)}`);
  } else {
    lines.push(`DTSTART;VALUE=DATE:${compactDate(e.date)}`, `DTEND;VALUE=DATE:${compactDate(shiftDate(e.date, 1))}`);
  }
  lines.push(`SUMMARY:${escapeText(e.summary)}`);
  if (e.description) lines.push(`DESCRIPTION:${escapeText(e.description)}`);
  if (e.alarm && e.time) lines.push("BEGIN:VALARM", "ACTION:DISPLAY", `DESCRIPTION:${escapeText(e.summary)}`, "TRIGGER:-PT30M", "END:VALARM");
  lines.push("END:VEVENT");
  return lines;
}

export function buildCalendar(events: IcsEvent[], now = new Date()): string {
  const lines = ["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Adrient Workspace//CRM//EN", "CALSCALE:GREGORIAN", "METHOD:PUBLISH", ...events.flatMap((e) => buildEvent(e, now)), "END:VCALENDAR"];
  return `${lines.map(foldLine).join(CRLF)}${CRLF}`;
}

const VALID_DATE = /^\d{4}-\d{2}-\d{2}$/;
const VALID_TIME = /^([01]\d|2[0-3]):[0-5]\d$/;

/** Open follow-ups and open deals with a close date, as calendar events. Rows with a bad date are skipped. */
export function crmEvents(
  followups: Followup[],
  deals: Deal[],
  names: { contact?: (id: string) => string; deal?: (id: string) => string; company?: (id: string) => string } = {},
): IcsEvent[] {
  const out: IcsEvent[] = [];
  for (const f of followups) {
    if (f.done || !VALID_DATE.test(f.due_date)) continue;
    const ctx = [f.contact_id && names.contact?.(f.contact_id), f.deal_id && names.deal?.(f.deal_id)].filter(Boolean).join(" · ");
    out.push({
      uid: `crm-followup-${f.id}@adrient`,
      summary: `Follow up: ${f.title}`,
      description: ctx ? `${ctx}\nFrom Adrient CRM.` : "From Adrient CRM.",
      date: f.due_date,
      time: VALID_TIME.test(f.due_time) ? f.due_time : "",
      minutes: 30,
      alarm: true,
    });
  }
  for (const d of deals) {
    if (d.stage === "won" || d.stage === "lost" || !d.expected_close || !VALID_DATE.test(d.expected_close.slice(0, 10))) continue;
    const co = d.company_id && names.company?.(d.company_id);
    out.push({ uid: `crm-deal-${d.id}@adrient`, summary: `Expected close: ${d.title}`, description: co ? `${co}\nFrom Adrient CRM.` : "From Adrient CRM.", date: d.expected_close.slice(0, 10) });
  }
  return out;
}
