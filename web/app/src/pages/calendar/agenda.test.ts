import { describe, expect, it } from "vitest";
import { groupAgenda } from "./agenda";
import type { CalendarEvent } from "./types";

const ev = (id: string, title: string, s: string, e: string, allDay = false): CalendarEvent => ({
  id, org_id: "", owner_id: "", title, description: "", location: "",
  start_at: new Date(s).toISOString(), end_at: new Date(e).toISOString(), all_day: allDay,
  color: "", recurrence: "", created_at: "", updated_at: "",
});

describe("groupAgenda", () => {
  const from = new Date(2026, 10, 19);
  it("groups by day, sorts all-day first then by time", () => {
    const days = groupAgenda(
      [
        ev("b", "Later", "2026-11-19T15:00:00", "2026-11-19T16:00:00"),
        ev("a", "Early", "2026-11-19T09:00:00", "2026-11-19T10:00:00"),
        ev("h", "Holiday", "2026-11-19T00:00:00", "2026-11-20T00:00:00", true),
        ev("c", "Next day", "2026-11-20T09:00:00", "2026-11-20T10:00:00"),
      ],
      from,
    );
    expect(days.map((d) => d.key)).toEqual(["2026-11-19", "2026-11-20"]);
    expect(days[0].events.map((e) => e.id)).toEqual(["h", "a", "b"]);
  });
  it("lists multi-day events on each day and ends at midnight", () => {
    const days = groupAgenda([ev("t", "Trip", "2026-11-20T00:00:00", "2026-11-23T00:00:00", true)], from);
    expect(days.map((d) => d.key)).toEqual(["2026-11-20", "2026-11-21", "2026-11-22"]);
  });
  it("clips to the window and handles events that started earlier", () => {
    const days = groupAgenda([ev("o", "Old", "2026-11-10T10:00:00", "2026-11-20T10:00:00"), ev("far", "Far", "2027-03-01T10:00:00", "2027-03-01T11:00:00")], from, 7);
    expect(days.map((d) => d.key)).toEqual(["2026-11-19", "2026-11-20"]);
  });
});
