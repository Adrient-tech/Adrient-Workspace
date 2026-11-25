import { describe, it, expect } from "vitest";
import { apps } from "./apps";
import { GROUP_ORDER, groupApps, groupOf } from "./groups";
import { GLYPHS } from "../components/icons/glyphs";
import { glyphFor } from "../components/AppIcon";

describe("catalog/groups", () => {
  it("every catalog app has an Adrient glyph", () => {
    const missing = apps.filter((a) => !a.iconUrl && !GLYPHS[a.iconName]).map((a) => a.id);
    expect(missing).toEqual([]);
  });

  it("every catalog app belongs to a real group (not the catch-all by accident)", () => {
    // Apps that deliberately live in "more" are the media/learning set.
    const inMore = apps.filter((a) => groupOf(a.id) === "more").map((a) => a.id);
    for (const id of inMore) {
      expect(["photos", "music", "video", "books", "games", "podcasts", "translate", "3d", "archaeology", "maps", "learn"]).toContain(id);
    }
  });

  it("groups apps in launcher order and drops empty groups", () => {
    const out = groupApps([{ id: "mail" }, { id: "drive" }, { id: "crm" }, { id: "unknown-app" }]);
    expect(out.map((g) => g.group)).toEqual(["office", "communicate", "business", "more"]);
    expect(out[3].items[0].id).toBe("unknown-app");
    expect(GROUP_ORDER).toEqual(["office", "communicate", "business", "admin", "more"]);
  });

  it("keeps the curated order inside a group", () => {
    const out = groupApps([{ id: "tasks" }, { id: "docs" }, { id: "drive" }]);
    expect(out[0].items.map((a) => a.id)).toEqual(["drive", "docs", "tasks"]);
  });

  it("maps legacy Material names to glyphs", () => {
    expect(glyphFor("Folder")).toBe("drive");
    expect(glyphFor("GridView")).toBe("workspace");
    expect(glyphFor("sheets")).toBe("sheets");
    expect(glyphFor("SomethingUnknown")).toBeUndefined();
  });
});
