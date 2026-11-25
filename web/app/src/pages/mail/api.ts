import type {
  MailMessage,
  MailAttachment,
  ListMessagesResponse,
  ListThreadsResponse,
  GetThreadResponse,
  SendInput,
  ModifyInput,
  MailRule,
  RuleInput,
  MailLabelEntity,
  ListLabelsResponse2,
  MailFilter,
  FilterInput,
  MailSavedSearch,
  MailSignature,
} from "./types";

const API_BASE = "/api/v1";

async function jsonFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${API_BASE}${path}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return (await resp.json()) as T;
}

export function listMessages(
  folder: string,
  opts?: { label?: string; query?: string; starred?: boolean },
): Promise<ListMessagesResponse> {
  const q = new URLSearchParams();
  if (folder) q.set("folder", folder);
  if (opts?.label) q.set("label", opts.label);
  if (opts?.query) q.set("query", opts.query);
  if (opts?.starred) q.set("starred", "true");
  return jsonFetch<ListMessagesResponse>(`/mail/messages?${q.toString()}`);
}

export function getMessage(id: string): Promise<MailMessage> {
  return jsonFetch<MailMessage>(`/mail/messages/${id}`);
}

// getRawSource returns the full RFC822 source (headers + body) of a message for
// "Show original" inspection. Plain text, not JSON.
export async function getRawSource(id: string): Promise<string> {
  const resp = await fetch(
    `${API_BASE}/mail/messages/${encodeURIComponent(id)}/raw`,
    { credentials: "same-origin", headers: { Accept: "text/plain" } },
  );
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return resp.text();
}

export function listThreads(
  folder: string,
  opts?: { label?: string; query?: string; starred?: boolean },
): Promise<ListThreadsResponse> {
  const q = new URLSearchParams();
  if (folder) q.set("folder", folder);
  if (opts?.label) q.set("label", opts.label);
  if (opts?.query) q.set("query", opts.query);
  if (opts?.starred) q.set("starred", "true");
  return jsonFetch<ListThreadsResponse>(`/mail/threads?${q.toString()}`);
}

export async function getThread(
  threadId: string,
  folder?: string,
): Promise<MailMessage[]> {
  const q = new URLSearchParams();
  if (folder) q.set("folder", folder);
  const r = await jsonFetch<GetThreadResponse>(
    `/mail/threads/${threadId}?${q.toString()}`,
  );
  return r.messages ?? [];
}

export async function listLabels(): Promise<string[]> {
  const r = await jsonFetch<ListLabelsResponse2>("/mail/labels");
  return r.labels ?? [];
}

export async function listLabelsWithEntities(): Promise<ListLabelsResponse2> {
  const r = await jsonFetch<ListLabelsResponse2>("/mail/labels");
  return { labels: r.labels ?? [], label_objects: r.label_objects ?? [] };
}

