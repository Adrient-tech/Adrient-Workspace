// Playwright pass over the screens converted in the second scale round.
// Usage: node e2e-scale2.cjs <port> <outdir>   (stack on <port> backed by the scale scratch database)
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
const shot = (page, name) => page.screenshot({ path: `${out}/${name}.png` });

(async () => {
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({
    executablePath: process.env.HOME + "/.cache/ms-playwright/chromium-1247/chrome-linux64/chrome",
    args: ["--no-sandbox"],
  });
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await ctx.addCookies([{ name: "adrient_session", value: TOKEN, domain: "workspace.localtest.me", path: "/" }]);
  const page = await ctx.newPage();
  const big = [];
  page.on("response", async (r) => {
    if (!r.url().includes("/api/v1/")) return;
    const len = Number(r.headers()["content-length"] || 0);
    if (len > 1_000_000) big.push(`${r.url()} ${len}`);
  });
  const step = async (name, fn) => {
    try {
      await fn();
    } catch (e) {
      check(name, false, String(e).split("\n")[0]);
      await shot(page, `fail-${name.replace(/\W+/g, "-")}`).catch(() => {});
    }
  };

  await step("hr people paged", async () => {
    await page.goto(`${base}/hr/people`, { waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="employee-table"] tbody tr', { timeout: 20000 });
    const rows = await page.locator('[data-testid="employee-table"] tbody tr').count();
    check("hr people paged", rows > 0 && rows <= 100, `${rows} rows; ${await page.textContent('[data-testid="employee-table"] [data-testid="table-range"]')}`);
    await page.getByRole("button", { name: "Next page" }).click();
    await page.waitForFunction(() => location.search.includes("emp_page=2"), null, { timeout: 10000 });
    check("hr people next page", true);
    await shot(page, "hr-people");
  });

  await step("accounting transactions keyset", async () => {
    await page.goto(`${base}/accounting`, { waitUntil: "networkidle" });
    await page.locator('[data-testid="acct-nav-transactions"]').click();
    await page.waitForSelector('[data-testid="acct-journal-table"] tbody tr', { timeout: 20000 });
    const first = await page.locator('[data-testid="acct-journal-table"] tbody tr').first().textContent();
    await page.getByRole("button", { name: "Next page" }).click();
    await page.waitForFunction((f) => document.querySelector('[data-testid="acct-journal-table"] tbody tr')?.textContent !== f, first, { timeout: 10000 });
    check("accounting transactions keyset", true, await page.textContent('[data-testid="acct-journal-table"] [data-testid="table-range"]'));
    await shot(page, "accounting-transactions");
  });

  await step("invoices load more", async () => {
    await page.goto(`${base}/invoicing`, { waitUntil: "networkidle" });
    await page.waitForLoadState("networkidle");
    await page.waitForTimeout(1500);
    const n = await page.locator('[data-testid="invc-row"]').count();
    check("invoices first page bounded", n <= 100, `${n} rows`);
    await shot(page, "invoices");
  });

  await step("crm owner picker", async () => {
    await page.goto(`${base}/crm`, { waitUntil: "networkidle" });
    await page.waitForTimeout(1500);
    await shot(page, "crm");
    check("crm loads", true);
  });

  await step("chat people search", async () => {
    await page.goto(`${base}/chat`, { waitUntil: "networkidle" });
    await page.waitForTimeout(1500);
    await shot(page, "chat");
    check("chat loads", true);
  });

  for (const s of ["honeypot", "ratelimit", "groups", "signin"]) {
    await step(`admin ${s}`, async () => {
      await page.goto(`${base}/admin/${s}`, { waitUntil: "networkidle" });
      await page.waitForTimeout(1200);
      await shot(page, `admin-${s}`);
      check(`admin ${s} renders`, (await page.locator("text=Something went wrong").count()) === 0);
    });
  }

  await step("admin group members picker", async () => {
    await page.goto(`${base}/admin/groups`, { waitUntil: "networkidle" });
    const create = page.getByRole("button", { name: /Create group|New group/i }).first();
    await create.click();
    const inp = page.locator('[data-testid="admin-group-members"]');
    await inp.click();
    await inp.fill("tanaka");
    await page.waitForResponse((r) => r.url().includes("directory/search") && r.url().includes("q=tanaka"), { timeout: 10000 });
    await page.waitForTimeout(500);
    const label = await page.locator('[data-testid="picker-option"]').first().textContent();
    check("admin group picker shows the typed match", /tanaka/i.test(label ?? ""), label ?? "");
    const n = await page.locator('[data-testid="picker-option"]').count();
    check("admin group members picker searches server", n > 0 && n < 20, `${n} options`);
    await shot(page, "admin-group-members");
  });

  check("no API reply over 1 MB", big.length === 0, big.join("; "));
  await browser.close();
  fs.writeFileSync(`${out}/results2.json`, JSON.stringify(results, null, 2));
  process.exit(results.some((r) => !r.ok) ? 1 : 0);
})().catch((e) => {
  console.error(e);
  process.exit(2);
});
