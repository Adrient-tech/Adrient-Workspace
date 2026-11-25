/* eslint-disable @typescript-eslint/no-explicit-any -- FortuneSheet ref API is loosely typed. */
import { removeDuplicateRows, trimGrid } from "./dataCleanup";
import { colName } from "./dataActions";
import { selectionRect } from "./editActions";
import { cellText, type CellGrid } from "./sheetOps";
import { appTableToSheet } from "./appDataActions";
import type { AppTable } from "./appData";

// FortuneSheet glue for Data ▸ Data cleanup and Insert ▸ From app.

type Wb = any;

function notice(kind: "error" | "info", title: string, message: string) {
  window.dispatchEvent(new CustomEvent("adrient-sheet-notice", { detail: { kind, title, message } }));
}

function readBlock(wb: Wb) {
  const rect = selectionRect(wb);
  const grid: CellGrid = wb?.getSheet?.()?.data ?? [];
  if (!rect) return null;
  const block: CellGrid = [];
  for (let r = rect.r1; r <= rect.r2; r++) {
    const row = [];
    for (let c = rect.c1; c <= rect.c2; c++) row.push(grid[r]?.[c] ?? null);
    block.push(row);
  }
  return { rect, block };
}

function writeBlock(wb: Wb, rect: { r1: number; r2: number; c1: number; c2: number }, rows: CellGrid) {
  const height = rect.r2 - rect.r1 + 1;
  const width = rect.c2 - rect.c1 + 1;
  const data = Array.from({ length: height }, (_, i) => Array.from({ length: width }, (_, j) => rows[i]?.[j] ?? null));
  wb.setCellValuesByRange?.(data, { row: [rect.r1, rect.r2], column: [rect.c1, rect.c2] });
}

/** Column letters and first-row text of the current selection, for the dialog. */
export function dedupeColumns(wb: Wb): { columns: string[]; first: string[] } | null {
  const b = readBlock(wb);
  if (!b || b.block.length < 2) return null;
  const columns: string[] = [];
  const first: string[] = [];
  for (let c = b.rect.c1; c <= b.rect.c2; c++) {
    columns.push(colName(c));
    first.push(cellText(b.block[0][c - b.rect.c1]));
  }
  return { columns, first };
}

export function applyRemoveDuplicates(wb: Wb, opts: { header: boolean; keyCols: number[] }) {
  const b = readBlock(wb);
  if (!b) return;
  const res = removeDuplicateRows(b.block, opts.keyCols, opts.header);
  if (res.removed > 0) writeBlock(wb, b.rect, res.rows);
  notice("info", "Remove duplicates", res.removed ? `${res.removed} duplicate row${res.removed === 1 ? "" : "s"} removed; ${res.kept} unique row${res.kept === 1 ? "" : "s"} remain.` : "No duplicates found.");
}

export function applyTrimWhitespace(wb: Wb): boolean {
  const b = readBlock(wb);
  if (!b) {
    notice("error", "Trim whitespace", "Select a range first.");
    return false;
  }
  const res = trimGrid(b.block);
  if (res.changed) writeBlock(wb, b.rect, res.rows);
  notice("info", "Trim whitespace", res.changed ? `${res.changed} cell${res.changed === 1 ? "" : "s"} trimmed.` : "Nothing to trim.");
  return true;
}

export function insertAppTable(wb: Wb, table: AppTable) {
  appTableToSheet(wb, table);
  notice("info", "Insert from app", `${table.rows.length} rows added as a new sheet.`);
}
