import { describe, it, expect } from "vitest";
import { addToCart, dayRange, money, priceCart, quickTender, receiptText, summarizeTenders } from "./logic";
import type { CartLine, PosSale, PosTaxRate } from "./types";

const vat: PosTaxRate = { id: "t1", name: "VAT", rate: 10, inclusive: false, is_default: true };
const gst: PosTaxRate = { id: "t2", name: "GST", rate: 18, inclusive: true, is_default: false };

const line = (o: Partial<CartLine> = {}): CartLine => ({
  key: "k",
  product_id: "",
  name: "x",
  sku: "",
  qty: 1,
  unit_price: 10,
  discount_kind: "",
  discount_value: 0,
  tax_rate_id: "",
  note: "",
  ...o,
});

describe("priceCart", () => {
  it("adds exclusive tax and applies line discounts", () => {
    const r = priceCart([line({ qty: 2, tax_rate_id: "t1" }), line({ unit_price: 5, discount_kind: "percent", discount_value: 20 })], [vat]);
    expect(r).toMatchObject({ subtotal: 25, discount: 1, tax: 2, total: 26 });
  });
  it("extracts inclusive tax", () => {
    const r = priceCart([line({ unit_price: 118, tax_rate_id: "t2" })], [gst]);
    expect(r).toMatchObject({ total: 118, tax: 18 });
  });
  it("spreads an order discount to the cent", () => {
    const r = priceCart([line(), line(), line()], [], "amount", 10);
    expect(r.total).toBe(20);
    expect(r.lines.reduce((a, l) => a + l.discount, 0)).toBeCloseTo(10, 2);
  });
  it("caps discounts at the line value", () => {
    expect(priceCart([line({ unit_price: 5, discount_kind: "amount", discount_value: 50 })], [], "percent", 150).total).toBe(0);
  });
});

describe("summarizeTenders", () => {
  it("returns change only from cash", () => {
    expect(summarizeTenders(26, [{ method: "cash", amount: 20 }, { method: "card", amount: 10 }])).toEqual({
      applied: 26,
      remaining: 0,
      change: 4,
      ok: true,
    });
  });
  it("reports what remains", () => {
    expect(summarizeTenders(26, [{ method: "cash", amount: 20 }])).toMatchObject({ remaining: 6, ok: false });
  });
  it("rejects card overpay", () => {
    expect(summarizeTenders(10, [{ method: "card", amount: 12 }]).ok).toBe(false);
  });
});

describe("quickTender", () => {
  it("offers the exact amount then round notes", () => {
    expect(quickTender(26)).toEqual([26, 30, 40, 50, 100]);
    expect(quickTender(0)).toEqual([]);
  });
});

describe("addToCart", () => {
  const p = { product_id: "p1", name: "Widget", sku: "W", unit_price: 4, tax_rate_id: "" };
  it("merges the same product", () => {
    const c = addToCart(addToCart([], p), p);
    expect(c).toHaveLength(1);
    expect(c[0].qty).toBe(2);
  });
  it("keeps edited lines separate", () => {
    const c = addToCart([line({ product_id: "p1", unit_price: 4, note: "no ice" })], p);
    expect(c).toHaveLength(2);
  });
});

describe("formatting", () => {
  it("formats money with and without a currency", () => {
    expect(money(3.5)).toBe("3.50");
    expect(money(3.5, "USD")).toContain("3.50");
    expect(money(NaN)).toBe("0.00");
  });
  it("builds a plain-text receipt", () => {
    const sale = {
      number: 7,
      created_at: "2026-01-01T10:00:00Z",
      completed_at: "2026-01-01T10:00:00Z",
      customer_name: "Sam",
      lines: [{ qty: 2, name: "Widget", line_total: 20, note: "" }],
      subtotal: 20,
      discount_total: 0,
      tax_total: 0,
      total: 20,
      payments: [{ method: "cash", tendered: 25, amount: 20 }],
      change_due: 5,
      refunds: [],
    } as unknown as PosSale;
    const t = receiptText(sale, { store_name: "Shop", currency: "", receipt_header: "", receipt_footer: "Bye" });
    expect(t).toContain("Receipt #7");
    expect(t).toContain("2 x Widget  20.00");
    expect(t).toContain("Change: 5.00");
    expect(t.endsWith("Bye")).toBe(true);
  });
  it("computes day ranges", () => {
    const r = dayRange(0, new Date(2026, 0, 15, 13));
    expect(new Date(r.since).getDate()).toBe(15);
    expect(new Date(r.until).getDate()).toBe(16);
  });
});
