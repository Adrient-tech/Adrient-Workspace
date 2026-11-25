// Real-browser run of the PDF editor against a running workspace:
// open a sample, edit a line in place, add an image, sign, highlight, fill a
// form field, reorder pages, download, then re-open the download and check
// the changes stuck. Env: ADRIENT_URL, STATE (storage state JSON with a
// signed-in session), CHROMIUM_PATH, PLAYWRIGHT_CORE, OUT (dir with
// sample.pdf + logo.png; screenshots land there), VP=phone for 390x844.
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";
import { readFileSync } from "node:fs";
import * as pdfjs from "pdfjs-dist/legacy/build/pdf.mjs";
import { PDFDocument } from "pdf-lib";

const BASE = (process.env.ADRIENT_URL || "http://workspace.localtest.me:8080").replace(/\/$/, "");
const OUT = process.env.OUT;
const phone = process.env.VP === "phone";
const tag = phone ? "phone" : "desk";
const req = createRequire(pathToFileURL(process.env.PLAYWRIGHT_CORE + "/").href);
const { chromium } = req(process.env.PLAYWRIGHT_CORE);

const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH, args: ["--no-sandbox"] });
const context = await browser.newContext({
  viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
  storageState: process.env.STATE,
  acceptDownloads: true,
  colorScheme: "light",
});
const page = await context.newPage();
const errors = [];
page.on("pageerror", (e) => errors.push(e.message));
page.on("console", (m) => {
  if (m.type() === "warning" && m.text().startsWith("edit text")) console.log("  console:", m.text());
});
const step = async (name, fn) => {
  try {
    await fn();
    console.log("PASS", name);
  } catch (e) {
    console.log("FAIL", name, String(e).split("\n")[0]);
    await page.screenshot({ path: `${OUT}/${tag}-fail-${name}.png` });
  }
};
const shot = (n) => page.screenshot({ path: `${OUT}/${tag}-${n}.png` });
const canvasBox = async () => (await page.locator(".react-pdf__Page canvas").first().boundingBox());
// Click a point given in PDF points (612x792 page, origin bottom-left).
const clickPt = async (x, y) => {
  const c = page.locator(".react-pdf__Page canvas").first();
  await c.scrollIntoViewIfNeeded();
  const b0 = await canvasBox();
  // Bring the target point into the viewport (phone pages are taller than the screen).
  const vh = page.viewportSize().height;
  const ty = b0.y + (1 - y / 792) * b0.height;
  if (ty < 60 || ty > vh - 90) await page.mouse.wheel(0, ty - vh / 2);
  await page.waitForTimeout(200);
  const b = await canvasBox();
  await page.mouse.click(b.x + (x / 612) * b.width, b.y + (1 - y / 792) * b.height);
};
const tool = async (id) => {
  const t = page.getByTestId(id).first();
  if (!(await t.isVisible())) {
    const toggle = page.getByTestId("tools-toggle");
    if (await toggle.count()) await toggle.first().click();
  }
  await t.click();
  // Phone: the sheet closes itself for drawing tools; close it for the rest.
  await page.waitForTimeout(150);
  if (phone && (await page.getByTestId("tools-sheet").isVisible())) await page.getByTestId("tools-toggle").click();
};

