import { describe, expect, it } from "vitest";
import { findSettings, groupOf, NAV_GROUPS, SECTION_INFO } from "./AdminNav";

const items = [
  { id: "users", label: "Users" }, { id: "geo", label: "Region access" }, { id: "audit", label: "Audit log" },
  { id: "domains", label: "Domains" }, { id: "sessions", label: "Sessions & logins" },
];
describe("find a setting", () => {
  it("matches labels first, then keywords", () => {
    expect(findSettings(items, "users")[0].id).toBe("users");
    expect(findSettings(items, "country")[0].id).toBe("geo");
    expect(findSettings(items, "dns")[0].id).toBe("domains");
  });
  it("requires every word to match", () => {
    expect(findSettings(items, "audit zzzz")).toEqual([]);
    expect(findSettings(items, "")).toEqual([]);
  });
  it("groups sections and never lists one twice", () => {
    expect(groupOf("geo")).toBe("Security");
    expect(groupOf("unknown")).toBe("More");
    const all = NAV_GROUPS.flatMap((g) => g.ids);
    expect(new Set(all).size).toBe(all.length);
    for (const id of all) expect(SECTION_INFO[id]).toBeTruthy();
  });
});
