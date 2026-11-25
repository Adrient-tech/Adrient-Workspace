// Rehearse timings and kiosk helpers. A rehearsal measures how long each slide
// stayed on screen and stores it as the slide's `advanceAfter` (ms).
import type { DeckDoc } from "./model";

export const KIOSK_DEFAULT_MS = 6000;
const MIN_MS = 1000;
const MAX_MS = 3_600_000;

/** Add `ms` of dwell time to slide index `idx` (a slide can be visited twice). */
export function addDwell(dwell: Record<number, number>, idx: number, ms: number): Record<number, number> {
  if (!(ms > 0)) return dwell;
  return { ...dwell, [idx]: (dwell[idx] ?? 0) + ms };
}

/** Round a dwell time to whole seconds, within sane bounds. */
export function clampTiming(ms: number): number {
  return Math.min(MAX_MS, Math.max(MIN_MS, Math.round(ms / 1000) * 1000));
}

/** Store rehearsed timings. `ids[i]` is the slide id of shown-slide `i`; slides never visited keep what they had. */
export function applyRehearsal(doc: DeckDoc, ids: string[], dwell: Record<number, number>): DeckDoc {
  const byId = new Map<string, number>();
  for (const [i, ms] of Object.entries(dwell)) {
    const id = ids[Number(i)];
    if (id && ms > 0) byId.set(id, clampTiming(ms));
  }
  if (!byId.size) return doc;
  return { ...doc, slides: doc.slides.map((s) => (byId.has(s.id) ? { ...s, advanceAfter: byId.get(s.id) } : s)) };
}

export function clearTimings(doc: DeckDoc): DeckDoc {
  return {
    ...doc,
    slides: doc.slides.map((s) => {
      if (s.advanceAfter === undefined) return s;
      const { advanceAfter: _a, ...rest } = s;
      return rest;
    }),
  };
}

export function totalTime(doc: DeckDoc): number {
  return doc.slides.filter((s) => !s.hidden).reduce((n, s) => n + (s.advanceAfter ?? 0), 0);
}

export function formatDuration(ms: number): string {
  const t = Math.round(ms / 1000);
  const m = Math.floor(t / 60);
  const s = t % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

/** Delay before a kiosk show advances: the slide's timing, else a default. */
export function kioskDelay(advanceAfter: number | undefined): number {
  return advanceAfter && advanceAfter > 0 ? advanceAfter : KIOSK_DEFAULT_MS;
}
