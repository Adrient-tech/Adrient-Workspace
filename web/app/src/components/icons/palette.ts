import type { AppGroup } from "../../catalog/groups";

/**
 * Icon colour families, one per app group. Each family has a light variant
 * (soft tinted tile, deep stroke) and a dark variant (deep tinted tile, light
 * stroke), plus a solid variant used for favicons and badges.
 *   tintA/tintB  light tile gradient   fg  stroke   ac  accent fill
 *   darkA/darkB  dark tile gradient    dfg dark stroke  dac dark accent
 *   solidA/solidB saturated gradient (white glyph)
 */
export interface IconFamily {
  tintA: string; tintB: string; fg: string; ac: string;
  darkA: string; darkB: string; dfg: string; dac: string;
  solidA: string; solidB: string;
}

export const ICON_FAMILIES: Record<AppGroup, IconFamily> = {
  // Blue to teal
  office: {
    tintA: "#E4EEFF", tintB: "#D9F4F0", fg: "#1E4FC4", ac: "#12B3A0",
    darkA: "#182C55", darkB: "#0F3A3C", dfg: "#9BC0FF", dac: "#34D3BF",
    solidA: "#3B7BF0", solidB: "#12B3A0",
  },
  // Violet to pink
  communicate: {
    tintA: "#EEE7FF", tintB: "#FCE4F1", fg: "#6A2FD0", ac: "#E5489A",
    darkA: "#2B1E55", darkB: "#4A1D3E", dfg: "#C8AEFF", dac: "#F472B6",
    solidA: "#8B5CF6", solidB: "#EC4899",
  },
  // Adrient green with warm amber accents
  business: {
    tintA: "#DDF2EA", tintB: "#E3F6E8", fg: "#126C5A", ac: "#F2A21B",
    darkA: "#0F3B31", darkB: "#0E3A28", dfg: "#6EE7B7", dac: "#FBBF24",
    solidA: "#126C5A", solidB: "#10A57C",
  },
  // Slate
  admin: {
    tintA: "#ECEFF4", tintB: "#DFE5EC", fg: "#33415A", ac: "#6B7C95",
    darkA: "#252D3A", darkB: "#1B222D", dfg: "#CBD5E5", dac: "#8EA0BB",
    solidA: "#5B6B85", solidB: "#334155",
  },
  // Coral to amber
  more: {
    tintA: "#FFE7E0", tintB: "#FFF0D4", fg: "#D8452C", ac: "#F2A21B",
    darkA: "#4B2118", darkB: "#4A3312", dfg: "#FFA996", dac: "#FBBF24",
    solidA: "#F4694F", solidB: "#F5A524",
  },
};
