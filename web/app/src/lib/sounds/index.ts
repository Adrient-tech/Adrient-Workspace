import { useSyncExternalStore } from "react";
import { getPreferences, updatePreferences } from "../../pages/settings/api";
import { sounds } from "./manager";
import { PREFS_EXTRA_KEY, sanitizePrefs, type SoundPrefs } from "./prefs";
import type { SoundId } from "./catalog";

export * from "./catalog";
export * from "./prefs";
export { sounds, SoundManager } from "./manager";

const LS_KEY = "adrient.sounds";
let started = false;

function parseExtra(extra: string | null | undefined): Record<string, unknown> {
  try {
    const v = JSON.parse(extra || "{}");
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

/** Reads the sound prefs out of the preferences `extra` JSON bag. */
export function prefsFromExtra(extra: string | null | undefined): SoundPrefs | null {
  const x = parseExtra(extra);
  return PREFS_EXTRA_KEY in x ? sanitizePrefs(x[PREFS_EXTRA_KEY]) : null;
}

/** Returns `extra` with the sound prefs set, other keys kept. */
export function withSoundPrefs(extra: string | null | undefined, p: SoundPrefs): string {
  const x = parseExtra(extra);
  x[PREFS_EXTRA_KEY] = p;
  return JSON.stringify(x);
}

/**
 * Starts the sound system once per tab: loads prefs (local copy first, then
 * the server's), preloads the tones and unlocks audio on the first gesture.
 */
export function startSounds() {
  if (started) return;
  started = true;
  try {
    const raw = localStorage.getItem(LS_KEY);
    if (raw) sounds.setPrefs(JSON.parse(raw));
  } catch {
    /* corrupt local copy: defaults */
  }
  sounds.preload();
  sounds.unlockOnGesture();
  void getPreferences()
    .then((p) => {
      const s = prefsFromExtra(p.extra);
      if (s) {
        sounds.setPrefs(s);
        try {
          localStorage.setItem(LS_KEY, JSON.stringify(s));
        } catch {
          /* private mode */
        }
      }
    })
    .catch(() => {});
}

/** Applies new prefs now and stores them (this device immediately, the account in the background). */
export async function saveSoundPrefs(next: SoundPrefs): Promise<void> {
  sounds.setPrefs(next);
  const p = sounds.getPrefs();
  try {
    localStorage.setItem(LS_KEY, JSON.stringify(p));
  } catch {
    /* private mode */
  }
  const cur = await getPreferences();
  await updatePreferences({ extra: withSoundPrefs(cur.extra, p) });
}

export function useSoundPrefs(): SoundPrefs {
  return useSyncExternalStore(
    (fn) => sounds.onPrefs(fn),
    () => sounds.getPrefs(),
  );
}

/** Fire-and-forget one-shot. */
export function playSound(id: SoundId) {
  void sounds.play(id);
}

/**
 * Desktop notification for a background tab, only when the user opted in (or
 * had already granted permission and never answered the preference).
 */
export function desktopNotify(title: string, body: string, onClick?: () => void, tag?: string) {
  if (typeof Notification === "undefined" || Notification.permission !== "granted") return;
  const pref = sounds.getPrefs().desktop;
  if (pref === false) return;
  if (document.visibilityState === "visible") return;
  try {
    const n = new Notification(title, { body, icon: "/icon-192.png", tag });
    n.onclick = () => {
      window.focus();
      onClick?.();
      n.close();
    };
  } catch {
    /* some browsers only allow notifications from the service worker */
  }
}
