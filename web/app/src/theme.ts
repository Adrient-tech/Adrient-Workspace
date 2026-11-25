import { extendTheme } from "@mui/joy/styles";
import { defaultBrand } from "./brand/defaultBrand";

/**
 * Adrient design tokens and the MUI Joy theme built from them.
 *
 * Anchor: Adrient green #126C5A. Warm secondary: saffron #F2A21B. Deep ink:
 * #0E2420 (text and dark surfaces are tinted toward green, never pure grey).
 * Type pairing: Plus Jakarta Sans for display and headings, Inter for body and
 * data, both bundled locally (styles/adrient.css). See docs/design.md.
 *
 * MUI Joy derives colour shades at build time so the palette must contain
 * real colour values, not CSS custom property references.  The BrandProvider
 * still emits the CSS vars for components that use sx/style props directly.
 */

/** Raw tokens, shared with components that style through `sx` or inline SVG. */
export const tokens = {
  color: {
    green: defaultBrand.primaryColor,
    greenDeep: "#0C4A3E",
    saffron: "#F2A21B",
    ink: "#0E2420",
    paper: "#F7F9F8",
  },
  /** 4px base spacing scale; Joy's `sx` spacing unit is 8px, so 1 = 8px. */
  space: { xs: 4, sm: 8, md: 12, lg: 16, xl: 24, xxl: 32, xxxl: 48 },
  radius: { sm: 8, md: 12, lg: 16, xl: 22, pill: 999 },
  font: {
    display: '"Plus Jakarta Sans", "Inter", system-ui, -apple-system, "Segoe UI", Roboto, sans-serif',
    body: '"Inter", system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", sans-serif',
  },
  layout: { railWidth: 64, topbarHeight: 56, pageMaxWidth: 1180 },
} as const;

// Adrient green scale (500 = brand #126C5A, 900 = ink #00261F).
const adrientGreen = {
  50: "#ECF6F3",
  100: "#D2EBE4",
  200: "#A6D6C9",
  300: "#72BCA9",
  400: "#3F9A84",
  500: defaultBrand.primaryColor,
  600: "#0F5C4D",
  700: "#0C4A3E",
  800: "#08372E",
  900: "#00261F",
};

// Saffron: the warm secondary, used for highlights, warnings and accents.
const saffron = {
  50: "#FFF8E8",
  100: "#FEEFC6",
  200: "#FCDD8E",
  300: "#F9C95A",
  400: "#F5B535",
  500: "#F2A21B",
  600: "#D4880C",
  700: "#A96A09",
  800: "#7D4F0A",
  900: "#523407",
};

// Neutrals carry a faint green so surfaces sit with the brand.
const ink = {
  50: "#F6F8F7",
  100: "#EEF2F0",
  200: "#E1E8E5",
  300: "#CCD7D2",
  400: "#9BAAA4",
  500: "#6E807A",
  600: "#51635D",
  700: "#3B4A45",
  800: "#24312D",
  900: "#0E2420",
};

const { font, radius } = tokens;

