import { describe, expect, it } from "vitest";
import { checkImageFile, emptyIndex, notesInNotebook } from "./extrasApi";

describe("keep extras", () => {
  it("accepts only real image types up to 2 MB", () => {
    expect(checkImageFile({ type: "image/png", size: 1000 })).toBe("");
    expect(checkImageFile({ type: "image/webp", size: 2 * 1024 * 1024 })).toBe("");
    expect(checkImageFile({ type: "image/svg+xml", size: 100 })).not.toBe("");
    expect(checkImageFile({ type: "text/html", size: 100 })).not.toBe("");
    expect(checkImageFile({ type: "image/jpeg", size: 2 * 1024 * 1024 + 1 })).not.toBe("");
    expect(checkImageFile({ type: "image/png", size: 0 })).not.toBe("");
  });
  it("filters notes by notebook", () => {
    const idx = { ...emptyIndex(), note_notebook: { a: "nb1", b: "nb2", c: "nb1" } };
    const notes = [{ id: "a" }, { id: "b" }, { id: "c" }, { id: "d" }];
    expect(notesInNotebook(notes, idx, "nb1").map((n) => n.id)).toEqual(["a", "c"]);
    expect(notesInNotebook(notes, idx, "none")).toEqual([]);
  });
});
