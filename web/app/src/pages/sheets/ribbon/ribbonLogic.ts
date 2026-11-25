/* eslint-disable @typescript-eslint/no-explicit-any -- FortuneSheet API is loosely typed. */
// Pure(ish) logic behind the Sheets ribbon: cell styles, borders, merge variants,
// the format painter and the number-format gallery. Everything works against the
// FortuneSheet workbook handle (`w`) so it can be unit-tested with a mock.

import { formatValue } from "../numberFormat";
import { applyNumberFormat, selectionRanges } from "../numberFormatActions";
import { currentSheet, patchSheet } from "../sheetDataTools";

export type RibbonTabId = "home" | "insert" | "draw" | "layout" | "formulas" | "data" | "review" | "view" | "automate";

/**
 * Tabs the ribbon shows. Draw (needs a pen overlay saved with the sheet) and
 * Automate (needs a safe script sandbox) are not built, so they are hidden
 * rather than faked.
 */
export const RIBBON_TABS: { id: RibbonTabId; label: string; built: boolean }[] = [
  { id: "home", label: "Home", built: true },
  { id: "insert", label: "Insert", built: true },
  { id: "draw", label: "Draw", built: false },
  { id: "layout", label: "Page Layout", built: true },
  { id: "formulas", label: "Formulas", built: true },
  { id: "data", label: "Data", built: true },
  { id: "review", label: "Review", built: true },
  { id: "view", label: "View", built: true },
  { id: "automate", label: "Automate", built: false },
];
export const visibleTabs = () => RIBBON_TABS.filter((t) => t.built);

// ---- cell styles ------------------------------------------------------------------

export interface CellStyle {
  id: string;
  group: "Good, bad and neutral" | "Titles and headings" | "Themed" | "Number";
  label: string;
  fc?: string;
  bg?: string;
  bl?: 0 | 1;
  it?: 0 | 1;
  fs?: number;
  fa?: string;
}

export const CELL_STYLES: CellStyle[] = [
  { id: "normal", group: "Good, bad and neutral", label: "Normal" },
  { id: "good", group: "Good, bad and neutral", label: "Good", fc: "#006100", bg: "#C6EFCE" },
  { id: "bad", group: "Good, bad and neutral", label: "Bad", fc: "#9C0006", bg: "#FFC7CE" },
  { id: "neutral", group: "Good, bad and neutral", label: "Neutral", fc: "#9C5700", bg: "#FFEB9C" },
  { id: "title", group: "Titles and headings", label: "Title", fc: "#1F3864", bl: 1, fs: 18 },
  { id: "heading1", group: "Titles and headings", label: "Heading 1", fc: "#1F3864", bl: 1, fs: 15 },
  { id: "heading2", group: "Titles and headings", label: "Heading 2", fc: "#1F3864", bl: 1, fs: 13 },
  { id: "heading3", group: "Titles and headings", label: "Heading 3", fc: "#1F3864", bl: 1, fs: 11 },
  { id: "total", group: "Titles and headings", label: "Total", bl: 1, bg: "#F2F2F2" },
  { id: "input", group: "Titles and headings", label: "Input", fc: "#3F3F76", bg: "#FFCC99" },
  { id: "calc", group: "Titles and headings", label: "Calculation", fc: "#FA7D00", bg: "#F2F2F2", bl: 1 },
  { id: "note", group: "Titles and headings", label: "Note", fc: "#000000", bg: "#FFFFCC" },
  { id: "accent1", group: "Themed", label: "Accent 1", fc: "#FFFFFF", bg: "#126C5A" },
  { id: "accent2", group: "Themed", label: "Accent 2", fc: "#FFFFFF", bg: "#ED7D31" },
  { id: "accent3", group: "Themed", label: "Accent 3", fc: "#FFFFFF", bg: "#5B7DB1" },
  { id: "accent1-20", group: "Themed", label: "20% Accent 1", fc: "#000000", bg: "#CFE8E2" },
  { id: "comma", group: "Number", label: "Comma", fa: "#,##0.00" },
  { id: "currency", group: "Number", label: "Currency", fa: "$#,##0.00" },
  { id: "percent", group: "Number", label: "Percent", fa: "0.00%" },
];

/** Applies a named cell style to the selection (unset attributes are cleared first). */
export function applyCellStyle(w: any, style: CellStyle): void {
  for (const range of selectionRanges(w)) {
    for (const attr of ["fc", "bg", "bl", "it", "fs"] as const) {
      const v = style[attr];
      w.setCellFormatByRange(attr, v === undefined ? undefined : v, range);
    }
    if (style.fa) applyNumberFormat(w, style.fa, [range]);
    else if (style.id === "normal") applyNumberFormat(w, "General", [range]);
  }
}

// ---- borders ----------------------------------------------------------------------

export type BorderKind = "all" | "outer" | "inner" | "top" | "bottom" | "left" | "right" | "thick-outer" | "none";
export const BORDER_KINDS: { id: BorderKind; label: string }[] = [
  { id: "all", label: "All borders" },
  { id: "outer", label: "Outside borders" },
  { id: "thick-outer", label: "Thick outside borders" },
  { id: "inner", label: "Inside borders" },
  { id: "top", label: "Top border" },
  { id: "bottom", label: "Bottom border" },
  { id: "left", label: "Left border" },
  { id: "right", label: "Right border" },
  { id: "none", label: "No border" },
];

