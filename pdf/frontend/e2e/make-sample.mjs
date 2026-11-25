// Writes a small two-page sample PDF (text lines, a form field) and a PNG
// for the editor browser run: node e2e/make-sample.mjs <out-dir>
import { PDFDocument, StandardFonts, rgb } from "pdf-lib";
import fontkit from "@pdf-lib/fontkit";
import { readFileSync, writeFileSync } from "node:fs";
import { deflateSync } from "node:zlib";

const out = process.argv[2] || ".";
const doc = await PDFDocument.create();
const helv = await doc.embedFont(StandardFonts.Helvetica);
const bold = await doc.embedFont(StandardFonts.HelveticaBold);
doc.registerFontkit(fontkit);
// Embedded whole, so the program keeps its Unicode cmap (subset programs
// written without a cmap fall back to a standard font when edited).
const noto = await doc.embedFont(readFileSync(new URL("../src/features/editor/fonts/NotoSans-Regular.ttf", import.meta.url)), { subset: false });
const p1 = doc.addPage([612, 792]);
// A coloured line in an embedded TrueType font.
p1.drawText("Reference number QX-42 applies.", { x: 72, y: 600, size: 13, font: noto, color: rgb(0.8, 0.1, 0.1) });
p1.drawText("Service agreement", { x: 72, y: 720, size: 22, font: bold });
p1.drawText("The monthly fee is 1,200 rupees per seat.", { x: 72, y: 660, size: 14, font: helv });
p1.drawText("Payment is due within thirty days.", { x: 72, y: 630, size: 14, font: helv });
p1.drawText("Client name:", { x: 72, y: 560, size: 12, font: helv });
const form = doc.getForm();
const tf = form.createTextField("client_name");
tf.addToPage(p1, { x: 160, y: 552, width: 220, height: 22 });
const p2 = doc.addPage([612, 792]);
p2.drawText("Second page", { x: 72, y: 720, size: 22, font: bold });
p2.drawText("Signatures follow below.", { x: 72, y: 660, size: 14, font: helv, color: rgb(0.1, 0.1, 0.1) });
writeFileSync(`${out}/sample.pdf`, await doc.save());

// 64x40 solid teal PNG.
function png(w, h, [r, g, b]) {
  const crcTable = Array.from({ length: 256 }, (_, n) => {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    return c >>> 0;
  });
  const crc = (buf) => {
    let c = 0xffffffff;
    for (const x of buf) c = crcTable[(c ^ x) & 0xff] ^ (c >>> 8);
    return (c ^ 0xffffffff) >>> 0;
  };
  const chunk = (type, data) => {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const td = Buffer.concat([Buffer.from(type), data]);
    const c = Buffer.alloc(4);
    c.writeUInt32BE(crc(td));
    return Buffer.concat([len, td, c]);
  };
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr[8] = 8;
  ihdr[9] = 2;
  const raw = Buffer.alloc((w * 3 + 1) * h);
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) raw.set([r, g, b], y * (w * 3 + 1) + 1 + x * 3);
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk("IHDR", ihdr), chunk("IDAT", deflateSync(raw)), chunk("IEND", Buffer.alloc(0))]);
}
writeFileSync(`${out}/logo.png`, png(64, 40, [18, 108, 90]));
console.log("ok");
