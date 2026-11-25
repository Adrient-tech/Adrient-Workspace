// Pure invoicing helpers. Totals mirror internal/invoicing/calc.go (the server
// is authoritative; this powers the live preview in the editor).
import type { DisplayStatus, DocInput, InvDocument, Line } from "./types";

export const ACCENT = "#C2410C";

export const round2 = (v: number) => Math.round((v + Number.EPSILON * Math.sign(v)) * 100) / 100;

export function money(n: number, currency = "INR"): string {
  const v = Number.isFinite(n) ? n : 0;
  try {
    return new Intl.NumberFormat(undefined, { style: "currency", currency: currency.toUpperCase() }).format(v);
  } catch {
    return `${currency} ${v.toFixed(2)}`;
  }
}

const clampPct = (p: number) => Math.min(Math.max(p || 0, 0), 100);

export function computeTotals(lines: Line[], discountKind: string, discountValue: number) {
  let sub = 0;
  let inclusive = false;
  const nets = lines.map((l) => {
    let n = (l.qty || 0) * (l.price || 0) * (1 - clampPct(l.discount_pct) / 100);
    if (l.tax_inclusive && (l.tax_rate || 0) > 0) {
      n = n / (1 + l.tax_rate / 100);
      inclusive = true;
    }
    sub += n;
    return n;
  });
  const disc =
    discountKind === "amount" ? Math.min(Math.max(discountValue || 0, 0), sub) : (sub * clampPct(discountValue)) / 100;
  let tax = 0;
  let gross = 0;
  if (sub > 0)
    lines.forEach((l, i) => {
      const share = nets[i] - (disc * nets[i]) / sub;
      tax += (share * Math.max(l.tax_rate || 0, 0)) / 100;
      gross += share * (1 + Math.max(l.tax_rate || 0, 0) / 100);
    });
  const subtotal = round2(sub);
  const discount_total = round2(disc);
  let tax_total = round2(tax);
  let total = round2(subtotal - discount_total + tax_total);
  if (inclusive) {
    total = round2(gross);
    tax_total = round2(total - subtotal + discount_total);
  }
  return { subtotal, discount_total, tax_total, total };
}

export const emptyLine = (): Line => ({
  description: "",
  qty: 1,
  unit: "",
  price: 0,
  discount_pct: 0,
  tax_rate: 0,
  tax_name: "",
});

export const todayISO = () => new Date().toISOString().slice(0, 10);

