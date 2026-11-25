import { describe, expect, it } from "vitest";
import { deckToPptx } from "./write";
import { readPptx } from "./read";
import { newElement, newSlide, type DeckDoc, type Slide } from "../model";
import { addEffect } from "../animOps";
import { newDiagram } from "../diagrams";
import { OFFICE_THEME } from "../theme";
import { addSection } from "../sections";
import { layoutsOf } from "../layouts";
import { newLayout } from "../masterOps";

describe("pptx fidelity of a wave-4 deck", () => {
  it("keeps master layouts, animations on shapes and groups, transitions, sections, timings", async () => {
    const custom = { ...newLayout({ slides: [newSlide()] }, "Brand content"), background: "#eef7f4" };
    const rect = newElement("rect");
    const diag = newDiagram("process", "A\nB\nC", { x: 60, y: 120, w: 840, h: 300 }, OFFICE_THEME);
    let s1: Slide = { ...newSlide(), elements: [rect], layout: custom.id, transition: "push" as const, advanceAfter: 4000 };
    s1 = addEffect(s1, [rect.id], "entr", "fly").slide;
    let s2: Slide = { ...newSlide(), elements: [diag] };
    s2 = addEffect(s2, [diag.id], "entr", "fade").slide;
    let deck: DeckDoc = { slides: [s1, s2, newSlide()], layouts: [...layoutsOf({}), custom] };
    deck = addSection(deck, deck.slides[1].id, "Part two");
    const back = (await readPptx(await deckToPptx(deck))).deck;
    const report = {
      layouts: back.layouts?.some((l) => l.name === "Brand content"),
      shapeAnim: (back.slides[0].anims ?? []).map((a) => a.kind),
      groupAnim: (back.slides[1].anims ?? []).map((a) => a.kind),
      transition: back.slides[0].transition,
      advanceAfter: back.slides[0].advanceAfter,
      sections: back.sections?.map((s) => s.name),
    };
    console.log("FIDELITY", JSON.stringify(report));
    expect(report.layouts).toBe(true);
    expect(report.shapeAnim).toEqual(["fly"]);
    expect(report.transition).toBe("push");
    expect(report.advanceAfter).toBe(4000);
    expect(report.sections).toEqual(["Part two"]);
    expect(report.groupAnim).toEqual(["fade"]);
  }, 30000);
});
