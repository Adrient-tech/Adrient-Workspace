import { describe, expect, it } from "vitest";
import { darken, embedSnippet, isEmail, isHexColor, splitEmails, themeVars } from "./fxLogic";

describe("forms extras logic", () => {
  it("validates colours and builds theme variables only from valid ones", () => {
    expect(isHexColor("#126C5A")).toBe(true);
    expect(isHexColor("red")).toBe(false);
    expect(isHexColor("#12345")).toBe(false);
    const v = themeVars({ theme_color: "#8e24aa", theme_bg: "#f3e5f5" });
    expect(v["--joy-palette-primary-solidBg"]).toBe("#8e24aa");
    expect(v["--joy-palette-background-level1"]).toBe("#f3e5f5");
    expect(themeVars({ theme_color: "url(x)", theme_bg: "" })).toEqual({});
  });
  it("darkens a colour", () => {
    expect(darken("#ffffff", 0.5)).toBe("#808080");
    expect(darken("#000000")).toBe("#000000");
    expect(darken("nope")).toBe("nope");
  });
  it("builds a safe embed snippet", () => {
    const s = embedSnippet("https://x.test/", 'a"b', 120);
    expect(s).toContain('src="https://x.test/forms/d/a%22b/viewform"');
    expect(s).toContain('height="300"');
    expect(s.startsWith("<iframe ")).toBe(true);
  });
  it("splits and checks emails", () => {
    expect(splitEmails("A@x.com, b@y.org;\n a@x.com")).toEqual(["a@x.com", "b@y.org"]);
    expect(isEmail("a@b.co")).toBe(true);
    expect(isEmail("a b@c.d")).toBe(false);
    expect(isEmail("Name <a@b.co>")).toBe(false);
  });
});
