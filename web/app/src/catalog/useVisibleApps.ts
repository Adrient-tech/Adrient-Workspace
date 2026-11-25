import { useEffect, useMemo, useState } from "react";
import { apps, type AppTile } from "./apps";
import { applyServiceSettings, useServiceSettings } from "./serviceSettings";
import { applyMyApps, useMyApps } from "./myApps";
import { usePermissions } from "../lib/permissions";
import { adminWhoAmI } from "../pages/admin/usersApi";

export interface AdminInfo {
  /** Team-org member (the Admin tile is shown). */
  showAdmin: boolean;
  /** Admin with user management wired (the "Add user" button). */
  canAddUser: boolean;
}

let adminInflight: Promise<AdminInfo> | null = null;

function loadAdminInfo(): Promise<AdminInfo> {
  if (!adminInflight) {
    adminInflight = adminWhoAmI()
      .then((w) => ({ showAdmin: !w.isPersonal, canAddUser: w.isAdmin && w.userMgmtEnabled && !w.isPersonal }))
      .catch(() => {
        adminInflight = null;
        return { showAdmin: false, canAddUser: false };
      });
  }
  return adminInflight;
}

/**
 * useVisibleApps resolves the catalog for the signed-in user exactly like the
 * old dashboard did: org service switches (disabled services hidden, external
 * URL overrides), per-user app policies, RBAC (accounting) and the team-org
 * Admin tile. Fails open for built-in apps; the server enforces access anyway.
 */
export function useVisibleApps(enabled = true): { apps: AppTile[]; admin: AdminInfo } {
  const settings = useServiceSettings(enabled);
  const myApps = useMyApps(enabled);
  const { can } = usePermissions();
  const [admin, setAdmin] = useState<AdminInfo>({ showAdmin: false, canAddUser: false });

  useEffect(() => {
    if (!enabled) return;
    let alive = true;
    void loadAdminInfo().then((a) => {
      if (alive) setAdmin(a);
    });
    return () => {
      alive = false;
    };
  }, [enabled]);

  const ledger = can("accounting:ledger:read");
  const visible = useMemo(
    () =>
      applyMyApps(applyServiceSettings(apps, settings ?? null), myApps).filter((a) => {
        if (a.id === "accounting" && !ledger) return false;
        if (a.id === "admin" && !admin.showAdmin) return false;
        return true;
      }),
    [settings, myApps, ledger, admin.showAdmin],
  );
  return { apps: visible, admin };
}
