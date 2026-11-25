import type { ReactNode } from "react";
import { Box, Tooltip } from "@mui/joy";
import { Link as RouterLink, useLocation } from "react-router-dom";
import HomeRoundedIcon from "@mui/icons-material/HomeRounded";
import SearchRoundedIcon from "@mui/icons-material/SearchRounded";
import { AppIcon } from "./AppIcon";
import { useVisibleApps } from "../catalog/useVisibleApps";
import { usePinnedApps, visiblePins } from "../catalog/pins";
import { externalHref } from "../catalog/apps";
import { CommandPaletteHost } from "../pages/home/CommandPalette";
import { openPalette } from "../pages/home/paletteStore";
import { workspaceLogoSVG } from "../brand/defaultBrand";

/** Routes that are public, full-screen or focus-mode and so get no dock rail:
 *  document editors, public share pages, the POS customer display. */
const NO_RAIL = [
  /^\/pos\/display/,
  /^\/docs\/share\//,
  /^\/(docs|sheets|slides)\/d\//,
  /^\/whiteboard\/d\//,
  /^\/(video|live)\/(watch|w)\//,
  /^\/sites\/view/,
  /^\/(sign|book|help|careers|invoice|tickets\/submit|games)(\/|$)/,
];

/** railVisible says whether the dock rail shows for a pathname. */
export function railVisible(pathname: string): boolean {
  return !NO_RAIL.some((re) => re.test(pathname));
}

function RailButton({
  label,
  to,
  active,
  onClick,
  children,
  testId,
  href,
}: {
  label: string;
  to?: string;
  href?: string;
  active?: boolean;
  onClick?: () => void;
  children: ReactNode;
  testId?: string;
}) {
  const common = {
    "aria-label": label,
    "data-testid": testId,
    "aria-current": active ? ("page" as const) : undefined,
    sx: {
      all: "unset",
      boxSizing: "border-box",
      position: "relative",
      width: 44,
      height: 44,
      borderRadius: "14px",
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
      cursor: "pointer",
      color: "text.secondary",
      transition: "background-color 140ms",
      "&:hover": { bgcolor: "background.level1" },
      "&:focus-visible": { outline: "2px solid var(--joy-palette-primary-400)", outlineOffset: 1 },
      ...(active
        ? {
            bgcolor: "background.level1",
            "&::before": {
              content: '""',
              position: "absolute",
              left: -10,
              top: 10,
              bottom: 10,
              width: 4,
              borderRadius: "0 4px 4px 0",
              bgcolor: "var(--ad-saffron)",
            },
          }
        : {}),
    },
  };
  return (
    <Tooltip title={label} placement="right" arrow>
      {to ? (
        <Box component={RouterLink} to={to} {...common}>
          {children}
        </Box>
      ) : href ? (
        <Box component="a" href={href} target="_blank" rel="noopener noreferrer" {...common}>
          {children}
        </Box>
      ) : (
        <Box component="button" type="button" onClick={onClick} {...common}>
          {children}
        </Box>
      )}
    </Tooltip>
  );
}

/** DockRail is the slim left rail: logo, search, Home, the user's pinned apps
 *  and the full app grid. Hidden on phones (the header menu replaces it). */
function DockRail() {
  const { pathname } = useLocation();
  const { apps } = useVisibleApps();
  const { pins } = usePinnedApps();
  const pinned = visiblePins(pins, apps).filter((a) => !a.comingSoon);
  const isActive = (id: string) => pathname === `/${id}` || pathname.startsWith(`/${id}/`);

  return (
    <Box
      component="nav"
      aria-label="Dock"
      data-testid="dock-rail"
      sx={{
        display: { xs: "none", md: "flex" },
        position: "fixed",
        top: 0,
        left: 0,
        bottom: 0,
        width: "var(--ad-rail-w)",
        zIndex: 1100,
        flexDirection: "column",
        alignItems: "center",
        gap: 0.75,
        py: 1.25,
        bgcolor: "background.surface",
        borderRight: "1px solid",
        borderColor: "divider",
      }}
    >
      <Box
        component={RouterLink}
        to="/"
        aria-label="Adrient home"
        data-testid="dock-logo"
        sx={{ width: 36, height: 36, display: "flex", mb: 0.75, borderRadius: "11px", flexShrink: 0 }}
        dangerouslySetInnerHTML={{ __html: workspaceLogoSVG }}
      />
      <RailButton label="Search (Ctrl K)" onClick={() => openPalette()} testId="dock-search">
        <SearchRoundedIcon />
      </RailButton>
      <RailButton label="Home" to="/" active={pathname === "/"} testId="dock-home">
        <HomeRoundedIcon />
      </RailButton>
      <Box sx={{ width: 28, height: "1px", bgcolor: "divider", my: 0.5, flexShrink: 0 }} />
      <Box sx={{ display: "flex", flexDirection: "column", gap: 0.75, overflowY: "auto", overflowX: "hidden", flex: 1, minHeight: 0, alignItems: "center", scrollbarWidth: "none", "&::-webkit-scrollbar": { display: "none" } }}>
        {pinned.map((a) => (
          <RailButton key={a.id} label={a.name} to={externalHref(a) ? undefined : `/${a.id}`} href={externalHref(a)} active={isActive(a.id)} testId={`dock-${a.id}`}>
            <AppIcon iconName={a.iconName} iconUrl={a.iconUrl} color={a.accentColor} label={a.name} size={34} />
          </RailButton>
        ))}
      </Box>
      <RailButton label="All apps" to="/apps" active={pathname === "/apps"} testId="dock-all-apps">
        <AppIcon iconName="workspace" size={34} />
      </RailButton>
    </Box>
  );
}

/** Shell wraps the routed pages: the dock rail on the left (md and up) and the
 *  Ctrl-K command bar. Every module inherits it without any module change. */
export function Shell({ enabled, children }: { enabled: boolean; children: ReactNode }) {
  const { pathname } = useLocation();
  if (!enabled) return <>{children}</>;
  const rail = railVisible(pathname);
  return (
    <>
      {rail && <DockRail />}
      <CommandPaletteHost />
      <Box sx={{ pl: { xs: 0, md: rail ? "var(--ad-rail-w)" : 0 }, minHeight: "100vh" }}>{children}</Box>
    </>
  );
}
