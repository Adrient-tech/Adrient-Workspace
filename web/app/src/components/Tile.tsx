import { Box, Typography } from "@mui/joy";
import StarRoundedIcon from "@mui/icons-material/StarRounded";
import StarBorderRoundedIcon from "@mui/icons-material/StarBorderRounded";
import { Link as RouterLink } from "react-router-dom";
import { AppIcon } from "./AppIcon";
import { externalHref, type AppTile } from "../catalog/apps";

interface TileProps {
  app: AppTile;
  /** When set, a pin/unpin star shows on hover (and always on touch). */
  pinned?: boolean;
  onTogglePin?: (id: string) => void;
}

/**
 * Tile renders one app as a phone-app-style icon: a soft-circular avatar
 * containing the app's Material Symbol icon (colored with the accent
 * color), and the app name as a label below. The whole tile is clickable.
 */
export function Tile({ app, pinned, onTogglePin }: TileProps) {
  const href = externalHref(app);

  return (
    <Box
      {...(app.comingSoon
        ? {}
        : href
          ? {
              component: "a" as const,
              href,
              target: "_blank",
              rel: "noopener noreferrer",
            }
          : {
              component: RouterLink,
              to: `/${app.id}`,
            })}
      data-testid={`tile-${app.id}`}
      onClick={app.comingSoon ? undefined : app.onLaunch}
      aria-disabled={app.comingSoon || undefined}
      // Native hover tooltip describing what the app is.
      title={app.blurb ? `${app.name} — ${app.blurb}` : app.name}
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        textAlign: "center",
        gap: 1,
        py: 1.5,
        px: 1,
        textDecoration: "none",
        color: "inherit",
        borderRadius: "md",
        transition: "background-color 120ms",
        cursor: app.comingSoon ? "default" : "pointer",
        opacity: app.comingSoon ? 0.55 : 1,
        pointerEvents: app.comingSoon ? "none" : "auto",
        position: "relative",
        "&:hover": app.comingSoon ? {} : { bgcolor: "background.level1" },
        "&:hover .adrient-pin, &:focus-within .adrient-pin": { opacity: 1 },
        "&:hover .adrient-tile-icon": app.comingSoon
          ? {}
          : { transform: "scale(1.04)" },
      }}
    >
      <Box sx={{ position: "relative" }}>
        <AppIcon
          className="adrient-tile-icon"
          iconName={app.iconName}
          color={app.accentColor}
          iconUrl={app.iconUrl}
          label={app.name}
          size={64}
        />
        {app.isNew && (
          <Box
            sx={{
              position: "absolute",
              top: -4,
              right: -4,
              px: 0.5,
              py: "1px",
              borderRadius: "sm",
              bgcolor: "#E5484D",
              color: "#fff",
              fontSize: 9,
              fontWeight: 700,
              lineHeight: 1.4,
              letterSpacing: 0.3,
              boxShadow: "xs",
              pointerEvents: "none",
            }}
          >
            NEW
          </Box>
        )}
        {app.isBeta && (
          <Box
            sx={{
              position: "absolute",
              top: -4,
              left: -4,
              px: 0.5,
              py: "1px",
              borderRadius: "sm",
              bgcolor: "#F2A21B",
              color: "#fff",
              fontSize: 9,
              fontWeight: 700,
              lineHeight: 1.4,
              letterSpacing: 0.3,
              boxShadow: "xs",
              pointerEvents: "none",
            }}
          >
            BETA
          </Box>
        )}
      </Box>
      {onTogglePin && !app.comingSoon && (
        <Box
          component="button"
          type="button"
          className="adrient-pin"
          data-testid={`pin-${app.id}`}
          aria-label={pinned ? `Unpin ${app.name}` : `Pin ${app.name}`}
          aria-pressed={!!pinned}
          onClick={(e: React.MouseEvent) => {
            e.preventDefault();
            e.stopPropagation();
            onTogglePin(app.id);
          }}
          sx={{
            position: "absolute",
            top: 4,
            right: 4,
            width: 24,
            height: 24,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            border: 0,
            borderRadius: "50%",
            cursor: "pointer",
            bgcolor: "background.surface",
            color: pinned ? "#D4880C" : "text.tertiary",
            boxShadow: "var(--joy-shadow-sm)",
            opacity: pinned ? 1 : 0,
            "@media (hover: none)": { opacity: 1 },
            "&:hover": { color: "#D4880C" },
          }}
        >
          {pinned ? <StarRoundedIcon sx={{ fontSize: 16 }} /> : <StarBorderRoundedIcon sx={{ fontSize: 16 }} />}
        </Box>
      )}
      <Typography level="body-sm" sx={{ fontWeight: 600, lineHeight: 1.2 }}>
        {app.name}
      </Typography>
      {app.comingSoon && (
        <Typography
          level="body-xs"
          sx={{ mt: -0.75, color: "#d97706", fontWeight: 500 }}
        >
          (coming soon)
        </Typography>
      )}
      {!app.comingSoon && app.subLabel && (
        <Typography
          level="body-xs"
          sx={{ mt: -0.75, opacity: 0.55, fontWeight: 500 }}
        >
          ({app.subLabel})
        </Typography>
      )}
    </Box>
  );
}
