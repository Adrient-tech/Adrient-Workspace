// Row and column grouping (Excel's Data ▸ Group / Outline, Sheets' View ▸ Group).
// Groups are stored on the sheet as `adrientOutline`; collapsing a group hides
// its rows or columns through FortuneSheet, expanding shows them again. Groups
// may nest (up to 8 levels like Excel) but may not partly overlap.
/* eslint-disable @typescript-eslint/no-explicit-any -- FortuneSheet API is loosely typed. */
import { currentSheet, patchSheet } from "./sheetDataTools";

export type OutlineAxis = "row" | "col";

export interface OutlineGroup {
  id: string;
  axis: OutlineAxis;
  /** First and last row/column index (0-based, inclusive). */
  start: number;
  end: number;
  collapsed?: boolean;
}

export const MAX_OUTLINE_LEVEL = 8;

export function sheetOutline(sheet: any): OutlineGroup[] {
  return Array.isArray(sheet?.adrientOutline) ? sheet.adrientOutline : [];
}

/** Nesting depth of a group (1 = outermost). */
export function outlineLevel(list: OutlineGroup[], g: OutlineGroup): number {
  return 1 + list.filter((o) => o.id !== g.id && o.axis === g.axis && o.start <= g.start && o.end >= g.end && !(o.start === g.start && o.end === g.end)).length;
}

/** Add a group; an error message when it would partly overlap another group, repeat one, or nest too deep. */
export function addGroup(list: OutlineGroup[], axis: OutlineAxis, start: number, end: number, id = `g${Date.now().toString(36)}`): OutlineGroup[] | string {
  const a = Math.max(0, Math.min(start, end));
  const b = Math.max(start, end);
  for (const o of list) {
    if (o.axis !== axis) continue;
    if (o.start === a && o.end === b) return "These rows or columns are already grouped.";
    const overlap = a <= o.end && b >= o.start;
    const nested = (a >= o.start && b <= o.end) || (o.start >= a && o.end <= b);
    if (overlap && !nested) return "A group cannot partly overlap another group. Select whole groups or rows inside one.";
  }
  const g: OutlineGroup = { id, axis, start: a, end: b };
  const next = [...list, g];
  if (next.filter((o) => o.axis === axis).some((o) => outlineLevel(next, o) > MAX_OUTLINE_LEVEL)) return `Groups can nest at most ${MAX_OUTLINE_LEVEL} levels.`;
  return next.sort((x, y) => (x.axis === y.axis ? x.start - y.start || y.end - x.end : x.axis < y.axis ? -1 : 1));
}

/** Remove the innermost group that covers the selection exactly or contains it. */
export function ungroup(list: OutlineGroup[], axis: OutlineAxis, start: number, end: number): { list: OutlineGroup[]; removed?: OutlineGroup } {
  const a = Math.min(start, end);
  const b = Math.max(start, end);
  const cands = list.filter((o) => o.axis === axis && o.start <= a && o.end >= b).sort((x, y) => x.end - x.start - (y.end - y.start));
  const exact = list.find((o) => o.axis === axis && o.start === a && o.end === b);
  const removed = exact ?? cands[0];
  if (!removed) return { list };
  return { list: list.filter((o) => o.id !== removed.id), removed };
}

/**
 * The indices to hide and show after a group changes state. Expanding a group
 * keeps the rows of collapsed groups inside it hidden.
 */
export function visibilityAfter(list: OutlineGroup[], changed: OutlineGroup): { hide: number[]; show: number[] } {
  const span = (g: OutlineGroup) => Array.from({ length: g.end - g.start + 1 }, (_, i) => g.start + i);
  if (changed.collapsed) return { hide: span(changed), show: [] };
  const stillHidden = new Set<number>();
  for (const o of list) {
    if (o.axis !== changed.axis || o.id === changed.id || !o.collapsed) continue;
    for (const i of span(o)) stillHidden.add(i);
  }
  return { hide: [], show: span(changed).filter((i) => !stillHidden.has(i)) };
}