export const adrientTheme = extendTheme({
  colorSchemes: {
    light: {
      palette: {
        primary: {
          ...adrientGreen,
          solidBg: "var(--joy-palette-primary-500)",
          solidHoverBg: "var(--joy-palette-primary-600)",
          solidActiveBg: "var(--joy-palette-primary-700)",
          softBg: "var(--joy-palette-primary-50)",
          softColor: "var(--joy-palette-primary-700)",
          softHoverBg: "var(--joy-palette-primary-100)",
          plainColor: "var(--joy-palette-primary-600)",
          plainHoverBg: "var(--joy-palette-primary-50)",
          outlinedColor: "var(--joy-palette-primary-600)",
          outlinedBorder: "var(--joy-palette-primary-200)",
          outlinedHoverBg: "var(--joy-palette-primary-50)",
        },
        warning: { ...saffron },
        neutral: {
          ...ink,
          outlinedBorder: "#D9E2DE",
          softBg: "#EEF2F0",
          softHoverBg: "#E3EAE7",
        },
        background: {
          body: "#F6F8F7",
          surface: "#FFFFFF",
          level1: "#F0F4F2",
          level2: "#E5ECE9",
          level3: "#D9E3DF",
        },
        divider: "rgba(14, 36, 32, 0.09)",
        focusVisible: "var(--joy-palette-primary-400)",
        text: {
          primary: "#0E2420",
          secondary: "#3F5650",
          tertiary: "#6B807A",
        },
      },
    },
    // Dark mode is its own palette (deep green ink, lifted greens), not an
    // inversion of the light one.
    dark: {
      palette: {
        primary: {
          ...adrientGreen,
          500: "#1A8F76",
          600: "#2BA68A",
          solidBg: "#178A71",
          solidHoverBg: "#1D9B80",
          solidActiveBg: "#14795F",
          softBg: "rgba(63, 154, 132, 0.16)",
          softColor: "#8FE0C7",
          softHoverBg: "rgba(63, 154, 132, 0.26)",
          plainColor: "#6FD6B8",
          plainHoverBg: "rgba(63, 154, 132, 0.16)",
          outlinedColor: "#6FD6B8",
          outlinedBorder: "rgba(111, 214, 184, 0.35)",
          outlinedHoverBg: "rgba(63, 154, 132, 0.16)",
        },
        warning: { ...saffron, solidBg: "#D4880C", softBg: "rgba(242, 162, 27, 0.16)", softColor: "#F9C95A" },
        neutral: {
          ...ink,
          outlinedBorder: "rgba(231, 240, 236, 0.14)",
          softBg: "rgba(231, 240, 236, 0.07)",
          softHoverBg: "rgba(231, 240, 236, 0.12)",
          plainColor: "#C8D6D1",
          outlinedColor: "#C8D6D1",
        },
        background: {
          body: "#0A1311",
          surface: "#111D1A",
          level1: "#162520",
          level2: "#1D2F29",
          level3: "#263B34",
        },
        divider: "rgba(231, 240, 236, 0.09)",
        focusVisible: "#6FD6B8",
        text: {
          primary: "#E8F1ED",
          secondary: "#AFC2BB",
          tertiary: "#7E948D",
        },
      },
    },
  },
  fontFamily: {
    body: font.body,
    display: font.display,
  },
  fontWeight: { sm: 500, md: 600, lg: 700, xl: 800 },
  typography: {
    h1: { fontFamily: font.display, fontWeight: 800, letterSpacing: "-0.03em" },
    h2: { fontFamily: font.display, fontWeight: 800, letterSpacing: "-0.025em" },
    h3: { fontFamily: font.display, fontWeight: 700, letterSpacing: "-0.02em" },
    h4: { fontFamily: font.display, fontWeight: 700, letterSpacing: "-0.015em" },
    "title-lg": { fontFamily: font.display, fontWeight: 700, letterSpacing: "-0.01em" },
    "title-md": { fontFamily: font.display, fontWeight: 600 },
    "title-sm": { fontFamily: font.display, fontWeight: 600 },
    "body-md": { letterSpacing: "-0.003em" },
  },
  radius: {
    xs: "6px",
    sm: `${radius.sm}px`,
    md: `${radius.md}px`,
    lg: `${radius.lg}px`,
    xl: `${radius.xl}px`,
  },
  shadow: {
    xs: "0 1px 1px rgba(14, 36, 32, 0.04)",
    sm: "0 1px 2px rgba(14, 36, 32, 0.05), 0 1px 1px rgba(14, 36, 32, 0.03)",
    md: "0 6px 16px -4px rgba(14, 36, 32, 0.08), 0 2px 4px rgba(14, 36, 32, 0.03)",
    lg: "0 18px 40px -12px rgba(14, 36, 32, 0.16), 0 4px 10px rgba(14, 36, 32, 0.04)",
    xl: "0 28px 64px -16px rgba(14, 36, 32, 0.22), 0 8px 16px rgba(14, 36, 32, 0.05)",
  },
  components: {
    JoyButton: {
      styleOverrides: {
        root: ({ ownerState }) => ({
          fontWeight: 600,
          letterSpacing: "-0.005em",
          borderRadius: ownerState.size === "sm" ? 9 : 11,
          transition: "background-color 140ms, box-shadow 140ms, transform 80ms",
          "&:active": { transform: "translateY(0.5px)" },
          ...(ownerState.variant === "solid" && ownerState.color === "primary"
            ? { boxShadow: "inset 0 1px 0 rgba(255,255,255,0.14), 0 1px 2px rgba(12,74,62,0.25)" }
            : {}),
        }),
      },
    },
    JoyIconButton: {
      styleOverrides: { root: { borderRadius: 10, transition: "background-color 140ms" } },
    },
    JoyInput: {
      styleOverrides: {
        root: { borderRadius: 11, transition: "border-color 140ms, box-shadow 140ms", "--Input-focusedThickness": "2px" },
      },
    },
    JoySelect: { styleOverrides: { root: { borderRadius: 11 } } },
    JoyTextarea: { styleOverrides: { root: { borderRadius: 11 } } },
    JoyCard: {
      styleOverrides: {
        root: {
          borderRadius: 16,
          boxShadow: "var(--joy-shadow-xs)",
          borderColor: "var(--joy-palette-divider)",
        },
      },
    },
    JoySheet: { styleOverrides: { root: { borderRadius: 0 } } },
    JoyChip: {
      styleOverrides: {
        root: { fontWeight: 600, letterSpacing: "0.005em", borderRadius: 999, "--Chip-radius": "999px" },
      },
    },
    // Segmented-control tabs: a soft track with a raised selected pill.
    JoyTabList: {
      styleOverrides: {
        root: {
          borderRadius: 12,
          padding: 3,
          gap: 2,
          backgroundColor: "var(--joy-palette-background-level1)",
          "--ListItem-radius": "9px",
        },
      },
    },
    JoyTab: {
      styleOverrides: {
        root: {
          fontWeight: 600,
          borderRadius: 9,
          "&.Mui-selected": {
            backgroundColor: "var(--joy-palette-background-surface)",
            boxShadow: "var(--joy-shadow-sm)",
            color: "var(--joy-palette-text-primary)",
          },
          "&.Mui-selected::after": { display: "none" },
        },
      },
    },
    // Quiet tables: tinted header row, hairline separators, soft row hover.
    JoyTable: {
      styleOverrides: {
        root: {
          "--TableCell-headBackground": "var(--joy-palette-background-level1)",
          "--TableRow-hoverBackground": "var(--joy-palette-background-level1)",
          "--TableCell-borderColor": "var(--joy-palette-divider)",
          "& thead th": {
            fontSize: "0.72rem",
            fontWeight: 700,
            textTransform: "uppercase",
            letterSpacing: "0.06em",
            color: "var(--joy-palette-text-tertiary)",
          },
        },
      },
    },
    JoyModalDialog: {
      styleOverrides: { root: { borderRadius: 22, boxShadow: "var(--joy-shadow-xl)" } },
    },
    JoyMenu: {
      styleOverrides: { root: { borderRadius: 14, boxShadow: "var(--joy-shadow-lg)", padding: 6 } },
    },
    JoyMenuItem: { styleOverrides: { root: { borderRadius: 9 } } },
    JoyTooltip: { styleOverrides: { root: { borderRadius: 8, fontWeight: 500 } } },
    JoyBadge: { styleOverrides: { badge: { fontWeight: 700 } } },
  },
});
