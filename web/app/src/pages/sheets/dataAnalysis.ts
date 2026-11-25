// Data ▸ Analysis ▸ Summary statistics: Excel's "Descriptive Statistics" table
// for each selected column (numbers only; text and blanks are skipped).

export interface ColumnStats {
  label: string;
  count: number;
  mean: number;
  stdError: number;
  median: number;
  mode: number | null;
  stdDev: number;
  variance: number;
  kurtosis: number | null;
  skewness: number | null;
  range: number;
  min: number;
  max: number;
  sum: number;
}

export const STAT_ROWS: [keyof ColumnStats, string][] = [
  ["mean", "Mean"],
  ["stdError", "Standard error"],
  ["median", "Median"],
  ["mode", "Mode"],
  ["stdDev", "Standard deviation"],
  ["variance", "Sample variance"],
  ["kurtosis", "Kurtosis"],
  ["skewness", "Skewness"],
  ["range", "Range"],
  ["min", "Minimum"],
  ["max", "Maximum"],
  ["sum", "Sum"],
  ["count", "Count"],
];

export function describe(values: number[], label = ""): ColumnStats | null {
  const xs = values.filter((v) => typeof v === "number" && Number.isFinite(v));
  const n = xs.length;
  if (!n) return null;
  const sum = xs.reduce((a, b) => a + b, 0);
  const mean = sum / n;
  const sorted = [...xs].sort((a, b) => a - b);
  const median = n % 2 ? sorted[(n - 1) / 2] : (sorted[n / 2 - 1] + sorted[n / 2]) / 2;
  const counts = new Map<number, number>();
  let mode: number | null = null;
  let best = 1;
  for (const x of xs) {
    const c = (counts.get(x) ?? 0) + 1;
    counts.set(x, c);
    if (c > best) {
      best = c;
      mode = x;
    }
  }
  const m2 = xs.reduce((a, x) => a + (x - mean) ** 2, 0);
  const variance = n > 1 ? m2 / (n - 1) : 0;
  const sd = Math.sqrt(variance);
  // Excel's SKEW and KURT (sample, bias-corrected).
  const skewness = n > 2 && sd > 0 ? (n / ((n - 1) * (n - 2))) * xs.reduce((a, x) => a + ((x - mean) / sd) ** 3, 0) : null;
  const kurtosis =
    n > 3 && sd > 0
      ? ((n * (n + 1)) / ((n - 1) * (n - 2) * (n - 3))) * xs.reduce((a, x) => a + ((x - mean) / sd) ** 4, 0) - (3 * (n - 1) ** 2) / ((n - 2) * (n - 3))
      : null;
  return {
    label,
    count: n,
    mean,
    stdError: sd / Math.sqrt(n),
    median,
    mode,
    stdDev: sd,
    variance,
    kurtosis,
    skewness,
    range: sorted[n - 1] - sorted[0],
    min: sorted[0],
    max: sorted[n - 1],
    sum,
  };
}

/** Columns of a cell block (rows × cols of raw values); the first row is labels when `headers`. */
export function describeColumns(block: unknown[][], headers: boolean): ColumnStats[] {
  const cols = Math.max(0, ...block.map((r) => r.length));
  const out: ColumnStats[] = [];
  for (let c = 0; c < cols; c++) {
    const label = headers ? String(block[0]?.[c] ?? `Column ${c + 1}`) : `Column ${c + 1}`;
    const nums = block.slice(headers ? 1 : 0).map((r) => r[c]).filter((v): v is number => typeof v === "number");
    const s = describe(nums, label);
    if (s) out.push(s);
  }
  return out;
}

/** The output block: a label column then one column per described column. */
export function statsGrid(stats: ColumnStats[]): (string | number | null)[][] {
  return [["", ...stats.map((s) => s.label)], ...STAT_ROWS.map(([k, name]) => [name, ...stats.map((s) => s[k] as number | null)])];
}
