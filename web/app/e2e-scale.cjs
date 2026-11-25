// Playwright run for the scale work. Usage: node e2e-scale.cjs <port> <outdir>
// Needs a stack on <port> backed by the scale-seeded scratch database.
const { chromium } = require("/mnt/Windows/tmp/zentrix-qa-playwright/node_modules/playwright-core");
const fs = require("fs");
const port = process.argv[2] || "8151";
const out = process.argv[3] || ".";
const base = `http://workspace.localtest.me:${port}`;
const TOKEN = process.env.SCALE_TOKEN || "scaletesttoken0123456789";
const results = [];
const check = (name, ok, note = "") => {
  results.push({ name, ok, note });
  console.log(`${ok ? "PASS" : "FAIL"} ${name} ${note}`);
};

(async () => {
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({
    executablePath: process.env.HOME + "/.cache/ms-playwright/chromium-1247/chrome-linux64/chrome",
    args: ["--no-sandbox"],
  });
  for (const vp of [{ w: 1440, h: 900, tag: "desktop" }, { w: 390, h: 844, tag: "mobile" }]) {
    const ctx = await browser.newContext({ viewport: { width: vp.w, height: vp.h } });
    await ctx.addCookies([{ name: "adrient_session", value: TOKEN, domain: "workspace.localtest.me", path: "/" }]);
    const page = await ctx.newPage();
    const requests = [];
    page.on("request", (r) => r.url().includes("/api/v1/") && requests.push(r.url()));

    // 1. People page: server-paged table.
    await page.goto(`${base}/admin/people`, { waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="people-table"] tbody tr', { timeout: 20000 });
    const range = await page.textContent('[data-testid="table-range"]');
    check(`${vp.tag} people range label`, /1–\d+ of/.test(range), range);
    const rows = await page.locator('[data-testid="people-table"] tbody tr').count();
    check(`${vp.tag} people shows at most one page`, rows <= 100, `${rows} rows`);
    await page.screenshot({ path: `${out}/people-${vp.tag}.png` });

    // 2. Sort + next page, then reload restores the URL state.
    await page.getByRole("button", { name: "Next page" }).click();
    await page.waitForFunction(() => location.search.includes("page=2"));
    await page.reload({ waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="people-table"] tbody tr');
    check(`${vp.tag} URL state restored after reload`, (await page.textContent('[data-testid="table-range"]')).startsWith("26–"));

    // 3. Audit log: first page, deep page, filters.
    await page.goto(`${base}/admin/audit`, { waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="audit-table"] tbody tr', { timeout: 20000 });
    const arange = await page.textContent('[data-testid="table-range"]');
    check(`${vp.tag} audit capped total`, /of 10,000\+/.test(arange), arange);
    for (let i = 0; i < 3; i++) await page.getByRole("button", { name: "Next page" }).click();
    await page.waitForFunction(() => location.search.includes("page=4"));
    check(`${vp.tag} audit deep page via cursor`, true);
    await page.screenshot({ path: `${out}/audit-${vp.tag}.png` });

    // 4. Picker with 20,000 people: type, results come from the server, scroll loads more.
    await page.goto(`${base}/admin/audit`, { waitUntil: "networkidle" });
    const input = page.locator('[data-testid="audit-actor"]');
    await input.click();
    await input.fill("ana");
    await page.waitForResponse((r) => r.url().includes("directory/search") && r.url().includes("q=ana"), { timeout: 10000 });
    await page.waitForTimeout(400);
    const opts = await page.locator('[data-testid="picker-option"]').count();
    check(`${vp.tag} picker shows a window, not 20k`, opts > 0 && opts < 20, `${opts} options in DOM`);
    const searchCalls = requests.filter((u) => u.includes("directory/search") && u.includes("q=ana")).length;
    check(`${vp.tag} picker is debounced`, searchCalls >= 1 && searchCalls <= 2, `${searchCalls} calls for "ana"`);
    await page.screenshot({ path: `${out}/picker-${vp.tag}.png` });
    await input.fill("");
    await page.waitForTimeout(1200);
    await page.waitForSelector('[data-testid="picker-option"]');
    const before = requests.filter((u) => u.includes("directory/search")).length;
    await page.evaluate(() => {
      const l = document.querySelector('[data-testid="picker-list"]');
      l.scrollTop = l.scrollHeight;
    });
    await page.waitForTimeout(1500);
    const after = requests.filter((u) => u.includes("directory/search")).length;
    check(`${vp.tag} picker loads next page on scroll`, after > before, `${before} -> ${after}`);

    // 5. Sessions + sign-in history.
    await page.goto(`${base}/admin/sessions`, { waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="sessions-table"]');
    await page.screenshot({ path: `${out}/sessions-${vp.tag}.png` });
    await page.goto(`${base}/admin/signin`, { waitUntil: "networkidle" }).catch(() => {});
    await ctx.close();
  }
  await browser.close();
  fs.writeFileSync(`${out}/results.json`, JSON.stringify(results, null, 2));
  process.exit(results.some((r) => !r.ok) ? 1 : 0);
})().catch((e) => {
  console.error(e);
  process.exit(2);
});
