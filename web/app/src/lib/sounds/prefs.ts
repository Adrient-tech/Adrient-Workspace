import { CATEGORIES, SOUND_SETS, type SoundCategory, type SoundSet } from "./catalog";

/** Quiet hours / Do Not Disturb. */
export interface DndPrefs {
  /** Manual switch: everything is silent while on. */
  on: boolean;
  /** Scheduled quiet hours, "HH:MM" local time; may wrap past midnight. */
  scheduled: boolean;
  from: string;
  to: string;
}

export interface SoundPrefs {
  /** Master volume 0..1. */
  volume: number;
  set: SoundSet;
  enabled: Record<SoundCategory, boolean>;
  dnd: DndPrefs;
  /** Stay quiet (except calls) when the OS asks for reduced motion/noise. */
  followSystem: boolean;
  /** Desktop notifications opted in. undefined = never asked. */
  desktop?: boolean;
}

export const DEFAULT_PREFS: SoundPrefs = {
  volume: 0.7,
  set: "classic",
  enabled: Object.fromEntries(CATEGORIES.map((c) => [c, true])) as Record<SoundCategory, boolean>,
  dnd: { on: false, scheduled: false, from: "22:00", to: "07:00" },
  followSystem: true,
};

const TIME_RE = /^([01]\d|2[0-3]):[0-5]\d$/;

/** Normalises anything read from storage or the server into valid prefs. */
export function sanitizePrefs(raw: unknown): SoundPrefs {
  const r = (raw && typeof raw === "object" ? raw : {}) as Record<string, unknown>;
  const vol = typeof r.volume === "number" && isFinite(r.volume) ? Math.min(1, Math.max(0, r.volume)) : DEFAULT_PREFS.volume;
  const set = SOUND_SETS.includes(r.set as SoundSet) ? (r.set as SoundSet) : DEFAULT_PREFS.set;
  const en = (r.enabled && typeof r.enabled === "object" ? r.enabled : {}) as Record<string, unknown>;
  const enabled = { ...DEFAULT_PREFS.enabled };
  for (const c of CATEGORIES) if (typeof en[c] === "boolean") enabled[c] = en[c] as boolean;
  const d = (r.dnd && typeof r.dnd === "object" ? r.dnd : {}) as Record<string, unknown>;
  const dnd: DndPrefs = {
    on: d.on === true,
    scheduled: d.scheduled === true,
    from: typeof d.from === "string" && TIME_RE.test(d.from) ? d.from : DEFAULT_PREFS.dnd.from,
    to: typeof d.to === "string" && TIME_RE.test(d.to) ? d.to : DEFAULT_PREFS.dnd.to,
  };
  return {
    volume: vol,
    set,
    enabled,
    dnd,
    followSystem: r.followSystem !== false,
    desktop: typeof r.desktop === "boolean" ? r.desktop : undefined,
  };
}

function minutes(hhmm: string): number {
  const [h, m] = hhmm.split(":").map(Number);
  return h * 60 + m;
}

/** True when `now` falls inside the quiet hours (handles ranges that wrap midnight). */
export function inQuietHours(dnd: DndPrefs, now: Date): boolean {
  if (!dnd.scheduled) return false;
  const cur = now.getHours() * 60 + now.getMinutes();
  const a = minutes(dnd.from);
  const b = minutes(dnd.to);
  if (a === b) return false;
  return a < b ? cur >= a && cur < b : cur >= a || cur < b;
}

/** True when Do Not Disturb is active right now. */
export function isDnd(dnd: DndPrefs, now: Date = new Date()): boolean {
  return dnd.on || inQuietHours(dnd, now);
}

export const PREFS_EXTRA_KEY = "sounds";
