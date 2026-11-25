import { describe, expect, it } from "vitest";
import { MemoryStore, classifyStatus, flushOutbox, newIdemKey } from "./outbox";
import type { OutboxEntry, Sender } from "./outbox";
import { DRAWER_KICK, receiptBytes, usbSupport } from "./hardware";

const entry = (key: string, at: string): OutboxEntry => ({
  key,
  created_at: at,
  sale: { register_id: "r1", lines: [] },
  summary: { total: 100, lines: 1, customer: "", register: "Front" },
  status: "queued",
  error: "",
  attempts: 0,
});

describe("idempotency keys", () => {
  it("are unique, long enough for the server and fit its limit", () => {
    const a = newIdemKey();
    const b = newIdemKey();
    expect(a).not.toBe(b);
    expect(a.length).toBeGreaterThanOrEqual(8);
    expect(a.length).toBeLessThanOrEqual(80);
  });
});

describe("classifyStatus", () => {
  it("separates done, permanent and retry", () => {
    expect(classifyStatus(200)).toBe("done");
    expect(classifyStatus(422)).toBe("permanent");
    expect(classifyStatus(400)).toBe("permanent");
    expect(classifyStatus(403)).toBe("permanent");
    expect(classifyStatus(409)).toBe("retry");
    expect(classifyStatus(429)).toBe("retry");
    expect(classifyStatus(502)).toBe("retry");
    expect(classifyStatus(503)).toBe("retry");
  });
});

describe("flushOutbox", () => {
  it("sends oldest first, removes what was accepted", async () => {
    const store = new MemoryStore();
    await store.put(entry("k2", "2026-01-01T10:00:02Z"));
    await store.put(entry("k1", "2026-01-01T10:00:01Z"));
    const order: string[] = [];
    const send: Sender = async (e) => {
      order.push(e.key);
      return { status: 200 };
    };
    const r = await flushOutbox(store, send);
    expect(order).toEqual(["k1", "k2"]);
    expect(r).toMatchObject({ sent: 2, failed: 0, waiting: 0 });
    expect(await store.list()).toHaveLength(0);
  });

  it("stops at the first network failure and keeps everything queued", async () => {
    const store = new MemoryStore();
    for (const k of ["a1", "a2", "a3"]) await store.put(entry(k, `2026-01-01T10:00:0${k[1]}Z`));
    let calls = 0;
    const r = await flushOutbox(store, async () => {
      calls++;
      return { status: 0 };
    });
    expect(calls).toBe(1);
    expect(r.stoppedOffline).toBe(true);
    expect(r.waiting).toBe(3);
    expect((await store.list()).every((e) => e.status === "queued")).toBe(true);
  });

  it("marks a rejected sale failed and still sends the next one", async () => {
    const store = new MemoryStore();
    await store.put(entry("good1", "2026-01-01T10:00:01Z"));
    await store.put(entry("bad", "2026-01-01T10:00:02Z"));
    await store.put(entry("good2", "2026-01-01T10:00:03Z"));
    const r = await flushOutbox(store, async (e) => (e.key === "bad" ? { status: 422, message: "payments do not cover the total" } : { status: 200 }));
    expect(r).toMatchObject({ sent: 2, failed: 1 });
    const left = await store.list();
    expect(left).toHaveLength(1);
    expect(left[0]).toMatchObject({ key: "bad", status: "failed", error: "payments do not cover the total", attempts: 1 });
    // A failed sale is not retried automatically.
    let again = 0;
    await flushOutbox(store, async () => {
      again++;
      return { status: 200 };
    });
    expect(again).toBe(0);
  });

  it("sends the same key every time a sale is retried (so the server can deduplicate)", async () => {
    const store = new MemoryStore();
    await store.put(entry("stable-key-1", "2026-01-01T10:00:01Z"));
    const seen: string[] = [];
    await flushOutbox(store, async (e) => {
      seen.push(e.key);
      return { status: 503 };
    });
    await flushOutbox(store, async (e) => {
      seen.push(e.key);
      return { status: 200 };
    });
    expect(seen).toEqual(["stable-key-1", "stable-key-1"]);
    expect(await store.list()).toHaveLength(0);
  });
});

describe("hardware", () => {
  it("builds the standard drawer kick and a cut", () => {
    expect(Array.from(DRAWER_KICK)).toEqual([0x1b, 0x70, 0x00, 0x19, 0xfa]);
    const b = receiptBytes("Total: ₹ 10", { kickDrawer: true });
    expect(Array.from(b.slice(0, 2))).toEqual([0x1b, 0x40]);
    expect(new TextDecoder().decode(b)).toContain("Total: Rs. 10");
    expect(Array.from(b.slice(-5))).toEqual(Array.from(DRAWER_KICK));
    const noCut = receiptBytes("x", { cut: false });
    expect(Array.from(noCut)).not.toContain(0x56);
  });
  it("says why USB is unavailable instead of failing silently", () => {
    const s = usbSupport();
    expect(s.webusb).toBe(false);
    expect(s.reason.length).toBeGreaterThan(10);
  });
});
