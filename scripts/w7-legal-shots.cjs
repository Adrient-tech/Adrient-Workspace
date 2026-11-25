// Browser run for Legal and Cap table: logs in on :8080 (shared cookie), then walks both apps on PORT at 1440x900 and 390x844.
// Usage: node scripts/w7-legal-shots.cjs <outdir> [port]
const { chromium } = require("/mnt/Windows/tmp/zentrix-qa-playwright/node_modules/playwright-core");
const out = process.argv[2];
const port = process.argv[3] || "8133";
const exe = process.env.HOME + "/.cache/ms-playwright/chromium-1247/chrome-linux64/chrome";
const base = `http://workspace.localtest.me:${port}`;

// Session cookie from a curl cookie jar (path in COOKIES), made by POST /api/v1/auth/login-password.
function jarCookies() {
  const fs = require("fs");
  return fs.readFileSync(process.env.COOKIES, "utf8").split("\n").filter((l) => l.startsWith("#HttpOnly_") || (l && !l.startsWith("#"))).map((l) => {
    const p = l.replace("#HttpOnly_", "").split("\t");
    return { name: p[5], value: p[6], domain: p[0], path: p[2], httpOnly: true, secure: false };
  });
}
async function login(page) {
  await page.context().addCookies(jarCookies());
}

(async () => {
  const browser = await chromium.launch({ executablePath: exe, args: ["--no-sandbox"] });
  for (const [name, vp] of [["desktop", { width: 1440, height: 900 }], ["mobile", { width: 390, height: 844 }]]) {
    const ctx = await browser.newContext({ viewport: vp });
    const page = await ctx.newPage();
    const errors = [];
    page.on("pageerror", (e) => errors.push(String(e)));
    page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
    await login(page);
    const shot = async (file) => { await page.waitForLoadState("networkidle"); await page.waitForTimeout(1200); await page.screenshot({ path: `${out}/${name}-${file}.png`, fullPage: false }); };
    await page.goto(`${base}/legal`, { waitUntil: "networkidle" });
    await page.getByText("Upcoming deadlines").waitFor({ timeout: 30000 });
    await shot("legal-dashboard");
    await page.getByRole("tab", { name: "Matters" }).click();
    await shot("legal-matters");
    await page.locator('[data-testid="matter-row"]').last().click();
    await shot("legal-matter-detail");
    await page.getByRole("tab", { name: /Time & expenses/ }).click();
    await shot("legal-matter-time");
    await page.getByRole("tab", { name: /Bills/ }).first().click();
    await shot("legal-matter-bills");
    await page.keyboard.press("Escape");
    await page.getByRole("tab", { name: "Conflict check" }).click();
    await page.locator('[data-testid="conflict-q"] input').fill("Bharat");
    await page.locator('[data-testid="conflict-go"]').click();
    await shot("legal-conflict");
    await page.getByRole("tab", { name: "Deadlines" }).click();
    await shot("legal-deadlines");
    await page.goto(`${base}/captable`, { waitUntil: "networkidle" });
    await page.getByText("Shares outstanding").waitFor({ timeout: 30000 });
    await shot("captable-dashboard");
    await page.getByRole("tab", { name: "Cap table" }).click();
    await shot("captable-table");
    await page.getByRole("tab", { name: "Securities" }).click();
    await shot("captable-securities");
    await page.getByRole("tab", { name: "Rounds" }).click();
    await page.locator('[data-testid="pre-money"] input').fill("60 crore");
    await page.locator('[data-testid="investment"] input').fill("10 crore");
    await page.locator('[data-testid="model-round"]').click();
    await page.waitForTimeout(800);
    await shot("captable-round-model");
    await page.getByRole("tab", { name: "Exit scenarios" }).click();
    await page.locator('[data-testid="run-exit"]').click();
    await shot("captable-exit");
    console.log(name, "errors:", JSON.stringify(errors.slice(0, 8)));
    await ctx.close();
  }
  await browser.close();
})();
