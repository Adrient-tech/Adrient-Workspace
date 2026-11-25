/**
 * Pure logic of the Video app: chapters from description timestamps, comment
 * threads, resume position, number and time formatting, and the keyboard map
 * of the player. No React or DOM, so it is unit-tested (logic.test.ts).
 */

export interface Chapter {
  time: number;
  label: string;
}

const STAMP_LINE = /^\s*(?:[-*•]\s*)?(?:(\d{1,2}):)?(\d{1,2}):(\d{2})\b[\s\-–—:.)]*(.*)$/;

/** parseTimestamp reads "1:02", "01:02:03" -> seconds; null if it is not one. */
export function parseTimestamp(s: string): number | null {
  const m = /^(?:(\d{1,2}):)?(\d{1,2}):(\d{2})$/.exec(s.trim());
  if (!m) return null;
  const h = m[1] ? Number(m[1]) : 0;
  const min = Number(m[2]);
  const sec = Number(m[3]);
  if (sec > 59 || (m[1] && min > 59)) return null;
  return h * 3600 + min * 60 + sec;
}

/** parseChapters finds chapter lines ("0:00 Intro") in a description. A valid
 *  list starts at 0:00, has at least two entries and rises strictly. */
export function parseChapters(description: string, duration = 0): Chapter[] {
  const out: Chapter[] = [];
  for (const line of description.split(/\r?\n/)) {
    const m = STAMP_LINE.exec(line);
    if (!m) continue;
    const t = parseTimestamp(`${m[1] ? m[1] + ":" : ""}${m[2]}:${m[3]}`);
    const label = (m[4] ?? "").trim();
    if (t === null || !label) continue;
    if (duration > 0 && t > duration) continue;
    out.push({ time: t, label });
  }
  if (out.length < 2 || out[0].time !== 0) return [];
  for (let i = 1; i < out.length; i++) if (out[i].time <= out[i - 1].time) return [];
  return out;
}

/** chapterAt is the index of the chapter containing time t (-1 if none). */
export function chapterAt(chapters: Chapter[], t: number): number {
  let ans = -1;
  for (let i = 0; i < chapters.length; i++) if (chapters[i].time <= t) ans = i;
  return ans;
}

export type Segment = { kind: "text"; text: string } | { kind: "time"; text: string; seconds: number } | { kind: "link"; text: string; href: string };

const INLINE = /(https?:\/\/[^\s<>"')]+)|(\b(?:\d{1,2}:)?\d{1,2}:\d{2}\b)/g;

/** segmentText splits a description or comment into plain text, clickable
 *  timestamps and http(s) links. Only http and https links are produced. */
export function segmentText(text: string): Segment[] {
  const out: Segment[] = [];
  let last = 0;
  INLINE.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = INLINE.exec(text)) !== null) {
    if (m.index > last) out.push({ kind: "text", text: text.slice(last, m.index) });
    if (m[1]) out.push({ kind: "link", text: m[1], href: m[1] });
    else {
      const sec = parseTimestamp(m[2]);
      out.push(sec === null ? { kind: "text", text: m[2] } : { kind: "time", text: m[2], seconds: sec });
    }
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push({ kind: "text", text: text.slice(last) });
  return out;
}

export interface CommentLike {
  id: string;
  parent_id: string;
  created_at: string;
}

/** buildThreads groups replies under their top-level comment (oldest reply
 *  first); top-level comments are newest first, or by `sort`. */
export function buildThreads<T extends CommentLike & { likes: number }>(comments: T[], sort: "new" | "top" = "new"): { comment: T; replies: T[] }[] {
  const tops = comments.filter((c) => !c.parent_id);
  const rep = new Map<string, T[]>();
  for (const c of comments) if (c.parent_id) rep.set(c.parent_id, [...(rep.get(c.parent_id) ?? []), c]);
  tops.sort((a, b) => (sort === "top" ? b.likes - a.likes || b.created_at.localeCompare(a.created_at) : b.created_at.localeCompare(a.created_at)));
  return tops.map((c) => ({ comment: c, replies: (rep.get(c.id) ?? []).sort((a, b) => a.created_at.localeCompare(b.created_at)) }));
}

/** resumeAt: where playback should start. Near the start or the end means from the top. */
export function resumeAt(position: number, duration: number, watched = false): number {
  if (!isFinite(position) || position < 5) return 0;
  if (watched && duration > 0 && position > duration - 10) return 0;
  if (duration > 0 && position > duration - 10) return 0;
  return Math.floor(position);
}

/** formatCount: 950 -> "950", 1234 -> "1.2K", 2_500_000 -> "2.5M". */
export function formatCount(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10_000 ? 1 : 0).replace(/\.0$/, "")}K`;
  return `${(n / 1_000_000).toFixed(n < 10_000_000 ? 1 : 0).replace(/\.0$/, "")}M`;
}

/** timeAgo: "just now", "5 minutes ago", "3 days ago", "2 months ago". */
export function timeAgo(iso: string, now = Date.now()): string {
  const s = Math.max(0, Math.floor((now - new Date(iso).getTime()) / 1000));
  const units: [number, string][] = [
    [31536000, "year"],
    [2592000, "month"],
    [604800, "week"],
    [86400, "day"],
    [3600, "hour"],
    [60, "minute"],
  ];
  for (const [sec, name] of units) {
    if (s >= sec) {
      const n = Math.floor(s / sec);
      return `${n} ${name}${n === 1 ? "" : "s"} ago`;
    }
  }
  return "just now";
}

/** watchTime: seconds -> "2 h 5 min" / "14 min" / "40 s". */
export function watchTime(sec: number): string {
  if (sec < 90) return `${Math.round(sec)} s`;
  const m = Math.round(sec / 60);
  return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60} min`;
}

