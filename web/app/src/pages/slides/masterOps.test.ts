import { describe, expect, it } from "vitest";
import { defaultDeck, newSlide, type DeckDoc } from "./model";
import { layoutsOf } from "./layouts";
import { addPlaceholder, deleteLayout, duplicateLayout, layoutToSlide, layoutUsage, moveLayout, newLayout, renameLayout, slideToLayout } from "./masterOps";

const deck = (): DeckDoc => ({ slides: [{ ...newSlide(), layout: "obj" }, { ...newSlide(), layout: "title" }] });

describe("master ops", () => {
  it("round-trips a layout through its editing slide", () => {
    const l = layoutsOf(deck())[1];
    const s = layoutToSlide(l);
    expect(s.id).toBe(l.id);
    const back = slideToLayout(l, { ...s, background: "#102030" });
    expect(back.background).toBe("#102030");
    expect(back.elements).toBe(l.elements);
  });
  it("adds placeholders with unique ids and indexes", () => {
    const d = deck();
    let l = layoutsOf(d).find((x) => x.id === "titleOnly")!;
    const before = l.elements.length;
    l = addPlaceholder(l, "body", d);
    l = addPlaceholder(l, "body", d);
    l = addPlaceholder(l, "pic", d);
    expect(l.elements).toHaveLength(before + 3);
    expect(new Set(l.elements.map((e) => e.id)).size).toBe(l.elements.length);
    const idxs = l.elements.filter((e) => e.placeholder?.type === "body").map((e) => e.placeholder!.idx);
    expect(new Set(idxs).size).toBe(idxs.length);
    expect(l.elements.at(-1)!.type).toBe("image");
  });
  it("creates, duplicates, renames and moves layouts", () => {
    let d = deck();
    const n0 = layoutsOf(d).length;
    const nl = newLayout(d, "  Agenda ");
    d = { ...d, layouts: [...layoutsOf(d), nl] };
    expect(nl.name).toBe("Agenda");
    d = duplicateLayout(d, nl.id);
    expect(layoutsOf(d)).toHaveLength(n0 + 2);
    expect(new Set(layoutsOf(d).map((l) => l.id)).size).toBe(n0 + 2);
    d = renameLayout(d, nl.id, "Agenda 2");
    expect(layoutsOf(d).find((l) => l.id === nl.id)!.name).toBe("Agenda 2");
    const m = moveLayout(d, nl.id, -1);
    expect(layoutsOf(m).findIndex((l) => l.id === nl.id)).toBe(layoutsOf(d).findIndex((l) => l.id === nl.id) - 1);
    expect(moveLayout(d, layoutsOf(d)[0].id, -1)).toBe(d);
  });
  it("deleting a layout unlinks its slides and keeps at least one layout", () => {
    let d = deck();
    expect(layoutUsage(d)).toEqual({ obj: 1, title: 1 });
    d = deleteLayout(d, "obj");
    expect(d.slides[0].layout).toBeUndefined();
    expect(layoutsOf(d).some((l) => l.id === "obj")).toBe(false);
    const one: DeckDoc = { ...defaultDeck(), layouts: [layoutsOf(d)[0]] };
    expect(deleteLayout(one, one.layouts![0].id)).toBe(one);
  });
});
