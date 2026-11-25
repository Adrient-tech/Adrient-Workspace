// Pure accounting helpers. The server is authoritative; these power live
// validation in the journal editor and the CSV/print exports.
import type { JLine } from "./types";

export const ACCENT = "#2E7D32";

export const round2 = (v: number) => Math.round((v + Number.EPSILON * Math.sign(v)) * 100) / 100;

export function money(n: number, currency = "INR"): string {
  const v = Number.isFinite(n) ? n : 0;
  try {
    return new Intl.NumberFormat("en-IN", { style: "currency", currency: currency.toUpperCase() }).format(v);
  } catch {
    return `${currency} ${v.toFixed(2)}`;
  }
}

/** Amount for a report cell: blank for zero. */
export const cell = (n: number) => (Math.abs(n) < 0.005 ? "" : money(n));

export const todayISO = () => {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};

/** First day of the financial year containing `iso` (default April). */
export function fyStart(iso: string, startMonth = 4): string {
  const [y, m] = iso.split("-").map(Number);
  if (!y || !m) return "";
  const year = m < startMonth ? y - 1 : y;
  return `${year}-${String(startMonth).padStart(2, "0")}-01`;
}

export function fyEnd(start: string): string {
  const [y, m] = start.split("-").map(Number);
  const d = new Date(Date.UTC(y + 1, m - 1, 0));
  return d.toISOString().slice(0, 10);
}

export interface Balance {
  debit: number;
  credit: number;
  diff: number;
  balanced: boolean;
}

/** Totals in whole paise so 0.1 + 0.2 equals 0.3. */
export function balanceOf(lines: Pick<JLine, "debit" | "credit">[]): Balance {
  let d = 0;
  let c = 0;
  for (const l of lines) {
    d += Math.round((l.debit || 0) * 100);
    c += Math.round((l.credit || 0) * 100);
  }
  return { debit: d / 100, credit: c / 100, diff: (d - c) / 100, balanced: d === c && d > 0 };
}

export const emptyLine = (): JLine => ({ account_id: "", debit: 0, credit: 0, memo: "" });

export function gstOf(amount: number, rate: number) {
  const gst = round2((amount * (rate || 0)) / 100);
  return { gst, total: round2(amount + gst) };
}

export function csvEscape(v: string | number): string {
  const s = String(v ?? "");
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

export function toCSV(rows: (string | number)[][]): string {
  return rows.map((r) => r.map(csvEscape).join(",")).join("\n");
}

export function downloadCSV(name: string, rows: (string | number)[][]) {
  const blob = new Blob([toCSV(rows)], { type: "text/csv;charset=utf-8" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = name;
  a.click();
  URL.revokeObjectURL(a.href);
}

export const SOURCE_LABEL: Record<string, string> = {
  manual: "Manual",
  pos_sale: "POS sale",
  pos_refund: "POS refund",
  invoice: "Invoice",
  invoice_payment: "Invoice payment",
  expense: "Expense",
  expense_payment: "Bill payment",
  bank_line: "Bank",
  report: "Reimbursement",
};

export const STATUS_COLOR: Record<string, "neutral" | "primary" | "success" | "warning" | "danger"> = {
  draft: "neutral",
  submitted: "primary",
  approved: "success",
  rejected: "danger",
  reimbursed: "success",
  pending: "warning",
  added: "success",
  matched: "success",
  excluded: "neutral",
};
