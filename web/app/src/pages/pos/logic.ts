// Pure POS logic. The pricing and tender rules mirror internal/pos/pricing.go
// (the server is authoritative; this only drives live totals in the register).
import type { CartLine, DiscountKind, PayMethod, PosPaymentInput, PosSale, PosSettings, PosTaxRate } from "./types";

export const round2 = (n: number) => Math.round((n + Number.EPSILON) * 100) / 100;

export function money(n: number, currency = ""): string {
  const v = Number.isFinite(n) ? n : 0;
  if (/^[A-Za-z]{3}$/.test(currency)) {
    try {
      return new Intl.NumberFormat(undefined, { style: "currency", currency: currency.toUpperCase() }).format(v);
    } catch {
      /* unknown code: fall through */
    }
  }
  return v.toFixed(2);
}

export const qtyText = (n: number) => (Number.isInteger(n) ? String(n) : String(Math.round(n * 1000) / 1000));

function discountOf(kind: DiscountKind | string, value: number, base: number): number {
  if (base <= 0 || value <= 0) return 0;
  let d = 0;
  if (kind === "percent") d = round2((base * Math.min(value, 100)) / 100);
  else if (kind === "amount") d = round2(value);
  else return 0;
  return Math.min(d, base);
}

export interface PricedLine {
  gross: number;
  discount: number;
  tax: number;
  total: number;
}

export interface PricedCart {
  lines: PricedLine[];
  subtotal: number;
  discount: number;
  tax: number;
  total: number;
}

export function priceCart(
  lines: CartLine[],
  rates: PosTaxRate[],
  orderKind: DiscountKind | string = "",
  orderValue = 0,
): PricedCart {
  const byId = new Map(rates.map((r) => [r.id, r]));
  const gross = lines.map((l) => round2(l.qty * l.unit_price));
  const lineDisc = lines.map((l, i) => discountOf(l.discount_kind, l.discount_value, gross[i]));
  const after = gross.map((g, i) => round2(g - lineDisc[i]));
  const afterSum = after.reduce((a, b) => a + b, 0);
  const od = discountOf(orderKind, orderValue, round2(afterSum));
  let last = -1;
  after.forEach((a, i) => {
    if (a > 0) last = i;
  });
  let remaining = od;
  const out: PosRateLine[] = [];
  const res: PricedCart = { lines: [], subtotal: 0, discount: 0, tax: 0, total: 0 };
  lines.forEach((l, i) => {
    let share = afterSum > 0 ? round2((od * after[i]) / afterSum) : 0;
    if (i === last) share = remaining;
    share = Math.min(share, after[i]);
    remaining = round2(remaining - share);
    const base = round2(after[i] - share);
    const t = l.tax_rate_id ? byId.get(l.tax_rate_id) : undefined;
    let tax = 0;
    let total = base;
    if (t && t.rate > 0) {
      if (t.inclusive) tax = round2(base - base / (1 + t.rate / 100));
      else {
        tax = round2((base * t.rate) / 100);
        total = round2(base + tax);
      }
    }
    const discount = round2(lineDisc[i] + share);
    out.push({ gross: gross[i], discount, tax, total });
    res.subtotal += gross[i];
    res.discount += discount;
    res.tax += tax;
    res.total += total;
  });
  res.lines = out;
  res.subtotal = round2(res.subtotal);
  res.discount = round2(res.discount);
  res.tax = round2(res.tax);
  res.total = round2(res.total);
  return res;
}
type PosRateLine = PricedLine;

export interface Tender {
  method: PayMethod;
  amount: number;
  /** Payment reference; "gift:CODE" pays from a gift card. */
  ref?: string;
}

export interface TenderSummary {
  /** Sum applied to the sale. */
  applied: number;
  /** Still to pay (0 when covered). */
  remaining: number;
  /** Cash change to hand back. */
  change: number;
  /** True when the tenders cover the total and no non-cash method overpays. */
  ok: boolean;
}

/** Mirrors AllocateTenders: non-cash first, cash takes the rest and may overshoot. */
export function summarizeTenders(total: number, tenders: Tender[]): TenderSummary {
  let due = round2(total);
  let change = 0;
  let ok = true;
  for (const t of tenders.filter((x) => x.method !== "cash")) {
    const a = round2(t.amount);
    if (a <= 0) continue;
    if (a > due + 0.004) ok = false;
    due = round2(due - Math.min(a, due));
  }
  for (const t of tenders.filter((x) => x.method === "cash")) {
    const a = round2(t.amount);
    if (a <= 0) continue;
    const use = Math.min(a, due);
    change = round2(change + a - use);
    due = round2(due - use);
  }
  return { applied: round2(total - due), remaining: due <= 0.004 ? 0 : due, change, ok: ok && due <= 0.004 };
}

