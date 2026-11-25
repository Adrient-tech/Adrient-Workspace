import { describe, expect, it } from "vitest";
import { finalSegments, recognitionCtor } from "./transcribe";

describe("transcribe", () => {
  it("keeps only final, non-empty results from resultIndex on", () => {
    const r = (isFinal: boolean, t: string) => ({ isFinal, 0: { transcript: t } });
    const segs = finalSegments({ resultIndex: 1, results: [r(true, "old"), r(true, "  hello   there "), r(false, "partial"), r(true, "  ")] });
    expect(segs).toEqual(["hello there"]);
  });
  it("finds the recogniser constructor, prefixed or not", () => {
    class A {}
    expect(recognitionCtor({ webkitSpeechRecognition: A })).toBe(A);
    expect(recognitionCtor({ SpeechRecognition: A })).toBe(A);
    expect(recognitionCtor({})).toBeNull();
  });
});