export function addDays(iso: string, days: number): string {
  const d = new Date(iso + "T00:00:00Z");
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

export const STATUS_LABEL: Record<DisplayStatus, string> = {
  draft: "Draft",
  sent: "Sent",
  partial: "Partly paid",
  paid: "Paid",
  overdue: "Overdue",
  written_off: "Written off",
  accepted: "Accepted",
  declined: "Declined",
  converted: "Converted",
  credited: "Credited",
  expired: "Expired",
  superseded: "Replaced",
  open: "Open",
  partly_used: "Partly used",
  refunded: "Refunded",
  applied: "Applied",
};

export const STATUS_COLOR: Record<DisplayStatus, "neutral" | "primary" | "success" | "warning" | "danger"> = {
  draft: "neutral",
  sent: "primary",
  partial: "warning",
  paid: "success",
  overdue: "danger",
  written_off: "neutral",
  accepted: "success",
  declined: "danger",
  converted: "neutral",
  credited: "neutral",
  expired: "warning",
  superseded: "neutral",
  open: "primary",
  partly_used: "warning",
  refunded: "neutral",
  applied: "success",
};

export function blankDoc(kind: "invoice" | "estimate", currency: string, dueDays: number, defaults: { notes: string; terms: string; tax_note: string }): DocInput {
  const issue = todayISO();
  return {
    kind,
    contact_id: "",
    client_name: "",
    client_email: "",
    client_address: "",
    currency,
    issue_date: issue,
    due_date: kind === "invoice" ? addDays(issue, dueDays) : "",
    valid_until: kind === "estimate" ? addDays(issue, 30) : "",
    place_of_supply: "",
    client_gstin: "",
    lines: [emptyLine()],
    discount_kind: "percent",
    discount_value: 0,
    notes: defaults.notes,
    terms: defaults.terms,
    tax_note: defaults.tax_note,
    schedule: [],
    attachments: [],
  };
}

export function inputFromDoc(d: InvDocument): DocInput {
  return {
    kind: d.kind,
    contact_id: d.contact_id,
    client_name: d.client_name,
    client_email: d.client_email,
    client_address: d.client_address,
    currency: d.currency,
    issue_date: d.issue_date,
    due_date: d.due_date,
    lines: d.lines.map((l) => ({ ...l })),
    discount_kind: d.discount_kind,
    discount_value: d.discount_value,
    notes: d.notes,
    terms: d.terms,
    tax_note: d.tax_note,
    schedule: d.schedule.map((s) => ({ ...s })),
    attachments: d.attachments.map((a) => ({ ...a })),
    number: d.number,
    valid_until: d.valid_until ?? "",
    place_of_supply: d.place_of_supply ?? "",
    client_gstin: d.client_gstin ?? "",
    price_list_id: d.price_list_id ?? "",
  };
}

/** GST state codes for the place-of-supply picker. */
export const GST_STATES: [string, string][] = [
  ["01", "Jammu and Kashmir"], ["02", "Himachal Pradesh"], ["03", "Punjab"], ["04", "Chandigarh"], ["05", "Uttarakhand"],
  ["06", "Haryana"], ["07", "Delhi"], ["08", "Rajasthan"], ["09", "Uttar Pradesh"], ["10", "Bihar"], ["11", "Sikkim"],
  ["12", "Arunachal Pradesh"], ["13", "Nagaland"], ["14", "Manipur"], ["15", "Mizoram"], ["16", "Tripura"], ["17", "Meghalaya"],
  ["18", "Assam"], ["19", "West Bengal"], ["20", "Jharkhand"], ["21", "Odisha"], ["22", "Chhattisgarh"], ["23", "Madhya Pradesh"],
  ["24", "Gujarat"], ["26", "Dadra and Nagar Haveli and Daman and Diu"], ["27", "Maharashtra"], ["29", "Karnataka"], ["30", "Goa"],
  ["31", "Lakshadweep"], ["32", "Kerala"], ["33", "Tamil Nadu"], ["34", "Puducherry"], ["35", "Andaman and Nicobar Islands"],
  ["36", "Telangana"], ["37", "Andhra Pradesh"], ["38", "Ladakh"], ["96", "Other country (export)"], ["97", "Other territory"],
];

/** Mirrors internal/invoicing/gst.go IsIntraState: unknown place of supply counts as the supplier's state. */
export function isIntraState(supplier: string, pos: string): boolean {
  if (!pos) return true;
  if (pos === "96") return false;
  if (!supplier) return true;
  return supplier === pos;
}

/** UPI deep link used for the QR code on invoices (NPCI "upi://pay" format). */
export function upiLink(vpa: string, name: string, amount: number, note: string): string {
  const p = new URLSearchParams({ pa: vpa, pn: name, am: amount.toFixed(2), cu: "INR", tn: note });
  return `upi://pay?${p.toString()}`;
}

/** Split a total into n instalments; the last one absorbs the rounding. */
export function splitSchedule(total: number, n: number, firstDue: string, everyDays = 30): { label: string; due_date: string; amount: number }[] {
  if (n < 1) return [];
  const each = round2(total / n);
  return Array.from({ length: n }, (_, i) => ({
    label: `Instalment ${i + 1} of ${n}`,
    due_date: addDays(firstDue, i * everyDays),
    amount: i === n - 1 ? round2(total - each * (n - 1)) : each,
  }));
}

// ---- CSV ----

export function parseCSV(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let cur = "";
  let q = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (q) {
      if (c === '"' && text[i + 1] === '"') {
        cur += '"';
        i++;
      } else if (c === '"') q = false;
      else cur += c;
    } else if (c === '"') q = true;
    else if (c === ",") {
      row.push(cur);
      cur = "";
    } else if (c === "\n" || c === "\r") {
      if (c === "\r" && text[i + 1] === "\n") i++;
      row.push(cur);
      cur = "";
      if (row.some((x) => x.trim() !== "")) rows.push(row);
      row = [];
    } else cur += c;
  }
  row.push(cur);
  if (row.some((x) => x.trim() !== "")) rows.push(row);
  return rows;
}

