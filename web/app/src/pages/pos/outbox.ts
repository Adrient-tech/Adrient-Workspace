// Offline sales outbox. A sale is saved on this device (IndexedDB) with a
// client-made idempotency key and sent to /api/v1/pos/x/sales/complete. The
// server creates the sale once per key, so retrying after a lost response or
// a flaky connection can never produce a duplicate. The store is an interface
// so the logic is testable without a browser.

export interface OutboxEntry {
  key: string; // idempotency key
  created_at: string;
  sale: unknown; // PosSaleInput JSON
  approval_pin?: string;
  summary: { total: number; lines: number; customer: string; register: string };
  status: "queued" | "failed";
  error: string;
  attempts: number;
}

export interface OutboxStore {
  list(): Promise<OutboxEntry[]>;
  put(e: OutboxEntry): Promise<void>;
  remove(key: string): Promise<void>;
}

/** Keeps the outbox in memory: the fallback when IndexedDB is unavailable, and the test double. */
export class MemoryStore implements OutboxStore {
  private m = new Map<string, OutboxEntry>();
  async list() {
    return [...this.m.values()].sort((a, b) => a.created_at.localeCompare(b.created_at));
  }
  async put(e: OutboxEntry) {
    this.m.set(e.key, { ...e });
  }
  async remove(key: string) {
    this.m.delete(key);
  }
}

const DB_NAME = "adrient-pos";
const STORE = "outbox";

/** IndexedDB-backed outbox: survives a closed tab or a reboot. */
export class IdbStore implements OutboxStore {
  private db: Promise<IDBDatabase> | null = null;
  private open(): Promise<IDBDatabase> {
    if (!this.db) {
      this.db = new Promise((resolve, reject) => {
        const req = indexedDB.open(DB_NAME, 1);
        req.onupgradeneeded = () => req.result.createObjectStore(STORE, { keyPath: "key" });
        req.onsuccess = () => resolve(req.result);
        req.onerror = () => reject(req.error);
      });
    }
    return this.db;
  }
  private async tx<T>(mode: IDBTransactionMode, fn: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
    const db = await this.open();
    return new Promise((resolve, reject) => {
      const req = fn(db.transaction(STORE, mode).objectStore(STORE));
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
  }
  async list() {
    const all = await this.tx<OutboxEntry[]>("readonly", (s) => s.getAll() as IDBRequest<OutboxEntry[]>);
    return all.sort((a, b) => a.created_at.localeCompare(b.created_at));
  }
  async put(e: OutboxEntry) {
    await this.tx("readwrite", (s) => s.put(e));
  }
  async remove(key: string) {
    await this.tx("readwrite", (s) => s.delete(key));
  }
}

export function defaultStore(): OutboxStore {
  return typeof indexedDB !== "undefined" ? new IdbStore() : new MemoryStore();
}

/** A fresh idempotency key: time-ordered, random enough that two devices never collide. */
export function newIdemKey(): string {
  const rnd = typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `${Math.random().toString(36).slice(2)}${Math.random().toString(36).slice(2)}`;
  return `pos-${Date.now().toString(36)}-${rnd}`.slice(0, 80);
}

export type Outcome = "done" | "permanent" | "retry";

/** Decides what a server answer means for the outbox. 2xx done; 4xx other than busy/rate limit is permanent; the rest is retried later. */
export function classifyStatus(status: number): Outcome {
  if (status >= 200 && status < 300) return "done";
  if (status === 409 || status === 408 || status === 429) return "retry"; // busy or slow down
  if (status === 422 || (status >= 400 && status < 500)) return "permanent";
  return "retry";
}

export interface SendResult {
  status: number; // 0 = network failure
  message?: string;
}
export type Sender = (e: OutboxEntry) => Promise<SendResult>;

export interface FlushReport {
  sent: number;
  failed: number;
  waiting: number;
  stoppedOffline: boolean;
}

/**
 * Sends queued sales oldest first. A network failure stops the run (the rest
 * would fail too); a permanent rejection marks that sale failed and goes on, so
 * one bad sale never blocks the queue; a retryable answer leaves it queued.
 */
export async function flushOutbox(store: OutboxStore, send: Sender): Promise<FlushReport> {
  const rep: FlushReport = { sent: 0, failed: 0, waiting: 0, stoppedOffline: false };
  const entries = (await store.list()).filter((e) => e.status === "queued");
  for (const e of entries) {
    let r: SendResult;
    try {
      r = await send(e);
    } catch {
      r = { status: 0 };
    }
    if (r.status === 0) {
      rep.stoppedOffline = true;
      rep.waiting = entries.length - rep.sent - rep.failed;
      return rep;
    }
    const o = classifyStatus(r.status);
    if (o === "done") {
      await store.remove(e.key);
      rep.sent++;
    } else if (o === "permanent") {
      await store.put({ ...e, status: "failed", error: r.message || `Rejected (${r.status})`, attempts: e.attempts + 1 });
      rep.failed++;
    } else {
      await store.put({ ...e, attempts: e.attempts + 1, error: r.message || "" });
      rep.waiting++;
    }
  }
  return rep;
}

/** Posts one entry to the server. Exposed separately so tests can swap it. */
export const httpSender: Sender = async (e) => {
  try {
    const resp = await fetch("/api/v1/pos/x/sales/complete", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ idempotency_key: e.key, sale: e.sale, approval_pin: e.approval_pin }),
    });
    let message = "";
    if (!resp.ok) {
      try {
        const b = (await resp.json()) as { error?: string; message?: string };
        message = b.error || b.message || "";
      } catch {
        /* no body */
      }
    }
    return { status: resp.status, message };
  } catch {
    return { status: 0 };
  }
};
