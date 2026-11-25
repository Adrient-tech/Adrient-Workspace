import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { AppIcon } from "../AppIcon";
import { GLYPHS, GLYPH_NAMES } from "./glyphs";
import { ICON_FAMILIES } from "./palette";

const wrap = (ui: React.ReactElement) => render(ui);

describe("AppIcon", () => {
  it("draws an inline SVG glyph for a catalog app", () => {
    const { container } = wrap(<AppIcon iconName="drive" size={48} />);
    const svg = container.querySelector("svg.ad-glyph");
    expect(svg).toBeTruthy();
    expect(svg!.getAttribute("viewBox")).toBe("0 0 24 24");
    expect(container.querySelector("[data-glyph='drive']")).toBeTruthy();
  });

  it("falls back to a letter for an unknown icon and uses an image when iconUrl is set", () => {
    const a = wrap(<AppIcon iconName="NopeNotAnIcon" label="Zed" />);
    expect(a.container.textContent).toBe("Z");
    const b = wrap(<AppIcon iconName="drive" iconUrl="/x.png" />);
    expect(b.container.querySelector("img")).toBeTruthy();
    expect(b.container.querySelector("svg.ad-glyph")).toBeNull();
  });

  it("every glyph is original inline SVG content with at least one shape", () => {
    expect(GLYPH_NAMES.length).toBeGreaterThanOrEqual(50);
    for (const name of GLYPH_NAMES) {
      const { container } = render(<svg>{GLYPHS[name]}</svg>);
      expect(container.querySelectorAll("path,rect,circle,ellipse").length, name).toBeGreaterThan(0);
    }
  });

  it("has a light and a dark colour family per group", () => {
    for (const fam of Object.values(ICON_FAMILIES)) {
      for (const v of Object.values(fam)) expect(v).toMatch(/^#[0-9A-Fa-f]{6}$/);
    }
    // Business is the Adrient green family with an amber accent.
    expect(ICON_FAMILIES.business.fg).toBe("#126C5A");
    expect(ICON_FAMILIES.business.ac.toLowerCase()).toBe("#f2a21b");
  });
});
