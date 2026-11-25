import { describe, expect, it } from "vitest";
import { deckToPptx } from "./write";
import { readPptx } from "./read";
import { newSlide, type DeckDoc } from "../model";
import { addSection, sectionRanges } from "../sections";

describe("pptx sections round trip", () => {
  it("keeps section names and boundaries", async () => {
    let deck: DeckDoc = { slides: [newSlide(), newSlide(), newSlide(), newSlide(), newSlide()] };
    deck = addSection(deck, deck.slides[0].id, "Opening");
    deck = addSection(deck, deck.slides[2].id, "Details & more");
    const bytes = await deckToPptx(deck);
    const back = await readPptx(bytes);
    const r = sectionRanges(back.deck).map((x) => [x.section?.name, x.start, x.end]);
    expect(r).toEqual([["Opening", 0, 2], ["Details & more", 2, 5]]);
  });
  it("writes nothing for decks without sections", async () => {
    const back = await readPptx(await deckToPptx({ slides: [newSlide(), newSlide()] }));
    expect(back.deck.sections).toBeUndefined();
  });
});
