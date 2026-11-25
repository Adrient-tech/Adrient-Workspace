import { describe, expect, it } from "vitest";
import { nps, perDay, scaleAverage, topWords } from "./insights";

describe("form insights", () => {
  it("computes the recommendation score", () => {
    const r = nps({ "10": 4, "9": 2, "8": 2, "7": 1, "6": 1, "0": 2, x: 5 });
    expect(r).toEqual({ score: 25, promoters: 6, passives: 3, detractors: 3, total: 12 });
    expect(nps({})).toBeNull();
    expect(nps({ "11": 3, "-1": 2 })).toBeNull();
  });
  it("averages a scale and ignores non numbers", () => {
    expect(scaleAverage({ "1": 1, "5": 3, other: 9 })).toBe(4);
    expect(scaleAverage({})).toBeNull();
  });
  it("finds frequent words, one vote per answer, no stop words", () => {
    const w = topWords(["The support was slow slow slow", "slow support, great team", "Great product and support"]);
    expect(w).toEqual([{ word: "support", count: 3 }, { word: "great", count: 2 }, { word: "slow", count: 2 }]);
  });
  it("buckets responses per day, oldest first", () => {
    const now = new Date(2030, 0, 10, 12);
    const r = perDay([new Date(2030, 0, 10, 1).toISOString(), new Date(2030, 0, 10, 23).toISOString(), new Date(2030, 0, 8).toISOString(), new Date(2029, 5, 1).toISOString(), "junk"], 3, now);
    expect(r).toEqual([{ day: "2030-01-08", count: 1 }, { day: "2030-01-09", count: 0 }, { day: "2030-01-10", count: 2 }]);
  });
});
