/**
 * Assistant tools for Sheets (reference set, see docs/ai.md).
 *
 * The tools talk to the open workbook only through a SheetsAdapter, so they
 * can be tested without FortuneSheet and extended by the Sheets app.
 * fortuneAdapter.ts implements the adapter for the current editor. Optional
 * adapter methods (chart, table, filter, pivot) hide their tool when missing.
 */
import type { AssistantTool, ToolOutcome } from "@/lib/ai";

export interface Rect {
  r1: number;
  c1: number;
  r2: number;
  c2: number;
}
export type CellInput = string | number | boolean | null;

export interface CellFormat {
  bold?: boolean;
  italic?: boolean;
  color?: string;
  background?: string;
  numberFormat?: string;
  align?: "left" | "center" | "right";
}

export interface SheetsAdapter {
  /** Name of the active sheet. */
  sheetName(): string;
  /** Bounds of the used area of the active sheet (null when empty). */
  usedRange(): Rect | null;
  /** Display values and formulas ("=..." or null) of a rectangle. */
  read(r: Rect): { values: CellInput[][]; formulas: (string | null)[][] };
  /** Writes a block at (r, c) as ONE edit; strings starting with "=" are formulas. Returns undo. */
  write(r: number, c: number, block: CellInput[][]): () => void;
  /** Applies a format to a rectangle. Returns undo. */
  format?(r: Rect, f: CellFormat): () => void;
  /** Sorts rows of r by an absolute column. Returns undo, or an error message. */
  sort?(r: Rect, column: number, ascending: boolean, header: boolean): (() => void) | string;
  addChart?(spec: { type: string; range: Rect; title: string }): { undo: () => void };
  createTable?(r: Rect, hasHeaders: boolean): (() => void) | string;
  /** Filter: show rows where column (absolute) matches op/value. Returns undo. */
  filter?(r: Rect, column: number, op: string, value: string): () => void;
  createPivot?(spec: { source: Rect; rows: number[]; values: { column: number; agg: string }[] }): (() => void) | string;
}

// ---- A1 helpers ----------------------------------------------------------------------------

export function colName(c: number): string {
  let s = "";
  c += 1;
  while (c > 0) {
    const m = (c - 1) % 26;
    s = String.fromCharCode(65 + m) + s;
    c = Math.floor((c - 1) / 26);
  }
  return s;
}

function colIndex(s: string): number {
  let n = 0;
  for (const ch of s.toUpperCase()) n = n * 26 + (ch.charCodeAt(0) - 64);
  return n - 1;
}

const CELL = /^\$?([A-Za-z]{1,3})\$?(\d{1,7})$/;
const MAX_CELLS = 5000;

/** Parses "A1", "B2:D9", "Sheet1!A1:B2" (sheet part ignored, active sheet only). */
export function parseRange(a1: string): Rect | null {
  const s = a1.includes("!") ? a1.slice(a1.lastIndexOf("!") + 1) : a1;
  const [a, b = a] = s.trim().split(":");
  const m1 = CELL.exec(a);
  const m2 = CELL.exec(b);
  if (!m1 || !m2) return null;
  const r1 = Number(m1[2]) - 1, c1 = colIndex(m1[1]), r2 = Number(m2[2]) - 1, c2 = colIndex(m2[1]);
  if (r1 < 0 || r2 < 0) return null;
  return { r1: Math.min(r1, r2), c1: Math.min(c1, c2), r2: Math.max(r1, r2), c2: Math.max(c1, c2) };
}

export function a1(r: Rect): string {
  const tl = `${colName(r.c1)}${r.r1 + 1}`;
  return r.r1 === r.r2 && r.c1 === r.c2 ? tl : `${tl}:${colName(r.c2)}${r.r2 + 1}`;
}

const size = (r: Rect) => (r.r2 - r.r1 + 1) * (r.c2 - r.c1 + 1);

function needRange(v: unknown, what = "range"): Rect | string {
  const r = typeof v === "string" ? parseRange(v) : null;
  if (!r) return `${what} must be an A1 range like B2:D10`;
  if (size(r) > MAX_CELLS) return `${what} is larger than ${MAX_CELLS} cells; use a smaller range`;
  return r;
}

function columnOf(v: unknown, r: Rect): number | string {
  if (typeof v !== "string" || !/^[A-Za-z]{1,3}$/.test(v)) return "column must be a column letter like B";
  const c = colIndex(v);
  return c < r.c1 || c > r.c2 ? "column must be inside the range" : c;
}

