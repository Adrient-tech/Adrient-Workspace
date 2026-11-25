import { describe, expect, it } from "vitest";
import { MemoryStore } from "./outbox";
import type { OutboxEntry } from "./outbox";
import { completeOrQueue, isTransient, offlineSale } from "./saleSync";
import { priceCart } from "./logic";
import type { CartLine } from "./types";

const q = (s: MemoryStore) => ({ enqueue: (e: OutboxEntry) => s.put(e) });
const summary = { total: 100, lines: 1, customer: "", register: "Front" };
const sale = { register_id: "r1", lines: [{ name: "x", qty: 1, unit_price: 100 }], payments: [{ method: "cash", amount: 100 }] };

describe("completeOrQueue", () => {
  it("returns the sale when the server accepts it", async () => {
    const store = new MemoryStore();
    const r = await completeOrQueue(q(store), sale as never, summary, undefined, {
      key: () => "k-online-0001",
      post: async () => ({ status: 200, body: { sale: { id: "s1", number: 7 }, replay: false } }),
    });
    expect(r.kind).toBe("sale");
    expect(await store.list()).toHaveLength(0);
  });

  it("queues under the same key when offline, and a later retry uses that key", async () => {
    const store = new MemoryStore();
    const keys: string[] = [];
    const r = await completeOrQueue(q(store), sale as never, summary, "4829", {
      key: () => "k-offline-001",
      now: () => new Date("2026-11-18T10:00:00Z"),
      post: async (k) => {
        keys.push(k);
        return { status: 0, body: {} };
      },
    });
    expect(r.kind).toBe("queued");
    const [e] = await store.list();
    expect(e).toMatchObject({ key: "k-offline-001", status: "queued", approval_pin: "4829", created_at: "2026-11-18T10:00:00.000Z" });
    expect(keys).toEqual(["k-offline-001"]);
  });

  it("queues on 5xx (the server may have saved it) but never on a refusal", async () => {
    const store = new MemoryStore();
    const queued = await completeOrQueue(q(store), sale as never, summary, undefined, { key: () => "k-5xx-000001", post: async () => ({ status: 502, body: {} }) });
    expect(queued.kind).toBe("queued");
    const refused = await completeOrQueue(q(store), sale as never, summary, undefined, {
      key: () => "k-refused-001",
      post: async () => ({ status: 422, body: { error: "payments do not cover the total of 100.00" } }),
    });
    expect(refused).toEqual({ kind: "error", message: "payments do not cover the total of 100.00" });
    expect(await store.list()).toHaveLength(1);
  });
});

describe("isTransient", () => {
  it("treats network failures and server errors as retryable only", () => {
    expect([0, 408, 409, 429, 500, 502, 503].every(isTransient)).toBe(true);
    expect([200, 400, 401, 403, 404, 422].some(isTransient)).toBe(false);
  });
});

describe("offlineSale", () => {
  it("builds a receipt with change and a clear 'not numbered yet' marker", () => {
    const cart: CartLine[] = [{ key: "a", product_id: "p", name: "Tea", sku: "", qty: 2, unit_price: 40, discount_kind: "", discount_value: 0, tax_rate_id: "", note: "" }];
    const priced = priceCart(cart, [], "", 0);
    const entry: OutboxEntry = { key: "k1", created_at: "2026-11-18T10:00:00.000Z", sale: {}, summary, status: "queued", error: "", attempts: 0 };
    const s = offlineSale(entry, cart, priced, [{ method: "cash", amount: 100 }], "Sam", "Front");
    expect(String(s.number)).toContain("OFFLINE");
    expect(s.total).toBe(80);
    expect(s.change_due).toBe(20);
    expect(s.lines[0].line_total).toBe(80);
  });
});
