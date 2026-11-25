/** Pure helpers of the Stories player (no DOM), so they can be unit tested. */

export const SPEEDS = [0.75, 1, 1.25, 1.5, 1.75, 2] as const;

export interface Sentence {
  text: string;
  speaker?: string;
  para: number;
}
export interface Timing {
  i: number;
  s: number;
  e: number;
}

/** Next speed in the list (dir +1 faster, -1 slower), staying inside 0.75..2. */
export function stepSpeed(cur: number, dir: 1 | -1): number {
  let idx = SPEEDS.findIndex((s) => Math.abs(s - cur) < 0.001);
  if (idx < 0) idx = SPEEDS.findIndex((s) => s >= cur);
  if (idx < 0) idx = SPEEDS.length - 1;
  return SPEEDS[Math.min(SPEEDS.length - 1, Math.max(0, idx + dir))];
}

export function clampSpeed(v: number): number {
  if (!Number.isFinite(v)) return 1;
  return Math.min(2, Math.max(0.75, v));
}

/** 1:05 or 1:02:03. */
export function fmtTime(ms: number): string {
  const total = Math.max(0, Math.floor((Number.isFinite(ms) ? ms : 0) / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const ss = String(s).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}

/** "45 sec", "12 min", "1 h 05 min". */
export function fmtLength(ms: number): string {
  const min = Math.round(ms / 60000);
  if (ms > 0 && ms < 60000) return `${Math.max(1, Math.round(ms / 1000))} sec`;
  if (min < 60) return `${min} min`;
  return `${Math.floor(min / 60)} h ${String(min % 60).padStart(2, "0")} min`;
}

export function clampSeek(ms: number, durationMs: number): number {
  if (!Number.isFinite(ms)) return 0;
  const max = durationMs > 0 ? durationMs : Number.MAX_SAFE_INTEGER;
  return Math.min(max, Math.max(0, ms));
}

/** Same weighting as the server uses to share a chunk's time between sentences. */
export function weight(s: string): number {
  const n = Array.from(s).length;
  if (/[.!?।]$/.test(s)) return n + 6;
  if (/[,;:]$/.test(s)) return n + 3;
  return n;
}

/** Estimated timings (when the audio has none, and for the browser voice). */
export function estimateTimings(sentences: Sentence[], totalMs: number): Timing[] {
  const total = sentences.reduce((a, s) => a + weight(s.text), 0) || 1;
  let at = 0;
  return sentences.map((s, i) => {
    const start = at;
    at += (totalMs * weight(s.text)) / total;
    return { i, s: Math.round(start), e: Math.round(at) };
  });
}

/** Index of the sentence being spoken at time ms (-1 when there are none). */
export function sentenceAt(timings: Timing[], ms: number): number {
  if (timings.length === 0) return -1;
  let lo = 0;
  let hi = timings.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (timings[mid].s <= ms) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}

export interface Paragraph {
  para: number;
  speaker?: string;
  items: { i: number; text: string }[];
}

/** Groups sentences into paragraphs for the read-along view. */
export function paragraphs(sentences: Sentence[]): Paragraph[] {
  const out: Paragraph[] = [];
  sentences.forEach((s, i) => {
    let p = out[out.length - 1];
    if (!p || p.para !== s.para) {
      p = { para: s.para, speaker: s.speaker, items: [] };
      out.push(p);
    }
    p.items.push({ i, text: s.text });
  });
  return out;
}

export type Sleep = { kind: "off" } | { kind: "minutes"; until: number; minutes: number } | { kind: "end" };

/** Whether playback should stop now because of the sleep timer. */
export function sleepDue(sleep: Sleep, now: number, atEnd: boolean): boolean {
  if (sleep.kind === "minutes") return now >= sleep.until;
  if (sleep.kind === "end") return atEnd;
  return false;
}

export function sleepLabel(sleep: Sleep, now: number): string {
  if (sleep.kind === "minutes") {
    const left = Math.max(0, Math.ceil((sleep.until - now) / 60000));
    return `${left} min left`;
  }
  if (sleep.kind === "end") return "End of episode";
  return "Off";
}

export interface VoiceLike {
  voiceURI: string;
  lang: string;
  name: string;
  localService?: boolean;
}

/** Picks the browser voice for a story language (en, hi, ta ...); a saved choice wins. */
export function chooseVoice<T extends VoiceLike>(voices: T[], lang: string, preferredURI?: string): T | undefined {
  if (preferredURI) {
    const p = voices.find((v) => v.voiceURI === preferredURI && v.lang.toLowerCase().startsWith(lang.toLowerCase()));
    if (p) return p;
  }
  const l = lang.toLowerCase();
  const exact = voices.filter((v) => v.lang.toLowerCase().replace("_", "-").startsWith(l));
  return exact.find((v) => v.localService) ?? exact[0];
}

/** Where to resume: finished episodes start over. */
export function resumePosition(positionMs: number, durationMs: number, completed: boolean): number {
  if (completed) return 0;
  if (durationMs > 0 && positionMs > durationMs - 3000) return 0;
  return Math.max(0, positionMs);
}

/** 0..1 progress of an episode for the underline in lists. */
export function progressFraction(positionMs: number, durationMs: number, completed: boolean): number {
  if (completed) return 1;
  if (durationMs <= 0) return 0;
  return Math.min(1, Math.max(0, positionMs / durationMs));
}

/** Whether the listener's position should be saved now (every 10 s of playback). */
export function shouldSave(lastSavedMs: number, positionMs: number): boolean {
  return Math.abs(positionMs - lastSavedMs) >= 10000;
}

/** Keyboard shortcuts of the player; returns an action name or null. */
export function shortcutFor(e: { key: string; shiftKey?: boolean; ctrlKey?: boolean; metaKey?: boolean; altKey?: boolean }, typing: boolean): string | null {
  if (typing || e.ctrlKey || e.metaKey || e.altKey) return null;
  switch (e.key) {
    case " ":
    case "k":
      return "toggle";
    case "ArrowLeft":
      return e.shiftKey ? "previous" : "back";
    case "ArrowRight":
      return e.shiftKey ? "next" : "forward";
    case "]":
      return "faster";
    case "[":
      return "slower";
    case "m":
      return "mute";
  }
  return null;
}
