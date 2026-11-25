import { Box, Typography } from "@mui/joy";
import { useLocation } from "react-router-dom";
import { apps } from "../catalog/apps";
import { AppIcon } from "./AppIcon";

/** Maps a pathname like "/drive/folder/1" to its catalog entry (by first segment). */
export function appForPath(pathname: string) {
  const seg = pathname.split("/").filter(Boolean)[0];
  return seg ? apps.find((a) => a.id === seg) : undefined;
}

const KEYFRAMES = `
@keyframes adrient-splash-bar{0%{left:-40%;width:40%}50%{left:30%;width:50%}100%{left:100%;width:40%}}
@keyframes adrient-splash-in{from{opacity:0}to{opacity:1}}
@media (prefers-reduced-motion: reduce){
  .adrient-splash-bar>i{animation:none!important;left:0!important;width:100%!important;opacity:.6}
  .adrient-splash{animation:none!important}
}`;

/**
 * Full-screen loading splash (Google-style): the app's own icon, its name and
 * a thin indeterminate green bar. The icon follows the current route; the
 * dashboard and unknown routes show the Adrient mark.
 */
export function AppSplash() {
  const { pathname } = useLocation();
  const app = appForPath(pathname);
  return (
    <Box
      className="adrient-splash"
      role="status"
      aria-label="Loading"
      data-testid="app-splash"
      sx={{
        position: "fixed",
        inset: 0,
        zIndex: 1300,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 2,
        bgcolor: "background.body",
        animation: "adrient-splash-in 120ms ease-out",
      }}
    >
      <style>{KEYFRAMES}</style>
      {app ? (
        <AppIcon
          iconName={app.iconName}
          color={app.accentColor}
          iconUrl={app.iconUrl}
          label={app.name}
          size={72}
        />
      ) : (
        <img src="/adrient-mark.png" alt="Adrient" width={72} height={72} style={{ padding: 8, boxSizing: "border-box", background: "#fff", borderRadius: 18, boxShadow: "0 0 0 1px rgba(18,108,90,.14)" }} />
      )}
      <Typography level="title-lg" sx={{ color: "text.secondary" }}>
        {app ? `Adrient ${app.name}` : "Adrient Workspace"}
      </Typography>
      <Box
        className="adrient-splash-bar"
        sx={{
          position: "relative",
          width: 160,
          height: 3,
          borderRadius: 2,
          overflow: "hidden",
          bgcolor: "rgba(18,108,90,0.15)",
          "& > i": {
            position: "absolute",
            top: 0,
            bottom: 0,
            bgcolor: "#126C5A",
            borderRadius: 2,
            animation: "adrient-splash-bar 1.3s ease-in-out infinite",
          },
        }}
      >
        <i />
      </Box>
    </Box>
  );
}
