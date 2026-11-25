// @vitest-environment node
import { describe, expect, it } from "vitest";
import { addGroup, groupLabel, maxLevel, outlineLevel, setCollapsed, showLevel, ungroup, visibilityAfter, type OutlineGroup } from "./outlineOps";

const ok = (v: OutlineGroup[] | string): OutlineGroup[] => {
  if (typeof v === "string") throw new Error(v);
  return v;
};

describe("outline groups", () => {
  it("nest and refuse partial overlaps and repeats", () => {
    let l = ok(addGroup([], "row", 1, 10, "a"));
    l = ok(addGroup(l, "row", 2, 4, "b"));
    expect(outlineLevel(l, l.find((g) => g.id === "b")!)).toBe(2);
    expect(addGroup(l, "row", 8, 14)).toMatch(/partly overlap/);
    expect(addGroup(l, "row", 1, 10)).toMatch(/already grouped/);
    expect(ok(addGroup(l, "col", 8, 14, "c"))).toHaveLength(3);
    expect(maxLevel(l, "row")).toBe(2);
  });

  it("limits nesting to 8 levels", () => {
    let l: OutlineGroup[] = [];
    for (let i = 0; i < 8; i++) l = ok(addGroup(l, "row", i, 20 - i, `g${i}`));
    expect(addGroup(l, "row", 9, 10)).toMatch(/at most 8/);
  });

  it("keeps inner collapsed groups hidden when the outer one expands", () => {
    let l = ok(addGroup([], "row", 1, 6, "a"));
    l = ok(addGroup(l, "row", 2, 3, "b"));
    l = setCollapsed(l, "b", true).list;
    const c = setCollapsed(l, "a", true);
    expect(visibilityAfter(c.list, c.group!).hide).toEqual([1, 2, 3, 4, 5, 6]);
    const e = setCollapsed(c.list, "a", false);
    expect(visibilityAfter(e.list, e.group!).show).toEqual([1, 4, 5, 6]);
  });

  it("ungroups the innermost group around the selection", () => {
    let l = ok(addGroup([], "row", 1, 6, "a"));
    l = ok(addGroup(l, "row", 2, 3, "b"));
    expect(ungroup(l, "row", 2, 2).removed?.id).toBe("b");
    expect(ungroup(l, "row", 1, 6).removed?.id).toBe("a");
    expect(ungroup(l, "row", 9, 9).removed).toBeUndefined();
  });

  it("shows outline levels like Excel's level buttons", () => {
    let l = ok(addGroup([], "row", 1, 6, "a"));
    l = ok(addGroup(l, "row", 2, 3, "b"));
    expect(showLevel(l, "row", 1).map((g) => !!g.collapsed)).toEqual([true, true]);
    expect(showLevel(l, "row", 2).map((g) => !!g.collapsed)).toEqual([false, true]);
    expect(showLevel(l, "row", 3).map((g) => !!g.collapsed)).toEqual([false, false]);
  });

  it("labels groups", () => {
    expect(groupLabel({ id: "x", axis: "row", start: 1, end: 4 })).toBe("Rows 2–5");
    expect(groupLabel({ id: "y", axis: "col", start: 26, end: 27 })).toBe("Columns AA–AB");
  });
});
