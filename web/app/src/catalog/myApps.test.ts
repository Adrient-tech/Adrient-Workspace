import { describe, expect, it } from "vitest";
import { applyMyApps } from "./myApps";
import { applyServiceSettings } from "./serviceSettings";
import type { AppTile } from "./apps";

const tile = (id: string) => ({ id, name: id, blurb: "" }) as unknown as AppTile;
const catalog = [tile("drive"), tile("accounting"), tile("inventory"), tile("admin")];

describe("applyMyApps", () => {
  it("fails open when the list is unavailable", () => {
    expect(applyMyApps(catalog, null).map((a) => a.id)).toHaveLength(4);
    expect(applyMyApps(catalog, undefined)).toHaveLength(4);
  });
  it("drops denied apps", () => {
    const out = applyMyApps(catalog, { allowed: ["drive"], denied: ["accounting"] });
    expect(out.map((a) => a.id)).toEqual(["drive", "inventory", "admin"]);
  });
  it("runs after the org-wide switches, which stay the first filter", () => {
    const orgFiltered = applyServiceSettings(catalog, [
      { service_id: "inventory", enabled: false },
    ]);
    const out = applyMyApps(orgFiltered, { allowed: [], denied: ["accounting"] });
    expect(out.map((a) => a.id)).toEqual(["drive", "admin"]);
  });
});
