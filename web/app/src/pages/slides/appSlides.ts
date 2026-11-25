// Insert ▸ From app on slides: turns rows from other Adrient modules (the
// shared sheets/appData sources) into a table or a simple chart element.
import type { AppSource, AppTable } from "../sheets/appData";
import { newChartElement } from "./chartElement";
import type { SlideElement } from "./model";
import { newTableElement } from "./tableOps";

/** Rows / columns that fit legibly on one slide. */
export const MAX_SLIDE_ROWS = 12;
export const MAX_SLIDE_COLS = 6;

const fmt = (v: string | number): string =>
  typeof v === "number" ? v.toLocaleString("en-US", { maximumFractionDigits: 2 }) : v;

/** A table element holding the header plus up to `maxRows` data rows. */
export function appTableElement(
  tbl: AppTable,
  maxRows = MAX_SLIDE_ROWS,
  maxCols = MAX_SLIDE_COLS,
): { el: SlideElement; shown: number; total: number } {
  const header = tbl.header.slice(0, maxCols);
  const rows = tbl.rows.slice(0, maxRows);
  const el = newTableElement(rows.length + 1, Math.max(1, header.length), { x: 40, y: 80 });
  const w = 880;
  const rowH = Math.min(40, Math.floor(400 / (rows.length + 1)));
  el.w = w;
  el.h = rowH * (rows.length + 1);
  el.fontSize = rows.length > 8 ? 12 : 14;
  if (el.table) {
    el.table.cells = [header.map(String), ...rows.map((r) => header.map((_, i) => fmt(r[i] ?? "")))];
  }
  return { el, shown: rows.length, total: tbl.rows.length };
}

export interface ChartSummary {
  title: string;
  /** Chart data sheet: row 0 = ["", series name], then [category, value]. */
  data: string[][];
  type: "column" | "pie" | "line";
}

const col = (tbl: AppTable, name: string) => tbl.header.indexOf(name);
const num = (v: unknown) => (typeof v === "number" ? v : Number(v) || 0);

function groupSum(tbl: AppTable, keyCol: string, valCol: string, keyFn?: (s: string) => string) {
  const k = col(tbl, keyCol);
  const v = col(tbl, valCol);
  const sums = new Map<string, number>();
  if (k < 0 || v < 0) return sums;
  for (const r of tbl.rows) {
    const key = (keyFn ? keyFn(String(r[k] ?? "")) : String(r[k] ?? "")) || "(none)";
    sums.set(key, (sums.get(key) ?? 0) + num(r[v]));
  }
  return sums;
}

const round = (n: number) => String(Math.round(n * 100) / 100);

/** A summary chart for a source: inventory stock by category, POS sales by
 *  month, invoice totals by status, CRM pipeline value by stage. */
export function summaryChart(src: AppSource["id"], tbl: AppTable): ChartSummary | null {
  let sums: Map<string, number>;
  let title: string;
  let series: string;
  let type: ChartSummary["type"] = "column";
  switch (src) {
    case "inventory": {
      sums = new Map();
      const c = col(tbl, "Category");
      const s = col(tbl, "Stock");
      if (c >= 0 && s >= 0)
        for (const r of tbl.rows) {
          const key = String(r[c] ?? "") || "(none)";
          sums.set(key, (sums.get(key) ?? 0) + num(r[s]));
        }
      title = "Stock by category";
      series = "Units";
      break;
    }
    case "pos":
      sums = groupSum(tbl, "Date", "Total", (d) => d.slice(0, 7));
      title = "POS sales by month";
      series = "Sales";
      type = "line";
      break;
    case "invoicing":
      sums = groupSum(tbl, "Status", "Total");
      title = "Invoice total by status";
      series = "Total";
      type = "pie";
      break;
    case "crm":
      sums = groupSum(tbl, "Stage", "Value");
      title = "Pipeline value by stage";
      series = "Value";
      break;
  }
  if (sums.size === 0) return null;
  let entries = [...sums.entries()];
  if (src === "pos") entries.sort((a, b) => a[0].localeCompare(b[0]));
  else entries.sort((a, b) => b[1] - a[1]);
  entries = entries.slice(0, 12);
  return { title, type, data: [["", series], ...entries.map(([k, v]) => [k, round(v)])] };
}

/** A chart element for a summary. */
export function summaryChartElement(s: ChartSummary): SlideElement {
  const el = newChartElement(s.type, { x: 120, y: 90 });
  el.w = 720;
  el.h = 400;
  if (el.chart) {
    el.chart.title = s.title;
    el.chart.data = s.data;
  }
  return el;
}
