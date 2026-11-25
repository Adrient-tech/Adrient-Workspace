import { isEmbedded } from "../lib/embed";
import { appForPath, appLabel } from "../lib/openWith";
import { usePermissions } from "../lib/permissions";
import { useState, useEffect, useMemo } from "react";
import {
  Box,
  Sheet,
  Typography,
  Avatar,
  Dropdown,
  MenuButton,
  Menu,
  MenuItem,
  IconButton,
  useColorScheme,
  Divider,
  CircularProgress,
} from "@mui/joy";
import { Link as RouterLink, useLocation, useNavigate } from "react-router-dom";
import * as Icons from "@mui/icons-material";
import { AppIcon } from "./AppIcon";
import { useBrand } from "../brand/Brand";
import type { User, Org, AccountInfo } from "../api/types";
import {
  logout,
  loginURL,
  whoami,
  listAccounts,
  activateAccount,
  removeAccount,
} from "../api/client";
import { apps, externalHref, type AppTile } from "../catalog/apps";
import {
  applyServiceSettings,
  useServiceSettings,
} from "../catalog/serviceSettings";
import { applyMyApps, useMyApps } from "../catalog/myApps";
import { NotificationBell } from "./NotificationBell";
import { SecurityDialog } from "./SecurityDialog";
import { AboutDialog } from "../brand/AboutDialog";
import { openPalette } from "../pages/home/paletteStore";
import { railVisible } from "./Shell";
import { breadcrumbsFor } from "./breadcrumbs";

interface HeaderProps {
  user: User | null;
}

/** Header is the top app-bar shown on every authenticated page.
 *  Layout: [service or brand] ........ [apps-switcher] [user menu]
 *  Inside a service (e.g. /docs, /drive) the top-left shows that service's
 *  icon + name; on the dashboard it shows the workspace brand. */
/** Reads ?return=<path>, accepting only in-app absolute paths. */
export function safeReturnParam(search: string): string | null {
  const r = new URLSearchParams(search).get("return");
  if (!r || !r.startsWith("/") || r.startsWith("//") || r.includes("\\")) return null;
  return r;
}

