// Client for /api/v1/import/wizard (see internal/cloudimport/wizard.go).

export type Kind =
  "users" | "groups" | "files" | "mail" | "calendar" | "contacts" | "chat";

export interface Field {
  key: string;
  label: string;
  type: "text" | "password" | "url" | "select";
  placeholder?: string;
  options?: string[];
  required: boolean;
  secret: boolean;
}

export interface Provider {
  id: string;
  name: string;
  short: string;
  auth: "oauth" | "password";
  scopes: ("me" | "org")[];
  kinds: Kind[];
  needs_app: boolean;
  configured: boolean;
  notes: string;
  setup_doc: string;
  fields: Field[] | null;
}

export interface Connection {
  id: string;
  provider: string;
  label: string;
  scope: "me" | "org";
  config: Record<string, string>;
  status: "ready" | "needs_auth" | "error";
  created_at: string;
  mine: boolean;
}

export interface Counter {
  seen: number;
  imported: number;
  skipped: number;
  failed: number;
  bytes?: number;
  note?: string;
}

export interface Run {
  id: string;
  connection_id: string;
  kinds: Kind[];
  dry_run: boolean;
  status:
    "queued" | "running" | "done" | "failed" | "cancelled" | "interrupted";
  counters: Record<string, Counter> | null;
  message: string;
  created_at: string;
  updated_at: string;
  finished_at?: string;
  error_count: number;
}

export interface UserMapRow {
  remote_id: string;
  remote_email: string;
  remote_name: string;
  local_user_id: string;
  local_email: string;
  skip: boolean;
}

export interface ImportError {
  kind: string;
  remote_id: string;
  label: string;
  message: string;
  at: string;
}

export interface ProviderSettings {
  provider: string;
  client_id: string;
  has_secret: boolean;
  has_service_account: boolean;
  extra: Record<string, string>;
  redirect_url: string;
}

const base = "/api/v1/import/wizard/";

async function call<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const r = await fetch(base + path, {
    method,
    credentials: "same-origin",
    headers:
      body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await r.text();
  let data: unknown = {};
  try {
    data = text ? JSON.parse(text) : {};
  } catch {
    data = {};
  }
  if (!r.ok) {
    const msg = (data as { error?: string }).error || `HTTP ${r.status}`;
    throw new Error(msg);
  }
  return data as T;
}

export const wizardApi = {
  providers: () =>
    call<{ providers: Provider[]; is_admin: boolean; redirect_url: string }>(
      "GET",
      "providers",
    ),
  getSettings: (p: string) => call<ProviderSettings>("GET", `settings/${p}`),
  saveSettings: (
    p: string,
    v: {
      client_id: string;
      client_secret?: string;
      service_account_json?: string;
      extra?: Record<string, string>;
    },
  ) => call<{ ok: boolean }>("PUT", `settings/${p}`, v),
  connections: () =>
    call<{ connections: Connection[] }>("GET", "connections").then(
      (d) => d.connections,
    ),
  createConnection: (v: {
    provider: string;
    scope: string;
    label?: string;
    config: Record<string, string>;
  }) => call<Connection>("POST", "connections", v),
  deleteConnection: (id: string) =>
    call<{ ok: boolean }>("DELETE", `connections/${id}`),
  oauthStart: (id: string) =>
    call<{ url: string }>("POST", `connections/${id}/oauth/start`),
  discover: (id: string) =>
    call<{ found: number; matched: number }>(
      "POST",
      `connections/${id}/discover`,
    ),
  users: (id: string) =>
    call<{ users: UserMapRow[] }>("GET", `connections/${id}/users`).then(
      (d) => d.users,
    ),
  setUser: (
    id: string,
    remote: string,
    v: { local_email?: string; local_user_id?: string; skip: boolean },
  ) =>
    call<{ ok: boolean }>(
      "PUT",
      `connections/${id}/users/${encodeURIComponent(remote)}`,
      v,
    ),
  startRun: (id: string, kinds: Kind[], dryRun: boolean) =>
    call<Run>("POST", `connections/${id}/runs`, { kinds, dry_run: dryRun }),
  runs: (id: string) =>
    call<{ runs: Run[] }>("GET", `connections/${id}/runs`).then((d) => d.runs),
  run: (id: string) => call<Run>("GET", `runs/${id}`),
  cancel: (id: string) => call<{ status: string }>("POST", `runs/${id}/cancel`),
  errors: (id: string) =>
    call<{ errors: ImportError[] }>("GET", `runs/${id}/errors`).then(
      (d) => d.errors,
    ),
  errorsCsvUrl: (id: string) => `${base}runs/${id}/errors?format=csv`,
};

export const kindLabels: Record<Kind, string> = {
  users: "Users",
  groups: "Groups",
  files: "Drive files",
  mail: "Mail",
  calendar: "Calendar",
  contacts: "Contacts",
  chat: "Chat history",
};
