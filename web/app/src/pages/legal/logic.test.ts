import { describe, expect, it } from "vitest";
import { csv, fmtHM, fmtMoney, parseDuration, parseMoney, relDays, roundUp } from "./logic";

describe("legal money", () => {
  it("parses rupee amounts to exact paise", () => {
    expect(parseMoney("1,234.50")).toBe(123450);
    expect(parseMoney("₹ 4500")).toBe(450000);
    expect(parseMoney("0.1")).toBe(10);
    expect(parseMoney("19.99")).toBe(1999);
    expect(parseMoney("1.234")).toBeNull();
    expect(parseMoney("abc")).toBeNull();
    expect(parseMoney("-25")).toBe(-2500);
  });
  it("formats paise with Indian grouping and no float rounding", () => {
    expect(fmtMoney(123450)).toBe("₹1,234.50");
    expect(fmtMoney(1000000007)).toBe("₹1,00,00,000.07");
    expect(fmtMoney(-5)).toBe("-₹0.05");
    expect(fmtMoney(null)).toBe("");
  });
});

describe("durations", () => {
  it("reads every input form", () => {
    const cases: Record<string, number> = { "1.5": 90, "1,5": 90, "1:12": 72, "1h 12m": 72, "1h12": 72, "45m": 45, "2 hrs": 120, ".3": 18, "0:30": 30 };
    for (const [k, v] of Object.entries(cases)) expect(parseDuration(k)).toBe(v);
    expect(parseDuration("")).toBeNull();
    expect(parseDuration("1:75")).toBeNull();
    expect(parseDuration("soon")).toBeNull();
  });
  it("rounds up to the increment", () => {
    expect(roundUp(7, 6)).toBe(12);
    expect(roundUp(6, 6)).toBe(6);
    expect(roundUp(0, 6)).toBe(0);
    expect(fmtHM(72)).toBe("1h 12m");
    expect(fmtHM(120)).toBe("2h");
  });
});

describe("misc", () => {
  it("describes days", () => {
    expect(relDays(0)).toBe("today");
    expect(relDays(3)).toBe("in 3 days");
    expect(relDays(-2)).toBe("2 days ago");
  });
  it("neutralises spreadsheet formulas in CSV", () => {
    expect(csv([["=HYPERLINK(1)", 'a "b", c']])).toBe(`'=HYPERLINK(1),"a ""b"", c"`);
  });
});
