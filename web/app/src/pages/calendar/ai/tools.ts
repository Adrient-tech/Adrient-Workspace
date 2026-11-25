/**
 * Assistant tools for Calendar. Event titles, descriptions and attendee
 * names can come from other people, so they are returned as data only.
 * Creating or changing an event always asks the user to confirm in the panel
 * and can be undone. Deleting events is not offered.
 */
import { chainUndo, clip, defineTool, guard, num, str, strList, type AssistantTool } from "@/lib/ai";

export interface CalEvent {
  id: string;
  title: string;
  description: string;
  location: string;
  start_at: string;
  end_at: string;
  all_day: boolean;
  color: string;
  recurrence: string;
  attendees?: string[];
  recurring_event_id?: string;
  item_type?: string;
  reminders?: number[];
  status?: string;
  visibility?: string;
  task_done?: boolean;
}
export type CalEventInput = Omit<CalEvent, "id" | "recurring_event_id"> & { attendees: string[]; item_type: string; reminders: number[]; status: string; visibility: string; task_done: boolean };
export interface RelatedHit { app: string; title: string; snippet: string; url: string }

export interface CalendarAdapter {
  listEvents(fromIso: string, toIso: string): Promise<CalEvent[]>;
  createEvent(input: CalEventInput): Promise<CalEvent>;
  updateEvent(id: string, input: CalEventInput): Promise<CalEvent>;
  deleteEvent(id: string): Promise<void>;
  /** Org free/busy for attendees; optional. */
  findTime?(q: { emails: string[]; from: string; to: string; duration_min: number; limit: number }): Promise<Array<{ start: string; end: string; conflicts: string[] }>>;
  search(query: string): Promise<RelatedHit[]>;
  /** Current day / range the user is looking at. */
  visibleRange(): { from: Date; to: Date; label: string };
  now(): Date;
}

export interface Slot { start: string; end: string }

/** Free slots of at least `durationMin` minutes inside working hours, from the user's own events. */
export function freeSlots(
  events: Pick<CalEvent, "start_at" | "end_at" | "all_day" | "status" | "item_type">[],
  from: Date,
  to: Date,
  durationMin: number,
  opts: { startHour?: number; endHour?: number; weekends?: boolean; limit?: number; notBefore?: Date } = {},
): Slot[] {
  const sh = opts.startHour ?? 9;
  const eh = opts.endHour ?? 18;
  const limit = opts.limit ?? 5;
  const busy = events
    .filter((e) => e.status !== "free" && e.item_type !== "task" && (!e.all_day || e.item_type === "out_of_office"))
    .map((e) => [new Date(e.start_at).getTime(), new Date(e.end_at).getTime()] as const)
    .filter(([a, b]) => !isNaN(a) && !isNaN(b) && b > a)
    .sort((x, y) => x[0] - y[0]);
  const out: Slot[] = [];
  const day = new Date(from);
  day.setHours(0, 0, 0, 0);
  const need = durationMin * 60000;
  for (let guardN = 0; day < to && guardN < 62 && out.length < limit; guardN++, day.setDate(day.getDate() + 1)) {
    const dow = day.getDay();
    if (!opts.weekends && (dow === 0 || dow === 6)) continue;
    const ws = new Date(day);
    ws.setHours(sh, 0, 0, 0);
    const we = new Date(day);
    we.setHours(eh, 0, 0, 0);
    let cur = Math.max(ws.getTime(), from.getTime(), opts.notBefore?.getTime() ?? 0);
    // round up to the next 15 minutes
    cur = Math.ceil(cur / 900000) * 900000;
    const end = Math.min(we.getTime(), to.getTime());
    for (const [a, b] of busy) {
      if (b <= cur) continue;
      if (a >= end) break;
      if (a - cur >= need) {
        out.push({ start: new Date(cur).toISOString(), end: new Date(cur + need).toISOString() });
        if (out.length >= limit) return out;
      }
      cur = Math.max(cur, Math.ceil(b / 900000) * 900000);
    }
    if (end - cur >= need && out.length < limit) out.push({ start: new Date(cur).toISOString(), end: new Date(cur + need).toISOString() });
  }
  return out;
}

function parseDate(v: unknown, name: string): Date | string {
  const s = str(v);
  if (!s) return `${name} is required (ISO 8601, e.g. 2026-11-20T15:00:00+05:30)`;
  const d = new Date(s);
  return isNaN(d.getTime()) ? `${name} is not a valid ISO 8601 date` : d;
}

function toInput(e: CalEvent): CalEventInput {
  return {
    title: e.title,
    description: e.description ?? "",
    location: e.location ?? "",
    start_at: e.start_at,
    end_at: e.end_at,
    all_day: e.all_day,
    color: e.color ?? "",
    recurrence: e.recurrence ?? "",
    attendees: e.attendees ?? [],
    item_type: e.item_type ?? "event",
    reminders: e.reminders ?? [],
    status: e.status ?? "busy",
    visibility: e.visibility ?? "default",
    task_done: e.task_done ?? false,
  };
}

