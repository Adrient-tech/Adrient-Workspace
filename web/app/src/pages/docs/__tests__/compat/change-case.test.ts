// Change case (fixtures are Adrient's own sentences). Adrient exposes change
// case as Format > Text > Change case (Sentence case / lowercase / UPPERCASE /
// Capitalize Each Word / tOGGLE cASE), implemented by changeCase() in
// textCase.ts. Equations (M11) are left unchanged.
import { describe, expect, it } from "vitest";
import type { Editor } from "@tiptap/core";
import { changeCase, changeCaseText, type CaseMode } from "../../textCase";
import { MathInput } from "../../math/autocorrect";
import { toLinear } from "../../math/linear";
import { contentOf } from "../../math/MathNode";
import {
  htmlSnapshot,
  makeEditor,
  paragraphPos,
  paragraphText,
  paragraphTexts,
  pressKey,
  selectedText,
  selectInParagraph,
  selectRange,
} from "../harness";

const apply = (e: Editor, mode: CaseMode) => changeCase(e, mode);

// One paragraph of several sentences in each casing.
const SENTENCE =
  "The garden was quiet at dawn, and the bees were not awake. Small birds sang in the hedge. A gardener opened the gate, then closed it again.";
const UPPER = SENTENCE.toUpperCase();
const LOWER = SENTENCE.toLowerCase();
// Every word capitalised, rest lower-case.
const WORDS =
  "The Garden Was Quiet At Dawn, And The Bees Were Not Awake. Small Birds Sang In The Hedge. A Gardener Opened The Gate, Then Closed It Again.";
// Toggle of SENTENCE: each letter's case inverted.
const TOGGLED =
  "tHE GARDEN WAS QUIET AT DAWN, AND THE BEES WERE NOT AWAKE. sMALL BIRDS SANG IN THE HEDGE. a GARDENER OPENED THE GATE, THEN CLOSED IT AGAIN.";

/** Load one paragraph and select all of its text. */
function paragraphSelected(text: string): Editor {
  const e = makeEditor(`<p>${text}</p>`);
  selectInParagraph(e, 0, 0, text.length);
  return e;
}

describe("change-case: whole paragraph", () => {
  it("Sentence case paragraph", () => {
    const e = paragraphSelected(LOWER);
    apply(e, "sentence");
    expect(paragraphText(e, 0)).toBe(SENTENCE);
  });

  it("Upper case paragraph", () => {
    const e = paragraphSelected(SENTENCE);
    apply(e, "upper");
    expect(paragraphText(e, 0)).toBe(UPPER);
  });

  it("Lower case paragraph", () => {
    const e = paragraphSelected(TOGGLED);
    apply(e, "lower");
    expect(paragraphText(e, 0)).toBe(LOWER);
  });

  it("Toggle case paragraph", () => {
    const e = paragraphSelected(SENTENCE);
    apply(e, "toggle");
    expect(paragraphText(e, 0)).toBe(TOGGLED);
  });

  it("CapitalizeWords case paragraph", () => {
    const e = paragraphSelected(SENTENCE);
    apply(e, "capitalize");
    expect(paragraphText(e, 0)).toBe(WORDS);
  });
});

describe("change-case: partial selection", () => {
  it("Sentence case", () => {
    // Selecting "red. apple" in "red. apples" raises the first letter of
    // each sentence in the selection; the unselected "s" is untouched.
    const e = makeEditor("<p>red. apples</p>");
    selectInParagraph(e, 0, 0, 10);
    apply(e, "sentence");
    expect(paragraphText(e, 0)).toBe("Red. Apples");
  });

  it("Upper case", () => {
    const e = makeEditor("<p>Big blue. ocean</p>");
    selectInParagraph(e, 0, 4, 8); // "blue"
    apply(e, "upper");
    expect(paragraphText(e, 0)).toBe("Big BLUE. ocean");
  });

  it("Lower case", () => {
    const e = makeEditor("<p>Big Blue. ocean</p>");
    selectInParagraph(e, 0, 4, 8); // "Blue"
    apply(e, "lower");
    expect(paragraphText(e, 0)).toBe("Big blue. ocean");
  });

  it("Toggle case", () => {
    const e = makeEditor("<p>Big Blue. ocean</p>");
    selectInParagraph(e, 0, 4, 8);
    apply(e, "toggle");
    expect(paragraphText(e, 0)).toBe("Big bLUE. ocean");
  });

  it("CapitalizeWords case", () => {
    const e = makeEditor("<p>big blue. ocean</p>");
    selectInParagraph(e, 0, 0, 8); // "big blue"
    apply(e, "capitalize");
    expect(paragraphText(e, 0)).toBe("Big Blue. ocean");
  });
});

