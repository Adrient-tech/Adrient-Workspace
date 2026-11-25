import { useEffect, useState } from "react";
import type { AppTile } from "./apps";

/** GET /api/v1/me/apps: the caller's resolved app access (policies by group,
 *  department, role and user). Only `denied` is used to filter; unknown apps
 *  stay visible so a new catalog entry is never hidden by an old response. */
export interface MyApps {
  allowed: string[];
  denied: string[];
}

let inflight: Promise<MyApps | null> | null = null;

/** fetchMyApps resolves to null on any error so callers fail open (the server
 *  still enforces access on every API route). */
export function fetchMyApps(): Promise<MyApps | null> {
  if (!inflight) {
    inflight = fetch("/api/v1/me/apps", { credentials: "same-origin" })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
      .then((d: Partial<MyApps>) => ({
        allowed: d.allowed ?? [],
        denied: d.denied ?? [],
      }))
      .catch(() => null)
      .finally(() => {
        inflight = null;
      });
  }
  return inflight;
}

/** useMyApps returns undefined while loading, null if unavailable. */
export function useMyApps(enabled = true): MyApps | null | undefined {
  const [v, setV] = useState<MyApps | null | undefined>(undefined);
  useEffect(() => {
    if (!enabled) {
      setV(null);
      return;
    }
    let alive = true;
    void fetchMyApps().then((m) => {
      if (alive) setV(m);
    });
    return () => {
      alive = false;
    };
  }, [enabled]);
  return v;
}

/** applyMyApps drops tiles the caller's access policies deny. It runs after
 *  applyServiceSettings, so the org-wide service switches stay the first filter. */
export function applyMyApps(
  tiles: readonly AppTile[],
  mine: MyApps | null | undefined,
): AppTile[] {
  if (!mine || mine.denied.length === 0) return [...tiles];
  const denied = new Set(mine.denied);
  return tiles.filter((a) => !denied.has(a.id));
}
