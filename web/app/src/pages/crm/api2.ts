// Endpoints added in wave 3: custom fields, views, search, automation, capture, teams, products, 360, import, reports.
import type { FieldDef, Me, Report, SavedViewRow, UserLite } from "./types";

const BASE = "/api/v1/crm";

async function jf<T>(url: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, { credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json" }, ...init });
  if (!resp.ok) {
    const text = (await resp.text().catch(() => "")).trim();
    let msg = text;
    try {
      const j = JSON.parse(text) as { error?: string };
      if (j.error) msg = j.error;
    } catch {
      /* plain text */
    }
    throw new Error(msg || `HTTP ${resp.status}`);
  }
  return (await resp.json()) as T;
}
const body = (method: string, b?: unknown): RequestInit => ({ method, body: b === undefined ? undefined : JSON.stringify(b) });

/** list<T>("fields","fields") -> GET /crm/fields -> {fields: T[]} */
export async function list<T>(path: string, key: string): Promise<T[]> {
  const r = await jf<Record<string, T[]>>(`${BASE}/${path}`);
  return r[key] ?? [];
}
export const save = <T>(path: string, id: string, v: unknown) => jf<T>(`${BASE}/${path}${id ? "/" + id : ""}`, body(id ? "PUT" : "POST", v));
export const remove = (path: string, id: string) => jf<unknown>(`${BASE}/${path}/${id}`, body("DELETE"));
export const post = <T>(path: string, v?: unknown) => jf<T>(`${BASE}/${path}`, body("POST", v ?? {}));
export const put = <T>(path: string, v: unknown) => jf<T>(`${BASE}/${path}`, body("PUT", v));
export const get = <T>(path: string) => jf<T>(`${BASE}/${path}`);

export const getMe = () => jf<Me>(`${BASE}/me`).catch(() => ({ user_id: "", is_admin: false, scope: "all", auto_log_mail: false }) as Me);
export const getUsers = async () => (await jf<{ users: UserLite[] }>(`${BASE}/users`).catch(() => ({ users: [] as UserLite[] }))).users;
export const getFields = () => list<FieldDef>("fields", "fields").catch(() => [] as FieldDef[]);
export const getViews = (entity: string) => list<SavedViewRow>("views", "views").then((v) => v.filter((x) => x.entity === entity)).catch(() => [] as SavedViewRow[]);
export const search = async (q: string) => (await jf<{ hits: SearchHit[] }>(`${BASE}/search?q=${encodeURIComponent(q)}`)).hits;
export const runReport = (name: string, extra = "") => jf<Report>(`${BASE}/reports/${name}${extra}`);

export interface SearchHit {
  kind: "lead" | "company" | "deal" | "activity";
  id: string;
  title: string;
  sub: string;
}

/** Download a CSV/zip produced by the server (keeps the session cookie). */
export async function download(path: string, filename: string) {
  const r = await fetch(`${BASE}/${path}`, { credentials: "same-origin" });
  if (!r.ok) throw new Error((await r.text().catch(() => "")) || `HTTP ${r.status}`);
  const url = URL.createObjectURL(await r.blob());
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
