// Per-user pinned apps (favourites) for the dock and the Today home.
//
// Stored in the server-side user preferences (`/api/v1/me/preferences`, the
// free-form `extra` JSON bag, key `pinned_apps`) so the dock follows the user
// across devices, with a localStorage copy for an instant first paint. A user
// who never pinned anything gets DEFAULT_PINS; an explicit empty list is kept
// (they unpinned everything on purpose).
import { useCallback, useEffect, useState } from "react";
import { getPreferences, updatePreferences } from "../pages/settings/api";
import { parseExtra } from "../lib/shortcutScheme";

export const PINS_PREF_KEY = "pinned_apps";
export const PINS_STORAGE_KEY = "adrient.pinnedApps";
export const MAX_PINS = 12;

/** Shown in the dock until the user chooses their own. */
export const DEFAULT_PINS = ["drive", "mail", "calendar", "docs", "sheets", "chat", "meet", "tasks"];

/** normalizePins keeps only unique non-empty string ids, capped at MAX_PINS. */
export function normalizePins(v: unknown): string[] {
  if (!Array.isArray(v)) return [];
  const out: string[] = [];
  for (const x of v) {
    if (typeof x === "string" && x && !out.includes(x)) out.push(x);
    if (out.length >= MAX_PINS) break;
  }
  return out;
}

/** pinsFromExtra reads the pins from the preferences `extra` bag; undefined
 *  when the user never saved a choice. */
export function pinsFromExtra(extra: string | null | undefined): string[] | undefined {
  const x = parseExtra(extra);
  return PINS_PREF_KEY in x ? normalizePins(x[PINS_PREF_KEY]) : undefined;
}

/** withPins returns `extra` with the pins set, other keys kept. */
export function withPins(extra: string | null | undefined, pins: string[]): string {
  const x = parseExtra(extra);
  x[PINS_PREF_KEY] = normalizePins(pins);
  return JSON.stringify(x);
}

/** togglePin pins or unpins an app. Pinning past MAX_PINS drops the oldest. */
export function togglePin(pins: string[], id: string): string[] {
  if (pins.includes(id)) return pins.filter((p) => p !== id);
  const next = [...pins, id];
  return next.length > MAX_PINS ? next.slice(next.length - MAX_PINS) : next;
}

/** visiblePins drops pins for apps the user cannot see (disabled, denied). */
export function visiblePins<T extends { id: string }>(pins: string[], visible: T[]): T[] {
  const byId = new Map(visible.map((a) => [a.id, a]));
  return pins.map((id) => byId.get(id)).filter((a): a is T => !!a);
}

function readLocal(): string[] | undefined {
  try {
    const raw = localStorage.getItem(PINS_STORAGE_KEY);
    return raw ? normalizePins(JSON.parse(raw)) : undefined;
  } catch {
    return undefined;
  }
}

function writeLocal(pins: string[]) {
  try {
    localStorage.setItem(PINS_STORAGE_KEY, JSON.stringify(pins));
  } catch {
    /* private mode: the server copy still holds it */
  }
}

// One shared store so the dock and the home screen stay in sync.
let current: string[] | undefined = readLocal();
let loaded = false;
const listeners = new Set<(p: string[]) => void>();

function publish(p: string[]) {
  current = p;
  writeLocal(p);
  listeners.forEach((l) => l(p));
}

/** usePinnedApps returns the user's pinned ids (defaults until they choose)
 *  and a toggle that persists to preferences. */
export function usePinnedApps(enabled = true): {
  pins: string[];
  toggle: (id: string) => void;
  isPinned: (id: string) => boolean;
} {
  const [pins, setPins] = useState<string[]>(current ?? DEFAULT_PINS);

  useEffect(() => {
    const l = (p: string[]) => setPins(p);
    listeners.add(l);
    return () => {
      listeners.delete(l);
    };
  }, []);

  useEffect(() => {
    if (!enabled || loaded) return;
    loaded = true;
    getPreferences()
      .then((prefs) => {
        const server = pinsFromExtra(prefs.extra);
        if (server) publish(server);
      })
      .catch(() => {
        loaded = false;
      });
  }, [enabled]);

  const toggle = useCallback((id: string) => {
    const next = togglePin(current ?? DEFAULT_PINS, id);
    publish(next);
    getPreferences()
      .then((prefs) => updatePreferences({ extra: withPins(prefs.extra, next), update_mask: ["extra"] }))
      .catch(() => {
        /* offline: the local copy keeps the choice */
      });
  }, []);

  const isPinned = useCallback((id: string) => pins.includes(id), [pins]);
  return { pins, toggle, isPinned };
}
