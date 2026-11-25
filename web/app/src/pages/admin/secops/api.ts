// Client for the identity operations console (internal/iamops). Plain fetch.
// A 428 answer means "confirm your password first" and carries stepUp = true.

export class IamError extends Error {
  status: number;
  stepUp: boolean;
  constructor(message: string, status: number, stepUp = false) {
    super(message);
    this.status = status;
    this.stepUp = stepUp;
  }
}

export async function iam<T>(path: string, init?: Omit<RequestInit, "body"> & { json?: unknown }): Promise<T> {
  const { json, ...rest } = init ?? {};
  const r = await fetch(path, {
    credentials: "same-origin",
    headers: { Accept: "application/json", ...(json !== undefined ? { "Content-Type": "application/json" } : {}) },
    ...(json !== undefined ? { body: JSON.stringify(json) } : {}),
    ...rest,
  });
  const text = await r.text();
  let body: Record<string, unknown> = {};
  try {
    body = text ? (JSON.parse(text) as Record<string, unknown>) : {};
  } catch {
    /* not json */
  }
  if (!r.ok) {
    throw new IamError(String(body.error ?? `HTTP ${r.status}`), r.status, body.step_up === true);
  }
  return body as T;
}

export const A = "/api/v1/admin/iam";
export const MY = "/api/v1/iam/my";

export interface Me {
  roles: string[];
  super: boolean;
  capabilities: string[];
  step_up_until: string | null;
  email: string;
  user_id: string;
  idp_configured: boolean;
  summary: Summary;
  role_catalogue: { Key: string; Name: string; Description: string }[];
}
export interface Summary {
  Open: number;
  Investigating: number;
  Critical: number;
  High: number;
  Breached: number;
  Today: number;
}
export interface Alert {
  id: string;
  user_id: string;
  email: string;
  ip: string;
  rule: string;
  severity: "low" | "medium" | "high" | "critical";
  status: string;
  title: string;
  evidence: Record<string, unknown>;
  recommended: string;
  assignee_id: string;
  assignee_name: string;
  sla_due_at: string | null;
  sla_breached: boolean;
  resolved_at: string | null;
  created_at: string;
}
export interface SignIn {
  id: number;
  user_id: string;
  email: string;
  ip: string;
  user_agent: string;
  method: string;
  success: boolean;
  reason: string;
  country: string;
  city: string;
  mfa_result: string;
  at: string;
}
export interface Comment {
  id: number;
  author: string;
  kind: string;
  body: string;
  created_at: string;
}
export interface Person {
  id: string;
  email: string;
  name: string;
  status: string;
  type: string;
  is_admin: boolean;
  quarantined: boolean;
  last_sign_in: string | null;
  open_alerts: number;
  sessions: number;
}
export interface Factor {
  id: string;
  kind: string;
  name: string;
  state: string;
}
export interface SessionRow {
  id: string;
  ip: string;
  device: string;
  country: string;
  city: string;
  created_at: string;
  last_seen_at: string | null;
  current: boolean;
}
export interface ActionRow {
  id: number;
  actor: string;
  actor_role: string;
  target_email: string;
  action: string;
  reason: string;
  ticket: string;
  status: string;
  created_at: string;
}
export interface SecurityView {
  person: Person;
  factors: Factor[];
  factors_error?: string;
  methods: string[];
  idp_links: { idp_id: string; idp_name: string; user_name: string }[];
  idp_state: string;
  sessions: SessionRow[];
  tokens: { id: string; name: string; prefix: string; last_used_at: string | null; created_at: string }[];
  events: SignIn[];
  alerts: Alert[];
  actions: ActionRow[];
  quarantine_reason?: string;
  expires_at: string | null;
  blocked_reason: string;
  capabilities: string[];
  can: { allowed: boolean; self: boolean };
}
export interface Step {
  name: string;
  status: string;
  note: string;
}
export interface ActionResult {
  status: "done" | "dry_run" | "pending_approval";
  message: string;
  steps?: Step[];
  secret?: string;
  secret_label?: string;
  approval_id?: string;
}
export interface IamSettings {
  thresholds: Record<string, number | boolean>;
  anon_networks: string[];
  security_mailbox: string;
  webhook_url: string;
  notify_manager: boolean;
  mfa_policy: { mode: string };
  require_approval: string[];
}
