import { PDFDocument } from "pdf-lib";

/**
 * Parses a page range like "1-3, 5, 8-" into 0-based page indexes in the order
 * written (duplicates removed). Open-ended ranges run to the last page.
 * Throws an Error with a plain message for anything it cannot read.
 */
export function parseRanges(spec: string, pageCount: number): number[] {
  const out: number[] = [];
  const seen = new Set<number>();
  const parts = spec.split(",").map((p) => p.trim()).filter(Boolean);
  if (parts.length === 0) throw new Error("Enter at least one page or range, such as 1-3, 5.");
  for (const part of parts) {
    const m = /^(\d+)?\s*-\s*(\d+)?$/.exec(part) ?? /^(\d+)$/.exec(part);
    if (!m) throw new Error(`"${part}" is not a page or range.`);
    let from: number;
    let to: number;
    if (m.length === 2) {
      from = to = Number(m[1]);
    } else {
      if (m[1] === undefined && m[2] === undefined) throw new Error(`"${part}" is not a page or range.`);
      from = m[1] === undefined ? 1 : Number(m[1]);
      to = m[2] === undefined ? pageCount : Number(m[2]);
    }
    if (from < 1 || to < 1 || from > pageCount || to > pageCount) throw new Error(`Page ${from > pageCount || from < 1 ? from : to} is outside this document (1 to ${pageCount}).`);
    if (to < from) throw new Error(`"${part}" goes backwards.`);
    for (let p = from; p <= to; p++) {
      if (!seen.has(p - 1)) {
        seen.add(p - 1);
        out.push(p - 1);
      }
    }
  }
  return out;
}

/** Groups of pages for splitting: each comma separated range becomes one file. */
export function splitGroups(spec: string, pageCount: number): number[][] {
  const groups = spec.split(";").flatMap((g) => g.split(",")).map((g) => g.trim()).filter(Boolean);
  if (groups.length === 0) throw new Error("Enter at least one page or range, such as 1-3, 4-6.");
  return groups.map((g) => parseRanges(g, pageCount));
}

/** Every N pages become one file: [[0,1],[2,3],[4]]. */
export function chunkPages(pageCount: number, every: number): number[][] {
  if (!Number.isInteger(every) || every < 1) throw new Error("Pages per file must be a whole number of 1 or more.");
  const out: number[][] = [];
  for (let i = 0; i < pageCount; i += every) out.push(Array.from({ length: Math.min(every, pageCount - i) }, (_, k) => i + k));
  return out;
}

export async function pageCountOf(bytes: Uint8Array): Promise<number> {
  return (await PDFDocument.load(bytes, { ignoreEncryption: false })).getPageCount();
}

/** A new PDF made of the chosen pages of `src`, in the order given. */
export async function extractPages(src: Uint8Array, pages: number[]): Promise<Uint8Array> {
  const doc = await PDFDocument.load(src);
  const out = await PDFDocument.create();
  const copied = await out.copyPages(doc, pages);
  copied.forEach((p) => out.addPage(p));
  return out.save();
}

/** Re-saves a PDF with compact object streams and, optionally, without document metadata. Text stays selectable. */
export async function optimizePdf(src: Uint8Array, stripMetadata: boolean): Promise<{ bytes: Uint8Array; before: number; after: number }> {
  const doc = await PDFDocument.load(src);
  if (stripMetadata) {
    doc.setTitle("");
    doc.setAuthor("");
    doc.setSubject("");
    doc.setKeywords([]);
    doc.setCreator("");
    doc.setProducer("");
  }
  const bytes = await doc.save({ useObjectStreams: true, addDefaultPage: false });
  return { bytes, before: src.byteLength, after: bytes.byteLength };
}

/** Builds a PDF whose pages are the given images (JPEG or PNG), each sized to its source page. */
export async function imagesToPdf(pages: { bytes: Uint8Array; type: "jpeg" | "png"; width: number; height: number }[]): Promise<Uint8Array> {
  const out = await PDFDocument.create();
  for (const p of pages) {
    const img = p.type === "jpeg" ? await out.embedJpg(p.bytes) : await out.embedPng(p.bytes);
    const page = out.addPage([p.width, p.height]);
    page.drawImage(img, { x: 0, y: 0, width: p.width, height: p.height });
  }
  return out.save({ useObjectStreams: true });
}

export function humanSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(2)} MB`;
}

/** "-23%" style change, or "no change". */
export function sizeChange(before: number, after: number): string {
  if (before <= 0) return "";
  const pct = Math.round(((after - before) / before) * 100);
  if (pct === 0) return "no change";
  return `${pct > 0 ? "+" : ""}${pct}%`;
}

// ---- compare ----

export interface DiffPart {
  text: string;
  kind: "same" | "added" | "removed";
}

const tokenize = (s: string) => s.split(/\s+/).filter(Boolean);

/**
 * Word level difference between two texts (longest common subsequence).
 * For very large inputs it falls back to a line-by-line check so it never
 * freezes the page.
 */
export function diffWords(a: string, b: string, maxCells = 4_000_000): DiffPart[] {
  const x = tokenize(a);
  const y = tokenize(b);
  if (x.length * y.length > maxCells) return coarseDiff(x, y);
  const n = x.length;
  const m = y.length;
  const dp: Uint32Array[] = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) dp[i][j] = x[i] === y[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
  const out: DiffPart[] = [];
  const push = (text: string, kind: DiffPart["kind"]) => {
    const last = out[out.length - 1];
    if (last && last.kind === kind) last.text += " " + text;
    else out.push({ text, kind });
  };
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (x[i] === y[j]) {
      push(x[i], "same");
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) push(x[i++], "removed");
    else push(y[j++], "added");
  }
  while (i < n) push(x[i++], "removed");
  while (j < m) push(y[j++], "added");
  return out;
}

function coarseDiff(x: string[], y: string[]): DiffPart[] {
  const out: DiffPart[] = [];
  const len = Math.max(x.length, y.length);
  const block = 50;
  for (let i = 0; i < len; i += block) {
    const a = x.slice(i, i + block).join(" ");
    const b = y.slice(i, i + block).join(" ");
    if (a === b) out.push({ text: a, kind: "same" });
    else {
      if (a) out.push({ text: a, kind: "removed" });
      if (b) out.push({ text: b, kind: "added" });
    }
  }
  return out;
}

export function diffSummary(parts: DiffPart[]): { added: number; removed: number; same: number } {
  const count = (k: DiffPart["kind"]) => parts.filter((p) => p.kind === k).reduce((n, p) => n + tokenize(p.text).length, 0);
  return { added: count("added"), removed: count("removed"), same: count("same") };
}
