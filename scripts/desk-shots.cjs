// Screenshots and checks of the service desk and admin console. Usage:
//   SESSION_TOKEN=... PORT=8153 OUT=/path node scripts/desk-shots.cjs
const { chromium } = require("/mnt/Windows/tmp/zentrix-qa-playwright/node_modules/playwright-core");
const PORT = process.env.PORT || "8153";
const BASE = `http://workspace.localtest.me:${PORT}`;
const OUT = process.env.OUT || "/tmp";
const TOKEN = process.env.SESSION_TOKEN;
const exe = process.env.HOME + "/.cache/ms-playwright/chromium-1247/chrome-linux64/chrome";
const results = [];
const ok = (n, c, d = "") => { results.push(`${c ? "PASS" : "FAIL"} ${n} ${d}`); };

(async () => {
  const browser = await chromium.launch({ executablePath: exe });
  for (const [label, vp] of [["desktop", { width: 1440, height: 900 }], ["phone", { width: 390, height: 844 }]]) {
    const ctx = await browser.newContext({ viewport: vp });
    await ctx.addCookies([{ name: "adrient_session", value: TOKEN, domain: "workspace.localtest.me", path: "/" }]);
    const page = await ctx.newPage();
    const errs = [];
    page.on("pageerror", (e) => errs.push(e.message));
    const shot = (n) => page.screenshot({ path: `${OUT}/${label}-${n}.png` });
    await page.goto(`${BASE}/tickets`, { waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="workspace"]', { timeout: 20000 });
    await page.waitForSelector('[data-testid^="row-"]', { timeout: 20000 });
    await shot("workspace");
    const rows = await page.locator('[data-testid^="row-"]').count();
    ok(`${label} list rendered`, rows > 5, `rows=${rows}`);
    await page.locator('[data-testid^="row-"]').first().click();
    await page.waitForSelector('[data-testid="ticket-pane"]', { timeout: 15000 });
    await page.waitForTimeout(800);
    await shot("ticket");
    ok(`${label} ticket pane`, await page.locator('[data-testid="composer"]').count() === 1);
    if (label === "desktop") {
      // keyboard: j moves, r focuses reply, ? opens help
      await page.locator("body").click({ position: { x: 5, y: 300 } });
      await page.keyboard.press("j");
      await page.keyboard.press("r");
      await page.waitForTimeout(300);
      ok("shortcut r focuses composer", await page.evaluate(() => document.activeElement && document.activeElement.getAttribute("data-testid") === "composer"));
      await page.keyboard.type("Hello from the browser test");
      await page.waitForTimeout(2500);
      ok("draft autosaved", (await page.locator("text=Draft saved").count()) > 0);
      await page.keyboard.press("Escape");
      await page.keyboard.press("h");
      ok("shortcut help opens", await page.locator('[data-testid="shortcut-help"]').count() === 1);
      await shot("shortcuts");
      await page.keyboard.press("Escape");
      await page.waitForTimeout(600);
      await shot("before-note");
      console.log("MODALS", JSON.stringify(await page.evaluate(() => [...document.querySelectorAll(".MuiModal-root")].map((m) => (m.innerText || "").slice(0, 60) + " | " + m.className.slice(0, 30)))));
      // internal note
      await page.locator('[data-testid="tab-note"]').click({ timeout: 5000 }).catch(async () => {
        console.log("BLOCKED", JSON.stringify(await page.evaluate(() => [...document.querySelectorAll(".MuiModal-root")].map((m) => m.outerHTML.slice(0, 400)))));
        await page.screenshot({ path: `${OUT}/blocked.png` });
        process.exit(2);
      });
      await page.locator('[data-testid="composer"]').fill("Internal: checked the logs");
      await page.locator('[data-testid="send-btn"]').click();
      await page.waitForSelector("text=internal note", { timeout: 8000 });
      ok("internal note posted", true);
      // command bar
      await page.keyboard.press("Control+Shift+K");
      await page.waitForSelector('[data-testid="command-bar"]');
      await shot("command-bar");
      await page.keyboard.press("Escape");
      await page.waitForTimeout(600);
      // views
      await page.locator('[data-testid="view-unassigned"]').click();
      await page.waitForTimeout(800);
      await shot("view-unassigned");
      await page.locator('[data-testid="view-all_open"]').click();
      // collision: second user presence is checked via API in tests
    }
    for (const [n, url, sel] of [
      ["reports", null, '[data-testid="open-report"]'], ["kb", null, '[data-testid="open-kb"]'], ["settings", null, '[data-testid="open-help-settings"]'],
    ]) {
      await page.goto(`${BASE}/tickets`, { waitUntil: "networkidle" });
      await page.waitForSelector(sel);
      if (label === "phone") { await shot("tickets-phone-top"); }
      await page.locator(sel).click();
      await page.waitForTimeout(1500);
      await shot(n);
    }
    if (label === "desktop") {
      for (const t of ["teams", "hours", "sla", "automations", "forms"]) {
        await page.locator(`[data-testid="set-${t}"]`).click();
        await page.waitForTimeout(900);
        await shot(`settings-${t}`);
      }
      await page.locator('[data-testid="set-automations"]').click();
      await page.locator('[data-testid="new-auto"]').click();
      await page.waitForSelector('[data-testid="auto-dialog"]');
      await page.locator('[data-testid="auto-name"]').fill("Browser test rule");
      await shot("rule-builder");
      await page.keyboard.press("Escape");
      await page.waitForTimeout(600);
    } else {
      for (const t of ["teams", "sla", "automations"]) {
        await page.locator(`[data-testid="set-${t}"]`).click().catch(() => {});
        await page.waitForTimeout(700);
        await shot(`settings-${t}`);
      }
    }
    // portal
    const tok = await (await ctx.request.get(`${BASE}/api/v1/tickets/projects`)).json();
    const proj = tok.projects.find((p) => p.key === "HELP");
    await page.goto(`${BASE}/support/${proj.public_token}`, { waitUntil: "networkidle" });
    await shot("portal");
    await page.locator('[data-testid^="portal-form-"]').first().click();
    await page.waitForSelector('[data-testid="portal-form"]');
    await shot("portal-form");
    // admin
    await page.goto(`${BASE}/admin/dashboard`, { waitUntil: "networkidle" });
    await page.waitForTimeout(1200);
    await shot("admin-dashboard");
    if (label === "desktop") {
      await page.locator('[data-testid="admin-find"]:visible').fill("country");
      await page.waitForTimeout(300);
      ok("admin find a setting", await page.locator('[data-testid="admin-nav-geo"]:visible').count() === 1);
      await shot("admin-find");
      await page.locator('[data-testid="admin-find"]:visible').fill("");
      for (const s of ["users", "groups", "controls"]) {
        await page.goto(`${BASE}/admin/${s}`, { waitUntil: "networkidle" });
        await page.waitForTimeout(1000);
        await shot(`admin-${s}`);
      }
    } else {
      await page.goto(`${BASE}/admin/users`, { waitUntil: "networkidle" });
      await shot("admin-users");
    }
    ok(`${label} no page errors`, errs.length === 0, errs.slice(0, 2).join(" | "));
    await ctx.close();
  }
  await browser.close();
  console.log(results.join("\n"));
})().catch((e) => { console.log(results.join("\n")); console.error("SCRIPT ERROR", e.message); process.exit(1); });
