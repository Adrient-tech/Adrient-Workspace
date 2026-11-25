import { describe, expect, it } from "vitest";
import { extractCardLinks } from "./LinkCards";

const O = "http://workspace.localtest.me:8080";

describe("extractCardLinks", () => {
  it("classifies meetings, files and workspace objects; skips external links", () => {
    const body = `join abc-defg-hij or ${O}/sheets/42 and ${O}/invoicing/7 and https://example.com/x`;
    const l = extractCardLinks(body, O);
    expect(l.map((x) => x.target.app)).toEqual(["sheets", "invoicing", "meet"]);
  });
  it("dedupes and caps", () => {
    const body = [1, 2, 3, 4, 5].map((i) => `${O}/docs/${i}`).join(" ") + ` ${O}/docs/1`;
    expect(extractCardLinks(body, O)).toHaveLength(3);
  });
});
