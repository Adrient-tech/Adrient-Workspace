#!/usr/bin/env node
// Product-tour screenshots: 1440x900 JPEGs (quality 84) with stable names, in tour order.
//
//   ADRIENT_LOGIN=admin ADRIENT_PASSWORD=... node scripts/screenshots.mjs [outDir] [--only name,name]
//
// Default outDir is docs/screenshots. See lib-browser.mjs for the environment.
import { mkdirSync } from "node:fs";
import path from "node:path";
import { BASE, launch, signIn } from "./lib-browser.mjs";

const args = process.argv.slice(2);
const only = (args.find((a) => a.startsWith("--only=")) || "").replace("--only=", "").split(",").filter(Boolean);
const out = args.find((a) => !a.startsWith("--")) || "docs/screenshots";
mkdirSync(out, { recursive: true });

// click: visible text of a nav button to press after load. open: text of the first card/row to open.
const tour = [
  { name: "home", url: "/" },
  { name: "apps", url: "/apps" },
  { name: "drive", url: "/drive" },
  { name: "mail", url: "/mail", openFirst: "[data-testid^='mail-thread'], [role='listitem'], li" },
  { name: "chat", url: "/chat", click: "general" },
  { name: "meet", url: "/meet" },
  { name: "calendar", url: "/calendar", scroll: 330 },
  { name: "contacts", url: "/contacts" },
  { name: "docs", url: "/docs" },
  { name: "docs-editor", url: "/docs", openDoc: "Meeting notes: Quarterly" },
  { name: "sheets", url: "/sheets" },
  { name: "sheets-editor", url: "/sheets", openDoc: "Budget FY" },
  { name: "slides", url: "/slides" },
  { name: "slides-editor", url: "/slides", openDoc: "Q3 results" },
  { name: "forms", url: "/forms" },
  { name: "keep", url: "/keep" },
  { name: "tasks", url: "/tasks", click: "Month-end checklist" },
  { name: "projects", url: "/projects" },
  { name: "tickets", url: "/tickets", click: "IT and facilities helpdesk" },
  { name: "crm-pipeline", url: "/crm", click: "Pipeline" },
  { name: "crm-deals", url: "/crm", click: ["Pipeline", "List"] },
  { name: "hr-dashboard", url: "/hr/dashboard" },
  { name: "hr-people", url: "/hr/people" },
  { name: "payroll", url: "/hr/payroll" },
  { name: "talent-pipeline", url: "/talent", click: "Pipeline" },
  { name: "accounting-overview", url: "/accounting" },
  { name: "accounting-reports", url: "/accounting", click: "Reports" },
  { name: "invoicing", url: "/invoicing" },
  { name: "inventory", url: "/inventory" },
  { name: "pos", url: "/pos" },
  { name: "admin-console", url: "/admin/dashboard" },
  { name: "admin-ai", url: "/admin/ai" },
  { name: "cloudimport", url: "/cloudimport" },
  // Operations and knowledge apps.
  { name: "shifts", url: "/shifts" },
  { name: "timetracking", url: "/timetracking" },
  { name: "fieldservice", url: "/fieldservice" },
  { name: "fleet", url: "/fleet" },
  { name: "records", url: "/records" },
  { name: "records-suppliers", url: "/records", openDoc: "Suppliers register" },
  { name: "records-launches", url: "/records", openDoc: "Product launch tracker" },
  { name: "wiki", url: "/wiki" },
  { name: "wiki-database", url: "/wiki", openDoc: "IT request log" },
  { name: "contracts", url: "/contracts" },
  { name: "contracts-all", url: "/contracts", click: "All contracts" },
  { name: "signatures", url: "/signatures" },
  { name: "legal", url: "/legal" },
  { name: "captable", url: "/captable" },
  { name: "signatures-detail", url: "/signatures", openDoc: "Mutual NDA with Kaveri Retail" },
];

const { browser, context } = await launch({ width: 1440, height: 900 });
const page = await context.newPage();
await signIn(page);
const failed = [];
for (const s of tour) {
  if (only.length && !only.includes(s.name)) continue;
  try {
    await page.goto(BASE + s.url, { waitUntil: "domcontentloaded" });
    await page.waitForLoadState("networkidle", { timeout: 15000 }).catch(() => {});
    for (const label of [].concat(s.click || [])) {
      const re = new RegExp("^\\s*" + label + "\\s*$", "i");
      const el = page.getByRole("button", { name: re }).first();
      const alt = page.getByText(re).first();
      await (await el.count() ? el : alt).click({ timeout: 8000 }).catch(() => {});
      await page.waitForTimeout(800);
    }
    if (s.scroll) { await page.mouse.move(800, 500); await page.mouse.wheel(0, s.scroll); await page.waitForTimeout(500); }
    await page.mouse.move(1439, 899); // no stray hover highlight
    if (s.openDoc) {
      await page.getByText(s.openDoc, { exact: false }).first().click({ timeout: 8000 });
      await page.waitForTimeout(2500);
    }
    await page.waitForLoadState("networkidle", { timeout: 10000 }).catch(() => {});
    await page.waitForTimeout(1200);
    await page.screenshot({ path: path.join(out, s.name + ".jpg"), type: "jpeg", quality: 84 });
    console.log("ok", s.name);
  } catch (e) {
    failed.push(s.name);
    console.log("FAIL", s.name, String(e).split("\n")[0]);
  }
}
await browser.close();
if (failed.length) process.exitCode = 1;
