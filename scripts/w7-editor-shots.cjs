// Screenshots of the three editors at several widths against a scratch backend.
// usage: node scripts/w7-editor-shots.cjs <outDir> [port] [kind] [widths]
// The session cookie is a row seeded into the scratch database (see wave 7 notes).
const { chromium } = require("/mnt/Windows/tmp/zentrix-qa-playwright/node_modules/playwright-core");
const fs = require("fs");
const out = process.argv[2] || "/tmp/shots";
const port = process.argv[3] || "8135";
const only = process.argv[4] || "all";
const widths = (process.argv[5] || "1440,1280,1024,390").split(",").map(Number);
const TOKEN = process.env.W7_TOKEN || "w7editorstoken0123456789abcdef0123456789abcdef";
const DOCS = {
  sheets: { id: process.env.W7_SHEET || "f83ed072-af03-44b3-9571-ceb2ce3318e3", tabs: ["Home", "Insert", "Formulas", "Data", "View"] },
  slides: { id: process.env.W7_SLIDE || "3438f29b-837f-47dc-92bd-3e28428439c3", tabs: ["Home", "Insert", "Animations", "View"] },
  docs: { id: process.env.W7_DOC || "24bbbb61-6a8f-4b4d-b6b0-ef75f72bed56", tabs: ["Home", "Insert", "Layout", "Review"] },
};
(async () => {
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({ executablePath: process.env.HOME + "/.cache/ms-playwright/chromium-1247/chrome-linux64/chrome", args: ["--no-sandbox"] });
  for (const kind of Object.keys(DOCS)) {
    if (only !== "all" && only !== kind) continue;
    for (const w of widths) {
      const ctx = await browser.newContext({ viewport: { width: w, height: w === 390 ? 844 : 900 } });
      await ctx.addCookies([{ name: "adrient_session", value: TOKEN, domain: "127.0.0.1", path: "/" }]);
      const page = await ctx.newPage();
      await page.goto(`http://127.0.0.1:${port}/${kind}/d/${DOCS[kind].id}`, { waitUntil: "load" });
      await page.waitForTimeout(3500);
      const tabs = w === 390 ? [""] : DOCS[kind].tabs;
      for (const t of tabs) {
        if (t) {
          const el = page.getByRole("tab", { name: t, exact: true }).first();
          if (await el.count()) await el.click();
          await page.waitForTimeout(700);
        }
        const info = await page.evaluate(() => {
          const rows = [...document.querySelectorAll('[data-ribbon-step]')];
          return rows.map((r) => ({ step: r.getAttribute("data-ribbon-step"), sw: r.scrollWidth, cw: r.clientWidth }));
        });
        console.log(kind, w, t || "mobile", JSON.stringify(info));
        await page.screenshot({ path: `${out}/${kind}-${w}${t ? "-" + t.toLowerCase() : ""}.png` });
      }
      await ctx.close();
    }
  }
  await browser.close();
})();
