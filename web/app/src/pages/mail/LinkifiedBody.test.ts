import { describe, expect, it } from "vitest";
import { splitLinks } from "./LinkifiedBody";
import { previewKind } from "./AttachmentCard";

describe("splitLinks", () => {
  it("finds urls and meeting codes, trimming trailing punctuation", () => {
    const parts = splitLinks("Join abc-defg-hij or see https://x.test/a/b. Thanks");
    expect(parts.filter((p) => p.link).map((p) => p.text)).toEqual(["abc-defg-hij", "https://x.test/a/b"]);
    expect(parts.map((p) => p.text).join("")).toBe("Join abc-defg-hij or see https://x.test/a/b. Thanks");
  });
});

describe("previewKind", () => {
  it("previews images and pdf only", () => {
    expect(previewKind({ filename: "a.PNG", content_type: "" })).toBe("image");
    expect(previewKind({ filename: "a", content_type: "application/pdf" })).toBe("pdf");
    expect(previewKind({ filename: "a.xlsx", content_type: "application/vnd.ms-excel" })).toBeNull();
  });
});
