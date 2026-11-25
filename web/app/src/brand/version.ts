/** Product identity. The version and build stamp are injected at build time from
 *  the repo-root VERSION file (see vite.config.ts); the server reports the same
 *  number at GET /api/v1/version. */
export const PRODUCT_NAME = "Adrient Workspace";
export const PRODUCT_URL = "https://adrient.com";
export const SUPPORT_EMAIL = "info@adrient.com";
export const LICENCE = "MIT";

export const APP_VERSION: string = typeof __ADRIENT_VERSION__ !== "undefined" ? __ADRIENT_VERSION__ : "dev";
/** "2026-11-21 · a1b2c3d" (date, short commit when git is available). */
export const APP_BUILD: string = typeof __ADRIENT_BUILD__ !== "undefined" ? __ADRIENT_BUILD__ : "unknown";
