// Browser probe: types every catalog function into the live editor and compares the
// displayed value with the Go engine. Usage:
//   PW=… BASE=http://workspace.localtest.me:8161 node scripts/sheet-fn-probe.cjs cases.json engine.json report.json [cell|lower|bar] [limit]
const L = require("./sheet-fn-lib.cjs");
const fs = require("fs");
const [casesF, engineF, reportF, mode = "cell", limit = "0"] = process.argv.slice(2);
const VOLATILE = /^(NOW|TODAY|RAND|RANDBETWEEN|RANDARRAY|STOCKHISTORY)$/;
const sameVal = (disp, eng) => {
  if (!disp) return false;
  const dn = Number(disp.v), en = Number(eng.v);
  if (disp.v != null && eng.v != null && typeof disp.v !== "boolean" && typeof eng.v !== "boolean" && !Number.isNaN(dn) && !Number.isNaN(en) && String(disp.v).trim() !== "" && String(eng.v).trim() !== "")
    return Math.abs(dn - en) <= 1e-6 * Math.max(1, Math.abs(en));
  if (typeof eng.v === "boolean") return String(disp.v).toUpperCase() === String(eng.v).toUpperCase() || String(disp.m).toUpperCase() === String(eng.v).toUpperCase();
  return String(disp.m ?? disp.v) === String(eng.m) || String(disp.v) === String(eng.v);
};
(async () => {
  const { cases: all, fixture, formulaCol } = JSON.parse(fs.readFileSync(casesF, "utf8"));
  const engine = JSON.parse(fs.readFileSync(engineF, "utf8"));
  const cases = Number(limit) ? all.slice(0, Number(limit)) : all;
  const { browser, page } = await L.launch();
  const errors = [];
  page.on("pageerror", (e) => { errors.push(String(e.stack || e).slice(0, 400)); if (process.env.DEBUG) console.log("PAGEERROR", String(e.stack || e).slice(0, 600)); });
  const log = (...a) => process.env.DEBUG && console.log(...a);
  const report = { mode, total: cases.length, results: [], pageErrors: errors };
  const open = async (title) => {
    const id = await L.createSheet(page, title, L.workbook(fixture, []));
    await page.goto(`${L.base}/sheets/d/${id}`);
    await page.waitForFunction(() => window.__adrientSheet && window.__adrientSheet.wb(), null, { timeout: 30000 });
    await page.waitForTimeout(1500);
    return id;
  };
  const gotoCell = async (addr) => {
    const nb = page.locator(".fortune-name-box").first();
    await nb.click(); await page.keyboard.press("Control+A"); await page.keyboard.type(addr); await page.keyboard.press("Enter");
    await page.waitForTimeout(150);
  };
  const typeFormulas = async (items, startCol) => {
    // items: [{row, text}] consecutive rows starting at row 0 of column startCol
    await gotoCell(L.colName(startCol) + "1");
    for (const it of items) {
      if (mode === "bar") {
        await page.locator(".fortune-fx-input").first().click();
        await page.keyboard.type(it.text);
        await page.keyboard.press("Enter");
      } else {
        await page.keyboard.type(it.text);
        await page.keyboard.press("Enter");
      }
      await page.waitForTimeout(40);
    }
  };
  const settle = async () => {
    let last = Date.now();
    const h = (r) => { if (r.url().includes("/recalc")) last = Date.now(); };
    page.on("response", h);
    await page.waitForTimeout(2500);
    while (Date.now() - last < 3000) await page.waitForTimeout(500);
    page.off("response", h);
  };
  const read = () => page.evaluate(([col, n]) => {
    const wb = window.__adrientSheet.wb(); const d = wb.getAllSheets()[0].data || [];
    const out = [];
    for (let r = 0; r < n; r++) { const c = d[r] && d[r][col]; out.push(c ? { v: c.v, m: c.m, f: c.f } : null); }
    return out;
  }, [formulaCol, cases.length]);
  const txt = (c) => "=" + (mode === "lower" ? c.formula.replace(/"[^"]*"|[A-Za-z][A-Za-z0-9._]*(?=\()/g, (s) => (s[0] === '"' ? s : s.toLowerCase())) : c.formula);
  const id = await open("fn probe " + mode); log("opened");
  await typeFormulas(cases.map((c) => ({ text: txt(c) })), formulaCol);
  await settle();
  log("typed+settled"); let disp = await read(); log(JSON.stringify(disp).slice(0, 1500));
  const judge = (c, d) => {
    const eng = engine[c.name];
    if (!d || d.v == null && d.m == null) return "empty";
    const m = String(d.m ?? d.v);
    if (L.isErr(m) && !(eng && L.isErr(String(eng.m)))) return "error " + m;
    if (VOLATILE.test(c.name)) return "ok";
    return sameVal(d, eng) ? "ok" : `differs shown=${m} engine=${eng && eng.m}`;
  };
  const first = cases.map((c, i) => judge(c, disp[i]));
  // Retry failures alone (spills in the shared column may have blocked each other).
  const failed = first.map((s, i) => (s === "ok" ? -1 : i)).filter((i) => i >= 0);
  const final = first.slice();
  if (failed.length) {
    await page.request.delete(`${L.base}/api/v1/sheets/d/${id}`);
    for (const i of failed) {
      const sid = await open("fn retry");
      await typeFormulas([{ text: txt(cases[i]) }], formulaCol);
      await settle();
      await page.waitForFunction(() => window.__adrientSheet && window.__adrientSheet.wb(), null, { timeout: 20000 }).catch(async () => { await page.screenshot({ path: reportF + '.png' }); errors.push('editor gone after ' + cases[i].name); });
      const d = (await page.evaluate((col) => { const w = window.__adrientSheet.wb(); const c = w && w.getAllSheets()[0].data[0][col]; return c ? { v: c.v, m: c.m } : null; }, formulaCol));
      final[i] = judge(cases[i], d);
      await page.request.delete(`${L.base}/api/v1/sheets/d/${sid}`);
    }
  } else await page.request.delete(`${L.base}/api/v1/sheets/d/${id}`);
  cases.forEach((c, i) => report.results.push({ name: c.name, formula: c.formula, shared: first[i], final: final[i], shown: disp[i] && (disp[i].m ?? disp[i].v) }));
  fs.writeFileSync(reportF, JSON.stringify(report, null, 1));
  const ok1 = first.filter((s) => s === "ok").length, ok2 = final.filter((s) => s === "ok").length;
  console.log(`mode=${mode} tested=${cases.length} ok_first_pass=${ok1} ok_after_isolated_retry=${ok2} pageErrors=${errors.length}`);
  cases.forEach((c, i) => final[i] !== "ok" && console.log(c.name.padEnd(20), final[i], "|", c.formula));
  errors.slice(0, 5).forEach((e) => console.log("PAGEERROR", e));
  await browser.close();
})();
