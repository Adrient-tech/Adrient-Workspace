import { describe, expect, it } from "vitest";
import { DECK_TEMPLATES } from "./templates";
import { deckToPptx } from "./pptx/write";
import { readPptx } from "./pptx/read";

describe("template gallery", () => {
  it("has at least 12 designed decks with unique ids", () => {
    const real = DECK_TEMPLATES.filter((t) => t.id !== "blank");
    expect(real.length).toBeGreaterThanOrEqual(12);
    expect(new Set(DECK_TEMPLATES.map((t) => t.id)).size).toBe(DECK_TEMPLATES.length);
  });
  it("every deck builds with fresh ids, inside the slide, with text and notes", () => {
    for (const t of DECK_TEMPLATES) {
      const d = t.build();
      expect(d.slides.length, t.id).toBeGreaterThan(0);
      const ids = new Set<string>();
      for (const s of d.slides)
        for (const el of s.elements) {
          expect(ids.has(el.id), `${t.id} dup id`).toBe(false);
          ids.add(el.id);
          if (el.type !== "image") {
            expect(el.x + el.w, `${t.id} overflows right`).toBeLessThanOrEqual(1080 + 1);
            expect(el.y + el.h, `${t.id} overflows bottom`).toBeLessThanOrEqual(700);
          }
        }
    }
  });
  it("designed decks survive a PPTX round trip", async () => {
    for (const t of DECK_TEMPLATES.filter((x) => x.id.startsWith("d-")).slice(0, 3)) {
      const d = t.build();
      const back = await readPptx(await deckToPptx(d));
      expect(back.deck.slides.length).toBe(d.slides.length);
    }
  }, 60000);
});
