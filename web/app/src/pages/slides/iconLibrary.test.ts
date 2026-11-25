import { describe, expect, it } from "vitest";
import { ICON_LIBRARY, iconSvgDocument, searchIcons } from "./iconLibrary";

describe("icon library", () => {
  it("has unique names and many icons", () => {
    expect(ICON_LIBRARY.length).toBeGreaterThan(80);
    expect(new Set(ICON_LIBRARY.map((i) => i.name)).size).toBe(ICON_LIBRARY.length);
  });
  it("searches by label words", () => {
    expect(searchIcons("arrow").map((i) => i.name)).toContain("ArrowForward");
    expect(searchIcons("pie chart").map((i) => i.name)).toEqual(["PieChart"]);
    expect(searchIcons("zzzz")).toEqual([]);
    expect(searchIcons("")).toBe(ICON_LIBRARY);
  });
  it("builds a coloured svg and rejects odd colours", () => {
    const svg = iconSvgDocument('<svg class="x" viewBox="0 0 24 24"><path d="M0 0h24v24H0z"/></svg>', "#126C5A");
    expect(svg).toContain('fill="#126C5A"');
    expect(svg).toContain('<path d="M0 0h24v24H0z"/>');
    expect(iconSvgDocument("<svg></svg>", 'red" onload="x')).toContain('fill="#000000"');
  });
});
