import { describe, expect, it } from "vitest";
import { detectCountry, easterSunday, holidayEvents, holidaysInRange, isHoliday, resolveHolidayCountry } from "./holidays";

const names = (c: string, from: string, to: string) =>
  holidaysInRange(c, new Date(from + "T00:00:00"), new Date(to + "T00:00:00")).map((h) => `${h.date} ${h.name}`);

describe("holidays", () => {
  it("computes Easter", () => {
    expect(easterSunday(2026)).toEqual({ month: 4, day: 5 });
    expect(easterSunday(2024)).toEqual({ month: 3, day: 31 });
  });
  it("US rules and observed days", () => {
    expect(names("us", "2026-11-01", "2026-11-30")).toEqual(["2026-11-11 Veterans Day", "2026-11-26 Thanksgiving Day"]);
    // 4 July 2026 is a Saturday: observed Friday the 3rd
    expect(names("us", "2026-07-01", "2026-07-31")).toEqual(["2026-07-03 Independence Day (observed)", "2026-07-04 Independence Day"]);
  });
  it("UK Christmas on a weekend takes the next free weekdays", () => {
    expect(names("uk", "2027-12-20", "2027-12-31")).toEqual([
      "2027-12-25 Christmas Day",
      "2027-12-26 Boxing Day",
      "2027-12-27 Christmas Day (observed)",
      "2027-12-28 Boxing Day (observed)",
    ]);
  });
  it("India lists the three national days", () => {
    expect(names("in", "2026-01-01", "2026-12-31")).toEqual(["2026-01-26 Republic Day", "2026-08-15 Independence Day", "2026-10-02 Gandhi Jayanti"]);
  });
  it("Japan bridges a single working day between two holidays", () => {
    expect(names("japan", "2026-09-01", "2026-09-30")).toEqual([]);
    expect(names("jp", "2026-09-19", "2026-09-24")).toEqual(["2026-09-21 Respect for the Aged Day", "2026-09-22 Citizens' Holiday", "2026-09-23 Autumnal Equinox Day"]);
  });
  it("range edges include observed days from the neighbouring year", () => {
    // 1 Jan 2028 is a Saturday: the US observes it on Friday 31 Dec 2027.
    expect(names("us", "2027-12-30", "2028-01-02")).toContain("2027-12-31 New Year's Day (observed)");
  });
  it("detects country from zone then language", () => {
    expect(detectCountry("Asia/Kolkata", [])).toBe("in");
    expect(detectCountry("", ["de-DE"])).toBe("de");
    expect(detectCountry("America/Sao_Paulo", ["pt-BR"])).toBe("");
    expect(resolveHolidayCountry("")).toBe("");
    expect(resolveHolidayCountry("xx")).toBe("");
    expect(resolveHolidayCountry("jp")).toBe("jp");
  });
  it("builds read-only all-day events", () => {
    const ev = holidayEvents("in", new Date(2026, 0, 20), new Date(2026, 0, 31));
    expect(ev).toHaveLength(1);
    expect(isHoliday(ev[0])).toBe(true);
    expect(ev[0].all_day).toBe(true);
  });
});
