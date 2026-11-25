import type {
  AdjustInput,
  ImportResult,
  InvHistoryEntry,
  InvLocation,
  InvProduct,
  InvProductSupplier,
  InvReport,
  InvSupplier,
  ProductInput,
  ReorderLine,
} from "./types";

const BASE = "/api/v1/inventory";

async function jsonFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${BASE}${path}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`;
    try {
      const body = (await resp.json()) as { message?: string };
      if (body.message) msg = body.message;
    } catch {
      /* keep the status text */
    }
    throw new Error(msg);
  }
  return (await resp.json()) as T;
}

const send = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

// ---- Products ----

export interface ProductQuery {
  query?: string;
  barcode?: string;
  category?: string;
  location_id?: string;
  include_archived?: boolean;
  archived_only?: boolean;
  low_stock_only?: boolean;
  flat?: boolean;
}

export async function listProducts(q: ProductQuery = {}): Promise<InvProduct[]> {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) {
    if (v !== undefined && v !== "" && v !== false) p.set(k, String(v));
  }
  const qs = p.toString() ? `?${p.toString()}` : "";
  const r = await jsonFetch<{ products: InvProduct[] }>(`/products${qs}`);
  return r.products ?? [];
}

/** One keyset page of products (families with their variants unless flat). */
export async function listProductsPage(
  q: { q?: string; category?: string; location_id?: string; low?: boolean; archived?: boolean; flat?: boolean; limit?: number },
  cursor = "",
): Promise<{ items: InvProduct[]; next: string }> {
  const p = new URLSearchParams({ limit: String(q.limit ?? 50) });
  if (q.q) p.set("q", q.q);
  if (q.category) p.set("category", q.category);
  if (q.location_id) p.set("location_id", q.location_id);
  if (q.low) p.set("low", "1");
  if (q.archived) p.set("archived", "1");
  if (q.flat) p.set("flat", "1");
  if (cursor) p.set("cursor", cursor);
  const r = await fetch(`/api/v1/stock/products/page?${p}`, { credentials: "same-origin" });
  if (!r.ok) throw new Error((await r.text()).trim() || `HTTP ${r.status}`);
  const d = (await r.json()) as { items: InvProduct[]; next_cursor?: string };
  return { items: d.items ?? [], next: d.next_cursor ?? "" };
}

export const getProduct = (id: string) =>
  jsonFetch<InvProduct>(`/products/${id}`);

export const createProduct = (input: ProductInput) =>
  jsonFetch<InvProduct>("/products", send("POST", input));

export const updateProduct = (id: string, input: ProductInput) =>
  jsonFetch<InvProduct>(`/products/${id}`, send("PATCH", input));

export const archiveProduct = (id: string, archived: boolean) =>
  jsonFetch<InvProduct>(`/products/${id}/archive`, send("POST", { archived }));

export async function deleteProduct(id: string): Promise<void> {
  await jsonFetch<unknown>(`/products/${id}`, send("DELETE"));
}

export const setProductSuppliers = (
  id: string,
  suppliers: Partial<InvProductSupplier>[],
) => jsonFetch<InvProduct>(`/products/${id}/suppliers`, send("PUT", { suppliers }));

export const importProducts = (
  mode: "update" | "add" | "skip",
  rows: ProductInput[],
  location_names: string[],
) =>
  jsonFetch<ImportResult>("/import", send("POST", { mode, rows, location_names }));

// ---- Stock ----

export const adjustStock = (id: string, input: AdjustInput) =>
  jsonFetch<InvProduct>(`/products/${id}/adjust`, send("POST", input));

export const transferStock = (
  id: string,
  from_location_id: string,
  to_location_id: string,
  qty: number,
  note = "",
) =>
  jsonFetch<InvProduct>(
    `/products/${id}/transfer`,
    send("POST", { from_location_id, to_location_id, qty, note }),
  );

export async function listHistory(opts: {
  product_id?: string;
  reason?: string;
  since?: string;
  limit?: number;
} = {}): Promise<InvHistoryEntry[]> {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(opts)) {
    if (v !== undefined && v !== "") p.set(k, String(v));
  }
  const qs = p.toString() ? `?${p.toString()}` : "";
  const r = await jsonFetch<{ entries: InvHistoryEntry[] }>(`/history${qs}`);
  return r.entries ?? [];
}

export async function reorderList(): Promise<ReorderLine[]> {
  const r = await jsonFetch<{ lines: ReorderLine[] }>("/reorder");
  return r.lines ?? [];
}

export const getReport = () => jsonFetch<InvReport>("/report");

// ---- Locations ----

export async function listLocations(): Promise<InvLocation[]> {
  const r = await jsonFetch<{ locations: InvLocation[] }>("/locations");
  return r.locations ?? [];
}

export const createLocation = (l: Partial<InvLocation>) =>
  jsonFetch<InvLocation>("/locations", send("POST", l));

export const updateLocation = (id: string, l: Partial<InvLocation>) =>
  jsonFetch<InvLocation>(`/locations/${id}`, send("PATCH", l));

export async function deleteLocation(id: string): Promise<void> {
  await jsonFetch<unknown>(`/locations/${id}`, send("DELETE"));
}

// ---- Suppliers ----

export async function listSuppliers(): Promise<InvSupplier[]> {
  const r = await jsonFetch<{ suppliers: InvSupplier[] }>("/suppliers");
  return r.suppliers ?? [];
}

export const createSupplier = (s: Partial<InvSupplier>) =>
  jsonFetch<InvSupplier>("/suppliers", send("POST", s));

export const updateSupplier = (id: string, s: Partial<InvSupplier>) =>
  jsonFetch<InvSupplier>(`/suppliers/${id}`, send("PATCH", s));

export async function deleteSupplier(id: string): Promise<void> {
  await jsonFetch<unknown>(`/suppliers/${id}`, send("DELETE"));
}

// ---- Contacts link + photos ----

export interface ContactLite {
  id: string;
  display_name: string;
  company: string;
  emails: string[];
  phones: string[];
}

export async function searchContacts(): Promise<ContactLite[]> {
  const r = await fetch("/api/v1/contacts", { credentials: "same-origin" });
  if (!r.ok) return [];
  const body = (await r.json()) as { contacts?: ContactLite[] };
  return body.contacts ?? [];
}

/** Upload a product photo to Drive and return the Drive file id. */
export async function uploadPhoto(file: File): Promise<string> {
  const fd = new FormData();
  fd.append("file", file);
  const r = await fetch("/api/v1/drive/files/upload", {
    method: "POST",
    credentials: "same-origin",
    body: fd,
  });
  if (!r.ok) throw new Error(`photo upload failed: ${r.status}`);
  const f = (await r.json()) as { id: string };
  return f.id;
}

export const photoURL = (fileId: string) => `/api/v1/drive/files/${fileId}/content`;
