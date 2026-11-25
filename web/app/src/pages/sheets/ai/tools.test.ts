import { describe, expect, it } from "vitest";
import { a1, colName, createSheetTools, parseRange, sheetContext, type CellInput, type Rect, type SheetsAdapter } from "./tools";

function memSheet() {
  const cells = new Map<string, CellInput>();
  const k = (r: number, c: number) => `${r},${c}`;
  const ad: SheetsAdapter = {
    sheetName: () => "Sheet1",
    usedRange: () => {
      let r2 = -1, c2 = -1;
      for (const key of cells.keys()) {
        const [r, c] = key.split(",").map(Number);
        r2 = Math.max(r2, r);
        c2 = Math.max(c2, c);
      }
      return r2 < 0 ? null : { r1: 0, c1: 0, r2, c2 };
    },
    read: (r: Rect) => {
      const values: CellInput[][] = [], formulas: (string | null)[][] = [];
      for (let i = r.r1; i <= r.r2; i++) {
        const vr: CellInput[] = [], fr: (string | null)[] = [];
        for (let j = r.c1; j <= r.c2; j++) {
          const v = cells.get(k(i, j)) ?? null;
          const f = typeof v === "string" && v.startsWith("=");
          vr.push(f ? 0 : v);
          fr.push(f ? (v as string) : null);
        }
        values.push(vr);
        formulas.push(fr);
      }
      return { values, formulas };
    },
    write: (r, c, block) => {
      const before = new Map<string, CellInput | undefined>();
      block.forEach((row, i) => row.forEach((v, j) => {
        before.set(k(r + i, c + j), cells.get(k(r + i, c + j)));
        if (v === null) cells.delete(k(r + i, c + j));
        else cells.set(k(r + i, c + j), v);
      }));
      return () => before.forEach((v, key) => (v === undefined ? cells.delete(key) : cells.set(key, v)));
    },
  };
  return { cells, ad };
}

const run = async (tools: ReturnType<typeof createSheetTools>, name: string, args: Record<string, unknown>) => {
  const t = tools.find((x) => x.name === name);
  if (!t) throw new Error(`no tool ${name}`);
  return t.handler(args);
};

describe("A1 helpers", () => {
  it("parses and prints ranges", () => {
    expect(parseRange("B2:D10")).toEqual({ r1: 1, c1: 1, r2: 9, c2: 3 });
    expect(parseRange("Sheet1!$A$1")).toEqual({ r1: 0, c1: 0, r2: 0, c2: 0 });
    expect(parseRange("D10:B2")).toEqual({ r1: 1, c1: 1, r2: 9, c2: 3 });
    expect(parseRange("nope")).toBeNull();
    expect(colName(0)).toBe("A");
    expect(colName(25)).toBe("Z");
    expect(colName(26)).toBe("AA");
    expect(a1({ r1: 0, c1: 27, r2: 4, c2: 28 })).toBe("AB1:AC5");
  });
});

describe("sheet tools", () => {
  it("hides tools the adapter cannot do", () => {
    const names = createSheetTools(memSheet().ad).map((t) => t.name);
    expect(names).toEqual(["sheet_read_range", "sheet_write_cells", "sheet_set_formula", "sheet_clear_range", "sheet_explain_formula"]);
  });

  it("writes a budget block with formulas and undoes it exactly", async () => {
    const { cells, ad } = memSheet();
    cells.set("0,0", "keep me");
    const tools = createSheetTools(ad);
    const out = await run(tools, "sheet_write_cells", { start: "A2", values: [["Rent", 25000], ["Total", "=SUM(B2:B2)"]] });
    expect(out.summary).toBe("Wrote 4 cells in A2:B3");
    expect(cells.get("2,1")).toBe("=SUM(B2:B2)");
    out.undo!();
    expect([...cells.entries()]).toEqual([["0,0", "keep me"]]);
  });

  it("validates input", async () => {
    const tools = createSheetTools(memSheet().ad);
    expect((await run(tools, "sheet_write_cells", { start: "zz", values: [[1]] })).error).toContain("cell like A1");
    expect((await run(tools, "sheet_set_formula", { cell: "A1:B2", formula: "=1" })).error).toContain("one cell");
    expect((await run(tools, "sheet_set_formula", { cell: "A1", formula: "SUM(1)" })).error).toContain("start with =");
    expect((await run(tools, "sheet_read_range", { range: "A1:ZZ9999" })).error).toContain("larger than");
  });

  it("reads the used range and explains a formula with its references", async () => {
    const { cells, ad } = memSheet();
    cells.set("0,0", 5);
    cells.set("1,0", 7);
    cells.set("2,0", "=A1+A2");
    const tools = createSheetTools(ad);
    const read = await run(tools, "sheet_read_range", { range: "used" });
    expect(read.result).toMatchObject({ range: "A1:A3", formulas: { A3: "=A1+A2" } });
    const ex = await run(tools, "sheet_explain_formula", { cell: "A3" });
    expect(ex.result).toMatchObject({ formula: "=A1+A2", references: { A1: [[5]], A2: [[7]] } });
  });

  it("clear is destructive and undoable", async () => {
    const { cells, ad } = memSheet();
    cells.set("0,0", 1);
    const tools = createSheetTools(ad);
    expect(tools.find((t) => t.name === "sheet_clear_range")?.risk).toBe("destructive");
    const out = await run(tools, "sheet_clear_range", { range: "A1:B2" });
    expect(cells.size).toBe(0);
    out.undo!();
    expect(cells.get("0,0")).toBe(1);
  });

  it("builds the selection chip", () => {
    const { cells, ad } = memSheet();
    cells.set("0,0", "a");
    expect(sheetContext(ad, { r1: 0, c1: 0, r2: 0, c2: 1 })).toEqual({ label: "Sheet1!A1:B1", text: "Cells A1:B1 (tab separated):\na\t" });
    expect(sheetContext(ad, null)).toEqual({ label: "Sheet1" });
  });
});
