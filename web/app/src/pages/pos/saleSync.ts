import { defaultStore, newIdemKey } from "./outbox";
import type { OutboxEntry } from "./outbox";
import { round2, summarizeTenders } from "./logic";
import type { PricedCart, Tender } from "./logic";
import type { CartLine, PosSale, PosSaleInput } from "./types";

export type CompleteResult =
  | { kind: "sale"; sale: PosSale; replay: boolean }
  | { kind: "queued"; entry: OutboxEntry }
  | { kind: "error"; message: string };

export interface Queue {
  enqueue: (e: OutboxEntry) => Promise<void>;
}

/** True when a thrown fetch error or a status means "the server could not be reached", not "the sale was refused". */
export function isTransient(status: number): boolean {
  return status === 0 || status === 408 || status === 409 || status === 429 || status >= 500;
}

export interface CompleteDeps {
  post?: (key: string, sale: unknown, pin?: string) => Promise<{ status: number; body: Record<string, unknown> }>;
  key?: () => string;
  now?: () => Date;
}

async function httpPost(key: string, sale: unknown, pin?: string) {
  try {
    const r = await fetch("/api/v1/pos/x/sales/complete", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ idempotency_key: key, sale, approval_pin: pin }),
    });
    let body: Record<string, unknown> = {};
    try {
      body = (await r.json()) as Record<string, unknown>;
    } catch {
      /* empty body */
    }
    return { status: r.status, body };
  } catch {
    return { status: 0, body: {} };
  }
}

/**
 * Completes a sale exactly once. Online it is sent at once. When the server
 * cannot be reached (offline, timeout, 5xx) the sale is saved in the outbox
 * under the same idempotency key and sent later, so a sale that did reach the
 * server before the connection dropped is recognised and not created twice.
 * A refusal (for example payments that do not cover the total) is returned as
 * an error and never queued.
 */
export async function completeOrQueue(
  queue: Queue,
  sale: PosSaleInput,
  summary: OutboxEntry["summary"],
  pin: string | undefined,
  deps: CompleteDeps = {},
): Promise<CompleteResult> {
  const key = (deps.key ?? newIdemKey)();
  const post = deps.post ?? httpPost;
  const r = await post(key, sale, pin);
  if (r.status >= 200 && r.status < 300 && r.body.sale) {
    return { kind: "sale", sale: r.body.sale as PosSale, replay: !!r.body.replay };
  }
  if (isTransient(r.status)) {
    const entry: OutboxEntry = {
      key,
      created_at: (deps.now?.() ?? new Date()).toISOString(),
      sale,
      approval_pin: pin,
      summary,
      status: "queued",
      error: "",
      attempts: 0,
    };
    await queue.enqueue(entry);
    return { kind: "queued", entry };
  }
  const msg = (r.body.error as string) || (r.body.message as string) || `The sale was not accepted (HTTP ${r.status}).`;
  return { kind: "error", message: msg };
}

/** A receipt-shaped sale for the screen while the real one waits to sync. It has no number until the server assigns one. */
export function offlineSale(entry: OutboxEntry, cart: CartLine[], priced: PricedCart, tenders: Tender[], customer: string, registerName: string): PosSale {
  const sum = summarizeTenders(priced.total, tenders);
  return {
    id: entry.key,
    number: "OFFLINE (number assigned when synced)",
    register_id: "",
    register_name: registerName,
    shift_id: "",
    customer_id: "",
    customer_name: customer,
    status: "completed",
    subtotal: priced.subtotal,
    discount_total: priced.discount,
    tax_total: priced.tax,
    total: priced.total,
    paid: round2(priced.total),
    change_due: sum.change,
    discount_kind: "",
    discount_value: 0,
    note: "",
    hold_label: "",
    refunded_total: 0,
    loyalty_earned: 0,
    created_by: "",
    created_at: entry.created_at,
    completed_at: entry.created_at,
    lines: cart.map((l, i) => ({
      id: `${entry.key}-${i}`,
      product_id: l.product_id,
      name: l.name,
      sku: l.sku,
      qty: l.qty,
      unit_price: l.unit_price,
      discount_kind: l.discount_kind,
      discount_value: l.discount_value,
      tax_rate_id: l.tax_rate_id,
      tax_name: "",
      tax_rate: 0,
      tax_inclusive: false,
      discount_amount: priced.lines[i].discount,
      tax_amount: priced.lines[i].tax,
      line_total: priced.lines[i].total,
      refunded_qty: 0,
      refunded_amount: 0,
      note: l.note,
    })),
    payments: tenders.map((t, i) => ({ id: `${entry.key}-p${i}`, method: t.method, amount: t.amount, tendered: t.amount, ref: t.ref ?? "", created_at: entry.created_at })),
    refunds: [],
  };
}

// ---- catalogue cache: lets the register keep selling after the connection drops ----

const CACHE_PREFIX = "adrient-pos-cache:";

export function cachePut(name: string, value: unknown): void {
  try {
    localStorage.setItem(CACHE_PREFIX + name, JSON.stringify(value));
  } catch {
    /* storage full or blocked: caching is best effort */
  }
}

export function cacheGet<T>(name: string): T | null {
  try {
    const raw = localStorage.getItem(CACHE_PREFIX + name);
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
}

export { defaultStore };
