// Browser run for the AI assistant in Mail, Calendar, Tasks, Forms (and others).
// Usage: node e2e-ai2.cjs <app> "<prompt with @@tool lines>" <out.png>
const { chromium } = require("/mnt/Windows/tmp/zentrix-qa-playwright/node_modules/playwright-core");
const BASE = "http://workspace.localtest.me:8112";
(async () => {
  const [app, prompt, out, path] = process.argv.slice(2);
  const b = await chromium.launch({ executablePath: process.env.HOME + "/.cache/ms-playwright/chromium-1247/chrome-linux64/chrome", args: ["--no-sandbox"] });
  const ctx = await b.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  const login = await page.request.post(BASE + "/api/v1/auth/login-password", { data: { username: "admin", password: "DevPassword!1" } });
  console.log("login", login.status());
  await page.goto(BASE + (path || "/" + app), { waitUntil: "networkidle" });
  await page.getByTestId("assistant-open").first().click();
  await page.waitForSelector("textarea", { timeout: 10000 });
  await page.fill("textarea", prompt.replace(/\\n/g, "\n"));
  await page.keyboard.press("Enter");
  await page.waitForTimeout(9000);
  // confirm any destructive prompt
  const allow = page.getByRole("button", { name: /^allow/i });
  if (await allow.count()) { await allow.first().click(); await page.waitForTimeout(3000); }
  await page.screenshot({ path: out });
  console.log((await page.locator("[data-assistant-slot]").innerText()).slice(0, 1500));
  await b.close();
})().catch((e) => { console.error(e); process.exit(1); });
