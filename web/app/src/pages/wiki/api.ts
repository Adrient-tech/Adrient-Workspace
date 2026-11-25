import { jf } from "../know/http";

export interface Space { id: string; name: string; icon: string; kind: "team" | "personal"; owner_id: string; org_access: "none" | "view" | "edit"; role: number }
export interface Member { user_id: string; name?: string; role: "viewer" | "editor" | "admin" }
export interface Page {
  id: string; space_id: string; parent_id?: string; title: string; icon: string; kind: "page" | "database"; position: number; version: number;
  content?: unknown; records_base_id?: string; records_table_id?: string; updated_at: string; updated_by?: string; trashed_at?: string | null; role?: number; favorite?: boolean;
}
export interface PageRef { id: string; space_id: string; space_name: string; title: string; icon: string; snippet?: string; updated_at: string }
export interface Comment { id: string; user_id: string; author: string; body: string; resolved: boolean; created_at: string }
export interface Version { id: string; title: string; saved_at: string; saved_by: string }

const B = "/api/v1/wiki";
export const wiki = {
  templates: () => jf<{ templates: { ID: string; Name: string }[] }>(`${B}/templates`).then((r) => r.templates),
  people: () => jf<{ people: { id: string; name: string; email: string }[]; me: string }>(`${B}/people`),
  spaces: () => jf<{ spaces: Space[] }>(`${B}/spaces`).then((r) => r.spaces),
  createSpace: (s: { name: string; icon?: string; org_access?: string }) => jf<Space>(`${B}/spaces`, { method: "POST", body: s }),
  updateSpace: (id: string, s: { name: string; icon?: string; org_access?: string }) => jf<{ ok: boolean }>(`${B}/spaces/${id}`, { method: "PUT", body: s }),
  deleteSpace: (id: string) => jf<{ ok: boolean }>(`${B}/spaces/${id}`, { method: "DELETE" }),
  members: (id: string) => jf<{ members: Member[] }>(`${B}/spaces/${id}/members`).then((r) => r.members),
  setMembers: (id: string, members: Member[]) => jf<{ ok: boolean }>(`${B}/spaces/${id}/members`, { method: "PUT", body: { members } }),
  tree: (spaceId: string, trashed = false) => jf<{ pages: Page[] }>(`${B}/spaces/${spaceId}/pages${trashed ? "?trashed=1" : ""}`).then((r) => r.pages),
  page: (id: string) => jf<Page>(`${B}/pages/${id}`),
  create: (p: { space_id: string; parent_id?: string; title?: string; icon?: string; kind?: string; template?: string }) => jf<Page>(`${B}/pages`, { method: "POST", body: p }),
  save: (id: string, p: { title?: string; icon?: string; content?: unknown; version?: number }) => jf<Page>(`${B}/pages/${id}`, { method: "PUT", body: p }),
  trash: (id: string) => jf<{ ok: boolean }>(`${B}/pages/${id}`, { method: "DELETE" }),
  remove: (id: string) => jf<{ ok: boolean }>(`${B}/pages/${id}?permanent=1`, { method: "DELETE" }),
  restore: (id: string) => jf<{ ok: boolean }>(`${B}/pages/${id}/restore`, { method: "POST" }),
  move: (id: string, m: { parent_id?: string; space_id?: string; index: number }) => jf<{ ok: boolean }>(`${B}/pages/${id}/move`, { method: "POST", body: m }),
  duplicate: (id: string) => jf<Page>(`${B}/pages/${id}/duplicate`, { method: "POST" }),
  favorite: (id: string, on: boolean) => jf<{ ok: boolean }>(`${B}/pages/${id}/favorite`, { method: on ? "PUT" : "DELETE" }),
  favorites: () => jf<{ pages: PageRef[] }>(`${B}/favorites`).then((r) => r.pages),
  recent: () => jf<{ pages: PageRef[] }>(`${B}/recent`).then((r) => r.pages),
  search: (q: string) => jf<{ pages: PageRef[] }>(`${B}/search?q=${encodeURIComponent(q)}`).then((r) => r.pages),
  backlinks: (id: string) => jf<{ pages: PageRef[] }>(`${B}/pages/${id}/backlinks`).then((r) => r.pages),
  versions: (id: string) => jf<{ versions: Version[] }>(`${B}/pages/${id}/versions`).then((r) => r.versions),
  restoreVersion: (id: string, v: string) => jf<Page>(`${B}/pages/${id}/versions/${v}/restore`, { method: "POST" }),
  comments: (id: string) => jf<{ comments: Comment[] }>(`${B}/pages/${id}/comments`).then((r) => r.comments),
  addComment: (id: string, body: string) => jf<Comment>(`${B}/pages/${id}/comments`, { method: "POST", body: { body } }),
  resolve: (cid: string, on: boolean) => jf<{ ok: boolean }>(`${B}/comments/${cid}/${on ? "resolve" : "reopen"}`, { method: "POST" }),
  delComment: (cid: string) => jf<{ ok: boolean }>(`${B}/comments/${cid}`, { method: "DELETE" }),
  importMd: (spaceId: string, md: string, parentId = "") => jf<Page>(`${B}/import?space_id=${spaceId}&parent_id=${parentId}`, { method: "POST", raw: md }),
  exportUrl: (id: string, format: "md" | "html") => `${B}/pages/${id}/export?format=${format}`,
};
