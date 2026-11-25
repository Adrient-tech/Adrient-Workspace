import type { CalendarEvent } from "./types";

/** A yearly date from a contact, as returned by /api/v1/cx/day-events. */
export interface DayEvent {
  contact_id: string;
  name: string;
  kind: "birthday" | "anniversary";
  date: string; // YYYY-MM-DD
  years?: number;
}

const PREFIX = "cx:";

/** Local midnight of a YYYY-MM-DD string (so the day never shifts with the time zone). */
export function localMidnight(date: string, plusDays = 0): Date {
  const [y, m, d] = date.split("-").map(Number);
  return new Date(y, m - 1, d + plusDays, 0, 0, 0, 0);
}

export function isContactDay(e: Pick<CalendarEvent, "id">): boolean {
  return e.id.startsWith(PREFIX);
}

/** Title such as "Asha's birthday (35)". The age is shown only when it is plausible. */
export function dayTitle(d: DayEvent): string {
  const base = d.kind === "birthday" ? `${d.name}'s birthday` : `${d.name}'s anniversary`;
  return d.years && d.years > 0 && d.years < 130 ? `${base} (${d.years})` : base;
}

/** Read-only all-day calendar entries for contact dates. */
export function toCalendarEvents(list: DayEvent[]): CalendarEvent[] {
  return list.map((d) => ({
    id: `${PREFIX}${d.contact_id}:${d.date}:${d.kind}`,
    org_id: "",
    owner_id: "",
    title: dayTitle(d),
    description: "",
    location: "",
    start_at: localMidnight(d.date).toISOString(),
    end_at: localMidnight(d.date, 1).toISOString(),
    all_day: true,
    color: d.kind === "birthday" ? "#f4511e" : "#8e24aa",
    recurrence: "",
    created_at: "",
    updated_at: "",
    item_type: "event",
    status: "free",
  }));
}
