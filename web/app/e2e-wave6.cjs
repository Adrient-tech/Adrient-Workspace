// Real-browser run of the wave 6 Docs features. Usage:
//   node e2e-wave6.cjs <outdir> <dir with .docx corpus dumps>
// Serves the editor from vite on :8126 (proxying /api to the :8080 backend);
// log in once on :8080 (the script does it).
const { chromium } = require("/mnt/Windows/tmp/zentrix-qa-playwright/node_modules/playwright-core");
const fs = require("fs");
const cp = require("child_process");
const out = process.argv[2];
const corpus = process.argv[3];
fs.mkdirSync(out, { recursive: true });
const LOGIN = "http://workspace.localtest.me:8080";
const BASE = "http://workspace.localtest.me:8126";
const results = [];
const made = [];
const track = (p) => { const m = /\/docs\/d\/([^/?#]+)/.exec(p.url()); if (m) made.push(m[1]); };
const check = (name, ok, extra = "") => {
  results.push({ name, ok });
  console.log(ok ? "PASS" : "FAIL", name, extra);
};

(async () => {
  const browser = await chromium.launch({ executablePath: process.env.HOME + "/.cache/ms-playwright/chromium-1247/chrome-linux64/chrome", args: ["--no-sandbox"] });
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, acceptDownloads: true });
  const page = await ctx.newPage();
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(LOGIN + "/");
  await page.waitForLoadState("networkidle");
  if (await page.locator("input[type=password]").count()) {
    await page.locator("input").nth(0).fill("admin@adrient.localtest.me");
    await page.locator("input").nth(1).fill(process.env.DOCS_PW || "DevPassword!1");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await page.waitForTimeout(6000);
  }
  await page.goto(BASE + "/docs");
  await page.waitForLoadState("networkidle");
  // Templates in the gallery.
  const gallery = await page.locator("body").innerText();
  check("template gallery lists Report, Resume, Essay", /Report/.test(gallery) && /Resume/.test(gallery) && /Essay/.test(gallery));
  await page.screenshot({ path: out + "/01-gallery-1440.png" });

  const id = await page.evaluate(async () => {
    const r = await fetch("/api/v1/docs", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ title: "Wave6 check" }) });
    const j = JSON.parse(await r.text());
    return (j.doc || j).id;
  });
  console.log("doc", id);
  await page.goto(`${BASE}/docs/d/${id}`);
  await page.waitForSelector(".ProseMirror");
  await page.waitForTimeout(1500);
  const ed = page.locator(".ProseMirror").first();
  await ed.click();
  await page.keyboard.type("pear");
  await page.keyboard.press("Enter");
  await page.keyboard.type("apple");
  await page.keyboard.press("Enter");
  await page.keyboard.type("fig");
  await page.keyboard.press("Control+A");

  // Sort paragraphs.
  await page.getByRole("button", { name: "Tools", exact: true }).click();
  await page.locator('[data-testid="tools-sort"]').click();
  await page.waitForSelector('[data-testid="sort-dialog"]');
  await page.screenshot({ path: out + "/02-sort-dialog-1440.png" });
  await page.locator('[data-testid="sort-apply"]').click();
  await page.waitForTimeout(400);
  const texts = await ed.locator("p").allInnerTexts();
  check("Sort paragraphs A to Z", texts.slice(0, 3).join(",") === "apple,fig,pear", texts.join("|"));

  // Font effects on "fig".
  await ed.locator("p").nth(1).click({ clickCount: 3 });
  await page.getByRole("button", { name: "Format", exact: true }).click();
  await page.locator('[data-testid="format-font-effects"]').click();
  await page.waitForSelector('[data-testid="font-effects-dialog"]');
  await page.locator('[data-testid="fx-caps"] input').check();
  await page.locator('[data-testid="fx-spacing"]').fill("3");
  await page.locator('[data-testid="fx-hidden"] input').check().catch(() => {});
  await page.locator('[data-testid="fx-hidden"] input').uncheck();
  await page.screenshot({ path: out + "/03-font-effects-1440.png" });
  await page.locator('[data-testid="fx-apply"]').click();
  await page.waitForTimeout(400);
  const css = await page.evaluate(() => {
    const s = document.querySelector(".ProseMirror span[data-font-effects]");
    if (!s) return null;
    const c = getComputedStyle(s);
    return { transform: c.textTransform, spacing: c.letterSpacing, text: s.textContent };
  });
  check("Font effects paint caps and spacing", !!css && css.transform === "uppercase" && parseFloat(css.spacing) > 3, JSON.stringify(css));

  // Statistics with readability.
  await ed.click();
  await page.keyboard.press("Control+End");
  await page.keyboard.press("Enter");
  await page.keyboard.type("The cat sat on the mat. The report was written by Ann.");
  await page.getByRole("button", { name: "Tools", exact: true }).click();
  await page.locator('[data-testid="tools-stats"]').click();
  await page.waitForSelector('[data-testid="readability"]');
  const ease = await page.locator('[data-testid="read-ease"]').innerText();
  check("Document statistics shows readability", /\d/.test(ease), ease.replace(/\s+/g, " "));
  await page.screenshot({ path: out + "/04-stats-1440.png" });
  await page.keyboard.press("Escape");

  // Read aloud menu entry exists (speech synthesis present in chromium).
  await page.getByRole("button", { name: "Tools", exact: true }).click();
  check("Read aloud is offered", (await page.locator('[data-testid="tools-read-aloud"]').count()) === 1);
  await page.keyboard.press("Escape");

  // Download .docx and look inside.
  await page.getByRole("button", { name: "File", exact: true }).click();
  await page.getByText("Download", { exact: true }).click();
  const dl = page.waitForEvent("download", { timeout: 20000 });
  await page.getByText("Microsoft Word (.docx)").click();
  const d = await dl;
  const docxPath = out + "/export.docx";
  await d.saveAs(docxPath);
  const xml = cp.execSync(`unzip -p "${docxPath}" word/document.xml`).toString();
  check("exported .docx carries w:caps and w:spacing", xml.includes("<w:caps/>") && /<w:spacing w:val="\d+"/.test(xml));

  // Re-import the export through the list.
  await page.goto(BASE + "/docs");
  await page.waitForLoadState("networkidle");
  await page.locator("input[type=file]").first().setInputFiles(docxPath);
  await page.waitForURL(/\/docs\/d\//, { timeout: 30000 }).catch(() => {});
  await page.waitForSelector(".ProseMirror");
  track(page);
  await page.waitForTimeout(2500);
  const again = await page.evaluate(() => {
    const s = document.querySelector(".ProseMirror span[data-font-effects]");
    return s ? getComputedStyle(s).textTransform : null;
  });
  check("re-imported .docx keeps all caps", again === "uppercase", String(again));

  // Corpus documents through the real import path.
  const names = corpus ? fs.readdirSync(corpus).filter((n) => n.endsWith(".in.docx")) : [];
  for (const n of names) {
    await page.goto(BASE + "/docs");
    await page.waitForLoadState("networkidle");
    await page.locator("input[type=file]").first().setInputFiles(corpus + "/" + n);
    await page.waitForURL(/\/docs\/d\//, { timeout: 30000 }).catch(() => {});
    const ok = await page.waitForSelector(".ProseMirror", { timeout: 20000 }).then(() => true).catch(() => false);
    track(page);
    await page.waitForTimeout(1500);
    const len = ok ? (await page.locator(".ProseMirror").first().innerText()).length : 0;
    check("corpus import opens: " + n, ok && len > 0, `${len} chars`);
    if (/formatting-matrix|fields|tracked/.test(n)) await page.screenshot({ path: out + `/05-${n.replace(".in.docx", "")}-1440.png` });
  }

  // Mobile.
  const m = await browser.newContext({ viewport: { width: 390, height: 844 }, storageState: await ctx.storageState() });
  const mp = await m.newPage();
  mp.on("pageerror", (e) => errors.push("mobile " + e.message));
  await mp.goto(`${BASE}/docs/d/${id}`);
  await mp.waitForSelector(".ProseMirror");
  await mp.waitForTimeout(2000);
  await mp.screenshot({ path: out + "/06-mobile-390.png" });
  check("mobile editor renders", (await mp.locator(".ProseMirror").count()) > 0);

  // Clean up the test document.
  await page.goto(BASE + "/docs");
  for (const i of [id, ...made]) await page.evaluate(async (x) => { await fetch("/api/v1/docs/" + x, { method: "DELETE", credentials: "same-origin" }); }, i);
  check("no page errors", errors.length === 0, errors.slice(0, 3).join(" | "));
  const failed = results.filter((r) => !r.ok).length;
  console.log(`${results.length - failed}/${results.length} checks passed`);
  await browser.close();
  process.exit(failed ? 1 : 0);
})();
