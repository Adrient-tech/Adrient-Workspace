import { describe, expect, it } from "vitest";
import {
  cleanAvailability,
  dayKey,
  defaultAvailability,
  formatDuration,
  groupSlotsByDay,
  isValidEmail,
  publicLink,
  timeLabel,
  validateWindows,
} from "./logic";

describe("booking logic", () => {
  it("validates windows like the server", () => {
    expect(validateWindows([{ start: "09:00", end: "12:00" }, { start: "13:00", end: "17:00" }])).toBe("");
    expect(validateWindows([{ start: "12:00", end: "09:00" }])).not.toBe("");
    expect(validateWindows([{ start: "09:00", end: "11:00" }, { start: "10:00", end: "12:00" }])).toBe("Windows overlap");
    expect(validateWindows([{ start: "9am", end: "10:00" }])).not.toBe("");
  });

  it("defaults to Monday to Friday 9 to 17 and drops empty days", () => {
    const a = defaultAvailability();
    expect(Object.keys(a).sort()).toEqual(["1", "2", "3", "4", "5"]);
    expect(cleanAvailability({ ...a, "6": [] })["6"]).toBeUndefined();
  });

  it("groups slots by local day in the viewer time zone", () => {
    // 22:30 UTC is already the next day in Kolkata (UTC+5:30).
    const slots = ["2030-01-07T22:30:00Z", "2030-01-07T09:00:00Z"];
    expect(dayKey(slots[0], "UTC")).toBe("2030-01-07");
    expect(dayKey(slots[0], "Asia/Kolkata")).toBe("2030-01-08");
    const g = groupSlotsByDay(slots, "Asia/Kolkata");
    expect([...g.keys()]).toEqual(["2030-01-07", "2030-01-08"]);
    expect(g.get("2030-01-07")![0].label).toBe("14:30");
    expect(timeLabel(slots[0], "Asia/Kolkata")).toBe("04:00");
  });

  it("formats durations, links and emails", () => {
    expect(formatDuration(30)).toBe("30 min");
    expect(formatDuration(90)).toBe("1 h 30 min");
    expect(formatDuration(120)).toBe("2 h");
    expect(publicLink("https://x.test/", "intro-ab12")).toBe("https://x.test/book/intro-ab12");
    expect(isValidEmail("a@b.co")).toBe(true);
    expect(isValidEmail("nope")).toBe(false);
  });
});
