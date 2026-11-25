import { describe, it, expect } from "vitest";
import { DEFAULT_PINS, MAX_PINS, normalizePins, pinsFromExtra, togglePin, visiblePins, withPins } from "./pins";

describe("pins", () => {
  it("normalizes: unique strings only, capped", () => {
    expect(normalizePins(["a", "a", 3, "", "b"])).toEqual(["a", "b"]);
    expect(normalizePins("nope")).toEqual([]);
    expect(normalizePins(Array.from({ length: 30 }, (_, i) => `x${i}`))).toHaveLength(MAX_PINS);
  });

  it("reads and writes the preferences extra bag, keeping other keys", () => {
    expect(pinsFromExtra("{}")).toBeUndefined();
    expect(pinsFromExtra("not json")).toBeUndefined();
    const extra = withPins('{"docs_shortcut_scheme":"google"}', ["mail", "drive"]);
    expect(JSON.parse(extra)).toEqual({ docs_shortcut_scheme: "google", pinned_apps: ["mail", "drive"] });
    expect(pinsFromExtra(extra)).toEqual(["mail", "drive"]);
    // An explicit empty list is a real choice, not "unset".
    expect(pinsFromExtra(withPins("{}", []))).toEqual([]);
  });

  it("toggles and drops the oldest past the cap", () => {
    expect(togglePin(["a"], "b")).toEqual(["a", "b"]);
    expect(togglePin(["a", "b"], "a")).toEqual(["b"]);
    const full = Array.from({ length: MAX_PINS }, (_, i) => `p${i}`);
    const next = togglePin(full, "new");
    expect(next).toHaveLength(MAX_PINS);
    expect(next[next.length - 1]).toBe("new");
    expect(next).not.toContain("p0");
  });

  it("hides pins for apps the user cannot see, keeping pin order", () => {
    const vis = [{ id: "docs" }, { id: "mail" }];
    expect(visiblePins(["mail", "crm", "docs"], vis).map((a) => a.id)).toEqual(["mail", "docs"]);
    expect(DEFAULT_PINS.length).toBeGreaterThan(0);
  });
});
