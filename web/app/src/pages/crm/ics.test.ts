import { describe, expect, it } from "vitest";
import { buildCalendar, buildEvent, crmEvents, escapeText, foldLine } from "./ics";
import type { Deal, Followup } from "./types";

const now = new Date("2030-01-02T03:04:05Z");
const fu = (o: Partial<Followup>): Followup => ({ id: "f1", title: "Call Asha", due_date: "2030-02-01", due_time: "", deal_id: null, contact_id: null, done: false, calendar_event_id: "", created_at: "", ...o });

describe("crm ics", () => {
  it("escapes text and strips injected lines", () => {
    expect(escapeText("a,b;c\\d\nBEGIN:VEVENT\r\nx\u0007")).toBe("a\\,b\\;c\\\\d\\nBEGIN:VEVENT\\nx");
  });
  it("folds at 75 octets on code points", () => {
    const f = foldLine("X:" + "é".repeat(60) + "😀".repeat(10));
    for (const l of f.split("\r\n")) expect(new TextEncoder().encode(l).length).toBeLessThanOrEqual(75);
    expect(f.replace(/\r\n /g, "")).toBe("X:" + "é".repeat(60) + "😀".repeat(10));
  });
  it("builds all-day and timed events, rolling past midnight", () => {
    expect(buildEvent({ uid: "u", summary: "S", date: "2030-02-28" }, now).join("|")).toContain("DTSTART;VALUE=DATE:20300228|DTEND;VALUE=DATE:20300301");
    const t = buildEvent({ uid: "u", summary: "S", date: "2030-12-31", time: "23:45", minutes: 30, alarm: true }, now).join("|");
    expect(t).toContain("DTSTART:20301231T234500|DTEND:20310101T001500");
    expect(t).toContain("TRIGGER:-PT30M");
  });
  it("exports only open follow-ups and open deals with valid dates", () => {
    const deals = [
      { id: "d1", title: "Big", stage: "neg", expected_close: "2030-03-01", company_id: "c1" },
      { id: "d2", title: "Won", stage: "won", expected_close: "2030-03-01" },
      { id: "d3", title: "None", stage: "new", expected_close: "" },
    ] as unknown as Deal[];
    const ev = crmEvents([fu({}), fu({ id: "f2", done: true }), fu({ id: "f3", due_date: "bad" }), fu({ id: "f4", due_time: "09:30" })], deals, { company: () => "Acme" });
    expect(ev.map((e) => e.uid)).toEqual(["crm-followup-f1@adrient", "crm-followup-f4@adrient", "crm-deal-d1@adrient"]);
    const cal = buildCalendar(ev, now);
    expect(cal.startsWith("BEGIN:VCALENDAR\r\n")).toBe(true);
    expect(cal.endsWith("END:VCALENDAR\r\n")).toBe(true);
    expect(cal).toContain("DTSTART:20300201T093000");
    expect(cal).toContain("Acme");
  });
});
