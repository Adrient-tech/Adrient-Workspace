import { describe, expect, it } from "vitest";
import { docParagraphs, docTextElement, parseRange, sheetNames, sheetTable, tableChart } from "./adrientSources";

const wb = JSON.stringify([
  { name: "Sales", celldata: [
    { r: 0, c: 0, v: { v: "Region" } }, { r: 0, c: 1, v: { v: "Revenue" } },
    { r: 1, c: 0, v: { v: "North" } }, { r: 1, c: 1, v: { v: 1200, m: "1,200" } },
    { r: 2, c: 0, v: { v: "South" } }, { r: 2, c: 1, v: { v: 800 } },
    { r: 5, c: 3, v: { v: "far" } },
  ] },
  { name: "Empty", celldata: [] },
]);

describe("adrient sources", () => {
  it("lists sheet names and rejects bad json", () => {
    expect(sheetNames(wb)).toEqual(["Sales", "Empty"]);
    expect(sheetNames("nope")).toEqual([]);
  });
  it("parses A1 ranges in any corner order", () => {
    expect(parseRange("A1:B3")).toEqual({ r0: 0, c0: 0, r1: 2, c1: 1 });
    expect(parseRange("c3:a1")).toEqual({ r0: 0, c0: 0, r1: 2, c1: 2 });
    expect(parseRange("A1")).toBeNull();
    expect(parseRange("1:2")).toBeNull();
  });
  it("reads a range with formatted values and numeric cells", () => {
    const t = sheetTable(wb, 0, "A1:B3")!;
    expect(t.header).toEqual(["Region", "Revenue"]);
    expect(t.rows).toEqual([["North", 1200], ["South", 800]]);
  });
  it("uses the used area when no range is given and handles empty sheets", () => {
    const t = sheetTable(wb, 0, "")!;
    expect(t.header.length).toBe(4);
    expect(sheetTable(wb, 1, "")).toBeNull();
    expect(sheetTable(wb, 0, "bad")).toBeNull();
    expect(sheetTable(wb, 9, "")).toBeNull();
  });
  it("builds a chart from category and numeric column", () => {
    const c = tableChart(sheetTable(wb, 0, "A1:B3")!, "Revenue")!;
    expect(c.data).toEqual([["", "Revenue"], ["North", "1200"], ["South", "800"]]);
    expect(tableChart({ title: "t", header: ["a", "b"], rows: [["x", "y"]] }, "t")).toBeNull();
  });
  it("extracts doc paragraphs and builds a linked text box", () => {
    const p = docParagraphs("<h1>Plan</h1><p>First   para</p><ul><li>One</li></ul><p></p>");
    expect(p).toEqual(["Plan", "First para", "• One"]);
    const el = docTextElement("Plan", p, "d1");
    expect(el.text).toContain("First para");
    expect(el.url).toBe("/docs/d/d1");
  });
});
