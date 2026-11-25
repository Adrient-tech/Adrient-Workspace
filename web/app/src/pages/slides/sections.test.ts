import { describe, expect, it } from "vitest";
import { newSlide, type DeckDoc } from "./model";
import {
  addSection, deleteSectionWithSlides, moveSection, normalizeSections, reanchorSections,
  removeSection, renameSection, sectionOfSlide, sectionRanges,
} from "./sections";

const mk = (n: number): DeckDoc => ({ slides: Array.from({ length: n }, () => newSlide()) });

describe("sections", () => {
  it("has one implicit group without sections", () => {
    const d = mk(3);
    expect(sectionRanges(d)).toEqual([{ section: null, start: 0, end: 3 }]);
  });
  it("adds sections and ranges cover every slide", () => {
    let d = mk(6);
    d = addSection(d, d.slides[2].id, "Intro");
    d = addSection(d, d.slides[4].id, "Body");
    const r = sectionRanges(d);
    expect(r.map((x) => [x.section?.name ?? null, x.start, x.end])).toEqual([[null, 0, 2], ["Intro", 2, 4], ["Body", 4, 6]]);
    expect(sectionOfSlide(d, d.slides[3].id)?.name).toBe("Intro");
    expect(sectionOfSlide(d, d.slides[0].id)).toBeNull();
  });
  it("adding on an existing start renames it", () => {
    let d = mk(2);
    d = addSection(d, d.slides[0].id, "A");
    d = addSection(d, d.slides[0].id, "B");
    expect(d.sections).toHaveLength(1);
    expect(d.sections![0].name).toBe("B");
  });
  it("rename ignores blanks; remove clears the field", () => {
    let d = addSection(mk(2), "x", "no");
    expect(d.sections).toBeUndefined();
    d = addSection(mk(2), mk(2).slides[0].id);
    const d2 = mk(2);
    const d3 = addSection(d2, d2.slides[1].id, "S");
    expect(renameSection(d3, d3.sections![0].id, "  ")).toBe(d3);
    expect(removeSection(d3, d3.sections![0].id).sections).toBeUndefined();
  });
  it("moves a section with its slides", () => {
    let d = mk(4);
    const ids = d.slides.map((s) => s.id);
    d = addSection(d, ids[0], "A");
    d = addSection(d, ids[2], "B");
    const moved = moveSection(d, d.sections![1].id, -1);
    expect(moved.slides.map((s) => s.id)).toEqual([ids[2], ids[3], ids[0], ids[1]]);
    expect(normalizeSections(moved).map((s) => s.name)).toEqual(["B", "A"]);
  });
  it("deletes a section with its slides but never the last slide", () => {
    let d = mk(3);
    d = addSection(d, d.slides[1].id, "B");
    expect(deleteSectionWithSlides(d, d.sections![0].id).slides).toHaveLength(1);
    let one = mk(2);
    one = addSection(one, one.slides[0].id, "All");
    expect(deleteSectionWithSlides(one, one.sections![0].id)).toBe(one);
  });
  it("re-anchors a section whose first slide was deleted", () => {
    let d = mk(4);
    d = addSection(d, d.slides[1].id, "B");
    const next = d.slides.filter((_, i) => i !== 1);
    const secs = reanchorSections(d, next)!;
    expect(secs[0].slideId).toBe(d.slides[2].id);
  });
});