// --- Label entity CRUD ---
export function createMailLabel(
  name: string,
  color: string,
): Promise<MailLabelEntity> {
  return jsonFetch<MailLabelEntity>("/mail/labels", {
    method: "POST",
    body: JSON.stringify({ name, color }),
  });
}
export function updateMailLabel(
  id: string,
  name: string,
  color: string,
): Promise<MailLabelEntity> {
  return jsonFetch<MailLabelEntity>(`/mail/labels/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ id, name, color }),
  });
}
export async function deleteMailLabel(id: string): Promise<void> {
  await jsonFetch<unknown>(`/mail/labels/${id}`, { method: "DELETE" });
}
export async function applyMailLabel(
  messageId: string,
  labelId: string,
): Promise<void> {
  await jsonFetch<unknown>(`/mail/messages/${messageId}/labels/${labelId}`, {
    method: "POST",
    body: "{}",
  });
}
export async function removeMailLabel(
  messageId: string,
  labelId: string,
): Promise<void> {
  await jsonFetch<unknown>(`/mail/messages/${messageId}/labels/${labelId}`, {
    method: "DELETE",
  });
}

export function sendMessage(input: SendInput): Promise<MailMessage> {
  return jsonFetch<MailMessage>("/mail/messages", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function modifyMessage(
  id: string,
  input: ModifyInput,
): Promise<MailMessage> {
  return jsonFetch<MailMessage>(`/mail/messages/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export async function deleteMessage(id: string): Promise<void> {
  await jsonFetch<unknown>(`/mail/messages/${id}`, { method: "DELETE" });
}

// --- rules / filters ---
export async function listRules(): Promise<MailRule[]> {
  const r = await jsonFetch<{ rules: MailRule[] }>("/mail/rules");
  return r.rules ?? [];
}
export function createRule(input: RuleInput): Promise<MailRule> {
  return jsonFetch<MailRule>("/mail/rules", {
    method: "POST",
    body: JSON.stringify(input),
  });
}
export async function deleteRule(id: string): Promise<void> {
  await jsonFetch<unknown>(`/mail/rules/${id}`, { method: "DELETE" });
}

// --- normalized filters ---
export async function listFilters(): Promise<MailFilter[]> {
  const r = await jsonFetch<{ filters: MailFilter[] }>("/mail/filters");
  return r.filters ?? [];
}
export function createFilter(input: FilterInput): Promise<MailFilter> {
  return jsonFetch<MailFilter>("/mail/filters", {
    method: "POST",
    body: JSON.stringify(input),
  });
}
export async function deleteFilter(id: string): Promise<void> {
  await jsonFetch<unknown>(`/mail/filters/${id}`, { method: "DELETE" });
}
export async function applyFiltersNow(): Promise<{ modified: number }> {
  return jsonFetch<{ modified: number }>("/mail/filters:apply", {
    method: "POST",
    body: "{}",
  });
}

// --- attachments ---
export async function uploadAttachments(
  files: FileList | File[],
): Promise<MailAttachment[]> {
  const fd = new FormData();
  for (const f of Array.from(files)) fd.append("file", f);
  const resp = await fetch(`${API_BASE}/mail/attachments`, {
    method: "POST",
    credentials: "same-origin",
    body: fd,
  });
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  const r = (await resp.json()) as { attachments: MailAttachment[] };
  return r.attachments ?? [];
}
export interface SavedDriveFile {
  id: string;
  name: string;
  mime_type: string;
  size: number;
}

/** Copies a mail attachment into Drive. folderId: "" = the "Mail attachments"
 *  folder, "root" = My Drive root, else a folder id. Deduped per attachment. */
export async function saveAttachmentToDrive(
  messageId: string,
  attachmentId: string,
  folderId = "",
): Promise<SavedDriveFile> {
  const resp = await fetch(
    `${API_BASE}/mail/messages/${messageId}/attachments/${attachmentId}/save-to-drive`,
    {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ folder_id: folderId }),
    },
  );
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return ((await resp.json()) as { file: SavedDriveFile }).file;
}

/** Copies a Drive file into a real mail attachment (no download/upload). */
export async function attachFromDrive(fileId: string): Promise<MailAttachment[]> {
  const resp = await fetch(`${API_BASE}/mail/attachments/from-drive`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ file_id: fileId }),
  });
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return ((await resp.json()) as { attachments: MailAttachment[] }).attachments ?? [];
}

export function attachmentURL(id: string): string {
  return `${API_BASE}/mail/attachments/${id}/content`;
}

// --- Saved searches & signatures ---
export async function listSavedSearches(): Promise<MailSavedSearch[]> {
  const r = await jsonFetch<{ searches: MailSavedSearch[] }>("/mail/saved-searches");
  return r.searches ?? [];
}
export function saveSearch(name: string, query: string): Promise<MailSavedSearch> {
  return jsonFetch<MailSavedSearch>("/mail/saved-searches", {
    method: "POST",
    body: JSON.stringify({ name, query }),
  });
}
export async function deleteSavedSearch(id: string): Promise<void> {
  await jsonFetch<unknown>(`/mail/saved-searches/${id}`, { method: "DELETE" });
}
export async function listSignatures(): Promise<MailSignature[]> {
  const r = await jsonFetch<{ signatures: MailSignature[] }>("/mail/signatures");
  return r.signatures ?? [];
}
export function saveSignature(input: { name: string; body: string; is_default: boolean }): Promise<MailSignature> {
  return jsonFetch<MailSignature>("/mail/signatures", { method: "POST", body: JSON.stringify(input) });
}
export async function deleteSignature(id: string): Promise<void> {
  await jsonFetch<unknown>(`/mail/signatures/${id}`, { method: "DELETE" });
}
