import { describe, expect, it } from "vitest";
import { describeGap, hoursStatus, minutesByEmployee } from "./logic";
import type { Shift } from "./api";

const sh = (id: string, emp: string): Shift => ({ id, date: "2026-11-02", start: "09:00", end: "17:00", break_minutes: 30, role: "Cashier", location: "", notes: "", employee_id: emp, published: true });

describe("shifts logic", () => {
  it("sums minutes per employee and ignores open shifts", () => {
    const m = minutesByEmployee([sh("a", "e1"), sh("b", "e1"), sh("c", "")], () => 450);
    expect(m.get("e1")).toBe(900);
    expect(m.has("")).toBe(false);
  });
  it("compares hours with target and max", () => {
    expect(hoursStatus(2500, { target_hours: 30, max_hours: 40 })).toBe("over");
    expect(hoursStatus(600, { target_hours: 30, max_hours: 40 })).toBe("under");
    expect(hoursStatus(2000, { target_hours: 30, max_hours: 40 })).toBe("ok");
    expect(hoursStatus(5000, { target_hours: null, max_hours: null })).toBe("ok");
  });
  it("describes a gap", () => {
    expect(describeGap({ date: "2026-11-02", role: "Cook", required: 2, scheduled: 1, missing: 1 })).toBe("2026-11-02 Cook: 1 of 2 (1 short)");
  });
});
