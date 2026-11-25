import { describe, expect, it } from "vitest";
import { buildThreads, chapterAt, formatCount, keyAction, nextUp, parseChapters, parseTimestamp, previewTimes, resumeAt, segmentText, stepSpeed, timeAgo, watchTime } from "./logic";

describe("timestamps and chapters", () => {
  it("parses timestamps", () => {
    expect(parseTimestamp("1:02")).toBe(62);
    expect(parseTimestamp("01:02:03")).toBe(3723);
    expect(parseTimestamp("1:75")).toBeNull();
    expect(parseTimestamp("abc")).toBeNull();
  });
  it("reads a chapter list from a description", () => {
    const d = "About this video\n\nChapters\n0:00 Intro\n0:08 - The main idea\n1:16 A closer look\nBye";
    expect(parseChapters(d)).toEqual([
      { time: 0, label: "Intro" },
      { time: 8, label: "The main idea" },
      { time: 76, label: "A closer look" },
    ]);
  });
  it("ignores lists that do not start at zero, are not rising, or are a single line", () => {
    expect(parseChapters("0:05 A\n0:10 B")).toEqual([]);
    expect(parseChapters("0:00 A\n0:10 B\n0:05 C")).toEqual([]);
    expect(parseChapters("0:00 Only one")).toEqual([]);
  });
  it("drops chapters beyond the video length", () => {
    expect(parseChapters("0:00 A\n0:10 B\n9:00 C", 30)).toHaveLength(2);
  });
  it("finds the current chapter", () => {
    const ch = parseChapters("0:00 A\n0:10 B\n0:20 C");
    expect(chapterAt(ch, 0)).toBe(0);
    expect(chapterAt(ch, 15)).toBe(1);
    expect(chapterAt(ch, 99)).toBe(2);
  });
  it("splits text into plain, timestamp and link segments", () => {
    const s = segmentText("see 1:30 and https://example.org/x for more, ver 3:99");
    expect(s.filter((x) => x.kind === "time")).toEqual([{ kind: "time", text: "1:30", seconds: 90 }]);
    expect(s.find((x) => x.kind === "link")).toEqual({ kind: "link", text: "https://example.org/x", href: "https://example.org/x" });
    expect(s.map((x) => x.text).join("")).toBe("see 1:30 and https://example.org/x for more, ver 3:99");
  });
  it("never makes a link from javascript: urls", () => {
    expect(segmentText("javascript:alert(1)").some((x) => x.kind === "link")).toBe(false);
  });
});

describe("comments", () => {
  const c = (id: string, parent: string, at: string, likes = 0) => ({ id, parent_id: parent, created_at: at, likes });
  it("nests replies oldest first under newest-first comments", () => {
    const t = buildThreads([c("a", "", "2026-01-01"), c("b", "", "2026-01-03"), c("r2", "a", "2026-01-05"), c("r1", "a", "2026-01-04")]);
    expect(t.map((x) => x.comment.id)).toEqual(["b", "a"]);
    expect(t[1].replies.map((r) => r.id)).toEqual(["r1", "r2"]);
  });
  it("can sort by likes", () => {
    const t = buildThreads([c("a", "", "2026-01-01", 5), c("b", "", "2026-01-03", 1)], "top");
    expect(t[0].comment.id).toBe("a");
  });
});

describe("resume, formatting and player keys", () => {
  it("resumes only inside the video", () => {
    expect(resumeAt(3, 100)).toBe(0);
    expect(resumeAt(40.7, 100)).toBe(40);
    expect(resumeAt(95, 100)).toBe(0);
    expect(resumeAt(50, 0)).toBe(50);
  });
  it("formats counts and times", () => {
    expect(formatCount(950)).toBe("950");
    expect(formatCount(1234)).toBe("1.2K");
    expect(formatCount(25_000)).toBe("25K");
    expect(formatCount(2_500_000)).toBe("2.5M");
    const now = new Date("2026-11-21T12:00:00Z").getTime();
    expect(timeAgo("2026-11-21T11:59:40Z", now)).toBe("just now");
    expect(timeAgo("2026-11-21T09:00:00Z", now)).toBe("3 hours ago");
    expect(timeAgo("2026-11-20T09:00:00Z", now)).toBe("1 day ago");
    expect(watchTime(30)).toBe("30 s");
    expect(watchTime(3900)).toBe("1 h 5 min");
  });
  it("maps keys to actions", () => {
    expect(keyAction(" ")).toEqual({ type: "toggle" });
    expect(keyAction("j")).toEqual({ type: "seekBy", by: -10 });
    expect(keyAction("5")).toEqual({ type: "seekTo", fraction: 0.5 });
    expect(keyAction("t")).toEqual({ type: "theatre" });
    expect(keyAction("z")).toBeNull();
    expect(keyAction(".", true)).toEqual({ type: "speed", by: 1 });
  });
  it("steps speed inside the list", () => {
    expect(stepSpeed(1, 1)).toBe(1.25);
    expect(stepSpeed(2, 1)).toBe(2);
    expect(stepSpeed(0.5, -1)).toBe(0.5);
    expect(stepSpeed(1.1, 1)).toBe(1.25);
  });
  it("picks thumbnail candidate times", () => {
    expect(previewTimes(100, 4)).toEqual([20, 40, 60, 80]);
    expect(previewTimes(0)).toEqual([0]);
  });
  it("finds the next video", () => {
    expect(nextUp([{ id: "a" }, { id: "b" }], "a")?.id).toBe("b");
    expect(nextUp([{ id: "a" }, { id: "b" }], "b")).toBeNull();
  });
});
