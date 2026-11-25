// Insert ▸ From Adrient: snapshots of a Sheets range (as table or chart) and a
// Docs page (as text). Objects are copies taken at insert time; each carries a
// link back to its source so a click in the show opens the original.
import type { AppTable } from "../sheets/appData";
import { newTextElement } from "./adrientText";
import type { ChartSummary } from "./appSlides";
import type { SlideElement } from "./model";

interface FortuneCell {
  r: number;
  c: number;
  v?: { v?: unknown; m?: unknown } | null;
}
interface FortuneSheet {
  name?: string;
  celldata?: FortuneCell[];
  data?: ({ v?: unknown; m?: unknown } | null)[][];
}

/** Sheet names in a stored workbook JSON. */
export function sheetNames(json: string): string[] {
  try {
    const wb = JSON.parse(json) as FortuneSheet[];
    return Array.isArray(wb) ? wb.map((s, i) => s.name || `Sheet${i + 1}`) : [];
  } catch {
    return [];
  }
}

/** "B2:D9" → zero-based bounds; null when invalid. Single cells and whole columns are not accepted. */
export function parseRange(a1: string): { r0: number; c0: number; r1: number; c1: number } | null {
  const m = /^\s*([A-Za-z]{1,3})(\d{1,6})\s*:\s*([A-Za-z]{1,3})(\d{1,6})\s*$/.exec(a1);
  if (!m) return null;
  const col = (s: string) => [...s.toUpperCase()].reduce((n, ch) => n * 26 + ch.charCodeAt(0) - 64, 0) - 1;
  const [c0, c1] = [col(m[1]), col(m[3])].sort((a, b) => a - b);
  const [r0, r1] = [Number(m[2]) - 1, Number(m[4]) - 1].sort((a, b) => a - b);
  return { r0, c0, r1, c1 };
}

const cellText = (v: FortuneCell["v"]): string => {
  if (!v) return "";
  const x = v.m ?? v.v;
  return x === undefined || x === null ? "" : String(x);
};

/** Rows of a sheet inside `range` (A1 notation), or its used area when empty. First row = header. */
export function sheetTable(json: string, sheetIdx: number, range: string, maxCells = 2000): AppTable | null {
  let wb: FortuneSheet[];
  try {
    wb = JSON.parse(json);
  } catch {
    return null;
  }
  const sh = Array.isArray(wb) ? wb[sheetIdx] : undefined;
  if (!sh) return null;
  const cells = new Map<string, string>();
  let maxR = -1;
  let maxC = -1;
  const put = (r: number, c: number, v: FortuneCell["v"]) => {
    const t = cellText(v);
    if (!t) return;
    cells.set(`${r}:${c}`, t);
    maxR = Math.max(maxR, r);
    maxC = Math.max(maxC, c);
  };
  for (const cd of sh.celldata ?? []) put(cd.r, cd.c, cd.v);
  (sh.data ?? []).forEach((row, r) => (row ?? []).forEach((v, c) => put(r, c, v)));
  if (maxR < 0) return null;
  const b = range.trim() ? parseRange(range) : { r0: 0, c0: 0, r1: maxR, c1: maxC };
  if (!b) return null;
  const r1 = Math.min(b.r1, b.r0 + Math.floor(maxCells / Math.max(1, b.c1 - b.c0 + 1)));
  const grid: string[][] = [];
  for (let r = b.r0; r <= r1; r++) {
    const row: string[] = [];
    for (let c = b.c0; c <= b.c1; c++) row.push(cells.get(`${r}:${c}`) ?? "");
    grid.push(row);
  }
  if (grid.length < 1 || grid.every((r) => r.every((x) => !x))) return null;
  const num = (s: string) => (s !== "" && isFinite(Number(s.replace(/[,₹$%\s]/g, ""))) ? Number(s.replace(/[,₹$%\s]/g, "")) : s);
  return { title: sh.name || "Sheet", header: grid[0], rows: grid.slice(1).map((r) => r.map(num)) };
}

/** A chart summary from a table: first column = categories, second numeric column = values. */
export function tableChart(tbl: AppTable, title: string): ChartSummary | null {
  const valCol = tbl.header.findIndex((_, i) => i > 0 && tbl.rows.some((r) => typeof r[i] === "number"));
  if (valCol < 0) return null;
  const rows = tbl.rows.slice(0, 12);
  if (!rows.length) return null;
  return {
    title,
    type: rows.length <= 6 && valCol === 1 && tbl.header.length === 2 ? "column" : "column",
    data: [["", tbl.header[valCol] || "Value"], ...rows.map((r) => [String(r[0] ?? ""), String(typeof r[valCol] === "number" ? r[valCol] : 0)])],
  };
}

/** Plain paragraphs of a Docs preview (HTML). */
export function docParagraphs(html: string, max = 12): string[] {
  const doc = new DOMParser().parseFromString(html, "text/html");
  const out: string[] = [];
  doc.body.querySelectorAll("h1,h2,h3,h4,p,li").forEach((n) => {
    const t = (n.textContent ?? "").replace(/\s+/g, " ").trim();
    if (t) out.push(n.tagName === "LI" ? `• ${t}` : t);
  });
  return out.slice(0, max);
}

/** A text element with a Docs page snapshot: its title and first paragraphs. */
export function docTextElement(title: string, paragraphs: string[], docId: string): SlideElement {
  const body = paragraphs.join("\n");
  const el = newTextElement(`${title}\n${body}`, { x: 60, y: 60, w: 840, h: 420 });
  el.fontSize = body.length > 700 ? 14 : body.length > 350 ? 18 : 22;
  el.name = `Docs page: ${title}`;
  el.url = `/docs/d/${docId}`;
  return el;
}
