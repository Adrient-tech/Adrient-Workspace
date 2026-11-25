import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createVideoTools, vttToText } from "./tools";

const V = (id: string, title: string) => ({ id, title, description: "d", owner_name: "Ann", duration_seconds: 120, views: 3, created_at: "2026-11-01T00:00:00Z", has_captions: true, visibility: "org" });
const VTT = "WEBVTT\n\n1\n00:00:00.500 --> 00:00:04.000\nWelcome, and thanks.\n\n2\n00:01:05.000 --> 00:01:09.000\nSecond <b>point</b>.\n";

let calls: { url: string; method: string }[] = [];
beforeEach(() => {
  calls = [];
  vi.stubGlobal("fetch", async (url: string, init?: RequestInit) => {
    calls.push({ url, method: init?.method ?? "GET" });
    let body: unknown = {};
    if (url.includes("/videos/x/feed")) body = { videos: [V("v1", "Planning"), V("v2", "Mail tips")] };
    else if (url.includes("/videos/x/videos/v1")) body = V("v1", "Planning");
    else if (url.includes("/videos/v1/captions")) body = { captions: [{ id: "c1", lang: "en", stream_url: "/api/v1/videos/captions/c1/content" }] };
    else if (url.includes("/captions/c1/content")) return new Response(VTT, { status: 200 });
    else if (url.endsWith("/video/playlists")) body = { id: "p1", name: "Mix" };
    return new Response(JSON.stringify(body), { status: 200 });
  });
});
afterEach(() => vi.unstubAllGlobals());

const tool = (name: string, cur: string | null = null) => createVideoTools({ currentVideoId: () => cur }).find((t) => t.name === name)!;

describe("vttToText", () => {
  it("keeps the words with their start times and strips tags", () => {
    expect(vttToText(VTT)).toBe("[0:00] Welcome, and thanks.\n[1:05] Second point.");
  });
});

describe("video assistant tools", () => {
  it("finds videos and passes the filters to the feed", async () => {
    const r = (await tool("video_find").handler({ query: "plan", duration: "short" })) as { result: { rows: unknown[] } };
    expect(r.result.rows).toHaveLength(2);
    expect(calls[0].url).toContain("q=plan");
    expect(calls[0].url).toContain("duration=short");
  });
  it("reads captions of the open video as untrusted text", async () => {
    const r = (await tool("video_captions", "v1").handler({})) as { result: { transcript: { untrusted_text: string }; captions: boolean } };
    expect(r.result.captions).toBe(true);
    expect(r.result.transcript.untrusted_text).toContain("[1:05] Second point.");
  });
  it("says so when there is no video", async () => {
    const r = (await tool("video_captions").handler({})) as { error?: string };
    expect(r.error).toMatch(/video_find/);
  });
  it("saving to Watch later is a confirmed write", async () => {
    const t = tool("video_watch_later");
    expect(t.risk).toBe("destructive");
    await t.handler({ video_ids: ["v1", "v2"] });
    expect(calls.filter((c) => c.url.includes("/watch-later/") && c.method === "PUT")).toHaveLength(2);
  });
  it("creates a playlist and adds videos in order", async () => {
    const r = (await tool("video_create_playlist").handler({ name: "Mix", video_ids: ["v1", "v2"] })) as { result?: { videos: number } };
    expect(r.result?.videos).toBe(2);
    expect(calls.filter((c) => c.url.includes("/playlists/p1/items"))).toHaveLength(2);
  });
});
