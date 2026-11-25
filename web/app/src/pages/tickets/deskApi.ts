// Desk extras: built-in views with counts, search filters, tags, types, merge, macros, bulk edit and the event trail.
const TS = "/api/v1/ts";

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(path, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  if (r.status === 204) return undefined as T;
  return (await r.json()) as T;
}

export type TicketType = "question" | "incident" | "problem" | "task";

export interface DeskTicket {
  id: string;
  project_id: string;
  ref: string;
  number: number;
  title: string;
  body?: string;
  status: string;
  priority: "low" | "normal" | "high" | "urgent";
  type: TicketType;
  tags: string[];
  requester_name: string;
  requester_email: string;
  assignee_user_id?: string;
  source: string;
  created_at: string;
  updated_at: string;
  resolved_at?: string;
  merged_into?: string;
  problem_id?: string;
}
export interface DeskView {
  id: string;
  title: string;
  count: number;
}
export interface TicketEvent {
  id: string;
  kind: string;
  actor_name: string;
  detail: Record<string, unknown>;
  created_at: string;
}
export interface MacroActions {
  status?: string;
  priority?: string;
  type?: string;
  assignee?: string;
  add_tags?: string[];
  remove_tags?: string[];
  reply?: string;
  internal?: boolean;
}
export interface Macro {
  id?: string;
  project_id?: string;
  name: string;
  actions: MacroActions;
}
export interface BulkChange {
  ids: string[];
  status?: string;
  priority?: string;
  type?: string;
  assignee?: string;
  add_tags?: string[];
  remove_tags?: string[];
}

export const desk = (pid: string, opts: { view?: string; q?: string } = {}) => {
  const p = new URLSearchParams();
  if (opts.view) p.set("view", opts.view);
  if (opts.q) p.set("q", opts.q);
  const qs = p.toString();
  return call<{ tickets: DeskTicket[]; views: DeskView[]; types: TicketType[] }>(`${TS}/projects/${pid}/desk${qs ? `?${qs}` : ""}`);
};
export const ticketDesk = (id: string) =>
  call<{ ticket: DeskTicket; events: TicketEvent[]; incidents: DeskTicket[]; followers: string[]; following: boolean }>(`${TS}/tickets/${id}/desk`);
export const setMeta = (id: string, m: { tags?: string[]; type?: TicketType; problem_id?: string }) =>
  call<DeskTicket>(`${TS}/tickets/${id}/meta`, { method: "PUT", body: JSON.stringify(m) });
export const bulk = (c: BulkChange) => call<{ changed: number }>(`${TS}/tickets/bulk`, { method: "POST", body: JSON.stringify(c) });
export const merge = (id: string, into: string) => call<DeskTicket>(`${TS}/tickets/${id}/merge`, { method: "POST", body: JSON.stringify({ into }) });
export const follow = (id: string) => call<{ following: boolean }>(`${TS}/tickets/${id}/follow`, { method: "POST", body: "{}" });
export const applyMacro = (id: string, macroId: string) => call<DeskTicket>(`${TS}/tickets/${id}/macro`, { method: "POST", body: JSON.stringify({ macro_id: macroId }) });
export const listMacros = (pid: string) => call<{ macros: Macro[] }>(`${TS}/macros?project=${encodeURIComponent(pid)}`).then((r) => r.macros);
export const saveMacro = (m: Macro) => call<Macro>(m.id ? `${TS}/macros/${m.id}` : `${TS}/macros`, { method: m.id ? "PUT" : "POST", body: JSON.stringify(m) });
export const deleteMacro = (id: string) => call<void>(`${TS}/macros/${id}`, { method: "DELETE" });
