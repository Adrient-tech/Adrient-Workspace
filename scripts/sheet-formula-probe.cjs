// Probe Sheets formulas in the live editor on :8080.
const { chromium } = require("/mnt/Windows/tmp/zentrix-qa-playwright/node_modules/playwright-core");
const exe = process.env.HOME + "/.cache/ms-playwright/chromium-1247/chrome-linux64/chrome";
const base = process.env.BASE || "http://workspace.localtest.me:8161";
const out = process.argv[2];
(async () => {
  const browser = await chromium.launch({ executablePath: exe, args: ["--no-sandbox"] });
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  const errors = [];
  let step="start"; page.on("pageerror", (e) => errors.push("pageerror@"+step+" " + (e.stack||String(e)).slice(0,1500)));
  page.on("console", (m) => m.type() === "error" && errors.push("console " + m.text()));
  page.on("response", (r) => { if (r.url().includes("/recalc")) errors.push(`recalc ${r.status()}`); });
  const lr = await page.request.post(`${base}/api/v1/auth/login-password`, {
    data: { username: "admin@adrient.localtest.me", password: process.env.PW },
  });
  console.log("login", lr.status(), (await lr.text()).slice(0, 120));
  const cr = await page.request.post(`${base}/api/v1/sheets`, { data: { title: "Formula probe" } });
  console.log("create", cr.status(), (await cr.text()).slice(0, 200));
  let id;
  try { const j = await cr.json(); id = j.id || (j.sheet && j.sheet.id); } catch {}
  if (!id) { await page.goto(`${base}/sheets`); await page.waitForTimeout(3000); await page.screenshot({ path: `${out}/list.png` }); console.log(errors); await browser.close(); return; }
  await page.goto(`${base}/sheets/d/${id}`);
  await page.waitForTimeout(5000);
  const inputs = [["apple", "10"], ["pear", "20"]];
  // Click A1 via the name box if present, else the grid.
  const cellXY = async (r, c) => {
    const box = await page.locator("canvas").first().boundingBox();
    return { x: box.x + 46 + 73 * c + 30, y: box.y + 20 + 19 * r + 9 };
  };
  const typeAt = async (r, c, text) => { step = text;
    const p = await cellXY(r, c);
    await page.mouse.click(p.x, p.y);
    await page.keyboard.type(text);
    await page.keyboard.press("Enter");
    await page.waitForTimeout(300);
  };
  await typeAt(0, 0, "apple"); await typeAt(0, 1, "10");
  await typeAt(1, 0, "pear"); await typeAt(1, 1, "20");
  const f = ["=sum(B1:B2)", '=xlookup("pear",a1:a2,b1:b2)', "=SUM(B1;B2)", "= SUM(B1:B2)", "=SUM(B1:B2", '=XLOOKUP("kiwi",A1:A2,B1:B2,"none")', "=SUMIF(A1:A2,\"pear\",B1:B2)", "=COUNTA(A1:A2)"];
  for (let i = 0; i < f.length; i++) await typeAt(2 + i, 0, f[i]);
  await page.waitForTimeout(4000);
  await page.screenshot({ path: `${out}/sheet.png` });
  const r = await page.request.get(`${base}/api/v1/sheets/d/${id}`);
  console.log("get", r.status(), (await r.text()).slice(0, 1500));
  console.log("id", id); await page.request.delete(`${base}/api/v1/sheets/d/${id}`);
  console.log(errors.filter(e=>e.startsWith("pageerror")).join("\n"));
  await browser.close();
})();