export function Header({ user }: HeaderProps) {
  const brand = useBrand();
  const location = useLocation();
  // The launchers (hamburger + 9-dot) resolve the catalog exactly like the
  // dashboard: org external-URL overrides applied, services the org disabled
  // hidden, and bring-your-own tiles (Home Assistant) dropped until they have
  // a URL. Fail-open for built-ins if the settings can't be fetched.
  const serviceSettings = useServiceSettings(!!user);
  const myApps = useMyApps(!!user);
  const { can } = usePermissions();
  const launcherApps = useMemo(
    () =>
      applyMyApps(applyServiceSettings(apps, serviceSettings ?? null), myApps).filter(
        (a) => a.id !== "accounting" || can("accounting:ledger:read"),
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [serviceSettings, myApps, can("accounting:ledger:read")],
  );
  const navigate = useNavigate();
  const [securityOpen, setSecurityOpen] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(false);
  const [org, setOrg] = useState<Org | null>(null);
  // Multi-account list — fetched when the dropdown opens.
  const [accounts, setAccounts] = useState<AccountInfo[] | null>(null);
  const [accountsLoading, setAccountsLoading] = useState(false);
  // Track avatar cache-buster so uploads bust the header avatar too.
  const [avatarBuster, setAvatarBuster] = useState<number | null>(null);

  useEffect(() => {
    if (!user) return;
    let alive = true;
    whoami()
      .then((r) => {
        if (alive && r.status === "ok") setOrg(r.data.org);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [user]);

  // Listen for avatar uploads from the Settings page.
  useEffect(() => {
    const handler = () => setAvatarBuster(Date.now());
    window.addEventListener("avatar-changed", handler);
    return () => window.removeEventListener("avatar-changed", handler);
  }, []);

  const initial = (user?.display_name || user?.email || "?")
    .charAt(0)
    .toUpperCase();
  // Use the PER-USER avatar URL (same as the account-switcher list, which works)
  // so each account's image gets its own browser cache key — switching profiles
  // and back no longer shows a stale/other avatar. /me/avatar is session-relative
  // and shares one cache entry across accounts, which caused the conflation.
  // The upload buster (?v=) forces a refresh after the user changes their photo.
  const avatarSrc = user
    ? `/api/v1/users/${user.id}/avatar?u=${user.id}${avatarBuster != null ? `&v=${avatarBuster}` : ""}`
    : undefined;

  /** Fetch the multi-account list when the dropdown opens. */
  function onMenuOpen() {
    if (accounts !== null) return; // already loaded
    setAccountsLoading(true);
    listAccounts()
      .then((list) => setAccounts(list))
      .catch(() => setAccounts([]))
      .finally(() => setAccountsLoading(false));
  }

  /** Switch to a different already-signed-in account (no OIDC redirect). */
  async function switchToAccount(sessionId: string) {
    try {
      await activateAccount(sessionId);
      // After switching, reload whoami to pick up the new session.
      window.location.reload();
    } catch {
      // If activation fails, fall back to a fresh login.
      window.location.href = `${loginURL()}?prompt=select_account`;
    }
  }

  /** Sign out one account from the browser's list. */
  async function signOutAccount(sessionId: string, isActive: boolean) {
    try {
      const result = await removeAccount(sessionId);
      if (result.signed_out || isActive) {
        // Active account was removed — refresh to sign-in state.
        window.location.reload();
      } else {
        // Refresh the account list.
        const updated = await listAccounts();
        setAccounts(updated);
      }
    } catch {
      window.location.reload();
    }
  }

  const current = apps.find(
    (a) =>
      !a.comingSoon &&
      (location.pathname === `/${a.id}` ||
        location.pathname.startsWith(`/${a.id}/`)),
  );

  // Embedded in an AppPane: the parent window owns the header.
  if (isEmbedded()) return null;

  const showRail = railVisible(location.pathname);
  const crumbs = breadcrumbsFor(location.pathname, current?.name);
  const backTo = safeReturnParam(location.search);
  const backApp = backTo ? appForPath(backTo) : null;

  return (
    <Sheet
      component="header"
      sx={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        gap: 1,
        px: { xs: 1.5, sm: 2.5 },
        minHeight: "var(--ad-topbar-h)",
        py: 0.75,
        position: "sticky",
        top: 0,
        zIndex: 1000,
        borderBottom: "1px solid",
        borderColor: "divider",
        bgcolor: "background.surface",
        backdropFilter: "saturate(1.4) blur(10px)",
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.25, minWidth: 0 }}>
        {user && (
          <Box sx={{ display: showRail ? { xs: "flex", md: "none" } : "flex" }}>
            <ServicesMenu currentId={current?.id} items={launcherApps} />
          </Box>
        )}
        {current ? (
          <RouterLink
            to={`/${current.id}`}
            style={{ display: "flex", alignItems: "center", gap: 10, textDecoration: "none", color: "inherit", minWidth: 0 }}
          >
            <AppIcon iconName={current.iconName} color={current.accentColor} iconUrl={current.iconUrl} label={current.name} size={32} />
            <Typography level="title-lg" noWrap sx={{ color: "text.primary", fontSize: "1.05rem" }}>
              {current.name}
            </Typography>
          </RouterLink>
        ) : (
          <RouterLink
            to="/"
            style={{ display: "flex", alignItems: "center", gap: 10, textDecoration: "none", color: "inherit", minWidth: 0 }}
          >
            {brand.logoSVG ? (
              <Box aria-label={brand.productName} sx={{ width: 32, height: 32, display: "flex", flexShrink: 0, borderRadius: "10px", overflow: "hidden" }} dangerouslySetInnerHTML={{ __html: brand.logoSVG }} />
            ) : (
              <Avatar variant="solid" sx={{ bgcolor: brand.primaryColor, color: "white", fontWeight: 700 }}>
                {brand.productName.charAt(0).toUpperCase()}
              </Avatar>
            )}
            <Typography level="title-lg" noWrap sx={{ color: "text.primary", fontSize: "1.05rem" }}>
              {brand.productName}
            </Typography>
          </RouterLink>
        )}
        {crumbs.length > 0 && (
          <Box component="nav" aria-label="Breadcrumb" data-testid="breadcrumbs" sx={{ display: { xs: "none", lg: "flex" }, alignItems: "center", gap: 0.75, color: "text.tertiary", fontSize: "0.85rem", minWidth: 0 }}>
            {crumbs.map((c, i) => (
              <Box key={c.to} sx={{ display: "flex", alignItems: "center", gap: 0.75, minWidth: 0 }}>
                <Icons.ChevronRightRounded sx={{ fontSize: 16, opacity: 0.6 }} />
                {i === crumbs.length - 1 ? (
                  <Box component="span" aria-current="page" sx={{ color: "text.secondary", fontWeight: 600, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                    {c.label}
                  </Box>
                ) : (
                  <RouterLink to={c.to} style={{ color: "inherit", textDecoration: "none", whiteSpace: "nowrap" }}>
                    {c.label}
                  </RouterLink>
                )}
              </Box>
            ))}
          </Box>
        )}
        {backTo && (
          <RouterLink
            to={backTo}
            data-testid="back-chip"
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 4,
              padding: "3px 10px",
              borderRadius: 999,
              border: "1px solid var(--joy-palette-divider)",
              fontSize: 13,
              textDecoration: "none",
              color: "inherit",
              whiteSpace: "nowrap",
            }}
          >
            <span aria-hidden>←</span>
            {backApp && backApp !== "other" ? `Back to ${appLabel(backApp)}` : "Back"}
          </RouterLink>
        )}
      </Box>

      {user && (
        <Box
          component="button"
          type="button"
          onClick={() => openPalette()}
          data-testid="topbar-search"
          aria-label="Search (Ctrl K)"
          sx={{
            all: "unset",
            boxSizing: "border-box",
            cursor: "text",
            flex: 1,
            maxWidth: 480,
            mx: { xs: 0, md: 2 },
            display: { xs: "none", sm: "flex" },
            alignItems: "center",
            gap: 1,
            px: 1.5,
            py: 0.75,
            borderRadius: "999px",
            bgcolor: "background.level1",
            border: "1px solid transparent",
            color: "text.tertiary",
            fontSize: "0.875rem",
            transition: "border-color 140ms, background-color 140ms",
            "&:hover": { borderColor: "divider", bgcolor: "background.surface" },
            "&:focus-visible": { outline: "2px solid var(--joy-palette-primary-400)", outlineOffset: 1 },
          }}
        >
          <Icons.SearchRounded sx={{ fontSize: 18 }} />
          <Box component="span" sx={{ flex: 1, textAlign: "left" }}>
            Search or ask Adrient
          </Box>
          <Box component="span" sx={{ fontSize: "0.7rem", px: 0.75, py: 0.1, border: "1px solid", borderColor: "divider", borderRadius: "6px", display: { xs: "none", lg: "block" } }}>
            Ctrl K
          </Box>
        </Box>
      )}

      {user && (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <IconButton
            variant="plain"
            color="neutral"
            aria-label="search"
            onClick={() => openPalette()}
            sx={{ display: { xs: "inline-flex", sm: "none" } }}
          >
            <Icons.SearchRounded />
          </IconButton>
          <NotificationBell />
          <AppsSwitcher items={launcherApps} />
          {org && (
            <OrgSwitcher
              org={org}
              accounts={accounts}
              loading={accountsLoading}
              onOpen={onMenuOpen}
              onSwitch={switchToAccount}
            />
          )}
          <Dropdown
            onOpenChange={(_, isOpen) => {
              if (isOpen) onMenuOpen();
            }}
          >
            <MenuButton
              slots={{ root: Avatar }}
              slotProps={{
                root: {
                  src: avatarSrc,
                  variant: "soft",
                  "aria-label": "user menu",
                } as any,
              }}
            >
              {initial}
            </MenuButton>
            <Menu placement="bottom-end" sx={{ minWidth: 260 }}>
              {/* Active account info */}
              <Box sx={{ px: 1.5, py: 1 }}>
                <Typography level="title-sm" noWrap>
                  {user.display_name || user.email}
                </Typography>
                {user.display_name && (
                  <Typography level="body-xs" sx={{ opacity: 0.7 }} noWrap>
                    {user.email}
                  </Typography>
                )}
                {org && (
                  <Typography
                    level="body-xs"
                    sx={{ mt: 0.25, opacity: 0.6 }}
                    noWrap
                    startDecorator={
                      <Icons.BusinessOutlined sx={{ fontSize: 14 }} />
                    }
                  >
                    {org.display_name || org.slug}
                  </Typography>
                )}
              </Box>

              <Divider />

              {/* Other signed-in accounts */}
              {accountsLoading && (
                <Box sx={{ display: "flex", justifyContent: "center", py: 1 }}>
                  <CircularProgress size="sm" />
                </Box>
              )}
              {!accountsLoading &&
                accounts &&
                accounts.filter((a) => !a.active).length > 0 && (
                  <>
                    <Box sx={{ px: 1.5, pt: 1, pb: 0.25 }}>
                      <Typography
                        level="body-xs"
                        sx={{
                          opacity: 0.6,
                          fontWeight: 600,
                          textTransform: "uppercase",
                          letterSpacing: "0.05em",
                        }}
                      >
                        Other accounts
                      </Typography>
                    </Box>
                    {accounts
                      .filter((a) => !a.active)
                      .map((acct) => (
                        <MenuItem
                          key={acct.session_id}
                          sx={{
                            display: "flex",
                            alignItems: "center",
                            gap: 1.5,
                            pr: 0.5,
                          }}
                        >
                          <Box
                            sx={{
                              display: "flex",
                              alignItems: "center",
                              gap: 1.5,
                              flex: 1,
                              minWidth: 0,
                              cursor: "pointer",
                            }}
                            onClick={() => switchToAccount(acct.session_id)}
                          >
                            <Avatar
                              src={acct.avatar_url}
                              size="sm"
                              sx={{ flexShrink: 0 }}
                            >
                              {(acct.display_name || acct.email || "?")
                                .charAt(0)
                                .toUpperCase()}
                            </Avatar>
                            <Box sx={{ minWidth: 0 }}>
                              <Typography
                                level="body-sm"
                                noWrap
                                fontWeight="md"
                              >
                                {acct.display_name || acct.email}
                              </Typography>
                              {acct.display_name && (
                                <Typography
                                  level="body-xs"
                                  noWrap
                                  sx={{ opacity: 0.7 }}
                                >
                                  {acct.email}
                                </Typography>
                              )}
                              {(acct.org_name || acct.org_slug) && (
                                <Typography
                                  level="body-xs"
                                  noWrap
                                  sx={{ opacity: 0.5 }}
                                >
                                  {acct.org_name || acct.org_slug}
                                </Typography>
                              )}
                            </Box>
                          </Box>
                          <IconButton
                            size="sm"
                            variant="plain"
                            color="neutral"
                            onClick={(e) => {
                              e.stopPropagation();
                              signOutAccount(acct.session_id, false);
                            }}
                            title="Sign out this account"
                            sx={{
                              opacity: 0.5,
                              "&:hover": { opacity: 1 },
                              flexShrink: 0,
                            }}
                          >
                            <Icons.CloseOutlined fontSize="small" />
                          </IconButton>
                        </MenuItem>
                      ))}
                    <Divider />
                  </>
                )}

              <ThemeModeItem />
              <MenuItem onClick={() => navigate("/settings")}>
                <Icons.SettingsOutlined sx={{ mr: 1 }} fontSize="small" />{" "}
                Settings
              </MenuItem>
              <MenuItem onClick={() => setSecurityOpen(true)}>
                <Icons.SecurityOutlined sx={{ mr: 1 }} fontSize="small" />{" "}
                Security
              </MenuItem>
              <MenuItem
                onClick={() => {
                  window.location.href = `${loginURL()}?prompt=select_account`;
                }}
                data-testid="add-account"
              >
                <Icons.PersonAddAlt1Outlined sx={{ mr: 1 }} fontSize="small" />{" "}
                Add another account
              </MenuItem>
              <MenuItem onClick={() => setAboutOpen(true)} data-testid="about-menu">
                <Icons.InfoOutlined sx={{ mr: 1 }} fontSize="small" /> About Adrient
              </MenuItem>
              <MenuItem
                onClick={() => logout().then(() => window.location.reload())}
                data-testid="sign-out"
              >
                <Icons.LogoutOutlined sx={{ mr: 1 }} fontSize="small" /> Sign
                out
              </MenuItem>
            </Menu>
          </Dropdown>
        </Box>
      )}
      <AboutDialog open={aboutOpen} onClose={() => setAboutOpen(false)} />
      {user && securityOpen && (
        <SecurityDialog user={user} onClose={() => setSecurityOpen(false)} />
      )}
    </Sheet>
  );
}

/** ThemeModeItem renders the Light/Dark/System switcher as FLAT menu items
 *  inside the avatar menu. A nested hover-submenu was unreliable here (it lives
 *  outside a real Joy submenu context), so the options are inlined; the active
 *  one is checked. Joy's useColorScheme persists the choice to localStorage. */
function ThemeModeItem() {
  const { mode, setMode } = useColorScheme();
  const opts: {
    key: "light" | "dark" | "system";
    label: string;
    icon: React.ReactNode;
  }[] = [
    {
      key: "light",
      label: "Light",
      icon: <Icons.LightModeOutlined sx={{ mr: 1 }} fontSize="small" />,
    },
    {
      key: "dark",
      label: "Dark",
      icon: <Icons.DarkModeOutlined sx={{ mr: 1 }} fontSize="small" />,
    },
    {
      key: "system",
      label: "System",
      icon: (
        <Icons.SettingsBrightnessOutlined sx={{ mr: 1 }} fontSize="small" />
      ),
    },
  ];
  return (
    <>
      <MenuItem disabled sx={{ opacity: 0.7 }}>
        <Icons.BrightnessMediumOutlined sx={{ mr: 1 }} fontSize="small" />
        <Typography level="body-xs" sx={{ fontWeight: 600 }}>
          Theme
        </Typography>
      </MenuItem>
      {opts.map((o) => (
        <MenuItem
          key={o.key}
          selected={mode === o.key}
          onClick={() => setMode(o.key)}
          data-testid={`theme-${o.key}`}
          sx={{ pl: 2.5 }}
        >
          {o.icon}
          {o.label}
          {mode === o.key && (
            <Icons.Check fontSize="small" sx={{ ml: "auto", opacity: 0.7 }} />
          )}
        </MenuItem>
      ))}
    </>
  );
}

/** ServicesMenu renders the hamburger (☰) at the far top-left, before the
 *  service/brand logo. It opens a vertical list of every service — Workspace
 *  pinned at the top — so users can switch apps from the left as well as via
 *  the right-side 9-dot AppsSwitcher. `currentId` marks the active row.
 *  External apps open in a new tab; coming-soon apps route to their teaser. */
function ServicesMenu({
  currentId,
  items,
}: {
  currentId?: string;
  items: AppTile[];
}) {
  const brand = useBrand();
  return (
    <Dropdown>
      <MenuButton
        slots={{ root: IconButton }}
        slotProps={{
          root: {
            variant: "plain",
            color: "neutral",
            "aria-label": "services menu",
            "data-testid": "services-menu",
          } as any,
        }}
      >
        <Icons.Menu />
      </MenuButton>
      <Menu
        placement="bottom-start"
        sx={{ minWidth: 240, maxHeight: "70vh", overflowY: "auto", py: 0.5 }}
      >
        <MenuItem
          component={RouterLink}
          to="/"
          selected={!currentId}
          data-testid="services-menu-workspace"
        >
          <Icons.GridView
            sx={{ mr: 1.5, color: brand.primaryColor, fontSize: 20 }}
          />
          Workspace
        </MenuItem>
        <Divider />
        {items.map((app) => (
          <ServiceMenuItem
            key={app.id}
            app={app}
            selected={app.id === currentId}
          />
        ))}
      </Menu>
    </Dropdown>
  );
}

/** ServiceMenuItem is one row in the ServicesMenu list: the app's icon (tinted
 *  with its accent color) + name, linking with the same rules as the apps-grid
 *  tiles (external → new tab, coming-soon → teaser, else the internal route). */
function ServiceMenuItem({
  app,
  selected,
}: {
  app: (typeof apps)[number];
  selected: boolean;
}) {
  const href = externalHref(app);
  const linkProps = href
    ? {
        component: "a" as const,
        href,
        target: "_blank",
        rel: "noopener noreferrer",
      }
    : {
        component: RouterLink,
        to: app.comingSoon ? `/coming-soon/${app.id}` : `/${app.id}`,
      };
  return (
    <MenuItem
      {...linkProps}
      selected={selected}
      data-testid={`services-menu-${app.id}`}
    >
      <Box sx={{ mr: 1.5, display: "flex" }}>
        <AppIcon iconName={app.iconName} color={app.accentColor} iconUrl={app.iconUrl} label={app.name} size={24} />
      </Box>
      {app.name}
    </MenuItem>
  );
}

/** AppsSwitcher renders the 9-dot grid icon. On click, opens a popover with
 *  a mini grid of every app from the catalog, each linking to its route. */
function AppsSwitcher({ items }: { items: AppTile[] }) {
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      const t = e.target as HTMLElement | null;
      if (
        t &&
        (t.closest("[data-apps-switcher]") || t.closest('[aria-label="apps"]'))
      )
        return;
      setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown, true);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown, true);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <Dropdown open={open} onOpenChange={(_, isOpen) => setOpen(isOpen)}>
      <MenuButton
        slots={{ root: IconButton }}
        slotProps={{
          root: {
            variant: "plain",
            color: "neutral",
            "aria-label": "apps",
          } as any,
        }}
      >
        <Icons.Apps />
      </MenuButton>
      <Menu
        placement="bottom-end"
        sx={{ p: 1.5, minWidth: 360 }}
        slotProps={{ root: { "data-apps-switcher": true } as any }}
      >
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: "repeat(4, minmax(80px, 1fr))",
            gap: 0.5,
          }}
        >
          <WorkspaceTile onNavigate={close} />
          {items.map((app) => (
            <MiniTile key={app.id} app={app} onNavigate={close} />
          ))}
        </Box>
      </Menu>
    </Dropdown>
  );
}

