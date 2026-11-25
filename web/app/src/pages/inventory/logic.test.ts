import { describe, it, expect } from "vitest";
import {
  barcodeSvg,
  csvToImport,
  detectColumns,
  ean13CheckDigit,
  ean13Modules,
  generateBarcode,
  isValidEan13,
  makeEan13,
  orderSheets,
  parseCsv,
  stockStatus,
  toCsv,
} from "./logic";
import type { InvProduct, ReorderLine } from "./types";

describe("csv", () => {
  it("parses quotes, escaped quotes, CRLF and embedded newlines", () => {
    expect(parseCsv('a,"b,1","c ""q"""\r\nx,"l1\nl2",z\r\n')).toEqual([
      ["a", "b,1", 'c "q"'],
      ["x", "l1\nl2", "z"],
    ]);
  });
  it("round-trips and neutralises formulas", () => {
    const out = toCsv([["=1+1", 'he said "hi"', 5]]);
    expect(out).toBe(`'=1+1,"he said ""hi""",5\r\n`);
  });
  it("detects columns by alias", () => {
    expect(detectColumns(["Product Name", "EAN", "Qty", "Selling_Price"])).toEqual({
      name: 0,
      barcode: 1,
      quantity: 2,
      price: 3,
    });
  });
  it("builds import rows and reports bad rows", () => {
    const r = csvToImport("Name,SKU,Qty,Price,Warehouse\nBolt,B1,12,$1.50,Main\n,X,1,1,\n");
    expect(r.rows).toHaveLength(1);
    expect(r.rows[0]).toMatchObject({ name: "Bolt", sku: "B1", opening_qty: 12, price: 1.5 });
    expect(r.locations).toEqual(["Main"]);
    expect(r.errors).toHaveLength(1);
  });
  it("needs a name column", () => {
    expect(csvToImport("SKU,Qty\nA,1\n").errors[0]).toMatch(/Name/);
  });
});

describe("ean13", () => {
  it("computes the check digit of a known code", () => {
    expect(ean13CheckDigit("400638133393")).toBe(1);
    expect(isValidEan13("4006381333931")).toBe(true);
    expect(isValidEan13("4006381333932")).toBe(false);
  });
  it("generates valid internal codes", () => {
    const c = generateBarcode(42);
    expect(c).toHaveLength(13);
    expect(c.startsWith("200")).toBe(true);
    expect(isValidEan13(c)).toBe(true);
    expect(isValidEan13(makeEan13("1"))).toBe(true);
  });
  it("encodes 95 modules with guards", () => {
    const m = ean13Modules("4006381333931");
    expect(m).toHaveLength(95);
    expect(m.startsWith("101")).toBe(true);
    expect(m.slice(45, 50)).toBe("01010");
    expect(m.endsWith("101")).toBe(true);
    expect(barcodeSvg("4006381333931")).toContain("<svg");
    expect(barcodeSvg("123")).toBeNull();
  });
});

describe("stock", () => {
  it("classifies status", () => {
    expect(stockStatus(0, 5)).toBe("out");
    expect(stockStatus(5, 5)).toBe("low");
    expect(stockStatus(6, 5)).toBe("ok");
    expect(stockStatus(1, 0)).toBe("ok");
  });
  it("groups order sheets by supplier", () => {
    const p = { name: "Bolt", variant_label: "", sku: "B1" } as InvProduct;
    const lines = [
      { product: p, stock: 1, suggested_qty: 10, supplier_id: "s", supplier_name: "Acme", supplier_sku: "A-1", unit_cost: 2 },
      { product: p, stock: 1, suggested_qty: 1, supplier_id: "", supplier_name: "", supplier_sku: "", unit_cost: 3 },
    ] as ReorderLine[];
    const t = orderSheets(lines);
    expect(t).toContain("Order for Acme");
    expect(t).toContain("10 x Bolt [A-1] @ 2.00 = 20.00");
    expect(t).toContain("Order for No supplier");
  });
});
