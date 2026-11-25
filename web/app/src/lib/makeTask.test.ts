import { describe, expect, it } from "vitest";
import { titleFrom } from "./makeTask";

describe("makeTask titleFrom", () => {
  it("uses the first non-empty line", () => {
    expect(titleFrom("\n\n  Please send the report  \nthanks")).toBe("Please send the report");
  });
  it("strips markup and trims long text", () => {
    expect(titleFrom("<p>Hello <b>team</b></p>")).toBe("Hello team");
    const long = "x".repeat(300);
    expect(titleFrom(long)).toHaveLength(120);
    expect(titleFrom(long).endsWith("...")).toBe(true);
  });
  it("falls back when empty", () => {
    expect(titleFrom("   ")).toBe("Follow up");
    expect(titleFrom("", "Mail follow-up")).toBe("Mail follow-up");
  });
});
