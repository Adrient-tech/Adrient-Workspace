// Wave 6 Docs features: DOCX fidelity (kept field codes, font effects, RTL
// paragraphs, unused custom styles), readability statistics, sort paragraphs,
// read-aloud chunking, the fidelity metric itself and the new templates.
import { describe, expect, it } from "vitest";
import JSZip from "jszip";
import { makeEditor, paragraphTexts, selectAll } from "./harness";
import { buildDocx } from "./docx-fixture";
import { readDocx } from "../docx/read";
import { writeDocx } from "../docx/write";
import { applyDocxImport, collectDocxInput } from "../docx/apply";
import { aggregate, compareInventories, documentScore, inventory, readParts } from "../docx/fidelity";
import { easeLabel, isPassive, syllables, textStatistics } from "../textStats";
import { leadingNumber, sortSelectedParagraphs, sortedIds } from "../sortParagraphs";
import { splitForSpeech } from "../readAloud";
import { effectsCss, isNoEffects, parseEffectNumber, SCALE_RANGE, SPACING_RANGE } from "../fontEffects";
import { TEMPLATES, templateHtml } from "../templates";

const NS = 'xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"';
const r = (t: string, rPr = "") => `<w:r>${rPr ? `<w:rPr>${rPr}</w:rPr>` : ""}<w:t xml:space="preserve">${t}</w:t></w:r>`;

async function roundTrip(body: string, extra: { styles?: string } = {}) {
  const bytes = await buildDocx({ body, styles: extra.styles });
  const editor = makeEditor("<p></p>");
  applyDocxImport(editor, await readDocx(bytes));
  const input = collectDocxInput(editor, { title: "T" });
  input.now = new Date(0);
  const out = await writeDocx(input);
  const zip = await JSZip.loadAsync(out);
  return { editor, bytes, out, xml: await zip.file("word/document.xml")!.async("string") };
}

describe("docx fidelity (wave 6)", () => {
  it("keeps a document-property field code and its result", async () => {
    const { xml } = await roundTrip(
      `<w:p>${r("By ")}<w:fldSimple w:instr=" AUTHOR ">${r("Ann")}</w:fldSimple> ${"<w:fldSimple w:instr=\" KEYWORDS \">" + r("k1") + "</w:fldSimple>"}</w:p>`,
    );
    expect(xml).toMatch(/AUTHOR/);
    expect(xml).toMatch(/KEYWORDS/);
    expect(xml).toContain("Ann");
  });

  it("keeps all caps, small caps, character spacing, scale, raise and hidden text", async () => {
    const { xml, editor } = await roundTrip(
      `<w:p>${r("CAPS", "<w:caps/>")}${r("small", "<w:smallCaps/>")}${r("wide", '<w:spacing w:val="40"/>')}${r("stretch", '<w:w w:val="150"/>')}${r("up", '<w:position w:val="6"/>')}${r("secret", "<w:vanish/>")}</w:p>`,
    );
    for (const tag of ["<w:caps/>", "<w:smallCaps/>", '<w:spacing w:val="40"/>', '<w:w w:val="150"/>', '<w:position w:val="6"/>', "<w:vanish/>"]) expect(xml).toContain(tag);
    expect(paragraphTexts(editor)[0]).toContain("secret"); // hidden text is kept, not dropped
  });

  it("round-trips a right-to-left paragraph and swaps its logical alignment", async () => {
    const { xml, editor } = await roundTrip(`<w:p><w:pPr><w:bidi/><w:jc w:val="left"/></w:pPr>${r("שלום")}</w:p>`);
    expect(xml).toContain("<w:bidi/>");
    expect(xml).toMatch(/<w:jc w:val="left"\/>/); // logical start, as it came in
    const para = editor.getJSON().content![0];
    expect(para.attrs?.direction).toBe("rtl");
    expect(para.attrs?.textAlign).toBe("right"); // visually right
  });

  it("keeps a custom style that no paragraph uses", async () => {
    const styles = `<w:styles ${NS}><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style><w:style w:type="paragraph" w:customStyle="1" w:styleId="Callout"><w:name w:val="Callout"/><w:basedOn w:val="Normal"/><w:rPr><w:i/></w:rPr></w:style></w:styles>`;
    const out = await roundTrip(`<w:p>${r("plain")}</w:p>`, { styles });
    const zip = await JSZip.loadAsync(out.out);
    expect(await zip.file("word/styles.xml")!.async("string")).toContain('w:styleId="Callout"');
  });

  it("scores structure kept per feature", async () => {
    const { bytes, out } = await roundTrip(`<w:p>${r("a", "<w:b/>")}</w:p><w:p>${r("b", "<w:caps/>")}</w:p>`);
    const scores = compareInventories(inventory(await readParts(bytes)), inventory(await readParts(out)));
    expect(scores.find((s) => s.feature === "fmt-bold")).toMatchObject({ before: 1, kept: 1 });
    expect(documentScore(scores)).toBeGreaterThan(0.95);
    const agg = aggregate([scores, [{ feature: "table", before: 2, kept: 1 }]]);
    expect(agg.perFeature[0]).toMatchObject({ feature: "table", ratio: 0.5 });
  });
});

