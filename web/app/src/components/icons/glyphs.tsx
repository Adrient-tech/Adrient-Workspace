/**
 * Adrient icon set: original geometric glyphs drawn on a 24x24 grid.
 *
 * Rules (see docs/design.md):
 *  - 1.7px round-capped stroke for the primary shape, one accent shape per
 *    glyph filled (class "a") or stroked (class "as") with the group's accent
 *    colour. Optical size: the live area is 3.5..20.5, 17px square.
 *  - No external artwork or icon font; every path here is Adrient's own.
 *  - Colour comes from CSS variables set by the group (see ./groups.ts), so
 *    one glyph renders in light, dark and solid (favicon) variants.
 */
import type { ReactElement } from "react";

export type GlyphName = string;

const g = (el: ReactElement) => el;

/** Glyph bodies keyed by glyph name (an app id, or a UI glyph such as "workspace"). */
export const GLYPHS: Record<GlyphName, ReactElement> = {
  // ---- Office ----
  drive: g(
    <>
      <path d="M3.5 7.5a2 2 0 0 1 2-2h4l2 2.2h7a2 2 0 0 1 2 2v7.8a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z" />
      <rect className="a" x="7.5" y="13" width="9" height="2.2" rx="1.1" />
    </>,
  ),
  docs: g(
    <>
      <path d="M6.5 3.5h7.5l3.5 3.5v12.5a1 1 0 0 1-1 1h-10a1 1 0 0 1-1-1v-15a1 1 0 0 1 1-1z" />
      <path d="M9 10.5h6M9 13.5h6" />
      <rect className="a" x="9" y="16" width="4" height="1.8" rx=".9" />
    </>,
  ),
  sheets: g(
    <>
      <path className="s" d="M4 7a3 3 0 0 1 3-3h10a3 3 0 0 1 3 3v3H4z" />
      <rect x="4" y="4" width="16" height="16" rx="3" />
      <path d="M4 10h16M10 10v10" />
    </>,
  ),
  slides: g(
    <>
      <rect x="3.5" y="5" width="17" height="11" rx="2" />
      <path d="M12 16v3.5M8.5 19.5h7" />
      <circle className="a" cx="9" cy="10.5" r="1.8" />
      <path d="M13 9.5h4.5M13 12h3" />
    </>,
  ),
  pdf: g(
    <>
      <path d="M6.5 3.5h7.5l3.5 3.5v12.5a1 1 0 0 1-1 1h-10a1 1 0 0 1-1-1v-15a1 1 0 0 1 1-1z" />
      <rect className="a" x="5.5" y="12.5" width="13" height="5" rx="1.2" />
      <path d="M8.2 15h1.4M11.3 15h1.4M14.4 15h1.4" stroke="#fff" strokeWidth="1.3" className="w" />
    </>,
  ),
  forms: g(
    <>
      <rect x="5.5" y="4.5" width="13" height="16" rx="2.5" />
      <rect className="a" x="9" y="2.8" width="6" height="3.4" rx="1.4" />
      <path d="M9 11l1.2 1.2 2-2.2M14 11.5h2M9 16h7" />
    </>,
  ),
  keep: g(
    <>
      <path d="M12 3.5a5.5 5.5 0 0 0-3.2 10c.5.4.7.9.7 1.5v.5h5V15c0-.6.2-1.1.7-1.5A5.5 5.5 0 0 0 12 3.5z" />
      <path d="M10 18.5h4M10.8 20.5h2.4" />
      <circle className="a" cx="12" cy="9" r="1.8" />
    </>,
  ),
  whiteboard: g(
    <>
      <rect x="3.5" y="4.5" width="17" height="11" rx="2" />
      <path d="M8 20l2-4.5M16 20l-2-4.5M7 11c1.4-2.8 2.8 2.6 4.4 0s2.6-1.6 4.1 0" />
      <circle className="a" cx="17" cy="8" r="1.3" />
    </>,
  ),
  wiki: g(
    <>
      <path d="M12 6.5c-1.8-1.3-4.2-1.8-7-1.5v12c2.8-.3 5.2.2 7 1.5 1.8-1.3 4.2-1.8 7-1.5V5c-2.8-.3-5.2.2-7 1.5zM12 6.5v12" />
      <rect className="a" x="14.6" y="5.6" width="2.2" height="4.6" rx=".8" />
    </>,
  ),
  tasks: g(
    <>
      <circle className="s" cx="12" cy="12" r="5.2" />
      <circle cx="12" cy="12" r="8.5" />
      <path d="M8.6 12.2l2.4 2.4 4.5-5" />
    </>,
  ),

  // ---- Communicate ----
  mail: g(
    <>
      <path className="s" d="M4.5 8l7.5 5.5L19.5 8z" />
      <rect x="3.5" y="5.5" width="17" height="13" rx="3" />
      <path d="M4.5 8l7.5 5.5L19.5 8" />
    </>,
  ),
  chat: g(
    <>
      <path d="M5 5.5h14A1.5 1.5 0 0 1 20.5 7v8a1.5 1.5 0 0 1-1.5 1.5h-7l-4 3.5v-3.5H5A1.5 1.5 0 0 1 3.5 15V7A1.5 1.5 0 0 1 5 5.5z" />
      <circle className="a" cx="8.5" cy="11" r="1.1" />
      <circle className="a" cx="12" cy="11" r="1.1" />
      <circle className="a" cx="15.5" cy="11" r="1.1" />
    </>,
  ),
  meet: g(
    <>
      <rect x="3.5" y="6.5" width="11" height="11" rx="3" />
      <path d="M14.5 10.5l5-2.8v8.6l-5-2.8" />
      <circle className="a" cx="9" cy="12" r="1.8" />
    </>,
  ),
  calendar: g(
    <>
      <rect x="4" y="5.5" width="16" height="14.5" rx="3" />
      <path d="M4 10h16M8.5 3.5v4M15.5 3.5v4" />
      <rect className="a" x="8" y="13" width="3.4" height="3.4" rx="1" />
    </>,
  ),
  contacts: g(
    <>
      <circle className="s" cx="12" cy="9" r="3.2" />
      <circle cx="12" cy="9" r="3.2" />
      <path d="M5.5 19.5c.8-3.3 3.4-5 6.5-5s5.7 1.7 6.5 5" />
    </>,
  ),
  telephony: g(
    <>
      <path d="M6.5 4.5h3l1.5 4-2 1.3a10 10 0 0 0 4.7 4.7l1.3-2 4 1.5v3a2 2 0 0 1-2 2A13.5 13.5 0 0 1 4.5 6.5a2 2 0 0 1 2-2z" />
      <path className="as" d="M14.5 4.8a4.7 4.7 0 0 1 4.7 4.7" />
    </>,
  ),
  groups: g(
    <>
      <circle cx="9" cy="9" r="2.8" />
      <circle className="a" cx="16.5" cy="10" r="2.2" />
      <path d="M3.8 19c.5-3 2.6-4.6 5.2-4.6s4.7 1.6 5.2 4.6M16 14.5c2.2-.1 3.9 1.3 4.3 4" />
    </>,
  ),
  tickets: g(
    <>
      <path d="M4 7.5h16v3a1.7 1.7 0 0 0 0 3.4v3.1H4v-3.1a1.7 1.7 0 0 0 0-3.4z" />
      <path d="M14.5 8.5v7" strokeDasharray="1.6 1.8" />
      <circle className="a" cx="8.8" cy="12" r="1.6" />
    </>,
  ),
  live: g(
    <>
      <circle className="a" cx="12" cy="12" r="2.2" />
      <path d="M8 8a5.7 5.7 0 0 0 0 8M16 8a5.7 5.7 0 0 1 0 8M5 5.2a9.8 9.8 0 0 0 0 13.6M19 5.2a9.8 9.8 0 0 1 0 13.6" />
    </>,
  ),

  // ---- Business ----
  inventory: g(
    <>
      <rect x="4" y="4.5" width="16" height="4.5" rx="1.5" />
      <path d="M5.5 9v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2-2V9" />
      <path className="as" d="M10 13h4" />
    </>,
  ),
  pos: g(
    <>
      <rect x="7" y="3.5" width="10" height="5" rx="1.5" />
      <rect x="4.5" y="10" width="15" height="10" rx="2.5" />
      <circle className="a" cx="8.5" cy="14.3" r=".95" />
      <circle className="a" cx="12" cy="14.3" r=".95" />
      <circle className="a" cx="15.5" cy="14.3" r=".95" />
      <circle className="a" cx="8.5" cy="17.2" r=".95" />
      <circle className="a" cx="12" cy="17.2" r=".95" />
    </>,
  ),
  invoicing: g(
    <>
      <path d="M6.5 3.5h11v17l-2.2-1.5-2.3 1.5-2.3-1.5-2.2 1.5z" />
      <path d="M9.5 8h5M9.5 11.2h5" />
      <rect className="a" x="9.5" y="14" width="3.2" height="1.9" rx=".9" />
    </>,
  ),
  accounting: g(
    <>
      <path className="s" d="M3.5 9.5L12 4l8.5 5.5z" />
      <path d="M3.5 9.5L12 4l8.5 5.5zM6 11.5v6M10 11.5v6M14 11.5v6M18 11.5v6M4 20h16" />
    </>,
  ),
  crm: g(
    <>
      <circle cx="12" cy="12" r="8.5" />
      <circle cx="12" cy="12" r="4.6" />
      <circle className="a" cx="12" cy="12" r="1.8" />
    </>,
  ),
  hr: g(
    <>
      <rect x="5" y="3.5" width="14" height="17" rx="3" />
      <path d="M10 6.5h4M8.6 17.5c.5-2 1.8-3 3.4-3s2.9 1 3.4 3" />
      <circle className="a" cx="12" cy="11.4" r="2.3" />
    </>,
  ),
  talent: g(
    <>
      <circle cx="10" cy="8.5" r="3" />
      <path d="M3.8 19.5c.6-3.4 3-5.2 6.2-5.2 1.6 0 3 .4 4 1.2" />
      <path className="as" d="M18 9v6M15 12h6" />
    </>,
  ),
  shifts: g(
    <>
      <rect x="4" y="5.5" width="9" height="3.4" rx="1.7" />
      <rect className="a" x="8" y="10.3" width="12" height="3.4" rx="1.7" />
      <rect x="4" y="15.1" width="8" height="3.4" rx="1.7" />
    </>,
  ),
  timetracking: g(
    <>
      <path className="s" d="M12 13.5V7a6.5 6.5 0 0 1 6.5 6.5z" />
      <circle cx="12" cy="13.5" r="7" />
      <path d="M12 13.5V9.8M9.5 3.5h5M12 3.5v3" />
    </>,
  ),
  fieldservice: g(
    <>
      <path d="M20.2 6.6a4.4 4.4 0 0 1-5.9 5.6L7.2 19.3a2 2 0 1 1-2.8-2.8l7.1-7.1a4.4 4.4 0 0 1 5.6-5.8l-2.6 2.6.5 2.2 2.2.5z" />
      <circle className="a" cx="5.8" cy="18" r="1.1" />
    </>,
  ),
  fleet: g(
    <>
      <rect x="2.5" y="7" width="11" height="9" rx="1.8" />
      <path d="M13.5 10h4l3 3.2V16h-7z" />
      <circle className="a" cx="7" cy="17.5" r="2" />
      <circle className="a" cx="17" cy="17.5" r="2" />
    </>,
  ),
  projects: g(
    <>
      <rect x="4" y="4.5" width="16" height="15" rx="3" />
      <path d="M8.2 8v8M15.8 8v4.5" />
      <rect className="a" x="10.4" y="8" width="3.2" height="6" rx="1" />
    </>,
  ),
  contracts: g(
    <>
      <path d="M6.5 3.5h8l3.5 3.5v13.5h-11.5z" />
      <path d="M9 14c1-1.8 1.8-1.8 2.5 0s1.4 1.2 2-.2M9 8.5h4" />
      <circle className="a" cx="15.4" cy="17.6" r="1.9" />
    </>,
  ),
  legal: g(
    <>
      <path d="M12 4.5v15M7.5 19.5h9M5 7.5h14" />
      <path d="M5 7.5l-2 5.2a2.4 2.4 0 0 0 4 0zM19 7.5l-2 5.2a2.4 2.4 0 0 0 4 0z" />
      <circle className="a" cx="12" cy="4.6" r="1.6" />
    </>,
  ),
  captable: g(
    <>
      <circle cx="12" cy="12" r="7.5" />
      <path d="M12 4.5V12h7.5" />
      <path className="a" d="M12 12V4.5a7.5 7.5 0 0 1 7.5 7.5z" />
    </>,
  ),
  signatures: g(
    <>
      <path d="M4.5 19.5l1-4.5L16 4.5a2.1 2.1 0 0 1 3 3L8.5 18zM14 6.5l3 3" />
      <path className="as" d="M12.5 20.5h7" />
    </>,
  ),
  records: g(
    <>
      <ellipse className="s" cx="12" cy="6.5" rx="7" ry="2.5" />
      <path d="M5 6.5c0-1.4 3.1-2.5 7-2.5s7 1.1 7 2.5-3.1 2.5-7 2.5-7-1.1-7-2.5zM5 6.5v11c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5v-11M5 12c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5" />
    </>,
  ),
  sites: g(
    <>
      <rect x="3.5" y="5" width="17" height="14" rx="3" />
      <path d="M3.5 9.5h17M7.5 13.5h9M7.5 16.2h5" />
      <circle className="a" cx="6.6" cy="7.3" r=".8" />
      <circle className="a" cx="9.2" cy="7.3" r=".8" />
    </>,
  ),

  // ---- Admin ----
  admin: g(
    <>
      <path className="s" d="M12 3.5l7.5 3v5.5c0 4.4-3 7.6-7.5 9-4.5-1.4-7.5-4.6-7.5-9V6.5z" />
      <path d="M12 3.5l7.5 3v5.5c0 4.4-3 7.6-7.5 9-4.5-1.4-7.5-4.6-7.5-9V6.5zM9 12l2.2 2.2 3.9-4.4" />
    </>,
  ),
  access: g(
    <>
      <rect className="a" x="9" y="3.5" width="6" height="4.5" rx="1.5" />
      <rect x="3.5" y="16" width="6" height="4.5" rx="1.5" />
      <rect x="14.5" y="16" width="6" height="4.5" rx="1.5" />
      <path d="M12 8v4M6.5 16v-4h11v4" />
    </>,
  ),
  git: g(
    <>
      <circle cx="7" cy="6" r="2" />
      <circle cx="7" cy="18" r="2" />
      <circle className="a" cx="17" cy="9" r="2" />
      <path d="M7 8v8M17 11c0 3-3.5 3.5-8.2 5.2" />
    </>,
  ),
  cloudimport: g(
    <>
      <path d="M7.5 18.5a4.5 4.5 0 0 1-.4-9 5.8 5.8 0 0 1 11.2 1.6 3.7 3.7 0 0 1-.8 7.4z" />
      <path className="as" d="M12 10.5v6M9.6 14l2.4 2.5 2.4-2.5" />
    </>,
  ),
  orgsync: g(
    <>
      <path d="M5 9h13l-3.5-3.5" />
      <path className="as" d="M19 15H6l3.5 3.5" />
    </>,
  ),
  vpn: g(
    <>
      <rect x="5.5" y="10.5" width="13" height="9.5" rx="2.5" />
      <path d="M8.5 10.5V8a3.5 3.5 0 0 1 7 0v2.5" />
      <circle className="a" cx="12" cy="15.2" r="1.5" />
    </>,
  ),
  homeassistant: g(
    <>
      <path d="M4 11l8-6.5 8 6.5v8a1.5 1.5 0 0 1-1.5 1.5h-13A1.5 1.5 0 0 1 4 19z" />
      <rect className="a" x="10" y="14" width="4" height="6.5" rx="1" />
    </>,
  ),
  assemble: g(
    <>
      <circle className="a" cx="12" cy="12" r="2.5" />
      <circle cx="5.5" cy="6" r="1.8" />
      <circle cx="18.5" cy="6" r="1.8" />
      <circle cx="12" cy="19.5" r="1.8" />
      <path d="M7 7.2l3.2 3.2M17 7.2l-3.2 3.2M12 14.5v3.2" />
    </>,
  ),
  spacelight: g(
    <>
      <path d="M4 12l8-7 8 7M6.5 10.5v9h11v-9" />
      <circle className="a" cx="12" cy="14.5" r="2" />
    </>,
  ),

  // ---- Media / More ----
  photos: g(
    <>
      <rect x="3.5" y="4.5" width="17" height="15" rx="3" />
      <path d="M4 17l5-5 4 4 2.5-2.5 4.5 4.5" />
      <circle className="a" cx="15.5" cy="9" r="1.8" />
    </>,
  ),
  music: g(
    <>
      <path d="M9 17.5V6l9-2v11.5" />
      <circle className="a" cx="7" cy="17.5" r="2.2" />
      <circle className="a" cx="16" cy="15.5" r="2.2" />
    </>,
  ),
  video: g(
    <>
      <rect x="3.5" y="5.5" width="17" height="13" rx="3" />
      <path className="a" d="M10.5 9.4v5.2l4.2-2.6z" />
    </>,
  ),
  games: g(
    <>
      <path d="M7 8h10a4.5 4.5 0 0 1 4.4 5.4l-.7 3.2a2.4 2.4 0 0 1-4.2 1L14.8 16H9.2l-1.7 1.6a2.4 2.4 0 0 1-4.2-1l-.7-3.2A4.5 4.5 0 0 1 7 8z" />
      <path d="M8 11v3M6.5 12.5h3" />
      <circle className="a" cx="15" cy="11.6" r=".95" />
      <circle className="a" cx="17" cy="13.6" r=".95" />
    </>,
  ),
  podcasts: g(
    <>
      <circle className="a" cx="12" cy="10.2" r="2.3" />
      <path d="M7.7 15a6 6 0 1 1 8.6 0M4.8 17.6a10 10 0 1 1 14.4 0M12 14v6" />
    </>,
  ),
  books: g(
    <>
      <rect x="4" y="4.5" width="4" height="15" rx="1.2" />
      <rect className="a" x="10" y="4.5" width="4" height="15" rx="1.2" />
      <rect x="16" y="4.5" width="4" height="15" rx="1.2" />
    </>,
  ),
  translate: g(
    <>
      <path d="M4 6.5h9M8.5 4.5v2M6 6.5c.5 3 2.5 5.5 5.5 7M11 6.5c-.5 3-3 5.7-6.5 7.5" />
      <path className="as" d="M13.5 19.5l3.3-8 3.3 8M14.8 16.8h4" />
    </>,
  ),
  "3d": g(
    <>
      <path className="s" d="M12 12l7.5-4.2v8.5L12 20.5z" />
      <path d="M12 3.5l7.5 4.2v8.6L12 20.5l-7.5-4.2V7.7zM4.7 7.8L12 12l7.3-4.2M12 12v8.3" />
    </>,
  ),
  archaeology: g(
    <>
      <path d="M9 4h6M10 4v2.2c-2.5 1.3-4 3.4-4 6.1 0 4 2.7 7.2 6 7.2s6-3.2 6-7.2c0-2.7-1.5-4.8-4-6.1V4" />
      <path className="as" d="M7 11.5h10" />
    </>,
  ),
  maps: g(
    <>
      <path d="M12 20.5s6-5.2 6-10a6 6 0 0 0-12 0c0 4.8 6 10 6 10z" />
      <circle className="a" cx="12" cy="10.5" r="2.2" />
    </>,
  ),
  learn: g(
    <>
      <path className="s" d="M2.5 9.5L12 5l9.5 4.5L12 14z" />
      <path d="M2.5 9.5L12 5l9.5 4.5L12 14zM6.5 11.8V16c1.4 1.4 3.4 2.2 5.5 2.2s4.1-.8 5.5-2.2v-4.2M21.5 9.5V15" />
    </>,
  ),

  // ---- Product UI glyphs ----
  workspace: g(
    <>
      <rect x="4" y="4" width="6.5" height="6.5" rx="2" />
      <rect className="a" x="13.5" y="4" width="6.5" height="6.5" rx="2" />
      <rect x="4" y="13.5" width="6.5" height="6.5" rx="2" />
      <rect x="13.5" y="13.5" width="6.5" height="6.5" rx="2" />
    </>,
  ),
  ask: g(
    <>
      <path d="M10.5 3.5l1.9 5.4 5.4 1.9-5.4 1.9-1.9 5.4-1.9-5.4-5.4-1.9 5.4-1.9z" />
      <path className="a" d="M18.5 15.5l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8z" />
    </>,
  ),
};

/** Glyph names that have a drawing (used by the tests and the legacy fallback). */
export const GLYPH_NAMES = Object.keys(GLYPHS);
