import { describe, expect, it } from "vitest";
import {
  activeLine,
  appendTrack,
  buildMixes,
  groupAlbums,
  groupArtists,
  humanDuration,
  insertNext,
  matchesQuery,
  moveItem,
  nextIndex,
  parseLRC,
  prevIndex,
  quickPicks,
  removeAt,
  shuffled,
  spread,
} from "./logic";
import type { Track } from "./types";

const mk = (id: string, extra: Partial<Track> = {}): Track => ({
  id,
  org_id: "o",
  owner_id: "u",
  title: `Song ${id}`,
  artist: "A",
  album: "X",
  content_type: "audio/mpeg",
  size: 1,
  duration_seconds: 60,
  stream_url: "",
  created_at: `2026-01-0${(Number(id.replace(/\D/g, "")) % 9) + 1}T00:00:00Z`,
  updated_at: "",
  ...extra,
});
const ids = (q: Track[]) => q.map((t) => t.id).join("");

describe("queue operations", () => {
  const base = { queue: ["a", "b", "c", "d"].map((i) => mk(i)), index: 1 };
  it("play next inserts after the current track", () => {
    expect(ids(insertNext(base, mk("x")).queue)).toBe("abxcd");
  });
  it("play next on an idle queue appends", () => {
    expect(ids(insertNext({ queue: base.queue, index: -1 }, mk("x")).queue)).toBe("abcdx");
  });
  it("add to queue appends and keeps the index", () => {
    const r = appendTrack(base, mk("x"));
    expect(ids(r.queue)).toBe("abcdx");
    expect(r.index).toBe(1);
  });
  it("moving an item keeps the current track current", () => {
    expect(moveItem(base, 3, 0).index).toBe(2); // d jumps above current b
    expect(ids(moveItem(base, 3, 0).queue)).toBe("dabc");
    expect(moveItem(base, 0, 3).index).toBe(0); // a moves below, b slides up
    expect(moveItem(base, 1, 3).index).toBe(3); // moving the current one
    expect(moveItem(base, 2, 3).index).toBe(1); // neither side of current
  });
  it("ignores out-of-range moves", () => {
    expect(moveItem(base, 9, 0)).toBe(base);
    expect(moveItem(base, 1, 1)).toBe(base);
  });
  it("removing before the current shifts the index", () => {
    const r = removeAt(base, 0);
    expect(r.index).toBe(0);
    expect(ids(r.queue)).toBe("bcd");
    expect(r.removedCurrent).toBe(false);
  });
  it("removing the current track selects the one that slides in", () => {
    const r = removeAt(base, 1);
    expect(ids(r.queue)).toBe("acd");
    expect(r.index).toBe(1);
    expect(r.removedCurrent).toBe(true);
  });
  it("removing the last item of the queue ends playback", () => {
    const r = removeAt({ queue: [mk("a")], index: 0 }, 0);
    expect(r.index).toBe(-1);
    expect(r.queue).toHaveLength(0);
  });
  it("removing the current last entry steps back", () => {
    expect(removeAt({ queue: ["a", "b"].map((i) => mk(i)), index: 1 }, 1).index).toBe(0);
  });
});

describe("next and previous", () => {
  const s = { queue: ["a", "b", "c"].map((i) => mk(i)), index: 2 };
  it("stops at the end without repeat", () => expect(nextIndex(s, "off", false)).toBe(-1));
  it("wraps with repeat all", () => expect(nextIndex(s, "all", false)).toBe(0));
  it("repeat one stays", () => expect(nextIndex(s, "one", false)).toBe(2));
  it("shuffle never picks the current track", () => {
    for (let k = 0; k < 20; k++) expect(nextIndex(s, "off", true, () => k / 20)).not.toBe(2);
  });
  it("previous goes back and wraps only with repeat all", () => {
    expect(prevIndex({ ...s, index: 0 }, "off")).toBe(-1);
    expect(prevIndex({ ...s, index: 0 }, "all")).toBe(2);
    expect(prevIndex(s, "off")).toBe(1);
  });
  it("shuffled keeps every item and optionally the first", () => {
    const out = shuffled([1, 2, 3, 4, 5], () => 0.5, true);
    expect(out[0]).toBe(1);
    expect([...out].sort()).toEqual([1, 2, 3, 4, 5]);
  });
});

