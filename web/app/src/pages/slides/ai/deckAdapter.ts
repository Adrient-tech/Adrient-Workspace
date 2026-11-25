/**
 * SlidesAdapter over the editor's Slide[] model. Undo restores the slides it
 * touched by id, so edits to other slides in between are kept.
 */
import type { Slide, SlideElement } from "../model";
import type { NewSlide, SlideOutline, SlidesAdapter, Undo } from "./tools";

export interface DeckAdapterOptions {
  getSlides: () => Slide[];
  setSlides: (slides: Slide[]) => void;
  /** 0-based index of the current slide. */
  getCurrent: () => number;
  layouts?: () => { id: string; name: string }[];
  applyLayout?: (slide: Slide, layoutId: string) => Slide;
}

const id = () => Math.random().toString(36).slice(2, 10);

const textEls = (s: Slide) => s.elements.filter((e) => e.type === "text" && typeof e.text === "string");

/** The title element: a title placeholder, else the largest-font text box. */
export function titleElement(s: Slide): SlideElement | undefined {
  const els = textEls(s);
  return (
    els.find((e) => e.placeholder?.type === "title" || e.placeholder?.type === "ctrTitle") ??
    [...els].sort((a, b) => (b.fontSize ?? 18) - (a.fontSize ?? 18) || a.y - b.y)[0]
  );
}

function bodyElement(s: Slide): SlideElement | undefined {
  const t = titleElement(s);
  return textEls(s).find((e) => e !== t && e.placeholder?.type !== "dt" && e.placeholder?.type !== "ftr" && e.placeholder?.type !== "sldNum");
}

function makeSlide(n: NewSlide, background: string): Slide {
  const els: SlideElement[] = [
    { id: id(), type: "text", x: 60, y: 36, w: 840, h: 80, text: n.title, fontSize: 32, bold: true, color: "#202124", align: "left", valign: "middle", fontFamily: "Arial", placeholder: { type: "title" } },
  ];
  if (n.bullets?.length) {
    els.push({ id: id(), type: "text", x: 60, y: 136, w: 840, h: 360, text: n.bullets.join("\n"), fontSize: 20, color: "#3c4043", align: "left", valign: "top", fontFamily: "Arial", list: "bullet", placeholder: { type: "body" } });
  }
  return { id: id(), background, elements: els, notes: n.notes };
}

export function deckAdapter(o: DeckAdapterOptions): SlidesAdapter {
  const restore = (before: Slide[]): Undo => () => {
    const byId = new Map(before.map((s) => [s.id, s]));
    o.setSlides(o.getSlides().map((s) => byId.get(s.id) ?? s));
  };
  const edit = (n: number, f: (s: Slide) => Slide): Undo | string => {
    const slides = o.getSlides();
    const s = slides[n - 1];
    if (!s) return "no such slide";
    const before = JSON.parse(JSON.stringify(s)) as Slide;
    o.setSlides(slides.map((x, i) => (i === n - 1 ? f(x) : x)));
    return restore([before]);
  };
  const ad: SlidesAdapter = {
    outline: (): SlideOutline[] =>
      o.getSlides().map((s, i) => {
        const b = bodyElement(s);
        return { number: i + 1, title: titleElement(s)?.text ?? "", bullets: b?.text ? b.text.split("\n").filter(Boolean) : [], notes: s.notes ?? "", layout: s.layout };
      }),
    current: () => o.getCurrent() + 1,
    addSlides: (after, list) => {
      const slides = o.getSlides();
      const bg = slides[Math.max(0, after - 1)]?.background || "#ffffff";
      const made = list.map((n) => makeSlide(n, bg));
      o.setSlides([...slides.slice(0, after), ...made, ...slides.slice(after)]);
      const ids = new Set(made.map((s) => s.id));
      return () => o.setSlides(o.getSlides().filter((s) => !ids.has(s.id)));
    },
    setText: (n, patch) =>
      edit(n, (s) => {
        let next: Slide = { ...s, elements: [...s.elements] };
        const set = (el: SlideElement | undefined, text: string, make: () => SlideElement) => {
          if (el) next.elements = next.elements.map((e) => (e.id === el.id ? { ...e, text, runs: undefined } : e));
          else next = { ...next, elements: [...next.elements, make()] };
        };
        if (patch.title !== undefined) set(titleElement(s), patch.title, () => makeSlide({ title: patch.title! }, s.background).elements[0]);
        if (patch.bullets !== undefined) set(bodyElement(s), patch.bullets.join("\n"), () => makeSlide({ title: "", bullets: patch.bullets }, s.background).elements[1]);
        return next;
      }),
    setNotes: (n, notes) => edit(n, (s) => ({ ...s, notes })),
  };
  if (o.layouts && o.applyLayout) {
    const layouts = o.layouts, apply = o.applyLayout;
    ad.layouts = () => layouts().map((l) => l.name);
    ad.applyLayout = (n, name) => {
      const l = layouts().find((x) => x.name === name);
      return l ? edit(n, (s) => apply(s, l.id)) : "unknown layout";
    };
  }
  return ad;
}
