import { describe, expect, it } from "vitest";
import { describeEvent, parseTags, selectionState, toggleAll, withFilter } from "./deskLogic";

describe("desk logic", () => {
  it("adds filters and replaces the same key", () => {
    expect(withFilter("refund", "status:open")).toBe("refund status:open");
    expect(withFilter("status:open refund", "status:solved")).toBe("refund status:solved");
    expect(withFilter("#4 x", "#9")).toBe("x #9");
  });
  it("parses tags like the server", () => {
    expect(parseTags("Billing, refund request,#VIP,, billing")).toEqual(["billing", "refund-request", "vip"]);
    expect(parseTags("")).toEqual([]);
  });
  it("describes events", () => {
    expect(describeEvent({ kind: "status", detail: { from: "open", to: "resolved" } })).toBe("changed status from open to resolved");
    expect(describeEvent({ kind: "tags", detail: { to: ["a", "b"] } })).toBe("set tags to a, b");
    expect(describeEvent({ kind: "assignee", detail: { to: "" } })).toBe("removed the assignee");
    expect(describeEvent({ kind: "macro", detail: { name: "Close" } })).toBe('applied the macro "Close"');
  });
  it("tracks selection", () => {
    const ids = ["a", "b", "c"];
    expect(selectionState(ids, new Set())).toBe("none");
    expect(selectionState(ids, new Set(["a"]))).toBe("some");
    expect(selectionState(ids, new Set(ids))).toBe("all");
    expect([...toggleAll(ids, new Set(["a"]))].sort()).toEqual(ids);
    expect(toggleAll(ids, new Set(ids)).size).toBe(0);
  });
});
