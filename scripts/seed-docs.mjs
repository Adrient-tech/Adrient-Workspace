#!/usr/bin/env node
// Writes the demo Docs bodies. Docs keeps live-edit (Yjs) state that cannot be
// written from outside the editor, so this opens each demo document in a real
// browser and hands the editor its content through the same sessionStorage
// seed ("docseed:<id>") the template gallery uses. Run after `adrient-admin
// seed-demo`. Safe to re-run: documents that already have text are skipped.
//
//   ADRIENT_LOGIN=admin ADRIENT_PASSWORD=... node scripts/seed-docs.mjs
import { BASE, launch, signIn } from "./lib-browser.mjs";

const th = "style='border:1px solid #bfc9c6;padding:6px 10px;background:#d7efe8;text-align:left'";
const td = "style='border:1px solid #bfc9c6;padding:6px 10px'";
const table = (head, rows) =>
  `<table><tbody><tr>${head.map((h) => `<th ${th}><p><strong>${h}</strong></p></th>`).join("")}</tr>` +
  rows.map((r) => `<tr>${r.map((c) => `<td ${td}><p>${c}</p></td>`).join("")}</tr>`).join("") + "</tbody></table>";
const ul = (xs) => `<ul>${xs.map((x) => `<li><p>${x}</p></li>`).join("")}</ul>`;
const ol = (xs) => `<ol>${xs.map((x) => `<li><p>${x}</p></li>`).join("")}</ol>`;

const DOCS = {
  "Meeting notes: Quarterly business review":
    `<h1>Quarterly business review</h1><p><strong>Date:</strong> Friday 20 November 2026, 11:00 to 13:00<br><strong>Location:</strong> Board room and Adrient Meet<br><strong>Attendees:</strong> Asha Menon, Rohan Deshpande, Priya Nair, Imran Sheikh, Neha Kapoor, Divya Iyer</p>` +
    `<h2>Agenda</h2>${ol(["Q3 results", "Pipeline and renewals", "Discounting policy", "Hiring plan", "Q4 priorities"])}` +
    `<h2>Q3 results</h2><p>Revenue closed at 112 percent of target, driven by Indore and Pune. Gross margin was 31.2 percent, 1.2 points above plan.</p>` +
    table(["Metric", "Target", "Actual", "Status"], [["Revenue (INR lakh)", "166", "186", "Ahead"], ["Gross margin", "30%", "31.2%", "Ahead"], ["New customers", "40", "37", "At risk"], ["Receivables over 60 days", "3 or fewer", "3", "On track"]]) +
    `<h2>Decisions</h2>${ul(["Cap discounts at 8 percent without approval from the Head of Sales.", "Move new suppliers to 30-day payment terms from December.", "Run a 4-week last-mile pilot with Coromandel Logistics in Pune."])}` +
    `<h2>Action items</h2>` + table(["Owner", "Action", "Due"], [["Rohan Deshpande", "Share the renewal pipeline and discount proposal", "25 Nov"], ["Priya Nair", "Send October MIS and receivables ageing", "26 Nov"], ["Karan Malhotra", "Agree pilot volumes with Coromandel", "30 Nov"], ["Neha Kapoor", "Finish the onboarding plan for Nikhil Hegde", "23 Nov"]]) +
    `<p><em>Next review: Friday 18 December.</em></p>`,
  "Project brief: Warehouse automation":
    `<h1>Warehouse automation rollout</h1><p><strong>Owner:</strong> Asha Menon, Head of Operations &nbsp;|&nbsp; <strong>Status:</strong> In progress &nbsp;|&nbsp; <strong>Target:</strong> go-live 14 December 2026</p>` +
    `<h2>Background</h2><p>Receiving and dispatch are manual. Cycle counts take two days and variance is found late. Barcode scanning and automatic reorder points will cut errors and give an up-to-date stock position across both warehouses.</p>` +
    `<h2>Goals</h2>${ul(["Cut dispatch time by 30 percent.", "Keep stock variance under 0.5 percent.", "Raise purchase orders automatically for the top 150 products."])}` +
    `<h2>Scope</h2>${ul(["In scope: bin labels, scanner app, reorder points, training for both shifts.", "Out of scope: new warehouse construction, vendor portal."])}` +
    `<h2>Milestones</h2>` + table(["Milestone", "Owner", "Due", "Status"], [["Bin locations mapped", "Arjun Patil", "30 Oct", "Done"], ["Pune store pilot", "Faisal Khan", "28 Nov", "In progress"], ["Reorder points live", "Karan Malhotra", "5 Dec", "In progress"], ["Training, shifts A and B", "Arjun Patil", "12 Dec", "Not started"], ["Go-live", "Asha Menon", "14 Dec", "Not started"]]) +
    `<h2>Budget</h2><p>Approved: INR 6.4 lakh. Scanners (INR 2.8 lakh), labels and printers (INR 1.1 lakh), integration work (INR 1.9 lakh), training and contingency (INR 0.6 lakh).</p>` +
    `<h2>Risks</h2>${ul(["Scanner delivery may slip, vendor contract is at risk.", "Shift B availability for training.", "Data quality of bin master data."])}`,
  "Policy: Supplier onboarding":
    `<h1>Supplier onboarding policy</h1><p><strong>Version 3 · Owner: Operations · Effective 1 December 2026</strong></p>` +
    `<h2>1. Purpose</h2><p>This policy makes sure every new supplier is verified, approved and set up in the same way before the first purchase order is raised.</p>` +
    `<h2>2. Who it applies to</h2><p>All employees who request, approve or pay suppliers, including store managers.</p>` +
    `<h2>3. Requirements before the first PO</h2>${ol(["GST registration and, where applicable, MSME certificate are verified on the government portals.", "A cancelled cheque or bank letter is collected and the account holder name is matched with the legal name.", "Two quotes are compared for orders above INR 50,000.", "A conflict of interest declaration is signed by the requester."])}` +
    `<h2>4. Approvals</h2>` + table(["Order value (INR)", "Approver"], [["Up to 50,000", "Procurement Manager"], ["50,001 to 5,00,000", "Head of Operations"], ["Above 5,00,000", "Head of Operations and Finance Controller"]]) +
    `<h2>5. Payment terms</h2><p>New suppliers are paid 30 days after receipt of goods and a correct invoice. Advance payments need written approval from the Finance Controller.</p>` +
    `<h2>6. Review</h2><p>New suppliers are reviewed after 30 days for quality, delivery and invoice accuracy. The policy is reviewed every year.</p>`,
  "Proposal: Coromandel Logistics partnership":
    `<h1>Proposal: last-mile delivery partnership</h1><p><em>Prepared for Adrient Trading by Coromandel Logistics · 12-month agreement</em></p>` +
    `<h2>Summary</h2><p>Coromandel Logistics will provide next-day last-mile delivery in Pune, Nashik and Indore, with a guaranteed slot for Adrient's top accounts and live tracking shared by link.</p>` +
    `<h2>What we offer</h2>${ul(["Next-day delivery for orders confirmed before 4 pm.", "Dedicated vehicles for the Pune store.", "Proof of delivery with photo and signature.", "Weekly reports on on-time delivery and damages."])}` +
    `<h2>Pricing</h2>` + table(["City", "Per parcel (INR)", "Monthly minimum (INR)"], [["Pune", "42", "60,000"], ["Nashik", "48", "35,000"], ["Indore", "51", "35,000"]]) +
    `<h2>Pilot</h2><p>We propose a 4-week pilot in Pune starting 1 December with no minimum. Success means 97 percent on-time delivery and fewer than 0.5 percent damaged parcels.</p>` +
    `<h2>Timeline</h2>${ol(["Week 0: onboarding and system integration", "Weeks 1 to 4: Pune pilot", "Week 5: review and decision", "Week 6 onwards: roll out to Nashik and Indore"])}` +
    `<h2>Next steps</h2><p>Confirm the pilot volumes with Karan Malhotra and sign the pilot letter by 27 November.</p>`,
};

