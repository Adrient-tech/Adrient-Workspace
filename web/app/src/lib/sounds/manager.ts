import {
  CATEGORY_OF,
  LOOPING,
  SOUND_IDS,
  soundUrl,
  type SoundId,
} from "./catalog";
import { DEFAULT_PREFS, isDnd, sanitizePrefs, type SoundPrefs } from "./prefs";

/** The slice of HTMLAudioElement the manager uses (so tests can fake it). */
export interface AudioLike {
  src: string;
  volume: number;
  loop: boolean;
  currentTime: number;
  preload: string;
  play(): Promise<void>;
  pause(): void;
  addEventListener(type: string, fn: () => void): void;
}

export type SoundEventKind = "play" | "loop" | "stop" | "blocked" | "skipped";
export interface SoundEvent {
  id: SoundId;
  kind: SoundEventKind;
  /** Why a sound was skipped: disabled | category | dnd | system | volume. */
  reason?: string;
}

export interface ManagerDeps {
  createAudio: (url: string) => AudioLike;
  now: () => Date;
  /** True when the OS asks for less motion/noise. */
  systemQuiet: () => boolean;
  /** Loops stop by themselves after this long so a forgotten ring cannot play forever. */
  maxLoopMs: number;
}

const browserDeps = (): ManagerDeps => ({
  createAudio: (url) => {
    const a = new Audio();
    a.preload = "auto";
    a.src = url;
    return a as AudioLike;
  },
  now: () => new Date(),
  systemQuiet: () =>
    typeof window !== "undefined" && typeof window.matchMedia === "function"
      ? window.matchMedia("(prefers-reduced-motion: reduce)").matches
      : false,
  maxLoopMs: 60_000,
});

export class SoundManager {
  private prefs: SoundPrefs = DEFAULT_PREFS;
  private els = new Map<SoundId, AudioLike>();
  private loops = new Map<SoundId, ReturnType<typeof setTimeout>>();
  /** Loops requested before the browser allowed audio: started on first gesture. */
  private pendingLoops = new Set<SoundId>();
  private unlocked = false;
  private listeners = new Set<(e: SoundEvent) => void>();
  private prefListeners = new Set<(p: SoundPrefs) => void>();

  constructor(private deps: ManagerDeps = browserDeps()) {}

  // ---- preferences -------------------------------------------------------
  getPrefs(): SoundPrefs {
    return this.prefs;
  }

  setPrefs(next: unknown) {
    const p = sanitizePrefs(next);
    const setChanged = p.set !== this.prefs.set;
    this.prefs = p;
    if (setChanged) this.reload();
    for (const [id, el] of this.els) el.volume = this.volumeFor(id);
    // A switch that silences a looping sound stops it right away.
    for (const id of [...this.loops.keys()]) if (this.blockReason(id)) this.stop(id);
    this.prefListeners.forEach((f) => f(this.prefs));
  }

  onPrefs(fn: (p: SoundPrefs) => void) {
    this.prefListeners.add(fn);
    return () => this.prefListeners.delete(fn);
  }