const fail = (error: string): ToolOutcome => ({ error });

/** Builds the Sheets tool set over an adapter. */
export function createSheetTools(ad: SheetsAdapter): AssistantTool[] {
  const tools: AssistantTool[] = [
    {
      name: "sheet_read_range",
      risk: "read",
      description: "Read the values and formulas of a range on the active sheet. Use the used range when unsure (pass \"used\").",
      parameters: { type: "object", properties: { range: { type: "string", description: "A1 range, e.g. A1:D20, or \"used\"" } }, required: ["range"] },
      label: (a) => `Read ${String(a.range)}`,
      handler: (a) => {
        const r = a.range === "used" ? ad.usedRange() : needRange(a.range);
        if (r === null) return { result: { sheet: ad.sheetName(), empty: true }, summary: "Read the sheet (empty)" };
        if (typeof r === "string") return fail(r);
        const { values, formulas } = ad.read(r);
        const f: Record<string, string> = {};
        formulas.forEach((row, i) => row.forEach((x, j) => { if (x) f[`${colName(r.c1 + j)}${r.r1 + i + 1}`] = x; }));
        return { result: { sheet: ad.sheetName(), range: a1(r), values, formulas: f }, summary: `Read ${a1(r)}` };
      },
    },
    {
      name: "sheet_write_cells",
      risk: "write",
      description: "Write a block of values starting at a cell. Each inner array is one row. Strings starting with = are formulas (e.g. \"=SUM(B2:B9)\"). Overwrites what is there.",
      parameters: {
        type: "object",
        properties: {
          start: { type: "string", description: "Top-left cell, e.g. A1" },
          values: { type: "array", items: { type: "array" }, description: "Rows of cell values" },
        },
        required: ["start", "values"],
      },
      label: (a) => `Write cells at ${String(a.start)}`,
      handler: (a) => {
        const at = typeof a.start === "string" ? parseRange(a.start) : null;
        if (!at) return fail("start must be a cell like A1");
        const rows = a.values as unknown[][];
        if (!rows.length || rows.some((r) => !Array.isArray(r))) return fail("values must be a list of rows");
        const width = Math.max(...rows.map((r) => r.length));
        if (rows.length * width > MAX_CELLS) return fail(`write at most ${MAX_CELLS} cells at a time`);
        const block: CellInput[][] = rows.map((r) =>
          Array.from({ length: width }, (_, j) => {
            const v = r[j];
            return v === undefined || v === null || typeof v === "object" ? null : (v as CellInput);
          }),
        );
        const rect = { r1: at.r1, c1: at.c1, r2: at.r1 + rows.length - 1, c2: at.c1 + width - 1 };
        const undo = ad.write(at.r1, at.c1, block);
        return { result: { wrote: a1(rect), cells: rows.length * width }, undo, summary: `Wrote ${rows.length * width} cells in ${a1(rect)}` };
      },
    },
    {
      name: "sheet_set_formula",
      risk: "write",
      description: "Put one formula into one cell.",
      parameters: { type: "object", properties: { cell: { type: "string" }, formula: { type: "string", description: "Starts with =" } }, required: ["cell", "formula"] },
      label: (a) => `Set formula in ${String(a.cell)}`,
      handler: (a) => {
        const at = parseRange(String(a.cell));
        const f = String(a.formula).trim();
        if (!at || at.r1 !== at.r2 || at.c1 !== at.c2) return fail("cell must be one cell like C10");
        if (!f.startsWith("=") || f.length > 2000) return fail("formula must start with =");
        const undo = ad.write(at.r1, at.c1, [[f]]);
        return { result: { cell: a1(at), formula: f }, undo, summary: `Set ${a1(at)} to ${f}` };
      },
    },
    {
      name: "sheet_clear_range",
      risk: "destructive",
      description: "Delete the contents of a range. Ask only when the user wants cells cleared.",
      parameters: { type: "object", properties: { range: { type: "string" } }, required: ["range"] },
      label: (a) => `Clear ${String(a.range)}`,
      handler: (a) => {
        const r = needRange(a.range);
        if (typeof r === "string") return fail(r);
        const empty = Array.from({ length: r.r2 - r.r1 + 1 }, () => Array<CellInput>(r.c2 - r.c1 + 1).fill(null));
        const undo = ad.write(r.r1, r.c1, empty);
        return { result: { cleared: a1(r) }, undo, summary: `Cleared ${a1(r)}` };
      },
    },
    {
      name: "sheet_explain_formula",
      risk: "read",
      description: "Get a cell's formula and the values of the cells it refers to, to explain it to the user.",
      parameters: { type: "object", properties: { cell: { type: "string" } }, required: ["cell"] },
      label: (a) => `Look at the formula in ${String(a.cell)}`,
      handler: (a) => {
        const at = parseRange(String(a.cell));
        if (!at) return fail("cell must be like C10");
        const { values, formulas } = ad.read({ r1: at.r1, c1: at.c1, r2: at.r1, c2: at.c1 });
        const f = formulas[0]?.[0];
        if (!f) return { result: { cell: a1(at), value: values[0]?.[0] ?? null, formula: null } };
        const refs: Record<string, unknown> = {};
        for (const m of f.matchAll(/\$?[A-Z]{1,3}\$?\d+(?::\$?[A-Z]{1,3}\$?\d+)?/g)) {
          const r = parseRange(m[0].replace(/\$/g, ""));
          if (r && size(r) <= 50) refs[m[0]] = ad.read(r).values;
          if (Object.keys(refs).length >= 10) break;
        }
        return { result: { cell: a1(at), formula: f, value: values[0]?.[0] ?? null, references: refs }, summary: `Read the formula in ${a1(at)}` };
      },
    },
  ];
  const fmt = ad.format?.bind(ad);
  if (fmt) {
    tools.push({
      name: "sheet_format_range",
      risk: "write",
      description: "Format a range: bold, italic, text color, background color (hex like #126C5A), number format (e.g. \"#,##0.00\", \"0%\", \"yyyy-mm-dd\"), alignment.",
      parameters: {
        type: "object",
        properties: {
          range: { type: "string" },
          bold: { type: "boolean" },
          italic: { type: "boolean" },
          color: { type: "string" },
          background: { type: "string" },
          numberFormat: { type: "string" },
          align: { type: "string", enum: ["left", "center", "right"] },
        },
        required: ["range"],
      },
      label: (a) => `Format ${String(a.range)}`,
      handler: (a) => {
        const r = needRange(a.range);
        if (typeof r === "string") return fail(r);
        const hex = /^#[0-9a-fA-F]{6}$/;
        for (const k of ["color", "background"] as const) if (a[k] !== undefined && !hex.test(String(a[k]))) return fail(`${k} must be a hex colour like #126C5A`);
        const f: CellFormat = { bold: a.bold as boolean, italic: a.italic as boolean, color: a.color as string, background: a.background as string, numberFormat: a.numberFormat as string, align: a.align as CellFormat["align"] };
        return { result: { formatted: a1(r) }, undo: fmt(r, f), summary: `Formatted ${a1(r)}` };
      },
    });
  }
  const sort = ad.sort?.bind(ad);
  if (sort) {
    tools.push({
      name: "sheet_sort_range",
      risk: "write",
      description: "Sort the rows of a range by one column.",
      parameters: { type: "object", properties: { range: { type: "string" }, column: { type: "string", description: "Column letter inside the range" }, ascending: { type: "boolean" }, header: { type: "boolean", description: "First row is a header" } }, required: ["range", "column"] },
      label: (a) => `Sort ${String(a.range)} by ${String(a.column)}`,
      handler: (a) => {
        const r = needRange(a.range);
        if (typeof r === "string") return fail(r);
        const c = columnOf(a.column, r);
        if (typeof c === "string") return fail(c);
        const res = sort(r, c, a.ascending !== false, a.header === true);
        return typeof res === "string" ? fail(res) : { result: { sorted: a1(r) }, undo: res, summary: `Sorted ${a1(r)} by ${String(a.column)}` };
      },
    });
  }
  const filter = ad.filter?.bind(ad);
  if (filter) {
    tools.push({
      name: "sheet_filter_range",
      risk: "write",
      description: "Filter a range (header row first) to show only rows where a column matches. Rows are hidden, not deleted.",
      parameters: {
        type: "object",
        properties: {
          range: { type: "string" },
          column: { type: "string" },
          op: { type: "string", enum: ["equals", "doesNotEqual", "isGreaterThan", "isLessThan", "contains", "beginsWith"] },
          value: { type: "string" },
        },
        required: ["range", "column", "op", "value"],
      },
      label: (a) => `Filter ${String(a.range)}`,
      handler: (a) => {
        const r = needRange(a.range);
        if (typeof r === "string") return fail(r);
        const c = columnOf(a.column, r);
        if (typeof c === "string") return fail(c);
        return { result: { filtered: a1(r) }, undo: filter(r, c, String(a.op), String(a.value)), summary: `Filtered ${a1(r)} where ${String(a.column)} ${String(a.op)} ${String(a.value)}` };
      },
    });
  }
  const chart = ad.addChart?.bind(ad);
  if (chart) {
    tools.push({
      name: "sheet_create_chart",
      risk: "write",
      description: "Create a chart from a range. The first row holds series names and the first column category labels.",
      parameters: { type: "object", properties: { range: { type: "string" }, type: { type: "string", enum: ["column", "bar", "line", "area", "pie", "doughnut", "scatter"] }, title: { type: "string" } }, required: ["range", "type"] },
      label: (a) => `Create a ${String(a.type)} chart`,
      handler: (a) => {
        const r = needRange(a.range);
        if (typeof r === "string") return fail(r);
        const { undo } = chart({ type: String(a.type), range: r, title: String(a.title ?? "").slice(0, 120) });
        return { result: { chart: String(a.type), range: a1(r) }, undo, summary: `Added a ${String(a.type)} chart of ${a1(r)}` };
      },
    });
  }
  const table = ad.createTable?.bind(ad);
  if (table) {
    tools.push({
      name: "sheet_create_table",
      risk: "write",
      description: "Format a range as a table with header buttons (sort and filter).",
      parameters: { type: "object", properties: { range: { type: "string" }, hasHeaders: { type: "boolean" } }, required: ["range"] },
      label: (a) => `Make ${String(a.range)} a table`,
      handler: (a) => {
        const r = needRange(a.range);
        if (typeof r === "string") return fail(r);
        const res = table(r, a.hasHeaders !== false);
        return typeof res === "string" ? fail(res) : { result: { table: a1(r) }, undo: res, summary: `Made ${a1(r)} a table` };
      },
    });
  }
  const pivot = ad.createPivot?.bind(ad);
  if (pivot) {
    tools.push({
      name: "sheet_create_pivot",
      risk: "write",
      description: "Create a pivot table from a source range (header row first): group by row columns and aggregate value columns.",
      parameters: {
        type: "object",
        properties: {
          source: { type: "string" },
          rows: { type: "array", items: { type: "string" }, description: "Column letters to group by" },
          values: { type: "array", items: { type: "object", properties: { column: { type: "string" }, agg: { type: "string", enum: ["sum", "count", "average", "min", "max"] } }, required: ["column", "agg"] } },
        },
        required: ["source", "rows", "values"],
      },
      label: () => "Create a pivot table",
      handler: (a) => {
        const r = needRange(a.source, "source");
        if (typeof r === "string") return fail(r);
        const rows: number[] = [];
        for (const x of a.rows as string[]) {
          const c = columnOf(x, r);
          if (typeof c === "string") return fail(c);
          rows.push(c);
        }
        const vals: { column: number; agg: string }[] = [];
        for (const v of a.values as { column: string; agg: string }[]) {
          const c = columnOf(v.column, r);
          if (typeof c === "string") return fail(c);
          vals.push({ column: c, agg: v.agg });
        }
        const res = pivot({ source: r, rows, values: vals });
        return typeof res === "string" ? fail(res) : { result: { pivot: a1(r) }, undo: res, summary: `Created a pivot of ${a1(r)}` };
      },
    });
  }
  return tools;
}

/** Selection chip + context text for the panel. */
export function sheetContext(ad: SheetsAdapter, sel: Rect | null): { label: string; text?: string } {
  const name = ad.sheetName();
  if (!sel) return { label: name };
  const r = size(sel) > 400 ? { ...sel, r2: sel.r1 + Math.floor(400 / (sel.c2 - sel.c1 + 1)) } : sel;
  const { values } = ad.read(r);
  const text = values.map((row) => row.map((v) => (v === null ? "" : String(v))).join("\t")).join("\n");
  return { label: `${name}!${a1(sel)}`, text: text.trim() ? `Cells ${a1(r)} (tab separated):\n${text}` : undefined };
}