const row = (e: CalEvent) => ({
  id: e.id,
  title: clip(e.title, 120),
  start: e.start_at,
  end: e.end_at,
  all_day: e.all_day,
  location: clip(e.location, 80),
  attendees: (e.attendees ?? []).slice(0, 12),
  recurring: !!e.recurrence || !!e.recurring_event_id,
  type: e.item_type ?? "event",
});

export function createCalendarTools(ad: CalendarAdapter): AssistantTool[] {
  return [
    defineTool({
      name: "calendar_list_events",
      description: "List the user's calendar events between two instants (ISO 8601), sorted by start. Use it to summarise a day or week, check conflicts, or before moving something. Max 60 events. Omit from/to for the range on screen.",
      risk: "read",
      parameters: { type: "object", properties: { from: { type: "string" }, to: { type: "string" }, include_descriptions: { type: "boolean" } } },
      handler: (a) =>
        guard(async () => {
          const vr = ad.visibleRange();
          const f = str(a.from) ? parseDate(a.from, "from") : vr.from;
          const t = str(a.to) ? parseDate(a.to, "to") : vr.to;
          if (typeof f === "string") return { error: f };
          if (typeof t === "string") return { error: t };
          if (t.getTime() - f.getTime() > 93 * 86400000) return { error: "range is longer than 93 days" };
          const evs = (await ad.listEvents(f.toISOString(), t.toISOString())).sort((x, y) => x.start_at.localeCompare(y.start_at));
          return {
            result: {
              from: f.toISOString(),
              to: t.toISOString(),
              count: evs.length,
              events: evs.slice(0, 60).map((e) => (a.include_descriptions ? { ...row(e), description: clip(e.description, 400) } : row(e))),
            },
          };
        }),
    }),
    defineTool({
      name: "calendar_find_free_slot",
      description: "Find up to 5 free slots of a given length in working hours (09:00-18:00 by default, weekdays). Pass attendee emails to check their free/busy too when the organisation allows it.",
      risk: "read",
      parameters: {
        type: "object",
        properties: {
          duration_min: { type: "integer", description: "Length in minutes, default 30" },
          from: { type: "string", description: "Earliest instant, default now" },
          to: { type: "string", description: "Latest instant, default 7 days after from" },
          attendees: { type: "array", items: { type: "string" }, description: "Emails of other people" },
          start_hour: { type: "integer" },
          end_hour: { type: "integer" },
        },
      },
      handler: (a) =>
        guard(async () => {
          const dur = num(a.duration_min, 30, 5, 480);
          const now = ad.now();
          const f = str(a.from) ? parseDate(a.from, "from") : now;
          if (typeof f === "string") return { error: f };
          const t = str(a.to) ? parseDate(a.to, "to") : new Date(f.getTime() + 7 * 86400000);
          if (typeof t === "string") return { error: t };
          if (t <= f) return { error: "to must be after from" };
          const emails = strList(a.attendees, 20);
          if (emails.length > 0 && ad.findTime) {
            try {
              const s = await ad.findTime({ emails, from: f.toISOString(), to: t.toISOString(), duration_min: dur, limit: 5 });
              return { result: { slots: s.map((x) => ({ start: x.start, end: x.end, conflicts: x.conflicts })), checked_attendees: emails } };
            } catch {
              /* fall back to the user's own calendar */
            }
          }
          const evs = await ad.listEvents(f.toISOString(), t.toISOString());
          const slots = freeSlots(evs, f, t, dur, { startHour: num(a.start_hour, 9, 0, 23), endHour: num(a.end_hour, 18, 1, 24), notBefore: now });
          return { result: { slots, checked_attendees: [], note: emails.length ? "Attendee free/busy was not available; only your own calendar was checked." : undefined } };
        }),
    }),
    defineTool({
      name: "calendar_create_event",
      description: "Create a calendar event. The user confirms first. Use calendar_find_free_slot or calendar_list_events to avoid conflicts. Times are ISO 8601 with offset.",
      risk: "destructive",
      parameters: {
        type: "object",
        properties: {
          title: { type: "string" },
          start: { type: "string" },
          end: { type: "string" },
          all_day: { type: "boolean" },
          description: { type: "string" },
          location: { type: "string" },
          attendees: { type: "array", items: { type: "string" }, description: "Attendee email addresses" },
        },
        required: ["title", "start", "end"],
      },
      label: (a) => `Create event "${clip(str(a.title), 60)}" at ${str(a.start)}`,
      handler: (a) =>
        guard(async () => {
          const title = str(a.title);
          if (!title) return { error: "title is required" };
          const s = parseDate(a.start, "start");
          const e = parseDate(a.end, "end");
          if (typeof s === "string") return { error: s };
          if (typeof e === "string") return { error: e };
          if (e <= s) return { error: "end must be after start" };
          const ev = await ad.createEvent({
            title: clip(title, 200), description: clip(str(a.description), 4000), location: clip(str(a.location), 300),
            start_at: s.toISOString(), end_at: e.toISOString(), all_day: a.all_day === true, color: "", recurrence: "",
            attendees: strList(a.attendees, 50), item_type: "event", reminders: [], status: "busy", visibility: "default", task_done: false,
          });
          return { result: { id: ev.id, title: ev.title, start: ev.start_at, end: ev.end_at }, summary: `Created event "${clip(ev.title, 60)}"`, undo: () => ad.deleteEvent(ev.id) };
        }),
    }),
    defineTool({
      name: "calendar_update_event",
      description: "Move or edit an existing single (non-recurring) event: change start/end, title, description (for example a drafted agenda), location or attendees. The user confirms first. Recurring events must be edited in the calendar itself.",
      risk: "destructive",
      parameters: {
        type: "object",
        properties: {
          event_id: { type: "string" },
          lookup_from: { type: "string", description: "ISO start of a window that contains the event" },
          lookup_to: { type: "string", description: "ISO end of that window" },
          title: { type: "string" }, start: { type: "string" }, end: { type: "string" },
          description: { type: "string" }, location: { type: "string" },
          attendees: { type: "array", items: { type: "string" } },
        },
        required: ["event_id", "lookup_from", "lookup_to"],
      },
      label: (a) => `Change event ${str(a.event_id).slice(0, 8)}${a.start ? ` → ${str(a.start)}` : ""}${a.title ? ` "${clip(str(a.title), 40)}"` : ""}${a.description ? " (new description)" : ""}`,
      handler: (a) =>
        guard(async () => {
          const f = parseDate(a.lookup_from, "lookup_from");
          const t = parseDate(a.lookup_to, "lookup_to");
          if (typeof f === "string") return { error: f };
          if (typeof t === "string") return { error: t };
          const cur = (await ad.listEvents(f.toISOString(), t.toISOString())).find((e) => e.id === str(a.event_id));
          if (!cur) return { error: "event not found in that window; use calendar_list_events first" };
          if (cur.recurrence || cur.recurring_event_id) return { error: "this is a recurring event; it cannot be changed by the assistant" };
          const next = toInput(cur);
          if (a.start !== undefined || a.end !== undefined) {
            const s = a.start !== undefined ? parseDate(a.start, "start") : new Date(cur.start_at);
            const e = a.end !== undefined ? parseDate(a.end, "end") : a.start !== undefined ? new Date(new Date(cur.end_at).getTime() - new Date(cur.start_at).getTime() + (s as Date).getTime()) : new Date(cur.end_at);
            if (typeof s === "string") return { error: s };
            if (typeof e === "string") return { error: e };
            if (e <= s) return { error: "end must be after start" };
            next.start_at = s.toISOString();
            next.end_at = e.toISOString();
          }
          if (a.title !== undefined) next.title = clip(str(a.title), 200) || cur.title;
          if (a.description !== undefined) next.description = clip(typeof a.description === "string" ? a.description : "", 4000);
          if (a.location !== undefined) next.location = clip(str(a.location), 300);
          if (a.attendees !== undefined) next.attendees = strList(a.attendees, 50);
          const before = toInput(cur);
          const ev = await ad.updateEvent(cur.id, next);
          return { result: { id: ev.id, title: ev.title, start: ev.start_at, end: ev.end_at }, summary: `Updated "${clip(ev.title, 60)}"`, undo: chainUndo([async () => void (await ad.updateEvent(cur.id, before))]) };
        }),
    }),
    defineTool({
      name: "calendar_meeting_context",
      description: "Collect material for a meeting briefing: searches the user's own mail, docs and files for the meeting title or other keywords and returns the matching titles and snippets. Use together with calendar_list_events.",
      risk: "read",
      parameters: { type: "object", properties: { query: { type: "string", description: "Meeting title or keywords" } }, required: ["query"] },
      handler: (a) =>
        guard(async () => {
          const q = str(a.query);
          if (!q) return { error: "query is required" };
          const hits = await ad.search(q);
          return { result: { hits: hits.slice(0, 12).map((h) => ({ app: h.app, title: clip(h.title, 120), snippet: clip(h.snippet, 240), url: h.url })) } };
        }),
    }),
  ];
}

export function calendarContext(label: string, events: CalEvent[]): { label: string; text: string } | null {
  if (!label) return null;
  const text = events
    .slice(0, 40)
    .map((e) => `${e.start_at} - ${e.end_at} ${e.title}${e.location ? ` @ ${clip(e.location, 60)}` : ""}`)
    .join("\n");
  return { label, text: text || "(no events)" };
}

export const CALENDAR_SUGGESTIONS = [
  "Summarise my day",
  "Summarise my week",
  "Find a free 30-minute slot this week",
  "Draft an agenda for my next meeting",
  "Prepare a briefing for my next meeting",
];
