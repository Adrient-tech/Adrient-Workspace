import { searchDirectory } from "../../api/directory";
import type { SlaInfo } from "./helpLogic";

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
const TS = "/api/v1/ts";

export interface SlaPolicy {
  priority: string;
  first_response_min: number;
  resolve_min: number;
}
export interface Rule {
  id?: string;
  name: string;
  enabled: boolean;
  match_keyword: string;
  match_priority: string;
  match_source: string;
  set_assignee: string;
  set_priority: string;
  set_status: string;
}
export interface Canned {
  id: string;
  project_id: string;
  title: string;
  body: string;
}
export interface Customer {
  email: string;
  name: string;
  organisation: string;
  total: number;
  open: number;
  last_at: string;
}
export interface Article {
  id: string;
  slug: string;
  title: string;
  body: string;
  published: boolean;
  updated_at: string;
}
export interface Report {
  created: number;
  resolved: number;
  open_now: number;
  avg_first_response_min: number;
  avg_resolution_min: number;
  sla_met_pct: number;
  sla_checked: number;
  csat_avg: number;
  csat_count: number;
  by_priority: Record<string, number>;
  by_assignee: Record<string, number>;
  created_per_day: { day: string; count: number }[];
  resolved_per_day: { day: string; count: number }[];
}
export interface Csat {
  token?: string;
  ref: string;
  title: string;
  rating: number | null;
  comment: string;
  answered: boolean;
}
export interface Member {
  id: string;
  name: string;
  email: string;
}

export const getSla = (pid: string) => call<{ policies: SlaPolicy[] }>(`${TS}/projects/${pid}/sla`).then((r) => r.policies);
export const saveSla = (pid: string, policies: SlaPolicy[]) => call(`${TS}/projects/${pid}/sla`, { method: "PUT", body: JSON.stringify({ policies }) });
export const slaStates = (pid: string) => call<{ states: Record<string, SlaInfo> }>(`${TS}/projects/${pid}/sla-state`).then((r) => r.states);
export const listRules = (pid: string) => call<{ rules: Rule[] }>(`${TS}/projects/${pid}/rules`).then((r) => r.rules);
export const createRule = (pid: string, r: Rule) => call<Rule>(`${TS}/projects/${pid}/rules`, { method: "POST", body: JSON.stringify(r) });
export const updateRule = (pid: string, r: Rule) => call<Rule>(`${TS}/rules/${r.id}`, { method: "PUT", body: JSON.stringify({ ...r, project_id: pid }) });
export const deleteRule = (id: string) => call<void>(`${TS}/rules/${id}`, { method: "DELETE" });
export const listCanned = (pid: string) => call<{ canned: Canned[] }>(`${TS}/canned?project=${encodeURIComponent(pid)}`).then((r) => r.canned);
export const createCanned = (projectId: string, title: string, body: string) => call<Canned>(`${TS}/canned`, { method: "POST", body: JSON.stringify({ ProjectID: projectId, Title: title, Body: body }) });
export const deleteCanned = (id: string) => call<void>(`${TS}/canned/${id}`, { method: "DELETE" });
export const customers = (pid: string, q: string) => call<{ customers: Customer[] }>(`${TS}/customers?project=${encodeURIComponent(pid)}&q=${encodeURIComponent(q)}`).then((r) => r.customers);
export const listArticles = (pid: string) => call<{ articles: Article[] }>(`${TS}/projects/${pid}/kb`).then((r) => r.articles);
export const saveArticle = (pid: string, a: Partial<Article>) => call<Article>(`${TS}/projects/${pid}/kb`, { method: a.id ? "PUT" : "POST", body: JSON.stringify(a) });
export const deleteArticle = (id: string) => call<void>(`${TS}/kb/${id}`, { method: "DELETE" });
export const report = (pid: string, days: number) => call<Report>(`${TS}/projects/${pid}/report?days=${days}`);
export const csatFor = (ticketId: string) => call<Csat>(`${TS}/tickets/${ticketId}/csat`);
export const members = (q = "") =>
  searchDirectory(q)
    .then((m) => m as Member[])
    .catch(() => [] as Member[]);

export const publicHelp = (token: string) => call<{ name: string; articles: Article[] }>(`/api/v1/public/help/${encodeURIComponent(token)}`);
export const publicCsat = (token: string) => call<Csat>(`/api/v1/public/csat/${encodeURIComponent(token)}`);
export const answerCsat = (token: string, rating: number, comment: string) => call<{ ok: boolean }>(`/api/v1/public/csat/${encodeURIComponent(token)}`, { method: "POST", body: JSON.stringify({ rating, comment }) });
