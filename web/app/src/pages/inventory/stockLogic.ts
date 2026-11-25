// Pure helpers for the stock and purchasing screens (no React, no network).
import { barcodeSvg, isValidEan13 } from "./logic";

// ------------------------------------------------------------- Code 128

const C128 = [
  "11011001100", "11001101100", "11001100110", "10010011000", "10010001100", "10001001100", "10011001000", "10011000100", "10001100100", "11001001000",
  "11001000100", "11000100100", "10110011100", "10011011100", "10011001110", "10111001100", "10011101100", "10011100110", "11001110010", "11001011100",
  "11001001110", "11011100100", "11001110100", "11101101110", "11101001100", "11100101100", "11100100110", "11101100100", "11100110100", "11100110010",
  "11011011000", "11011000110", "11000110110", "10100011000", "10001011000", "10001000110", "10110001000", "10001101000", "10001100010", "11010001000",
  "11000101000", "11000100010", "10110111000", "10110001110", "10001101110", "10111011000", "10111000110", "10001110110", "11101110110", "11010001110",
  "11000101110", "11011101000", "11011100010", "11011101110", "11101011000", "11101000110", "11100010110", "11101101000", "11101100010", "11100011010",
  "11101111010", "11001000010", "11110001010", "10100110000", "10100001100", "10010110000", "10010000110", "10000101100", "10000100110", "10110010000",
  "10110000100", "10011010000", "10011000010", "10000110100", "10000110010", "11000010010", "11001010000", "11110111010", "11000010100", "10001111010",
  "10100111100", "10010111100", "10010011110", "10111100100", "10011110100", "10011110010", "11110100100", "11110010100", "11110010010", "11011011110",
  "11011110110", "11110110110", "10101111000", "10100011110", "10001011110", "10111101000", "10111100010", "11110101000", "11110100010", "10111011110",
  "10111101110", "11101011110", "11110101110", "11010000100", "11010010000", "11010011100",
];
const C128_STOP = "1100011101011";

/** Code 128 (subset B, ASCII 32-126) as a "0"/"1" module string. Throws on other characters. */
export function code128Modules(text: string): string {
  if (text.length === 0) throw new Error("nothing to encode");
  const START_B = 104;
  const values: number[] = [START_B];
  let sum = START_B;
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i);
    if (c < 32 || c > 126) throw new Error("Code 128 labels support plain ASCII only");
    const v = c - 32;
    values.push(v);
    sum += v * (i + 1);
  }
  values.push(sum % 103);
  return values.map((v) => C128[v]).join("") + C128_STOP;
}

/** Barcode SVG for any label text: EAN-13 when valid, else Code 128. */
export function labelBarcodeSvg(code: string, height = 40): string {
  if (isValidEan13(code)) return barcodeSvg(code, height) ?? "";
  let bits: string;
  try {
    bits = code128Modules(code);
  } catch {
    return "";
  }
  const q = 10;
  let bars = "";
  for (let i = 0; i < bits.length; ) {
    if (bits[i] === "1") {
      let j = i;
      while (j < bits.length && bits[j] === "1") j++;
      bars += `<rect x="${i + q}" y="0" width="${j - i}" height="${height}"/>`;
      i = j;
    } else i++;
  }
  const w = bits.length + q * 2;
  const safe = code.replace(/&/g, "&amp;").replace(/</g, "&lt;");
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${height + 14}" preserveAspectRatio="none" width="100%" height="100%"><rect width="${w}" height="${height + 14}" fill="#fff"/><g fill="#000">${bars}</g><text x="${w / 2}" y="${height + 11}" font-family="monospace" font-size="10" text-anchor="middle">${safe}</text></svg>`;
}

// ------------------------------------------------------------ label sheets

export interface LabelPreset {
  id: string;
  name: string;
  pageW: number; // mm
  pageH: number;
  cols: number;
  rows: number;
  labelW: number;
  labelH: number;
  marginLeft: number;
  marginTop: number;
  gapX: number;
  gapY: number;
}

export const LABEL_PRESETS: LabelPreset[] = [
  { id: "a4-65", name: "A4 sheet, 65 labels (38.1 × 21.2 mm)", pageW: 210, pageH: 297, cols: 5, rows: 13, labelW: 38.1, labelH: 21.2, marginLeft: 4.7, marginTop: 10.7, gapX: 2.5, gapY: 0 },
  { id: "a4-24", name: "A4 sheet, 24 labels (63.5 × 33.9 mm)", pageW: 210, pageH: 297, cols: 3, rows: 8, labelW: 63.5, labelH: 33.9, marginLeft: 7.2, marginTop: 13, gapX: 2.5, gapY: 0 },
  { id: "a4-14", name: "A4 sheet, 14 labels (99.1 × 38.1 mm)", pageW: 210, pageH: 297, cols: 2, rows: 7, labelW: 99.1, labelH: 38.1, marginLeft: 4.7, marginTop: 15.1, gapX: 2.5, gapY: 0 },
  { id: "roll-50x25", name: "Label roll, 50 × 25 mm", pageW: 50, pageH: 25, cols: 1, rows: 1, labelW: 50, labelH: 25, marginLeft: 0, marginTop: 0, gapX: 0, gapY: 0 },
  { id: "roll-100x50", name: "Label roll, 100 × 50 mm", pageW: 100, pageH: 50, cols: 1, rows: 1, labelW: 100, labelH: 50, marginLeft: 0, marginTop: 0, gapX: 0, gapY: 0 },
];

export interface LabelSlot {
  page: number;
  x: number; // mm from the page's left edge
  y: number;
}

