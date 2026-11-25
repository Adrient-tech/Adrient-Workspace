// Tests for the logic behind the Docs ribbon additions: table sort and
// formulas, citations, building blocks, smart chips, accessibility checker,
// AutoSave state and the offline flag.
import { describe, expect, it, beforeEach } from "vitest";
import type { Editor } from "@tiptap/core";
import { TableMap } from "@tiptap/pm/tables";
import { makeEditor } from "./harness";
import { compareCells, evalTableFn, formatResult, formulaRange, insertTableFormula, parseNumber, sortOrder, sortTable } from "../tableOps";
import { bibliographyHtml, formatReference, inTextCitation, parseAuthors, type Source } from "../citations";
import { BUILDING_BLOCKS, blockHtml } from "../buildingBlocks";
import { chipHtml, dateChip, personChip } from "../chips";
import { checkAccessibility } from "../a11y";
import { nextSaveState } from "../saveStatus";
import { isOffline, offlineIds, setOffline } from "../offline";
import { speechCtor } from "../voice";

const tbl = (rows: string[][], header = true) =>
  `<table><tbody>${rows
    .map((r, i) => {
      const t = header && i === 0 ? "th" : "td";
      return `<tr>${r.map((c) => `<${t}><p>${c}</p></${t}>`).join("")}</tr>`;
    })
    .join("")}</tbody></table>`;

function caretIn(e: Editor, r: number, c: number) {
  let tablePos = -1;
  e.state.doc.descendants((n, pos) => {
    if (tablePos < 0 && n.type.name === "table") tablePos = pos;
    return tablePos < 0;
  });
  const t = e.state.doc.nodeAt(tablePos)!;
  const map = TableMap.get(t);
  e.commands.setTextSelection(tablePos + 1 + map.map[r * map.width + c] + 2);
}
const cellText = (e: Editor) => {
  const out: string[][] = [];
  e.state.doc.descendants((n) => {
    if (n.type.name === "tableRow") {
      const r: string[] = [];
      n.forEach((c) => r.push(c.textContent));
      out.push(r);
    }
    return true;
  });
  return out;
};

describe("table numbers and compare", () => {
  it("parses currency, percent, negatives", () => {
    expect(parseNumber("$1,234.50")).toBe(1234.5);
    expect(parseNumber("(5)")).toBe(-5);
    expect(parseNumber("12%")).toBe(12);
    expect(parseNumber("abc")).toBeNull();
    expect(parseNumber("")).toBeNull();
  });
  it("sorts numbers numerically, empties last, stable", () => {
    expect(sortOrder(["10", "9", "", "100"], "number", true)).toEqual([1, 0, 3, 2]);
    expect(sortOrder(["10", "9", "", "100"], "number", false)).toEqual([3, 0, 1, 2]);
    expect(compareCells("b", "A", "text")).toBeGreaterThan(0);
    expect(compareCells("2026-01-02", "2025-12-31", "date")).toBeGreaterThan(0);
  });
  it("evaluates functions", () => {
    expect(evalTableFn("SUM", ["1", "2.5", "x"])).toBe(3.5);
    expect(evalTableFn("AVERAGE", ["2", "4"])).toBe(3);
    expect(evalTableFn("MAX", ["2", "9", "4"])).toBe(9);
    expect(evalTableFn("COUNT", ["a", "1", "2"])).toBe(2);
    expect(evalTableFn("MIN", ["a"])).toBeNull();
    expect(formatResult(0.1 + 0.2)).toBe("0.3");
  });
  it("formula range stops at the first empty cell after numbers", () => {
    const rows = [["h"], ["1"], ["2"], [""], ["9"], [""]];
    expect(formulaRange(rows, 5, 0, "above")).toEqual(["9"]);
  });
});

describe("table sort / formula commands", () => {
  it("sorts body rows by the caret column keeping the header", () => {
    const e = makeEditor(tbl([["Name", "Qty"], ["b", "10"], ["a", "9"], ["c", "100"]]));
    caretIn(e, 1, 1);
    expect(sortTable(e, { type: "number", asc: true })).toBe(true);
    expect(cellText(e).map((r) => r[0])).toEqual(["Name", "a", "b", "c"]);
    caretIn(e, 1, 0);
    sortTable(e, { type: "text", asc: false });
    expect(cellText(e).map((r) => r[0])).toEqual(["Name", "c", "b", "a"]);
  });
  it("writes SUM of cells above into the caret cell", () => {
    const e = makeEditor(tbl([["Item", "Cost"], ["x", "10"], ["y", "5.5"], ["Total", ""]]));
    caretIn(e, 3, 1);
    expect(insertTableFormula(e, "SUM", "above")).toBe("15.5");
    expect(cellText(e)[3]).toEqual(["Total", "15.5"]);
  });
  it("outside a table does nothing", () => {
    const e = makeEditor("<p>hi</p>");
    expect(sortTable(e)).toBe(false);
    expect(insertTableFormula(e, "SUM", "left")).toBeNull();
  });
});

