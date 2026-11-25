import { describe, expect, it } from "vitest";
import { arcPath, fmtPct, fmtPrice, fmtShares, parseExit, parseShares, slices } from "./logic";

describe("cap table formatting", () => {
  it("groups shares the Indian way and parses them back exactly", () => {
    expect(fmtShares(10000000)).toBe("1,00,00,000");
    expect(parseShares("1,00,00,000")).toBe(10000000);
    expect(parseShares("12.5")).toBeNull();
    expect(parseShares("-3")).toBeNull();
    expect(parseShares("")).toBeNull();
  });
  it("shows server percentages without float rounding", () => {
    expect(fmtPct(166667, 4)).toBe("16.6667%");
    expect(fmtPct(833333, 2)).toBe("83.33%");
    expect(fmtPct(1000000, 0)).toBe("100%");
    expect(fmtPct(5, 4)).toBe("0.0005%");
  });
  it("shows prices with the decimals they need", () => {
    expect(fmtPrice("12.5")).toBe("₹12.50");
    expect(fmtPrice("260")).toBe("₹260.00");
    expect(fmtPrice("0.4512")).toBe("₹0.4512");
  });
  it("reads exit values with crore and lakh units into exact paise", () => {
    expect(parseExit("150 cr")).toBe(150_00_00_000 * 100);
    expect(parseExit("2.5 crore")).toBe(2_50_00_000 * 100);
    expect(parseExit("40 lakh")).toBe(40_00_000 * 100);
    expect(parseExit("1,00,000")).toBe(10_000_000);
    expect(parseExit("x")).toBeNull();
  });
  it("builds chart geometry", () => {
    const s = slices([{ label: "a", value: 3 }, { label: "b", value: 0 }, { label: "c", value: 1 }]);
    expect(s.map((x) => x.label)).toEqual(["a", "c"]);
    expect(arcPath(50, 50, 30, 45, 0, 0.5)).toMatch(/^M50\.00,5\.00 A45,45 0 0 1 50\.00,95\.00/);
    expect(arcPath(50, 50, 30, 45, 0, 1)).toContain("A45,45 0 1 1");
  });
});
