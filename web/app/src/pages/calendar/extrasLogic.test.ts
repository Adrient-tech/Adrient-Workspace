import { describe, expect, it } from "vitest";
import { feedUrl, parseEmails, summarize, windowsError } from "./extrasLogic";

describe("calendar extras logic", () => {
  it("validates working-hour windows", () => {
    expect(windowsError([{ start: "09:00", end: "12:00" }, { start: "13:00", end: "17:00" }])).toBe("");
    expect(windowsError([{ start: "12:00", end: "09:00" }])).not.toBe("");
    expect(windowsError([{ start: "09:00", end: "11:00" }, { start: "10:00", end: "12:00" }])).toBe("Windows overlap");
    expect(windowsError([{ start: "nine", end: "10:00" }])).not.toBe("");
  });
  it("parses pasted email lists", () => {
    expect(parseEmails("A@x.com, b@y.org\nnot-an-email; a@x.com")).toEqual(["a@x.com", "b@y.org"]);
  });
  it("summarises days and builds feed URLs", () => {
    expect(summarize([])).toBe("Not working");
    expect(summarize([{ start: "09:00", end: "12:00" }])).toBe("09:00–12:00");
    expect(feedUrl("https://x.test/", "/api/v1/public/calendar/feed/t.ics")).toBe("https://x.test/api/v1/public/calendar/feed/t.ics");
  });
});