export type PlayerAction =
  | { type: "toggle" }
  | { type: "seekBy"; by: number }
  | { type: "seekTo"; fraction: number }
  | { type: "fullscreen" }
  | { type: "theatre" }
  | { type: "mute" }
  | { type: "captions" }
  | { type: "pip" }
  | { type: "speed"; by: number }
  | { type: "volume"; by: number }
  | { type: "frame"; by: number }
  | { type: "chapter"; by: number };

/** keyAction maps a key press to a player action (null = not a player key). */
export function keyAction(key: string, shift = false): PlayerAction | null {
  switch (key) {
    case " ":
    case "k":
    case "K":
      return { type: "toggle" };
    case "ArrowRight":
      return { type: "seekBy", by: 5 };
    case "ArrowLeft":
      return { type: "seekBy", by: -5 };
    case "l":
    case "L":
      return { type: "seekBy", by: 10 };
    case "j":
    case "J":
      return { type: "seekBy", by: -10 };
    case "ArrowUp":
      return { type: "volume", by: 0.05 };
    case "ArrowDown":
      return { type: "volume", by: -0.05 };
    case "f":
    case "F":
      return { type: "fullscreen" };
    case "t":
    case "T":
      return { type: "theatre" };
    case "m":
    case "M":
      return { type: "mute" };
    case "c":
    case "C":
      return { type: "captions" };
    case "i":
    case "I":
      return { type: "pip" };
    case ">":
      return { type: "speed", by: 1 };
    case "<":
      return { type: "speed", by: -1 };
    case ".":
      return shift ? { type: "speed", by: 1 } : { type: "frame", by: 1 };
    case ",":
      return shift ? { type: "speed", by: -1 } : { type: "frame", by: -1 };
    case "N":
      return { type: "chapter", by: 1 };
    case "P":
      return { type: "chapter", by: -1 };
    default:
      if (/^[0-9]$/.test(key)) return { type: "seekTo", fraction: Number(key) / 10 };
      return null;
  }
}

export const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2];

/** stepSpeed moves along SPEEDS by one step and stays inside the list. */
export function stepSpeed(current: number, by: number): number {
  const i = SPEEDS.indexOf(current);
  const base = i === -1 ? SPEEDS.indexOf(1) : i;
  return SPEEDS[Math.min(SPEEDS.length - 1, Math.max(0, base + by))];
}

/** previewTimes picks `n` evenly spaced candidate thumbnail times. */
export function previewTimes(duration: number, n = 4): number[] {
  if (!duration || duration <= 0) return [0];
  return Array.from({ length: n }, (_, i) => Math.min(duration - 0.1, Math.max(0.5, (duration * (i + 1)) / (n + 1))));
}

/** nextUp picks the video after `id` in a list (wraps to nothing at the end). */
export function nextUp<T extends { id: string }>(list: T[], id: string): T | null {
  const i = list.findIndex((v) => v.id === id);
  return i >= 0 && i + 1 < list.length ? list[i + 1] : null;
}
