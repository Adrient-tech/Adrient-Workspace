import { Box } from "@mui/joy";
import * as Icons from "@mui/icons-material";
import { GLYPHS } from "./icons/glyphs";
import { ICON_FAMILIES } from "./icons/palette";
import { groupOf, type AppGroup } from "../catalog/groups";

type IconMap = Record<string, React.ComponentType<{ sx?: object }>>;

/** resolveAppIcon prefers the Rounded variant of a Material icon and falls
 *  back to the plain icon name. Only used for icons with no Adrient glyph
 *  (third-party tiles, legacy callers). */
export function resolveAppIcon(iconName: string) {
  const map = Icons as unknown as IconMap;
  const base = iconName.replace(/(Rounded|Outlined|Sharp|TwoTone)$/, "");
  return map[`${base}Rounded`] ?? map[iconName];
}

/** Legacy Material icon names used outside the catalog (search result types,
 *  the workspace tile) mapped to the matching Adrient glyph. */
const LEGACY_GLYPH: Record<string, string> = {
  Folder: "drive",
  Description: "docs",
  TableChart: "sheets",
  Slideshow: "slides",
  CoPresent: "slides",
  Contacts: "contacts",
  Lightbulb: "keep",
  CalendarToday: "calendar",
  Mail: "mail",
  GridView: "workspace",
  Apps: "workspace",
  AutoAwesome: "ask",
};

/** glyphFor resolves an iconName (a glyph key or a legacy Material name) to a
 *  glyph key, or undefined when there is no Adrient drawing for it. */
export function glyphFor(iconName: string): string | undefined {
  if (GLYPHS[iconName]) return iconName;
  const legacy = LEGACY_GLYPH[iconName];
  return legacy && GLYPHS[legacy] ? legacy : undefined;
}

interface AppIconProps {
  /** Glyph key (an app id such as "drive") or a legacy Material icon name. */
  iconName: string;
  /** Accent colour of the tile; only used by third-party icons without a glyph. */
  color?: string;
  /** Outer size in px. */
  size?: number;
  iconUrl?: string;
  label?: string;
  className?: string;
  /** Override the colour family (defaults to the app's group). */
  group?: AppGroup;
  /** "soft" is the in-app tile; "solid" is the saturated badge used for
   *  favicons, splash and share cards. */
  variant?: "soft" | "solid";
}

/**
 * AppIcon draws one app's icon: a soft rounded square tinted with its group's
 * gradient and an Adrient glyph (see docs/design.md "Icons"). The same glyph
 * renders in light, dark and solid variants through CSS variables.
 */
export function AppIcon({
  iconName,
  color,
  size = 64,
  iconUrl,
  label,
  className,
  group,
  variant = "soft",
}: AppIconProps) {
  const glyph = iconUrl ? undefined : glyphFor(iconName);
  const fam = ICON_FAMILIES[group ?? groupOf(glyph ?? iconName)];
  const radius = Math.round(size * 0.3);
  const solid = variant === "solid";
  const Legacy = !iconUrl && !glyph ? resolveAppIcon(iconName) : undefined;

  const vars: Record<string, string> = {
    "--ai-a": fam.tintA,
    "--ai-b": fam.tintB,
    "--ai-fg": fam.fg,
    "--ai-ac": fam.ac,
    "--ai-da": fam.darkA,
    "--ai-db": fam.darkB,
    "--ai-dfg": fam.dfg,
    "--ai-dac": fam.dac,
  };

  return (
    <Box
      className={className}
      aria-hidden
      data-glyph={glyph}
      sx={{
        ...vars,
        width: size,
        height: size,
        flexShrink: 0,
        borderRadius: `${radius}px`,
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        overflow: "hidden",
        position: "relative",
        background: iconUrl
          ? "transparent"
          : solid
            ? `linear-gradient(145deg, ${fam.solidA}, ${fam.solidB})`
            : glyph
              ? "linear-gradient(145deg, var(--ai-a), var(--ai-b))"
              : `linear-gradient(145deg, ${color ?? fam.solidA}22, ${color ?? fam.solidA}38)`,
        boxShadow: iconUrl || solid ? "none" : "inset 0 0 0 1px rgba(15, 40, 35, 0.06)",
        "[data-joy-color-scheme='dark'] &": {
          background: iconUrl
            ? "transparent"
            : solid
              ? `linear-gradient(145deg, ${fam.solidA}, ${fam.solidB})`
              : glyph
                ? "linear-gradient(145deg, var(--ai-da), var(--ai-db))"
                : `linear-gradient(145deg, ${color ?? fam.solidA}44, ${color ?? fam.solidA}22)`,
          boxShadow: iconUrl || solid ? "none" : "inset 0 0 0 1px rgba(255, 255, 255, 0.07)",
        },
        transition: "transform 150ms ease",
        "& svg.ad-glyph": {
          width: "62%",
          height: "62%",
          fill: "none",
          stroke: solid ? "#fff" : "var(--ai-fg)",
          strokeWidth: 1.7,
          strokeLinecap: "round",
          strokeLinejoin: "round",
          "& .a": { fill: solid ? "rgba(255,255,255,.92)" : "var(--ai-ac)", stroke: "none" },
          "& .s": {
            fill: solid ? "rgba(255,255,255,.3)" : "var(--ai-ac)",
            fillOpacity: solid ? 1 : 0.28,
            stroke: "none",
          },
          "& .as": { stroke: solid ? "rgba(255,255,255,.92)" : "var(--ai-ac)" },
          "& .w": { stroke: solid ? "var(--ai-fg)" : "#fff" },
        },
        "[data-joy-color-scheme='dark'] & svg.ad-glyph": {
          stroke: solid ? "#fff" : "var(--ai-dfg)",
          "& .a": { fill: solid ? "rgba(255,255,255,.92)" : "var(--ai-dac)" },
          "& .s": { fill: solid ? "rgba(255,255,255,.3)" : "var(--ai-dac)" },
          "& .as": { stroke: solid ? "rgba(255,255,255,.92)" : "var(--ai-dac)" },
          "& .w": { stroke: "#1b1b1b" },
        },
      }}
    >
      {iconUrl ? (
        <Box component="img" src={iconUrl} alt="" sx={{ width: size, height: size, objectFit: "cover" }} />
      ) : glyph ? (
        <svg className="ad-glyph" viewBox="0 0 24 24" focusable="false">
          {GLYPHS[glyph]}
        </svg>
      ) : Legacy ? (
        <Legacy sx={{ fontSize: Math.round(size * 0.52), color: color ?? fam.fg }} />
      ) : (
        <Box component="span" sx={{ fontWeight: 700, fontSize: Math.round(size * 0.42), color: color ?? fam.fg }}>
          {(label ?? "?").charAt(0).toUpperCase()}
        </Box>
      )}
    </Box>
  );
}
