import { describe, expect, it } from "vitest";
import { autoSumSelection, proposeSum, type CellKind } from "../autoSum";
import { formatKind, parseInput } from "../numberFormat";
import { parseA1Range, rectToA1 } from "../cellValue";
import type { CellRect } from "../cellRange";
import fixture from "./autoSum.fixture.json";

// AutoSum: cells, selections and AutoSum results replayed against
// autoSum.ts. Checks Adrient answers differently are listed in PENDING with
// the reason (the case index).

type Op =
  | { op: "set"; ref: string; v: string }
  | { op: "clean"; ref: string }
  | { op: "fmt"; ref: string; fmt: string }
  | { op: "select"; ref: string }
  | { op: "autosum" }
  | { op: "expectFormula"; ref: string; f: string }
  | { op: "expectSelection"; ref: string };

const HEADER_ONE = "known-diff/header-one-number: the baseline writes nothing for a text header over a single number; Adrient (like Excel) sums the number below the selection";
const TRAILING = "known-diff/trailing-empty-columns: with two empty columns after the data the baseline writes no row totals; Adrient puts them in the selection's last column";
const PARTIAL = "known-diff/partial-row-totals: the baseline fills only the second row's total and no corner; Adrient totals every row and the corner";
const PENDING: Record<number, string> = {
  13: HEADER_ONE,
  14: HEADER_ONE,
  225: TRAILING,
  226: TRAILING,
  233: TRAILING,
  234: TRAILING,
  248: PARTIAL,
  249: PARTIAL,
};

interface Cell {
  v?: string;
  fmt?: string;
  f?: string;
}

function replay(ops: Op[]) {
  const grid = new Map<string, Cell>();
  const key = (r: number, c: number) => `${r},${c}`;
  const each = (ref: string, fn: (r: number, c: number) => void) => {
    const rect = parseA1Range(ref)!;
    for (let r = rect.r1; r <= rect.r2; r++) for (let c = rect.c1; c <= rect.c2; c++) fn(r, c);
  };
  const kind = (r: number, c: number): CellKind => {
    const cell = grid.get(key(r, c));
    if (!cell) return "empty";
    if (cell.f) return "sum";
    if (cell.v == null || cell.v === "") return "empty";
    if (cell.fmt === "@") return "text";
    const parsed = parseInput(cell.v);
    if (!parsed || typeof parsed.value !== "number") return "text";
    const fk = formatKind(cell.fmt ?? parsed.format);
    return fk === "date" || fk === "time" ? "date" : "num";
  };
  let sel: CellRect = { r1: 0, c1: 0, r2: 0, c2: 0 };
  const failures: string[] = [];
  let pendingHit = 0;
  ops.forEach((o, i) => {
    switch (o.op) {
      case "set":
        each(o.ref, (r, c) => grid.set(key(r, c), { ...grid.get(key(r, c)), v: o.v, f: undefined }));
        break;
      case "clean":
        each(o.ref, (r, c) => grid.delete(key(r, c)));
        break;
      case "fmt":
        each(o.ref, (r, c) => grid.set(key(r, c), { ...grid.get(key(r, c)), fmt: o.fmt }));
        break;
      case "select":
        sel = parseA1Range(o.ref)!;
        break;
      case "autosum": {
        if (sel.r1 === sel.r2 && sel.c1 === sel.c2) {
          proposeSum(kind, sel.r1, sel.c1); // one cell: the editor opens with the proposal
          break;
        }
        const res = autoSumSelection(kind, sel);
        if (res) {
          for (const w of res.writes) grid.set(key(w.r, w.c), { ...grid.get(key(w.r, w.c)), f: w.f, v: "" });
          sel = res.selection;
        }
        break;
      }
      case "expectFormula":
      case "expectSelection": {
        const rect = parseA1Range(o.ref)!;
        const got = o.op === "expectFormula" ? grid.get(key(rect.r1, rect.c1))?.f ?? "" : rectToA1(sel);
        const want = o.op === "expectFormula" ? o.f : o.ref;
        if (got !== want) {
          if (PENDING[i]) pendingHit++;
          else failures.push(`#${i} ${o.op} ${o.ref}: got ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
        }
        break;
      }
    }
  });
  return { failures, pendingHit };
}

describe("AutoSum", () => {
  it("autoCompleteFormula", () => {
    const { failures, pendingHit } = replay(fixture.ops as Op[]);
    expect(failures).toEqual([]);
    expect(pendingHit).toBe(Object.keys(PENDING).length);
  });
});
