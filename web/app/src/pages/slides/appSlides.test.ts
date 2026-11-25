import { describe, expect, it } from "vitest";
import { appTableElement, summaryChart, summaryChartElement } from "./appSlides";

const deals = {
  title: "Deals",
  header: ["Title", "Stage", "Value", "Currency", "A", "B", "C", "D"],
  rows: [
    ["a", "won", 1000, "USD", "", "", "", ""],
    ["b", "won", 500.5, "USD", "", "", "", ""],
    ["c", "lead", 20, "USD", "", "", "", ""],
  ],
};

describe("appTableElement", () => {
  it("caps columns and rows and formats numbers", () => {
    const { el, shown, total } = appTableElement(deals, 2, 4);
    expect(el.type).toBe("table");
    expect(el.table?.cols).toBe(4);
    expect(el.table?.rows).toBe(3);
    expect(el.table?.cells[1][2]).toBe("1,000");
    expect(shown).toBe(2);
    expect(total).toBe(3);
  });
});

describe("summaryChart", () => {
  it("sums CRM value by stage, largest first", () => {
    const s = summaryChart("crm", deals)!;
    expect(s.data).toEqual([
      ["", "Value"],
      ["won", "1500.5"],
      ["lead", "20"],
    ]);
    const el = summaryChartElement(s);
    expect(el.type).toBe("chart");
    expect(el.chart?.title).toBe("Pipeline value by stage");
  });
  it("groups POS by month ascending and returns null without data", () => {
    const s = summaryChart("pos", {
      title: "x",
      header: ["Date", "Total"],
      rows: [["2026-02-03", 5], ["2026-01-09", 2], ["2026-01-10", 3]],
    })!;
    expect(s.data.slice(1)).toEqual([["2026-01", "5"], ["2026-02", "5"]]);
    expect(summaryChart("crm", { title: "", header: ["Stage"], rows: [] })).toBeNull();
  });
});