/** FortuneSheet `config.borderInfo` entries for a border kind over ranges. */
export function borderEntries(kind: BorderKind, ranges: { row: number[]; column: number[] }[], color = "#000000") {
  const type: Record<Exclude<BorderKind, "none" | "thick-outer">, string> = {
    all: "border-all",
    outer: "border-outside",
    inner: "border-inside",
    top: "border-top",
    bottom: "border-bottom",
    left: "border-left",
    right: "border-right",
  };
  if (kind === "none") return ranges.map((r) => ({ rangeType: "range", borderType: "border-none", color, style: "1", range: [r] }));
  const thick = kind === "thick-outer";
  return ranges.map((r) => ({
    rangeType: "range",
    borderType: thick ? "border-outside" : type[kind],
    color,
    style: thick ? "8" : "1",
    range: [{ row: [r.row[0], r.row[1]], column: [r.column[0], r.column[1]] }],
  }));
}

export function applyBorder(w: any, kind: BorderKind, color?: string): void {
  const sheet = currentSheet(w);
  if (!sheet?.id) return;
  const ranges = selectionRanges(w);
  if (!ranges.length) return;
  const config = { ...(sheet.config ?? {}) };
  const existing = Array.isArray(config.borderInfo) ? [...config.borderInfo] : [];
  config.borderInfo = [...existing, ...borderEntries(kind, ranges, color)];
  patchSheet(w, sheet.id, { config });
}

// ---- merge variants ---------------------------------------------------------------

export type MergeKind = "all" | "horizontal" | "vertical" | "unmerge";
export const MERGE_KINDS: { id: MergeKind; label: string }[] = [
  { id: "all", label: "Merge all" },
  { id: "horizontal", label: "Merge horizontally" },
  { id: "vertical", label: "Merge vertically" },
  { id: "unmerge", label: "Unmerge cells" },
];

export function applyMerge(w: any, kind: MergeKind): void {
  const ranges = selectionRanges(w);
  if (kind === "unmerge") {
    if (ranges.length) w.cancelMerge(ranges);
    return;
  }
  for (const r of ranges) {
    if (kind === "all") {
      w.mergeCells([r], "merge-all");
    } else if (kind === "horizontal") {
      w.mergeCells([r], "merge-horizontal");
    } else {
      w.mergeCells([r], "merge-vertical");
    }
  }
}

// ---- format painter ---------------------------------------------------------------

const PAINT_ATTRS = ["bl", "it", "un", "cl", "fc", "bg", "fs", "ff", "ht", "vt", "tb"] as const;
export interface CopiedFormat {
  attrs: Partial<Record<(typeof PAINT_ATTRS)[number], any>>;
  ct?: { fa: string; t: string };
}

/** Captures the formatting of the active cell. */
export function captureFormat(w: any): CopiedFormat | null {
  const r = selectionRanges(w)[0];
  if (!r) return null;
  const cell = w.getCellsByRange?.({ row: [r.row[0], r.row[0]], column: [r.column[0], r.column[0]] })?.[0]?.[0] ?? {};
  const attrs: CopiedFormat["attrs"] = {};
  for (const a of PAINT_ATTRS) attrs[a] = cell?.[a];
  return { attrs, ct: cell?.ct?.fa ? { fa: cell.ct.fa, t: cell.ct.t } : undefined };
}

/** Paints a captured format onto the selection. */
export function paintFormat(w: any, f: CopiedFormat): void {
  for (const range of selectionRanges(w)) {
    for (const a of PAINT_ATTRS) w.setCellFormatByRange(a, f.attrs[a], range);
    applyNumberFormat(w, f.ct?.fa ?? "General", [range]);
  }
}

// ---- number-format gallery --------------------------------------------------------

export interface NumberFormatChoice {
  id: string;
  label: string;
  fa: string;
  sample: number;
}
export const NUMBER_FORMAT_GALLERY: NumberFormatChoice[] = [
  { id: "general", label: "General", fa: "General", sample: 1234.5 },
  { id: "number", label: "Number", fa: "#,##0.00", sample: 1234.5 },
  { id: "currency", label: "Currency", fa: "$#,##0.00", sample: 1234.5 },
  { id: "accounting", label: "Accounting", fa: '_($* #,##0.00_);_($* (#,##0.00);_($* "-"??_);_(@_)', sample: 1234.5 },
  { id: "short-date", label: "Short date", fa: "m/d/yyyy", sample: 45293 },
  { id: "long-date", label: "Long date", fa: "dddd, mmmm d, yyyy", sample: 45293 },
  { id: "time", label: "Time", fa: "h:mm:ss AM/PM", sample: 45293.5 },
  { id: "percent", label: "Percentage", fa: "0.00%", sample: 0.1234 },
  { id: "fraction", label: "Fraction", fa: "# ?/?", sample: 1.75 },
  { id: "scientific", label: "Scientific", fa: "0.00E+00", sample: 1234.5 },
  { id: "text", label: "Text", fa: "@", sample: 1234.5 },
];

/** Live preview of a format code for a sample value; "" if the code cannot be rendered. */
export function previewFormat(fa: string, sample: number | string = 1234.5): string {
  try {
    return formatValue(sample, fa);
  } catch {
    return "";
  }
}

/** Increase/decrease decimals on a code (falls back to a number format for General). */
export function stepDecimals(fa: string, dir: 1 | -1): string {
  if (!fa || fa === "General") return dir > 0 ? "0.0" : "0";
  const m = fa.match(/^([^0#]*[0#,]*)(?:\.([0#]*))?(.*)$/);
  if (!m) return fa;
  const dec = (m[2] ?? "").length;
  const next = Math.max(0, Math.min(15, dec + dir));
  return `${m[1]}${next ? "." + "0".repeat(next) : ""}${m[3]}`;
}
