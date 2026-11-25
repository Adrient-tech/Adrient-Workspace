/**
 * Pure logic of the Music app: play queue operations, synced-lyrics parsing,
 * and the groupings behind the home shelves. Kept free of React and the DOM so
 * it can be unit-tested (logic.test.ts).
 */
import type { Track } from "./types";

export type RepeatMode = "off" | "all" | "one";

export interface QueueState {
  queue: Track[];
  /** Index of the current track, or -1 when idle. */
  index: number;
}

/** insertNext puts a track right after the current one (or at the end when idle). */
export function insertNext(s: QueueState, t: Track): QueueState {
  const at = s.index >= 0 ? s.index + 1 : s.queue.length;
  return { queue: [...s.queue.slice(0, at), t, ...s.queue.slice(at)], index: s.index };
}

/** appendTrack adds a track at the end of the queue. */
export function appendTrack(s: QueueState, t: Track): QueueState {
  return { queue: [...s.queue, t], index: s.index };
}

/** moveItem moves one queue entry and keeps the current index pointing at the same track. */
export function moveItem(s: QueueState, from: number, to: number): QueueState {
  const n = s.queue.length;
  if (from === to || from < 0 || to < 0 || from >= n || to >= n) return s;
  const q = s.queue.slice();
  const [item] = q.splice(from, 1);
  q.splice(to, 0, item);
  let idx = s.index;
  if (idx === from) idx = to;
  else if (from < idx && to >= idx) idx -= 1;
  else if (from > idx && to <= idx) idx += 1;
  return { queue: q, index: idx };
}

/** removeAt drops one queue entry. `removedCurrent` is true when it was the playing one. */
export function removeAt(s: QueueState, at: number): QueueState & { removedCurrent: boolean } {
  if (at < 0 || at >= s.queue.length) return { ...s, removedCurrent: false };
  const q = s.queue.filter((_, i) => i !== at);
  if (at < s.index) return { queue: q, index: s.index - 1, removedCurrent: false };
  if (at === s.index) {
    // The next track slides into this slot; stay on it unless the queue ended.
    return { queue: q, index: q.length === 0 ? -1 : Math.min(at, q.length - 1), removedCurrent: true };
  }
  return { queue: q, index: s.index, removedCurrent: false };
}

/** nextIndex is the index that plays after the current track ends (-1 = stop). */
export function nextIndex(s: QueueState, repeat: RepeatMode, shuffle: boolean, rng: () => number = Math.random): number {
  const n = s.queue.length;
  if (n === 0) return -1;
  if (repeat === "one") return s.index;
  if (shuffle) {
    if (n === 1) return repeat === "all" ? 0 : -1;
    let r = Math.floor(rng() * (n - 1));
    if (r >= s.index) r += 1;
    return r;
  }
  const ni = s.index + 1;
  if (ni < n) return ni;
  return repeat === "all" ? 0 : -1;
}

/** prevIndex is the index "previous" goes to (-1 = nothing before). */
export function prevIndex(s: QueueState, repeat: RepeatMode): number {
  const n = s.queue.length;
  if (n === 0) return -1;
  if (s.index > 0) return s.index - 1;
  return repeat === "all" ? n - 1 : -1;
}

/** shuffled returns a Fisher-Yates copy, keeping `keepFirst` (the playing track) at the front. */
export function shuffled<T>(items: T[], rng: () => number = Math.random, keepFirst = false): T[] {
  const a = items.slice();
  for (let i = a.length - 1; i > (keepFirst ? 1 : 0); i--) {
    const j = (keepFirst ? 1 : 0) + Math.floor(rng() * (i - (keepFirst ? 1 : 0) + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}

// ---- synced lyrics ----------------------------------------------------------

export interface LyricLine {
  /** Seconds from the start; -1 for plain (untimed) lyrics. */
  time: number;
  text: string;
}

const STAMP = /\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]/g;

/** parseLRC reads timed lyrics ("[mm:ss.xx] words"). Lines with several stamps
 *  repeat; "[offset:ms]" shifts all times. Text without any stamp comes back
 *  as untimed lines (time -1). */
export function parseLRC(text: string): { timed: boolean; lines: LyricLine[] } {
  const out: LyricLine[] = [];
  let offset = 0;
  let any = false;
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line) continue;
    const off = /^\[offset:\s*(-?\d+)\s*\]$/i.exec(line);
    if (off) {
      offset = Number(off[1]) / 1000;
      continue;
    }
    if (/^\[[a-z]{2,}:[^\]]*\]$/i.test(line)) continue; // [ti:...] [ar:...] tags
    const stamps: number[] = [];
    STAMP.lastIndex = 0;
    let m: RegExpExecArray | null;
    let last = 0;
    while ((m = STAMP.exec(line)) !== null) {
      const frac = m[3] ? Number(m[3].padEnd(3, "0").slice(0, 3)) / 1000 : 0;
      stamps.push(Number(m[1]) * 60 + Number(m[2]) + frac);
      last = STAMP.lastIndex;
    }
    if (stamps.length) {
      any = true;
      const words = line.slice(last).trim();
      for (const t of stamps) out.push({ time: Math.max(0, t + offset), text: words });
    } else {
      out.push({ time: -1, text: line });
    }
  }
  if (!any) return { timed: false, lines: out.map((l) => ({ time: -1, text: l.text })) };
  return { timed: true, lines: out.filter((l) => l.time >= 0).sort((a, b) => a.time - b.time) };
}

