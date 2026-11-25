import { describe, it, expect } from "vitest";
import { railVisible } from "./Shell";
import { breadcrumbsFor } from "./breadcrumbs";
import { adrientTheme, tokens } from "../theme";

describe("shell", () => {
  it("shows the dock rail on app pages but not in focus or public routes", () => {
    for (const p of ["/", "/apps", "/drive", "/mail", "/docs", "/sheets", "/crm/deals", "/admin/users"]) expect(railVisible(p), p).toBe(true);
    for (const p of ["/docs/d/abc", "/sheets/d/1", "/slides/d/1", "/pos/display", "/docs/share/tok", "/games/chess"]) expect(railVisible(p), p).toBe(false);
  });

  it("builds breadcrumbs without ids", () => {
    expect(breadcrumbsFor("/")).toEqual([]);
    expect(breadcrumbsFor("/docs")).toEqual([]);
    expect(breadcrumbsFor("/settings")).toEqual([{ label: "Settings", to: "/settings" }]);
    expect(breadcrumbsFor("/drive/trash").map((c) => c.label)).toEqual(["Trash"]);
    expect(breadcrumbsFor("/docs/d/3f2b8f10-1111-2222-3333-444455556666")).toEqual([]);
    expect(breadcrumbsFor("/admin/users-and-groups").map((c) => c.label)).toEqual(["Users and groups"]);
  });
});

describe("design tokens", () => {
  it("anchors on Adrient green with a saffron secondary and bundled fonts", () => {
    expect(tokens.color.green).toBe("#126C5A");
    expect(tokens.color.saffron.toLowerCase()).toBe("#f2a21b");
    expect(tokens.font.display).toContain("Plus Jakarta Sans");
    expect(tokens.font.body).toContain("Inter");
    expect(adrientTheme.colorSchemes.light.palette.primary[500]).toBe("#126C5A");
    expect(adrientTheme.colorSchemes.dark.palette.background.body).not.toBe(adrientTheme.colorSchemes.light.palette.background.body);
  });
});
