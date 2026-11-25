import { describe, it, expect } from "vitest";
import { existsSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { readOdp } from "./read";
import { readPptx } from "../pptx/read";
import { deckToPptx } from "../pptx/write";

// Point ADRIENT_CONVERSION_CORPUS at research/corpus/core (the default probe); the tests skip when
// the files aren't there (CI).
//
//   ADRIENT_CONVERSION_CORPUS=$PWD/../../research/corpus/core npx vitest run src/pages/slides/odp/corpus.test.ts

const ROOT = resolve(
  process.env.ADRIENT_CONVERSION_CORPUS ||
    join(__dirname, "../../../../../../research/corpus/core"),
);
const DIR = "OdfFile/Test/test_odf/ExampleFiles";

const CASES: [string, string, string][] = [
  ["audio.odp", "audio.odp", "audio.pptx"],
  ["entrance.odp", "entrance.odp", "entrance.pptx"],
  ["motion.odp", "motion.odp", "motion.odp-my.pptx"],
  ["openDocument.odp", "openDocument.odp", "openDocument.pptx"],
  ["playAudio.odp", "playAudio.odp", "playAudio.pptx"],
  ["runProgram.odp", "runProgram.odp", "runProgram.pptx"],
];

const words = (s: string | undefined) => (s ?? "").replace(/\s+/g, " ").trim();

describe("odp corpus (local only)", () => {
  for (const [tag, odp, pptx] of CASES) {
    const file = join(ROOT, DIR, odp);
    it.skipIf(!existsSync(file))(`${tag}`, async () => {
      const src = await readOdp(new Uint8Array(readFileSync(file)));
      expect(src.deck.slides.length).toBeGreaterThan(0);
      const els = src.deck.slides.flatMap((s) => s.elements);
      expect(els.length).toBeGreaterThan(0);
      for (const e of els) for (const v of [e.x, e.y, e.w, e.h]) expect(Number.isFinite(v)).toBe(true);
      // Every element lies (at least partly) on its slide.
      for (const e of els) {
        expect(e.x + e.w).toBeGreaterThan(0);
        expect(e.x).toBeLessThan(960);
      }

      const ref = join(ROOT, DIR, pptx);
      if (existsSync(ref)) {
        const other = await readPptx(new Uint8Array(readFileSync(ref)));
        expect(src.deck.slides.length).toBe(other.deck.slides.length);
      }

      // ODP → Adrient → pptx → Adrient keeps the slides, something drawn on
      // each, and all their text (pptx may merge a shape and its text).
      const again = await readPptx(await deckToPptx(src.deck));
      expect(again.deck.slides.length).toBe(src.deck.slides.length);
      const texts = (d: typeof src.deck) =>
        d.slides.map((s) => s.elements.map((e) => words(e.text)).filter(Boolean).sort());
      expect(texts(again.deck)).toEqual(texts(src.deck));
      for (const [i, s] of again.deck.slides.entries())
        expect(s.elements.length > 0).toBe(src.deck.slides[i].elements.length > 0);
    });
  }
});