/** WorkspaceTile is the dashboard/home entry inside the apps-switcher grid. */
function WorkspaceTile({ onNavigate }: { onNavigate?: () => void }) {
  const brand = useBrand();
  return (
    <Box
      component={RouterLink}
      to="/"
      onClick={onNavigate}
      data-testid="switcher-workspace"
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        textAlign: "center",
        gap: 0.5,
        py: 1.25,
        px: 0.5,
        textDecoration: "none",
        color: "inherit",
        borderRadius: "sm",
        transition: "background-color 120ms",
        "&:hover": { bgcolor: "background.level1" },
      }}
    >
      <AppIcon iconName="GridView" color={brand.primaryColor} label="Workspace" size={42} />
      <Typography level="body-xs" sx={{ fontWeight: 500, lineHeight: 1.2 }}>
        Workspace
      </Typography>
    </Box>
  );
}

interface MiniTileProps {
  app: (typeof apps)[number];
  onNavigate?: () => void;
}

function MiniTile({ app, onNavigate }: MiniTileProps) {
  const href = externalHref(app);
  const linkProps = href
    ? {
        component: "a" as const,
        href,
        target: "_blank",
        rel: "noopener noreferrer",
      }
    : {
        component: RouterLink,
        to: app.comingSoon ? `/coming-soon/${app.id}` : `/${app.id}`,
      };
  return (
    <Box
      {...linkProps}
      onClick={onNavigate}
      data-testid={`switcher-${app.id}`}
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        textAlign: "center",
        gap: 0.5,
        py: 1.25,
        px: 0.5,
        textDecoration: "none",
        color: "inherit",
        borderRadius: "sm",
        transition: "background-color 120ms",
        "&:hover": { bgcolor: "background.level1" },
      }}
    >
      <AppIcon iconName={app.iconName} color={app.accentColor} iconUrl={app.iconUrl} label={app.name} size={42} />
      <Typography level="body-xs" sx={{ fontWeight: 500, lineHeight: 1.2 }}>
        {app.name}
      </Typography>
    </Box>
  );
}


