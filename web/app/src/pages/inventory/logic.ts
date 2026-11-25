// Pure inventory logic: CSV, EAN-13 barcodes, stock status, reorder text.

import type { InvProduct, ProductInput, ReorderLine } from "./types";

// ---------------------------------------------------------------- CSV

/** RFC-4180 style parser: quotes, escaped quotes, CRLF, embedded newlines. */
export function parseCsv(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let quoted = false;
  const src = text.replace(/^\uFEFF/, "");
  for (let i = 0; i < src.length; i++) {
    const c = src[i];
    if (quoted) {
      if (c === '"') {
        if (src[i + 1] === '"') {
          cell += '"';
          i++;
        } else quoted = false;
      } else cell += c;
    } else if (c === '"') quoted = true;
    else if (c === ",") {
      row.push(cell);
      cell = "";
    } else if (c === "\n" || c === "\r") {
      if (c === "\r" && src[i + 1] === "\n") i++;
      row.push(cell);
      cell = "";
      rows.push(row);
      row = [];
    } else cell += c;
  }
  if (cell !== "" || row.length > 0) {
    row.push(cell);
    rows.push(row);
  }
  return rows.filter((r) => r.some((c) => c.trim() !== ""));
}

/** Quote a cell; also neutralise spreadsheet formula injection. */
export function csvCell(v: string | number | boolean | undefined | null): string {
  let s = v === undefined || v === null ? "" : String(v);
  if (/^[=+@]/.test(s) || (/^-/.test(s) && Number.isNaN(Number(s)))) s = `'${s}`;
  return /[",\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

export function toCsv(rows: (string | number | boolean | undefined | null)[][]): string {
  return rows.map((r) => r.map(csvCell).join(",")).join("\r\n") + "\r\n";
}

export const EXPORT_HEADER = [
  "Name",
  "SKU",
  "Barcode",
  "Category",
  "Unit",
  "Price",
  "Cost",
  "Min stock",
  "Reorder qty",
  "Location",
  "Quantity",
  "Shelf",
  "Description",
];

/** One row per product per location with stock (or one blank-location row). */
export function productsToCsv(products: InvProduct[]): string {
  const rows: (string | number)[][] = [EXPORT_HEADER];
  const flat: InvProduct[] = [];
  for (const p of products) {
    if (p.variants?.length) {
      for (const v of p.variants) flat.push({ ...v, name: `${p.name}${v.variant_label ? ` - ${v.variant_label}` : ""}`, category: v.category || p.category });
    } else flat.push(p);
  }
  for (const p of flat) {
    const base = [p.name, p.sku, p.barcode, p.category, p.unit, p.price, p.cost, p.min_stock, p.reorder_qty];
    if (p.stock.length === 0) rows.push([...base, "", 0, "", p.description]);
    for (const s of p.stock) rows.push([...base, s.location_name, s.qty, s.shelf_code, p.description]);
  }
  return toCsv(rows);
}

const ALIASES: Record<string, string[]> = {
  name: ["name", "product", "product name", "item", "title"],
  sku: ["sku", "code", "item code", "product code"],
  barcode: ["barcode", "ean", "upc", "gtin", "ean13"],
  category: ["category", "type", "group"],
  unit: ["unit", "uom", "unit of measure"],
  price: ["price", "selling price", "sell price", "retail"],
  cost: ["cost", "purchase cost", "buy price", "unit cost"],
  min_stock: ["min stock", "minimum", "min", "threshold", "low stock"],
  reorder_qty: ["reorder qty", "reorder quantity", "reorder"],
  location: ["location", "warehouse", "store"],
  quantity: ["quantity", "qty", "stock", "on hand", "in stock"],
  shelf: ["shelf", "bin", "shelf code"],
  description: ["description", "notes", "details"],
};

/** Map header cells to canonical field names (case/space/punctuation tolerant). */
export function detectColumns(header: string[]): Record<string, number> {
  const out: Record<string, number> = {};
  const norm = (s: string) => s.toLowerCase().replace(/[_\-.]+/g, " ").replace(/\s+/g, " ").trim();
  header.forEach((h, i) => {
    const n = norm(h);
    for (const [field, names] of Object.entries(ALIASES)) {
      if (out[field] === undefined && names.includes(n)) out[field] = i;
    }
  });
  return out;
}

export interface ParsedImport {
  rows: ProductInput[];
  locations: string[];
  errors: string[];
}

const num = (s: string | undefined): number => {
  if (!s) return 0;
  const n = Number(s.replace(/[^0-9.\-]/g, ""));
  return Number.isFinite(n) ? n : 0;
};

export function csvToImport(text: string): ParsedImport {
  const table = parseCsv(text);
  const res: ParsedImport = { rows: [], locations: [], errors: [] };
  if (table.length < 2) {
    res.errors.push("The file needs a header row and at least one product.");
    return res;
  }
  const cols = detectColumns(table[0]);
  if (cols.name === undefined) {
    res.errors.push('No "Name" column found.');
    return res;
  }
  const get = (r: string[], f: string) => (cols[f] === undefined ? "" : (r[cols[f]] ?? "").trim());
  table.slice(1).forEach((r, i) => {
    const name = get(r, "name");
    if (!name) {
      res.errors.push(`Row ${i + 2}: missing name, skipped.`);
      return;
    }
    res.rows.push({
      name,
      sku: get(r, "sku"),
      barcode: get(r, "barcode"),
      category: get(r, "category"),
      unit: get(r, "unit") || "pcs",
      price: num(get(r, "price")),
      cost: num(get(r, "cost")),
      min_stock: num(get(r, "min_stock")),
      reorder_qty: num(get(r, "reorder_qty")),
      description: get(r, "description"),
      opening_qty: Math.max(0, num(get(r, "quantity"))),
    });
    res.locations.push(get(r, "location"));
  });
  return res;
}

// ------------------------------------------------------------ EAN-13

export function ean13CheckDigit(first12: string): number {
  let sum = 0;
  for (let i = 0; i < 12; i++) sum += Number(first12[i]) * (i % 2 === 0 ? 1 : 3);
  return (10 - (sum % 10)) % 10;
}

export function isValidEan13(code: string): boolean {
  return /^\d{13}$/.test(code) && ean13CheckDigit(code.slice(0, 12)) === Number(code[12]);
}

/** Build an EAN-13 from a 12-digit body (or shorter: left-padded with zeros). */
export function makeEan13(body: string): string {
  const b = body.replace(/\D/g, "").slice(0, 12).padStart(12, "0");
  return b + ean13CheckDigit(b);
}

/** New internal barcode: "200" prefix (in-store use) + 9 digits from `n`. */
export function generateBarcode(n: number): string {
  return makeEan13("200" + String(Math.abs(Math.floor(n)) % 1e9).padStart(9, "0"));
}

const L = ["0001101", "0011001", "0010011", "0111101", "0100011", "0110001", "0101111", "0111011", "0110111", "0001011"];
const G = ["0100111", "0110011", "0011011", "0100001", "0011101", "0111001", "0000101", "0010001", "0001001", "0010111"];
const R = ["1110010", "1100110", "1101100", "1000010", "1011100", "1001110", "1010000", "1000100", "1001000", "1110100"];
const PARITY = ["LLLLLL", "LLGLGG", "LLGGLG", "LLGGGL", "LGLLGG", "LGGLLG", "LGGGLL", "LGLGLG", "LGLGGL", "LGGLGL"];

/** The 95 modules (as a "0"/"1" string) of an EAN-13 symbol. */
export function ean13Modules(code: string): string {
  if (!isValidEan13(code)) throw new Error("not a valid EAN-13");
  const parity = PARITY[Number(code[0])];
  let bits = "101";
  for (let i = 0; i < 6; i++) bits += (parity[i] === "L" ? L : G)[Number(code[i + 1])];
  bits += "01010";
  for (let i = 0; i < 6; i++) bits += R[Number(code[i + 7])];
  return bits + "101";
}

/** Standalone SVG for printing; null when the code is not EAN-13. */
export function barcodeSvg(code: string, height = 60): string | null {
  if (!isValidEan13(code)) return null;
  const bits = ean13Modules(code);
  const q = 9; // quiet zone modules
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
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${height + 14}" width="${w * 2}" height="${(height + 14) * 2}"><rect width="${w}" height="${height + 14}" fill="#fff"/><g fill="#000">${bars}</g><text x="${w / 2}" y="${height + 11}" font-family="monospace" font-size="10" text-anchor="middle">${code}</text></svg>`;
}

// ------------------------------------------------------ stock helpers

export type StockStatus = "out" | "low" | "ok";

export function stockStatus(qty: number, min: number): StockStatus {
  if (qty <= 0) return "out";
  if (min > 0 && qty <= min) return "low";
  return "ok";
}

export const money = (n: number): string =>
  n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });

export const qtyText = (n: number): string => String(Math.round(n * 1000) / 1000);

/** Plain-text order sheets, one block per supplier. */
export function orderSheets(lines: ReorderLine[]): string {
  const by = new Map<string, ReorderLine[]>();
  for (const l of lines) {
    const k = l.supplier_name || "No supplier";
    by.set(k, [...(by.get(k) ?? []), l]);
  }
  const out: string[] = [];
  for (const [sup, ls] of by) {
    out.push(`Order for ${sup}`);
    let total = 0;
    for (const l of ls) {
      const label = l.product.variant_label ? `${l.product.name} (${l.product.variant_label})` : l.product.name;
      const code = l.supplier_sku || l.product.sku;
      const line = l.suggested_qty * l.unit_cost;
      total += line;
      out.push(`- ${qtyText(l.suggested_qty)} x ${label}${code ? ` [${code}]` : ""} @ ${money(l.unit_cost)} = ${money(line)}`);
    }
    out.push(`Total: ${money(total)}`, "");
  }
  return out.join("\n");
}
