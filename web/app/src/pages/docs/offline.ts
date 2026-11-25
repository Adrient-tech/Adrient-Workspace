// "Available offline": a per-document flag plus a local copy of the Yjs
// state in IndexedDB. When the flag is on, the editor loads the stored state
// before connecting (so the document opens with no network) and writes every
// change back. Yjs merges the local edits with the server when the
// connection returns. Nothing is stored for documents that are not flagged.
import * as Y from "yjs";

const FLAG = "adrient.docs.offline";
const DB = "adrient-docs-offline";
const STORE = "state";

export function offlineIds(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(FLAG) ?? "[]");
    return Array.isArray(v) ? v.filter((x) => typeof x === "string") : [];
  } catch {
    return [];
  }
}
export const isOffline = (id: string) => offlineIds().includes(id);

export function setOffline(id: string, on: boolean) {
  const set = new Set(offlineIds());
  if (on) set.add(id);
  else set.delete(id);
  localStorage.setItem(FLAG, JSON.stringify([...set]));
  window.dispatchEvent(new CustomEvent("adrient-docs-offline-changed"));
  if (!on) void idbDelete(id);
}

function open(): Promise<IDBDatabase> {
  return new Promise((res, rej) => {
    const r = indexedDB.open(DB, 1);
    r.onupgradeneeded = () => r.result.createObjectStore(STORE);
    r.onsuccess = () => res(r.result);
    r.onerror = () => rej(r.error);
  });
}
async function idbGet(id: string): Promise<Uint8Array | null> {
  const db = await open();
  return new Promise((res, rej) => {
    const q = db.transaction(STORE).objectStore(STORE).get(id);
    q.onsuccess = () => res((q.result as Uint8Array) ?? null);
    q.onerror = () => rej(q.error);
  });
}
async function idbPut(id: string, bytes: Uint8Array): Promise<void> {
  const db = await open();
  return new Promise((res, rej) => {
    const tx = db.transaction(STORE, "readwrite");
    tx.objectStore(STORE).put(bytes, id);
    tx.oncomplete = () => res();
    tx.onerror = () => rej(tx.error);
  });
}
async function idbDelete(id: string): Promise<void> {
  try {
    const db = await open();
    db.transaction(STORE, "readwrite").objectStore(STORE).delete(id);
  } catch {
    /* ignore */
  }
}

/** attachOffline loads and keeps the local copy of `ydoc` for `id` when the
 *  document is flagged available offline. Returns a detach function. */
export function attachOffline(id: string, ydoc: Y.Doc): () => void {
  if (typeof indexedDB === "undefined" || !isOffline(id)) return () => {};
  let timer: ReturnType<typeof setTimeout> | undefined;
  let dead = false;
  const save = () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      if (!dead && isOffline(id)) void idbPut(id, Y.encodeStateAsUpdate(ydoc)).catch(() => {});
    }, 500);
  };
  idbGet(id)
    .then((b) => {
      if (b && !dead) Y.applyUpdate(ydoc, b, "offline");
    })
    .catch(() => {})
    .finally(() => {
      if (!dead) ydoc.on("update", save);
    });
  return () => {
    dead = true;
    clearTimeout(timer);
    ydoc.off("update", save);
  };
}

/** Writes the current state right away (used when the flag is switched on). */
export async function snapshotOffline(id: string, ydoc: Y.Doc): Promise<void> {
  await idbPut(id, Y.encodeStateAsUpdate(ydoc));
}
