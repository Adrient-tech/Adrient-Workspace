/* eslint-disable @typescript-eslint/no-explicit-any -- FortuneSheet's ref API and cells are loosely typed. */
/**
 * SheetsAdapter over the FortuneSheet workbook ref used by SheetEditor.
 * Each write goes through batchCallApis (one undo step, relayed to
 * collaborators as ordinary ops). Undo restores the cells captured before
 * the change; edits made to the same cells afterwards are overwritten by it.
 */
import { cellScalar, isError } from "../cellValue";
import type { ChartConfig, ChartType } from "../chartData";
import { cellGetter, selectionRect, sortRangeWith, writeCells } from "../editActions";
import { applyNumberFormat } from "../numberFormatActions";
import { applyFilter, currentSheet, sheetFilter } from "../sheetDataTools";
import { insertTable, saveTable, workbookTables } from "../tableTools";
import type { CellFormat, CellInput, Rect, SheetsAdapter } from "./tools";

type Wb = any;

function sheetId(w: Wb): string {
  return String(currentSheet(w)?.id ?? "");
}

function snapshot(w: Wb, r: Rect): { r: number; c: number; cell: any }[] {
  const get = cellGetter(w);
  const out: { r: number; c: number; cell: any }[] = [];
  for (let i = r.r1; i <= r.r2; i++) for (let j = r.c1; j <= r.c2; j++) {
    const cell = get(i, j);
    out.push({ r: i, c: j, cell: cell ? JSON.parse(JSON.stringify(cell)) : null });
  }
  return out;
}

function restoreValues(w: Wb, id: string, cells: { r: number; c: number; cell: any }[]) {
  // Clear first (clearCell also drops formulas, setCellValue(null) does not), then put the old values back.
  w.batchCallApis(cells.map((x) => ({ name: "clearCell", args: [x.r, x.c, { id }] })));
  const filled = cells.filter((x) => x.cell != null && (x.cell.v != null || x.cell.f != null));
  if (filled.length) writeCells(w, filled, id);
}

const FORMAT_ATTRS = ["bl", "it", "fc", "bg", "ht"] as const;

export interface FortuneAdapterOptions {
  getWb: () => Wb;
  /** Adds or replaces a chart (SheetEditor.upsertChart). */
  upsertChart?: (cfg: ChartConfig) => void;
  /** Removes a chart by id. */
  removeChart?: (id: string) => void;
}

