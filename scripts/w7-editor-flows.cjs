// Interaction checks for wave 7: Sheets grouping and a collapsed ribbon group, Slides motion path.
// usage: node scripts/w7-editor-flows.cjs <outDir> [port]
const { chromium } = require("/mnt/Windows/tmp/zentrix-qa-playwright/node_modules/playwright-core");
const out = process.argv[2] || "/tmp/flows";
const port = process.argv[3] || "8135";
const TOKEN = process.env.W7_TOKEN || "w7editorstoken0123456789abcdef0123456789abcdef";
(async () => {
  const browser = await chromium.launch({ executablePath: process.env.HOME + "/.cache/ms-playwright/chromium-1247/chrome-linux64/chrome", args: ["--no-sandbox"] });
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await ctx.addCookies([{ name: "adrient_session", value: TOKEN, domain: "127.0.0.1", path: "/" }]);
  const page = await ctx.newPage();
  page.on("dialog", (d) => {
    console.log("dialog:", d.message());
    d.dismiss();
  });
  // Sheets: group rows 2-6 and collapse them.
  await page.goto(`http://127.0.0.1:${port}/sheets/d/${process.env.W7_SHEET || "6ee1987f-bb92-42d9-a3a5-d15a09e43634"}`);
  await page.waitForTimeout(4000);
  await page.mouse.click(22, 322);
  await page.keyboard.down("Shift");
  await page.mouse.click(22, 402);
  await page.keyboard.up("Shift");
  await page.getByRole("tab", { name: "Data", exact: true }).click();
  await page.waitForTimeout(500);
  await page.getByRole("button", { name: "Group", exact: true }).click();
  await page.getByRole("menuitem", { name: "Group rows", exact: true }).click();
  await page.waitForTimeout(500);
  await page.getByRole("button", { name: "Group", exact: true }).click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: `${out}/flow-sheets-group-menu.png` });
  await page.getByRole("menuitem", { name: /Collapse Rows/ }).click();
  await page.waitForTimeout(800);
  await page.screenshot({ path: `${out}/flow-sheets-collapsed.png` });
  await page.getByRole("button", { name: "Group", exact: true }).click();
  await page.getByRole("menuitem", { name: /Expand Rows/ }).click();
  await page.waitForTimeout(500);
  // Narrow window: open a folded group.
  await page.setViewportSize({ width: 1024, height: 900 });
  await page.getByRole("tab", { name: "Home", exact: true }).click();
  await page.waitForTimeout(800);
  const folded = page.locator('[data-collapsed="1"] button').first();
  if (await folded.count()) {
    console.log("folded group:", await folded.getAttribute("aria-label"));
    await folded.click();
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${out}/flow-sheets-1024-folded-open.png` });
  } else console.log("no folded group at 1024");
  // Slides: add a motion path to the title.
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`http://127.0.0.1:${port}/slides/d/${process.env.W7_SLIDE || "3438f29b-837f-47dc-92bd-3e28428439c3"}`);
  await page.waitForTimeout(4000);
  await page.mouse.click(700, 330);
  await page.waitForTimeout(400);
  await page.getByRole("tab", { name: "Animations", exact: true }).click();
  await page.waitForTimeout(500);
  const arc = page.getByTestId("anim-path-pathArc");
  console.log("arc button enabled:", await arc.isEnabled().catch(() => "missing"));
  if (await arc.isEnabled().catch(() => false)) await arc.click();
  await page.waitForTimeout(800);
  await page.screenshot({ path: `${out}/flow-slides-motion.png` });
  await browser.close();
})();
