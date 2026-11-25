import { describe, expect, it } from "vitest";
import { activeClock, barHeights, fmtMinutes, parseArticle, slaText, splitLinks, splitMinutes, toMinutes, worstState } from "./helpLogic";

const now = new Date("2030-01-07T10:00:00Z");

describe("help desk logic", () => {
  it("shows time left and overdue", () => {
    expect(slaText({ state: "ok", due: "2030-01-07T12:05:00Z" }, now)).toBe("2 h 5 min left");
    expect(slaText({ state: "breached", due: "2030-01-07T09:30:00Z" }, now)).toBe("30 min overdue");
    expect(slaText({ state: "ok", due: "2030-01-10T10:00:00Z" }, now)).toBe("3 d 0 h left");
    expect(slaText({ state: "met", due: "2030-01-07T08:00:00Z" }, now)).toBe("Met");
    expect(slaText({ state: "" }, now)).toBe("");
  });

  it("picks the worst state and the active clock", () => {
    const i = { first_response: { state: "met" as const }, resolution: { state: "at_risk" as const, due: "x" } };
    expect(worstState(i)).toBe("at_risk");
    expect(activeClock(i)?.name).toBe("Resolution");
    const j = { first_response: { state: "ok" as const, due: "x" }, resolution: { state: "ok" as const, due: "y" } };
    expect(activeClock(j)?.name).toBe("First response");
    expect(worstState(undefined)).toBe("");
    expect(activeClock({ first_response: { state: "" as const }, resolution: { state: "" as const } })).toBeNull();
  });

  it("converts minutes to the unit people type", () => {
    expect(splitMinutes(2880)).toEqual({ value: 2, unit: "d" });
    expect(splitMinutes(120)).toEqual({ value: 2, unit: "h" });
    expect(splitMinutes(90)).toEqual({ value: 90, unit: "min" });
    expect(toMinutes(2, "d")).toBe(2880);
    expect(toMinutes(-5, "h")).toBe(0);
  });

  it("parses article text into safe blocks", () => {
    const b = parseArticle("# Reset\nGo to Settings\nthen Security.\n\n- one\n- two\n\nSee https://x.test/a?b=1 now <script>alert(1)</script>");
    expect(b[0]).toEqual({ kind: "h", text: "Reset" });
    expect(b[1]).toEqual({ kind: "p", text: "Go to Settings then Security." });
    expect(b[2]).toEqual({ kind: "ul", items: ["one", "two"] });
    expect(b[3].kind).toBe("p");
    const links = splitLinks((b[3] as { text: string }).text);
    expect(links.find((l) => l.href)?.href).toBe("https://x.test/a?b=1");
    expect(splitLinks("javascript:alert(1) and ftp://x").some((l) => l.href)).toBe(false);
  });

  it("scales report bars", () => {
    expect(barHeights([0, 5, 10])).toEqual([0, 50, 100]);
    expect(barHeights([0, 0])).toEqual([0, 0]);
    expect(fmtMinutes(0)).toBe("-");
    expect(fmtMinutes(90)).toBe("1.5 h");
    expect(fmtMinutes(2880)).toBe("2.0 d");
  });
});