const book: Source = { id: "1", kind: "book", authors: "Smith, Jane; Lee, Tom", title: "Writing well", year: "2020", publisher: "Acme Press" };
const art: Source = { id: "2", kind: "article", authors: "Doe, John Adam", title: "On things", year: "2019", container: "Journal of Stuff", volume: "4", pages: "10-20" };

describe("citations", () => {
  it("parses authors in both orders", () => {
    expect(parseAuthors("Smith, Jane; Tom Lee")).toEqual([
      { last: "Smith", first: "Jane" },
      { last: "Lee", first: "Tom" },
    ]);
  });
  it("formats in-text citations per style", () => {
    expect(inTextCitation(book, "apa", "4")).toBe("(Smith & Lee, 2020, p. 4)");
    expect(inTextCitation(book, "mla", "4")).toBe("(Smith and Lee 4)");
    expect(inTextCitation(book, "chicago")).toBe("(Smith and Lee 2020)");
    expect(inTextCitation({ ...book, authors: "A, B; C, D; E, F" }, "apa")).toBe("(A et al., 2020)");
  });
  it("formats references", () => {
    expect(formatReference(book, "apa")).toBe("Smith, J., & Lee, T. (2020). *Writing well.* Acme Press.");
    expect(formatReference(art, "apa")).toBe("Doe, J. A. (2019). On things. *Journal of Stuff*, *4*, 10-20.");
    expect(formatReference(book, "mla")).toContain("Smith, Jane, and Tom Lee. *Writing well.* Acme Press, 2020.");
    expect(formatReference(art, "chicago")).toContain('2019. "On things." *Journal of Stuff* 4: 10-20.');
  });
  it("builds a sorted bibliography with a heading", () => {
    const html = bibliographyHtml([art, book], "mla");
    expect(html.startsWith("<h1>Works Cited</h1>")).toBe(true);
    expect(html.indexOf("Doe")).toBeLessThan(html.indexOf("Smith"));
    expect(html).toContain("<em>");
  });
});

describe("building blocks and chips", () => {
  it("every block has html, and the cover page uses the page-break node", () => {
    for (const b of BUILDING_BLOCKS) expect(blockHtml(b).length).toBeGreaterThan(10);
    expect(blockHtml(BUILDING_BLOCKS.find((b) => b.id === "cover")!)).toContain("data-page-break");
    expect(BUILDING_BLOCKS.map((b) => b.id)).toEqual(expect.arrayContaining(["resume", "letter", "report", "meeting-notes"]));
    expect(blockHtml(BUILDING_BLOCKS.find((b) => b.id === "letter")!)).not.toContain("{date}");
  });
  it("inserts a block into the editor", () => {
    const e = makeEditor("<p></p>");
    e.commands.insertContent(blockHtml(BUILDING_BLOCKS.find((b) => b.id === "report")!));
    expect(e.getText()).toContain("Executive summary");
    expect(e.getJSON().content!.some((n) => n.type === "heading")).toBe(true);
  });
  it("chips are links with a chip title and survive in the editor", () => {
    expect(personChip("Ann", "a@x.io")).toContain('href="mailto:a@x.io"');
    expect(dateChip(new Date("2026-03-04T12:00:00Z"))).toContain("/calendar?date=2026-03-04");
    expect(chipHtml("app", "<b>", "/crm")).toContain("&lt;b&gt;");
    const e = makeEditor("<p></p>");
    e.commands.insertContent(chipHtml("app", "CRM", "/crm"));
    expect(e.getHTML()).toContain('title="chip:app"');
  });
});

describe("accessibility checker", () => {
  it("flags skipped headings, empty headings, vague links and header-less tables", () => {
    const e = makeEditor(`<h1>A</h1><h3>B</h3><h2></h2><p><a href="/x">click here</a></p>${tbl([["a", "b"], ["1", "2"]], false)}`);
    const rules = checkAccessibility(e.state.doc).map((i) => i.rule);
    expect(rules).toEqual(expect.arrayContaining(["heading-skip", "heading-empty", "link-text", "table-header"]));
  });
  it("is clean for a tidy document", () => {
    const e = makeEditor(`<h1>A</h1><h2>B</h2><p>text</p>${tbl([["a", "b"], ["1", "2"]])}`);
    expect(checkAccessibility(e.state.doc)).toEqual([]);
  });
});

describe("autosave state and offline flag", () => {
  beforeEach(() => localStorage.clear());
  it("maps connection and dirty flags to a state", () => {
    expect(nextSaveState(true, false, true)).toBe("saved");
    expect(nextSaveState(true, true, true)).toBe("saving");
    expect(nextSaveState(false, true, true)).toBe("offline");
    expect(nextSaveState(false, false, false)).toBe("connecting");
  });
  it("tracks the per-document offline flag", () => {
    expect(isOffline("d1")).toBe(false);
    setOffline("d1", true);
    setOffline("d2", true);
    expect(offlineIds().sort()).toEqual(["d1", "d2"]);
    setOffline("d1", false);
    expect(isOffline("d1")).toBe(false);
    expect(isOffline("d2")).toBe(true);
  });
  it("voice typing is only offered when the browser has speech recognition", () => {
    expect(speechCtor({})).toBeNull();
    class R {}
    expect(speechCtor({ webkitSpeechRecognition: R })).toBe(R);
  });
});
