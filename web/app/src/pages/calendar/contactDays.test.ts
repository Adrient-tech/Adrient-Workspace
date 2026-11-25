import { describe, expect, it } from "vitest";
import { dayTitle, isContactDay, localMidnight, toCalendarEvents } from "./contactDays";

describe("contact days on the calendar", () => {
  it("builds read-only all-day entries on the right local day", () => {
    const [e] = toCalendarEvents([{ contact_id: "c1", name: "Asha", kind: "birthday", date: "2030-05-17", years: 35 }]);
    expect(isContactDay(e)).toBe(true);
    expect(e.all_day).toBe(true);
    expect(new Date(e.start_at).getDate()).toBe(17);
    expect(new Date(e.end_at).getDate()).toBe(18);
    expect(e.title).toBe("Asha's birthday (35)");
  });
  it("hides implausible ages and labels anniversaries", () => {
    expect(dayTitle({ contact_id: "c", name: "Ben", kind: "anniversary", date: "2030-01-01", years: 0 })).toBe("Ben's anniversary");
    expect(dayTitle({ contact_id: "c", name: "Ben", kind: "birthday", date: "2030-01-01", years: 500 })).toBe("Ben's birthday");
  });
  it("uses local midnight without time zone drift", () => {
    expect(localMidnight("2030-03-31", 1).getMonth()).toBe(3); // April
  });
  it("does not treat real events as contact days", () => {
    expect(isContactDay({ id: "55c1a6f4-deae" })).toBe(false);
  });
});
