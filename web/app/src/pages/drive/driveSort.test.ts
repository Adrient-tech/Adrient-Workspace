import { describe, expect, it } from "vitest";
import { groupLabel, sortFiles, typeCategory } from "./driveSort";
import type { DriveFile } from "./types";

const f = (name: string, mime: string, size = 0, updated = "1767225600"): DriveFile =>
  ({ id: name, name, mime_type: mime, size_bytes: String(size), updated_at: updated, created_at: updated }) as unknown as DriveFile;

describe("driveSort", () => {
  const files = [f("b.txt", "text/plain", 5), f("a.png", "image/png", 50), f("Docs", "application/vnd.adrient.folder"), f("c.pdf", "application/pdf", 20)];
  it("puts folders first and sorts by name numerically", () => {
    expect(sortFiles(files, "name", false).map((x) => x.name)).toEqual(["Docs", "a.png", "b.txt", "c.pdf"]);
    expect(sortFiles([f("f10", "text/plain"), f("f2", "text/plain")], "name", false).map((x) => x.name)).toEqual(["f2", "f10"]);
  });
  it("sorts by size descending", () => {
    expect(sortFiles(files, "size", true).map((x) => x.name)).toEqual(["Docs", "a.png", "c.pdf", "b.txt"]);
  });
  it("categorises and buckets", () => {
    expect(typeCategory(f("x", "image/jpeg"))).toBe("Images");
    expect(typeCategory(f("x", "application/vnd.adrient.folder"))).toBe("Folders");
    const now = new Date("2026-06-15T12:00:00");
    expect(groupLabel(f("x", "text/plain", 0, String(Math.floor(new Date("2026-06-15T08:00:00").getTime() / 1000))), "modified", now)).toBe("Today");
    expect(groupLabel(f("x", "text/plain", 0, "1767225600"), "modified", now)).toBe("Older");
  });
});
