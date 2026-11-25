// Shared helpers for the Sheets function probes (login, workbook JSON, recalc).
const { chromium } = require(process.env.PW_CORE || "/mnt/Windows/tmp/zentrix-qa-playwright/node_modules/playwright-core");
const exe = process.env.HOME + "/.cache/ms-playwright/chromium-1247/chrome-linux64/chrome";
const base = process.env.BASE || "http://workspace.localtest.me:8161";
const colName = (c) => { let s = ""; c++; while (c > 0) { const m = (c - 1) % 26; s = String.fromCharCode(65 + m) + s; c = Math.floor((c - 1) / 26); } return s; };
const cellVal = (v) => ({ v, m: String(v), ct: { fa: "General", t: typeof v === "number" ? "n" : typeof v === "boolean" ? "b" : "g" } });
// formulas: [{r,c,f}] f without "=" or with.
function workbook(fixture, formulas) {
  const celldata = fixture.map(([r, c, v]) => ({ r, c, v: typeof v === "string" && v.startsWith("=") ? { f: v } : cellVal(v) }));
  for (const x of formulas) celldata.push({ r: x.r, c: x.c, v: { f: x.f.startsWith("=") ? x.f : "=" + x.f } });
  return [{ name: "Sheet1", id: "sheet1", index: "sheet1", status: 1, order: 0, row: 600, column: 40, celldata, config: {} }];
}
async function login(page) {
  const r = await page.request.post(`${base}/api/v1/auth/login-password`, { data: { username: "admin@adrient.localtest.me", password: process.env.PW } });
  if (r.status() !== 200) throw new Error("login " + r.status());
}
async function launch() {
  const browser = await chromium.launch({ executablePath: exe, args: ["--no-sandbox"] });
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await ctx.addInitScript(() => localStorage.setItem("adrient.sheets.debug", "1"));
  const page = await ctx.newPage();
  await login(page);
  return { browser, ctx, page };
}
async function createSheet(page, title, data) {
  const cr = await page.request.post(`${base}/api/v1/sheets`, { data: { title } });
  const id = (await cr.json()).id;
  if (data) {
    const r = await page.request.put(`${base}/api/v1/sheets/d/${id}/data`, { data: { data: JSON.stringify(data) } });
    if (!r.ok()) throw new Error("save " + r.status() + " " + (await r.text()).slice(0, 200));
  }
  return id;
}
async function recalc(page, id, data) {
  const r = await page.request.post(`${base}/api/v1/sheets/d/${id}/recalc`, { data: { data: JSON.stringify(data) } });
  if (!r.ok()) throw new Error("recalc " + r.status());
  return (await r.json()).cells || [];
}
const isErr = (s) => typeof s === "string" && /^#(N\/A|VALUE!|REF!|DIV\/0!|NUM!|NAME\?|NULL!|SPILL!|CALC!|ERROR!|UNKNOWN)/i.test(s.trim());
module.exports = { chromium, exe, base, colName, workbook, login, launch, createSheet, recalc, isErr };
