import { describe, expect, it } from "vitest";
import { removeDuplicateRows, trimGrid } from "./dataCleanup";

const c = (v: string | number) => ({ v, m: String(v) });

describe("removeDuplicateRows", () => {
  const block = [
    [c("Name"), c("City")],
    [c("Ann"), c("Pune")],
    [c(" ann "), c("Pune")],
    [c("Ann"), c("Goa")],
    [c("Bob"), c("Pune")],
  ];
  it("dedupes on all columns, keeps header and first occurrence", () => {
    const r = removeDuplicateRows(block, [0, 1], true);
    expect(r.removed).toBe(1);
    expect(r.rows.map((x) => x[0]?.v)).toEqual(["Name", "Ann", "Ann", "Bob"]);
  });
  it("dedupes on a key column only", () => {
    const r = removeDuplicateRows(block, [0], true);
    expect(r.removed).toBe(2);
    expect(r.kept).toBe(2);
  });
  it("is case sensitive on request", () => {
    const b = [[c("a")], [c("A")]];
    expect(removeDuplicateRows(b, [0], false, true).removed).toBe(0);
    expect(removeDuplicateRows(b, [0], false).removed).toBe(1);
  });
});

describe("trimGrid", () => {
  it("trims text cells only", () => {
    const r = trimGrid([[c("  a   b "), c(5), null, { v: " x", f: "=A1" }]]);
    expect(r.changed).toBe(1);
    expect(r.rows[0][0]?.v).toBe("a b");
    expect(r.rows[0][3]?.v).toBe(" x");
  });
});
