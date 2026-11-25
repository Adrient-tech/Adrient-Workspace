import { describe, expect, it } from "vitest";
import { PDFDocument } from "pdf-lib";
import { chunkPages, diffSummary, diffWords, extractPages, humanSize, imagesToPdf, optimizePdf, pageCountOf, parseRanges, sizeChange, splitGroups } from "./pdfTools";

async function makePdf(widths: number[]): Promise<Uint8Array> {
  const d = await PDFDocument.create();
  for (const w of widths) d.addPage([w, 200]);
  return d.save();
}

describe("page ranges", () => {
  it("parses pages, ranges and open ends in order without duplicates", () => {
    expect(parseRanges("1-3, 5", 10)).toEqual([0, 1, 2, 4]);
    expect(parseRanges("8-", 10)).toEqual([7, 8, 9]);
    expect(parseRanges("-2", 10)).toEqual([0, 1]);
    expect(parseRanges("3, 1-3", 10)).toEqual([2, 0, 1]);
  });
  it("explains bad input", () => {
    expect(() => parseRanges("", 5)).toThrow(/at least one/);
    expect(() => parseRanges("abc", 5)).toThrow(/not a page/);
    expect(() => parseRanges("0", 5)).toThrow(/outside/);
    expect(() => parseRanges("2-9", 5)).toThrow(/outside/);
    expect(() => parseRanges("4-2", 5)).toThrow(/backwards/);
    expect(() => parseRanges("-", 5)).toThrow();
  });
  it("splits into groups and chunks", () => {
    expect(splitGroups("1-2, 4; 6", 6)).toEqual([[0, 1], [3], [5]]);
    expect(chunkPages(5, 2)).toEqual([[0, 1], [2, 3], [4]]);
    expect(() => chunkPages(5, 0)).toThrow();
  });
});

describe("pdf operations", () => {
  it("extracts pages in the requested order", async () => {
    const src = await makePdf([100, 200, 300, 400]);
    const out = await extractPages(src, [3, 0]);
    const doc = await PDFDocument.load(out);
    expect(doc.getPageCount()).toBe(2);
    expect(doc.getPage(0).getWidth()).toBe(400);
    expect(doc.getPage(1).getWidth()).toBe(100);
    expect(await pageCountOf(src)).toBe(4);
  });

  it("optimises without losing pages and can strip metadata", async () => {
    const d = await PDFDocument.create();
    d.setTitle("Secret plan");
    d.setAuthor("Ann");
    for (let i = 0; i < 20; i++) d.addPage([300, 300]).drawText(`Page ${i} ` + "lorem ipsum ".repeat(40), { x: 10, y: 250, size: 8 });
    const src = await d.save({ useObjectStreams: false });
    const r = await optimizePdf(src, true);
    const doc = await PDFDocument.load(r.bytes);
    expect(doc.getPageCount()).toBe(20);
    expect(doc.getTitle() ?? "").toBe("");
    expect(doc.getAuthor() ?? "").toBe("");
    expect(r.after).toBeLessThanOrEqual(r.before);
    expect(r.before).toBe(src.byteLength);
  });

  it("builds a PDF from page images at the page size", async () => {
    // 1x1 PNG
    const png = Uint8Array.from(atob("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="), (c) => c.charCodeAt(0));
    const out = await imagesToPdf([{ bytes: png, type: "png", width: 300, height: 400 }, { bytes: png, type: "png", width: 500, height: 200 }]);
    const doc = await PDFDocument.load(out);
    expect(doc.getPageCount()).toBe(2);
    expect([doc.getPage(1).getWidth(), doc.getPage(1).getHeight()]).toEqual([500, 200]);
  });

  it("formats sizes", () => {
    expect(humanSize(900)).toBe("900 B");
    expect(humanSize(2048)).toBe("2.0 KB");
    expect(humanSize(3 * 1024 * 1024)).toBe("3.00 MB");
    expect(sizeChange(1000, 750)).toBe("-25%");
    expect(sizeChange(1000, 1000)).toBe("no change");
    expect(sizeChange(1000, 1100)).toBe("+10%");
  });
});

describe("compare", () => {
  it("marks added and removed words", () => {
    const parts = diffWords("The fee is 100 dollars per month", "The fee is 150 dollars per year");
    const added = parts.filter((p) => p.kind === "added").map((p) => p.text);
    const removed = parts.filter((p) => p.kind === "removed").map((p) => p.text);
    expect(removed).toEqual(["100", "month"]);
    expect(added).toEqual(["150", "year"]);
    expect(diffSummary(parts)).toEqual({ added: 2, removed: 2, same: 5 });
  });
  it("handles identical, empty and totally different texts", () => {
    expect(diffWords("a b c", "a b c")).toEqual([{ text: "a b c", kind: "same" }]);
    expect(diffWords("", "")).toEqual([]);
    expect(diffSummary(diffWords("", "x y"))).toEqual({ added: 2, removed: 0, same: 0 });
    expect(diffSummary(diffWords("x y", "p q"))).toEqual({ added: 2, removed: 2, same: 0 });
  });
  it("does not freeze on huge inputs", () => {
    const big = Array.from({ length: 3000 }, (_, i) => `w${i}`).join(" ");
    const big2 = big.replace("w1500", "CHANGED");
    const t = Date.now();
    const parts = diffWords(big, big2, 1_000_000); // forces the coarse path
    expect(Date.now() - t).toBeLessThan(2000);
    expect(parts.some((p) => p.kind !== "same")).toBe(true);
  });
});
