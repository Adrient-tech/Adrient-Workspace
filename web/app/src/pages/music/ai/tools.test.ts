import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMusicTools, type MusicToolHost } from "./tools";
import type { Track } from "../types";

const mk = (id: string, title: string, artist: string, genre = ""): Track => ({
  id,
  org_id: "o",
  owner_id: "u",
  title,
  artist,
  album: "A",
  genre,
  content_type: "audio/mpeg",
  size: 1,
  duration_seconds: 120,
  stream_url: "",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "",
});
const LIB = [mk("1", "Morning Ferry", "Lumen Harbor", "Ambient"), mk("2", "Night Bus", "Sable Trio", "Jazz"), mk("3", "Corner Table", "Sable Trio", "Jazz")];

let calls: { url: string; method: string; body?: string }[] = [];
beforeEach(() => {
  calls = [];
  vi.stubGlobal("fetch", async (url: string, init?: RequestInit) => {
    calls.push({ url, method: init?.method ?? "GET", body: init?.body as string | undefined });
    const body = url.endsWith("/music/x/library") ? { tracks: LIB } : url.endsWith("/music/playlists") ? { id: "p1", name: "Mix" } : {};
    return new Response(JSON.stringify(body), { status: 200 });
  });
});
afterEach(() => vi.unstubAllGlobals());

function host() {
  const h = {
    next: [] as Track[],
    end: [] as Track[],
    now: [] as Track[],
    current: () => LIB[0],
    queueNext: (t: Track) => void h.next.push(t),
    queueEnd: (t: Track) => void h.end.push(t),
    playNow: (l: Track[]) => void h.now.push(...l),
  };
  return h satisfies MusicToolHost;
}
const tool = (tools: ReturnType<typeof createMusicTools>, name: string) => tools.find((t) => t.name === name)!;

describe("music assistant tools", () => {
  it("finds songs by words and genre", async () => {
    const tools = createMusicTools(host());
    const r = (await tool(tools, "music_find").handler({ query: "sable table" })) as { result: { rows: { id: string }[] } };
    expect(r.result.rows.map((x) => x.id)).toEqual(["3"]);
    const g = (await tool(tools, "music_find").handler({ genre: "jazz" })) as { result: { total: number } };
    expect(g.result.total).toBe(2);
  });
  it("queues at the end, next, or now", async () => {
    const h = host();
    const tools = createMusicTools(h);
    await tool(tools, "music_queue").handler({ track_ids: ["2", "3"] });
    expect(h.end.map((t) => t.id)).toEqual(["2", "3"]);
    await tool(tools, "music_queue").handler({ track_ids: ["2", "3"], mode: "next" });
    expect(h.next.map((t) => t.id)).toEqual(["3", "2"]); // reversed so the order is kept
    await tool(tools, "music_queue").handler({ track_ids: ["1"], mode: "now" });
    expect(h.now.map((t) => t.id)).toEqual(["1"]);
  });
  it("refuses unknown ids", async () => {
    const r = (await tool(createMusicTools(host()), "music_queue").handler({ track_ids: ["nope"] })) as { error?: string };
    expect(r.error).toMatch(/music_find/);
  });
  it("creating a playlist is a confirmed write that adds each song", async () => {
    const t = tool(createMusicTools(host()), "music_create_playlist");
    expect(t.risk).toBe("destructive");
    const r = (await t.handler({ name: "Mix", track_ids: ["2", "3"], visibility: "org" })) as { result?: { songs: number } };
    expect(r.result?.songs).toBe(2);
    expect(calls.filter((c) => c.url.includes("/tracks") && c.method === "POST")).toHaveLength(2);
    expect(calls.some((c) => c.url.includes("/music/x/playlists/p1") && c.method === "PATCH" && c.body?.includes('"org"'))).toBe(true);
  });
  it("reports what is playing", async () => {
    const r = (await tool(createMusicTools(host()), "music_now_playing").handler({})) as { result: { title: string } };
    expect(r.result.title).toBe("Morning Ferry");
  });
});