/** Positions of `count` labels, starting at slot `startAt` (0-based) on the first sheet. */
export function labelSlots(p: LabelPreset, count: number, startAt = 0): LabelSlot[] {
  const per = p.cols * p.rows;
  const out: LabelSlot[] = [];
  for (let i = 0; i < count; i++) {
    const n = startAt + i;
    const page = Math.floor(n / per);
    const k = n % per;
    const row = Math.floor(k / p.cols);
    const col = k % p.cols;
    out.push({
      page,
      x: round2(p.marginLeft + col * (p.labelW + p.gapX)),
      y: round2(p.marginTop + row * (p.labelH + p.gapY)),
    });
  }
  return out;
}

export function sheetCount(p: LabelPreset, count: number, startAt = 0): number {
  return count <= 0 ? 0 : Math.floor((startAt + count - 1) / (p.cols * p.rows)) + 1;
}

const round2 = (n: number) => Math.round(n * 100) / 100;

// ------------------------------------------------------- purchase orders

export type PoState = "draft" | "approved" | "sent" | "acknowledged" | "partial" | "received" | "closed" | "cancelled";

export const PO_LABEL: Record<PoState, string> = {
  draft: "Draft",
  approved: "Approved",
  sent: "Sent",
  acknowledged: "Acknowledged",
  partial: "Partly received",
  received: "Received",
  closed: "Closed",
  cancelled: "Cancelled",
};

export const PO_TONE: Record<PoState, "neutral" | "primary" | "success" | "warning" | "danger"> = {
  draft: "neutral",
  approved: "primary",
  sent: "primary",
  acknowledged: "primary",
  partial: "warning",
  received: "success",
  closed: "neutral",
  cancelled: "danger",
};

export type PoAction = "edit" | "approve" | "send" | "acknowledge" | "receive" | "bill" | "close" | "cancel";

/** Which buttons a purchase order offers in each state (the server enforces the same rules). */
export function poActions(state: PoState, received: number): PoAction[] {
  switch (state) {
    case "draft":
      return ["edit", "approve", "cancel"];
    case "approved":
      return ["send", "receive", "acknowledge", "cancel"];
    case "sent":
      return ["send", "acknowledge", "receive", "cancel"];
    case "acknowledged":
      return ["receive", "cancel"];
    case "partial":
      return ["receive", "bill", "close"];
    case "received":
      return ["bill", "close"];
    default:
      return received > 0 ? ["bill"] : [];
  }
}

export interface LineLike {
  qty: number;
  unit_cost: number;
  tax_pct: number;
}

export function lineTotals(lines: LineLike[]): { subtotal: number; tax: number; total: number } {
  let sub = 0;
  let tax = 0;
  for (const l of lines) {
    const amt = Math.round(l.qty * l.unit_cost * 100) / 100;
    sub += amt;
    tax += Math.round(amt * l.tax_pct) / 100;
  }
  sub = Math.round(sub * 100) / 100;
  tax = Math.round(tax * 100) / 100;
  return { subtotal: sub, tax, total: Math.round((sub + tax) * 100) / 100 };
}

/** Most that may still be received on a line, given the over-receipt tolerance (percent). */
export function receiptLimit(ordered: number, received: number, overPct: number): number {
  return Math.max(0, Math.round((ordered * (1 + overPct / 100) - received) * 1000) / 1000);
}

// ------------------------------------------------------------ lots & expiry

export function expiryTone(daysLeft: number | null | undefined, recalled = false): "danger" | "warning" | "success" | "neutral" {
  if (recalled) return "danger";
  if (daysLeft === null || daysLeft === undefined) return "neutral";
  if (daysLeft < 0) return "danger";
  if (daysLeft <= 30) return "warning";
  return "success";
}

export function expiryText(daysLeft: number | null | undefined): string {
  if (daysLeft === null || daysLeft === undefined) return "No expiry";
  if (daysLeft < 0) return `Expired ${-daysLeft} day${daysLeft === -1 ? "" : "s"} ago`;
  if (daysLeft === 0) return "Expires today";
  return `${daysLeft} day${daysLeft === 1 ? "" : "s"} left`;
}

// ------------------------------------------------------------------ counts

export function countVariance(expected: number, counted: number | null, unitCost: number): { qty: number; value: number } {
  if (counted === null) return { qty: 0, value: 0 };
  const qty = Math.round((counted - expected) * 1000) / 1000;
  return { qty, value: Math.round(qty * unitCost * 100) / 100 };
}

// -------------------------------------------------------------- scanning

/** Cleans scanner/keyboard input: trims whitespace and control characters. */
export function cleanScan(raw: string): string {
  // eslint-disable-next-line no-control-regex
  return raw.replace(/[\u0000-\u001f\u007f]/g, "").trim();
}

export interface ScanLine {
  key: string; // product id (or po line id)
  label: string;
  qty: number;
}

/** Adds one unit for a scanned key, creating the line on first scan. Returns a new array. */
export function addScan(lines: ScanLine[], key: string, label: string, by = 1): ScanLine[] {
  const i = lines.findIndex((l) => l.key === key);
  if (i < 0) return [...lines, { key, label, qty: by }];
  const next = lines.slice();
  next[i] = { ...next[i], qty: Math.round((next[i].qty + by) * 1000) / 1000 };
  return next;
}

export function daysCoverText(d: number): string {
  if (d < 0) return "No recent sales";
  if (d === 0) return "Out of stock";
  if (d >= 365) return "1 year +";
  return `${Math.round(d)} days`;
}