export const csvCell = (v: string | number) => {
  const s = String(v ?? "");
  return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
};

export const toCSV = (rows: (string | number)[][]) => rows.map((r) => r.map(csvCell).join(",")).join("\n");

/** Price-list import: header names name, description, unit, price, tax (any order, case-insensitive). */
export function parseItemsCSV(text: string) {
  const rows = parseCSV(text);
  if (rows.length < 2) return [];
  const head = rows[0].map((h) => h.trim().toLowerCase());
  const col = (...names: string[]) => head.findIndex((h) => names.includes(h));
  const iName = col("name", "item");
  const iDesc = col("description", "details");
  const iUnit = col("unit");
  const iPrice = col("price", "rate", "unit price");
  const iTax = col("tax", "tax_rate", "tax %");
  if (iName < 0) return [];
  return rows
    .slice(1)
    .map((r) => ({
      name: (r[iName] ?? "").trim(),
      description: iDesc >= 0 ? (r[iDesc] ?? "").trim() : "",
      unit: iUnit >= 0 ? (r[iUnit] ?? "").trim() : "",
      price: iPrice >= 0 ? Number(r[iPrice]) || 0 : 0,
      tax_rate: iTax >= 0 ? Number(r[iTax]) || 0 : 0,
    }))
    .filter((x) => x.name);
}

/**
 * Invoice import: one row per line, grouped by `number` (or by client+date when
 * absent). Columns: number, client, email, date, due, currency, description, qty, price, tax.
 */
export function parseInvoicesCSV(text: string): Partial<DocInput>[] {
  const rows = parseCSV(text);
  if (rows.length < 2) return [];
  const head = rows[0].map((h) => h.trim().toLowerCase());
  const col = (...n: string[]) => head.findIndex((h) => n.includes(h));
  const c = {
    number: col("number", "invoice", "invoice number"),
    client: col("client", "customer", "client_name"),
    email: col("email", "client_email"),
    date: col("date", "issue_date"),
    due: col("due", "due_date"),
    cur: col("currency"),
    desc: col("description", "item"),
    qty: col("qty", "quantity"),
    price: col("price", "rate"),
    tax: col("tax", "tax_rate"),
  };
  const get = (r: string[], i: number) => (i >= 0 ? (r[i] ?? "").trim() : "");
  const groups = new Map<string, Partial<DocInput>>();
  for (const r of rows.slice(1)) {
    const client = get(r, c.client);
    const date = get(r, c.date);
    const key = get(r, c.number) || `${client}|${date}`;
    let g = groups.get(key);
    if (!g) {
      g = {
        kind: "invoice",
        number: get(r, c.number),
        client_name: client,
        client_email: get(r, c.email),
        issue_date: /^\d{4}-\d{2}-\d{2}$/.test(date) ? date : todayISO(),
        due_date: /^\d{4}-\d{2}-\d{2}$/.test(get(r, c.due)) ? get(r, c.due) : "",
        currency: get(r, c.cur).toUpperCase(),
        lines: [],
      };
      groups.set(key, g);
    }
    const description = get(r, c.desc);
    if (description) {
      g.lines!.push({
        ...emptyLine(),
        description,
        qty: c.qty >= 0 ? Number(get(r, c.qty)) || 1 : 1,
        price: Number(get(r, c.price)) || 0,
        tax_rate: Number(get(r, c.tax)) || 0,
      });
    }
  }
  return [...groups.values()].filter((g) => g.client_name && g.lines!.length > 0);
}

export function docsCSV(docs: InvDocument[]): string {
  return toCSV([
    ["number", "kind", "client", "email", "date", "due", "currency", "subtotal", "tax", "total", "paid", "balance", "status"],
    ...docs.map((d) => [d.number, d.kind, d.client_name, d.client_email, d.issue_date, d.due_date, d.currency, d.subtotal, d.tax_total, d.total, d.paid, d.balance, d.display_status]),
  ]);
}

export function downloadText(name: string, text: string, mime = "text/csv") {
  const url = URL.createObjectURL(new Blob([text], { type: mime }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
