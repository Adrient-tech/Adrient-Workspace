import { describe, expect, it } from "vitest";
import { activeCallAt, hintArgIndex } from "./formulaHint";

describe("activeCallAt", () => {
  it("finds the innermost open call and argument", () => {
    expect(activeCallAt("=SUM(A1,")).toEqual({ name: "SUM", argIndex: 1 });
    expect(activeCallAt("=IF(A1>1, SUM(B1:B3, ")).toEqual({ name: "SUM", argIndex: 1 });
    expect(activeCallAt("=IF(SUM(1,2), ")).toEqual({ name: "IF", argIndex: 1 });
  });
  it("ignores commas in strings and non-formulas", () => {
    expect(activeCallAt('=CONCAT("a,b", ')).toEqual({ name: "CONCAT", argIndex: 1 });
    expect(activeCallAt("SUM(1,")).toBeNull();
    expect(activeCallAt("=1+2")).toBeNull();
    expect(activeCallAt("=SUM(1,2)")).toBeNull();
  });
  it("honours the caret", () => {
    expect(activeCallAt("=ROUND(A1, 2)", 8)).toEqual({ name: "ROUND", argIndex: 0 });
  });
  it("skips bare parentheses", () => {
    expect(activeCallAt("=(1+SUM(2,")).toEqual({ name: "SUM", argIndex: 1 });
  });
});

describe("hintArgIndex", () => {
  it("clamps repeating tails", () => {
    expect(hintArgIndex([{}, { repeat: true }], 5)).toBe(1);
    expect(hintArgIndex([{}, {}], 5)).toBe(-1);
    expect(hintArgIndex([{}, {}], 1)).toBe(1);
  });
});
