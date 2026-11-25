// Table sort and "formulas-lite" (Word's Layout ▸ Sort / Formula). Pure
// helpers over the ProseMirror table at the selection; the ribbon's Table tab
// calls them.
import type { Editor } from "@tiptap/core";
import type { Node as PMNode } from "@tiptap/pm/model";
import { tableCtx } from "./tables";

export type SortType = "text" | "number" | "date";

/** parseNumber reads "1,234.50", "$12", "(5)" and "12%" as numbers. */
export function parseNumber(s: string): number | null {
  let t = s.trim();
  if (!t) return null;
  let neg = false;
  if (/^\(.*\)$/.test(t)) {
    neg = true;
    t = t.slice(1, -1);
  }
  t = t.replace(/^[^\d\-+.]+/, "").replace(/[%\s]+$/, "").replace(/,/g, "");
  if (!/^[-+]?(\d+\.?\d*|\.\d+)$/.test(t)) return null;
  const n = Number(t);
  return Number.isFinite(n) ? (neg ? -n : n) : null;
}

/** compareCells orders two cell texts for the given sort type; empties last. */
export function compareCells(a: string, b: string, type: SortType): number {
  if (!a.trim() && !b.trim()) return 0;
  if (!a.trim()) return 1;
  if (!b.trim()) return -1;
  if (type === "number") {
    const x = parseNumber(a);
    const y = parseNumber(b);
    if (x === null && y === null) return a.localeCompare(b);
    if (x === null) return 1;
    if (y === null) return -1;
    return x - y;
  }
  if (type === "date") {
    const x = Date.parse(a);
    const y = Date.parse(b);
    if (Number.isNaN(x) || Number.isNaN(y)) return a.localeCompare(b);
    return x - y;
  }
  return a.localeCompare(b, undefined, { sensitivity: "base", numeric: false });
}

/** sortOrder returns the new row order (indexes into `rows`). Stable. */
export function sortOrder(rows: string[], type: SortType, asc: boolean): number[] {
  const idx = rows.map((_, i) => i);
  idx.sort((i, j) => {
    // Empty cells stay last in both directions.
    const ei = !rows[i].trim();
    const ej = !rows[j].trim();
    if (ei || ej) return ei === ej ? i - j : ei ? 1 : -1;
    const c = compareCells(rows[i], rows[j], type);
    return (asc ? c : -c) || i - j;
  });
  return idx;
}

/** sortTable sorts the rows of the table at the selection by the column the
 *  caret is in. A table with merged cells (spans) is refused. */
export function sortTable(editor: Editor, opts: { type?: SortType; asc?: boolean; header?: boolean; column?: number } = {}): boolean {
  const ctx = tableCtx(editor.state);
  if (!ctx) return false;
  const { table, pos, rect } = ctx;
  const header = opts.header ?? true;
  const col = opts.column ?? rect.left;
  const rows: PMNode[] = [];
  table.forEach((r) => rows.push(r));
  for (const r of rows) {
    let spans = false;
    r.forEach((c) => {
      if ((c.attrs.colspan ?? 1) > 1 || (c.attrs.rowspan ?? 1) > 1) spans = true;
    });
    if (spans || r.childCount !== rows[0].childCount) return false;
  }
  const first = header ? 1 : 0;
  const body = rows.slice(first);
  if (body.length < 2 || col >= rows[0].childCount) return false;
  const keys = body.map((r) => r.child(col).textContent);
  const order = sortOrder(keys, opts.type ?? "text", opts.asc ?? true);
  const next = [...rows.slice(0, first), ...order.map((i) => body[i])];
  const tr = editor.state.tr.replaceWith(pos + 1, pos + table.nodeSize - 1, next);
  editor.view.dispatch(tr);
  return true;
}

// ---- formulas ---------------------------------------------------------------

export type TableFn = "SUM" | "AVERAGE" | "COUNT" | "MIN" | "MAX" | "PRODUCT";
export type TableDir = "above" | "left";

/** evalTableFn applies fn to the numeric values in `cells`; null = no numbers. */
export function evalTableFn(fn: TableFn, cells: string[]): number | null {
  const nums = cells.map(parseNumber).filter((n): n is number => n !== null);
  if (!nums.length) return fn === "COUNT" || fn === "SUM" ? 0 : null;
  switch (fn) {
    case "SUM":
      return nums.reduce((a, b) => a + b, 0);
    case "AVERAGE":
      return nums.reduce((a, b) => a + b, 0) / nums.length;
    case "COUNT":
      return nums.length;
    case "MIN":
      return Math.min(...nums);
    case "MAX":
      return Math.max(...nums);
    case "PRODUCT":
      return nums.reduce((a, b) => a * b, 1);
  }
}

/** formatResult rounds to at most `decimals` places and drops trailing zeros. */
export function formatResult(n: number, decimals = 2): string {
  const f = 10 ** decimals;
  const r = Math.round((n + Number.EPSILON) * f) / f;
  return String(Object.is(r, -0) ? 0 : r);
}

/** The cells a formula in the caret's cell refers to: the contiguous run of
 *  cells above (or to the left of) it, stopping at the first empty cell. */
export function formulaRange(rows: string[][], r: number, c: number, dir: TableDir): string[] {
  const out: string[] = [];
  if (dir === "above") {
    for (let i = r - 1; i >= 0; i--) {
      if (!rows[i]?.[c]?.trim() && out.length) break;
      if (rows[i]?.[c] !== undefined) out.push(rows[i][c]);
    }
  } else {
    for (let j = c - 1; j >= 0; j--) {
      if (!rows[r]?.[j]?.trim() && out.length) break;
      if (rows[r]?.[j] !== undefined) out.push(rows[r][j]);
    }
  }
  return out;
}

/** insertTableFormula writes the computed value of fn over the cells above or
 *  to the left of the caret cell into the caret cell (replacing its text).
 *  The value is static text: it is not recalculated when cells change. */
export function insertTableFormula(editor: Editor, fn: TableFn, dir: TableDir): string | null {
  const ctx = tableCtx(editor.state);
  if (!ctx) return null;
  const { table, rect, map, pos } = ctx;
  const text: string[][] = [];
  table.forEach((row) => {
    const cells: string[] = [];
    row.forEach((c) => cells.push(c.textContent));
    text.push(cells);
  });
  const cellPos = map.map[rect.top * map.width + rect.left];
  const cell = table.nodeAt(cellPos);
  if (!cell) return null;
  const val = evalTableFn(fn, formulaRange(text, rect.top, rect.left, dir));
  if (val === null) return null;
  const out = formatResult(val);
  const start = pos + 1 + cellPos; // position of the cell node
  const schema = editor.state.schema;
  const para = schema.nodes.paragraph.create(null, schema.text(out));
  const tr = editor.state.tr.replaceWith(start + 1, start + cell.nodeSize - 1, para);
  editor.view.dispatch(tr);
  return out;
}
