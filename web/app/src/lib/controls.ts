import { useEffect, useState } from "react";

/** Response of GET /api/v1/me/controls: effective per-app controls of the caller. */
export interface MyControls {
  controls: Record<string, boolean | string | number>;
  user_type: string;
}

let cached: Promise<MyControls | null> | null = null;

export function loadMyControls(force = false): Promise<MyControls | null> {
  if (!cached || force) {
    cached = fetch("/api/v1/me/controls", {
      credentials: "include",
      headers: { Accept: "application/json" },
    })
      .then((r) => (r.ok ? (r.json() as Promise<MyControls>) : null))
      .catch(() => null);
  }
  return cached;
}

/**
 * useControls returns typed readers for the caller's controls. The server
 * enforces every control; this only decides which actions the UI shows, so it
 * answers the permissive default while the controls are loading.
 */
export function useControls(): {
  loaded: boolean;
  userType: string;
  bool: (key: string, fallback?: boolean) => boolean;
  str: (key: string, fallback?: string) => string;
} {
  const [c, setC] = useState<MyControls | null>(null);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    let alive = true;
    void loadMyControls().then((v) => {
      if (alive) {
        setC(v);
        setLoaded(true);
      }
    });
    return () => {
      alive = false;
    };
  }, []);
  return {
    loaded,
    userType: c?.user_type ?? "member",
    bool: (key, fallback = true) => {
      const v = c?.controls[key];
      return typeof v === "boolean" ? v : fallback;
    },
    str: (key, fallback = "") => {
      const v = c?.controls[key];
      return typeof v === "string" ? v : fallback;
    },
  };
}
