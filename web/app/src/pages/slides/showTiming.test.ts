import { describe, expect, it } from "vitest";
import { newSlide, type DeckDoc } from "./model";
import { addDwell, applyRehearsal, clampTiming, clearTimings, formatDuration, kioskDelay, totalTime } from "./showTiming";

const deck = (): DeckDoc => ({ slides: [newSlide(), newSlide(), newSlide()] });

describe("show timing", () => {
  it("accumulates dwell per slide and ignores non-positive", () => {
    let d = addDwell({}, 1, 2000);
    d = addDwell(d, 1, 3500);
    d = addDwell(d, 0, 0);
    expect(d).toEqual({ 1: 5500 });
  });
  it("rounds to seconds within bounds", () => {
    expect(clampTiming(100)).toBe(1000);
    expect(clampTiming(4400)).toBe(4000);
    expect(clampTiming(9e9)).toBe(3_600_000);
  });
  it("applies rehearsal by slide id and keeps unvisited timings", () => {
    const d = deck();
    d.slides[2].advanceAfter = 7000;
    const out = applyRehearsal(d, d.slides.map((s) => s.id), { 0: 3200, 1: 12600 });
    expect(out.slides.map((s) => s.advanceAfter)).toEqual([3000, 13000, 7000]);
    expect(applyRehearsal(d, [], {})).toBe(d);
  });
  it("clears timings and totals only shown slides", () => {
    const d = deck();
    d.slides[0].advanceAfter = 4000;
    d.slides[1].advanceAfter = 6000;
    d.slides[1].hidden = true;
    expect(totalTime(d)).toBe(4000);
    expect(clearTimings(d).slides.every((s) => s.advanceAfter === undefined)).toBe(true);
  });
  it("formats and defaults kiosk delay", () => {
    expect(formatDuration(65000)).toBe("1:05");
    expect(kioskDelay(undefined)).toBe(6000);
    expect(kioskDelay(2500)).toBe(2500);
  });
});
