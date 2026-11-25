import { describe, expect, it } from "vitest";
import type { AnimEffect, Slide } from "./model";
import { ANIM_CATALOG, addEffect, buildTimeline, effectCss, effectVars, framesAt, frameStyle, pathEnd, pathVector } from "./animOps";
import { motionPathString, readMotionPath } from "./pptx/motionXml";

const slide = (): Slide =>
  ({ id: "s1", elements: [{ id: "a", type: "shape", x: 100, y: 100, w: 100, h: 60 }] }) as unknown as Slide;

const effect = (kind: AnimEffect["kind"], extra: Partial<AnimEffect> = {}): AnimEffect => ({ id: "e1", el: "a", cls: "path", kind, start: "click", ...extra });

describe("motion paths", () => {
  it("are in the catalogue as a fourth class", () => {
    expect(ANIM_CATALOG.path.map((d) => d.kind)).toEqual(["pathLine", "pathArc", "pathCircle", "pathZigzag"]);
  });

  it("add as an effect that starts on click", () => {
    const r = addEffect(slide(), ["a"], "path", "pathLine");
    expect(r.slide.anims?.[0]).toMatchObject({ cls: "path", kind: "pathLine", start: "click" });
  });

  it("leave a shape at the end of an open path and bring a circle back", () => {
    expect(pathEnd(effect("pathLine", { dx: 120, dy: -40 }))).toEqual({ dx: 120, dy: -40 });
    expect(pathEnd(effect("pathCircle", { dx: 100 }))).toEqual({ dx: 0, dy: 0 });
    expect(pathVector(effect("pathLine"))).toEqual({ dx: 200, dy: 0 });
  });

  it("play with CSS variables and then stay put", () => {
    const s = { ...slide(), anims: [effect("pathLine", { dx: 150, dy: 30 }), { ...effect("pathLine", { dx: 50 }), id: "e2" }] } as Slide;
    const t = buildTimeline(s);
    expect(effectCss(t.groups[1][0])).toContain("anPathLine");
    expect(effectVars(s.anims![0], s.elements[0], 960, 540)).toMatchObject({ "--px": "150px", "--py": "30px" });
    const playing = framesAt(s, t, 2, true).get("a")!;
    expect(playing.vars).toMatchObject({ "--ox": "150px", "--oy": "30px" });
    const done = frameStyle(framesAt(s, t, 2, false).get("a"));
    expect(done?.translate).toBe("200px 30px");
  });
});

describe("motion paths in pptx", () => {
  it("write and read back every kind", () => {
    for (const [kind, dx, dy] of [
      ["pathLine", 192, 108],
      ["pathArc", 288, 0],
      ["pathZigzag", 240, 54],
    ] as const) {
      const back = readMotionPath(motionPathString(effect(kind, { dx, dy })));
      expect(back?.kind).toBe(kind);
      expect(Math.abs((back?.dx ?? 0) - dx)).toBeLessThanOrEqual(1);
      expect(Math.abs((back?.dy ?? 0) - dy)).toBeLessThanOrEqual(1);
    }
    const circle = readMotionPath(motionPathString(effect("pathCircle", { dx: 160 })));
    expect(circle?.kind).toBe("pathCircle");
    expect(Math.abs((circle?.dx ?? 0) - 160)).toBeLessThanOrEqual(2);
  });

  it("refuse text that is not a path", () => {
    expect(readMotionPath("hello")).toBeUndefined();
  });
});
