export interface Extras {
  birthday: string;
  anniversary: string;
  website: string;
  address: string;
  nickname: string;
}
export const emptyExtras = (): Extras => ({ birthday: "", anniversary: "", website: "", address: "", nickname: "" });

export interface Links {
  events: { id: string; title: string; start: string }[];
  crm_status: string;
  deals: { id: string; title: string; stage: string; value: number; currency: string }[];
}
export interface HistoryItem {
  id: string;
  contact_id: string;
  name: string;
  action: "update" | "trash";
  at: string;
  undone: boolean;
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(`/api/v1/cx/${path}`, { credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json" }, ...init });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  return (await r.json()) as T;
}

export const getExtras = (id: string) => call<Extras>(`${id}/extras`);
export const saveExtras = (id: string, e: Partial<Extras>) => call<Extras>(`${id}/extras`, { method: "PUT", body: JSON.stringify({ ...emptyExtras(), ...e }) });
export const getLinks = (id: string) => call<Links>(`${id}/links`);
export const getHistory = () => call<{ history: HistoryItem[] }>("history").then((r) => r.history);
export const undoChange = (id: string) => call<{ ok: boolean }>(`history/${id}/undo`, { method: "POST" });