/** Quick cash buttons: the exact amount plus the next round notes above it. */
export function quickTender(total: number): number[] {
  const t = round2(total);
  if (t <= 0) return [];
  const out = new Set<number>([t]);
  for (const step of [1, 5, 10, 20, 50, 100, 500, 1000]) {
    const up = Math.ceil(t / step) * step;
    if (up >= t) out.add(round2(up));
    if (out.size >= 5) break;
  }
  return [...out].sort((a, b) => a - b).slice(0, 5);
}

/** Add a product to the cart; the same product without edits merges into one line. */
export function addToCart(
  cart: CartLine[],
  p: { product_id: string; name: string; sku: string; unit_price: number; tax_rate_id: string },
  qty = 1,
): CartLine[] {
  const i = cart.findIndex(
    (l) => p.product_id && l.product_id === p.product_id && l.discount_kind === "" && l.note === "" && l.unit_price === p.unit_price && l.name === p.name,
  );
  if (i >= 0) return cart.map((l, j) => (j === i ? { ...l, qty: round3(l.qty + qty) } : l));
  return [
    ...cart,
    {
      key: `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`,
      product_id: p.product_id,
      name: p.name,
      sku: p.sku,
      qty,
      unit_price: p.unit_price,
      discount_kind: "",
      discount_value: 0,
      tax_rate_id: p.tax_rate_id,
      note: "",
    },
  ];
}
const round3 = (n: number) => Math.round(n * 1000) / 1000;

export function cartFromSale(sale: PosSale): CartLine[] {
  return sale.lines.map((l) => ({
    key: l.id,
    product_id: l.product_id,
    name: l.name,
    sku: l.sku,
    qty: l.qty,
    unit_price: l.unit_price,
    discount_kind: l.discount_kind,
    discount_value: l.discount_value,
    tax_rate_id: l.tax_rate_id,
    note: l.note,
  }));
}

export function toPaymentInputs(tenders: Tender[]): PosPaymentInput[] {
  return tenders.filter((t) => t.amount > 0).map((t) => ({ method: t.method, amount: round2(t.amount), ...(t.ref ? { ref: t.ref } : {}) }));
}

export const methodLabel = (m: string) =>
  ({ cash: "Cash", card: "Card", other: "Other", store_credit: "Store credit", loyalty: "Loyalty points" })[m] ?? m;

export const statusLabel = (s: string) =>
  ({ held: "Held", completed: "Completed", partially_refunded: "Part refunded", refunded: "Refunded" })[s] ?? s;

/** Points needed to pay `amount` with loyalty points. */
export function pointsFor(amount: number, pointValue: number): number {
  return pointValue > 0 ? Math.ceil((amount / pointValue) * 100) / 100 : 0;
}

/** Plain-text receipt, used for the email link and copy. */
export function receiptText(sale: PosSale, s: Pick<PosSettings, "store_name" | "currency" | "receipt_header" | "receipt_footer">): string {
  const m = (n: number) => money(n, s.currency);
  const out: string[] = [];
  if (s.store_name) out.push(s.store_name);
  if (s.receipt_header) out.push(s.receipt_header);
  out.push(`Receipt #${sale.number}`, new Date(sale.completed_at || sale.created_at).toLocaleString());
  if (sale.customer_name) out.push(`Customer: ${sale.customer_name}`);
  out.push("");
  for (const l of sale.lines) {
    out.push(`${qtyText(l.qty)} x ${l.name}  ${m(l.line_total)}`);
    if (l.note) out.push(`   ${l.note}`);
  }
  out.push("");
  out.push(`Subtotal: ${m(sale.subtotal)}`);
  if (sale.discount_total > 0) out.push(`Discounts: -${m(sale.discount_total)}`);
  if (sale.tax_total > 0) out.push(`Tax: ${m(sale.tax_total)}`);
  out.push(`Total: ${m(sale.total)}`);
  for (const p of sale.payments) out.push(`${methodLabel(p.method)}: ${m(p.method === "cash" ? p.tendered : p.amount)}`);
  if (sale.change_due > 0) out.push(`Change: ${m(sale.change_due)}`);
  for (const r of sale.refunds) out.push(`Refund ${r.number}: -${m(r.total)}`);
  if (s.receipt_footer) out.push("", s.receipt_footer);
  return out.join("\n");
}

/** Local day boundaries as RFC3339 strings for the report/list filters. */
export function dayRange(daysBack: number, now = new Date()): { since: string; until: string } {
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate() - daysBack);
  const end = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  return { since: start.toISOString(), until: end.toISOString() };
}

export const num = (v: number | string | undefined) => Number(v ?? 0) || 0;