  onEvent(fn: (e: SoundEvent) => void) {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  private emit(e: SoundEvent) {
    this.listeners.forEach((f) => f(e));
    if (typeof window !== "undefined") window.dispatchEvent(new CustomEvent("adrient:sound", { detail: e }));
  }

  // ---- loading -----------------------------------------------------------
  /** Creates (and starts loading) the audio element of every sound in the current set. */
  preload() {
    for (const id of SOUND_IDS) this.el(id);
  }

  private reload() {
    const wasLooping = [...this.loops.keys()];
    for (const id of wasLooping) this.stop(id);
    this.els.clear();
    this.preload();
  }

  private el(id: SoundId): AudioLike {
    let el = this.els.get(id);
    if (!el) {
      el = this.deps.createAudio(soundUrl(this.prefs.set, id));
      el.loop = LOOPING.has(id);
      el.volume = this.volumeFor(id);
      this.els.set(id, el);
    }
    return el;
  }

  /** Master volume; the ringtone is slightly louder so it is not missed. */
  private volumeFor(id: SoundId): number {
    const v = this.prefs.volume * (LOOPING.has(id) ? 1 : 0.9);
    return Math.min(1, Math.max(0, v));
  }

  // ---- gating ------------------------------------------------------------
  /** Why `id` must stay silent right now, or "" when it may play. */
  blockReason(id: SoundId, opts: { force?: boolean } = {}): string {
    const p = this.prefs;
    if (p.volume <= 0) return "volume";
    if (opts.force) return "";
    if (!p.enabled[CATEGORY_OF[id]]) return "category";
    if (isDnd(p.dnd, this.deps.now())) return "dnd";
    if (p.followSystem && CATEGORY_OF[id] !== "calls" && this.deps.systemQuiet()) return "system";
    return "";
  }

  // ---- playback ----------------------------------------------------------
  /**
   * Starts listening for the first user gesture; browsers refuse audio before
   * one. Loops that were requested earlier start once it arrives.
   */
  unlockOnGesture(target: Pick<Window, "addEventListener" | "removeEventListener"> = window) {
    if (this.unlocked) return;
    const go = () => {
      this.unlocked = true;
      for (const t of ["pointerdown", "keydown", "touchstart"]) target.removeEventListener(t, go, true);
      this.preload();
      for (const id of [...this.pendingLoops]) {
        this.pendingLoops.delete(id);
        void this.loop(id);
      }
    };
    for (const t of ["pointerdown", "keydown", "touchstart"]) target.addEventListener(t, go, true);
  }

  /** Test hook / explicit unlock (e.g. after a click handler). */
  markUnlocked() {
    this.unlocked = true;
  }

  /** Plays a one-shot sound. Resolves true when the browser accepted it. */
  async play(id: SoundId, opts: { force?: boolean } = {}): Promise<boolean> {
    const why = this.blockReason(id, opts);
    if (why) {
      this.emit({ id, kind: "skipped", reason: why });
      return false;
    }
    const el = this.el(id);
    try {
      el.currentTime = 0;
    } catch {
      /* not seekable yet */
    }
    el.volume = this.volumeFor(id);
    try {
      await el.play();
      this.emit({ id, kind: "play" });
      return true;
    } catch {
      this.emit({ id, kind: "blocked", reason: "autoplay" });
      return false;
    }
  }

  /** Starts a looping sound (ringtone, ringback) until stop() or the safety timeout. */
  async loop(id: SoundId, opts: { force?: boolean } = {}): Promise<boolean> {
    if (this.loops.has(id)) return true;
    const why = this.blockReason(id, opts);
    if (why) {
      this.emit({ id, kind: "skipped", reason: why });
      return false;
    }
    const el = this.el(id);
    el.loop = true;
    el.volume = this.volumeFor(id);
    try {
      el.currentTime = 0;
    } catch {
      /* ignore */
    }
    try {
      await el.play();
    } catch {
      // Autoplay refused: try again on the first gesture.
      this.pendingLoops.add(id);
      this.unlockOnGesture();
      this.emit({ id, kind: "blocked", reason: "autoplay" });
      return false;
    }
    this.loops.set(
      id,
      setTimeout(() => this.stop(id), this.deps.maxLoopMs),
    );
    this.emit({ id, kind: "loop" });
    return true;
  }

  stop(id: SoundId) {
    this.pendingLoops.delete(id);
    const t = this.loops.get(id);
    if (t === undefined) return;
    clearTimeout(t);
    this.loops.delete(id);
    const el = this.els.get(id);
    if (el) {
      el.pause();
      try {
        el.currentTime = 0;
      } catch {
        /* ignore */
      }
    }
    this.emit({ id, kind: "stop" });
  }

  stopAll() {
    for (const id of [...this.loops.keys()]) this.stop(id);
    this.pendingLoops.clear();
  }

  isLooping(id: SoundId): boolean {
    return this.loops.has(id);
  }
}

/** The app-wide manager. */
export const sounds = new SoundManager();
