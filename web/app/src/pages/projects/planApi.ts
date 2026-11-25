import type { IssuePlan, Milestone } from "./plan";

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(`/api/v1/px/${path}`, { credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json" }, ...init });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  if (r.status === 204) return undefined as T;
  return (await r.json()) as T;
}

export interface SavedView {
  id: string;
  name: string;
  mode: string;
  filters: Record<string, unknown>;
  shared: boolean;
  mine: boolean;
}
export interface Link {
  id: string;
  kind: "crm_deal" | "hr_employee";
  ref_id: string;
  label: string;
}
export interface TemplateInfo {
  id: string;
  name: string;
  description: string;
  issues: number;
  milestones: number;
}

export const listPlans = () => call<{ extras: Record<string, IssuePlan> }>("extras").then((r) => r.extras ?? {});
export const savePlan = (id: string, p: IssuePlan) => call<IssuePlan>(`issues/${id}/extras`, { method: "PUT", body: JSON.stringify(p) });
export const logHours = (id: string, hours: number) => call<{ logged_hours: number }>(`issues/${id}/log`, { method: "POST", body: JSON.stringify({ hours }) });
export const listMilestones = (project = "") => call<{ milestones: Milestone[] }>(`milestones?project=${encodeURIComponent(project)}`).then((r) => r.milestones);
export const createMilestone = (projectId: string, m: Partial<Milestone>) => call<Milestone>(`projects/${projectId}/milestones`, { method: "POST", body: JSON.stringify(m) });
export const updateMilestone = (m: Milestone) => call<Milestone>(`milestones/${m.id}`, { method: "PUT", body: JSON.stringify(m) });
export const deleteMilestone = (id: string) => call<void>(`milestones/${id}`, { method: "DELETE" });
export const getBrief = (projectId: string) => call<{ body: string }>(`projects/${projectId}/brief`).then((r) => r.body);
export const saveBrief = (projectId: string, body: string) => call(`projects/${projectId}/brief`, { method: "PUT", body: JSON.stringify({ body }) });
export const listViews = () => call<{ views: SavedView[] }>("views").then((r) => r.views);
export const createView = (v: { name: string; mode: string; filters: Record<string, unknown>; shared: boolean }) => call<SavedView>("views", { method: "POST", body: JSON.stringify(v) });
export const deleteView = (id: string) => call<void>(`views/${id}`, { method: "DELETE" });
export const templates = () => call<{ templates: TemplateInfo[] }>("templates").then((r) => r.templates);
export const fromTemplate = (b: { template: string; name: string; team_id: string; start_date: string }) => call<{ id: string; name: string }>("from-template", { method: "POST", body: JSON.stringify(b) });
export const listLinks = (projectId: string) => call<{ links: Link[] }>(`projects/${projectId}/links`).then((r) => r.links);
export const addLink = (projectId: string, kind: string, refId: string) => call(`projects/${projectId}/links`, { method: "POST", body: JSON.stringify({ kind, ref_id: refId }) });
export const deleteLink = (id: string) => call<void>(`links/${id}`, { method: "DELETE" });
export const pick = (kind: string, q: string) => call<{ items: { id: string; label: string }[] }>(`pick?kind=${kind}&q=${encodeURIComponent(q)}`).then((r) => r.items);
export interface WipLimit { status: string; max: number }
export const listWip = (teamId: string) => call<{ limits: WipLimit[] }>(`wip?team=${encodeURIComponent(teamId)}`).then((r) => r.limits ?? []);
export const setWip = (teamId: string, status: string, max: number) => call("wip", { method: "PUT", body: JSON.stringify({ team_id: teamId, status, max }) });
export const archiveIssues = (ids: string[], archived: boolean) => call<{ changed: number }>("archive", { method: "POST", body: JSON.stringify({ ids, archived }) }).then((r) => r.changed);
