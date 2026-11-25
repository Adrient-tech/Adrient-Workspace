// Slide sections: named groups of consecutive slides (PowerPoint "sections").
// A section starts at `slideId` and runs to the next section's first slide.
// Slides before the first section belong to an implicit "Untitled" group, so
// a deck without sections behaves exactly as before.
import { uid, type DeckDoc, type Slide } from "./model";

export interface DeckSection {
  id: string;
  name: string;
  /** First slide of the section. */
  slideId: string;
  collapsed?: boolean;
}

export interface SectionRange {
  section: DeckSection | null; // null = the implicit leading group
  start: number;
  end: number; // exclusive
}

/** Drop sections whose first slide is gone or duplicated; keep deck order. */
export function normalizeSections(doc: DeckDoc): DeckSection[] {
  const secs = doc.sections ?? [];
  const idx = new Map(doc.slides.map((s, i) => [s.id, i]));
  const seen = new Set<number>();
  return secs
    .filter((s) => {
      const i = idx.get(s.slideId);
      if (i === undefined || seen.has(i)) return false;
      seen.add(i);
      return true;
    })
    .sort((a, b) => idx.get(a.slideId)! - idx.get(b.slideId)!);
}

/** Contiguous ranges covering every slide. */
export function sectionRanges(doc: DeckDoc): SectionRange[] {
  const secs = normalizeSections(doc);
  const n = doc.slides.length;
  const out: SectionRange[] = [];
  const starts = secs.map((s) => doc.slides.findIndex((x) => x.id === s.slideId));
  if (!secs.length || starts[0] > 0) out.push({ section: null, start: 0, end: secs.length ? starts[0] : n });
  secs.forEach((s, i) => out.push({ section: s, start: starts[i], end: i + 1 < secs.length ? starts[i + 1] : n }));
  return out.filter((r) => r.end > r.start || r.section);
}

export function sectionOfSlide(doc: DeckDoc, slideId: string): DeckSection | null {
  const i = doc.slides.findIndex((s) => s.id === slideId);
  return sectionRanges(doc).find((r) => i >= r.start && i < r.end)?.section ?? null;
}

/** Start a new section at `slideId`. An existing section there is renamed. */
export function addSection(doc: DeckDoc, slideId: string, name = "Untitled section"): DeckDoc {
  if (!doc.slides.some((s) => s.id === slideId)) return doc;
  const secs = normalizeSections(doc);
  const at = secs.find((s) => s.slideId === slideId);
  if (at) return renameSection(doc, at.id, name);
  return { ...doc, sections: normalizeSections({ ...doc, sections: [...secs, { id: uid(), name: name.trim() || "Untitled section", slideId }] }) };
}

export function renameSection(doc: DeckDoc, id: string, name: string): DeckDoc {
  const n = name.trim();
  if (!n) return doc;
  return { ...doc, sections: normalizeSections(doc).map((s) => (s.id === id ? { ...s, name: n } : s)) };
}

/** Remove a section marker; its slides join the previous section. */
export function removeSection(doc: DeckDoc, id: string): DeckDoc {
  const secs = normalizeSections(doc).filter((s) => s.id !== id);
  const { sections: _drop, ...rest } = doc;
  return secs.length ? { ...rest, sections: secs } : rest;
}

export function toggleCollapsed(doc: DeckDoc, id: string): DeckDoc {
  return { ...doc, sections: normalizeSections(doc).map((s) => (s.id === id ? { ...s, collapsed: !s.collapsed } : s)) };
}

/** Move a whole section (with its slides) one step up or down among sections. */
export function moveSection(doc: DeckDoc, id: string, dir: -1 | 1): DeckDoc {
  const ranges = sectionRanges(doc);
  const i = ranges.findIndex((r) => r.section?.id === id);
  const j = i + dir;
  if (i < 0 || j < 0 || j >= ranges.length) return doc;
  const order = ranges.slice();
  [order[i], order[j]] = [order[j], order[i]];
  const slides: Slide[] = order.flatMap((r) => doc.slides.slice(r.start, r.end));
  return { ...doc, slides };
}

/** Delete a section together with its slides (always keeps at least one slide). */
export function deleteSectionWithSlides(doc: DeckDoc, id: string): DeckDoc {
  const r = sectionRanges(doc).find((x) => x.section?.id === id);
  if (!r) return doc;
  const slides = doc.slides.filter((_, i) => i < r.start || i >= r.end);
  if (!slides.length) return doc;
  return removeSection({ ...doc, slides }, id);
}

/** Section marker + first-slide lookups for the thumbnail rail. */
export function sectionStartingAt(doc: DeckDoc, slideId: string): DeckSection | undefined {
  return normalizeSections(doc).find((s) => s.slideId === slideId);
}

/** After slides were deleted or reordered: a section whose first slide was
 *  deleted moves to the next surviving slide that followed it. */
export function reanchorSections(prev: DeckDoc, next: Slide[]): DeckDoc["sections"] {
  const secs = prev.sections;
  if (!secs?.length) return secs;
  const alive = new Set(next.map((s) => s.id));
  const out: DeckSection[] = [];
  for (const sec of secs) {
    if (alive.has(sec.slideId)) {
      out.push(sec);
      continue;
    }
    const at = prev.slides.findIndex((s) => s.id === sec.slideId);
    const repl = at < 0 ? undefined : prev.slides.slice(at + 1).find((s) => alive.has(s.id));
    if (repl && !secs.some((o) => o.slideId === repl.id)) out.push({ ...sec, slideId: repl.id });
  }
  const res = normalizeSections({ slides: next, sections: out });
  return res.length ? res : undefined;
}
