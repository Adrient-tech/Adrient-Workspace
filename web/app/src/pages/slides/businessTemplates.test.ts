import { describe, expect, it } from "vitest";
import { BUSINESS_TEMPLATES } from "./businessTemplates";
import { DECK_TEMPLATES } from "./templates";

describe("business templates", () => {
  it("are on the start screen after Blank", () => {
    expect(DECK_TEMPLATES[0].id).toBe("blank");
    for (const t of BUSINESS_TEMPLATES) expect(DECK_TEMPLATES.map((x) => x.id)).toContain(t.id);
  });
  it("build decks with unique ids and in-bounds elements", () => {
    for (const t of BUSINESS_TEMPLATES) {
      const d = t.build();
      expect(d.slides.length).toBeGreaterThanOrEqual(6);
      const ids = new Set<string>();
      for (const s of d.slides)
        for (const e of s.elements) {
          expect(ids.has(e.id)).toBe(false);
          ids.add(e.id);
          expect(e.x).toBeGreaterThanOrEqual(0);
          expect(e.x + e.w).toBeLessThanOrEqual(960);
          expect(e.y + e.h).toBeLessThanOrEqual(540);
        }
    }
  });
});
