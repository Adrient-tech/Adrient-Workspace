import { cellText, type Cell, type CellGrid } from "./sheetOps";

// Data ▸ Data cleanup: pure grid transforms (remove duplicates, trim
// whitespace).

export interface DedupeResult {
  rows: CellGrid;
  removed: number;
  kept: number;
}

/**
 * removeDuplicateRows keeps the first row for every distinct combination of
 * the `keyCols` (offsets within the block, compared case-insensitively after
 * trimming, like Sheets). With `header`, row 0 is kept untouched. Blank key
 * values still compare equal to each other.
 */
export function removeDuplicateRows(block: CellGrid, keyCols: number[], header: boolean, caseSensitive = false): DedupeResult {
  const seen = new Set<string>();
  const out: CellGrid = [];
  let removed = 0;
  block.forEach((row, i) => {
    if (header && i === 0) {
      out.push(row);
      return;
    }
    const key = JSON.stringify(
      keyCols.map((c) => {
        const t = cellText(row[c] ?? null).trim();
        return caseSensitive ? t : t.toLowerCase();
      }),
    );
    if (seen.has(key)) {
      removed++;
      return;
    }
    seen.add(key);
    out.push(row);
  });
  return { rows: out, removed, kept: out.length - (header && block.length ? 1 : 0) };
}

/** trimCell collapses runs of whitespace to one space and trims the ends of text cells. */
export function trimCell(cell: Cell): { cell: Cell; changed: boolean } {
  if (!cell || typeof cell.v !== "string" || cell.f) return { cell, changed: false };
  const t = cell.v.replace(/\s+/g, " ").trim();
  if (t === cell.v) return { cell, changed: false };
  return { cell: { ...cell, v: t, m: t }, changed: true };
}

export function trimGrid(block: CellGrid): { rows: CellGrid; changed: number } {
  let changed = 0;
  const rows = block.map((row) =>
    row.map((c) => {
      const r = trimCell(c);
      if (r.changed) changed++;
      return r.cell;
    }),
  );
  return { rows, changed };
}
