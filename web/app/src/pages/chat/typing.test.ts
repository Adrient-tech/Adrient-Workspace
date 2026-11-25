import { describe, expect, it } from "vitest";
import { activeTypers, typingLabel, typingSignal } from "./typing";

describe("typing", () => {
  it("throttles to one signal per window and ignores empty text", () => {
    let t = 0, sent = 0;
    const s = typingSignal(() => sent++, 4000, () => t);
    expect(s.typed("  ")).toBe(false);
    expect(s.typed("h")).toBe(true);
    t = 3999;
    expect(s.typed("he")).toBe(false);
    t = 4000;
    expect(s.typed("hel")).toBe(true);
    expect(sent).toBe(2);
    s.reset();
    expect(s.typed("x")).toBe(true);
  });
  it("labels", () => {
    expect(typingLabel([])).toBe("");
    expect(typingLabel(["Ana"])).toBe("Ana is typing");
    expect(typingLabel(["Ana", "Ben"])).toBe("Ana and Ben are typing");
    expect(typingLabel(["a", "b", "c"])).toBe("Several people are typing");
  });
  it("expires old entries", () => {
    expect(activeTypers({ a: 1000, b: 9000 }, 10000)).toEqual(["b"]);
  });
});
