import { describe, expect, it } from "vitest";
import type { Page } from "./api";
import { appendText, breadcrumb, buildTree, canMoveInto, docText, textToDoc, visibleRows, wordCount } from "./logic";

const p = (id: string, parent: string | undefined, position: number): Page => ({ id, space_id: "s", parent_id: parent, title: id, icon: "", kind: "page", position, version: 1, updated_at: "" });
const pages = [p("a", undefined, 2), p("b", undefined, 1), p("a1", "a", 2), p("a2", "a", 1), p("a2x", "a2", 1)];

describe("tree", () => {
  it("orders by position and nests", () => {
    const t = buildTree(pages);
    expect(t.map((n) => n.page.id)).toEqual(["b", "a"]);
    expect(t[1].children.map((n) => n.page.id)).toEqual(["a2", "a1"]);
    expect(t[1].children[0].children[0].depth).toBe(2);
  });
  it("shows only expanded children", () => {
    const t = buildTree(pages);
    expect(visibleRows(t, new Set()).map((n) => n.page.id)).toEqual(["b", "a"]);
    expect(visibleRows(t, new Set(["a", "a2"])).map((n) => n.page.id)).toEqual(["b", "a", "a2", "a2x", "a1"]);
  });
  it("finds ancestors and blocks cycles", () => {
    expect(breadcrumb(pages, "a2x").map((x) => x.id)).toEqual(["a", "a2"]);
    expect(canMoveInto(pages, "a", "a2x")).toBe(false);
    expect(canMoveInto(pages, "a", "a")).toBe(false);
    expect(canMoveInto(pages, "a2", "b")).toBe(true);
    expect(canMoveInto(pages, "a2", "")).toBe(true);
  });
});

describe("text helpers", () => {
  it("round-trips text", () => {
    const d = textToDoc("one two\n\nthree");
    expect(docText(d)).toBe("one two\nthree");
    expect(wordCount(d)).toBe(3);
  });
  it("appends after the last real paragraph", () => {
    const d = appendText({ type: "doc", content: [{ type: "paragraph", content: [{ type: "text", text: "hi" }] }, { type: "paragraph" }] }, "more");
    expect(docText(d)).toBe("hi\nmore");
  });
});
