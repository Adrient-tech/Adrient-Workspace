// Agenda (list) view: events grouped by day for a window of days. Pure, so it is unit tested.
import type { CalendarEvent } from "./types";

export const AGENDA_DAYS = 30;

export interface AgendaDay {
  /** YYYY-MM-DD in local time */
  key: string;
  date: Date;
  events: CalendarEvent[];
}

const keyOf = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

/**
 * Groups events by local day over [from, from + days). An event that spans
 * several days appears on each of them. Days with nothing are left out. Within
 * a day all-day items come first, then by start time, then by title.
 */
export function groupAgenda(events: CalendarEvent[], from: Date, days = AGENDA_DAYS): AgendaDay[] {
  const start = new Date(from.getFullYear(), from.getMonth(), from.getDate());
  const end = new Date(start.getFullYear(), start.getMonth(), start.getDate() + days);
  const map = new Map<string, AgendaDay>();
  for (const e of events) {
    const s = new Date(e.start_at);
    let eEnd = new Date(e.end_at);
    if (isNaN(s.getTime())) continue;
    if (isNaN(eEnd.getTime()) || eEnd < s) eEnd = s;
    // An event ending exactly at local midnight does not touch that day.
    const last = new Date(eEnd.getTime() - (eEnd > s ? 1 : 0));
    let d = new Date(Math.max(new Date(s.getFullYear(), s.getMonth(), s.getDate()).getTime(), start.getTime()));
    for (let i = 0; i < days + 400 && d < end && d <= last; i++) {
      const k = keyOf(d);
      const day = map.get(k) ?? { key: k, date: new Date(d), events: [] };
      day.events.push(e);
      map.set(k, day);
      d = new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1);
    }
  }
  const out = [...map.values()].sort((a, b) => a.key.localeCompare(b.key));
  for (const day of out) {
    day.events.sort(
      (a, b) =>
        Number(!!b.all_day) - Number(!!a.all_day) ||
        new Date(a.start_at).getTime() - new Date(b.start_at).getTime() ||
        a.title.localeCompare(b.title),
    );
  }
  return out;
}

/** "9:30 AM - 10:15 AM", "All day", or the time the event starts for an event that began earlier. */
export function agendaTime(e: CalendarEvent, day: Date): string {
  if (e.all_day) return "All day";
  const s = new Date(e.start_at);
  const f = (d: Date) => d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  const sameDay = s.getFullYear() === day.getFullYear() && s.getMonth() === day.getMonth() && s.getDate() === day.getDate();
  if (!sameDay) return "Continues";
  return `${f(s)} - ${f(new Date(e.end_at))}`;
}
