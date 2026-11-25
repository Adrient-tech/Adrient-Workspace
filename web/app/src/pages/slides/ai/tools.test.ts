import { describe, expect, it } from "vitest";
import { titleSlide, type Slide } from "../model";
import { deckAdapter } from "./deckAdapter";
import { createSlideTools, slideContext } from "./tools";

function deck() {
  let slides: Slide[] = [titleSlide()];
  slides[0].elements[0].text = "Adrient";
  const ad = deckAdapter({ getSlides: () => slides, setSlides: (s) => (slides = s), getCurrent: () => 0 });
  const tools = createSlideTools(ad);
  const run = async (name: string, args: Record<string, unknown>) => tools.find((t) => t.name === name)!.handler(args);
  return { get: () => slides, set: (s: Slide[]) => (slides = s), ad, run, tools };
}

describe("slide tools", () => {
  it("reads the outline", async () => {
    const d = deck();
    const r = await d.run("slide_read_outline", {});
    expect(r.result).toMatchObject({ current: 1, slides: [{ number: 1, title: "Adrient", bullets: ["Click to add subtitle"] }] });
    expect(slideContext(d.ad).label).toBe("Slide 1");
  });

  it("adds slides after the current one and undoes them", async () => {
    const d = deck();
    const r = await d.run("slide_add_slides", { slides: [{ title: "Problem", bullets: ["Too many tools"] }, { title: "Solution", notes: "Say hi" }] });
    expect(d.get().map((s) => d.ad.outline()[d.get().indexOf(s)].title)).toEqual(["Adrient", "Problem", "Solution"]);
    expect(d.get()[1].elements[1]).toMatchObject({ text: "Too many tools", list: "bullet" });
    expect(d.get()[2].notes).toBe("Say hi");
    r.undo!();
    expect(d.get()).toHaveLength(1);
  });

  it("edits text and notes, undo restores only that slide", async () => {
    const d = deck();
    await d.run("slide_add_slides", { slides: [{ title: "Two" }] });
    const r = await d.run("slide_set_text", { slide: 1, title: "Adrient Workspace", bullets: ["Open", "Private"] });
    expect(d.ad.outline()[0]).toMatchObject({ title: "Adrient Workspace", bullets: ["Open", "Private"] });
    const n = await d.run("slide_set_notes", { slide: 2, notes: "Pause here" });
    expect(d.get()[1].notes).toBe("Pause here");
    r.undo!();
    expect(d.ad.outline()[0].title).toBe("Adrient");
    expect(d.get()[1].notes).toBe("Pause here");
    n.undo!();
    expect(d.get()[1].notes).toBeUndefined();
    expect((await d.run("slide_set_text", { slide: 9, title: "x" })).error).toContain("between 1 and 2");
  });

  it("offers layouts only when the editor provides them", () => {
    expect(deck().tools.some((t) => t.name === "slide_apply_layout")).toBe(false);
    let slides: Slide[] = [titleSlide()];
    const ad = deckAdapter({ getSlides: () => slides, setSlides: (s) => (slides = s), getCurrent: () => 0, layouts: () => [{ id: "l1", name: "Title only" }], applyLayout: (s, id) => ({ ...s, layout: id }) });
    const tools = createSlideTools(ad);
    const apply = tools.find((t) => t.name === "slide_apply_layout")!;
    apply.handler({ layout: "Title only" });
    expect(slides[0].layout).toBe("l1");
  });
});
