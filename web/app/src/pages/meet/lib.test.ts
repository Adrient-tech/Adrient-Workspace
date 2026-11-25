import { describe, expect, it } from "vitest";
import {
  SpeakingDetector,
  computeGridLayout,
  computeSpotlightLayout,
  pickActiveSpeaker,
  pickFeatured,
  shortcutAction,
  videoQualityFor,
} from "./lib";

describe("grid layout", () => {
  it("places every tile inside the container", () => {
    for (const n of [1, 2, 3, 5, 9, 16]) {
      const { rects } = computeGridLayout(n, 1000, 600);
      expect(rects).toHaveLength(n);
      for (const r of rects) {
        expect(r.x).toBeGreaterThanOrEqual(0);
        expect(r.x + r.w).toBeLessThanOrEqual(1000.01);
        expect(r.y + r.h).toBeLessThanOrEqual(600.01);
      }
    }
  });
  it("handles empty/zero", () => {
    expect(computeGridLayout(0, 100, 100).rects).toEqual([]);
    expect(computeGridLayout(3, 0, 100).rects).toEqual([]);
  });
  it("spotlight puts strip beside featured in landscape", () => {
    const l = computeSpotlightLayout(3, 1000, 600);
    expect(l.strip).toHaveLength(3);
    expect(l.featured.w + 12 + l.strip[0].w).toBeCloseTo(1000, 0);
  });
});

describe("pickFeatured", () => {
  const tiles = [{ id: "a" }, { id: "b" }, { id: "c", sharing: true }];
  it("auto: grid unless sharing or pinned", () => {
    expect(pickFeatured("auto", [{ id: "a" }, { id: "b" }], null, "b")).toBeNull();
    expect(pickFeatured("auto", tiles, null, "a")).toBe("c");
    expect(pickFeatured("auto", [{ id: "a" }, { id: "b" }], "b", null)).toBe("b");
  });
  it("tiled ignores speaker, honours pin", () => {
    expect(pickFeatured("tiled", [{ id: "a" }, { id: "b" }], null, "b")).toBeNull();
    expect(pickFeatured("tiled", [{ id: "a" }, { id: "b" }], "a", "b")).toBe("a");
  });
  it("spotlight follows active speaker", () => {
    expect(pickFeatured("spotlight", [{ id: "a" }, { id: "b" }], null, "b")).toBe("b");
    expect(pickFeatured("spotlight", [{ id: "a" }, { id: "b" }], null, null)).toBe("a");
  });
  it("stale pin is ignored", () => {
    expect(pickFeatured("spotlight", [{ id: "a" }, { id: "b" }], "zzz", "b")).toBe("b");
  });
});

describe("speaking", () => {
  it("holds speaking until quiet for holdMs", () => {
    const d = new SpeakingDetector(0.02, 0.01, 900);
    expect(d.update(0.05, 0)).toBe(true);
    expect(d.update(0.001, 100)).toBe(true);
    expect(d.update(0.001, 1100)).toBe(false);
  });
  it("loudest speaker wins", () => {
    expect(
      pickActiveSpeaker({ a: 0.1, b: 0.3 }, { a: true, b: true }, null),
    ).toBe("b");
    expect(pickActiveSpeaker({ a: 0 }, { a: false }, null)).toBeNull();
  });
});

describe("adaptive video + shortcuts", () => {
  it("drops quality as the call grows", () => {
    expect(videoQualityFor(2).height).toBeGreaterThan(videoQualityFor(6).height);
    expect(videoQualityFor(6).maxBitrate).toBeGreaterThan(videoQualityFor(20).maxBitrate);
  });
  const base = { ctrlKey: true, metaKey: false, altKey: false, shiftKey: false };
  it("maps ctrl+d / ctrl+e", () => {
    expect(shortcutAction({ ...base, key: "d" })).toBe("mic");
    expect(shortcutAction({ ...base, key: "E" })).toBe("cam");
    expect(shortcutAction({ ...base, key: "x" })).toBeNull();
    expect(shortcutAction({ ...base, key: "d", ctrlKey: false })).toBeNull();
  });
});