describe("change-case: equations", () => {
  // Change case leaves equation text untouched (letters in math are variables and "." may be a
  // decimal point).
  function mathCase(typed: string, mode: CaseMode): string {
    const m = new MathInput();
    m.type(typed);
    const e = makeEditor("<p></p>");
    e.commands.insertEquation({ content: m.root, edit: false });
    selectRange(e, 1, e.state.doc.content.size - 1);
    apply(e, mode);
    let out = "";
    e.state.doc.descendants((n) => {
      if (n.type.name === "math") out = toLinear(contentOf(n));
    });
    return out;
  }
  it("Sentence case math", () => {
    expect(mathCase("(abc. aaaaa)/2 ", "sentence")).toBe("(abc. aaaaa)/2");
  });
  it("Upper case math", () => {
    expect(mathCase("abc/def+2_(xyz.rt\\delta aaa+2) ", "upper")).toBe("abc/def+2_(xyz.rtδaaa+2)");
  });
  it("Lower case math", () => {
    expect(mathCase("ABC/DEF+2_(XYZ.RTΔAAA+2) ", "lower")).toBe("ABC/DEF+2_(XYZ.RTΔAAA+2)");
  });
  it("ToggleCase case math", () => {
    expect(mathCase("aBC/Def+2_(XyZ.RtΔAaA+2) ", "toggle")).toBe("aBC/Def+2_(XyZ.RtΔAaA+2)");
  });
  it("CapitalizeWords case math", () => {
    expect(mathCase("aBC/Def+2_(XyZ.RtΔAaA+2) ", "capitalize")).toBe("aBC/Def+2_(XyZ.RtΔAaA+2)");
  });
});

// Before M1 the transform re-inserted the selection as one plain string, losing per-run formatting
// and merging the selected paragraphs.
describe("change case keeps document structure (Adrient)", () => {
  it("keeps each run's formatting", () => {
    const e = makeEditor("<p><strong>big</strong> blue <em>sea</em></p>");
    selectInParagraph(e, 0, 0, 12);
    apply(e, "upper");
    expect(htmlSnapshot(e)).toBe("<p><strong>BIG</strong> BLUE <em>SEA</em></p>");
  });

  it("keeps paragraphs separate", () => {
    const e = makeEditor("<p>one</p><p>two</p>");
    selectRange(e, paragraphPos(e, 0, 0), paragraphPos(e, 1, 3));
    apply(e, "upper");
    expect(paragraphTexts(e)).toEqual(["ONE", "TWO"]);
  });

  it("judges words across run boundaries", () => {
    // "wor" bold + "ld" plain is one word: only its first letter is raised.
    const e = makeEditor("<p>hello <strong>wor</strong>ld</p>");
    selectInParagraph(e, 0, 0, 11);
    apply(e, "capitalize");
    expect(htmlSnapshot(e)).toBe("<p>Hello <strong>Wor</strong>ld</p>");
  });

  it("uses the text before a partial selection as context", () => {
    // Selection starts mid-sentence: nothing in it starts a sentence.
    const e = makeEditor("<p>One two three</p>");
    selectInParagraph(e, 0, 4, 13);
    apply(e, "sentence");
    expect(paragraphText(e, 0)).toBe("One two three");
    // ... and mid-word for Capitalize Each Word.
    const f = makeEditor("<p>hello world</p>");
    selectInParagraph(f, 0, 2, 11);
    apply(f, "capitalize");
    expect(paragraphText(f, 0)).toBe("hello World");
  });

  it("keeps the selection on the changed text and undoes in one step", () => {
    const e = makeEditor("<p>alpha beta</p>");
    selectInParagraph(e, 0, 0, 5);
    apply(e, "upper");
    expect(selectedText(e)).toBe("ALPHA");
    pressKey(e, "Mod-z");
    expect(paragraphText(e, 0)).toBe("alpha beta");
  });

  it("changes the word at the caret when nothing is selected", () => {
    const e = makeEditor("<p>alpha beta</p>");
    selectRange(e, paragraphPos(e, 0, 7), paragraphPos(e, 0, 7));
    apply(e, "upper");
    expect(paragraphText(e, 0)).toBe("alpha BETA");
  });

  it("handles characters whose case changes length", () => {
    expect(changeCaseText("straße", "upper")).toBe("STRASSE");
    const e = makeEditor("<p>a <em>straße</em> b</p>");
    selectInParagraph(e, 0, 0, 10);
    apply(e, "upper");
    expect(htmlSnapshot(e)).toBe("<p>A <em>STRASSE</em> B</p>");
  });
});
