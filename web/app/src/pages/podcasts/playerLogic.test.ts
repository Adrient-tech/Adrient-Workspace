import { describe, expect, it } from "vitest";
import {
  chooseVoice, clampSeek, clampSpeed, estimateTimings, fmtLength, fmtTime, paragraphs, progressFraction, resumePosition,
  sentenceAt, shortcutFor, shouldSave, sleepDue, sleepLabel, stepSpeed,
} from "./playerLogic";

const sents = [
  { text: "Hello there.", para: 0 },
  { text: "How are you today?", para: 0 },
  { text: "[x]", para: 1, speaker: "Meera" },
  { text: "Fine, thanks.", para: 1, speaker: "Meera" },
];

describe("speed", () => {
  it("steps inside 0.75-2", () => {
    expect(stepSpeed(1, 1)).toBe(1.25);
    expect(stepSpeed(2, 1)).toBe(2);
    expect(stepSpeed(0.75, -1)).toBe(0.75);
    expect(stepSpeed(1.1, 1)).toBe(1.5);
  });
  it("clamps", () => {
    expect(clampSpeed(5)).toBe(2);
    expect(clampSpeed(0.1)).toBe(0.75);
    expect(clampSpeed(NaN)).toBe(1);
  });
});

describe("time", () => {
  it("formats", () => {
    expect(fmtTime(65000)).toBe("1:05");
    expect(fmtTime(3723000)).toBe("1:02:03");
    expect(fmtTime(-5)).toBe("0:00");
    expect(fmtLength(30000)).toBe("30 sec");
    expect(fmtLength(12 * 60000)).toBe("12 min");
    expect(fmtLength(65 * 60000)).toBe("1 h 05 min");
  });
  it("clamps seeks (10 second skips stay inside the episode)", () => {
    expect(clampSeek(-10000, 60000)).toBe(0);
    expect(clampSeek(70000, 60000)).toBe(60000);
    expect(clampSeek(5000 + 10000, 60000)).toBe(15000);
  });
});

describe("highlighting", () => {
  const t = estimateTimings(sents, 10000);
  it("covers the whole duration without gaps", () => {
    expect(t[0].s).toBe(0);
    expect(t[3].e).toBe(10000);
    for (let i = 1; i < t.length; i++) expect(Math.abs(t[i].s - t[i - 1].e)).toBeLessThanOrEqual(1);
  });
  it("finds the sentence at a time", () => {
    expect(sentenceAt(t, 0)).toBe(0);
    expect(sentenceAt(t, t[2].s + 1)).toBe(2);
    expect(sentenceAt(t, 99999)).toBe(3);
    expect(sentenceAt([], 5)).toBe(-1);
  });
  it("groups paragraphs", () => {
    const p = paragraphs(sents);
    expect(p).toHaveLength(2);
    expect(p[1].speaker).toBe("Meera");
    expect(p[0].items.map((x) => x.i)).toEqual([0, 1]);
  });
});

describe("sleep timer", () => {
  it("stops after minutes or at the end", () => {
    expect(sleepDue({ kind: "minutes", until: 1000, minutes: 5 }, 999, false)).toBe(false);
    expect(sleepDue({ kind: "minutes", until: 1000, minutes: 5 }, 1000, false)).toBe(true);
    expect(sleepDue({ kind: "end" }, 0, false)).toBe(false);
    expect(sleepDue({ kind: "end" }, 0, true)).toBe(true);
    expect(sleepDue({ kind: "off" }, 0, true)).toBe(false);
    expect(sleepLabel({ kind: "minutes", until: 150000, minutes: 5 }, 0)).toBe("3 min left");
  });
});

describe("resume and progress", () => {
  it("resumes where left, restarts finished ones", () => {
    expect(resumePosition(42000, 100000, false)).toBe(42000);
    expect(resumePosition(42000, 100000, true)).toBe(0);
    expect(resumePosition(99000, 100000, false)).toBe(0);
  });
  it("progress fraction", () => {
    expect(progressFraction(50, 100, false)).toBe(0.5);
    expect(progressFraction(0, 100, true)).toBe(1);
    expect(progressFraction(5, 0, false)).toBe(0);
  });
  it("saves every 10 seconds", () => {
    expect(shouldSave(0, 9000)).toBe(false);
    expect(shouldSave(0, 10000)).toBe(true);
    expect(shouldSave(30000, 12000)).toBe(true);
  });
});

describe("browser voice choice", () => {
  const voices = [
    { voiceURI: "a", lang: "en-US", name: "A", localService: false },
    { voiceURI: "b", lang: "hi-IN", name: "B", localService: false },
    { voiceURI: "c", lang: "hi-IN", name: "C", localService: true },
  ];
  it("matches the story language, preferring local voices and a saved choice", () => {
    expect(chooseVoice(voices, "hi")?.voiceURI).toBe("c");
    expect(chooseVoice(voices, "hi", "b")?.voiceURI).toBe("b");
    expect(chooseVoice(voices, "ta")).toBeUndefined();
    expect(chooseVoice(voices, "en")?.voiceURI).toBe("a");
  });
});

describe("keyboard shortcuts", () => {
  it("maps keys", () => {
    expect(shortcutFor({ key: " " }, false)).toBe("toggle");
    expect(shortcutFor({ key: "ArrowLeft" }, false)).toBe("back");
    expect(shortcutFor({ key: "ArrowRight" }, false)).toBe("forward");
    expect(shortcutFor({ key: "ArrowRight", shiftKey: true }, false)).toBe("next");
    expect(shortcutFor({ key: "]" }, false)).toBe("faster");
  });
  it("ignores typing and modifier combos", () => {
    expect(shortcutFor({ key: " " }, true)).toBeNull();
    expect(shortcutFor({ key: "ArrowLeft", ctrlKey: true }, false)).toBeNull();
    expect(shortcutFor({ key: "a" }, false)).toBeNull();
  });
});
