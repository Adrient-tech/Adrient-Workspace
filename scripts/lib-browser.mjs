// Shared Playwright helpers for the demo scripts (screenshots.mjs, seed-docs.mjs).
// No secrets live here: the login comes from the environment.
//
//   ADRIENT_URL        app origin (default http://workspace.localtest.me:8080)
//   ADRIENT_LOGIN_URL  origin to sign in on when ADRIENT_URL is an extra port (default: ADRIENT_URL)
//   ADRIENT_LOGIN      sign-in name (dev stack: admin)
//   ADRIENT_PASSWORD   its password (dev stack: see deploy/local/stack.sh)
//   PLAYWRIGHT_CORE    path to a playwright-core package (default: resolve "playwright-core")
//   CHROMIUM_PATH      chromium executable (default: Playwright's own browser)
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";

export const BASE = (process.env.ADRIENT_URL || "http://workspace.localtest.me:8080").replace(/\/$/, "");

const LOGIN = (process.env.ADRIENT_LOGIN_URL || BASE).replace(/\/$/, "");

export async function loadChromium() {
  let mod;
  if (process.env.PLAYWRIGHT_CORE) {
    const req = createRequire(pathToFileURL(process.env.PLAYWRIGHT_CORE + "/").href);
    mod = req(process.env.PLAYWRIGHT_CORE);
  } else {
    mod = await import("playwright-core");
  }
  return mod.chromium ?? mod.default.chromium;
}

export async function launch(viewport = { width: 1440, height: 900 }) {
  const chromium = await loadChromium();
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined, args: ["--no-sandbox"] });
  const context = await browser.newContext({ viewport, deviceScaleFactor: 1, colorScheme: "light" });
  return { browser, context };
}

/** signIn drives the hosted login (username, then password) and lands on the app. */
export async function signIn(page) {
  const user = process.env.ADRIENT_LOGIN;
  const pass = process.env.ADRIENT_PASSWORD;
  if (!user || !pass) throw new Error("set ADRIENT_LOGIN and ADRIENT_PASSWORD");
  await page.goto(`${LOGIN}/api/v1/auth/login`);
  await page.locator('input[name="loginName"], input[id="loginName"]').fill(user);
  await page.locator('button[type="submit"]').first().click();
  await page.locator('input[name="password"], input[id="password"]').fill(pass);
  await page.locator('button[type="submit"]').first().click();
  await page.waitForURL((u) => u.origin === new URL(LOGIN).origin, { timeout: 30000 });
  const who = await page.request.get(`${BASE}/api/v1/whoami`);
  if (who.status() !== 200) throw new Error("sign-in failed: whoami " + who.status());
}