describe("lyrics", () => {
  it("parses timed lines, multiple stamps and offset", () => {
    const r = parseLRC("[ti:Song]\n[offset:500]\n[00:01.00]one\n[00:03.50][00:09.00]chorus\n[01:02.5]end");
    expect(r.timed).toBe(true);
    expect(r.lines.map((l) => l.text)).toEqual(["one", "chorus", "chorus", "end"]);
    expect(r.lines[0].time).toBeCloseTo(1.5);
    expect(r.lines[3].time).toBeCloseTo(63);
  });
  it("returns plain lyrics as untimed lines", () => {
    const r = parseLRC("first\n\nsecond");
    expect(r.timed).toBe(false);
    expect(r.lines.map((l) => l.text)).toEqual(["first", "second"]);
  });
  it("finds the active line", () => {
    const { lines } = parseLRC("[00:02.00]a\n[00:05.00]b\n[00:09.00]c");
    expect(activeLine(lines, 0)).toBe(-1);
    expect(activeLine(lines, 2)).toBe(0);
    expect(activeLine(lines, 6)).toBe(1);
    expect(activeLine(lines, 100)).toBe(2);
  });
});

describe("shelves", () => {
  const tracks = [
    mk("1", { artist: "Lumen", genre: "Ambient", album: "Tide", track_no: 2, play_count: 5 }),
    mk("2", { artist: "Lumen", genre: "Ambient", album: "Tide", track_no: 1, play_count: 9 }),
    mk("3", { artist: "Lumen", genre: "Ambient", album: "Glass", liked: true }),
    mk("4", { artist: "Orchard", genre: "Folk", album: "Seat" }),
    mk("5", { artist: "", album: "" }),
  ];
  it("groups albums by title and artist, ordered by track number", () => {
    const a = groupAlbums(tracks);
    const tide = a.find((g) => g.name === "Tide")!;
    expect(tide.tracks.map((t) => t.id)).toEqual(["2", "1"]);
    expect(tide.sub).toBe("Lumen");
    expect(a.find((g) => g.name === "Singles")).toBeTruthy();
  });
  it("groups artists with a song count", () => {
    expect(groupArtists(tracks).find((g) => g.name === "Lumen")!.sub).toBe("3 songs");
    expect(groupArtists(tracks).some((g) => g.name === "Unknown artist")).toBe(true);
  });
  it("builds mixes only from three or more songs", () => {
    const mixes = buildMixes(tracks);
    expect(mixes.map((m) => m.name)).toContain("Ambient mix");
    expect(mixes.some((m) => m.name === "Folk mix")).toBe(false);
  });
  it("spreads artists round robin", () => {
    const t = [mk("a", { artist: "X" }), mk("b", { artist: "X" }), mk("c", { artist: "Y" })];
    expect(ids(spread(t))).toBe("acb");
  });
  it("quick picks rank by plays, then likes, then newest, without repeats", () => {
    const q = quickPicks(tracks, 4);
    expect(q.map((t) => t.id).slice(0, 3)).toEqual(["2", "1", "3"]);
    expect(new Set(q.map((t) => t.id)).size).toBe(q.length);
  });
  it("search needs every word", () => {
    expect(matchesQuery(tracks[0], "lumen tide")).toBe(true);
    expect(matchesQuery(tracks[0], "lumen folk")).toBe(false);
    expect(matchesQuery(tracks[0], "")).toBe(true);
  });
  it("formats durations", () => {
    expect(humanDuration(600)).toBe("10 min");
    expect(humanDuration(3900)).toBe("1 hr 5 min");
  });
});
