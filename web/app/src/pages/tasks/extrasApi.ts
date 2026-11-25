import { searchDirectory } from "../../api/directory";
import type { TaskExtras } from "./logic";
import type { Task } from "./types";

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(`/api/v1/tx/${path}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  if (r.status === 204) return undefined as T;
  return (await r.json()) as T;
}

export interface SharedList {
  id: string;
  name: string;
  owner_name: string;
  role: "viewer" | "editor";
}
export interface Share {
  user_id: string;
  email: string;
  name: string;
  role: string;
}
export interface Member {
  id: string;
  name: string;
  email: string;
}
export interface AssignedTask {
  id: string;
  list_id: string;
  list_name: string;
  title: string;
  notes: string;
  due_at: string;
  extras: TaskExtras;
}

export const listExtras = (listId: string) => call<{ extras: Record<string, TaskExtras> }>(`lists/${listId}/extras`).then((r) => r.extras ?? {});
export const saveExtras = (taskId: string, e: Partial<TaskExtras>) => call<TaskExtras>(`tasks/${taskId}/extras`, { method: "PUT", body: JSON.stringify(e) });
export const sharedLists = () => call<{ lists: SharedList[] }>("shared-lists").then((r) => r.lists);
export const assigned = () => call<{ tasks: AssignedTask[] }>("assigned").then((r) => r.tasks);
export const getShares = (listId: string) => call<{ shares: Share[] }>(`lists/${listId}/shares`).then((r) => r.shares);
export const addShare = (listId: string, email: string, role: string) => call<Share>(`lists/${listId}/shares`, { method: "POST", body: JSON.stringify({ email, role }) });
export const removeShare = (listId: string, userId: string) => call<void>(`lists/${listId}/shares/${userId}`, { method: "DELETE" });
export const quickTask = (b: { title: string; notes?: string; link?: string; due_at?: string }) => call<Task>("quick", { method: "POST", body: JSON.stringify(b) });
export const directory = (q: string) =>
  searchDirectory(q)
    .then((m) => m as Member[])
    .catch(() => [] as Member[]);
export interface LinkInfo {
  token: string;
  path: string;
}
export const getPublicLink = (listId: string) => call<LinkInfo>(`lists/${listId}/public`);
export const createPublicLink = (listId: string, rotate = false) => call<LinkInfo>(`lists/${listId}/public${rotate ? "?rotate=1" : ""}`, { method: "POST" });
export const revokePublicLink = (listId: string) => call<LinkInfo>(`lists/${listId}/public`, { method: "DELETE" });
export const getFeed = () => call<LinkInfo>("feed");
export const rotateFeed = () => call<LinkInfo>("feed/rotate", { method: "POST" });
