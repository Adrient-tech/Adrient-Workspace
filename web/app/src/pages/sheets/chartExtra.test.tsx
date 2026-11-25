import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { niceMax, radarPoint, squarify } from "./chartExtra";
import { ChartRenderer, chartTypeLabel } from "./ChartRenderer";
import { chartSpaceXml, readChartSpace } from "./xlsx/xlsxCharts";
import { parseXml } from "./xlsx/ooxml";
import type { ChartConfig } from "./chartData";

const input = {
  categories: ["Rent", "Food", "Travel", "Utilities"],
  series: [
    { name: "Jan", values: [1200, 450, 200, 150] },
    { name: "Feb", values: [1200, 520, 90, 160] },
  ],
};
const base: ChartConfig = { id: "x", type: "radar", range: { r0: 0, r1: 4, c0: 0, c1: 2 }, headerRow: true, labelCol: true } as ChartConfig;

describe("radar geometry", () => {
  it("rounds the ring maximum up to a nice number", () => {
    expect(niceMax(1200)).toBe(2000);
    expect(niceMax(450)).toBe(500);
    expect(niceMax(0)).toBe(1);
    expect(niceMax(1)).toBe(1);
    expect(niceMax(24)).toBe(25);
  });
  it("starts at 12 o'clock and goes clockwise", () => {
    const [x0, y0] = radarPoint(100, 100, 50, 0, 4, 10, 10);
    expect(x0).toBeCloseTo(100);
    expect(y0).toBeCloseTo(50);
    const [x1, y1] = radarPoint(100, 100, 50, 1, 4, 10, 10);
    expect(x1).toBeCloseTo(150);
    expect(y1).toBeCloseTo(100);
  });
  it("clamps negatives to the centre", () => {
    expect(radarPoint(0, 0, 50, 0, 3, -5, 10)).toEqual([0, 0]);
  });
});

describe("treemap layout", () => {
  const box = { x: 0, y: 0, w: 400, h: 200 };
  it("covers the box with areas proportional to the values", () => {
    const rects = squarify([6, 6, 4, 3, 2, 2, 1], box);
    expect(rects).toHaveLength(7);
    const total = rects.reduce((s, r) => s + r.w * r.h, 0);
    expect(total).toBeCloseTo(400 * 200, 3);
    for (const r of rects) expect((r.w * r.h) / (400 * 200)).toBeCloseTo(r.value / 24, 5);
  });
  it("keeps every rectangle inside the box", () => {
    for (const r of squarify([5, 1, 1, 9, 3], box)) {
      expect(r.x).toBeGreaterThanOrEqual(-1e-9);
      expect(r.y).toBeGreaterThanOrEqual(-1e-9);
      expect(r.x + r.w).toBeLessThanOrEqual(400 + 1e-6);
      expect(r.y + r.h).toBeLessThanOrEqual(200 + 1e-6);
    }
  });
  it("skips zero, negative and non-numeric values and keeps source indexes", () => {
    const rects = squarify([0, 5, -2, NaN, 5], box);
    expect(rects.map((r) => r.index).sort()).toEqual([1, 4]);
  });
  it("returns nothing for an empty box", () => {
    expect(squarify([1, 2], { x: 0, y: 0, w: 0, h: 10 })).toEqual([]);
  });
});

describe("rendering", () => {
  it("draws a radar chart", () => {
    const html = renderToStaticMarkup(<ChartRenderer config={base} input={input} />);
    expect(html).toContain('data-chart-type="radar"');
    expect(html).toContain('data-testid="radar"');
    expect(html.match(/<polygon/g)!.length).toBe(4 + 2); // 4 rings + 2 series
  });
  it("explains a radar with too few categories", () => {
    const html = renderToStaticMarkup(<ChartRenderer config={base} input={{ categories: ["a", "b"], series: [{ name: "s", values: [1, 2] }] }} />);
    expect(html).toContain("at least 3 categories");
  });
  it("draws a treemap with labels", () => {
    const html = renderToStaticMarkup(<ChartRenderer config={{ ...base, type: "treemap" }} input={input} width={480} height={300} />);
    expect(html).toContain('data-testid="treemap"');
    expect(html).toContain("Rent");
    expect(html.match(/<rect/g)!.length).toBeGreaterThanOrEqual(5);
  });
  it("labels the new types", () => {
    expect(chartTypeLabel("radar")).toBe("Radar");
    expect(chartTypeLabel("treemap")).toBe("Treemap");
  });
});

describe("radar xlsx", () => {
  it("writes c:radarChart and reads it back as radar", () => {
    const xml = chartSpaceXml(base, "Data");
    expect(xml).toContain("<c:radarChart>");
    const got = readChartSpace(parseXml(xml))!;
    expect(got.cfg.type).toBe("radar");
  });
});