export function fortuneAdapter(o: FortuneAdapterOptions): SheetsAdapter {
  const wb = () => {
    const w = o.getWb();
    if (!w) throw new Error("The sheet is not ready yet.");
    return w;
  };
  const ad: SheetsAdapter = {
    sheetName: () => String(currentSheet(wb())?.name ?? "Sheet1"),
    usedRange: () => {
      const data: any[][] = currentSheet(wb())?.data ?? [];
      let r2 = -1, c2 = -1;
      data.forEach((row, i) => row?.forEach((cell: any, j: number) => {
        if (cellScalar(cell) !== null || cell?.f) {
          r2 = Math.max(r2, i);
          c2 = Math.max(c2, j);
        }
      }));
      return r2 < 0 ? null : { r1: 0, c1: 0, r2: Math.min(r2, 199), c2: Math.min(c2, 25) };
    },
    read: (r) => {
      const get = cellGetter(wb());
      const values: CellInput[][] = [];
      const formulas: (string | null)[][] = [];
      for (let i = r.r1; i <= r.r2; i++) {
        const vr: CellInput[] = [];
        const fr: (string | null)[] = [];
        for (let j = r.c1; j <= r.c2; j++) {
          const cell = get(i, j);
          const s = cellScalar(cell);
          vr.push(isError(s) ? String(cell?.v ?? "#ERROR") : (s as CellInput));
          fr.push(typeof cell?.f === "string" && cell.f.startsWith("=") ? cell.f : null);
        }
        values.push(vr);
        formulas.push(fr);
      }
      return { values, formulas };
    },
    write: (r, c, block) => {
      const w = wb();
      const id = sheetId(w);
      const rect = { r1: r, c1: c, r2: r + block.length - 1, c2: c + Math.max(...block.map((x) => x.length)) - 1 };
      const data: any[][] = currentSheet(w)?.data ?? [];
      if (rect.r2 >= data.length || rect.c2 >= (data[0]?.length ?? 0)) {
        throw new Error(`The sheet has ${data.length} rows and ${data[0]?.length ?? 0} columns; ${rect.r2 + 1}x${rect.c2 + 1} does not fit.`);
      }
      const before = snapshot(w, rect);
      const calls: { name: string; args: unknown[] }[] = [];
      block.forEach((row, i) => row.forEach((v, j) => {
        calls.push(v === null || v === "" ? { name: "clearCell", args: [r + i, c + j, { id }] } : { name: "setCellValue", args: [r + i, c + j, v, null, { id }] });
      }));
      w.batchCallApis(calls);
      return () => restoreValues(wb(), id, before);
    },
    format: (r, f: CellFormat) => {
      const w = wb();
      const id = sheetId(w);
      const before = snapshot(w, r);
      const range = { row: [r.r1, r.r2], column: [r.c1, r.c2] };
      if (f.bold !== undefined) w.setCellFormatByRange("bl", f.bold ? 1 : 0, range, { id });
      if (f.italic !== undefined) w.setCellFormatByRange("it", f.italic ? 1 : 0, range, { id });
      if (f.color) w.setCellFormatByRange("fc", f.color, range, { id });
      if (f.background) w.setCellFormatByRange("bg", f.background, range, { id });
      if (f.align) w.setCellFormatByRange("ht", f.align === "center" ? 0 : f.align === "left" ? 1 : 2, range, { id });
      if (f.numberFormat) applyNumberFormat(w, f.numberFormat, [range]);
      return () => {
        const x = wb();
        for (const b of before) {
          const one = { row: [b.r, b.r], column: [b.c, b.c] };
          for (const attr of FORMAT_ATTRS) x.setCellFormatByRange(attr, b.cell?.[attr], one, { id });
          if (f.numberFormat) x.setCellFormatByRange("ct", b.cell?.ct ?? { fa: "General", t: "g" }, one, { id });
        }
      };
    },
    sort: (r, column, ascending, header) => {
      const w = wb();
      const id = sheetId(w);
      const before = snapshot(w, r);
      if (!sortRangeWith(w, r, { keys: [{ index: column, ascending }], header })) return "Nothing to sort in that range.";
      return () => restoreValues(wb(), id, before);
    },
    filter: (r, column, op, value) => {
      const w = wb();
      const prev = sheetFilter(currentSheet(w));
      applyFilter(w, { range: r, columns: { [String(column - r.c1)]: { type: "custom", and: true, conditions: [{ op: op as never, val: value }] } } });
      return () => applyFilter(wb(), prev ? JSON.parse(JSON.stringify(prev)) : null);
    },
    createTable: (r, hasHeaders) => {
      const w = wb();
      const id = sheetId(w);
      const names = new Set(workbookTables(w).map((t) => t.table.name));
      const before = snapshot(w, { ...r, r1: Math.max(0, r.r1 - 1) });
      const err = insertTable(w, r, hasHeaders, "TableStyleMedium2");
      if (err) return err;
      const made = workbookTables(w).find((t) => !names.has(t.table.name));
      return () => {
        const x = wb();
        if (made) saveTable(x, id, made.table.name, null);
        restoreValues(x, id, before);
      };
    },
  };
  if (o.upsertChart && o.removeChart) {
    const up = o.upsertChart, rm = o.removeChart;
    ad.addChart = ({ type, range, title }) => {
      const cfg: ChartConfig = {
        id: `chart_${Date.now()}_${Math.floor(Math.random() * 1e6)}`,
        type: type as ChartType,
        title,
        range: { r0: range.r1, r1: range.r2, c0: range.c1, c1: range.c2 },
        headerRow: true,
        labelCol: true,
        sheetId: sheetId(wb()),
        anchor: { r: range.r1, c: range.c2 + 2, dx: 0, dy: 0, w: 480, h: 288 },
      };
      up(cfg);
      return { undo: () => rm(cfg.id) };
    };
  }
  return ad;
}

/** The selected rectangle of the active sheet. */
export function fortuneSelection(getWb: () => Wb): Rect | null {
  const r = selectionRect(getWb());
  return r ? { r1: r.r1, c1: r.c1, r2: r.r2, c2: r.c2 } : null;
}
