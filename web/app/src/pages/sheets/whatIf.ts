// Client side of the what-if tools (Data ▸ What-if analysis): request types,
// input parsing and the grid a data table result is written as.

export interface DataTableResult {
  results: (number | string | boolean | null)[][];
}
export interface Scenario {
  name: string;
  values: Record<string, number>;
}
export interface ScenarioResult {
  rows: { name: string; results: (number | string | boolean | null)[] }[];
}
export interface SolverVariable {
  cell: string;
  min?: number;
  max?: number;
}
export interface SolverConstraint {
  cell: string;
  op: "<=" | ">=" | "=";
  value: number;
}
export interface SolverRequestUI {
  objective: string;
  goal: "max" | "min" | "value";
  target?: number;
  variables: SolverVariable[];
  constraints: SolverConstraint[];
}
export interface SolverResult {
  feasible: boolean;
  objective: number | string | null;
  values: Record<string, number>;
  violations?: string[];
  evaluations: number;
  method: string;
}

const CELL_RE = /^('[^']+'!|[^!\s']+!)?\$?[A-Za-z]{1,3}\$?\d+$/;
export const isCellRef = (s: string) => CELL_RE.test(s.trim());

/** Splits "A1, B2  C3" or "A1:A3" style cell lists (ranges are expanded, max 64 cells). */
export function parseCellList(s: string): string[] | null {
  const out: string[] = [];
  for (const part of s.split(/[,;\s]+/).filter(Boolean)) {
    const m = /^\$?([A-Za-z]{1,3})\$?(\d+):\$?([A-Za-z]{1,3})\$?(\d+)$/.exec(part);
    if (m) {
      const c1 = colIndex(m[1]);
      const c2 = colIndex(m[3]);
      const r1 = Number(m[2]);
      const r2 = Number(m[4]);
      for (let r = Math.min(r1, r2); r <= Math.max(r1, r2); r++)
        for (let c = Math.min(c1, c2); c <= Math.max(c1, c2); c++) {
          out.push(colName(c) + r);
          if (out.length > 64) return null;
        }
    } else if (isCellRef(part)) {
      out.push(part.replace(/\$/g, "").toUpperCase());
    } else {
      return null;
    }
  }
  return out.length ? out : null;
}

export function colIndex(letters: string): number {
  let n = 0;
  for (const ch of letters.toUpperCase()) n = n * 26 + (ch.charCodeAt(0) - 64);
  return n - 1;
}
export function colName(c: number): string {
  let s = "";
  let n = c + 1;
  while (n > 0) {
    const m = (n - 1) % 26;
    s = String.fromCharCode(65 + m) + s;
    n = Math.floor((n - 1) / 26);
  }
  return s;
}

/**
 * Parses a list of numbers: "1, 2, 5" or a series "start:step:end" (e.g.
 * "0:0.05:0.3"). Returns null for anything invalid or longer than max.
 */
export function parseValueList(s: string, max = 200): number[] | null {
  const t = s.trim();
  if (!t) return null;
  const series = /^(-?[\d.]+)\s*:\s*(-?[\d.]+)\s*:\s*(-?[\d.]+)$/.exec(t);
  if (series) {
    const [a, step, b] = series.slice(1).map(Number);
    if (![a, step, b].every(Number.isFinite) || step === 0 || (b - a) / step < 0) return null;
    const n = Math.floor((b - a) / step + 1e-9) + 1;
    if (n > max) return null;
    return Array.from({ length: n }, (_, i) => Number((a + i * step).toPrecision(12)));
  }
  const vals = t.split(/[,;\s]+/).filter(Boolean).map((x) => Number(x.replace(/_/g, "")));
  if (!vals.length || vals.length > max || !vals.every(Number.isFinite)) return null;
  return vals;
}

/** Parses "A1=200, A2=0.1" into a scenario's value map. */
export function parseAssignments(s: string): Record<string, number> | null {
  const out: Record<string, number> = {};
  const parts = s.split(/[,;\n]+/).map((p) => p.trim()).filter(Boolean);
  if (!parts.length) return null;
  for (const p of parts) {
    const m = /^([^=]+)=\s*(-?[\d.eE+-]+)$/.exec(p);
    if (!m || !isCellRef(m[1].trim()) || !Number.isFinite(Number(m[2]))) return null;
    out[m[1].trim().replace(/\$/g, "").toUpperCase()] = Number(m[2]);
  }
  return out;
}

/**
 * The block a one-variable data table writes: a header row ("Input",
 * formula refs…) then one row per input value with its results.
 */
export function dataTableGrid(inputCell: string, formulas: string[], values: number[], res: DataTableResult): (string | number | boolean | null)[][] {
  const head: (string | number | boolean | null)[] = [inputCell, ...formulas];
  return [head, ...values.map((v, i) => [v, ...(res.results[i] ?? [])])];
}

/** Two-variable table: corner = formula ref, first row = column values, first column = row values. */
export function dataTable2Grid(formula: string, rowValues: number[], colValues: number[], res: DataTableResult): (string | number | boolean | null)[][] {
  return [[formula, ...colValues], ...rowValues.map((rv, i) => [rv, ...(res.results[i] ?? [])])];
}