await page.goto(`${BASE}/pdf/editor`);
await step("open", async () => {
  await page.getByTestId("editor-file-input").setInputFiles(`${OUT}/sample.pdf`);
  await page.locator(".react-pdf__Page canvas").first().waitFor({ timeout: 20000 });
  await page.waitForTimeout(1200);
  await shot("1-open");
});
await step("edit-text-in-place", async () => {
  await tool("tool-edittext");
  await clickPt(150, 664);
  const ta = page.getByTestId("editor-textarea");
  await ta.waitFor({ timeout: 8000 });
  const v = await ta.inputValue();
  if (!v.includes("monthly fee")) throw new Error("textarea got: " + v);
  await ta.fill("The monthly fee is 950 rupees per seat.");
  await shot("2-editing");
  await page.keyboard.press("Escape");
});
// Edit the red line set in an embedded font: colour and font must be kept.
const editEmbedded = async (newText) => {
  await tool("tool-edittext");
  await clickPt(130, 604);
  const ta = page.getByTestId("editor-textarea");
  await ta.waitFor({ timeout: 8000 });
  const v = await ta.inputValue();
  if (!v.includes("QX-")) throw new Error("textarea got: " + v);
  const st = await ta.evaluate((el) => ({ color: getComputedStyle(el).color, font: getComputedStyle(el).fontFamily }));
  if (newText) await ta.fill(newText);
  await page.keyboard.press("Escape");
  return { value: v, ...st };
};
await step("edit-embedded-font-line", async () => {
  const st = await editEmbedded("Reference number QX-24 applies.");
  console.log("  style while editing:", st.color, "|", st.font.split(",")[0]);
  if (st.color !== "rgb(204, 26, 26)") throw new Error("colour not kept: " + st.color);
  if (!st.font.includes("pdfdoc-")) throw new Error("embedded font not reused: " + st.font);
});
await step("add-image", async () => {
  await tool("tool-image");
  const chooser = page.waitForEvent("filechooser");
  await clickPt(420, 720);
  await (await chooser).setFiles(`${OUT}/logo.png`);
  await page.waitForTimeout(600);
});
await step("sign", async () => {
  await tool("tool-signature");
  const dlg = page.getByTestId("sig-dialog");
  if (await dlg.isVisible().catch(() => false)) {
    if (await page.getByTestId("sig-tab-type").isVisible()) await page.getByTestId("sig-tab-type").click();
    await page.getByTestId("sig-type-input").fill("Asha Rao");
    await page.getByTestId("sig-confirm").click();
  }
  await page.waitForTimeout(500);
  await clickPt(300, 480);
  await page.waitForTimeout(500);
});
await step("highlight", async () => {
  await tool("tool-highlight");
  await page.locator(".react-pdf__Page canvas").first().scrollIntoViewIfNeeded();
  const b = await canvasBox();
  const sx = b.x + (72 / 612) * b.width, sy = b.y + (1 - 640 / 792) * b.height;
  await page.mouse.move(sx, sy);
  await page.mouse.down();
  await page.mouse.move(sx + 0.4 * b.width, sy - 0.02 * b.height, { steps: 6 });
  await page.mouse.up();
  await tool("tool-select");
});
await step("fill-form", async () => {
  const input = page.locator('[data-testid^="field-input-"]').first();
  await input.waitFor({ timeout: 5000 });
  await input.fill("Asha Rao");
});
await shot("3-annotated");
await step("reorder-pages", async () => {
  await tool("page-down");
  await page.waitForTimeout(1500);
});
let downloaded = null;
await step("download", async () => {
  const dl = page.waitForEvent("download", { timeout: 30000 });
  await page.getByTestId("editor-download").first().click();
  const d = await dl;
  downloaded = `${OUT}/${tag}-out.pdf`;
  await d.saveAs(downloaded);
});
await shot("4-after-download");

if (downloaded) {
  const bytes = new Uint8Array(readFileSync(downloaded));
  const doc = await pdfjs.getDocument({ data: bytes.slice() }).promise;
  const texts = [];
  for (let i = 1; i <= doc.numPages; i++) {
    const tc = await (await doc.getPage(i)).getTextContent();
    texts.push(tc.items.map((it) => it.str).join(" "));
  }
  const check = (name, ok) => console.log(ok ? "PASS" : "FAIL", name);
  check("reopen: pages reordered (page 1 is the second page)", texts[0]?.includes("Second page"));
  check("reopen: edited line present", texts[1]?.includes("950 rupees"));
  check("reopen: old line removed from the file", !texts.join(" ").includes("1,200"));
  check("reopen: other text kept", texts[1]?.includes("Payment is due"));
  check("reopen: embedded-font line edited", texts[1]?.includes("QX-24") && !texts[1]?.includes("QX-42"));
  {
    // The new text is drawn in the document's own font, not a standard one.
    const pg = await doc.getPage(2);
    await pg.getOperatorList();
    const tc = await pg.getTextContent();
    const it = tc.items.find((x) => x.str && x.str.includes("QX-24"));
    const fname = it && pg.commonObjs.has(it.fontName) ? pg.commonObjs.get(it.fontName).name : "";
    check("reopen: edited line drawn in the document's embedded font (" + fname + ")", /NotoSans/.test(fname));
  }
  const lib = await PDFDocument.load(bytes);
  const raw = Buffer.from(bytes).toString("latin1");
  check("reopen: images embedded (logo + signature)", (raw.match(/\/Subtype\s*\/Image/g) || []).length >= 2);
  let formValue = "";
  try {
    formValue = lib.getForm().getTextField("client_name").getText() ?? "";
  } catch {
    formValue = texts.join(" ").includes("Asha Rao") ? "Asha Rao (flattened)" : "";
  }
  check("reopen: form value kept (" + formValue + ")", formValue.includes("Asha Rao"));
  // Render the re-opened file in the browser for a visual check.
  await page.goto(`${BASE}/pdf/editor`);
  await page.getByTestId("editor-file-input").setInputFiles(downloaded);
  await page.locator(".react-pdf__Page canvas").first().waitFor({ timeout: 20000 });
  await page.getByTestId("editor-next-page").first().click().catch(() => {});
  await page.waitForTimeout(1500);
  await shot("5-reopened");
  // The downloaded line still carries its colour and the document's font.
  try {
    const st = await editEmbedded(null);
    check("reopen: edited line colour kept (" + st.color + ")", st.color === "rgb(204, 26, 26)");

  } catch (e) {
    check("reopen: style check " + String(e).split("\n")[0], false);
  }
}
if (errors.length) console.log("page errors:", errors.slice(0, 5).join(" | "));
await browser.close();
