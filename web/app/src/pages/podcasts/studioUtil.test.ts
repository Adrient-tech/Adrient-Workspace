import { describe, expect, it } from "vitest";
import { charactersIn, htmlToPlain, readingMinutes, textToHtml, toLocalInput } from "./studioUtil";

describe("textToHtml", () => {
  it("makes paragraphs, headings and lists, escaping markup", () => {
    const h = textToHtml("# Title\n\nFirst <b>para</b>.\nSecond line.\n\n- one\n- two");
    expect(h).toContain("<h2>Title</h2>");
    expect(h).toContain("<p>First &lt;b&gt;para&lt;/b&gt;.<br>Second line.</p>");
    expect(h).toContain("<ul><li>one</li><li>two</li></ul>");
  });
  it("handles bold and italics", () => {
    expect(textToHtml("A **bold** and *soft* word")).toBe("<p>A <strong>bold</strong> and <em>soft</em> word</p>");
  });
});

describe("plain text and characters", () => {
  it("extracts paragraphs and character markers", () => {
    const plain = htmlToPlain("<p>[Meera] Hello.</p><p>[Ravi] Hi!</p><p>[Narrator] Later.</p><p>[Meera] Bye.</p>");
    expect(plain.split("\n")).toHaveLength(4);
    expect(charactersIn(plain)).toEqual(["Meera", "Ravi"]);
  });
  it("estimates reading time", () => {
    expect(readingMinutes(Array(320).fill("word").join(" "))).toBe(2);
  });
  it("formats datetime-local", () => {
    expect(toLocalInput("2026-11-21T10:05:00")).toMatch(/^2026-11-21T10:05$/);
  });
});
