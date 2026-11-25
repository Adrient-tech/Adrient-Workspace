import { describe, expect, it } from "vitest";
import { buildRule, describeRule, initialDue, parseQuickAdd } from "./quickparse";

const today = "2026-11-19"; // Thursday
const lists = [{ id: "L1", name: "Home" }, { id: "L2", name: "Work" }];

describe("parseQuickAdd", () => {
  it("parses the full example", () => {
    const d = parseQuickAdd("pay rent friday 9am every month !high #home", { today, lists });
    expect(d.title).toBe("pay rent");
    expect(d.due).toBe("2026-11-20");
    expect(d.dueTime).toBe("09:00");
    expect(d.repeat).toBe("monthly");
    expect(d.priority).toBe(3);
    expect(d.listId).toBe("L1");
    expect(d.tokens.map((t) => t.type).sort()).toEqual(["due", "list", "priority", "repeat", "time"]);
  });
  it("handles relative dates and month names", () => {
    expect(parseQuickAdd("a tomorrow", { today }).due).toBe("2026-11-20");
    expect(parseQuickAdd("a next week", { today }).due).toBe("2026-11-23");
    expect(parseQuickAdd("a aug 12", { today }).due).toBe("2027-08-12");
    expect(parseQuickAdd("a 2026-02-30", { today }).due).toBeNull();
    expect(parseQuickAdd("a 2026-02-30", { today }).title).toBe("a 2026-02-30");
  });
  it("does not eat names that start like weekdays", () => {
    const d = parseQuickAdd("call monica", { today });
    expect(d.due).toBeNull();
    expect(d.title).toBe("call monica");
  });
  it("builds interval and weekday rules", () => {
    expect(parseQuickAdd("x every 2 weeks", { today }).repeat).toBe("every:2:weekly");
    expect(parseQuickAdd("x every mon,wed", { today }).repeat).toBe("every:1:weekly:1,3");
    expect(parseQuickAdd("x every weekday", { today }).repeat).toBe("weekdays");
    expect(parseQuickAdd("x every monday", { today }).due).toBe("2026-11-23");
  });
  it("keeps tags, caps at five, and returns dismissed chips to the title", () => {
    const d = parseQuickAdd("x #a #b #c #d #e #f", { today });
    expect(d.tags).toEqual(["a", "b", "c", "d", "e"]);
    expect(d.title).toBe("x #f");
    const i = parseQuickAdd("x tomorrow", { today, ignore: new Set(["due"]) });
    expect(i.due).toBeNull();
    expect(i.title).toBe("x tomorrow");
  });
  it("times default to today", () => {
    const d = parseQuickAdd("call at 15:30", { today });
    expect(d.due).toBe(today);
    expect(d.dueTime).toBe("15:30");
    expect(parseQuickAdd("a 9:99am", { today }).dueTime).toBeNull();
  });
});

describe("rules", () => {
  it("builds and describes", () => {
    expect(buildRule("weekly", 1, [1, 2, 3, 4, 5])).toBe("weekdays");
    expect(describeRule("every:2:weekly:1,3")).toBe("Every 2 weeks on Mon, Wed");
    expect(describeRule("every:3:daily:done")).toBe("Every 3 days after completion");
    expect(describeRule("weekly")).toBe("Every week");
  });
  it("starts weekday rules on the nearest chosen day", () => {
    expect(initialDue("every:1:weekly:1", today)).toBe("2026-11-23");
    expect(initialDue("every:1:weekly:4", today)).toBe(today);
  });
});