export function setCollapsed(list: OutlineGroup[], id: string, collapsed: boolean): { list: OutlineGroup[]; group?: OutlineGroup } {
  let group: OutlineGroup | undefined;
  const next = list.map((o) => {
    if (o.id !== id) return o;
    group = { ...o, collapsed };
    return group;
  });
  return { list: next, group };
}

/** Collapse or expand every group of one level (Excel's outline level buttons 1, 2, 3 …): levels deeper than `level` collapse. */
export function showLevel(list: OutlineGroup[], axis: OutlineAxis, level: number): OutlineGroup[] {
  return list.map((o) => (o.axis === axis ? { ...o, collapsed: outlineLevel(list, o) >= level } : o));
}

export function groupLabel(g: OutlineGroup): string {
  if (g.axis === "row") return g.start === g.end ? `Row ${g.start + 1}` : `Rows ${g.start + 1}–${g.end + 1}`;
  const col = (n: number) => {
    let s = "";
    for (let x = n + 1; x > 0; x = Math.floor((x - 1) / 26)) s = String.fromCharCode(65 + ((x - 1) % 26)) + s;
    return s;
  };
  return g.start === g.end ? `Column ${col(g.start)}` : `Columns ${col(g.start)}–${col(g.end)}`;
}

// ---- workbook side --------------------------------------------------------

function apply(w: any, axis: OutlineAxis, vis: { hide: number[]; show: number[] }) {
  const type = axis === "row" ? "row" : "column";
  if (vis.hide.length) w.hideRowOrColumn(vis.hide.map(String), type);
  if (vis.show.length) w.showRowOrColumn(vis.show.map(String), type);
}

/** Group the selected rows or columns of the current sheet; returns an error message or null. */
export function groupSelection(w: any, axis: OutlineAxis): string | null {
  const sheet = currentSheet(w);
  const r = w.getSelection?.()?.[0];
  if (!sheet || !r) return "Select the rows or columns to group first.";
  const [a, b] = axis === "row" ? r.row : r.column;
  const next = addGroup(sheetOutline(sheet), axis, a, b);
  if (typeof next === "string") return next;
  patchSheet(w, sheet.id, { adrientOutline: next });
  return null;
}

export function ungroupSelection(w: any, axis: OutlineAxis): string | null {
  const sheet = currentSheet(w);
  const r = w.getSelection?.()?.[0];
  if (!sheet || !r) return "Select rows or columns inside a group first.";
  const [a, b] = axis === "row" ? r.row : r.column;
  const { list, removed } = ungroup(sheetOutline(sheet), axis, a, b);
  if (!removed) return "The selection is not in a group.";
  if (removed.collapsed) apply(w, axis, visibilityAfter(list, { ...removed, collapsed: false }));
  patchSheet(w, sheet.id, { adrientOutline: list });
  return null;
}

export function toggleGroup(w: any, id: string): void {
  const sheet = currentSheet(w);
  if (!sheet) return;
  const cur = sheetOutline(sheet).find((o) => o.id === id);
  if (!cur) return;
  const { list, group } = setCollapsed(sheetOutline(sheet), id, !cur.collapsed);
  if (!group) return;
  apply(w, group.axis, visibilityAfter(list, group));
  patchSheet(w, sheet.id, { adrientOutline: list });
}

export function applyLevel(w: any, axis: OutlineAxis, level: number): void {
  const sheet = currentSheet(w);
  if (!sheet) return;
  const before = sheetOutline(sheet);
  const list = showLevel(before, axis, level);
  const hidden = new Set<number>();
  const all = new Set<number>();
  for (const o of list) {
    if (o.axis !== axis) continue;
    for (let i = o.start; i <= o.end; i++) {
      all.add(i);
      if (o.collapsed) hidden.add(i);
    }
  }
  apply(w, axis, { hide: [...hidden], show: [...all].filter((i) => !hidden.has(i)) });
  patchSheet(w, sheet.id, { adrientOutline: list });
}

export function maxLevel(list: OutlineGroup[], axis: OutlineAxis): number {
  return list.filter((o) => o.axis === axis).reduce((m, o) => Math.max(m, outlineLevel(list, o)), 0);
}