describe("font effects", () => {
  it("validates and clamps dialog input", () => {
    expect(parseEffectNumber("", SPACING_RANGE)).toBeNull();
    expect(parseEffectNumber("abc", SPACING_RANGE)).toBeNull();
    expect(parseEffectNumber("1,5 pt", SPACING_RANGE)).toBe(1.5);
    expect(parseEffectNumber("900", SCALE_RANGE)).toBe(600);
    expect(parseEffectNumber("-50", SPACING_RANGE)).toBe(-20);
  });

  it("paints effects as CSS and knows when there are none", () => {
    expect(effectsCss({ allCaps: true, spacing: 2, raise: 3 })).toContain("text-transform: uppercase");
    expect(effectsCss({ spacing: 2 })).toContain("letter-spacing: 2pt");
    expect(effectsCss({ scale: 150 })).toContain("scaleX(1.5)");
    expect(isNoEffects({ scale: 100 })).toBe(true);
    expect(isNoEffects({ hidden: true })).toBe(false);
  });

  it("sets and toggles the mark in the editor", () => {
    const e = makeEditor("<p>hello world</p>", { cursor: "all" });
    e.commands.setFontEffects({ spacing: 3 });
    e.commands.toggleFontEffect("allCaps");
    const m = e.getJSON().content![0].content![0].marks!.find((x) => x.type === "fontEffects")!;
    expect(m.attrs).toMatchObject({ allCaps: true, spacing: 3 });
    e.commands.toggleFontEffect("allCaps");
    e.commands.setFontEffects({ spacing: null });
    expect(e.getJSON().content![0].content![0].marks?.some((x) => x.type === "fontEffects")).toBeFalsy();
  });
});

describe("readability statistics", () => {
  it("estimates syllables and passive voice", () => {
    expect(syllables("cat")).toBe(1);
    expect(syllables("table")).toBe(2);
    expect(syllables("radio")).toBe(3);
    expect(isPassive("The report was written by Ann.")).toBe(true);
    expect(isPassive("Ann wrote the report.")).toBe(false);
  });

  it("computes Flesch scores for a simple text", () => {
    const s = textStatistics("The cat sat on the mat. The dog ran to the park.");
    expect(s.sentences).toBe(2);
    expect(s.words).toBe(12);
    expect(s.readability.fleschReadingEase).toBeGreaterThan(90);
    expect(easeLabel(s.readability.fleschReadingEase)).toBe("Very easy");
    expect(textStatistics("").readability.fleschReadingEase).toBe(0);
  });
});

describe("sort paragraphs", () => {
  it("reads leading numbers", () => {
    expect(leadingNumber("3. Pears")).toBe(3);
    expect(leadingNumber("$1,200 paid")).toBe(1200);
    expect(leadingNumber("1,5 kg")).toBe(1.5);
    expect(leadingNumber("Room A-5")).toBe(5);
    expect(leadingNumber("none")).toBeNull();
  });

  it("orders text naturally and numbers with blanks last", () => {
    const items = [
      { id: 0, text: "item 10" },
      { id: 1, text: "Item 2" },
      { id: 2, text: "apple" },
    ];
    expect(sortedIds(items, "textAsc")).toEqual([2, 1, 0]);
    expect(sortedIds(items, "textDesc")).toEqual([0, 1, 2]);
    expect(sortedIds(items, "numberDesc")).toEqual([0, 1, 2]);
    expect(sortedIds(items, "numberAsc")).toEqual([1, 0, 2]);
  });

  it("sorts the selected paragraphs in the editor, keeping their formatting", () => {
    const e = makeEditor("<p>pear</p><p><strong>apple</strong></p><p>fig</p>", { cursor: "all" });
    selectAll(e);
    expect(sortSelectedParagraphs(e, "textAsc")).toBeGreaterThan(0);
    expect(paragraphTexts(e)).toEqual(["apple", "fig", "pear"]);
    expect(JSON.stringify(e.getJSON().content![0])).toContain("bold");
    expect(() => {
      e.commands.setTextSelection(2);
      sortSelectedParagraphs(e, "textAsc");
    }).toThrow(/at least two/);
  });
});

describe("read aloud", () => {
  it("cuts text at sentence breaks within the limit", () => {
    const chunks = splitForSpeech("One. Two. Three.\nFour is here.", 12);
    expect(chunks.every((c) => c.length <= 12)).toBe(true);
    expect(chunks.join(" ")).toContain("Four is here.");
    expect(splitForSpeech("   ")).toEqual([]);
  });

  it("splits overlong runs at word breaks and never inside a surrogate pair", () => {
    const text = "word ".repeat(40) + "😀".repeat(10);
    const chunks = splitForSpeech(text, 30);
    expect(chunks.every((c) => c.length <= 31)).toBe(true);
    expect(chunks.join("")).not.toMatch(/[\ud800-\udbff]$/m);
  });
});

describe("templates", () => {
  it("offers report, resume and essay starters with the date filled in", () => {
    for (const id of ["report", "resume", "essay"]) {
      const t = TEMPLATES.find((x) => x.id === id)!;
      expect(t).toBeTruthy();
      expect(templateHtml(t)).not.toContain("{date}");
      expect(t.html).toContain("<h");
    }
  });
});
