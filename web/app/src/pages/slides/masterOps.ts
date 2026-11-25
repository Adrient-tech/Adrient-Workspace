// Slide master editing: layouts as editable pseudo-slides, placeholders,
// and the layout list operations (add, duplicate, delete, rename, move).
import { layoutsOf, ph } from "./layouts";
import { themeOf } from "./theme";
import { deckSize } from "./slideProps";
import { uid, type DeckDoc, type PlaceholderType, type Slide, type SlideElement, type SlideLayout } from "./model";

/** A layout shown on the editing canvas (same id, so edits map back). */
export function layoutToSlide(l: SlideLayout): Slide {
  return {
    id: l.id,
    background: l.background ?? "#ffffff",
    ...(l.bgRef ? { bgRef: l.bgRef } : {}),
    ...(l.bgFill ? { bgFill: l.bgFill } : {}),
    elements: l.elements,
  };
}

export function slideToLayout(l: SlideLayout, s: Slide): SlideLayout {
  const out: SlideLayout = { ...l, elements: s.elements, background: s.background };
  if (s.bgRef) out.bgRef = s.bgRef;
  else delete out.bgRef;
  if (s.bgFill) out.bgFill = s.bgFill;
  else delete out.bgFill;
  return out;
}

export const PLACEHOLDER_KINDS: { type: PlaceholderType; label: string }[] = [
  { type: "title", label: "Title" },
  { type: "subTitle", label: "Subtitle" },
  { type: "body", label: "Text / content" },
  { type: "pic", label: "Picture" },
  { type: "dt", label: "Date" },
  { type: "ftr", label: "Footer" },
  { type: "sldNum", label: "Slide number" },
];

/** Add a placeholder of `type`, below the existing ones; ids and idx stay unique. */
export function addPlaceholder(layout: SlideLayout, type: PlaceholderType, doc: DeckDoc): SlideLayout {
  const theme = themeOf(doc);
  const { h } = deckSize(doc);
  const sc = h / 540;
  const used = layout.elements.filter((e) => e.placeholder).map((e) => e.placeholder!.idx ?? 0);
  const idx = used.length ? Math.max(...used, 0) + 1 : 1;
  const sameType = layout.elements.filter((e) => e.placeholder?.type === type).length;
  const dy = Math.min(sameType * 24, 120);
  let el: SlideElement;
  if (type === "pic") {
    el = {
      id: `pic${idx}`,
      type: "image",
      x: 300,
      y: 140 + dy,
      w: 360,
      h: 300,
      src: "",
      placeholder: { type: "pic", idx },
    };
  } else if (type === "dt" || type === "ftr" || type === "sldNum") {
    const bx: [number, number] = type === "dt" ? [66, 216] : type === "ftr" ? [318, 324] : [678, 216];
    el = ph(type, idx, [bx[0], h / sc - 46, bx[1], 29], sc, theme, {
      fontSize: 12,
      valign: "middle",
      align: type === "dt" ? "left" : type === "ftr" ? "center" : "right",
      themeRefs: { color: "tx1/lumMod:65000/lumOff:35000" },
    });
  } else if (type === "title") {
    el = ph("title", undefined, [66, 29 + dy, 828, 90], sc, theme, { fontSize: 36, valign: "middle" });
  } else if (type === "subTitle") {
    el = ph("subTitle", idx, [120, 292 + dy, 720, 100], sc, theme, { fontSize: 22, align: "center" });
  } else {
    el = ph(type, idx, [66, 140 + dy, 828, 330], sc, theme, { fontSize: 22 });
  }
  el = { ...el, id: `${el.id}-${uid().slice(0, 4)}` };
  return { ...layout, elements: [...layout.elements, el] };
}

function uniqueId(layouts: SlideLayout[], base: string): string {
  let id = base;
  let i = 2;
  while (layouts.some((l) => l.id === id)) id = `${base}${i++}`;
  return id;
}

/** A new layout with a title and the date/footer/number boxes. */
export function newLayout(doc: DeckDoc, name = "Custom layout"): SlideLayout {
  const layouts = layoutsOf(doc);
  const base = layoutsOf({ ...doc, layouts: undefined }).find((l) => l.type === "titleOnly");
  const elements = (base?.elements ?? []).map((e) => ({ ...e, id: `${e.id}-${uid().slice(0, 4)}` }));
  return {
    id: uniqueId(layouts, "custom"),
    name: name.trim() || "Custom layout",
    elements,
    background: base?.background ?? "#ffffff",
    ...(base?.bgRef ? { bgRef: base.bgRef } : {}),
  };
}

export function duplicateLayout(doc: DeckDoc, id: string): DeckDoc {
  const layouts = layoutsOf(doc);
  const i = layouts.findIndex((l) => l.id === id);
  if (i < 0) return doc;
  const src = layouts[i];
  const copy: SlideLayout = {
    ...src,
    id: uniqueId(layouts, `${src.id}-copy`),
    name: `${src.name} copy`,
    type: undefined,
    elements: src.elements.map((e) => ({ ...e, id: `${e.id}-${uid().slice(0, 4)}` })),
  };
  if (!copy.type) delete copy.type;
  return { ...doc, layouts: [...layouts.slice(0, i + 1), copy, ...layouts.slice(i + 1)] };
}

/** Delete a layout; slides that used it keep their content and lose the link. Never the last one. */
export function deleteLayout(doc: DeckDoc, id: string): DeckDoc {
  const layouts = layoutsOf(doc);
  if (layouts.length <= 1 || !layouts.some((l) => l.id === id)) return doc;
  return {
    ...doc,
    layouts: layouts.filter((l) => l.id !== id),
    slides: doc.slides.map((s) => {
      if (s.layout !== id) return s;
      const { layout: _l, ...rest } = s;
      return rest;
    }),
  };
}

export function renameLayout(doc: DeckDoc, id: string, name: string): DeckDoc {
  const n = name.trim();
  if (!n) return doc;
  return { ...doc, layouts: layoutsOf(doc).map((l) => (l.id === id ? { ...l, name: n } : l)) };
}

export function moveLayout(doc: DeckDoc, id: string, dir: -1 | 1): DeckDoc {
  const layouts = layoutsOf(doc).slice();
  const i = layouts.findIndex((l) => l.id === id);
  const j = i + dir;
  if (i < 0 || j < 0 || j >= layouts.length) return doc;
  [layouts[i], layouts[j]] = [layouts[j], layouts[i]];
  return { ...doc, layouts };
}

/** Number of slides using each layout. */
export function layoutUsage(doc: DeckDoc): Record<string, number> {
  const out: Record<string, number> = {};
  for (const s of doc.slides) if (s.layout) out[s.layout] = (out[s.layout] ?? 0) + 1;
  return out;
}
