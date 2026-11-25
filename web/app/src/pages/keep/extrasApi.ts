export interface Notebook {
  id: string;
  name: string;
}
export interface AttachmentRef {
  id: string;
  kind: "image" | "drawing";
}
export interface KeepIndex {
  notebooks: Notebook[];
  note_notebook: Record<string, string>;
  attachments: Record<string, AttachmentRef[]>;
}

export const emptyIndex = (): KeepIndex => ({ notebooks: [], note_notebook: {}, attachments: {} });

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(`/api/v1/kx/${path}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  if (r.status === 204) return undefined as T;
  return (await r.json()) as T;
}

export const loadIndex = () => call<KeepIndex>("index");
export const createNotebook = (name: string) => call<Notebook>("notebooks", { method: "POST", body: JSON.stringify({ name }) });
export const renameNotebook = (id: string, name: string) => call<Notebook>(`notebooks/${id}`, { method: "PUT", body: JSON.stringify({ name }) });
export const deleteNotebook = (id: string) => call<void>(`notebooks/${id}`, { method: "DELETE" });
export const moveNote = (noteId: string, notebookId: string) => call<{ ok: boolean }>(`notes/${noteId}/notebook`, { method: "PUT", body: JSON.stringify({ notebook_id: notebookId }) });
export const attachmentUrl = (id: string) => `/api/v1/kx/attachments/${id}`;

export async function uploadAttachment(noteId: string, kind: "image" | "drawing", blob: Blob): Promise<AttachmentRef> {
  const r = await fetch(`/api/v1/kx/notes/${noteId}/attachments?kind=${kind}`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": blob.type || "application/octet-stream" },
    body: blob,
  });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  return (await r.json()) as AttachmentRef;
}
export const deleteAttachment = (id: string) => call<void>(`attachments/${id}`, { method: "DELETE" });

/** Accepts only the image types the server accepts and files up to 2 MB. */
export function checkImageFile(f: { type: string; size: number }): string {
  if (!["image/png", "image/jpeg", "image/gif", "image/webp"].includes(f.type)) return "Choose a PNG, JPEG, GIF or WebP image.";
  if (f.size > 2 * 1024 * 1024) return "That image is larger than 2 MB.";
  if (f.size === 0) return "That file is empty.";
  return "";
}

/** Notes that belong to a notebook, in the order given. */
export function notesInNotebook<T extends { id: string }>(notes: T[], idx: KeepIndex, notebookId: string): T[] {
  return notes.filter((n) => idx.note_notebook[n.id] === notebookId);
}
