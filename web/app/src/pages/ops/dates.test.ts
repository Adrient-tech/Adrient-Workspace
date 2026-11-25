import { describe, expect, it } from "vitest";
import { addDays, addMonths, durationMinutes, fmtMinutes, fmtRange, monthEnd, netMinutes, startOfWeek, weekDates } from "./dates";

describe("dates", () => {
  it("finds the Monday of a week, also for Sundays", () => {
    expect(startOfWeek("2026-11-04")).toBe("2026-11-02");
    expect(startOfWeek("2026-11-08")).toBe("2026-11-02");
    expect(startOfWeek("2026-11-02")).toBe("2026-11-02");
    expect(weekDates("2026-11-02")).toHaveLength(7);
  });
  it("adds days across month ends and months", () => {
    expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
    expect(addMonths("2026-11-19", 2)).toBe("2027-01-01");
    expect(monthEnd("2028-02-10")).toBe("2028-02-29");
  });
  it("handles overnight shifts and breaks", () => {
    expect(durationMinutes("22:00", "06:00")).toBe(480);
    expect(durationMinutes("09:00", "09:00")).toBeNull();
    expect(netMinutes("09:00", "17:00", 30)).toBe(450);
    expect(netMinutes("09:00", "09:30", 60)).toBe(0);
    expect(fmtRange("22:00", "06:00")).toBe("10 PM – 6 AM +1");
    expect(fmtMinutes(450)).toBe("7h 30m");
  });
});
