import { useEffect, useState } from "react";

/** Response of GET /api/v1/me/permissions. */
export interface MyPermissions {
  roles: string[];
  permissions: string[];
  admin: boolean;
}

/** Mirror of the Go authz.Match: "*" grants all but org:owner, "app:*" a prefix. */
export function matchPermission(pattern: string, perm: string): boolean {
  if (!pattern || !perm) return false;
  if (pattern === perm) return true;
  if (pattern === "*") return perm !== "org:owner";
  if (pattern.endsWith(":*")) return perm.startsWith(pattern.slice(0, -1));
  return false;
}

export function hasPermission(p: MyPermissions | null, perm: string): boolean {
  // Fail open while unknown: the server enforces every permission regardless.
  if (!p) return true;
  return p.permissions.some((pat) => matchPermission(pat, perm));
}

let cached: Promise<MyPermissions | null> | null = null;

export function loadMyPermissions(force = false): Promise<MyPermissions | null> {
  if (!cached || force) {
    cached = fetch("/api/v1/me/permissions", { credentials: "include", headers: { Accept: "application/json" } })
      .then((r) => (r.ok ? (r.json() as Promise<MyPermissions>) : null))
      .catch(() => null);
  }
  return cached;
}

/** usePermissions returns can(perm); true until the permissions have loaded. */
export function usePermissions(): { can: (perm: string) => boolean; loaded: boolean; roles: string[] } {
  const [p, setP] = useState<MyPermissions | null>(null);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    let alive = true;
    loadMyPermissions().then((r) => {
      if (alive) {
        setP(r);
        setLoaded(true);
      }
    });
    return () => {
      alive = false;
    };
  }, []);
  return { can: (perm: string) => hasPermission(p, perm), loaded, roles: p?.roles ?? [] };
}
