import { search } from "../../../api/client";
import { createEvent, deleteEvent, listEvents, updateEvent } from "../api";
import { findTime } from "../extrasApi";
import type { CalendarAdapter, CalEvent, CalEventInput } from "./tools";
import type { CalendarEvent, EventInput } from "../types";

const APP: Record<string, string> = {
  SEARCH_RESULT_TYPE_DRIVE: "drive", SEARCH_RESULT_TYPE_DOCS: "docs", SEARCH_RESULT_TYPE_SHEETS: "sheets", SEARCH_RESULT_TYPE_SLIDES: "slides",
  SEARCH_RESULT_TYPE_CONTACTS: "contacts", SEARCH_RESULT_TYPE_KEEP: "keep", SEARCH_RESULT_TYPE_CALENDAR: "calendar", SEARCH_RESULT_TYPE_MAIL: "mail",
};

/** The real adapter: the user's own session against /api/v1/calendar. */
export function calendarAdapter(h: { visibleRange: () => { from: Date; to: Date; label: string }; changed: () => void }): CalendarAdapter {
  const after = async <T,>(p: Promise<T>) => {
    const r = await p;
    h.changed();
    return r;
  };
  return {
    listEvents: async (f, t) => (await listEvents(f, t)) as CalEvent[],
    createEvent: (i: CalEventInput) => after(createEvent(i as EventInput)) as Promise<CalendarEvent>,
    updateEvent: (id, i) => after(updateEvent(id, i as EventInput)) as Promise<CalendarEvent>,
    deleteEvent: (id) => after(deleteEvent(id)),
    findTime: async (q) => (await findTime(q)).suggestions,
    search: async (q) => {
      const r = await search(q, 12);
      return r.groups.flatMap((g) => g.results.map((x) => ({ app: APP[x.type] ?? "other", title: x.title, snippet: x.snippet, url: x.url })));
    },
    visibleRange: h.visibleRange,
    now: () => new Date(),
  };
}