/** activeLine is the index of the line being sung at time t (-1 before the first). */
export function activeLine(lines: LyricLine[], t: number): number {
  let lo = 0;
  let hi = lines.length - 1;
  let ans = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (lines[mid].time <= t) {
      ans = mid;
      lo = mid + 1;
    } else hi = mid - 1;
  }
  return ans;
}

// ---- groupings --------------------------------------------------------------

export interface Group {
  key: string;
  name: string;
  /** Second line: artist for albums, track count for artists. */
  sub: string;
  tracks: Track[];
}

const UNKNOWN_ARTIST = "Unknown artist";
const UNKNOWN_ALBUM = "Singles";

export const artistOf = (t: Track) => t.artist?.trim() || UNKNOWN_ARTIST;
export const albumOf = (t: Track) => t.album?.trim() || UNKNOWN_ALBUM;

/** albumKey keeps two albums with the same title by different artists apart. */
export const albumKey = (t: Track) => `${albumOf(t).toLowerCase()}\u0001${artistOf(t).toLowerCase()}`;

function sortByNo(a: Track, b: Track): number {
  return (a.track_no ?? 0) - (b.track_no ?? 0) || a.title.localeCompare(b.title);
}

export function groupAlbums(tracks: Track[]): Group[] {
  const m = new Map<string, Group>();
  for (const t of tracks) {
    const k = albumKey(t);
    const g = m.get(k) ?? { key: k, name: albumOf(t), sub: artistOf(t), tracks: [] };
    g.tracks.push(t);
    m.set(k, g);
  }
  return [...m.values()].map((g) => ({ ...g, tracks: g.tracks.slice().sort(sortByNo) })).sort((a, b) => a.name.localeCompare(b.name));
}

export function groupArtists(tracks: Track[]): Group[] {
  const m = new Map<string, Group>();
  for (const t of tracks) {
    const k = artistOf(t).toLowerCase();
    const g = m.get(k) ?? { key: k, name: artistOf(t), sub: "", tracks: [] };
    g.tracks.push(t);
    m.set(k, g);
  }
  return [...m.values()]
    .map((g) => ({ ...g, sub: `${g.tracks.length} song${g.tracks.length === 1 ? "" : "s"}` }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

/** buildMixes makes "mixes": one per genre and one per artist with at least
 *  three songs, most songs first. Order inside a mix is stable but spread
 *  across artists so a mix does not play one album front to back. */
export function buildMixes(tracks: Track[], max = 8): Group[] {
  const out: Group[] = [];
  const byGenre = new Map<string, Track[]>();
  for (const t of tracks) {
    const g = t.genre?.trim();
    if (!g) continue;
    byGenre.set(g, [...(byGenre.get(g) ?? []), t]);
  }
  for (const [g, ts] of byGenre) if (ts.length >= 3) out.push({ key: `genre:${g}`, name: `${g} mix`, sub: sample(ts), tracks: spread(ts) });
  for (const a of groupArtists(tracks)) {
    if (a.tracks.length >= 3 && a.name !== UNKNOWN_ARTIST) out.push({ key: `artist:${a.key}`, name: `${a.name} mix`, sub: sample(a.tracks), tracks: spread(a.tracks) });
  }
  return out.sort((x, y) => y.tracks.length - x.tracks.length || x.name.localeCompare(y.name)).slice(0, max);
}

function sample(ts: Track[]): string {
  const names = [...new Set(ts.map(artistOf))].slice(0, 3);
  return names.join(", ");
}

/** spread interleaves tracks by artist (round robin). */
export function spread(ts: Track[]): Track[] {
  const by = new Map<string, Track[]>();
  for (const t of ts) by.set(artistOf(t), [...(by.get(artistOf(t)) ?? []), t]);
  const lists = [...by.values()];
  const out: Track[] = [];
  for (let i = 0; out.length < ts.length; i++) {
    for (const l of lists) if (i < l.length) out.push(l[i]);
  }
  return out;
}

/** quickPicks: what the user plays most, topped up with liked and newest
 *  songs, no repeats. */
export function quickPicks(tracks: Track[], n = 12): Track[] {
  const seen = new Set<string>();
  const out: Track[] = [];
  const add = (list: Track[]) => {
    for (const t of list) {
      if (out.length >= n) return;
      if (!seen.has(t.id)) {
        seen.add(t.id);
        out.push(t);
      }
    }
  };
  add(tracks.filter((t) => (t.play_count ?? 0) > 0).sort((a, b) => (b.play_count ?? 0) - (a.play_count ?? 0)));
  add(tracks.filter((t) => t.liked));
  add(tracks.slice().sort((a, b) => b.created_at.localeCompare(a.created_at)));
  return out;
}

/** matchesQuery is the search filter: every word must appear in title, artist, album or genre. */
export function matchesQuery(t: Track, q: string): boolean {
  const words = q.toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return true;
  const hay = `${t.title} ${t.artist} ${t.album} ${t.genre ?? ""}`.toLowerCase();
  return words.every((w) => hay.includes(w));
}

/** totalDuration sums track lengths (seconds). */
export const totalDuration = (ts: Track[]) => ts.reduce((s, t) => s + (t.duration_seconds || 0), 0);

/** humanDuration: "1 hr 5 min" / "12 min". */
export function humanDuration(sec: number): string {
  const m = Math.round(sec / 60);
  if (m < 60) return `${m} min`;
  return `${Math.floor(m / 60)} hr ${m % 60} min`;
}
