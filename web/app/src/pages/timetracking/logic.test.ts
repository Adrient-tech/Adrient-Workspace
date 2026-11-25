import { describe, expect, it } from "vitest";
import { fmtClock, fmtDuration, fmtTimer, parseDuration, roundSeconds, secondsByDay } from "./logic";

describe("parseDuration", () => {
  it("reads the common spellings", () => {
    expect(parseDuration("1:30")).toBe(5400);
    expect(parseDuration("90m")).toBe(5400);
    expect(parseDuration("1.5h")).toBe(5400);
    expect(parseDuration("1h 20m")).toBe(4800);
    expect(parseDuration("2h30")).toBe(9000);
    expect(parseDuration("2")).toBe(7200);
    expect(parseDuration("45")).toBe(2700);
    expect(parseDuration(",5")).toBe(1800);
    expect(parseDuration(":15")).toBe(900);
  });
  it("rejects nonsense", () => {
    for (const x of ["", "abc", "1:99", "h"]) expect(Number.isNaN(parseDuration(x))).toBe(true);
  });
});

describe("formatting and rounding", () => {
  it("formats", () => {
    expect(fmtClock(5400)).toBe("1:30");
    expect(fmtTimer(3723)).toBe("1:02:03");
    expect(fmtDuration(5400)).toBe("1h 30m");
    expect(fmtDuration(120)).toBe("2m");
  });
  it("rounds up to the increment", () => {
    expect(roundSeconds(61, 15)).toBe(900);
    expect(roundSeconds(900, 15)).toBe(900);
    expect(roundSeconds(500, 0)).toBe(500);
  });
  it("sums per day", () => {
    const m = secondsByDay([{ date: "2026-11-02", seconds: 60 }, { date: "2026-11-02", seconds: 30 }, { date: "2026-11-03", seconds: 5 }]);
    expect(m.get("2026-11-02")).toBe(90);
  });
});
