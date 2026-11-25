import { describe, expect, it } from "vitest";
import {
  LABEL_PRESETS,
  addScan,
  cleanScan,
  code128Modules,
  countVariance,
  daysCoverText,
  expiryText,
  expiryTone,
  labelBarcodeSvg,
  labelSlots,
  lineTotals,
  poActions,
  receiptLimit,
  sheetCount,
} from "./stockLogic";

describe("code128", () => {
  it("encodes with start B, checksum and stop pattern", () => {
    const bits = code128Modules("PJJ123C");
    // 1 start + 7 chars + 1 checksum = 9 symbols of 11 modules, plus the 13-module stop
    expect(bits.length).toBe(9 * 11 + 13);
    expect(bits.startsWith("11010010000")).toBe(true); // start code B
    expect(bits.endsWith("1100011101011")).toBe(true);
  });
  it("computes the check symbol (104 + 48*1 + 42*2 + 42*3 + 17*4 + 18*5 + 19*6 + 35*7 = 879, 879 mod 103 = 55)", () => {
    const bits = code128Modules("PJJ123C");
    const sym = (n: number) => bits.slice(n * 11, n * 11 + 11);
    expect(sym(8)).toBe("11101000110"); // pattern of value 55
  });
  it("every data symbol is 11 modules with 3 bars and 3 spaces", () => {
    for (let c = 32; c < 127; c++) {
      const bits = code128Modules(String.fromCharCode(c));
      for (let k = 0; k < 3; k++) {
        const s = bits.slice(k * 11, k * 11 + 11);
        expect(s.length).toBe(11);
        expect(s.match(/1+|0+/g)!.length).toBe(6);
        expect(s.startsWith("1") && s.endsWith("0")).toBe(true);
      }
    }
  });
  it("rejects non-ASCII and empty input", () => {
    expect(() => code128Modules("é")).toThrow();
    expect(() => code128Modules("")).toThrow();
  });
  it("picks EAN-13 when valid and falls back to Code 128", () => {
    expect(labelBarcodeSvg("2000000000008")).toContain("2000000000008");
    expect(labelBarcodeSvg("LOT-2026-A")).toContain("LOT-2026-A");
    expect(labelBarcodeSvg("é")).toBe("");
  });
});

describe("label sheets", () => {
  const a4 = LABEL_PRESETS.find((p) => p.id === "a4-65")!;
  it("places the first, second row and 66th labels", () => {
    const s = labelSlots(a4, 66);
    expect(s[0]).toEqual({ page: 0, x: 4.7, y: 10.7 });
    expect(s[1].x).toBeCloseTo(4.7 + 38.1 + 2.5, 2);
    expect(s[5]).toEqual({ page: 0, x: 4.7, y: 31.9 }); // row 2
    expect(s[65].page).toBe(1);
    expect(s[65].x).toBe(4.7);
  });
  it("respects a start offset on a partly used sheet", () => {
    const s = labelSlots(a4, 3, 63);
    expect(s.map((x) => x.page)).toEqual([0, 0, 1]);
    expect(sheetCount(a4, 3, 63)).toBe(2);
    expect(sheetCount(a4, 0)).toBe(0);
  });
  it("every preset fits its page", () => {
    for (const p of LABEL_PRESETS) {
      expect(p.marginLeft + p.cols * p.labelW + (p.cols - 1) * p.gapX).toBeLessThanOrEqual(p.pageW + 0.01);
      expect(p.marginTop + p.rows * p.labelH + (p.rows - 1) * p.gapY).toBeLessThanOrEqual(p.pageH + 0.01);
    }
  });
});

describe("purchase order helpers", () => {
  it("offers the right buttons per state", () => {
    expect(poActions("draft", 0)).toEqual(["edit", "approve", "cancel"]);
    expect(poActions("approved", 0)).toContain("send");
    expect(poActions("approved", 0)).not.toContain("bill");
    expect(poActions("partial", 4)).toEqual(["receive", "bill", "close"]);
    expect(poActions("cancelled", 0)).toEqual([]);
    expect(poActions("closed", 5)).toEqual(["bill"]);
  });
  it("totals lines with tax and rounding", () => {
    expect(lineTotals([{ qty: 3, unit_cost: 33.33, tax_pct: 18 }, { qty: 1, unit_cost: 10, tax_pct: 0 }])).toEqual({
      subtotal: 109.99,
      tax: 18,
      total: 127.99,
    });
    expect(lineTotals([])).toEqual({ subtotal: 0, tax: 0, total: 0 });
  });
  it("computes the over-receipt limit", () => {
    expect(receiptLimit(100, 60, 0)).toBe(40);
    expect(receiptLimit(100, 60, 10)).toBe(50);
    expect(receiptLimit(100, 130, 10)).toBe(0);
  });
});

describe("lots, counts and scans", () => {
  it("describes expiry", () => {
    expect(expiryTone(-1)).toBe("danger");
    expect(expiryTone(10)).toBe("warning");
    expect(expiryTone(200)).toBe("success");
    expect(expiryTone(null)).toBe("neutral");
    expect(expiryTone(200, true)).toBe("danger");
    expect(expiryText(-3)).toBe("Expired 3 days ago");
    expect(expiryText(1)).toBe("1 day left");
    expect(expiryText(null)).toBe("No expiry");
  });
  it("computes count variance with value", () => {
    expect(countVariance(30, 27, 2.5)).toEqual({ qty: -3, value: -7.5 });
    expect(countVariance(30, null, 2.5)).toEqual({ qty: 0, value: 0 });
  });
  it("tallies scans and strips scanner noise", () => {
    expect(cleanScan("  8901234567890\r\n")).toBe("8901234567890");
    let l = addScan([], "p1", "Pen");
    l = addScan(l, "p2", "Pad");
    l = addScan(l, "p1", "Pen", 2);
    expect(l).toEqual([{ key: "p1", label: "Pen", qty: 3 }, { key: "p2", label: "Pad", qty: 1 }]);
  });
  it("words days of cover", () => {
    expect(daysCoverText(-1)).toBe("No recent sales");
    expect(daysCoverText(0)).toBe("Out of stock");
    expect(daysCoverText(12.4)).toBe("12 days");
    expect(daysCoverText(400)).toBe("1 year +");
  });
});