/** OrgSwitcher shows the current organisation and, when the browser has other
 *  signed-in accounts (possibly in other orgs), lets the user jump to one. */
function OrgSwitcher({
  org,
  accounts,
  loading,
  onOpen,
  onSwitch,
}: {
  org: Org;
  accounts: AccountInfo[] | null;
  loading: boolean;
  onOpen: () => void;
  onSwitch: (sessionId: string) => void;
}) {
  const others = (accounts ?? []).filter((a) => !a.active);
  return (
    <Dropdown onOpenChange={(_, isOpen) => isOpen && onOpen()}>
      <MenuButton
        data-testid="org-switcher"
        aria-label="organisation"
        slots={{ root: Box }}
        slotProps={{
          root: {
            component: "button",
            type: "button",
            sx: {
              all: "unset",
              boxSizing: "border-box",
              cursor: "pointer",
              display: { xs: "none", md: "flex" },
              alignItems: "center",
              gap: 0.75,
              maxWidth: 180,
              px: 1.25,
              py: 0.6,
              borderRadius: "999px",
              border: "1px solid",
              borderColor: "divider",
              fontSize: "0.8rem",
              fontWeight: 600,
              color: "text.secondary",
              "&:hover": { bgcolor: "background.level1" },
              "&:focus-visible": { outline: "2px solid var(--joy-palette-primary-400)", outlineOffset: 1 },
            },
          } as any,
        }}
      >
        <Icons.BusinessOutlined sx={{ fontSize: 16 }} />
        <Box component="span" sx={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {org.display_name || org.slug}
        </Box>
        <Icons.UnfoldMoreRounded sx={{ fontSize: 16, opacity: 0.6 }} />
      </MenuButton>
      <Menu placement="bottom-end" sx={{ minWidth: 240 }}>
        <Box sx={{ px: 1.5, py: 0.75 }}>
          <Typography level="body-xs" sx={{ fontWeight: 700, letterSpacing: "0.06em", textTransform: "uppercase", color: "text.tertiary" }}>
            Organisation
          </Typography>
          <Typography level="title-sm" startDecorator={<Icons.CheckRounded sx={{ fontSize: 16 }} color="primary" />}>
            {org.display_name || org.slug}
          </Typography>
        </Box>
        {loading && (
          <Box sx={{ display: "flex", justifyContent: "center", py: 1 }}>
            <CircularProgress size="sm" />
          </Box>
        )}
        {others.length > 0 && <Divider />}
        {others.map((a) => (
          <MenuItem key={a.session_id} onClick={() => onSwitch(a.session_id)}>
            <Box sx={{ minWidth: 0 }}>
              <Typography level="body-sm" noWrap sx={{ fontWeight: 600 }}>
                {a.org_name || a.org_slug || a.email}
              </Typography>
              <Typography level="body-xs" noWrap sx={{ color: "text.tertiary" }}>
                {a.display_name || a.email}
              </Typography>
            </Box>
          </MenuItem>
        ))}
      </Menu>
    </Dropdown>
  );
}
