import { describe, expect, it } from "vitest";
import { createCalendarTools, freeSlots, type CalEvent, type CalendarAdapter } from "./tools";

const ev = (id: string, s: string, e: string, o: Partial<CalEvent> = {}): CalEvent => ({ id, title: id, description: "", location: "", start_at: s, end_at: e, all_day: false, color: "", recurrence: "", ...o });
const d = (h: number, m = 0, day = 18) => new Date(2026, 10, day, h, m).toISOString(); // Wed 18 Nov 2026

function setup(events: CalEvent[]) {
  const store = [...events];
  const ad: CalendarAdapter = {
    listEvents: async () => store.map((e) => ({ ...e })),
    createEvent: async (i) => { const e = { ...i, id: "n" + store.length } as CalEvent; store.push(e); return e; },
    updateEvent: async (id, i) => { const k = store.findIndex((e) => e.id === id); store[k] = { ...i, id } as CalEvent; return store[k]; },
    deleteEvent: async (id) => { store.splice(store.findIndex((e) => e.id === id), 1); },
    search: async () => [{ app: "mail", title: "Agenda mail", snippet: "x", url: "/mail" }],
    visibleRange: () => ({ from: new Date(2026, 10, 18), to: new Date(2026, 10, 19), label: "Wed" }),
    now: () => new Date(2026, 10, 18, 8, 0),
  };
  return { store, tools: Object.fromEntries(createCalendarTools(ad).map((t) => [t.name, t])) };
}

describe("freeSlots", () => {
  it("skips busy blocks and rounds to 15 minutes", () => {
    const s = freeSlots([ev("a", d(9), d(10, 40))], new Date(2026, 10, 18, 8), new Date(2026, 10, 19), 30, { limit: 2 });
    expect(new Date(s[0].start).getHours()).toBe(10);
    expect(new Date(s[0].start).getMinutes()).toBe(45);
  });
  it("skips weekends", () => {
    expect(freeSlots([], new Date(2026, 10, 21, 8), new Date(2026, 10, 22, 20), 30)).toEqual([]);
  });
});

describe("calendar tools", () => {
  it("finds a free slot after the existing meeting", async () => {
    const { tools } = setup([ev("a", d(9), d(12))]);
    const r = (await tools.calendar_find_free_slot.handler({ duration_min: 60, to: d(18) })) as { result: { slots: { start: string }[] } };
    expect(new Date(r.result.slots[0].start).getHours()).toBe(12);
  });
  it("create is confirmed and undo deletes; bad times rejected", async () => {
    const { tools, store } = setup([]);
    expect(tools.calendar_create_event.risk).toBe("destructive");
    const r = await tools.calendar_create_event.handler({ title: "Sync", start: d(14), end: d(15), attendees: ["a@x.io"] });
    expect(store).toHaveLength(1);
    await r.undo!();
    expect(store).toHaveLength(0);
    expect((await tools.calendar_create_event.handler({ title: "x", start: d(15), end: d(14) })).error).toContain("after");
  });
  it("moves an event keeping its length; undo restores; refuses recurring", async () => {
    const { tools, store } = setup([ev("a", d(9), d(10)), ev("r", d(9), d(10), { recurrence: "FREQ=DAILY" })]);
    const base = { lookup_from: d(0), lookup_to: d(23) };
    const r = await tools.calendar_update_event.handler({ event_id: "a", start: d(11), ...base });
    expect(store[0].start_at).toBe(d(11));
    expect(store[0].end_at).toBe(d(12));
    await r.undo!();
    expect(store[0].start_at).toBe(d(9));
    expect((await tools.calendar_update_event.handler({ event_id: "r", title: "x", ...base })).error).toContain("recurring");
  });
  it("lists events and returns related hits", async () => {
    const { tools } = setup([ev("a", d(9), d(10))]);
    expect(((await tools.calendar_list_events.handler({})) as { result: { count: number } }).result.count).toBe(1);
    expect(((await tools.calendar_meeting_context.handler({ query: "x" })) as { result: { hits: unknown[] } }).result.hits).toHaveLength(1);
  });
});
