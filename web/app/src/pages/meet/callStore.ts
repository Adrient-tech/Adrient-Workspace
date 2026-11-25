import { useCallback, useEffect, useSyncExternalStore } from "react";
import type { LocalMedia } from "./media";
import type { MeetRoom } from "./types";
import { logCallEnd } from "../../lib/calls";
import { sounds } from "../../lib/sounds";

/**
 * The active call lives OUTSIDE the Meet page so it survives navigation:
 * the user can open Chat, Mail or Docs while the call keeps running in a
 * call bar (like Teams), and come back with one click.
 */
export interface ActiveCall {
  room: MeetRoom;
  displayName: string;
  lm: LocalMedia;
  speakerId: string;
  /** Path of the call page, e.g. /meet/abc-defg-hij. */
  callPath: string;
  /** Where "Leave" returns to (the app the call was started from). */
  returnTo: string;
  startedAt: number;
  /** User undocked the call from its chat (shows the call bar instead). */
  undocked?: boolean;
}

let current: ActiveCall | null = null;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((l) => l());

const AUTO_KEY = (path: string) => `meet:rejoin:${path}`;

export const callStore = {
  get: () => current,
  subscribe(fn: () => void) {
    listeners.add(fn);
    return () => {
      listeners.delete(fn);
    };
  },
  start(call: Omit<ActiveCall, "startedAt">) {
    if (current && current.lm !== call.lm) current.lm.stop();
    current = { ...call, startedAt: Date.now() };
    // Remember the join so a page refresh rejoins straight into the call.
    try {
      sessionStorage.setItem(
        AUTO_KEY(call.callPath),
        JSON.stringify({ name: call.displayName, returnTo: call.returnTo }),
      );
    } catch {
      /* storage blocked: refresh shows the green room instead */
    }
    emit();
  },
  /** Undock: the call stays in the call bar even inside its chat. */
  setUndocked(v: boolean) {
    if (!current || !!current.undocked === v) return;
    current = { ...current, undocked: v };
    emit();
  },
  setSpeaker(id: string) {
    if (!current) return;
    current = { ...current, speakerId: id };
    emit();
  },
  end(): ActiveCall | null {
    const c = current;
    if (!c) return null;
    c.lm.stop();
    sounds.stop("ringback");
    if (c.room.code) void logCallEnd(c.room.code);
    try {
      sessionStorage.removeItem(AUTO_KEY(c.callPath));
    } catch {
      /* ignore */
    }
    current = null;
    emit();
    return c;
  },
  /** Saved rejoin info for a call path (set while a call was live). */
  rejoinInfo(path: string): { name: string; returnTo: string } | null {
    try {
      const raw = sessionStorage.getItem(AUTO_KEY(path));
      return raw ? (JSON.parse(raw) as { name: string; returnTo: string }) : null;
    } catch {
      return null;
    }
  },
};

export function useActiveCall(): ActiveCall | null {
  return useSyncExternalStore(callStore.subscribe, callStore.get, callStore.get);
}

/** Paths that must never be used as a "return to" target. */
export function safeReturn(path: string | null | undefined): string {
  if (!path || !path.startsWith("/") || path.startsWith("//")) return "";
  if (path.startsWith("/meet/")) return "";
  return path;
}

// ---- dock ------------------------------------------------------------------
// Chat registers a placeholder element for the channel whose call is live.
// CallHost keeps CallView mounted in one place (so WebRTC never restarts) and
// positions it over the dock element, which makes the call appear INSIDE the
// Chat layout like Teams.

export interface CallDock {
  el: HTMLElement;
  /** Path the dock belongs to; docks only when it equals the call's returnTo. */
  path: string;
}

let dock: CallDock | null = null;
const dockListeners = new Set<() => void>();

export const dockStore = {
  get: () => dock,
  subscribe(fn: () => void) {
    dockListeners.add(fn);
    return () => {
      dockListeners.delete(fn);
    };
  },
  set(next: CallDock | null) {
    if (dock?.el === next?.el && dock?.path === next?.path) return;
    dock = next;
    dockListeners.forEach((l) => l());
  },
};

export function useCallDock(): CallDock | null {
  return useSyncExternalStore(dockStore.subscribe, dockStore.get, dockStore.get);
}

/** Returns a ref callback that registers the element as the dock for `path`. */
export function useRegisterDock(path: string): (el: HTMLElement | null) => void {
  const ref = useCallback(
    (el: HTMLElement | null) => {
      if (el) dockStore.set({ el, path });
      else if (dockStore.get()?.path === path) dockStore.set(null);
    },
    [path],
  );
  useEffect(
    () => () => {
      if (dockStore.get()?.path === path) dockStore.set(null);
    },
    [path],
  );
  return ref;
}

/** True when the call should render inside the dock registered for its returnTo. */
export function isDocked(call: ActiveCall | null, d: CallDock | null): boolean {
  return !!call && !call.undocked && !!d && !!call.returnTo && d.path === call.returnTo;
}
