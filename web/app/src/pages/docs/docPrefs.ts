// Per-browser Docs view preferences (ribbon vs classic toolbar, ruler,
// collapsed ribbon). Stored in localStorage; a tiny store so components
// re-render via useSyncExternalStore.
import { useSyncExternalStore } from "react";

export interface DocPrefs {
  ribbon: boolean;
  collapsed: boolean;
  ruler: boolean;
  /** Last ribbon tab. */
  tab: string;
}
const KEY = "adrient.docs.prefs";
const DEFAULTS: DocPrefs = { ribbon: true, collapsed: false, ruler: true, tab: "home" };
let cache: DocPrefs | null = null;
const subs = new Set<() => void>();

export function loadPrefs(): DocPrefs {
  if (cache) return cache;
  try {
    cache = { ...DEFAULTS, ...JSON.parse(localStorage.getItem(KEY) ?? "{}") };
  } catch {
    cache = { ...DEFAULTS };
  }
  return cache!;
}
export function setPrefs(patch: Partial<DocPrefs>) {
  cache = { ...loadPrefs(), ...patch };
  try {
    localStorage.setItem(KEY, JSON.stringify(cache));
  } catch {
    /* ignore */
  }
  subs.forEach((f) => f());
}
export function usePrefs(): DocPrefs {
  return useSyncExternalStore(
    (f) => {
      subs.add(f);
      return () => subs.delete(f);
    },
    loadPrefs,
  );
}
/** Test hook. */
export function resetPrefsCache() {
  cache = null;
}