const { browser, context } = await launch({ width: 1440, height: 900 });
const page = await context.newPage();
await signIn(page);
const list = await (await page.request.get(`${BASE}/api/v1/docs`)).json().catch(() => ({}));
const docs = list.documents || list.docs || list.items || [];
let done = 0;
for (const [title, html] of Object.entries(DOCS)) {
  const d = docs.find((x) => x.title === title);
  if (!d) { console.log("missing doc (run seed-demo first):", title); continue; }
  await page.goto(BASE + "/docs", { waitUntil: "domcontentloaded" });
  await page.evaluate(([id, h]) => sessionStorage.setItem(`docseed:${id}`, h), [d.id, html]);
  await page.reload({ waitUntil: "domcontentloaded" });
  await page.getByText(title, { exact: true }).first().click({ timeout: 15000 });
  const ed = page.locator("[data-testid='doc-editor'] .ProseMirror, .ProseMirror").first();
  await ed.waitFor({ timeout: 20000 });
  await page.waitForTimeout(3000);
  const text = (await ed.innerText()).trim();
  console.log(text.length > 80 ? "ok" : "EMPTY", title, text.length);
  if (text.length > 80) done++;
  await page.waitForTimeout(2500); // let the collab log persist
  // The home grid thumbnail is stored separately; set it from the same HTML.
  const pr = await page.request.put(`${BASE}/api/v1/docs/d/${d.id}/preview`, { data: { html: html.slice(0, 8000) } });
  if (!pr.ok()) console.log("preview failed", title, pr.status());
}
await browser.close();
if (done < Object.keys(DOCS).length) process.exitCode = 1;
