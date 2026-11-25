import { describe, expect, it } from "vitest";
import { GST_STATES, addDays, computeTotals, isIntraState, parseCSV, parseInvoicesCSV, parseItemsCSV, splitSchedule, toCSV, upiLink } from "./logic";

const line = (qty: number, price: number, tax = 0, disc = 0) => ({
  description: "x",
  qty,
  unit: "",
  price,
  discount_pct: disc,
  tax_rate: tax,
  tax_name: "",
});

describe("computeTotals", () => {
  it("matches the server for percent discount and tax", () => {
    const t = computeTotals([line(2, 100, 18), line(1, 50)], "percent", 10);
    expect(t).toEqual({ subtotal: 250, discount_total: 25, tax_total: 32.4, total: 257.4 });
  });
  it("clamps an amount discount to the subtotal", () => {
    expect(computeTotals([line(1, 10)], "amount", 99).total).toBe(0);
  });
  it("applies line discounts", () => {
    expect(computeTotals([line(3, 10, 10, 50)], "percent", 0).total).toBe(16.5);
  });
});

describe("schedule", () => {
  it("splits so the parts add up to the total", () => {
    const s = splitSchedule(100, 3, "2026-01-01");
    expect(s.map((x) => x.amount)).toEqual([33.33, 33.33, 33.34]);
    expect(s[1].due_date).toBe(addDays("2026-01-01", 30));
  });
});

describe("csv", () => {
  it("handles quotes and newlines", () => {
    expect(parseCSV('a,"b,1","c ""q"""\n1,2,3\n')).toEqual([
      ["a", "b,1", 'c "q"'],
      ["1", "2", "3"],
    ]);
    expect(toCSV([["a,b", 'q"']])).toBe('"a,b","q"""');
  });
  it("parses a price list", () => {
    const r = parseItemsCSV("Name,Unit,Price,Tax\nWidget,pcs,12.5,18\n,,,\n");
    expect(r).toEqual([{ name: "Widget", description: "", unit: "pcs", price: 12.5, tax_rate: 18 }]);
  });
  it("groups invoice rows by number", () => {
    const r = parseInvoicesCSV(
      "number,client,date,description,qty,price\nA1,Acme,2026-02-01,Work,2,10\nA1,Acme,2026-02-01,More,1,5\nA2,Beta,2026-02-02,Thing,1,1\n",
    );
    expect(r).toHaveLength(2);
    expect(r[0].lines).toHaveLength(2);
    expect(r[0].number).toBe("A1");
  });
});


describe("GST place of supply (mirrors the server)", () => {
  it("is intra-state for the same state, an empty place of supply, or no supplier state", () => {
    expect(isIntraState("29", "29")).toBe(true);
    expect(isIntraState("29", "")).toBe(true);
    expect(isIntraState("", "27")).toBe(true);
  });
  it("is inter-state for another state and for exports", () => {
    expect(isIntraState("29", "27")).toBe(false);
    expect(isIntraState("29", "96")).toBe(false);
  });
  it("lists the GST state codes, including the export code", () => {
    const codes = GST_STATES.map(([c]) => c);
    expect(codes).toContain("29");
    expect(codes).toContain("96");
    expect(new Set(codes).size).toBe(codes.length);
  });
});

describe("upiLink", () => {
  it("builds an NPCI pay link with a 2-decimal amount and escaped note", () => {
    const l = upiLink("shop@okbank", "My Shop", 1180, "Invoice INV-0001");
    expect(l.startsWith("upi://pay?")).toBe(true);
    const q = new URLSearchParams(l.slice("upi://pay?".length));
    expect(q.get("pa")).toBe("shop@okbank");
    expect(q.get("am")).toBe("1180.00");
    expect(q.get("cu")).toBe("INR");
    expect(q.get("tn")).toBe("Invoice INV-0001");
  });
});

describe("computeTotals with tax-inclusive prices", () => {
  const inc = (qty: number, price: number, rate: number) => ({ description: "x", qty, unit: "", price, discount_pct: 0, tax_rate: rate, tax_name: "", tax_inclusive: true });
  it("takes the tax out of the quoted price and keeps the total exact", () => {
    expect(computeTotals([inc(3, 11.9, 19)], "percent", 0)).toEqual({ subtotal: 30, discount_total: 0, tax_total: 5.7, total: 35.7 });
    for (const p of [0.99, 1.07, 3.33, 19.99, 100.01]) {
      const t = computeTotals([inc(7, p, 18)], "percent", 0);
      expect(t.total).toBe(Math.round(7 * p * 100) / 100);
      expect(Math.round((t.subtotal - t.discount_total + t.tax_total) * 100) / 100).toBe(t.total);
    }
  });
  it("applies a document discount to the net and leaves exclusive lines alone", () => {
    expect(computeTotals([inc(1, 119, 19)], "percent", 10)).toEqual({ subtotal: 100, discount_total: 10, tax_total: 17.1, total: 107.1 });
    expect(computeTotals([{ ...inc(1, 100, 10), tax_inclusive: false }], "percent", 0).total).toBe(110);
  });
});
