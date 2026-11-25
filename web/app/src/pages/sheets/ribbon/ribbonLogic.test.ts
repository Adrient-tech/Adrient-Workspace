import { describe, expect, it, vi } from "vitest";
import {
  BORDER_KINDS,
  CELL_STYLES,
  NUMBER_FORMAT_GALLERY,
  applyBorder,
  applyCellStyle,
  applyMerge,
  borderEntries,
  captureFormat,
  paintFormat,
  previewFormat,
  stepDecimals,
  visibleTabs,
} from "./ribbonLogic";

function mockWb(sel = [{ row: [1, 2], column: [0, 1] }], cell: any = {}) {
  const sheet: any = { id: "s1", config: {} };
  return {
    sheet,
    getSelection: () => sel,
    getSheet: () => sheet,
    getAllSheets: () => [sheet],
    setCellFormatByRange: vi.fn(),
    getCellsByRange: vi.fn(() => [[cell]]),
    getCellValue: vi.fn(),
    mergeCells: vi.fn(),
    cancelMerge: vi.fn(),
    applyOp: vi.fn((ops: any[]) => {
      for (const o of ops) sheet[o.path[0]] = o.value;
    }),
  };
}

describe("ribbon tabs", () => {
  it("hides tabs that are not built", () => {
    const ids = visibleTabs().map((t) => t.id);
    expect(ids).toContain("home");
    expect(ids).not.toContain("draw");
    expect(ids).not.toContain("automate");
  });
});

describe("cell styles", () => {
  it("has unique ids", () => {
    expect(new Set(CELL_STYLES.map((s) => s.id)).size).toBe(CELL_STYLES.length);
  });
  it("applies colours and clears unset attributes", () => {
    const w = mockWb();
    applyCellStyle(w, CELL_STYLES.find((s) => s.id === "good")!);
    const calls = w.setCellFormatByRange.mock.calls;
    expect(calls).toContainEqual(["fc", "#006100", expect.anything()]);
    expect(calls).toContainEqual(["bg", "#C6EFCE", expect.anything()]);
    expect(calls).toContainEqual(["bl", undefined, expect.anything()]);
  });
  it("number styles set a format", () => {
    const w = mockWb();
    applyCellStyle(w, CELL_STYLES.find((s) => s.id === "percent")!);
    expect(w.setCellFormatByRange).toHaveBeenCalledWith("ct", { fa: "0.00%", t: "n" }, expect.anything());
  });
});

describe("borders", () => {
  it("builds one entry per range", () => {
    const e = borderEntries("all", [{ row: [0, 1], column: [0, 2] }]);
    expect(e).toHaveLength(1);
    expect(e[0].borderType).toBe("border-all");
  });
  it("thick outside uses the thick style", () => {
    const e = borderEntries("thick-outer", [{ row: [0, 0], column: [0, 0] }]);
    expect(e[0]).toMatchObject({ borderType: "border-outside", style: "8" });
  });
  it("every kind produces a valid entry", () => {
    for (const k of BORDER_KINDS) expect(borderEntries(k.id, [{ row: [0, 0], column: [0, 0] }])[0].borderType).toMatch(/^border-/);
  });
  it("appends to the sheet config", () => {
    const w = mockWb();
    applyBorder(w, "bottom", "#ff0000");
    applyBorder(w, "top");
    expect(w.sheet.config.borderInfo).toHaveLength(2);
    expect(w.sheet.config.borderInfo[0].color).toBe("#ff0000");
  });
});

describe("merge", () => {
  it("maps variants to FortuneSheet types", () => {
    const w = mockWb();
    applyMerge(w, "all");
    applyMerge(w, "horizontal");
    applyMerge(w, "vertical");
    applyMerge(w, "unmerge");
    expect(w.mergeCells.mock.calls.map((c: any) => c[1])).toEqual(["merge-all", "merge-horizontal", "merge-vertical"]);
    expect(w.cancelMerge).toHaveBeenCalledOnce();
  });
});

describe("format painter", () => {
  it("copies and applies attributes", () => {
    const src = mockWb([{ row: [0, 0], column: [0, 0] }], { bl: 1, fc: "#f00", ct: { fa: "0.00%", t: "n" } });
    const f = captureFormat(src)!;
    expect(f.attrs.bl).toBe(1);
    expect(f.ct?.fa).toBe("0.00%");
    const dst = mockWb();
    paintFormat(dst, f);
    expect(dst.setCellFormatByRange).toHaveBeenCalledWith("fc", "#f00", expect.anything());
    expect(dst.setCellFormatByRange).toHaveBeenCalledWith("ct", { fa: "0.00%", t: "n" }, expect.anything());
  });
  it("returns null without a selection", () => {
    expect(captureFormat(mockWb([]))).toBeNull();
  });
});

describe("number format gallery", () => {
  it("previews every entry", () => {
    for (const g of NUMBER_FORMAT_GALLERY) expect(previewFormat(g.fa, g.sample)).not.toBe("");
  });
  it("renders known samples", () => {
    expect(previewFormat("0.00%", 0.1234)).toBe("12.34%");
    expect(previewFormat("#,##0.00", 1234.5)).toBe("1,234.50");
    expect(previewFormat("$#,##0.00", 1234.5)).toBe("$1,234.50");
    expect(previewFormat("0.00E+00", 1234.5)).toBe("1.23E+03");
  });
  it("steps decimals", () => {
    expect(stepDecimals("General", 1)).toBe("0.0");
    expect(stepDecimals("0.00", 1)).toBe("0.000");
    expect(stepDecimals("0.00", -1)).toBe("0.0");
    expect(stepDecimals("0.0", -1)).toBe("0");
    expect(stepDecimals("0", -1)).toBe("0");
    expect(stepDecimals("#,##0.00", 1)).toBe("#,##0.000");
    expect(stepDecimals("0.00%", -1)).toBe("0.0%");
  });
});
