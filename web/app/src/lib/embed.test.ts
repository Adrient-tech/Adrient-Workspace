import { describe, expect, it } from "vitest";
import { isEmbedSearch, parseEmbedMessage, withEmbed, withoutEmbed } from "./embed";

describe("embed", () => {
  it("detects embed=1", () => {
    expect(isEmbedSearch("?embed=1")).toBe(true);
    expect(isEmbedSearch("?a=1&embed=1")).toBe(true);
    expect(isEmbedSearch("?embed=0")).toBe(false);
    expect(isEmbedSearch("")).toBe(false);
  });
  it("withEmbed / withoutEmbed", () => {
    expect(withEmbed("/sheets/1")).toBe("/sheets/1?embed=1");
    expect(withEmbed("/sheets/1?a=2#h")).toBe("/sheets/1?a=2&embed=1#h");
    expect(withEmbed("/sheets/1?embed=1")).toBe("/sheets/1?embed=1");
    expect(withEmbed("https://x.com")).toBe("https://x.com");
    expect(withoutEmbed("/sheets/1?a=2&embed=1#h")).toBe("/sheets/1?a=2#h");
    expect(withoutEmbed("/sheets/1?embed=1")).toBe("/sheets/1");
  });
  it("parses messages", () => {
    expect(parseEmbedMessage({ type: "adrient:navigate", href: "/x" })).toEqual({ type: "adrient:navigate", href: "/x" });
    expect(parseEmbedMessage({ type: "adrient:title", title: "T" })).toEqual({ type: "adrient:title", title: "T" });
    expect(parseEmbedMessage({ type: "other" })).toBeNull();
    expect(parseEmbedMessage(null)).toBeNull();
  });
});
