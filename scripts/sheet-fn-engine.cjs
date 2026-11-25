// Engine pass: each catalog function alone in a workbook through /recalc.
// node scripts/sheet-fn-engine.cjs cases.json out.json   (PW=… in the environment)
const L = require("./sheet-fn-lib.cjs");
const fs = require("fs");
(async () => {
  const { cases, fixture, formulaCol } = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
  const { browser, page } = await L.launch();
  const id = await L.createSheet(page, "fn engine pass");
  const res = {};
  for (const c of cases) {
    const cells = await L.recalc(page, id, L.workbook(fixture, [{ r: 0, c: formulaCol, f: c.formula }]));
    const top = cells.find((x) => x.r === 0 && x.c === formulaCol);
    res[c.name] = { formula: c.formula, m: top ? top.m : null, v: top ? top.v : null, n: cells.length };
  }
  await page.request.delete(`${L.base}/api/v1/sheets/d/${id}`);
  fs.writeFileSync(process.argv[3], JSON.stringify(res, null, 1));
  const bad = Object.entries(res).filter(([, r]) => r.m == null || L.isErr(r.m));
  console.log("total", cases.length, "errors", bad.length);
  for (const [n, r] of bad) console.log(n.padEnd(22), String(r.m).padEnd(10), r.formula);
  await browser.close();
})();
