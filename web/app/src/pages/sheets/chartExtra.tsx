// Radar and treemap charts (wave 4). Geometry is pure and unit-tested
// (chartExtra.test.ts); the renderers draw SVG in the ChartRenderer style.
import type { ReactNode } from "react";
import { seriesColor, type ChartConfig, type ChartSeries } from "./chartData";

export interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

// ---------------------------------------------------------------- radar

/** A "nice" maximum for the radar's value rings (1, 2, 2.5, 5 × 10^k). */
export function niceMax(v: number): number {
  if (!(v > 0)) return 1;
  const p = Math.pow(10, Math.floor(Math.log10(v)));
  for (const m of [1, 2, 2.5, 5, 10]) if (m * p >= v) return m * p;
  return 10 * p;
}

/** Point i of n spokes at value v (0..max), starting at 12 o'clock, clockwise. */
export function radarPoint(cx: number, cy: number, r: number, i: number, n: number, v: number, max: number): [number, number] {
  const a = -Math.PI / 2 + (2 * Math.PI * i) / Math.max(1, n);
  const f = max > 0 ? Math.max(0, v) / max : 0;
  return [cx + Math.cos(a) * r * f, cy + Math.sin(a) * r * f];
}

export function renderRadar(cfg: ChartConfig, categories: string[], series: ChartSeries[], box: Box): ReactNode {
  const n = Math.max(categories.length, ...series.map((s) => s.values.length));
  if (n < 3) {
    return (
      <text x={box.x + box.w / 2} y={box.y + box.h / 2} textAnchor="middle" fontSize={12} fill="#8a8a8a">
        A radar chart needs at least 3 categories
      </text>
    );
  }
  const cx = box.x + box.w / 2;
  const cy = box.y + box.h / 2;
  const r = Math.max(10, Math.min(box.w, box.h) / 2 - 22);
  const max = cfg.yAxis?.max ?? niceMax(Math.max(0, ...series.flatMap((s) => s.values.filter(Number.isFinite))));
  const rings = 4;
  const poly = (pts: [number, number][]) => pts.map((p) => p.join(",")).join(" ");
  return (
    <g data-testid="radar">
      {Array.from({ length: rings }, (_, k) => (
        <polygon key={k} points={poly(Array.from({ length: n }, (_, i) => radarPoint(cx, cy, r, i, n, ((k + 1) / rings) * max, max)))} fill="none" stroke="#e6e6e6" />
      ))}
      {Array.from({ length: n }, (_, i) => {
        const [x, y] = radarPoint(cx, cy, r, i, n, max, max);
        const [lx, ly] = radarPoint(cx, cy, r + 12, i, n, max, max);
        return (
          <g key={i}>
            <line x1={cx} y1={cy} x2={x} y2={y} stroke="#e6e6e6" />
            <text x={lx} y={ly + 4} fontSize={10} fill="#555" textAnchor={Math.abs(lx - cx) < 4 ? "middle" : lx > cx ? "start" : "end"}>
              {(categories[i] ?? String(i + 1)).slice(0, 14)}
            </text>
          </g>
        );
      })}
      {series.map((s, k) => {
        const pts = Array.from({ length: n }, (_, i) => radarPoint(cx, cy, r, i, n, Number.isFinite(s.values[i]) ? s.values[i] : 0, max));
        const c = seriesColor(cfg, k);
        return <polygon key={k} points={poly(pts)} fill={c} fillOpacity={0.15} stroke={c} strokeWidth={2} />;
      })}
    </g>
  );
}

// ---------------------------------------------------------------- treemap

export interface TreemapRect extends Box {
  index: number;
  value: number;
}

/**
 * Squarified treemap layout (Bruls, Huizing & van Wijk, 2000) of positive
 * values into a box. Non-positive values are skipped. Rectangles keep the
 * input index so labels and colours follow the source rows.
 */
export function squarify(values: number[], box: Box): TreemapRect[] {
  const items = values
    .map((value, index) => ({ value, index }))
    .filter((d) => Number.isFinite(d.value) && d.value > 0)
    .sort((a, b) => b.value - a.value);
  const total = items.reduce((s, d) => s + d.value, 0);
  if (!items.length || box.w <= 0 || box.h <= 0) return [];
  const scale = (box.w * box.h) / total;
  const out: TreemapRect[] = [];
  let free = { ...box };
  let row: { value: number; index: number }[] = [];
  const worst = (r: typeof row, side: number) => {
    const areas = r.map((d) => d.value * scale);
    const s = areas.reduce((a, b) => a + b, 0);
    const mx = Math.max(...areas);
    const mn = Math.min(...areas);
    return Math.max((side * side * mx) / (s * s), (s * s) / (side * side * mn));
  };
  const layRow = (r: typeof row) => {
    const area = r.reduce((a, d) => a + d.value * scale, 0);
    if (free.w >= free.h) {
      const w = area / free.h;
      let y = free.y;
      for (const d of r) {
        const h = (d.value * scale) / w;
        out.push({ x: free.x, y, w, h, index: d.index, value: d.value });
        y += h;
      }
      free = { x: free.x + w, y: free.y, w: free.w - w, h: free.h };
    } else {
      const h = area / free.w;
      let x = free.x;
      for (const d of r) {
        const w = (d.value * scale) / h;
        out.push({ x, y: free.y, w, h, index: d.index, value: d.value });
        x += w;
      }
      free = { x: free.x, y: free.y + h, w: free.w, h: free.h - h };
    }
  };
  for (const d of items) {
    const side = Math.min(free.w, free.h);
    if (!row.length || worst([...row, d], side) <= worst(row, side)) {
      row.push(d);
    } else {
      layRow(row);
      row = [d];
    }
  }
  if (row.length) layRow(row);
  return out;
}

export function renderTreemap(cfg: ChartConfig, categories: string[], series: ChartSeries[], box: Box): ReactNode {
  const values = series[0]?.values ?? [];
  const rects = squarify(values, box);
  if (!rects.length) {
    return (
      <text x={box.x + box.w / 2} y={box.y + box.h / 2} textAnchor="middle" fontSize={12} fill="#8a8a8a">
        No positive values
      </text>
    );
  }
  return (
    <g data-testid="treemap">
      {rects.map((t) => (
        <g key={t.index}>
          <rect x={t.x} y={t.y} width={Math.max(0, t.w - 1)} height={Math.max(0, t.h - 1)} fill={seriesColor(cfg, t.index)} />
          {t.w > 36 && t.h > 18 && (
            <text x={t.x + 4} y={t.y + 14} fontSize={11} fill="#fff">
              {(categories[t.index] ?? "").slice(0, Math.floor(t.w / 6.5))}
            </text>
          )}
          {t.w > 36 && t.h > 32 && (
            <text x={t.x + 4} y={t.y + 28} fontSize={10} fill="#fff" fillOpacity={0.85}>
              {t.value.toLocaleString()}
            </text>
          )}
        </g>
      ))}
    </g>
  );
}
