import { describe as group, expect, it } from "vitest";
import { colIndex, colName, dataTable2Grid, dataTableGrid, isCellRef, parseAssignments, parseCellList, parseValueList } from "./whatIf";
import { describe, describeColumns, statsGrid, STAT_ROWS } from "./dataAnalysis";

group("what-if parsing", () => {
  it("recognises cell references", () => {
    expect(isCellRef("B2")).toBe(true);
    expect(isCellRef("$B$2")).toBe(true);
    expect(isCellRef("Sheet2!C3")).toBe(true);
    expect(isCellRef("'My sheet'!C3")).toBe(true);
    expect(isCellRef("B")).toBe(false);
    expect(isCellRef("2")).toBe(false);
    expect(isCellRef("B2:C3")).toBe(false);
  });
  it("converts column letters", () => {
    expect(colIndex("A")).toBe(0);
    expect(colIndex("Z")).toBe(25);
    expect(colIndex("AA")).toBe(26);
    expect(colName(0)).toBe("A");
    expect(colName(27)).toBe("AB");
    for (const c of [0, 25, 26, 701, 702, 16383]) expect(colIndex(colName(c))).toBe(c);
  });
  it("expands cell lists and ranges", () => {
    expect(parseCellList("C1, D1")).toEqual(["C1", "D1"]);
    expect(parseCellList("B2:B4")).toEqual(["B2", "B3", "B4"]);
    expect(parseCellList("A1:B2")).toEqual(["A1", "B1", "A2", "B2"]);
    expect(parseCellList("$e$6")).toEqual(["E6"]);
    expect(parseCellList("")).toBeNull();
    expect(parseCellList("C1, nope")).toBeNull();
    expect(parseCellList("A1:A100")).toBeNull(); // over the 64-cell cap
  });
  it("parses value lists and series", () => {
    expect(parseValueList("1, 2, 5")).toEqual([1, 2, 5]);
    expect(parseValueList("1 2;3")).toEqual([1, 2, 3]);
    expect(parseValueList("0:0.1:0.3")).toEqual([0, 0.1, 0.2, 0.3]);
    expect(parseValueList("10:-5:0")).toEqual([10, 5, 0]);
    expect(parseValueList("0:0:1")).toBeNull();
    expect(parseValueList("1:1:0")).toBeNull();
    expect(parseValueList("a, 2")).toBeNull();
    expect(parseValueList("")).toBeNull();
    expect(parseValueList("0:1:1000")).toBeNull();
  });
  it("parses scenario assignments", () => {
    expect(parseAssignments("B2=1500, B3 = 400")).toEqual({ B2: 1500, B3: 400 });
    expect(parseAssignments("$b$2=-1.5e2")).toEqual({ B2: -150 });
    expect(parseAssignments("B2")).toBeNull();
    expect(parseAssignments("B2=x")).toBeNull();
    expect(parseAssignments("")).toBeNull();
  });
  it("lays out data table results", () => {
    expect(dataTableGrid("A1", ["C1", "D1"], [0, 1], { results: [[10, 1], [20, 2]] })).toEqual([
      ["A1", "C1", "D1"],
      [0, 10, 1],
      [1, 20, 2],
    ]);
    expect(dataTable2Grid("C1", [1, 2], [10, 20], { results: [[10, 20], [20, 40]] })).toEqual([
      ["C1", 10, 20],
      [1, 10, 20],
      [2, 20, 40],
    ]);
  });
});

group("summary statistics", () => {
  const xs = [2, 4, 4, 4, 5, 5, 7, 9];
  it("matches Excel's descriptive statistics", () => {
    const s = describe(xs)!;
    expect(s.count).toBe(8);
    expect(s.mean).toBe(5);
    expect(s.median).toBe(4.5);
    expect(s.mode).toBe(4);
    expect(s.variance).toBeCloseTo(4.571428571, 8);
    expect(s.stdDev).toBeCloseTo(2.138089935, 8);
    expect(s.stdError).toBeCloseTo(0.755928946, 8);
    expect(s.skewness).toBeCloseTo(0.818487553, 6); // Excel SKEW
    expect(s.kurtosis).toBeCloseTo(0.940625, 6); // Excel KURT
    expect(s.range).toBe(7);
    expect(s.sum).toBe(40);
  });
  it("has no mode when every value is unique, and no shape stats for tiny samples", () => {
    const s = describe([1, 2])!;
    expect(s.mode).toBeNull();
    expect(s.skewness).toBeNull();
    expect(s.kurtosis).toBeNull();
    expect(describe([])).toBeNull();
    expect(describe([3])!.variance).toBe(0);
  });
  it("describes columns with headers and skips text", () => {
    const cols = describeColumns(
      [
        ["Jan", "Note", "Feb"],
        [1, "x", 10],
        [3, "y", 30],
      ],
      true,
    );
    expect(cols.map((c) => c.label)).toEqual(["Jan", "Feb"]);
    expect(cols[1].mean).toBe(20);
  });
  it("builds the output block", () => {
    const g = statsGrid([describe([1, 2, 3], "A")!]);
    expect(g[0]).toEqual(["", "A"]);
    expect(g).toHaveLength(STAT_ROWS.length + 1);
    expect(g.find((r) => r[0] === "Mean")![1]).toBe(2);
  });
});
