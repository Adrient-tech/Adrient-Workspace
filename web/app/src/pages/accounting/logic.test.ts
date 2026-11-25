import { describe, expect, it } from "vitest";
import { balanceOf, csvEscape, fyEnd, fyStart, gstOf, money, toCSV } from "./logic";

describe("accounting logic", () => {
  it("balances in whole paise", () => {
    const b = balanceOf([
      { debit: 0.1, credit: 0 },
      { debit: 0.2, credit: 0 },
      { debit: 0, credit: 0.3 },
    ]);
    expect(b.balanced).toBe(true);
    expect(b.diff).toBe(0);
  });
  it("flags unbalanced and empty journals", () => {
    expect(balanceOf([{ debit: 100, credit: 0 }, { debit: 0, credit: 90 }]).balanced).toBe(false);
    expect(balanceOf([{ debit: 100, credit: 0 }, { debit: 0, credit: 90 }]).diff).toBe(10);
    expect(balanceOf([]).balanced).toBe(false);
  });
  it("finds the Indian financial year", () => {
    expect(fyStart("2026-11-16")).toBe("2026-04-01");
    expect(fyStart("2026-03-31")).toBe("2025-04-01");
    expect(fyStart("2026-01-15", 1)).toBe("2026-01-01");
    expect(fyEnd("2026-04-01")).toBe("2027-03-31");
  });
  it("computes GST", () => {
    expect(gstOf(1000, 18)).toEqual({ gst: 180, total: 1180 });
    expect(gstOf(99.99, 5).total).toBe(104.99);
  });
  it("formats rupees and CSV", () => {
    expect(money(1234567.5)).toContain("12,34,567.50");
    expect(csvEscape('a,"b"')).toBe('"a,""b"""');
    expect(toCSV([["x", 1], ["y,z", 2]])).toBe('x,1\n"y,z",2');
  });
});
