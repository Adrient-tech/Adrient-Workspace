import { describe, expect, it } from "vitest";
import { buildTree, currentPeriod, formatINR, initials, minutesToHM, monthDays, periodLabel, previewLeaveDays, shiftPeriod, statusLabel } from "./logic";
import type { OrgNode } from "./api";

const node = (id: string, manager_id = ""): OrgNode => ({ id, user_id: "", name: id, email: "", designation: "", department: "", department_id: "", manager_id, location: "" });

describe("formatINR", () => {
  it("groups digits the Indian way", () => {
    expect(formatINR(1234567.5)).toBe("₹12,34,567.50");
    expect(formatINR(999)).toBe("₹999.00");
    expect(formatINR(100000)).toBe("₹1,00,000.00");
    expect(formatINR(0)).toBe("₹0.00");
    expect(formatINR(-2500.1)).toBe("-₹2,500.10");
  });
});

describe("periods", () => {
  it("labels and shifts months", () => {
    expect(periodLabel("2026-06")).toBe("June 2026");
    expect(shiftPeriod("2026-01", -1)).toBe("2025-12");
    expect(shiftPeriod("2026-12", 1)).toBe("2027-01");
    expect(currentPeriod(new Date(2026, 4, 9))).toBe("2026-05");
    expect(monthDays("2026-02")).toHaveLength(28);
    expect(monthDays("2028-02")).toHaveLength(29);
  });
});

describe("previewLeaveDays", () => {
  it("skips weekly offs and holidays", () => {
    // Mon 10 Aug .. Sun 16 Aug 2026, Sunday off, Sat 15 holiday.
    expect(previewLeaveDays("2026-08-10", "2026-08-16", "", [0], ["2026-08-15"])).toBe(5);
  });
  it("handles half days", () => {
    expect(previewLeaveDays("2026-08-10", "2026-08-10", "first", [0], [])).toBe(0.5);
    expect(previewLeaveDays("2026-08-16", "2026-08-16", "first", [0], [])).toBe(0);
    expect(previewLeaveDays("2026-08-10", "2026-08-11", "first", [0], [])).toBe(0);
  });
  it("rejects reversed ranges", () => {
    expect(previewLeaveDays("2026-08-11", "2026-08-10", "", [0], [])).toBe(0);
  });
});

describe("misc", () => {
  it("formats labels", () => {
    expect(statusLabel("missing_punch")).toBe("Missing punch");
    expect(initials("Asha Rao")).toBe("AR");
    expect(initials("  Cher ")).toBe("C");
    expect(minutesToHM(545)).toBe("9h 05m");
    expect(minutesToHM(0)).toBe("0h");
  });
});

describe("buildTree", () => {
  it("nests reports under managers", () => {
    const t = buildTree([node("a"), node("b", "a"), node("c", "b"), node("d", "a")]);
    expect(t).toHaveLength(1);
    expect(t[0].reports.map((r) => r.node.id)).toEqual(["b", "d"]);
    expect(t[0].reports[0].reports[0].node.id).toBe("c");
  });
  it("surfaces loops and unknown managers as roots", () => {
    const t = buildTree([node("x", "y"), node("y", "x"), node("z", "ghost")]);
    const ids = new Set<string>();
    const walk = (ts: typeof t) => ts.forEach((n) => { ids.add(n.node.id); walk(n.reports); });
    walk(t);
    expect([...ids].sort()).toEqual(["x", "y", "z"]);
  });
});
